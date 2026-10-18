package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Direct-credit billing mode (B2B2C, per customer).
//
// A direct_credit customer is billed at the base price (cost factor 1,
// markup 1) on every call, with no reseller leg and no revenue row. The
// reseller/admin split happens once, when the reseller tops the customer up:
//
//	customer        += X
//	reseller cost    = round_half_up(X / markup)  (min 1)  -> reseller wallet -= cost
//	reseller profit  = X - cost                    (memo; the reseller collects it off-platform)
//	admin base       = round_half_up(cost / costFactor)
//	admin profit     = cost - base                 -> root via the micro-quota accumulator
//
// Every top-up is a DirectCreditGrant lot; removing unused credit reverses
// lots newest-first at each lot's own ratios.

const (
	BillingModeMarkup       = "" // default: per-call markup with reseller cost leg
	BillingModeDirectCredit = "direct_credit"

	RevenuePayerDirectCredit = "direct_credit"
	// RevenuePayerNone marks a billing context that must write no revenue row
	// (direct_credit customer calls).
	RevenuePayerNone        = "none"
	RevenueKindDCTopup      = "dc_topup"
	RevenueKindDCReversal   = "dc_reversal"
	RevenueSourceDirectCred = "direct_credit"

	ResellerLedgerDirectCreditCost           = "direct_credit_cost"
	ResellerLedgerDirectCreditResellerProfit = "direct_credit_reseller_profit"
	ResellerLedgerDirectCreditAdminProfit    = "direct_credit_admin_profit"
)

var (
	ErrInvalidBillingMode          = errors.New("invalid billing mode")
	ErrBillingModeSwitchNonZero    = errors.New("billing mode can only be changed when the customer balance is 0")
	ErrNotDirectCreditCustomer     = errors.New("customer is not in direct_credit mode")
	ErrDirectCreditExceedsGrants   = errors.New("amount exceeds the customer's unused direct credit")
	ErrDirectCreditAmount          = errors.New("amount must be greater than 0")
	ErrDirectCreditNoResellerSetup = errors.New("reseller profile with a valid default cost factor is required")
	ErrBillingModeChanged          = errors.New("customer billing mode changed; retry")
	ErrBillingModeSwitchInFlight   = errors.New("billing mode cannot be changed while the customer has unfinished tasks")
	ErrBillingModeSwitchBatch      = errors.New("billing mode cannot be changed while batch quota updates are enabled")
)

// NormalizeBillingMode maps API input to the stored value.
func NormalizeBillingMode(mode string) (string, error) {
	switch strings.TrimSpace(mode) {
	case "", "markup":
		return BillingModeMarkup, nil
	case BillingModeDirectCredit:
		return BillingModeDirectCredit, nil
	}
	return "", ErrInvalidBillingMode
}

// DirectCreditGrant is one direct-credit top-up lot.
type DirectCreditGrant struct {
	Id                  int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	CustomerId          int     `json:"customer_id" gorm:"index;not null"`
	ResellerId          int     `json:"reseller_id" gorm:"index;not null"`
	Credit              int64   `json:"credit" gorm:"type:bigint;not null"`
	Cost                int64   `json:"cost" gorm:"type:bigint;not null"`
	AdminProfitMicro    int64   `json:"admin_profit_micro" gorm:"type:bigint;not null"`
	Markup              float64 `json:"markup"`
	CostFactor          float64 `json:"cost_factor"`
	Remaining           int64   `json:"remaining" gorm:"type:bigint;not null"`
	CostRemaining       int64   `json:"cost_remaining" gorm:"type:bigint;not null"`
	AdminMicroRemaining int64   `json:"admin_micro_remaining" gorm:"type:bigint;not null"`
	// UsedMark is the customer's used_quota when the lot was last synced;
	// usage since then consumes open lots oldest-first (see syncDirectCreditLotsTx).
	UsedMark  int64 `json:"used_mark" gorm:"type:bigint;not null;default:0"`
	CreatedAt int64 `json:"created_at" gorm:"bigint"`
}

func (DirectCreditGrant) TableName() string { return "direct_credit_grants" }

// DirectCreditResult reports the amounts that moved.
type DirectCreditResult struct {
	GrantId        int64  `json:"grant_id,omitempty"`
	RevenueId      int64  `json:"revenue_id"`
	Credit         int64  `json:"credit"`
	ResellerCost   int64  `json:"reseller_cost"`
	ResellerProfit int64  `json:"reseller_profit"`
	AdminMicro     int64  `json:"admin_profit_micro"`
	AdminCredited  int64  `json:"admin_credited"`
	CreditState    string `json:"credit_state"`
}

