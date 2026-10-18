package service

import (
	"context"
	"math/rand"
	"net/http/httptest"
	"sync"
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

// Financial audit tests for per-call revenue records (B2B2C).

func cleanRevenue(t *testing.T) {
	t.Helper()
	model.DB.Exec("DELETE FROM revenue_records")
	model.DB.Exec("DELETE FROM revenue_monthly_snapshots")
	model.DB.Exec("DELETE FROM logs")
	model.DB.Exec("DELETE FROM reseller_ledger")
	model.DB.Exec("DELETE FROM profit_credit_accumulators")
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM reseller_ledger")
		model.DB.Exec("DELETE FROM profit_credit_accumulators")
		model.DB.Exec("DELETE FROM revenue_records")
		model.DB.Exec("DELETE FROM revenue_monthly_snapshots")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM quota_refund_journal")
	})
}

func setCostFactor(t *testing.T, cf float64) {
	t.Helper()
	require.NoError(t, model.DB.Model(&model.ResellerProfile{}).Where("user_id = ?", b2bResellerId).Update("default_cost_factor", cf).Error)
	model.InvalidateResellerCache(b2bResellerId)
	model.InvalidateCustomerPricingCache(b2bCustomerId)
}

func revenueRows(t *testing.T) []model.RevenueRecord {
	t.Helper()
	var rows []model.RevenueRecord
	require.NoError(t, model.DB.Order("id").Find(&rows).Error)
	return rows
}

// resellerCredited sums reseller_profit_credit ledger entries (profit
// credited back to the reseller wallet after each billed customer call).
func resellerCredited(t *testing.T) int {
	t.Helper()
	var v int64
	require.NoError(t, model.DB.Model(&model.ResellerLedger{}).Where("reseller_id = ? AND kind = ?", b2bResellerId, model.ResellerLedgerResellerProfitCredit).Select("COALESCE(SUM(quota_delta),0)").Scan(&v).Error)
	return int(v)
}

func usedQuota(t *testing.T, id int) (int, int) {
	t.Helper()
	var u model.User
	require.NoError(t, model.DB.Select("used_quota, request_count").First(&u, id).Error)
	return u.UsedQuota, u.RequestCount
}

// recordConsume mimics the relay: settle, then write the consume log.
func settleAndLog(t *testing.T, ctx *gin.Context, info *relaycommon.RelayInfo, actual int) {
	t.Helper()
	require.NoError(t, SettleBilling(ctx, info, actual))
	model.RecordConsumeLog(ctx, info.UserId, model.RecordConsumeLogParams{ModelName: "b2b-model", Quota: actual, IsStream: true})
}

func TestRevenueCustomerCallDualLegRecordedInOneRow(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 100000, 100000, 1.2, common.UserStatusEnabled)
	setCostFactor(t, 1.25)
	ctx, info := newResellerRelayInfo(t)
	require.InDelta(t, 1.5, info.PriceData.GroupRatioInfo.GroupRatio, 1e-9, "base x 1.25 x 1.2")

	r0, c0 := quotaOf(t, b2bResellerId), quotaOf(t, b2bCustomerId)
	used0, req0 := usedQuota(t, b2bResellerId)
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	settleAndLog(t, ctx, info, 150) // base 100 -> cost 125 -> price 150

	assert.Equal(t, c0-150, quotaOf(t, b2bCustomerId), "customer pays price")
	assert.Equal(t, 25, resellerCredited(t), "reseller profit credited")
	assert.Equal(t, r0-125+25, quotaOf(t, b2bResellerId), "reseller pays base x costFactor exactly, then is credited its profit")
	used1, req1 := usedQuota(t, b2bResellerId)
	assert.Equal(t, used0+125, used1, "reseller used_quota tracks customer usage cost")
	assert.Equal(t, req0+1, req1, "reseller request_count incremented")

	rows := revenueRows(t)
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, model.RevenuePayerCustomer, r.PayerType)
	assert.Equal(t, b2bResellerId, r.ResellerId)
	assert.Equal(t, b2bCustomerId, r.UserId)
	assert.Equal(t, int64(150), r.CustomerPrice)
	assert.Equal(t, int64(125), r.ResellerCost)
	assert.Equal(t, int64(25), r.ResellerProfit)
	assert.Equal(t, int64(125), r.AdminRevenue)
	assert.Equal(t, int64(100_000_000), r.BaseQuotaMicro, "base = 150/(1.25*1.2) = 100")
	assert.Equal(t, int64(25_000_000), r.AdminProfitMicro, "admin margin = base x (cf-1) = 25")
	assert.InDelta(t, 1.25, r.CostFactor, 1e-9)
	assert.InDelta(t, 1.2, r.Markup, 1e-9)
	assert.True(t, r.IdentityHolds())
	assert.Greater(t, r.LogId, int64(0), "linked to its log, written in the same transaction")
	assert.Equal(t, "log:"+itoa(r.LogId), r.DedupeKey)

	var lg model.Log
	require.NoError(t, model.DB.First(&lg, r.LogId).Error)
	assert.Equal(t, 150, lg.Quota)
	assert.Equal(t, 125, lg.ResellerCostQuota)
	assert.Contains(t, lg.Other, `"revenue_ctx"`, "factors kept admin-only in log for reconciliation")
}

