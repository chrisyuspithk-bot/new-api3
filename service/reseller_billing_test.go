package service

import (
	"context"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	b2bResellerId = 9101
	b2bCustomerId = 9102
	b2bTokenId    = 9103
)

func seedResellerLine(t *testing.T, resellerQuota, customerQuota int, defaultMarkup float64, resellerStatus int) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM users WHERE id IN (?, ?)", b2bResellerId, b2bCustomerId)
		model.DB.Exec("DELETE FROM tokens WHERE id = ?", b2bTokenId)
		model.DB.Exec("DELETE FROM reseller_profiles")
		model.DB.Exec("DELETE FROM reseller_groups")
		model.DB.Exec("DELETE FROM reseller_model_prices")
		model.DB.Exec("DELETE FROM reseller_model_markups")
		model.DB.Exec("DELETE FROM customer_model_markups")
		model.InvalidateResellerCache(b2bResellerId)
		model.InvalidateCustomerPricingCache(b2bCustomerId)
	})
	require.NoError(t, model.DB.Create(&model.User{Id: b2bResellerId, Username: "reseller", Role: common.RoleResellerUser, Status: resellerStatus, Quota: resellerQuota, AffCode: "r1"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: b2bCustomerId, Username: "customer", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: customerQuota, ResellerId: b2bResellerId, Group: "default", AffCode: "c1"}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: b2bTokenId, UserId: b2bCustomerId, Key: "b2b-token", Name: "t", Status: common.TokenStatusEnabled, RemainQuota: customerQuota}).Error)
	require.NoError(t, model.DB.Create(&model.ResellerProfile{UserId: b2bResellerId, DefaultMarkup: defaultMarkup, AllowRedemption: true}).Error)
	require.NoError(t, model.DB.Create(&model.ResellerGroup{ResellerId: b2bResellerId, GroupName: "default"}).Error)
	model.InvalidateResellerCache(b2bResellerId)
	model.InvalidateCustomerPricingCache(b2bCustomerId)
}

func newResellerRelayInfo(t *testing.T) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	info := &relaycommon.RelayInfo{
		UserId: b2bCustomerId, ResellerId: b2bResellerId, TokenId: b2bTokenId, TokenKey: "b2b-token",
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

func quotaOf(t *testing.T, id int) int {
	t.Helper()
	q, err := model.GetUserQuota(id, true)
	require.NoError(t, err)
	return q
}

func TestResellerDualLegReserveSettleAndRefund(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	assert.InDelta(t, 2.0, info.PriceData.GroupRatioInfo.GroupRatio, 1e-9, "markup is folded into the group ratio")

	require.Nil(t, PreConsumeBilling(ctx, 200, info))
	assert.Equal(t, 900, quotaOf(t, b2bResellerId), "reseller reserves ceil(200/2)")
	assert.Equal(t, 9800, quotaOf(t, b2bCustomerId))

	require.NoError(t, SettleBilling(ctx, info, 301))
	assert.Equal(t, 849, quotaOf(t, b2bResellerId), "reseller settles ceil(301/2)=151")
	assert.Equal(t, 9699, quotaOf(t, b2bCustomerId))
	assert.Equal(t, 151, info.ResellerCostQuota)
	assert.Equal(t, b2bResellerId, ctx.GetInt(model.ContextKeyLogResellerId))

	// A second request that fails is refunded on both legs.
	ctx2, info2 := newResellerRelayInfo(t)
	require.Nil(t, PreConsumeBilling(ctx2, 100, info2))
	assert.Equal(t, 799, quotaOf(t, b2bResellerId))
	info2.Billing.Refund(ctx2)
	require.Eventually(t, func() bool {
		return quotaOf(t, b2bResellerId) == 849 && quotaOf(t, b2bCustomerId) == 9699
	}, 2*time.Second, 20*time.Millisecond)
}

func TestResellerInsufficientBalanceRollsBackNothingCharged(t *testing.T) {
	seedResellerLine(t, 50, 10000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)

	apiErr := PreConsumeBilling(ctx, 200, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, 50, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId))
	var token model.Token
	require.NoError(t, model.DB.First(&token, b2bTokenId).Error)
	assert.Equal(t, 10000, token.RemainQuota)
}

func TestResellerCustomerInsufficientReleasesResellerLeg(t *testing.T) {
	seedResellerLine(t, 1000, 100, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)

	apiErr := PreConsumeBilling(ctx, 200, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId), "reseller reservation is rolled back")
	assert.Equal(t, 100, quotaOf(t, b2bCustomerId))
}

