package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Realtime (WSS) charge-once audit: per-chunk charges + final settle must move
// each wallet exactly once by the single-shot price of the actual total, and
// the revenue row must equal what moved.

type rtState struct{ reseller, customer, token, rUsed, rReq int }

func rtSnap(t *testing.T) rtState {
	t.Helper()
	var tok model.Token
	require.NoError(t, model.DB.First(&tok, b2bTokenId).Error)
	u, r := usedQuota(t, b2bResellerId)
	return rtState{quotaOf(t, b2bResellerId), quotaOf(t, b2bCustomerId), tok.RemainQuota, u, r}
}

func rtChunks(t *testing.T, info *relaycommon.RelayInfo, chunks ...int) {
	t.Helper()
	inc := info.Billing.(*BillingSession)
	for _, c := range chunks {
		require.NoError(t, inc.ConsumeIncrement(c))
	}
}

func assertChargedOnce(t *testing.T, before rtState, total int, markup float64) {
	t.Helper()
	after := rtSnap(t)
	cost := 0
	if total > 0 {
		cost = ResellerCostFromCharge(total, markup)
	}
	assert.Equal(t, total, before.customer-after.customer, "customer charged once")
	assert.Equal(t, total, before.token-after.token, "token charged once")
	assert.Equal(t, cost-resellerCredited(t), before.reseller-after.reseller, "reseller charged once (net of credited profit)")
	if credited := resellerCredited(t); credited != 0 {
		assert.Equal(t, total-cost, credited, "profit credited once")
	}
	assert.Equal(t, cost, after.rUsed-before.rUsed, "reseller used_quota counted once")
}

func TestRealtimeChunksThenSettleChargesOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pre    int
		chunks []int
		final  int
	}{
		{"exact", 0, []int{30, 40}, 70},
		{"final above chunks (top-up)", 0, []int{30, 40}, 101},
		{"final below chunks (refund diff)", 0, []int{30, 40}, 51},
		{"with initial pre-consume", 200, []int{30, 40}, 70},
		{"pre-consume exceeds total", 200, []int{5}, 9},
		{"odd amounts rounding", 0, []int{1, 1, 1}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanRevenue(t)
			seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
			ctx, info := newResellerRelayInfo(t)
			before := rtSnap(t)
			require.Nil(t, PreConsumeBilling(ctx, tc.pre, info))
			rtChunks(t, info, tc.chunks...)
			settleAndLog(t, ctx, info, tc.final)
			assertChargedOnce(t, before, tc.final, 2)
			rows := revenueRows(t)
			require.Len(t, rows, 1, "one revenue row per realtime call")
			assert.Equal(t, int64(tc.final), rows[0].CustomerPrice)
			assert.Equal(t, int64(ResellerCostFromCharge(tc.final, 2)), rows[0].ResellerCost)
			assert.Equal(t, rows[0].CustomerPrice-rows[0].ResellerCost, rows[0].ResellerProfit)
			assert.True(t, rows[0].IdentityHolds())
		})
	}
}

func TestRealtimeDisconnectWithZeroUsageRefundsChunks(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	before := rtSnap(t)
	require.Nil(t, PreConsumeBilling(ctx, 50, info))
	rtChunks(t, info, 20)
	// PostWssConsumeQuota sets quota=0 when upstream reported no tokens.
	require.NoError(t, SettleBilling(ctx, info, 0))
	assertChargedOnce(t, before, 0, 2)
}

func TestRealtimeDisconnectMidStreamChargesConsumedOnly(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	before := rtSnap(t)
	require.Nil(t, PreConsumeBilling(ctx, 0, info))
	rtChunks(t, info, 25, 17)
	// Client dropped: DoResponse returns the summed usage and PostWss settles it.
	settleAndLog(t, ctx, info, 42)
	assertChargedOnce(t, before, 42, 2)
}

func TestRealtimeFailureBeforeSettleRefundsAllChunks(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	info.RequestId = "rt-refund"
	before := rtSnap(t)
	require.Nil(t, PreConsumeBilling(ctx, 60, info))
	rtChunks(t, info, 30, 40)
	info.Billing.Refund(ctx)
	assertChargedOnce(t, before, 0, 2)
	info.Billing.Refund(ctx) // idempotent
	assertChargedOnce(t, before, 0, 2)
	assert.Empty(t, revenueRows(t))
}

