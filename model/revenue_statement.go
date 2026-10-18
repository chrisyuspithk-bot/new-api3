package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RevenueScopeAdmin    = "admin"
	RevenueScopeReseller = "reseller"
)

// RevenueAgg is one line of an earnings statement. Amounts are signed quota
// (refund rows already negative); *_micro are 1e-6 quota.
type RevenueAgg struct {
	Key              string `json:"key"`
	Id               int    `json:"id"`
	Name             string `json:"name"`
	Requests         int64  `json:"requests"`
	Refunds          int64  `json:"refunds"`
	CustomerPrice    int64  `json:"customer_price"`
	ResellerCost     int64  `json:"reseller_cost"`
	ResellerProfit   int64  `json:"reseller_profit"`
	AdminRevenue     int64  `json:"admin_revenue"`
	BaseQuotaMicro   int64  `json:"base_quota_micro,omitempty"`
	AdminProfitMicro int64  `json:"admin_profit_micro,omitempty"`
	FlaggedRows      int64  `json:"flagged_rows"`
}

func (a *RevenueAgg) add(b RevenueAgg) {
	a.Requests += b.Requests
	a.Refunds += b.Refunds
	a.CustomerPrice += b.CustomerPrice
	a.ResellerCost += b.ResellerCost
	a.ResellerProfit += b.ResellerProfit
	a.AdminRevenue += b.AdminRevenue
	a.BaseQuotaMicro += b.BaseQuotaMicro
	a.AdminProfitMicro += b.AdminProfitMicro
	a.FlaggedRows += b.FlaggedRows
}

// RevenueReconciliation holds the cross-checks printed on every statement.
type RevenueReconciliation struct {
	// IdentityDiffMicro = Σcharge·1e6 − (Σbase + Σadmin_profit + Σreseller_profit·1e6)
	// over unflagged rows; must be 0.
	IdentityOK        bool  `json:"identity_ok"`
	IdentityDiffMicro int64 `json:"identity_diff_micro"`
	// Totals = per-line sums (customer + self + direct).
	SectionsOK bool `json:"sections_ok"`
	// Log cross-check: consume minus refund logs that carry a revenue context.
	LogCompared        bool  `json:"log_compared"`
	LogChargeQuota     int64 `json:"log_charge_quota"`
	LogResellerCost    int64 `json:"log_reseller_cost"`
	LogChargeDiff      int64 `json:"log_charge_diff"`
	LogResellerCostDif int64 `json:"log_reseller_cost_diff"`
	FlaggedRows        int64 `json:"flagged_rows"`
	PendingRetry       int   `json:"pending_retry"`
}

type RevenueStatement struct {
	Period       string     `json:"period"`
	Timezone     string     `json:"timezone"`
	Scope        string     `json:"scope"`
	ResellerId   int        `json:"reseller_id,omitempty"`
	ResellerName string     `json:"reseller_name,omitempty"`
	Totals       RevenueAgg `json:"totals"`
	CustomerSum  RevenueAgg `json:"customer_total"`
	SelfSum      RevenueAgg `json:"self_total"`
	DirectSum    RevenueAgg `json:"direct_total"`
	// DirectCreditSum: direct-credit top-ups/reversals (split booked at top-up).
	DirectCreditSum RevenueAgg   `json:"direct_credit_total"`
	ByReseller      []RevenueAgg `json:"by_reseller,omitempty"`
	ResellerSelf    []RevenueAgg `json:"reseller_self,omitempty"`
	Direct          []RevenueAgg `json:"direct,omitempty"`
	ByCustomer      []RevenueAgg `json:"by_customer,omitempty"`

	Reconciliation RevenueReconciliation `json:"reconciliation"`

	RowCount         int64  `json:"row_count"`
	Checksum         string `json:"checksum"`
	Frozen           bool   `json:"frozen"`
	FrozenAt         int64  `json:"frozen_at,omitempty"`
	LateRows         int64  `json:"late_rows"`
	ChecksumMismatch bool   `json:"checksum_mismatch"`
	GeneratedAt      int64  `json:"generated_at"`
}

