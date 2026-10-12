package controller

import (
	"fmt"
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

// applyResellerPricingView rewrites the pricing table for B2B2C viewers.
// Customers see only their marked-up price (cost factor x markup folded into
// model_ratio / model_price and into billing_expr unit prices); resellers see
// their own cost. Plugin variants / usage schemas that would leak base prices
// are stripped, and groups are limited to the reseller whitelist. Direct users
// and anonymous visitors are untouched.
func applyResellerPricingView(userId int, pricing []model.Pricing, groupRatio map[string]float64, usableGroup map[string]string) ([]model.Pricing, map[string]float64, map[string]string) {
	user, err := model.GetUserCache(userId)
	if err != nil {
		return pricing, groupRatio, usableGroup
	}
	resellerId, customerId := user.ResellerId, user.Id
	if user.Role == common.RoleResellerUser {
		resellerId, customerId = user.Id, 0
	}
	if resellerId <= 0 {
		return pricing, groupRatio, usableGroup
	}
	cfg, err := model.GetResellerPricingConfig(resellerId)
	if err != nil {
		return []model.Pricing{}, map[string]float64{}, map[string]string{}
	}
	for g := range usableGroup {
		if !cfg.AllowsGroup(g) {
			delete(usableGroup, g)
			delete(groupRatio, g)
		}
	}
	for g := range groupRatio {
		if _, ok := usableGroup[g]; !ok {
			delete(groupRatio, g)
		}
	}
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	out := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		factor, completion, ok := service.ResellerPriceFactor(customerId, resellerId, item.ModelName)
		if !ok {
			continue
		}
		item.ModelRatio *= factor
		item.ModelPrice *= factor
		if completion != nil && item.QuotaType == 0 {
			item.CompletionRatio = *completion
		}
		// Tiered $/1M coefficients live in billing_expr. Keep and scale them so
		// the pricing UI shows costFactor×markup dollars instead of falling back
		// to model_ratio×2 (which is wrong when model_ratio is only a shadow
		// ratio for a tiered model). Plugin variants still hide base prices.
		if strings.TrimSpace(item.BillingExpr) != "" {
			if scaled, err := billingexpr.ScalePriceLiterals(item.BillingExpr, factor); err == nil {
				item.BillingExpr = scaled
			} else {
				common.SysLog(fmt.Sprintf("scale billing_expr for %s (factor=%g): %v", item.ModelName, factor, err))
				item.BillingExpr = ""
			}
		}
		item.BillingPluginVariants = nil
		item.BillingUsageSchema = nil
		item.BillingUsageExamples = nil
		out = append(out, item)
	}
	return out, groupRatio, usableGroup
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	maps.Copy(groupRatio, ratio_setting.GetGroupRatioCopy())
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	if exists {
		pricing, groupRatio, usableGroup = applyResellerPricingView(userId.(int), pricing, groupRatio, usableGroup)
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Model ratios reset", nil)
}