func roundHalfUpDiv(num int64, den float64) int64 {
	return decimal.NewFromInt(num).Div(decimal.NewFromFloat(den)).Round(0).IntPart()
}

// DirectCreditSplit computes the top-up split. Exposed for tests/UI preview.
func DirectCreditSplit(x int64, markup, costFactor float64) (cost, profit, base, adminMicro int64, err error) {
	if x <= 0 {
		return 0, 0, 0, 0, ErrDirectCreditAmount
	}
	if !ValidMarkup(markup) {
		return 0, 0, 0, 0, ErrInvalidMarkup
	}
	if !ValidCostFactor(costFactor) {
		return 0, 0, 0, 0, ErrInvalidCostFactor
	}
	cost = roundHalfUpDiv(x, markup)
	if cost < 1 {
		cost = 1 // fail closed: a top-up never costs the reseller nothing
	}
	base = roundHalfUpDiv(cost, costFactor)
	return cost, x - cost, base, (cost - base) * revenueMicro, nil
}

// directCreditFactorsTx reads the reseller profile and the customer markup.
// No fallbacks for the cost factor: a missing profile or a factor <= 0 fails.
func directCreditFactorsTx(tx *gorm.DB, resellerId int, customer *User) (markup, costFactor float64, err error) {
	var reseller User
	if err = tx.Select("id", "role", "status").Where("id = ?", resellerId).First(&reseller).Error; err != nil {
		return 0, 0, ErrResellerNotFound
	}
	if reseller.Role != common.RoleResellerUser || reseller.Status != common.UserStatusEnabled {
		return 0, 0, ErrResellerDisabled
	}
	var profile ResellerProfile
	if err = tx.Where("user_id = ?", resellerId).Limit(1).Find(&profile).Error; err != nil {
		return 0, 0, err
	}
	if profile.UserId != resellerId {
		return 0, 0, ErrDirectCreditNoResellerSetup
	}
	if !ValidCostFactor(profile.DefaultCostFactor) {
		return 0, 0, ErrInvalidCostFactor
	}
	markup = profile.DefaultMarkup
	if customer.Markup != nil && ValidMarkup(*customer.Markup) {
		markup = *customer.Markup
	}
	if !ValidMarkup(markup) {
		return 0, 0, ErrInvalidMarkup
	}
	return markup, profile.DefaultCostFactor, nil
}

func loadDirectCreditCustomerTx(tx *gorm.DB, resellerId, customerId int) (*User, error) {
	var u User
	if err := lockForUpdate(tx).Select("id", "reseller_id", "billing_mode", "markup", "quota", "used_quota", "username").
		Where("id = ? AND reseller_id = ?", customerId, resellerId).First(&u).Error; err != nil {
		return nil, err
	}
	if u.BillingMode != BillingModeDirectCredit {
		return nil, ErrNotDirectCreditCustomer
	}
	return &u, nil
}

// lotSlice returns the cost and admin-micro share of part credit taken from g
// at the lot's original ratios; the last slice takes the exact remainder.
func lotSlice(g *DirectCreditGrant, part int64) (c, a int64) {
	if part == g.Remaining {
		return g.CostRemaining, g.AdminMicroRemaining
	}
	c = min(decimal.NewFromInt(part).Mul(decimal.NewFromInt(g.Cost)).Div(decimal.NewFromInt(g.Credit)).Round(0).IntPart(), g.CostRemaining)
	a = decimal.NewFromInt(part).Mul(decimal.NewFromInt(g.AdminProfitMicro)).Div(decimal.NewFromInt(g.Credit)).Round(0).IntPart()
	if (g.AdminMicroRemaining >= 0 && a > g.AdminMicroRemaining) || (g.AdminMicroRemaining < 0 && a < g.AdminMicroRemaining) {
		a = g.AdminMicroRemaining
	}
	return c, a
}