func itoa(v int64) string { return model.FormatInt64(v) }

func TestRevenueResellerSelfCallRecordsResellerId(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	setCostFactor(t, 1.2)
	info := &relaycommon.RelayInfo{
		UserId: b2bResellerId, ResellerSelf: true, IsPlayground: true, OriginModelName: "b2b-model", UsingGroup: "default", UserGroup: "default",
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, ForcePreConsume: true,
		StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAI,
	}
	gr := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &gr)
	info.PriceData = hosttypes.PriceData{GroupRatioInfo: gr}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	settleAndLog(t, ctx, info, 48)
	assert.Equal(t, 10000-48, quotaOf(t, b2bResellerId))

	rows := revenueRows(t)
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, model.RevenuePayerResellerSelf, r.PayerType)
	assert.Equal(t, b2bResellerId, r.ResellerId, "self-call rows carry the reseller id")
	assert.Equal(t, int64(48), r.CustomerPrice)
	assert.Equal(t, int64(48), r.ResellerCost)
	assert.Equal(t, int64(0), r.ResellerProfit)
	assert.Equal(t, int64(40_000_000), r.BaseQuotaMicro, "48/1.2")
	assert.Equal(t, int64(8_000_000), r.AdminProfitMicro)
	assert.True(t, r.IdentityHolds())

	var lg model.Log
	require.NoError(t, model.DB.First(&lg, r.LogId).Error)
	assert.Zero(t, lg.ResellerId, "log keeps legacy semantics: no reseller leg on self-call")
}

func TestRevenueDirectUserRow(t *testing.T) {
	cleanRevenue(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", nil)
	SetRevenueContextFromRelayInfo(ctx, &relaycommon.RelayInfo{UserId: 77})
	model.RecordConsumeLog(ctx, 77, model.RecordConsumeLogParams{ModelName: "m", Quota: 333})
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, model.RevenuePayerDirect, rows[0].PayerType)
	assert.Equal(t, int64(333), rows[0].AdminRevenue)
	assert.Equal(t, int64(0), rows[0].AdminProfitMicro)
	assert.True(t, rows[0].IdentityHolds())
}

func TestRevenueNotWrittenWithoutBillingContext(t *testing.T) {
	cleanRevenue(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", nil)
	// e.g. the admin channel test: logged, never billed.
	model.RecordConsumeLog(ctx, 1, model.RecordConsumeLogParams{ModelName: "m", Quota: 10, TokenName: "Model test"})
	assert.Empty(t, revenueRows(t))
}

func TestRevenueWrittenEvenWhenConsumeLogDisabled(t *testing.T) {
	cleanRevenue(t)
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = true })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/", nil)
	SetRevenueContextFromRelayInfo(ctx, &relaycommon.RelayInfo{UserId: 78})
	model.RecordConsumeLog(ctx, 78, model.RecordConsumeLogParams{ModelName: "m", Quota: 10})
	var n int64
	model.DB.Model(&model.Log{}).Count(&n)
	assert.Zero(t, n)
	require.Len(t, revenueRows(t), 1)
}

func TestRevenueFailedRequestRefundsBothLegsSynchronouslyNoRow(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	info.RequestId = "req-refund-1"
	require.Nil(t, PreConsumeBilling(ctx, 200, info))
	assert.Equal(t, 900, quotaOf(t, b2bResellerId))
	assert.Equal(t, 9800, quotaOf(t, b2bCustomerId))
	info.Billing.Refund(ctx)
	// Immediate: no Eventually needed.
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId))
	assert.Empty(t, revenueRows(t), "nothing billed, nothing booked")
	// Calling Refund again is a no-op.
	info.Billing.Refund(ctx)
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId))
}

func TestRefundUserQuotaOnceIsIdempotentUnderRetry(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	require.NoError(t, model.RefundUserQuotaOnce(b2bResellerId, 50, "k1"))
	require.NoError(t, model.RefundUserQuotaOnce(b2bResellerId, 50, "k1"))
	assert.Equal(t, 1050, quotaOf(t, b2bResellerId), "same key credits once")
	require.NoError(t, model.RefundUserQuotaOnce(b2bResellerId, 50, "k2"))
	assert.Equal(t, 1100, quotaOf(t, b2bResellerId))
}

