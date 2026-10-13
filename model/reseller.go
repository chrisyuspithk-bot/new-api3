package model

import (
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResellerProfile holds per-reseller commercial settings (1:1 with a role=5 user).
type ResellerProfile struct {
	UserId          int     `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	DefaultMarkup   float64 `json:"default_markup" gorm:"type:decimal(18,6);not null;default:1"`
	MaxCustomers    int     `json:"max_customers" gorm:"not null;default:0"`
	AllowRedemption bool    `json:"allow_redemption"`
	DisplayName     string  `json:"display_name" gorm:"type:varchar(64);default:''"`
	CreatedAt       int64   `json:"created_at" gorm:"bigint;not null;default:0"`
	UpdatedAt       int64   `json:"updated_at" gorm:"bigint;not null;default:0"`
}

// ResellerGroup whitelists a group a reseller may hand out to its customers.
type ResellerGroup struct {
	Id         int    `json:"id"`
	ResellerId int    `json:"reseller_id" gorm:"not null;uniqueIndex:uniq_reseller_group,priority:1"`
	GroupName  string `json:"group_name" gorm:"type:varchar(64);not null;uniqueIndex:uniq_reseller_group,priority:2"`
}

// ResellerModelPrice is an admin-set override of the reseller's cost for one model.
// Phase 1 applies it as a multiplier on the base quota: CostMultiplier when set,
// otherwise ModelRatio/base ratio or ModelPrice/base price.
type ResellerModelPrice struct {
	Id              int      `json:"id"`
	ResellerId      int      `json:"reseller_id" gorm:"not null;uniqueIndex:uniq_rmp,priority:1"`
	ModelName       string   `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uniq_rmp,priority:2"`
	ModelRatio      *float64 `json:"model_ratio,omitempty" gorm:"type:decimal(18,6)"`
	ModelPrice      *float64 `json:"model_price,omitempty" gorm:"type:decimal(18,6)"`
	CompletionRatio *float64 `json:"completion_ratio,omitempty" gorm:"type:decimal(18,6)"`
	CostMultiplier  *float64 `json:"cost_multiplier,omitempty" gorm:"type:decimal(18,6)"`
	UpdatedAt       int64    `json:"updated_at" gorm:"bigint;not null;default:0"`
}

// ResellerModelMarkup is a reseller's per-model default markup.
type ResellerModelMarkup struct {
	Id         int     `json:"id"`
	ResellerId int     `json:"reseller_id" gorm:"not null;uniqueIndex:uniq_rmm,priority:1"`
	ModelName  string  `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uniq_rmm,priority:2"`
	Markup     float64 `json:"markup" gorm:"type:decimal(18,6);not null"`
	UpdatedAt  int64   `json:"updated_at" gorm:"bigint;not null;default:0"`
}

// CustomerModelMarkup is a rare customer x model markup override.
type CustomerModelMarkup struct {
	Id         int     `json:"id"`
	ResellerId int     `json:"reseller_id" gorm:"not null;index"`
	CustomerId int     `json:"customer_id" gorm:"not null;uniqueIndex:uniq_cmm,priority:1"`
	ModelName  string  `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uniq_cmm,priority:2"`
	Markup     float64 `json:"markup" gorm:"type:decimal(18,6);not null"`
	UpdatedAt  int64   `json:"updated_at" gorm:"bigint;not null;default:0"`
}

const (
	ResellerLedgerAdminCredit    = "admin_credit"
	ResellerLedgerAdminDebit     = "admin_debit"
	ResellerLedgerCustomerCredit = "customer_credit"
	ResellerLedgerCustomerDebit  = "customer_debit"
	// Reseller-issued redemption codes are prepaid: issuing moves the face
	// value from the reseller wallet into the code (negative delta, no
	// customer), voiding an unused code moves it back (positive delta, no
	// customer), and redeeming credits the customer from the code without
	// touching the reseller wallet again.
	ResellerLedgerRedemptionIssue  = "redemption_issue"
	ResellerLedgerRedemptionRefund = "redemption_refund"
	ResellerLedgerRedemptionRedeem = "redemption_redeem"
)

// ResellerLedger is the audit trail of manual balance movements. Consumption is
// recorded in logs only.
type ResellerLedger struct {
	Id               int64    `json:"id"`
	ResellerId       int      `json:"reseller_id" gorm:"not null;index"`
	CustomerId       int      `json:"customer_id" gorm:"not null;default:0;index"`
	Kind             string   `json:"kind" gorm:"type:varchar(32);not null"`
	QuotaDelta       int64    `json:"quota_delta" gorm:"not null"`
	MoneyUsd         *float64 `json:"money_usd,omitempty" gorm:"type:decimal(18,6)"`
	OperatorId       int      `json:"operator_id"`
	RefId            string   `json:"ref_id" gorm:"type:varchar(64);default:''"`
	Remark           string   `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt        int64    `json:"created_at" gorm:"bigint;index"`
	CustomerUsername string   `json:"customer_username,omitempty" gorm:"-"`
	OperatorUsername string   `json:"operator_username,omitempty" gorm:"-"`
	ResellerUsername string   `json:"reseller_username,omitempty" gorm:"-"`
}

var (
	ErrResellerNotFound             = errors.New("reseller not found")
	ErrResellerDisabled             = errors.New("reseller disabled")
	ErrInvalidMarkup                = errors.New("markup must be greater than 0")
	ErrGroupNotAllowed              = errors.New("group is not allowed for this reseller")
	ErrInsufficientQuota            = errors.New("insufficient quota")
	ErrAmountExceedsResellerBalance = errors.New("amount exceeds available cost balance")
	ErrCustomerLimitReached         = errors.New("reseller customer limit reached")
	ErrInvalidCostFactor            = errors.New("reseller cost factor must be greater than 0")
	ErrInvalidResellerPrice         = common.NewMessage("cost_multiplier, model_ratio and model_price must be greater than 0")
)

// ValidMarkup reports whether m is an acceptable markup (0, +Inf).
func ValidMarkup(m float64) bool {
	return m > 0 && m < 1e6
}

// ValidCostFactor reports whether f is a billable reseller cost factor (0, +Inf).
// A zero factor would make every call free, so pricing fails closed on it.
func ValidCostFactor(f float64) bool {
	return f > 0 && !math.IsInf(f, 1)
}

// ResellerPricingConfig is the cached snapshot used on the relay hot path.
type ResellerPricingConfig struct {
	ResellerId    int
	Status        int
	Role          int
	DefaultMarkup float64
	Groups        []string
	ModelPrices   map[string]ResellerModelPrice
	ModelMarkups  map[string]float64
}

// AllowsGroup reports whether customers of this reseller may use group.
func (cfg *ResellerPricingConfig) AllowsGroup(group string) bool {
	return slices.Contains(cfg.Groups, group)
}

// CustomerPricingConfig is the cached customer-level markup overrides.
type CustomerPricingConfig struct {
	Markup       *float64
	ModelMarkups map[string]float64
}

type resellerCacheEntry[T any] struct {
	value   *T
	expires time.Time
}

const resellerCacheTTL = 10 * time.Second

var (
	resellerCfgCache sync.Map // int -> resellerCacheEntry[ResellerPricingConfig]
	customerCfgCache sync.Map // int -> resellerCacheEntry[CustomerPricingConfig]
)

// InvalidateResellerCache drops the cached pricing/status snapshot of a reseller.
// Other nodes converge within resellerCacheTTL.
func InvalidateResellerCache(resellerId int) {
	resellerCfgCache.Delete(resellerId)
}

// InvalidateCustomerPricingCache drops the cached markup overrides of a customer.
func InvalidateCustomerPricingCache(customerId int) {
	customerCfgCache.Delete(customerId)
}

// GetResellerPricingConfig returns the (cached) reseller configuration.
func GetResellerPricingConfig(resellerId int) (*ResellerPricingConfig, error) {
	if v, ok := resellerCfgCache.Load(resellerId); ok {
		entry := v.(resellerCacheEntry[ResellerPricingConfig])
		if time.Now().Before(entry.expires) {
			return entry.value, nil
		}
	}
	var user User
	if err := DB.Select("id", "role", "status").Where("id = ?", resellerId).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrResellerNotFound
		}
		return nil, err
	}
	cfg := &ResellerPricingConfig{
		ResellerId:    resellerId,
		Status:        user.Status,
		Role:          user.Role,
		DefaultMarkup: 1,
		ModelPrices:   map[string]ResellerModelPrice{},
		ModelMarkups:  map[string]float64{},
	}
	var profile ResellerProfile
	if err := DB.Where("user_id = ?", resellerId).Limit(1).Find(&profile).Error; err != nil {
		return nil, err
	}
	if profile.UserId == resellerId && ValidMarkup(profile.DefaultMarkup) {
		cfg.DefaultMarkup = profile.DefaultMarkup
	}
	groups, err := GetResellerGroups(resellerId)
	if err != nil {
		return nil, err
	}
	cfg.Groups = groups
	var prices []ResellerModelPrice
	if err := DB.Where("reseller_id = ?", resellerId).Find(&prices).Error; err != nil {
		return nil, err
	}
	for _, p := range prices {
		cfg.ModelPrices[p.ModelName] = p
	}
	var markups []ResellerModelMarkup
	if err := DB.Where("reseller_id = ?", resellerId).Find(&markups).Error; err != nil {
		return nil, err
	}
	for _, m := range markups {
		cfg.ModelMarkups[m.ModelName] = m.Markup
	}
	resellerCfgCache.Store(resellerId, resellerCacheEntry[ResellerPricingConfig]{value: cfg, expires: time.Now().Add(resellerCacheTTL)})
	return cfg, nil
}

// GetCustomerPricingConfig returns the (cached) customer markup overrides.
func GetCustomerPricingConfig(customerId int) (*CustomerPricingConfig, error) {
	if v, ok := customerCfgCache.Load(customerId); ok {
		entry := v.(resellerCacheEntry[CustomerPricingConfig])
		if time.Now().Before(entry.expires) {
			return entry.value, nil
		}
	}
	var user User
	if err := DB.Select("id", "markup").Where("id = ?", customerId).First(&user).Error; err != nil {
		return nil, err
	}
	cfg := &CustomerPricingConfig{Markup: user.Markup, ModelMarkups: map[string]float64{}}
	var markups []CustomerModelMarkup
	if err := DB.Where("customer_id = ?", customerId).Find(&markups).Error; err != nil {
		return nil, err
	}
	for _, m := range markups {
		cfg.ModelMarkups[m.ModelName] = m.Markup
	}
	customerCfgCache.Store(customerId, resellerCacheEntry[CustomerPricingConfig]{value: cfg, expires: time.Now().Add(resellerCacheTTL)})
	return cfg, nil
}

// GetResellerGroups returns the reseller's group whitelist, sorted.
func GetResellerGroups(resellerId int) ([]string, error) {
	var groups []string
	err := DB.Model(&ResellerGroup{}).Where("reseller_id = ?", resellerId).Order("group_name").Pluck("group_name", &groups).Error
	return groups, err
}

// SetResellerGroups replaces the reseller's group whitelist. Customers whose group
// falls outside the new set are moved to fallbackGroup (which must be in the set).
func SetResellerGroups(resellerId int, groups []string, fallbackGroup string) error {
	normalized := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g != "" && !slices.Contains(normalized, g) {
			normalized = append(normalized, g)
		}
	}
	if len(normalized) > 0 && !slices.Contains(normalized, fallbackGroup) {
		fallbackGroup = normalized[0]
	}
	var movedIds []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("reseller_id = ?", resellerId).Delete(&ResellerGroup{}).Error; err != nil {
			return err
		}
		for _, g := range normalized {
			if err := tx.Create(&ResellerGroup{ResellerId: resellerId, GroupName: g}).Error; err != nil {
				return err
			}
		}
		query := tx.Model(&User{}).Where("reseller_id = ?", resellerId)
		if len(normalized) > 0 {
			query = query.Where(commonGroupCol+" NOT IN ?", normalized)
		}
		if err := query.Pluck("id", &movedIds).Error; err != nil {
			return err
		}
		if len(movedIds) == 0 || fallbackGroup == "" {
			return nil
		}
		return tx.Model(&User{}).Where("id IN ?", movedIds).Update("group", fallbackGroup).Error
	})
	if err != nil {
		return err
	}
	InvalidateResellerCache(resellerId)
	for _, id := range movedIds {
		if err := PublishUserAuthCache(id); err != nil {
			_ = invalidateUserCache(id)
		}
	}
	return nil
}

// GetResellerProfile returns the profile, or a default one if missing.
func GetResellerProfile(resellerId int) (*ResellerProfile, error) {
	profile := &ResellerProfile{UserId: resellerId, DefaultMarkup: 1}
	var rows []ResellerProfile
	if err := DB.Where("user_id = ?", resellerId).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 1 {
		profile = &rows[0]
	}
	return profile, nil
}

// SaveResellerProfile upserts the profile.
func SaveResellerProfile(profile *ResellerProfile) error {
	if !ValidMarkup(profile.DefaultMarkup) {
		return ErrInvalidMarkup
	}
	now := common.GetTimestamp()
	if profile.CreatedAt == 0 {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"default_markup", "max_customers", "allow_redemption", "display_name", "updated_at"}),
	}).Create(profile).Error
	if err != nil {
		return err
	}
	InvalidateResellerCache(profile.UserId)
	return nil
}

// UpsertResellerModelPrice creates or updates an admin cost override.
func UpsertResellerModelPrice(price *ResellerModelPrice) error {
	if price.CostMultiplier != nil && (price.ModelRatio != nil || price.ModelPrice != nil || price.CompletionRatio != nil) {
		return errors.New("cost_multiplier is mutually exclusive with model_ratio/model_price/completion_ratio")
	}
	for _, v := range []*float64{price.CostMultiplier, price.ModelRatio, price.ModelPrice} {
		if v != nil && !ValidCostFactor(*v) {
			return ErrInvalidResellerPrice
		}
	}
	if v := price.CompletionRatio; v != nil && !(*v >= 0 && !math.IsInf(*v, 1)) {
		return errors.New("completion_ratio must be non-negative")
	}
	price.UpdatedAt = common.GetTimestamp()
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "reseller_id"}, {Name: "model_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"model_ratio", "model_price", "completion_ratio", "cost_multiplier", "updated_at"}),
	}).Create(price).Error
	if err != nil {
		return err
	}
	InvalidateResellerCache(price.ResellerId)
	return nil
}

// DeleteResellerModelPrice removes a cost override.
func DeleteResellerModelPrice(resellerId int, modelName string) error {
	err := DB.Where("reseller_id = ? AND model_name = ?", resellerId, modelName).Delete(&ResellerModelPrice{}).Error
	InvalidateResellerCache(resellerId)
	return err
}

// UpsertResellerModelMarkup creates or updates a reseller per-model markup.
func UpsertResellerModelMarkup(resellerId int, modelName string, markup float64) error {
	if !ValidMarkup(markup) {
		return ErrInvalidMarkup
	}
	row := &ResellerModelMarkup{ResellerId: resellerId, ModelName: modelName, Markup: markup, UpdatedAt: common.GetTimestamp()}
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "reseller_id"}, {Name: "model_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"markup", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return err
	}
	InvalidateResellerCache(resellerId)
	return nil
}

// DeleteResellerModelMarkup removes a reseller per-model markup.
func DeleteResellerModelMarkup(resellerId int, modelName string) error {
	err := DB.Where("reseller_id = ? AND model_name = ?", resellerId, modelName).Delete(&ResellerModelMarkup{}).Error
	InvalidateResellerCache(resellerId)
	return err
}

// UpsertCustomerModelMarkup creates or updates a customer x model markup.
func UpsertCustomerModelMarkup(resellerId, customerId int, modelName string, markup float64) error {
	if !ValidMarkup(markup) {
		return ErrInvalidMarkup
	}
	row := &CustomerModelMarkup{ResellerId: resellerId, CustomerId: customerId, ModelName: modelName, Markup: markup, UpdatedAt: common.GetTimestamp()}
	err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "model_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"markup", "updated_at"}),
	}).Create(row).Error
	if err != nil {
		return err
	}
	InvalidateCustomerPricingCache(customerId)
	return nil
}

// DeleteCustomerModelMarkup removes a customer x model markup (scoped to reseller).
func DeleteCustomerModelMarkup(resellerId, customerId int, modelName string) error {
	err := DB.Where("reseller_id = ? AND customer_id = ? AND model_name = ?", resellerId, customerId, modelName).Delete(&CustomerModelMarkup{}).Error
	InvalidateCustomerPricingCache(customerId)
	return err
}

// GetResellerCustomer loads a customer only if it belongs to resellerId (IDOR guard).
func GetResellerCustomer(resellerId, customerId int) (*User, error) {
	var user User
	err := DB.Omit("password").Where("id = ? AND reseller_id = ?", customerId, resellerId).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// CountResellerCustomers returns the number of (non-deleted) customers of a reseller.
func CountResellerCustomers(resellerId int) (int64, error) {
	var count int64
	err := DB.Model(&User{}).Where("reseller_id = ?", resellerId).Count(&count).Error
	return count, err
}

// FillResellerNames sets ResellerUsername / ResellerDisplayName on users that
// belong to a reseller, loading all referenced resellers in one query.
// Soft-deleted resellers are included so historical affiliation stays readable.
func FillResellerNames(users []*User) error {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		if user.ResellerId > 0 && !slices.Contains(ids, user.ResellerId) {
			ids = append(ids, user.ResellerId)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var resellers []User
	if err := DB.Unscoped().Select("id", "username", "display_name").Where("id IN ?", ids).Find(&resellers).Error; err != nil {
		return err
	}
	byId := make(map[int]*User, len(resellers))
	for i := range resellers {
		byId[resellers[i].Id] = &resellers[i]
	}
	for _, user := range users {
		if reseller, ok := byId[user.ResellerId]; ok {
			user.ResellerUsername = reseller.Username
			user.ResellerDisplayName = reseller.DisplayName
		}
	}
	return nil
}

// AdjustQuotaWithLedger credits (delta>0) or debits (delta<0) a user's wallet
// and appends a ledger row in ONE database transaction, so a balance change
// never exists without its audit row (and vice versa). Debits never take a
// balance below zero (conditional UPDATE). The Redis user cache is dropped
// after commit so the next read reloads the committed balance.
//
// Admin→Reseller funding uses this helper. Reseller→Customer credit/debit must
// use TransferResellerCustomerQuota so the reseller cost balance moves too.
func AdjustQuotaWithLedger(userId int, delta int, entry ResellerLedger) error {
	if delta == 0 {
		return errors.New("quota delta cannot be zero")
	}
	entry.QuotaDelta = int64(delta)
	entry.CreatedAt = common.GetTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&User{}).Where("id = ?", userId)
		if delta < 0 {
			q = q.Where("quota >= ?", -delta)
		}
		res := q.Update("quota", gorm.Expr("quota + ?", delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if delta < 0 {
				return ErrInsufficientQuota
			}
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&entry).Error
	})
	if err != nil {
		return err
	}
	if cacheErr := invalidateUserCache(userId); cacheErr != nil {
		common.SysError(common.LogText("failed to invalidate user cache after ledger adjust (user=%d): %s", userId, cacheErr.Error()))
	}
	return nil
}

// applyQuotaDeltaTx applies delta to users.quota inside an open transaction.
// Negative deltas are conditional (quota >= |delta|) so the wallet never goes
// below zero. insufficient maps the zero-row case for debits.
func applyQuotaDeltaTx(tx *gorm.DB, userId int, delta int, insufficient error) error {
	q := tx.Model(&User{}).Where("id = ?", userId)
	if delta < 0 {
		q = q.Where("quota >= ?", -delta)
	}
	res := q.Update("quota", gorm.Expr("quota + ?", delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		if delta < 0 {
			return insufficient
		}
		return gorm.ErrRecordNotFound
	}
	return nil
}

// TransferResellerCustomerQuota moves quota between a reseller's cost balance
// and a customer's sale wallet in ONE transaction, then writes a single ledger
// row (quota_delta = customer side). customerDelta > 0 credits the customer and
// debits the reseller by the same amount; customerDelta < 0 is clawback
// (−customer, +reseller). Rejects when the funded side cannot cover the move.
func TransferResellerCustomerQuota(resellerId, customerId, customerDelta int, entry ResellerLedger) error {
	if customerDelta == 0 {
		return errors.New("quota delta cannot be zero")
	}
	if resellerId <= 0 || customerId <= 0 {
		return errors.New("invalid user id")
	}
	entry.ResellerId = resellerId
	entry.CustomerId = customerId
	entry.QuotaDelta = int64(customerDelta)
	entry.CreatedAt = common.GetTimestamp()
	resellerDelta := -customerDelta
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Debit the funding side first so a failed credit never orphans funds.
		if resellerDelta < 0 {
			if err := applyQuotaDeltaTx(tx, resellerId, resellerDelta, ErrAmountExceedsResellerBalance); err != nil {
				return err
			}
			if err := applyQuotaDeltaTx(tx, customerId, customerDelta, ErrInsufficientQuota); err != nil {
				return err
			}
		} else {
			if err := applyQuotaDeltaTx(tx, customerId, customerDelta, ErrInsufficientQuota); err != nil {
				return err
			}
			if err := applyQuotaDeltaTx(tx, resellerId, resellerDelta, ErrAmountExceedsResellerBalance); err != nil {
				return err
			}
		}
		return tx.Create(&entry).Error
	})
	if err != nil {
		return err
	}
	for _, uid := range []int{resellerId, customerId} {
		if cacheErr := invalidateUserCache(uid); cacheErr != nil {
			common.SysError(common.LogText("failed to invalidate user cache after customer transfer (user=%d): %s", uid, cacheErr.Error()))
		}
	}
	return nil
}

// CreateResellerCustomer inserts a customer with 0 sale quota. The
// max_customers check and the insert run in one transaction holding a row lock
// on the reseller profile, so concurrent creates cannot exceed the limit
// (MySQL/PG; SQLite serialises writers anyway).
func CreateResellerCustomer(resellerId int, user *User) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var profile ResellerProfile
		if err := lockForUpdate(tx).Where("user_id = ?", resellerId).First(&profile).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if profile.MaxCustomers > 0 {
			var count int64
			if err := tx.Model(&User{}).Where("reseller_id = ?", resellerId).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(profile.MaxCustomers) {
				return ErrCustomerLimitReached
			}
		}
		if err := user.InsertWithTx(tx, 0); err != nil {
			return err
		}
		// Customers start with 0 sale quota; the reseller credits them manually.
		user.Quota = 0
		return tx.Model(&User{}).Where("id = ?", user.Id).Update("quota", 0).Error
	})
}

// DisableResellerCustomer is the reseller "delete": the customer is disabled
// (sessions revoked) and all of its tokens are disabled. Rows are kept for
// logs, ledger and reports.
func DisableResellerCustomer(customerId int) error {
	if err := SetUserStatusAndRevoke(customerId, common.UserStatusDisabled); err != nil {
		return err
	}
	if err := InvalidateUserTokensCache(customerId); err != nil {
		common.SysError(common.LogText("failed to invalidate tokens cache for user %d: %s", customerId, err.Error()))
	}
	if err := DB.Model(&Token{}).Where("user_id = ?", customerId).Update("status", common.TokenStatusDisabled).Error; err != nil {
		return err
	}
	InvalidateCustomerPricingCache(customerId)
	return nil
}

// ReconcileIssue is one finding of the reseller reconciliation check.
type ReconcileIssue struct {
	ResellerId int    `json:"reseller_id"`
	Username   string `json:"username"`
	Kind       string `json:"kind"`
	Detail     string `json:"detail"`
	Value      int64  `json:"value"`
}

// ReconcileResellers flags (read-only) resellers with a negative cost balance
// and consume logs whose reseller leg looks inconsistent: a reseller customer
// log with no cost recorded, or a cost above the customer charge.
func ReconcileResellers(from int64) ([]ReconcileIssue, error) {
	issues := []ReconcileIssue{}
	var negative []User
	if err := DB.Select("id", "username", "quota").Where("role = ? AND quota < 0", common.RoleResellerUser).Find(&negative).Error; err != nil {
		return nil, err
	}
	for _, u := range negative {
		issues = append(issues, ReconcileIssue{ResellerId: u.Id, Username: u.Username, Kind: "negative_balance", Detail: "reseller cost balance below zero", Value: int64(u.Quota)})
	}
	var rows []struct {
		ResellerId int
		Missing    int64
		Excess     int64
	}
	q := LOG_DB.Model(&Log{}).
		Select("reseller_id, "+
			"COALESCE(SUM(CASE WHEN quota > 0 AND reseller_cost_quota = 0 THEN 1 ELSE 0 END),0) AS missing, "+
			"COALESCE(SUM(CASE WHEN reseller_cost_quota > quota THEN 1 ELSE 0 END),0) AS excess").
		Where("type = ? AND reseller_id > 0", LogTypeConsume)
	if from > 0 {
		q = q.Where("created_at >= ?", from)
	}
	if err := q.Group("reseller_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Missing > 0 {
			issues = append(issues, ReconcileIssue{ResellerId: r.ResellerId, Kind: "log_missing_cost", Detail: "consume logs with charge but no reseller cost", Value: r.Missing})
		}
		if r.Excess > 0 {
			issues = append(issues, ReconcileIssue{ResellerId: r.ResellerId, Kind: "log_cost_above_charge", Detail: "consume logs where reseller cost exceeds customer charge", Value: r.Excess})
		}
	}
	return issues, nil
}

// GetResellerLedger lists ledger rows for a reseller, newest first.
// kindFilter may be empty (all) or one of the ResellerLedger* constants.
func GetResellerLedger(resellerId int, startIdx, num int, kindFilter string) ([]ResellerLedger, int64, error) {
	var rows []ResellerLedger
	var total int64
	query := DB.Model(&ResellerLedger{}).Where("reseller_id = ?", resellerId)
	if kindFilter != "" {
		query = query.Where("kind = ?", kindFilter)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset(startIdx).Limit(num).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	if err := FillLedgerUsernames(rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// FillLedgerUsernames attaches customer_username, operator_username and
// reseller_username for UI lists.
func FillLedgerUsernames(rows []ResellerLedger) error {
	if len(rows) == 0 {
		return nil
	}
	idSet := map[int]struct{}{}
	for i := range rows {
		if rows[i].CustomerId > 0 {
			idSet[rows[i].CustomerId] = struct{}{}
		}
		if rows[i].OperatorId > 0 {
			idSet[rows[i].OperatorId] = struct{}{}
		}
		if rows[i].ResellerId > 0 {
			idSet[rows[i].ResellerId] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]int, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	var users []User
	if err := DB.Unscoped().Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return err
	}
	names := make(map[int]string, len(users))
	for _, u := range users {
		names[u.Id] = u.Username
	}
	for i := range rows {
		if rows[i].CustomerId > 0 {
			rows[i].CustomerUsername = names[rows[i].CustomerId]
		}
		if rows[i].OperatorId > 0 {
			rows[i].OperatorUsername = names[rows[i].OperatorId]
		}
		if rows[i].ResellerId > 0 {
			rows[i].ResellerUsername = names[rows[i].ResellerId]
		}
	}
	return nil
}

// ListResellerLedgerRange lists ledger rows in [from, toExclusive).
// resellerId, customerId, and kindFilter may be 0 / "" to skip that filter.
// num < 0 returns every matching row (exports).
func ListResellerLedgerRange(from, toExclusive int64, resellerId int, customerId int, kindFilter string, startIdx, num int) ([]ResellerLedger, int64, error) {
	var rows []ResellerLedger
	var total int64
	query := resellerLedgerScope(from, toExclusive, resellerId, customerId)
	if kindFilter != "" {
		query = query.Where("kind = ?", kindFilter)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset(startIdx).Limit(num).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	if err := FillLedgerUsernames(rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// resellerLedgerScope selects ledger rows in [from, toExclusive) for one role
// scope. Zero arguments skip that filter.
func resellerLedgerScope(from, toExclusive int64, resellerId int, customerId int) *gorm.DB {
	query := DB.Model(&ResellerLedger{})
	if from > 0 {
		query = query.Where("created_at >= ?", from)
	}
	if toExclusive > 0 {
		query = query.Where("created_at < ?", toExclusive)
	}
	if resellerId > 0 {
		query = query.Where("reseller_id = ?", resellerId)
	}
	if customerId > 0 {
		query = query.Where("customer_id = ?", customerId)
	}
	return query
}

// LedgerKindTotal sums the ledger rows of one kind.
type LedgerKindTotal struct {
	Kind     string  `json:"kind"`
	Entries  int64   `json:"entries"`
	Quota    int64   `json:"quota"`
	MoneyUsd float64 `json:"money_usd"`
}

// SumResellerLedgerRange totals ledger rows per kind in [from, toExclusive)
// with the same scope filters as ListResellerLedgerRange (no kind filter).
func SumResellerLedgerRange(from, toExclusive int64, resellerId int, customerId int) ([]LedgerKindTotal, error) {
	var totals []LedgerKindTotal
	err := resellerLedgerScope(from, toExclusive, resellerId, customerId).
		Select("kind, COUNT(*) AS entries, COALESCE(SUM(quota_delta),0) AS quota, COALESCE(SUM(money_usd),0) AS money_usd").
		Group("kind").
		Order("kind").
		Scan(&totals).Error
	return totals, err
}

// GrossProfitRow is one bucket of the gross profit report.
type GrossProfitRow struct {
	Key         string `json:"key"`
	Requests    int64  `json:"requests"`
	ChargeQuota int64  `json:"charge_quota"`
	CostQuota   int64  `json:"cost_quota"`
	ProfitQuota int64  `json:"profit_quota"`
}

// GetResellerGrossProfit aggregates consume logs by model or customer.
func GetResellerGrossProfit(resellerId int, from, to int64, groupBy string) ([]GrossProfitRow, error) {
	keyCol := "model_name"
	if groupBy == "customer" {
		keyCol = "username"
	}
	query := LOG_DB.Model(&Log{}).
		// Async task refunds / delta settlements carry the reseller leg too,
		// so refunds are netted out of both charge and cost.
		Select(keyCol+" AS key_name, "+
			"COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END),0) AS requests, "+
			"COALESCE(SUM(CASE WHEN type = ? THEN -quota ELSE quota END),0) AS charge_quota, "+
			"COALESCE(SUM(CASE WHEN type = ? THEN -reseller_cost_quota ELSE reseller_cost_quota END),0) AS cost_quota",
			LogTypeConsume, LogTypeRefund, LogTypeRefund).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund})
	if resellerId > 0 {
		query = query.Where("reseller_id = ?", resellerId)
	} else {
		query = query.Where("reseller_id > 0")
	}
	if from > 0 {
		query = query.Where("created_at >= ?", from)
	}
	if to > 0 {
		query = query.Where("created_at <= ?", to)
	}
	var raw []struct {
		KeyName     string
		Requests    int64
		ChargeQuota int64
		CostQuota   int64
	}
	if err := query.Group(keyCol).Scan(&raw).Error; err != nil {
		return nil, err
	}
	rows := make([]GrossProfitRow, 0, len(raw))
	for _, r := range raw {
		rows = append(rows, GrossProfitRow{Key: r.KeyName, Requests: r.Requests, ChargeQuota: r.ChargeQuota, CostQuota: r.CostQuota, ProfitQuota: r.ChargeQuota - r.CostQuota})
	}
	return rows, nil
}

// SetUserStatusAndRevoke changes a user's status, bumps its auth version so
// existing sessions/caches are re-evaluated, and drops reseller caches.
func SetUserStatusAndRevoke(userId int, status int) error {
	if err := DB.Model(&User{}).Where("id = ?", userId).Update("status", status).Error; err != nil {
		return err
	}
	if _, err := BumpUserAuthVersion(userId); err != nil {
		common.SysError(common.LogText("failed to bump auth version for user %d: %s", userId, err.Error()))
		_ = invalidateUserCache(userId)
	}
	InvalidateResellerCache(userId)
	return nil
}

// UpdateResellerCustomerFields updates whitelisted customer fields and drops caches.
func UpdateResellerCustomerFields(customerId int, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if err := DB.Model(&User{}).Where("id = ?", customerId).Updates(fields).Error; err != nil {
		return err
	}
	if err := PublishUserAuthCache(customerId); err != nil {
		_ = invalidateUserCache(customerId)
	}
	InvalidateCustomerPricingCache(customerId)
	return nil
}

// TableName pins the design name (singular) for the audit table.
func (ResellerLedger) TableName() string { return "reseller_ledger" }