// syncDirectCreditLotsTx marks credit consumed by usage: the growth of the
// customer's used_quota since the last sync consumes open lots oldest-first
// (no money moves; consumed credit is final). All usage is attributed to lots
// first, which can only shrink what is reversible (safe side). Caller holds
// the customer row lock.
func syncDirectCreditLotsTx(tx *gorm.DB, cust *User) error {
	var lots []DirectCreditGrant
	if err := lockForUpdate(tx).Where("customer_id = ? AND remaining > 0", cust.Id).
		Order("id ASC").Find(&lots).Error; err != nil {
		return err
	}
	if len(lots) == 0 {
		return nil
	}
	// Every sync stamps all open lots with the same mark, so the oldest
	// open lot's mark is the last sync point.
	used := int64(cust.UsedQuota)
	spent := max(used-lots[0].UsedMark, 0)
	for i := range lots {
		g := &lots[i]
		upd := map[string]any{"used_mark": used}
		if spent > 0 {
			part := min(spent, g.Remaining)
			c, a := lotSlice(g, part)
			upd["remaining"] = g.Remaining - part
			upd["cost_remaining"] = g.CostRemaining - c
			upd["admin_micro_remaining"] = g.AdminMicroRemaining - a
			spent -= part
		}
		if err := tx.Model(&DirectCreditGrant{}).Where("id = ?", g.Id).Updates(upd).Error; err != nil {
			return err
		}
	}
	return nil
}

// DirectCreditCustomer tops up a direct_credit customer by x in ONE transaction.
func DirectCreditCustomer(resellerId, customerId int, x int64, operatorId int, remark string, moneyUsd *float64) (*DirectCreditResult, error) {
	if x <= 0 {
		return nil, ErrDirectCreditAmount
	}
	res := &DirectCreditResult{Credit: x}
	var credits []profitCredit
	err := DB.Transaction(func(tx *gorm.DB) error {
		cust, err := loadDirectCreditCustomerTx(tx, resellerId, customerId)
		if err != nil {
			return err
		}
		if err := syncDirectCreditLotsTx(tx, cust); err != nil {
			return err
		}
		markup, costFactor, err := directCreditFactorsTx(tx, resellerId, cust)
		if err != nil {
			return err
		}
		cost, profit, base, adminMicro, err := DirectCreditSplit(x, markup, costFactor)
		if err != nil {
			return err
		}
		// Funding side first: insufficient reseller balance rejects with no writes.
		if err := applyQuotaDeltaTx(tx, resellerId, int(-cost), ErrAmountExceedsResellerBalance); err != nil {
			return err
		}
		if err := applyQuotaDeltaTx(tx, customerId, int(x), ErrInsufficientQuota); err != nil {
			return err
		}
		now := common.GetTimestamp()
		g := DirectCreditGrant{CustomerId: customerId, ResellerId: resellerId, Credit: x, Cost: cost, AdminProfitMicro: adminMicro,
			Markup: markup, CostFactor: costFactor, Remaining: x, CostRemaining: cost, AdminMicroRemaining: adminMicro, UsedMark: int64(cust.UsedQuota), CreatedAt: now}
		if err := tx.Create(&g).Error; err != nil {
			return err
		}
		ref := fmt.Sprintf("dc:%d", g.Id)
		rec := RevenueRecord{DedupeKey: ref, CreatedAt: now, Kind: RevenueKindDCTopup, PayerType: RevenuePayerDirectCredit,
			UserId: customerId, ResellerId: resellerId, CostFactor: costFactor, Markup: markup,
			BaseQuotaMicro: base * revenueMicro, CustomerPrice: x, ResellerCost: cost, ResellerProfit: profit,
			AdminRevenue: cost, AdminProfitMicro: adminMicro, Source: RevenueSourceDirectCred}
		root, whole, err := directCreditBookTx(tx, &rec, ref, operatorId, remark, moneyUsd, ResellerLedgerCustomerCredit, now)
		if err != nil {
			return err
		}
		if whole != 0 {
			credits = append(credits, profitCredit{root, whole})
		}
		res.GrantId, res.RevenueId, res.ResellerCost, res.ResellerProfit = g.Id, rec.Id, cost, profit
		res.AdminMicro, res.AdminCredited, res.CreditState = adminMicro, whole, rec.CreditState
		return nil
	})
	if err != nil {
		return nil, err
	}
	directCreditSyncCaches(resellerId, customerId, credits)
	return res, nil
}