func TestRevenueStreamingTopUpThenSettle(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 100000, 100000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	require.NoError(t, info.Billing.Reserve(400)) // mid-stream reservation grows
	assert.Equal(t, 100000-200, quotaOf(t, b2bResellerId))
	settleAndLog(t, ctx, info, 301) // final usage below the reservation
	assert.Equal(t, 100000-151+150, quotaOf(t, b2bResellerId), "round(301/2)=151 cost, 150 profit credited")
	assert.Equal(t, 100000-301, quotaOf(t, b2bCustomerId))
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(301), rows[0].CustomerPrice)
	assert.Equal(t, int64(151), rows[0].ResellerCost)
	assert.True(t, rows[0].IdentityHolds())
}

func TestRevenueInsufficientResellerBlocksCustomerAndBooksNothing(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 40, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	apiErr := PreConsumeBilling(ctx, 200, info) // needs 100 from reseller
	require.NotNil(t, apiErr)
	assert.Equal(t, 40, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId))
	assert.Empty(t, revenueRows(t))
}

func TestRevenueZeroOrInvalidFactorsFailClosed(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 1000, 2, common.UserStatusEnabled)
	zero := 0.0 // legacy row written before > 0 validation existed
	require.NoError(t, model.DB.Create(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &zero}).Error)
	model.InvalidateResellerCache(b2bResellerId)
	ctx, info := newResellerRelayInfo(t)
	require.Error(t, info.ResellerPricingErr, "zero cost factor never prices a call")
	require.NotNil(t, PreConsumeBilling(ctx, 10, info), "and the call is refused")
	assert.Empty(t, revenueRows(t))

	for _, rc := range []model.RevenueContext{
		{PayerType: model.RevenuePayerCustomer, ResellerId: 1, CostFactor: 0, Markup: 1.2},
		{PayerType: model.RevenuePayerCustomer, ResellerId: 1, CostFactor: 1.2, Markup: 0},
		{PayerType: model.RevenuePayerCustomer, ResellerId: 0, CostFactor: 1.2, Markup: 1.2},
		{PayerType: model.RevenuePayerResellerSelf, ResellerId: 1, CostFactor: -1},
	} {
		r := model.BuildRevenueRecord(rc, 5, "m", model.RevenueKindConsume, 100, 80)
		assert.Equal(t, model.RevenueFlagInvalidFactors, r.Flags, "%+v", rc)
		assert.Zero(t, r.BaseQuotaMicro, "no invented base")
		assert.Zero(t, r.AdminProfitMicro, "no invented margin")
		assert.True(t, r.IdentityHolds())
	}
}

func TestRevenueRoundingIdentityProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 50000; i++ {
		cf := []float64{1, 1.1, 1.2, 1.25, 1.333, 2.5}[i%6]
		m := []float64{1, 1.15, 1.2, 1.5, 3, 0.9}[(i/6)%6]
		charge := 1 + rng.Intn(3_000_000)
		cost := ResellerCostFromCharge(charge, m)
		r := model.BuildRevenueRecord(model.RevenueContext{PayerType: model.RevenuePayerCustomer, ResellerId: 1, CostFactor: cf, Markup: m},
			2, "m", model.RevenueKindConsume, int64(charge), int64(cost))
		require.True(t, r.IdentityHolds(), "charge=%d cf=%v m=%v", charge, cf, m)
		// reseller cost = base x costFactor to the nearest quota unit.
		baseTimesCf := float64(r.BaseQuotaMicro) / 1e6 * cf
		require.InDelta(t, baseTimesCf, float64(cost), 0.5+1e-6+cf*1e-6, "charge=%d cf=%v m=%v", charge, cf, m)
		neg := model.BuildRevenueRecord(model.RevenueContext{PayerType: model.RevenuePayerCustomer, ResellerId: 1, CostFactor: cf, Markup: m},
			2, "m", model.RevenueKindRefund, int64(charge), int64(cost))
		require.Equal(t, -r.CustomerPrice, neg.CustomerPrice)
		require.Equal(t, -r.AdminProfitMicro, neg.AdminProfitMicro)
		require.True(t, neg.IdentityHolds())
	}
}

func TestRevenueTaskRefundWritesNegativeRow(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", b2bResellerId).Update("quota", 850).Error)
	task := seedResellerTask(t, 300, 2)
	require.True(t, RefundTaskQuota(context.Background(), task, "failed"))
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, model.RevenueKindRefund, r.Kind)
	assert.Equal(t, int64(-300), r.CustomerPrice)
	assert.Equal(t, int64(-150), r.ResellerCost)
	assert.Equal(t, int64(-150), r.ResellerProfit)
	assert.True(t, r.IdentityHolds())
}

