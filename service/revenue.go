package service

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// RevenueContextFromRelayInfo snapshots who pays and the factors applied.
func RevenueContextFromRelayInfo(info *relaycommon.RelayInfo) model.RevenueContext {
	rc := revenueContextFromRelayInfo(info)
	rc.ProfitCredit = true
	return rc
}

func revenueContextFromRelayInfo(info *relaycommon.RelayInfo) model.RevenueContext {
	if info == nil {
		return model.RevenueContext{PayerType: model.RevenuePayerDirect}
	}
	if info.DirectCredit {
		// Booked at top-up time; calls write no revenue row.
		return model.RevenueContext{PayerType: model.RevenuePayerNone}
	}
	if info.ResellerId > 0 {
		return model.RevenueContext{PayerType: model.RevenuePayerCustomer, ResellerId: info.ResellerId,
			CostFactor: info.ResellerCostFactor, Markup: info.ResellerMarkup}
	}
	if info.ResellerSelf {
		return model.RevenueContext{PayerType: model.RevenuePayerResellerSelf, ResellerId: info.UserId,
			CostFactor: info.ResellerCostFactor, Markup: 1}
	}
	return model.RevenueContext{PayerType: model.RevenuePayerDirect}
}

// SetRevenueContextFromRelayInfo makes the next consume log on c also write a
// revenue record.
func SetRevenueContextFromRelayInfo(c *gin.Context, info *relaycommon.RelayInfo) {
	if info != nil && info.DirectCredit {
		return
	}
	model.SetRevenueContext(c, RevenueContextFromRelayInfo(info))
}

// taskRevenueContext rebuilds the revenue context frozen at task submit.
func taskRevenueContext(task *model.Task) *model.RevenueContext {
	if task == nil {
		return nil
	}
	bc := task.PrivateData.BillingContext
	if bc != nil && bc.DirectCredit {
		return nil
	}
	rc := model.RevenueContext{PayerType: model.RevenuePayerDirect}
	if bc != nil {
		if bc.ResellerId > 0 {
			rc = model.RevenueContext{PayerType: model.RevenuePayerCustomer, ResellerId: bc.ResellerId, CostFactor: bc.ResellerCostFactor, Markup: bc.ResellerMarkup}
		} else if bc.ResellerCostFactor > 0 {
			rc = model.RevenueContext{PayerType: model.RevenuePayerResellerSelf, ResellerId: task.UserId, CostFactor: bc.ResellerCostFactor, Markup: 1}
		}
	}
	rc.ProfitCredit = bc != nil && bc.ProfitCredit
	return &rc
}

// midjourneyRevenueContext decodes the context frozen on a legacy MJ task.
func midjourneyRevenueContext(task *model.Midjourney) *model.RevenueContext {
	if task == nil {
		return nil
	}
	rc := model.RevenueContext{PayerType: model.RevenuePayerDirect}
	if task.Revenue != "" {
		_ = json.Unmarshal([]byte(task.Revenue), &rc)
	}
	if rc.PayerType == model.RevenuePayerNone {
		return nil
	}
	return &rc
}

// SetMidjourneyRevenueContext sets the revenue context of a billed MJ task on c.
func SetMidjourneyRevenueContext(c *gin.Context, task *model.Midjourney) {
	if rc := midjourneyRevenueContext(task); rc != nil {
		model.SetRevenueContext(c, *rc)
	}
}
