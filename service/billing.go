package service

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, common.LogText("charging the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, common.LogText("refunding the difference after pre-consume: %s (actual %s, pre-consumed %s)",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, common.LogText("pre-consumed quota matches the actual quota, no adjustment: %s (per-request billing)",
				logger.FormatQuota(actualQuota),
			))
		}

		err := relayInfo.Billing.Settle(actualQuota)
		SetRevenueContextFromRelayInfo(ctx, relayInfo)
		if relayInfo.ResellerId > 0 {
			ctx.Set(model.ContextKeyLogResellerId, relayInfo.ResellerId)
			ctx.Set(model.ContextKeyLogResellerCostQuota, relayInfo.ResellerCostQuota)
		}
		if err != nil {
			return err
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	// (e.g. realtime/WSS: the reseller leg was already moved per response by
	// PreWssConsumeQuota and accumulated on relayInfo.ResellerCostQuota).
	// Reseller leg: true up to the single-shot cost of the actual total, so the
	// reseller is charged exactly once (chunks + this delta).
	// Scoped to realtime: other session-less paths never charged the reseller
	// leg here and keep their existing behaviour.
	if relayInfo.ResellerId > 0 && relayInfo.RelayFormat == types.RelayFormatOpenAIRealtime {
		target := ResellerCostFromCharge(actualQuota, relayInfo.ResellerMarkup)
		if actualQuota <= 0 {
			target = 0
		}
		rdelta := target - relayInfo.ResellerCostQuota
		var rerr error
		if rdelta > 0 {
			rerr = model.DecreaseUserQuota(relayInfo.ResellerId, rdelta, false)
		} else if rdelta < 0 {
			rerr = model.IncreaseUserQuota(relayInfo.ResellerId, -rdelta, false)
		}
		if rerr != nil {
			common.SysError(common.LogText("error settling reseller leg (fallback, resellerId=%d): %s", relayInfo.ResellerId, rerr.Error()))
		} else {
			relayInfo.ResellerCostQuota = target
			if target > 0 {
				model.UpdateUserUsedQuotaAndRequestCount(relayInfo.ResellerId, target)
			}
		}
	}
	SetRevenueContextFromRelayInfo(ctx, relayInfo)
	if relayInfo.ResellerId > 0 {
		ctx.Set(model.ContextKeyLogResellerId, relayInfo.ResellerId)
		ctx.Set(model.ContextKeyLogResellerCostQuota, relayInfo.ResellerCostQuota)
	}
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}
	return nil
}