func TestDisabledResellerFreezesCustomerLine(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusDisabled)
	ctx, info := newResellerRelayInfo(t)
	require.ErrorIs(t, info.ResellerPricingErr, model.ErrResellerDisabled)

	apiErr := PreConsumeBilling(ctx, 200, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, 403, apiErr.StatusCode)
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId))
}

func TestResellerGroupOutsideWhitelistIsRejected(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	info := &relaycommon.RelayInfo{UserId: b2bCustomerId, ResellerId: b2bResellerId, OriginModelName: "b2b-model", UsingGroup: "vip"}
	groupRatio := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &groupRatio)
	require.ErrorIs(t, info.ResellerPricingErr, model.ErrGroupNotAllowed)
}

func TestResellerMarkupPrecedenceAndCostOverride(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1.5, common.UserStatusEnabled)
	multiplier := 0.8
	require.NoError(t, model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &multiplier}))

	cases := []struct {
		name   string
		setup  func()
		markup float64
	}{
		{"reseller default", func() {}, 1.5},
		{"reseller per-model", func() { require.NoError(t, model.UpsertResellerModelMarkup(b2bResellerId, "b2b-model", 1.8)) }, 1.8},
		{"customer markup", func() {
			require.NoError(t, model.UpdateResellerCustomerFields(b2bCustomerId, map[string]any{"markup": 2.5}))
		}, 2.5},
		{"customer per-model", func() {
			require.NoError(t, model.UpsertCustomerModelMarkup(b2bResellerId, b2bCustomerId, "b2b-model", 0.9))
		}, 0.9},
	}
	for _, tc := range cases {
		tc.setup()
		costFactor, markup, err := ResolveResellerFactors(b2bCustomerId, b2bResellerId, "b2b-model")
		require.NoError(t, err, tc.name)
		assert.InDelta(t, 0.8, costFactor, 1e-9, tc.name)
		assert.InDelta(t, tc.markup, markup, 1e-9, tc.name)
	}
}

func TestResellerCostFromChargeRoundsUp(t *testing.T) {
	assert.Equal(t, 0, ResellerCostFromCharge(0, 2))
	assert.Equal(t, 1, ResellerCostFromCharge(1, 2))
	assert.Equal(t, 34, ResellerCostFromCharge(100, 3))
	assert.Equal(t, 200, ResellerCostFromCharge(100, 0.5), "markup below 1 sells at a loss")
}

func TestResellerSelfCallBillsCostFactorSingleLeg(t *testing.T) {
	seedResellerLine(t, 10000, 10000, 2, common.UserStatusEnabled)
	multiplier := 1.2
	require.NoError(t, model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &multiplier}))

	// The role written by the auth middleware marks a reseller's own call.
	cases := []struct {
		name         string
		user         model.UserBase
		resellerSelf bool
		groupRatio   float64
	}{
		{"reseller self", model.UserBase{Id: b2bResellerId, Role: common.RoleResellerUser}, true, 1.2},
		{"customer", model.UserBase{Id: b2bCustomerId, Role: common.RoleCommonUser, ResellerId: b2bResellerId}, false, 2.4},
		{"admin", model.UserBase{Id: 1, Role: common.RoleAdminUser}, false, 1},
	}
	for _, tc := range cases {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		ctx.Set("id", tc.user.Id)
		tc.user.WriteContext(ctx)
		info := relaycommon.GenRelayInfoOpenAI(ctx, nil)
		info.OriginModelName, info.UsingGroup = "b2b-model", "default"
		assert.Equal(t, tc.resellerSelf, info.ResellerSelf, tc.name)
		groupRatio := hosttypes.GroupRatioInfo{GroupRatio: 1}
		ApplyResellerPricing(info, &groupRatio)
		require.NoError(t, info.ResellerPricingErr, tc.name)
		assert.InDelta(t, tc.groupRatio, groupRatio.GroupRatio, 1e-9, tc.name)
	}

	// Pre-consume and settle charge only the reseller's own wallet.
	info := &relaycommon.RelayInfo{
		UserId: b2bResellerId, ResellerSelf: true, IsPlayground: true, OriginModelName: "b2b-model", UsingGroup: "default", UserGroup: "default",
		UserSetting: dto.UserSetting{BillingPreference: "wallet_only"}, ForcePreConsume: true,
		StartTime: time.Now(), RelayFormat: types.RelayFormatOpenAI,
	}
	groupRatio := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &groupRatio)
	info.PriceData = hosttypes.PriceData{GroupRatioInfo: groupRatio}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	require.Nil(t, PreConsumeBilling(ctx, 120, info))
	assert.Equal(t, 9880, quotaOf(t, b2bResellerId))
	require.NoError(t, SettleBilling(ctx, info, 240))
	assert.Equal(t, 9760, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId), "no customer leg")
	assert.Equal(t, 0, info.ResellerCostQuota, "no second reseller leg")
	assert.Zero(t, ctx.GetInt(model.ContextKeyLogResellerId))
	require.NoError(t, CheckMidjourneyResellerBilling(info, 4000000))

	other := model.NewLogOther()
	appendBillingInfo(info, other)
	assert.Equal(t, 1.2, other.Snapshot()["reseller_self_cost_factor"])

	// A frozen self cost factor is re-applied when a task is recalculated.
	task := &model.Task{UserId: b2bResellerId, Group: "default"}
	task.PrivateData.BillingContext = &model.TaskBillingContext{ResellerCostFactor: 1.2}
	assert.InDelta(t, 1.2, taskResellerSelfCostFactor(task), 1e-9)
	_, markup, _ := taskResellerLeg(task)
	assert.Zero(t, markup, "self task has no reseller leg")
}

