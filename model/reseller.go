package model

import (
	"errors"
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
)

// ResellerLedger is the audit trail of manual balance movements. Consumption is
// recorded in logs only.
type ResellerLedger struct {
	Id         int64    `json:"id"`
	ResellerId int      `json:"reseller_id" gorm:"not null;index"`
	CustomerId int      `json:"customer_id" gorm:"not null;default:0;index"`
	Kind       string   `json:"kind" gorm:"type:varchar(32);not null"`
	QuotaDelta int64    `json:"quota_delta" gorm:"not null"`
	MoneyUsd   *float64 `json:"money_usd,omitempty" gorm:"type:decimal(18,6)"`
	OperatorId int      `json:"operator_id"`
	RefId      string   `json:"ref_id" gorm:"type:varchar(64);default:''"`
	Remark     string   `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedAt  int64    `json:"created_at" gorm:"bigint;index"`
}

var (
	ErrResellerNotFound     = errors.New("reseller not found")
	ErrResellerDisabled     = errors.New("reseller disabled")
	ErrInvalidMarkup        = errors.New("markup must be greater than 0")
	ErrGroupNotAllowed      = errors.New("group is not allowed for this reseller")
	ErrInsufficientQuota    = errors.New("insufficient quota")
	ErrCustomerLimitReached = errors.New("reseller customer limit reached")
)

// ValidMarkup reports whether m is an acceptable markup (0, +Inf).
func ValidMarkup(m float64) bool {
	return m > 0 && m < 1e6
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

const resellerCacheTTL = 30 * time.Second

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
	profile := &ResellerProfile{UserId: resellerId, DefaultMarkup: 1, AllowRedemption: true}
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
	for _, v := range []*float64{price.CostMultiplier, price.ModelRatio, price.ModelPrice, price.CompletionRatio} {
		if v != nil && !(*v >= 0) {
			return errors.New("price values must be non-negative")
		}
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

// AdjustQuotaWithLedger credits (delta>0) or debits (delta<0) a user's wallet
// through the cache-aware quota paths and appends a ledger row. Debits never
// take a balance below zero.
func AdjustQuotaWithLedger(userId int, delta int, entry ResellerLedger) error {
	if delta == 0 {
		return errors.New("quota delta cannot be zero")
	}
	if delta > 0 {
		if err := IncreaseUserQuota(userId, delta, true); err != nil {
			return err
		}
	} else {
		ok, err := TryReserveUserQuota(userId, -delta)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInsufficientQuota
		}
	}
	entry.QuotaDelta = int64(delta)
	entry.CreatedAt = common.GetTimestamp()
	if err := DB.Create(&entry).Error; err != nil {
		common.SysError(common.LogText("failed to write reseller ledger (user=%d, delta=%d): %s", userId, delta, err.Error()))
		return err
	}
	return nil
}

// GetResellerLedger lists ledger rows for a reseller, newest first.
func GetResellerLedger(resellerId int, startIdx, num int) ([]ResellerLedger, int64, error) {
	var rows []ResellerLedger
	var total int64
	query := DB.Model(&ResellerLedger{}).Where("reseller_id = ?", resellerId)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id desc").Offset(startIdx).Limit(num).Find(&rows).Error
	return rows, total, err
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
		Select(keyCol+" AS "+"key_name, COUNT(*) AS requests, COALESCE(SUM(quota),0) AS charge_quota, COALESCE(SUM(reseller_cost_quota),0) AS cost_quota").
		Where("type = ?", LogTypeConsume)
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