// RevenueMonthlySnapshot freezes a closed month's statement per scope.
type RevenueMonthlySnapshot struct {
	Id       int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Period   string `json:"period" gorm:"type:varchar(7);uniqueIndex:idx_revenue_snap,priority:1;not null"`
	ScopeKey string `json:"scope_key" gorm:"type:varchar(32);uniqueIndex:idx_revenue_snap,priority:2;not null"`
	Payload  string `json:"-" gorm:"type:text"`
	RowCount int64  `json:"row_count"`
	Checksum string `json:"checksum" gorm:"type:varchar(64)"`
	FrozenAt int64  `json:"frozen_at"`
}

func (RevenueMonthlySnapshot) TableName() string { return "revenue_monthly_snapshots" }

func revenueScopeKey(scope string, resellerId int) string {
	if scope == RevenueScopeReseller {
		return "reseller:" + strconv.Itoa(resellerId)
	}
	return RevenueScopeAdmin
}

// revenueScopeQuery restricts revenue rows to the scope: a reseller sees its
// customers' rows and its own self-use rows (both carry reseller_id = it).
func revenueScopeQuery(db *gorm.DB, scope string, resellerId int, from, toEx int64) *gorm.DB {
	q := db.Model(&RevenueRecord{}).Where("created_at >= ? AND created_at < ?", from, toEx)
	if scope == RevenueScopeReseller {
		q = q.Where("reseller_id = ? AND payer_type IN ?", resellerId, []string{RevenuePayerCustomer, RevenuePayerResellerSelf})
	}
	return q
}

type revenueAggRow struct {
	PayerType        string
	ResellerId       int
	UserId           int
	Requests         int64
	Refunds          int64
	CustomerPrice    int64
	ResellerCost     int64
	ResellerProfit   int64
	AdminRevenue     int64
	BaseQuotaMicro   int64
	AdminProfitMicro int64
	FlaggedRows      int64
	IdentityMicro    int64
}