func TestRealtimeInsufficientResellerChunkMovesNothingThenSettleOnce(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 20, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	before := rtSnap(t)
	require.Nil(t, PreConsumeBilling(ctx, 0, info))
	rtChunks(t, info, 30) // reseller 15
	err := info.Billing.(*BillingSession).ConsumeIncrement(40)
	require.ErrorIs(t, err, ErrResellerInsufficient)
	mid := rtSnap(t)
	assert.Equal(t, before.customer-30, mid.customer, "failed chunk left customer untouched")
	assert.Equal(t, before.reseller-15, mid.reseller, "failed chunk left reseller untouched")
	// Usage already happened upstream: the final settle charges the total once.
	settleAndLog(t, ctx, info, 70)
	assertChargedOnce(t, before, 70, 2)
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(35), rows[0].ResellerCost)
}

func TestRealtimeInsufficientCustomerChunkRollsBackResellerLeg(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 50, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	before := rtSnap(t)
	require.Nil(t, PreConsumeBilling(ctx, 0, info))
	rtChunks(t, info, 40)
	require.Error(t, info.Billing.(*BillingSession).ConsumeIncrement(40))
	mid := rtSnap(t)
	assert.Equal(t, before.reseller-20, mid.reseller, "reseller reservation for failed chunk released")
	assert.Equal(t, before.customer-40, mid.customer)
}

func TestRealtimeResellerSelfCallChargesOnce(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	setCostFactor(t, 1.2)
	info := &relaycommon.RelayInfo{
		UserId: b2bResellerId, ResellerSelf: true, IsPlayground: true, OriginModelName: "b2b-model", UsingGroup: "default", UserGroup: "default",
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, ForcePreConsume: true,
		StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAIRealtime,
	}
	gr := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &gr)
	info.PriceData = hosttypes.PriceData{GroupRatioInfo: gr}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	require.Nil(t, PreConsumeBilling(ctx, 0, info))
	rtChunks(t, info, 24, 12)
	settleAndLog(t, ctx, info, 36)
	assert.Equal(t, 10000-36, quotaOf(t, b2bResellerId))
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, model.RevenuePayerResellerSelf, rows[0].PayerType)
	assert.Equal(t, int64(36), rows[0].CustomerPrice)
	assert.Equal(t, int64(30_000_000), rows[0].BaseQuotaMicro)
}

func TestRealtimeLegacyNoSessionChargesOnce(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	info.Billing = nil
	info.ResellerMarkup = 2
	info.RelayFormat = types.RelayFormatOpenAIRealtime
	before := rtSnap(t)
	// Simulate the legacy direct per-chunk debits the way PreWssConsumeQuota does.
	for _, c := range []int{30, 41} {
		cost := ResellerCostFromCharge(info.FinalPreConsumedQuota+c, 2) - info.ResellerCostQuota
		require.NoError(t, model.DecreaseUserQuota(b2bResellerId, cost, false))
		require.NoError(t, PostConsumeQuota(info, c, 0, false))
		info.FinalPreConsumedQuota += c
		info.ResellerCostQuota += cost
	}
	settleAndLog(t, ctx, info, 90)
	after := rtSnap(t)
	assert.Equal(t, 90, before.customer-after.customer)
	assert.Equal(t, 45-45, before.reseller-after.reseller, "45 cost, 45 profit credited")
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(45), rows[0].ResellerCost)
}

// End-to-end through PreWssConsumeQuota: the chunk must be routed into the
// session so the final settle of the same total charges nothing more.
func TestRealtimePreWssRoutesThroughSessionNoDoubleCharge(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000000, 10000000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	info.RelayFormat = types.RelayFormatOpenAIRealtime
	before := rtSnap(t)
	require.Nil(t, PreConsumeBilling(ctx, 0, info))
	usage := &dto.RealtimeUsage{TotalTokens: 20, InputTokens: 15, OutputTokens: 5}
	usage.InputTokenDetails.TextTokens = 15
	usage.OutputTokenDetails.TextTokens = 5
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	charged := before.customer - rtSnap(t).customer
	require.Greater(t, charged, 0)
	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	charged = before.customer - rtSnap(t).customer
	settleAndLog(t, ctx, info, charged)
	assertChargedOnce(t, before, charged, 2)
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(charged), rows[0].CustomerPrice)
}

// The session-less reseller true-up is scoped to realtime; other session-less
// paths keep their previous behaviour (no reseller debit from SettleBilling).
func TestSessionlessNonRealtimeSettleDoesNotDebitReseller(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	info.Billing = nil
	before := rtSnap(t)
	require.NoError(t, SettleBilling(ctx, info, 50))
	assert.Equal(t, before.reseller, rtSnap(t).reseller)
}
