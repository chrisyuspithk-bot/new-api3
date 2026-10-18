package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Profit crediting (B2B2C).
//
// Each newly inserted revenue row credits, in the SAME transaction:
//   - reseller_profit to the reseller wallet (customer rows only), with a
//     reseller_profit_credit ledger entry referencing the row;
//   - admin_profit (micro-quota) to the root account through an accumulator:
//     whole quota units are credited as admin_profit_credit ledger entries
//     and the sub-unit remainder is carried forward, so nothing is lost.
//
// Refund rows carry negative amounts and reverse the credits the same way.
// The credit is bound to the revenue row insert, whose dedupe key is unique,
// so retries, the pending queue and the log reconciler can never credit twice.

const (
	ResellerLedgerResellerProfitCredit = "reseller_profit_credit"
	ResellerLedgerAdminProfitCredit    = "admin_profit_credit"

	RevenueCreditStateCredited = "credited"
	// RevenueCreditStateNoRoot: no root account existed; the admin share is
	// still accrued under user 0 (reported, never silently dropped).
	RevenueCreditStateNoRoot = "no_root"
	// RevenueCreditStateNotCredited: context not eligible (see
	// RevenueContext.ProfitCredit): booked for reporting, wallets untouched.
	RevenueCreditStateNotCredited = "not_credited"
	// RevenueCreditStateLegMismatch: the reseller cost leg did not move the
	// expected amount (failed/skipped); booked and flagged, never credited.
	RevenueCreditStateLegMismatch = "leg_mismatch"
)