// revenueFingerprint returns the row count and a sha256 over every row in scope.
func revenueFingerprint(scope string, resellerId int, from, toEx int64) (int64, string, error) {
	var rows []RevenueRecord
	if err := revenueScopeQuery(DB, scope, resellerId, from, toEx).
		Select("id, dedupe_key, customer_price, reseller_cost, base_quota_micro, admin_profit_micro").
		Order("id").Find(&rows).Error; err != nil {
		return 0, "", err
	}
	h := sha256.New()
	for _, r := range rows {
		fmt.Fprintf(h, "%d|%s|%d|%d|%d|%d\n", r.Id, r.DedupeKey, r.CustomerPrice, r.ResellerCost, r.BaseQuotaMicro, r.AdminProfitMicro)
	}
	return int64(len(rows)), hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeRevenueStatement aggregates revenue rows live (no snapshot).
func ComputeRevenueStatement(period, scope string, resellerId int) (*RevenueStatement, error) {
	if scope != RevenueScopeAdmin && scope != RevenueScopeReseller {
		return nil, errors.New("invalid revenue scope")
	}
	if scope == RevenueScopeReseller && resellerId <= 0 {
		return nil, errors.New("reseller scope requires reseller id")
	}
	from, toEx, err := ParseInvoicePeriod(period)
	if err != nil {
		return nil, err
	}
	var raw []revenueAggRow
	err = revenueScopeQuery(DB, scope, resellerId, from, toEx).Select(
		"payer_type, reseller_id, user_id, " +
			"COALESCE(SUM(CASE WHEN kind = 'consume' THEN 1 ELSE 0 END),0) AS requests, " +
			"COALESCE(SUM(CASE WHEN kind = 'refund' THEN 1 ELSE 0 END),0) AS refunds, " +
			"COALESCE(SUM(customer_price),0) AS customer_price, " +
			"COALESCE(SUM(reseller_cost),0) AS reseller_cost, " +
			"COALESCE(SUM(reseller_profit),0) AS reseller_profit, " +
			"COALESCE(SUM(admin_revenue),0) AS admin_revenue, " +
			"COALESCE(SUM(base_quota_micro),0) AS base_quota_micro, " +
			"COALESCE(SUM(admin_profit_micro),0) AS admin_profit_micro, " +
			"COALESCE(SUM(CASE WHEN flags <> '' THEN 1 ELSE 0 END),0) AS flagged_rows, " +
			"COALESCE(SUM(CASE WHEN flags = '' THEN customer_price*1000000 - base_quota_micro - admin_profit_micro - reseller_profit*1000000 ELSE 0 END),0) AS identity_micro").
		Group("payer_type, reseller_id, user_id").Scan(&raw).Error
	if err != nil {
		return nil, err
	}
	st := &RevenueStatement{Period: period, Timezone: InvoicePeriodLocation().String(), Scope: scope, ResellerId: resellerId, GeneratedAt: common.GetTimestamp()}
	byReseller := map[int]*RevenueAgg{}
	selfByReseller := map[int]*RevenueAgg{}
	byUser := map[int]*RevenueAgg{}
	ids := map[int]struct{}{}
	var identity int64
	for _, r := range raw {
		a := RevenueAgg{Requests: r.Requests, Refunds: r.Refunds, CustomerPrice: r.CustomerPrice, ResellerCost: r.ResellerCost,
			ResellerProfit: r.ResellerProfit, AdminRevenue: r.AdminRevenue, BaseQuotaMicro: r.BaseQuotaMicro,
			AdminProfitMicro: r.AdminProfitMicro, FlaggedRows: r.FlaggedRows}
		identity += r.IdentityMicro
		st.Totals.add(a)
		switch r.PayerType {
		case RevenuePayerCustomer:
			st.CustomerSum.add(a)
			if scope == RevenueScopeAdmin {
				line(byReseller, r.ResellerId).add(a)
				ids[r.ResellerId] = struct{}{}
			} else {
				line(byUser, r.UserId).add(a)
				ids[r.UserId] = struct{}{}
			}
		case RevenuePayerDirectCredit:
			st.DirectCreditSum.add(a)
		case RevenuePayerResellerSelf:
			st.SelfSum.add(a)
			line(selfByReseller, r.ResellerId).add(a)
			ids[r.ResellerId] = struct{}{}
		default:
			st.DirectSum.add(a)
			if scope == RevenueScopeAdmin {
				line(byUser, r.UserId).add(a)
				ids[r.UserId] = struct{}{}
			}
		}
	}
	if scope == RevenueScopeReseller {
		ids[resellerId] = struct{}{}
	}
	names := revenueUserNames(ids)
	st.ByReseller = sortedLines(byReseller, names)
	if scope == RevenueScopeAdmin {
		st.ResellerSelf = sortedLines(selfByReseller, names)
		st.Direct = sortedLines(byUser, names)
	} else {
		st.ByCustomer = sortedLines(byUser, names)
		st.ResellerName = names[resellerId]
		// Admin list price and admin margin are not the reseller's business.
		stripAdminFields(&st.Totals)
		stripAdminFields(&st.CustomerSum)
		stripAdminFields(&st.SelfSum)
		stripAdminFields(&st.DirectCreditSum)
		for i := range st.ByCustomer {
			stripAdminFields(&st.ByCustomer[i])
		}
	}
	var sum RevenueAgg
	sum.add(st.CustomerSum)
	sum.add(st.SelfSum)
	sum.add(st.DirectSum)
	sum.add(st.DirectCreditSum)
	rec := &st.Reconciliation
	rec.SectionsOK = sum == st.Totals
	rec.IdentityDiffMicro = identity
	rec.IdentityOK = identity == 0
	rec.FlaggedRows = st.Totals.FlaggedRows
	rec.PendingRetry = PendingRevenueCount()
	if logsShareMainDB() {
		var lg struct {
			Charge int64
			Cost   int64
		}
		q := LOG_DB.Model(&Log{}).Where("created_at >= ? AND created_at < ? AND type IN ? AND other LIKE ?", from, toEx,
			[]int{LogTypeConsume, LogTypeRefund}, "%\""+logOtherRevenueKey+"\"%")
		if scope == RevenueScopeReseller {
			q = q.Where("(reseller_id = ? OR user_id = ?)", resellerId, resellerId)
		}
		if err := q.Select(fmt.Sprintf("COALESCE(SUM(CASE WHEN type = %d THEN -quota ELSE quota END),0) AS charge, "+
			"COALESCE(SUM(CASE WHEN type = %d THEN -reseller_cost_quota ELSE reseller_cost_quota END),0) AS cost", LogTypeRefund, LogTypeRefund)).
			Scan(&lg).Error; err == nil {
			rec.LogCompared = true
			rec.LogChargeQuota = lg.Charge
			rec.LogResellerCost = lg.Cost
			// Direct-credit top-ups have no consume logs.
			rec.LogChargeDiff = st.Totals.CustomerPrice - st.DirectCreditSum.CustomerPrice - lg.Charge
			// Customer rows' reseller cost is what the log reseller leg moved.
			rec.LogResellerCostDif = st.CustomerSum.ResellerCost - lg.Cost
		}
	}
	st.RowCount, st.Checksum, err = revenueFingerprint(scope, resellerId, from, toEx)
	if err != nil {
		return nil, err
	}
	return st, nil
}

func stripAdminFields(a *RevenueAgg) {
	a.BaseQuotaMicro = 0
	a.AdminProfitMicro = 0
}

func line(m map[int]*RevenueAgg, id int) *RevenueAgg {
	if a, ok := m[id]; ok {
		return a
	}
	a := &RevenueAgg{Id: id, Key: strconv.Itoa(id)}
	m[id] = a
	return a
}

func sortedLines(m map[int]*RevenueAgg, names map[int]string) []RevenueAgg {
	out := make([]RevenueAgg, 0, len(m))
	for id, a := range m {
		a.Name = names[id]
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CustomerPrice != out[j].CustomerPrice {
			return out[i].CustomerPrice > out[j].CustomerPrice
		}
		return out[i].Id < out[j].Id
	})
	return out
}

