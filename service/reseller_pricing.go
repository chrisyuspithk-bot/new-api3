package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// ResolveResellerFactors returns the reseller cost factor (relative to the base
// quota) and the customer markup for one model.
//
// cost   = base x costFactor
// charge = cost x markup
//
// Markup precedence: customer x model, customer, reseller x model, reseller default.
func ResolveResellerFactors(customerId, resellerId int, modelName string) (costFactor float64, markup float64, err error) {
	cfg, err := model.GetResellerPricingConfig(resellerId)
	if err != nil {
		return 0, 0, err
	}
	if cfg.Role != common.RoleResellerUser || cfg.Status != common.UserStatusEnabled {
		return 0, 0, model.ErrResellerDisabled
	}
	costFactor = 1
	if override, ok := cfg.ModelPrices[modelName]; ok {
		costFactor = resellerCostFactor(override, modelName)
	}

	markup = cfg.DefaultMarkup
	if m, ok := cfg.ModelMarkups[modelName]; ok && model.ValidMarkup(m) {
		markup = m
	}
	customer, err := model.GetCustomerPricingConfig(customerId)
	if err != nil {
		return 0, 0, err
	}
	if customer.Markup != nil && model.ValidMarkup(*customer.Markup) {
		markup = *customer.Markup
	}
	if m, ok := customer.ModelMarkups[modelName]; ok && model.ValidMarkup(m) {
		markup = m
	}
	if !model.ValidMarkup(markup) {
		return 0, 0, model.ErrInvalidMarkup
	}
	return costFactor, markup, nil
}

// resellerCostFactor converts an admin cost override into a multiplier on the
// base quota. Phase 1 only re-scales the base engine result, so tiered/expression
// billing is supported through cost_multiplier and the ratio/price overrides
// apply as ratio-of-ratios. completion_ratio overrides are stored but not applied.
func resellerCostFactor(override model.ResellerModelPrice, modelName string) float64 {
	if override.CostMultiplier != nil {
		return *override.CostMultiplier
	}
	if override.ModelPrice != nil {
		if base, usePrice := ratio_setting.GetModelPrice(modelName, false); usePrice && base > 0 {
			return *override.ModelPrice / base
		}
	}
	if override.ModelRatio != nil {
		if base, ok, _ := ratio_setting.GetModelRatio(modelName); ok && base > 0 {
			return *override.ModelRatio / base
		}
	}
	return 1
}

// ApplyResellerPricing folds reseller cost factor and markup into the group
// ratio, so the quota computed by every billing path is the customer charge.
// Direct users (ResellerId == 0) are untouched.
func ApplyResellerPricing(info *relaycommon.RelayInfo, groupRatioInfo *hosttypes.GroupRatioInfo) {
	if info == nil || info.ResellerId <= 0 {
		return
	}
	costFactor, markup, err := ResolveResellerFactors(info.UserId, info.ResellerId, info.GetBillingModelName())
	if err != nil {
		info.ResellerPricingErr = err
		return
	}
	// Defense in depth: the final (possibly auto-selected) group must be in
	// the reseller whitelist.
	if cfg, cfgErr := model.GetResellerPricingConfig(info.ResellerId); cfgErr != nil || !cfg.AllowsGroup(info.UsingGroup) {
		info.ResellerPricingErr = model.ErrGroupNotAllowed
		return
	}
	info.ResellerPricingErr = nil
	info.ResellerCostFactor = costFactor
	info.ResellerMarkup = markup
	groupRatioInfo.GroupRatio = groupRatioInfo.GroupRatio * costFactor * markup
}

// ResellerCostFromCharge derives the reseller cost leg from the customer charge:
// cost = ceil(charge / markup). A zero charge costs nothing.
func ResellerCostFromCharge(charge int, markup float64) int {
	if charge <= 0 {
		return 0
	}
	if !model.ValidMarkup(markup) {
		return charge
	}
	return common.QuotaFromDecimal(decimal.NewFromInt(int64(charge)).Div(decimal.NewFromFloat(markup)).Ceil())
}

// ErrResellerInsufficient is returned when the reseller cost balance cannot
// cover a customer request. Customers see a generic insufficient-quota message.
var ErrResellerInsufficient = errors.New("reseller cost balance insufficient")
