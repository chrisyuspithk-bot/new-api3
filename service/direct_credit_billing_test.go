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

// newDirectCreditRelayInfo mirrors genBaseRelayInfo for a direct_credit
// customer: ResellerId cleared, DirectCredit set.
func newDirectCreditRelayInfo(t *testing.T) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	info := &relaycommon.RelayInfo{
		UserId: b2bCustomerId, DirectCredit: true, DirectCreditResellerId: b2bResellerId, TokenId: b2bTokenId, TokenKey: "b2b-token",
		OriginModelName: "b2b-model", UsingGroup: "default", UserGroup: "default",
		UserSetting: dto.UserSetting{BillingPreference: "subscription_first"}, ForcePreConsume: true,
		StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAI,
	}
	groupRatio := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &groupRatio)
	info.PriceData = hosttypes.PriceData{GroupRatioInfo: groupRatio}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return ctx, info
}

func TestDirectCreditCallChargesBaseOnlyNoRevenueRow(t *testing.T) {
	cleanRevenue(t)
	seedResellerLine(t, 1000, 10000, 1.2, common.UserStatusEnabled)
	setCostFactor(t, 1.25)
	ctx, info := newDirectCreditRelayInfo(t)
	assert.InDelta(t, 1.0, info.PriceData.GroupRatioInfo.GroupRatio, 1e-9, "cost factor 1, markup 1")

	r0, c0 := quotaOf(t, b2bResellerId), quotaOf(t, b2bCustomerId)
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	assert.Equal(t, r0, quotaOf(t, b2bResellerId), "no reseller reservation")
	settleAndLog(t, ctx, info, 150)
	assert.Equal(t, c0-150, quotaOf(t, b2bCustomerId), "customer pays base once")
	assert.Equal(t, r0, quotaOf(t, b2bResellerId), "no reseller deduction")
	assert.Zero(t, info.ResellerCostQuota)
	assert.Empty(t, revenueRows(t), "no revenue row, no profit split")
	assert.Zero(t, resellerCredited(t))
	var lg model.Log
	require.NoError(t, model.DB.Order("id desc").First(&lg).Error)
	assert.Equal(t, 150, lg.Quota)
	assert.Zero(t, lg.ResellerCostQuota)
	assert.NotContains(t, lg.Other, `"revenue_ctx"`, "reconciler cannot rebuild a row")

	// Failed request: refunded in full, still nothing booked.
	ctx2, info2 := newDirectCreditRelayInfo(t)
	require.Nil(t, PreConsumeBilling(ctx2, 100, info2))
	info2.Billing.Refund(ctx2)
	require.Eventually(t, func() bool { return quotaOf(t, b2bCustomerId) == c0-150 }, 2*time.Second, 20*time.Millisecond)
	assert.Equal(t, r0, quotaOf(t, b2bResellerId))
	assert.Empty(t, revenueRows(t))
}

func TestDirectCreditRevenueContexts(t *testing.T) {
	info := &relaycommon.RelayInfo{UserId: 5, DirectCredit: true}
	assert.Equal(t, model.RevenuePayerNone, RevenueContextFromRelayInfo(info).PayerType)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	SetRevenueContextFromRelayInfo(ctx, info)
	_, ok := model.GetRevenueContext(ctx)
	assert.False(t, ok)
	task := &model.Task{UserId: 5}
	task.PrivateData.BillingContext = &model.TaskBillingContext{DirectCredit: true, ProfitCredit: true}
	assert.Nil(t, taskRevenueContext(task), "direct-credit tasks never book a row")
	assert.Nil(t, midjourneyRevenueContext(&model.Midjourney{Revenue: `{"payer":"none"}`}))
	assert.NotNil(t, midjourneyRevenueContext(&model.Midjourney{Revenue: ""}), "legacy MJ unchanged")
}