// directCreditBookTx writes the revenue row, the ledger rows and the admin
// accrual for a top-up (positive amounts) or reversal (negative amounts).
func directCreditBookTx(tx *gorm.DB, rec *RevenueRecord, ref string, operatorId int, remark string, moneyUsd *float64, customerKind string, now int64) (int, int64, error) {
	if err := tx.Create(rec).Error; err != nil {
		return 0, 0, err
	}
	rr := fmt.Sprintf("rev:%d %s", rec.Id, ref)
	if len(rr) > 64 {
		rr = rr[:64]
	}
	entries := []ResellerLedger{
		// Customer-visible: only the credit/debit itself.
		{ResellerId: rec.ResellerId, CustomerId: rec.UserId, Kind: customerKind, QuotaDelta: rec.CustomerPrice,
			MoneyUsd: moneyUsd, OperatorId: operatorId, RefId: ref, Remark: truncateRemark(remark), CreatedAt: now},
		// Reseller-side rows carry no customer id (never shown to the customer).
		{ResellerId: rec.ResellerId, Kind: ResellerLedgerDirectCreditCost, QuotaDelta: -rec.ResellerCost,
			OperatorId: operatorId, RefId: rr, Remark: truncateRemark(fmt.Sprintf("customer #%d credit=%d markup=%g", rec.UserId, rec.CustomerPrice, rec.Markup)), CreatedAt: now},
		// Memo: reseller profit is collected off-platform; not a wallet movement.
		{ResellerId: rec.ResellerId, Kind: ResellerLedgerDirectCreditResellerProfit, QuotaDelta: rec.ResellerProfit,
			OperatorId: operatorId, RefId: rr, Remark: truncateRemark(fmt.Sprintf("memo (not a wallet movement) customer #%d", rec.UserId)), CreatedAt: now},
	}
	for i := range entries {
		if err := tx.Create(&entries[i]).Error; err != nil {
			return 0, 0, err
		}
	}
	state := RevenueCreditStateCredited
	var root int
	var whole int64
	if rec.AdminProfitMicro != 0 {
		var st string
		var err error
		root, whole, st, err = accrueAdminProfitTx(tx, rec.AdminProfitMicro, ResellerLedgerDirectCreditAdminProfit, rr,
			fmt.Sprintf("%s customer #%d admin_profit_micro=%d", rec.Kind, rec.UserId, rec.AdminProfitMicro), rec.AdminProfitMicro < 0)
		if err != nil {
			return 0, 0, err
		}
		if st != "" {
			state = st
		}
	}
	rec.AdminCredited, rec.CreditState = whole, state
	if err := tx.Model(&RevenueRecord{}).Where("id = ?", rec.Id).Updates(map[string]any{
		"admin_credited": whole, "credit_state": state}).Error; err != nil {
		return 0, 0, err
	}
	return root, whole, nil
}

// ReverseDirectCredit removes y of unused direct credit from a customer,
// newest lot first, each at its original ratios. Rejects (no writes) when the
// customer balance or the unused lots cannot cover y, or the root wallet
// cannot cover the admin-profit reversal. No balance ever goes negative.
func ReverseDirectCredit(resellerId, customerId int, y int64, operatorId int, remark string, moneyUsd *float64) (*DirectCreditResult, error) {
	if y <= 0 {
		return nil, ErrDirectCreditAmount
	}
	res := &DirectCreditResult{Credit: -y}
	var credits []profitCredit
	err := DB.Transaction(func(tx *gorm.DB) error {
		cust, err := loadDirectCreditCustomerTx(tx, resellerId, customerId)
		if err != nil {
			return err
		}
		if int64(cust.Quota) < y {
			return ErrInsufficientQuota
		}
		if err := syncDirectCreditLotsTx(tx, cust); err != nil {
			return err
		}
		var lots []DirectCreditGrant
		if err := lockForUpdate(tx).Where("customer_id = ? AND reseller_id = ? AND remaining > 0", customerId, resellerId).
			Order("id DESC").Find(&lots).Error; err != nil {
			return err
		}
		left := y
		var cost, adminMicro int64
		var markup, costFactor float64
		for i := range lots {
			if left == 0 {
				break
			}
			g := &lots[i]
			part := min(left, g.Remaining)
			c, a := lotSlice(g, part)
			if err := tx.Model(&DirectCreditGrant{}).Where("id = ? AND remaining >= ?", g.Id, part).Updates(map[string]any{
				"remaining":             gorm.Expr("remaining - ?", part),
				"cost_remaining":        gorm.Expr("cost_remaining - ?", c),
				"admin_micro_remaining": gorm.Expr("admin_micro_remaining - ?", a),
			}).Error; err != nil {
				return err
			}
			left -= part
			cost += c
			adminMicro += a
			markup, costFactor = g.Markup, g.CostFactor
		}
		if left > 0 {
			return ErrDirectCreditExceedsGrants
		}
		baseMicro := cost*revenueMicro - adminMicro
		if err := applyQuotaDeltaTx(tx, customerId, int(-y), ErrInsufficientQuota); err != nil {
			return err
		}
		if cost > 0 {
			if err := applyQuotaDeltaTx(tx, resellerId, int(cost), ErrInsufficientQuota); err != nil {
				return err
			}
		}
		now := common.GetTimestamp()
		ref := fmt.Sprintf("dcr:%d:%d", customerId, now)
		rec := RevenueRecord{DedupeKey: fmt.Sprintf("%s:%d", ref, common.GetRandomInt(1_000_000_000)), CreatedAt: now, Kind: RevenueKindDCReversal,
			PayerType: RevenuePayerDirectCredit, UserId: customerId, ResellerId: resellerId, CostFactor: costFactor, Markup: markup,
			BaseQuotaMicro: -baseMicro, CustomerPrice: -y, ResellerCost: -cost, ResellerProfit: -(y - cost),
			AdminRevenue: -cost, AdminProfitMicro: -adminMicro, Source: RevenueSourceDirectCred}
		root, whole, err := directCreditBookTx(tx, &rec, ref, operatorId, remark, moneyUsd, ResellerLedgerCustomerDebit, now)
		if err != nil {
			return err
		}
		if whole != 0 {
			credits = append(credits, profitCredit{root, whole})
		}
		res.RevenueId, res.ResellerCost, res.ResellerProfit = rec.Id, -cost, -(y - cost)
		res.AdminMicro, res.AdminCredited, res.CreditState = -adminMicro, whole, rec.CreditState
		return nil
	})
	if err != nil {
		return nil, err
	}
	directCreditSyncCaches(resellerId, customerId, credits)
	return res, nil
}