func TestDisabledResellerSelfCallFailsClosed(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusDisabled)
	info := &relaycommon.RelayInfo{UserId: b2bResellerId, ResellerSelf: true, OriginModelName: "b2b-model", UsingGroup: "default"}
	groupRatio := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &groupRatio)
	require.ErrorIs(t, info.ResellerPricingErr, model.ErrResellerDisabled)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	apiErr := PreConsumeBilling(ctx, 100, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, 403, apiErr.StatusCode)
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId))
}

func TestResellerPriceOverridesMustBePositive(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1.2, common.UserStatusEnabled)
	for _, v := range []float64{0, -1, math.Inf(1), math.NaN()} {
		for name, row := range map[string]model.ResellerModelPrice{
			"cost_multiplier": {CostMultiplier: &v},
			"model_ratio":     {ModelRatio: &v},
			"model_price":     {ModelPrice: &v},
		} {
			row.ResellerId, row.ModelName = b2bResellerId, "b2b-model"
			assert.ErrorIs(t, model.UpsertResellerModelPrice(&row), model.ErrInvalidResellerPrice, "%s=%v", name, v)
		}
	}
	zero := 0.0
	require.NoError(t, model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CompletionRatio: &zero}))
}

func TestZeroCostFactorFailsClosedOnEveryPath(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1.2, common.UserStatusEnabled)
	// A legacy row written before the > 0 validation existed.
	zero := 0.0
	require.NoError(t, model.DB.Create(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &zero}).Error)
	model.InvalidateResellerCache(b2bResellerId)

	ctx, info := newResellerRelayInfo(t)
	require.ErrorIs(t, info.ResellerPricingErr, model.ErrInvalidCostFactor)
	apiErr := PreConsumeBilling(ctx, 100, info)
	require.NotNil(t, apiErr)
	assert.Equal(t, 403, apiErr.StatusCode)
	assert.Error(t, CheckMidjourneyResellerBilling(info, 100))

	self := &relaycommon.RelayInfo{UserId: b2bResellerId, ResellerSelf: true, OriginModelName: "b2b-model", UsingGroup: "default"}
	selfRatio := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(self, &selfRatio)
	require.ErrorIs(t, self.ResellerPricingErr, model.ErrInvalidCostFactor)
	assert.NotNil(t, PreConsumeBilling(ctx, 100, self))
	assert.Error(t, CheckMidjourneyResellerBilling(self, 100))

	// Paths that read the resolved factor directly must not treat 0 as 1 or as free.
	forged := &relaycommon.RelayInfo{UserId: b2bCustomerId, ResellerId: b2bResellerId, ResellerMarkup: 1.2, TokenKey: "b2b-token", OriginModelName: "b2b-model", UsingGroup: "default"}
	assert.Error(t, PreWssConsumeQuota(ctx, forged, &dto.RealtimeUsage{TotalTokens: 10}))
	assert.ErrorIs(t, CheckMidjourneyResellerBilling(forged, 100), model.ErrInvalidCostFactor)
	_, forgedErr := NewBillingSession(ctx, forged, 100)
	assert.NotNil(t, forgedErr)

	previousRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRatioByJSONString(previousRatios) })
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"b2b-model":2}`))
	task := &model.Task{UserId: b2bCustomerId, Group: "default", Quota: 500}
	task.PrivateData.BillingContext = &model.TaskBillingContext{ResellerId: b2bResellerId, ResellerMarkup: 1.2, OriginModelName: "b2b-model"}
	assert.False(t, RecalculateTaskQuotaByTokens(context.Background(), task, 1000), "task recalculation must keep the pre-consumed quota")

	assert.Equal(t, 1000, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId))
}