func revenueUserNames(ids map[int]struct{}) map[int]string {
	names := map[int]string{}
	if len(ids) == 0 {
		return names
	}
	list := make([]int, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	var users []struct {
		Id       int
		Username string
	}
	DB.Model(&User{}).Unscoped().Select("id, username").Where("id IN ?", list).Scan(&users)
	for _, u := range users {
		names[u.Id] = u.Username
	}
	return names
}

// revenuePeriodClosed reports whether the month has fully ended.
func revenuePeriodClosed(toEx int64) bool {
	return common.GetTimestamp() >= toEx
}

// GetRevenueStatement returns the statement, freezing closed months: the
// first read after a month ends stores a snapshot, later reads return it and
// flag rows that arrived afterwards (late_rows / checksum_mismatch).
func GetRevenueStatement(period, scope string, resellerId int) (*RevenueStatement, error) {
	_, toEx, err := ParseInvoicePeriod(period)
	if err != nil {
		return nil, err
	}
	if !revenuePeriodClosed(toEx) {
		return ComputeRevenueStatement(period, scope, resellerId)
	}
	key := revenueScopeKey(scope, resellerId)
	var snap RevenueMonthlySnapshot
	err = DB.Where("period = ? AND scope_key = ?", period, key).First(&snap).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		st, cerr := ComputeRevenueStatement(period, scope, resellerId)
		if cerr != nil {
			return nil, cerr
		}
		st.Frozen = true
		st.FrozenAt = common.GetTimestamp()
		payload, _ := json.Marshal(st)
		snap = RevenueMonthlySnapshot{Period: period, ScopeKey: key, Payload: string(payload), RowCount: st.RowCount, Checksum: st.Checksum, FrozenAt: st.FrozenAt}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&snap).Error; err != nil {
			return nil, err
		}
		if err := DB.Where("period = ? AND scope_key = ?", period, key).First(&snap).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	var st RevenueStatement
	if err := json.Unmarshal([]byte(snap.Payload), &st); err != nil {
		return nil, err
	}
	from, toEx2, _ := ParseInvoicePeriod(period)
	n, sum, err := revenueFingerprint(scope, resellerId, from, toEx2)
	if err != nil {
		return nil, err
	}
	st.LateRows = n - snap.RowCount
	st.ChecksumMismatch = sum != snap.Checksum
	return &st, nil
}

// RevenueRowsForExport lists the scope's rows for the CSV detail section.
func RevenueRowsForExport(period, scope string, resellerId int) ([]RevenueRecord, error) {
	from, toEx, err := ParseInvoicePeriod(period)
	if err != nil {
		return nil, err
	}
	var rows []RevenueRecord
	err = revenueScopeQuery(DB, scope, resellerId, from, toEx).Order("id").Find(&rows).Error
	return rows, err
}