func directCreditSyncCaches(resellerId, customerId int, credits []profitCredit) {
	for _, uid := range []int{resellerId, customerId} {
		if cacheErr := invalidateUserCache(uid); cacheErr != nil {
			common.SysError(common.LogText("failed to invalidate user cache after direct credit (user=%d): %s", uid, cacheErr.Error()))
		}
	}
	for _, c := range credits {
		if cacheErr := invalidateUserCache(c.userId); cacheErr != nil {
			common.SysError(common.LogText("failed to invalidate user cache after direct credit (user=%d): %s", c.userId, cacheErr.Error()))
		}
	}
}

// SetCustomerBillingMode switches a customer's mode; only allowed while the
// customer balance is exactly 0 (checked atomically in the UPDATE), no async
// task is unfinished (its refund would land under the other mode) and batch
// quota updates are off (the DB balance would lag). On switch every open
// direct-credit lot is closed: at balance 0 nothing in it is unused.
func SetCustomerBillingMode(customerId int, mode string) error {
	mode, err := NormalizeBillingMode(mode)
	if err != nil {
		return err
	}
	var cur User
	if err := DB.Select("id", "billing_mode").Where("id = ? AND reseller_id > 0", customerId).First(&cur).Error; err != nil {
		return err
	}
	if cur.BillingMode == mode {
		return nil
	}
	if common.BatchUpdateEnabled {
		return ErrBillingModeSwitchBatch
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var locked User
		if err := lockForUpdate(tx).Select("id", "quota", "billing_mode").Where("id = ?", customerId).First(&locked).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&Task{}).Where("user_id = ? AND status NOT IN ?", customerId,
			[]string{TaskStatusFailure, TaskStatusSuccess}).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			if err := tx.Model(&Midjourney{}).Where("user_id = ? AND status NOT IN ?", customerId,
				[]string{"FAILURE", "SUCCESS"}).Count(&n).Error; err != nil {
				return err
			}
		}
		if n > 0 {
			return ErrBillingModeSwitchInFlight
		}
		res := tx.Model(&User{}).Where("id = ? AND reseller_id > 0 AND quota = 0 AND billing_mode = ?", customerId, cur.BillingMode).
			Update("billing_mode", mode)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrBillingModeSwitchNonZero
		}
		return tx.Model(&DirectCreditGrant{}).Where("customer_id = ? AND remaining > 0", customerId).
			Updates(map[string]any{"remaining": 0, "cost_remaining": 0, "admin_micro_remaining": 0}).Error
	})
	if err != nil {
		return err
	}
	if err := PublishUserAuthCache(customerId); err != nil {
		_ = invalidateUserCache(customerId)
	}
	InvalidateCustomerPricingCache(customerId)
	return nil
}