// ProfitCreditAccumulator tracks admin profit in micro-quota per beneficiary.
// Invariant: CreditedQuota == floor(AccruedMicro / 1e6); the remainder
// AccruedMicro - CreditedQuota*1e6 is in [0, 1e6).
type ProfitCreditAccumulator struct {
	UserId        int   `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	AccruedMicro  int64 `json:"accrued_micro" gorm:"type:bigint;not null;default:0"`
	CreditedQuota int64 `json:"credited_quota" gorm:"type:bigint;not null;default:0"`
	UpdatedAt     int64 `json:"updated_at" gorm:"bigint;default:0"`
}

func (ProfitCreditAccumulator) TableName() string { return "profit_credit_accumulators" }

// RemainderMicro is the accrued, not yet credited sub-unit amount.
func (a ProfitCreditAccumulator) RemainderMicro() int64 {
	return a.AccruedMicro - a.CreditedQuota*revenueMicro
}

type profitCredit struct {
	userId int
	quota  int64
}

func floorDivMicro(v int64) int64 {
	q := v / revenueMicro
	if v%revenueMicro != 0 && v < 0 {
		q--
	}
	return q
}

func rootUserIdTx(tx *gorm.DB) int {
	var id int
	tx.Model(&User{}).Where("role = ?", common.RoleRootUser).Order("id").Limit(1).Pluck("id", &id)
	return id
}

func revenueLedgerRef(id int64) string { return fmt.Sprintf("rev:%d", id) }

func creditQuotaTx(tx *gorm.DB, userId int, delta int64) error {
	return tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", delta)).Error
}

func applyProfitCreditsTx(tx *gorm.DB, rec *RevenueRecord) ([]profitCredit, error) {
	if !rec.creditEligible || rec.creditBlocked != "" {
		st := RevenueCreditStateNotCredited
		if rec.creditEligible && rec.creditBlocked != "" {
			st = rec.creditBlocked
		}
		rec.CreditState = st
		return nil, tx.Model(&RevenueRecord{}).Where("id = ?", rec.Id).Update("credit_state", st).Error
	}
	now := common.GetTimestamp()
	var credits []profitCredit
	state := RevenueCreditStateCredited

	// 1) Reseller profit (customer rows only; self/direct rows have none).
	if rec.PayerType == RevenuePayerCustomer && rec.ResellerId > 0 && rec.ResellerProfit != 0 {
		if err := creditQuotaTx(tx, rec.ResellerId, rec.ResellerProfit); err != nil {
			return nil, err
		}
		if err := tx.Create(&ResellerLedger{
			// CustomerId stays 0: customer-scoped ledgers/exports must never
			// show reseller margin. The customer is reachable via RefId rev:<id>.
			ResellerId: rec.ResellerId,
			Kind:       ResellerLedgerResellerProfitCredit,
			QuotaDelta: rec.ResellerProfit,
			RefId:      revenueLedgerRef(rec.Id),
			Remark:     truncateRemark(fmt.Sprintf("%s %s %s", rec.Kind, rec.ModelName, rec.RequestId)),
			CreatedAt:  now,
		}).Error; err != nil {
			return nil, err
		}
		rec.ResellerCredited = rec.ResellerProfit
		credits = append(credits, profitCredit{rec.ResellerId, rec.ResellerProfit})
	}

	// 2) Admin profit via the micro-quota accumulator.
	if rec.AdminProfitMicro != 0 {
		root := rootUserIdTx(tx)
		if root == 0 {
			state = RevenueCreditStateNoRoot
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProfitCreditAccumulator{UserId: root, UpdatedAt: now}).Error; err != nil && !isDuplicateKeyErr(err) {
			return nil, err
		}
		// The UPDATE takes the row write lock until commit, so the read below
		// and the whole-unit delta are race-free across concurrent writers.
		if err := tx.Model(&ProfitCreditAccumulator{}).Where("user_id = ?", root).
			Updates(map[string]any{"accrued_micro": gorm.Expr("accrued_micro + ?", rec.AdminProfitMicro), "updated_at": now}).Error; err != nil {
			return nil, err
		}
		var acc ProfitCreditAccumulator
		if err := tx.Where("user_id = ?", root).First(&acc).Error; err != nil {
			return nil, err
		}
		whole := int64(0)
		if root > 0 {
			whole = floorDivMicro(acc.AccruedMicro) - acc.CreditedQuota
		}
		if whole != 0 {
			if err := creditQuotaTx(tx, root, whole); err != nil {
				return nil, err
			}
			if err := tx.Model(&ProfitCreditAccumulator{}).Where("user_id = ?", root).
				Update("credited_quota", gorm.Expr("credited_quota + ?", whole)).Error; err != nil {
				return nil, err
			}
			if err := tx.Create(&ResellerLedger{
				ResellerId: root,
				Kind:       ResellerLedgerAdminProfitCredit,
				QuotaDelta: whole,
				RefId:      revenueLedgerRef(rec.Id),
				Remark:     truncateRemark(fmt.Sprintf("%s %s admin_profit_micro=%d accrued_micro=%d", rec.Kind, rec.ModelName, rec.AdminProfitMicro, acc.AccruedMicro)),
				CreatedAt:  now,
			}).Error; err != nil {
				return nil, err
			}
			rec.AdminCredited = whole
			credits = append(credits, profitCredit{root, whole})
		}
	}

	rec.CreditState = state
	if err := tx.Model(&RevenueRecord{}).Where("id = ?", rec.Id).Updates(map[string]any{
		"reseller_credited": rec.ResellerCredited,
		"admin_credited":    rec.AdminCredited,
		"credit_state":      state,
	}).Error; err != nil {
		return nil, err
	}
	return credits, nil
}

// ProfitAccumulatorTotals returns the admin accumulator (root account).
func ProfitAccumulatorTotals() (ProfitCreditAccumulator, error) {
	var acc ProfitCreditAccumulator
	root := rootUserIdTx(DB)
	err := DB.Where("user_id = ?", root).Limit(1).Find(&acc).Error
	acc.UserId = root
	return acc, err
}

func truncateRemark(s string) string {
	if len(s) > 250 {
		return s[:250]
	}
	return s
}

// syncProfitCreditCaches mirrors committed wallet credits into the user quota
// cache (no-op without Redis). Debits invalidate by decrementing.
func syncProfitCreditCaches(credits []profitCredit) {
	if !common.RedisEnabled {
		return
	}
	for _, c := range credits {
		if c.quota > 0 {
			syncCreditUserQuotaCache(c.userId, int(c.quota), "profit credit")
		} else if c.quota < 0 {
			if err := cacheDecrUserQuota(c.userId, -c.quota); err != nil {
				common.SysLog(common.LogText("failed to sync profit reversal to quota cache: %s", err.Error()))
			}
		}
	}
}