func TestRevenueTaskRecalculateUpAndDown(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 850, 10000, 2, common.UserStatusEnabled)
	task := seedResellerTask(t, 300, 2)
	task.PrivateData.BillingContext.ProfitCredit = true // submitted post-deploy
	require.NoError(t, model.DB.Save(task).Error)
	RecalculateTaskQuota(context.Background(), task, 501, common.NewMessage("up"))
	RecalculateTaskQuota(context.Background(), task, 100, common.NewMessage("down"))
	rows := revenueRows(t)
	require.Len(t, rows, 2)
	assert.Equal(t, model.RevenueKindConsume, rows[0].Kind)
	assert.Equal(t, int64(201), rows[0].CustomerPrice)
	assert.Equal(t, int64(101), rows[0].ResellerCost) // round(501/2)=251 - 150
	assert.Equal(t, model.RevenueKindRefund, rows[1].Kind)
	assert.Equal(t, int64(-401), rows[1].CustomerPrice)
	assert.Equal(t, int64(-201), rows[1].ResellerCost) // 50 - 251
	var net int64
	for _, r := range rows {
		net += r.ResellerCost
	}
	assert.Equal(t, int64(50-150), net, "net cost delta matches reseller wallet movement")
	assert.Equal(t, 950+resellerCredited(t), quotaOf(t, b2bResellerId))
	assert.Equal(t, (201-101)+(-401+201), resellerCredited(t), "profit credited then reversed")
}

func TestRevenueMidjourneySettleAndRefund(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	info.IsPlayground = true
	task := &model.Midjourney{UserId: b2bCustomerId, MjId: "b2b-mj-rev", ChannelId: 1}
	prepared, err := PrepareMidjourneyTaskBilling(info, task, 400, true)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM midjourneys WHERE mj_id = ?", "b2b-mj-rev") })
	applied, err := SettleMidjourneyTaskBilling(info, task, prepared)
	require.NoError(t, err)
	require.True(t, applied)
	SetResellerLogContext(ctx, task.ResellerId, task.ResellerCostQuota)
	SetMidjourneyRevenueContext(ctx, task)
	model.RecordConsumeLog(ctx, b2bCustomerId, model.RecordConsumeLogParams{ModelName: "mj", Quota: task.Quota})
	require.True(t, RefundMidjourneyQuota(context.Background(), task, "failed"))
	rows := revenueRows(t)
	require.Len(t, rows, 2)
	assert.Equal(t, int64(400), rows[0].CustomerPrice)
	assert.Equal(t, int64(200), rows[0].ResellerCost)
	assert.Equal(t, int64(-400), rows[1].CustomerPrice)
	assert.Equal(t, int64(-200), rows[1].ResellerCost)
	assert.Equal(t, model.RevenuePayerCustomer, rows[1].PayerType, "frozen context survives to refund")
}

func TestRevenueConcurrentCustomerCallsNeverOverspendReseller(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 500, 1_000_000, 2, common.UserStatusEnabled)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, info := newResellerRelayInfo(t)
			if PreConsumeBilling(ctx, 100, info) != nil { // 50 reseller each
				return
			}
			require.NoError(t, SettleBilling(ctx, info, 100))
			model.RecordConsumeLog(ctx, info.UserId, model.RecordConsumeLogParams{ModelName: "b2b-model", Quota: 100})
			mu.Lock()
			ok++
			mu.Unlock()
		}()
	}
	wg.Wait()
	// Profit credits (50 per call at markup 2) may admit more calls; the
	// reserve must still never take the wallet negative.
	assert.GreaterOrEqual(t, ok, 10)
	assert.GreaterOrEqual(t, quotaOf(t, b2bResellerId), 0, "never negative from reserve")
	rows := revenueRows(t)
	assert.Len(t, rows, ok)
	var cost int64
	for _, r := range rows {
		cost += r.ResellerCost
	}
	assert.Equal(t, int64(500)-cost+int64(resellerCredited(t)), int64(quotaOf(t, b2bResellerId)), "revenue rows + credits reconcile to the reseller wallet")
}

func TestRevenueRealtimeFallbackPathRecordsResellerLeg(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	// Realtime: no BillingSession; PreWssConsumeQuota already moved both legs.
	info.Billing = nil
	info.FinalPreConsumedQuota = 120
	info.ResellerCostQuota = 60
	require.NoError(t, SettleBilling(ctx, info, 120))
	model.RecordConsumeLog(ctx, info.UserId, model.RecordConsumeLogParams{ModelName: "rt", Quota: 120})
	rows := revenueRows(t)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(120), rows[0].CustomerPrice)
	assert.Equal(t, int64(60), rows[0].ResellerCost, "reseller leg moved by realtime is booked")
	var lg model.Log
	require.NoError(t, model.DB.First(&lg, rows[0].LogId).Error)
	assert.Equal(t, 60, lg.ResellerCostQuota, "log also carries the reseller leg now")
}
