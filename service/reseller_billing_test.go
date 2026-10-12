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
