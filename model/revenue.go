package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Revenue records: one row per billed call (and one negative row per refund
// or downward settlement), kept in a table separate from logs so earnings are
// auditable even when consume logging is off or logs live in ClickHouse.
//
// Money identity enforced on every row (micro = 1e-6 quota):
//
//	customer_price*1e6 == base_quota_micro + admin_profit_micro + reseller_profit*1e6
//
// customer_price, reseller_cost, reseller_profit and admin_revenue are the
// exact integer quota amounts that moved between wallets. base_quota_micro is
// the admin list price before reseller factors: charge / (cost_factor*markup).
const (
	RevenuePayerCustomer     = "customer"      // customer of a reseller
	RevenuePayerResellerSelf = "reseller_self" // reseller calling for itself
	RevenuePayerDirect       = "direct"        // user owned by the admin

	RevenueKindConsume = "consume"
	RevenueKindRefund  = "refund"

	RevenueSourceLive      = "live"
	RevenueSourceRetry     = "retry"
	RevenueSourceReconcile = "reconcile"

	// RevenueFlagInvalidFactors marks a row whose cost factor / markup were
	// missing or invalid: base and admin margin are left at zero rather than
	// fabricated, and reconciliation reports the row.
	RevenueFlagInvalidFactors = "invalid_factors"

	revenueMicro = 1_000_000
)

type RevenueRecord struct {
	Id               int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	DedupeKey        string  `json:"dedupe_key" gorm:"type:varchar(128);uniqueIndex:idx_revenue_dedupe;not null"`
	LogId            int64   `json:"log_id" gorm:"index;default:0"`
	RequestId        string  `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	CreatedAt        int64   `json:"created_at" gorm:"index:idx_revenue_created;index:idx_revenue_reseller_created,priority:2"`
	Kind             string  `json:"kind" gorm:"type:varchar(16);not null"`
	PayerType        string  `json:"payer_type" gorm:"type:varchar(16);index;not null"`
	UserId           int     `json:"user_id" gorm:"index;not null"`
	ResellerId       int     `json:"reseller_id" gorm:"index:idx_revenue_reseller_created,priority:1;default:0"`
	ModelName        string  `json:"model_name" gorm:"type:varchar(128);default:''"`
	CostFactor       float64 `json:"cost_factor"`
	Markup           float64 `json:"markup"`
	BaseQuotaMicro   int64   `json:"base_quota_micro" gorm:"type:bigint"`
	CustomerPrice    int64   `json:"customer_price" gorm:"type:bigint"`
	ResellerCost     int64   `json:"reseller_cost" gorm:"type:bigint"`
	ResellerProfit   int64   `json:"reseller_profit" gorm:"type:bigint"`
	AdminRevenue     int64   `json:"admin_revenue" gorm:"type:bigint"`
	AdminProfitMicro int64   `json:"admin_profit_micro" gorm:"type:bigint"`
	Source           string  `json:"source" gorm:"type:varchar(16);default:'live'"`
	Flags            string  `json:"flags" gorm:"type:varchar(64);default:''"`
	// Profit crediting (applied in the same transaction as the row insert).
	// ResellerCredited is the quota credited (negative: reversed) to the
	// reseller wallet; AdminCredited the whole quota units credited to the root
	// account after accumulating AdminProfitMicro. CreditState is "credited"
	// once applied ("" for rows written before crediting existed).
	ResellerCredited int64  `json:"reseller_credited" gorm:"type:bigint;default:0"`
	AdminCredited    int64  `json:"admin_credited" gorm:"type:bigint;default:0"`
	CreditState      string `json:"credit_state" gorm:"type:varchar(16);default:''"`

	pendingCredits []profitCredit `gorm:"-"`
	creditEligible bool           `gorm:"-"`
}

func (RevenueRecord) TableName() string { return "revenue_records" }

// RevenueContext is the pricing snapshot of a billed call, set on the gin
// context at settlement (ContextKeyRevenue) and frozen in task billing context.
type RevenueContext struct {
	PayerType  string  `json:"payer"`
	ResellerId int     `json:"rid,omitempty"`
	CostFactor float64 `json:"cf,omitempty"`
	Markup     float64 `json:"m,omitempty"`
	// ProfitCredit marks a billing context created after profit crediting was
	// introduced: its rows credit (refunds: reverse) reseller/admin profit.
	// Contexts without it (pre-deploy tasks, violation fees) are booked but
	// not credited, so a refund never reverses a credit that was never given.
	ProfitCredit bool `json:"pc,omitempty"`
}

const ContextKeyRevenue = "b2b2c_revenue_ctx"

// logOtherRevenueKey stores the RevenueContext in the admin-only part of a log
// so the reconciler can rebuild a missing revenue row from the log alone.
const logOtherRevenueKey = "revenue_ctx"

func SetRevenueContext(c *gin.Context, rc RevenueContext) {
	if c != nil {
		c.Set(ContextKeyRevenue, rc)
	}
}

func GetRevenueContext(c *gin.Context) (RevenueContext, bool) {
	if c == nil {
		return RevenueContext{}, false
	}
	v, ok := c.Get(ContextKeyRevenue)
	if !ok {
		return RevenueContext{}, false
	}
	rc, ok := v.(RevenueContext)
	return rc, ok
}

// BuildRevenueRecord derives a revenue row from the amounts that actually moved.
// charge is the customer (payer) wallet amount and resellerCost the reseller
// wallet amount, both >= 0; kind refund negates every amount.
func BuildRevenueRecord(rc RevenueContext, userId int, modelName, kind string, charge, resellerCost int64) RevenueRecord {
	rec := RevenueRecord{
		creditEligible: rc.ProfitCredit,
		CreatedAt:      common.GetTimestamp(),
		Kind:           kind,
		PayerType:      rc.PayerType,
		UserId:         userId,
		ModelName:      truncateRunes(modelName, 128),
	}
	if charge < 0 {
		charge = -charge
	}
	if resellerCost < 0 {
		resellerCost = -resellerCost
	}
	chargeMicro := decimal.NewFromInt(charge).Mul(decimal.NewFromInt(revenueMicro))
	switch rc.PayerType {
	case RevenuePayerCustomer:
		rec.ResellerId = rc.ResellerId
		rec.CostFactor, rec.Markup = rc.CostFactor, rc.Markup
		rec.CustomerPrice = charge
		rec.ResellerCost = resellerCost
		rec.ResellerProfit = charge - resellerCost
		rec.AdminRevenue = resellerCost
		if rc.ResellerId <= 0 || !ValidCostFactor(rc.CostFactor) || !ValidMarkup(rc.Markup) {
			rec.Flags = RevenueFlagInvalidFactors
		} else {
			base := chargeMicro.Div(decimal.NewFromFloat(rc.CostFactor).Mul(decimal.NewFromFloat(rc.Markup))).Round(0).IntPart()
			rec.BaseQuotaMicro = base
			rec.AdminProfitMicro = resellerCost*revenueMicro - base
		}
	case RevenuePayerResellerSelf:
		rec.ResellerId = rc.ResellerId
		rec.CostFactor, rec.Markup = rc.CostFactor, 1
		rec.CustomerPrice = charge
		rec.ResellerCost = charge
		rec.AdminRevenue = charge
		if rc.ResellerId <= 0 || !ValidCostFactor(rc.CostFactor) {
			rec.Flags = RevenueFlagInvalidFactors
		} else {
			base := chargeMicro.Div(decimal.NewFromFloat(rc.CostFactor)).Round(0).IntPart()
			rec.BaseQuotaMicro = base
			rec.AdminProfitMicro = charge*revenueMicro - base
		}
	default:
		rec.PayerType = RevenuePayerDirect
		rec.CostFactor, rec.Markup = 1, 1
		rec.CustomerPrice = charge
		rec.AdminRevenue = charge
		rec.BaseQuotaMicro = charge * revenueMicro
	}
	if rec.Flags == RevenueFlagInvalidFactors {
		// Fail closed: never invent a margin. The full charge is booked as
		// reseller profit only when the reseller really paid nothing.
		rec.BaseQuotaMicro = 0
		rec.AdminProfitMicro = 0
	}
	if kind == RevenueKindRefund {
		rec.BaseQuotaMicro = -rec.BaseQuotaMicro
		rec.CustomerPrice = -rec.CustomerPrice
		rec.ResellerCost = -rec.ResellerCost
		rec.ResellerProfit = -rec.ResellerProfit
		rec.AdminRevenue = -rec.AdminRevenue
		rec.AdminProfitMicro = -rec.AdminProfitMicro
	}
	return rec
}

// IdentityHolds checks the per-row money identity.
// For customer rows the two wallet legs also tie out:
// customer_price == reseller_cost + reseller_profit and admin_revenue == reseller_cost.
// For self/direct rows the payer's charge is the admin revenue.
func (r RevenueRecord) IdentityHolds() bool {
	legs := r.AdminRevenue == r.CustomerPrice && r.ResellerProfit == 0
	if r.PayerType == RevenuePayerCustomer {
		legs = r.CustomerPrice == r.ResellerCost+r.ResellerProfit && r.AdminRevenue == r.ResellerCost
	}
	if r.Flags == RevenueFlagInvalidFactors {
		return legs && r.BaseQuotaMicro == 0 && r.AdminProfitMicro == 0
	}
	return legs && r.CustomerPrice*revenueMicro == r.BaseQuotaMicro+r.AdminProfitMicro+r.ResellerProfit*revenueMicro
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

// insertRevenueTx inserts rec idempotently: an existing dedupe key is success.
// When the row is newly inserted, the reseller and admin profit of the row are
// credited in the same transaction (see applyProfitCreditsTx); a duplicate
// key credits nothing, so every row is credited exactly once.
// tx == DB opens its own transaction and syncs caches after commit; callers
// passing an outer transaction must call syncProfitCreditCaches after commit.
func insertRevenueTx(tx *gorm.DB, rec *RevenueRecord) error {
	if rec.DedupeKey == "" {
		return errors.New("revenue record without dedupe key")
	}
	if tx == DB {
		var credits []profitCredit
		err := DB.Transaction(func(t *gorm.DB) error {
			var e error
			credits, e = insertRevenueAndCreditTx(t, rec)
			return e
		})
		if err != nil {
			return err
		}
		syncProfitCreditCaches(credits)
		return nil
	}
	credits, err := insertRevenueAndCreditTx(tx, rec)
	if err != nil {
		return err
	}
	rec.pendingCredits = credits
	return nil
}

func insertRevenueAndCreditTx(tx *gorm.DB, rec *RevenueRecord) ([]profitCredit, error) {
	res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedupe_key"}}, DoNothing: true}).Create(rec)
	if res.Error != nil {
		if isDuplicateKeyErr(res.Error) {
			return nil, nil
		}
		return nil, res.Error
	}
	if res.RowsAffected == 0 || rec.Id == 0 {
		return nil, nil // already recorded (and credited) under this key
	}
	return applyProfitCreditsTx(tx, rec)
}

// revenueRetryDelays are the synchronous retry back-offs of a revenue write.
var revenueRetryDelays = []time.Duration{0, 20 * time.Millisecond, 100 * time.Millisecond, 400 * time.Millisecond}

// revenueInsertHook lets tests inject write failures.
var revenueInsertHook func(rec *RevenueRecord) error

// InsertRevenueRecord writes rec with synchronous retries; on persistent
// failure it is queued for the background retrier (and the log reconciler
// remains the last line of defence).
func InsertRevenueRecord(rec RevenueRecord) error {
	var err error
	for _, d := range revenueRetryDelays {
		if d > 0 {
			time.Sleep(d)
		}
		if revenueInsertHook != nil {
			if err = revenueInsertHook(&rec); err != nil {
				continue
			}
		}
		r := rec
		if err = insertRevenueTx(DB, &r); err == nil {
			return nil
		}
	}
	common.SysError(fmt.Sprintf("revenue record write failed after retries, queued (key=%s): %v", rec.DedupeKey, err))
	enqueuePendingRevenue(rec)
	return err
}

var (
	pendingRevenueMu sync.Mutex
	pendingRevenue   []RevenueRecord
)

const maxPendingRevenue = 100000

func enqueuePendingRevenue(rec RevenueRecord) {
	pendingRevenueMu.Lock()
	defer pendingRevenueMu.Unlock()
	if len(pendingRevenue) >= maxPendingRevenue {
		common.SysError("revenue pending queue full; dropping oldest (reconciler will rebuild from logs)")
		pendingRevenue = pendingRevenue[1:]
	}
	pendingRevenue = append(pendingRevenue, rec)
}

// PendingRevenueCount is the number of revenue rows waiting for retry.
func PendingRevenueCount() int {
	pendingRevenueMu.Lock()
	defer pendingRevenueMu.Unlock()
	return len(pendingRevenue)
}

// FlushPendingRevenue retries queued rows once and returns how many remain.
func FlushPendingRevenue() int {
	pendingRevenueMu.Lock()
	batch := pendingRevenue
	pendingRevenue = nil
	pendingRevenueMu.Unlock()
	var failed []RevenueRecord
	for _, rec := range batch {
		rec.Source = RevenueSourceRetry
		r := rec
		if revenueInsertHook != nil {
			if err := revenueInsertHook(&r); err != nil {
				failed = append(failed, rec)
				continue
			}
		}
		if err := insertRevenueTx(DB, &r); err != nil {
			failed = append(failed, rec)
		}
	}
	pendingRevenueMu.Lock()
	pendingRevenue = append(failed, pendingRevenue...)
	n := len(pendingRevenue)
	pendingRevenueMu.Unlock()
	return n
}

func logTypeRevenueKind(logType int) (string, bool) {
	switch logType {
	case LogTypeConsume:
		return RevenueKindConsume, true
	case LogTypeRefund:
		return RevenueKindRefund, true
	}
	return "", false
}

// logsShareMainDB reports whether logs and revenue live in one SQL database,
// so a log row and its revenue row can be committed in one transaction.
func logsShareMainDB() bool {
	return LOG_DB != nil && LOG_DB == DB
}

// createLogWithRevenue writes the log and, when rc is set and the log is a
// consume/refund with money moved, its revenue row. When logs share the main
// DB both are written in ONE transaction (revenue keyed by log id); otherwise
// the revenue row is written separately with retries.
func createLogWithRevenue(log *Log, rc *RevenueContext, writeLog bool) {
	var rec *RevenueRecord
	if rc != nil {
		if kind, ok := logTypeRevenueKind(log.Type); ok && (log.Quota != 0 || log.ResellerCostQuota != 0) {
			r := BuildRevenueRecord(*rc, log.UserId, log.ModelName, kind, int64(log.Quota), int64(log.ResellerCostQuota))
			r.CreatedAt = log.CreatedAt
			r.Source = RevenueSourceLive
			rec = &r
		}
	}
	if !writeLog {
		if rec != nil {
			ensureLogRequestId(log)
			rec.RequestId = log.RequestId
			rec.DedupeKey = fmt.Sprintf("req:%s:%s:%d", log.RequestId, rec.Kind, time.Now().UnixNano())
			_ = InsertRevenueRecord(*rec)
		}
		return
	}
	if rec != nil && logsShareMainDB() {
		ensureLogRequestId(log)
		err := DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(log).Error; err != nil {
				return err
			}
			if revenueInsertHook != nil {
				if err := revenueInsertHook(rec); err != nil {
					return err
				}
			}
			rec.LogId = int64(log.Id)
			rec.RequestId = log.RequestId
			rec.DedupeKey = fmt.Sprintf("log:%d", log.Id)
			return insertRevenueTx(tx, rec)
		})
		if err == nil {
			syncProfitCreditCaches(rec.pendingCredits)
			return
		}
		common.SysError(fmt.Sprintf("log+revenue transaction failed, falling back to separate writes: %v", err))
		log.Id = 0
		rec.Id = 0
	}
	if err := createLog(log); err != nil {
		common.SysError(fmt.Sprintf("failed to record log: %v", err))
	}
	if rec != nil {
		rec.RequestId = log.RequestId
		if log.Id > 0 && logsShareMainDB() {
			rec.LogId = int64(log.Id)
			rec.DedupeKey = fmt.Sprintf("log:%d", log.Id)
		} else {
			rec.DedupeKey = fmt.Sprintf("req:%s:%s:%d", log.RequestId, rec.Kind, time.Now().UnixNano())
		}
		_ = InsertRevenueRecord(*rec)
	}
}

// attachRevenueToOther stores the revenue context in the admin-only log info.
func attachRevenueToOther(other *LogOther, rc *RevenueContext) {
	if other == nil || rc == nil {
		return
	}
	other.SetAdmin(logOtherRevenueKey, *rc)
}

// FormatInt64 is a small helper used by tests and exports.
func FormatInt64(v int64) string { return fmt.Sprintf("%d", v) }
