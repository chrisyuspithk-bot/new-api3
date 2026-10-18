package model

import "gorm.io/gorm"

// RevenueRecordFilter selects revenue rows for the real-time profit list.
type RevenueRecordFilter struct {
	Scope      string // RevenueScopeAdmin / RevenueScopeReseller
	ResellerId int    // reseller scope: the session reseller; admin: optional filter
	CustomerId int    // optional: rows of this paying user
	From       int64  // inclusive unix seconds, 0 = open
	To         int64  // exclusive unix seconds, 0 = open
	Page       int
	PageSize   int
}

// RevenueRecordView is one row of the profit list with display names.
type RevenueRecordView struct {
	RevenueRecord
	Username         string `json:"username" gorm:"column:username"`
	ResellerUsername string `json:"reseller_username" gorm:"column:reseller_username"`
}

// RevenueRecordTotals sums the filtered rows (not only the current page).
type RevenueRecordTotals struct {
	Rows             int64 `json:"rows" gorm:"column:row_count"`
	CustomerPrice    int64 `json:"customer_price"`
	ResellerCost     int64 `json:"reseller_cost"`
	ResellerProfit   int64 `json:"reseller_profit"`
	AdminProfitMicro int64 `json:"admin_profit_micro,omitempty"`
	ResellerCredited int64 `json:"reseller_credited"`
	AdminCredited    int64 `json:"admin_credited,omitempty"`
}

const maxRevenueExportRows = 50000

func (f RevenueRecordFilter) query() *gorm.DB {
	q := DB.Table("revenue_records AS r")
	if f.From > 0 {
		q = q.Where("r.created_at >= ?", f.From)
	}
	if f.To > 0 {
		q = q.Where("r.created_at < ?", f.To)
	}
	if f.Scope == RevenueScopeReseller {
		q = q.Where("r.reseller_id = ? AND r.payer_type IN ?", f.ResellerId, []string{RevenuePayerCustomer, RevenuePayerResellerSelf})
	} else if f.ResellerId > 0 {
		q = q.Where("r.reseller_id = ?", f.ResellerId)
	}
	if f.CustomerId > 0 {
		q = q.Where("r.user_id = ?", f.CustomerId)
	}
	return q
}

// ListRevenueRecords returns one page (newest first), the total row count and
// the totals over every filtered row.
func ListRevenueRecords(f RevenueRecordFilter) ([]RevenueRecordView, RevenueRecordTotals, error) {
	var totals RevenueRecordTotals
	if err := f.query().Select("COUNT(*) AS row_count, COALESCE(SUM(r.customer_price),0) AS customer_price, " +
		"COALESCE(SUM(r.reseller_cost),0) AS reseller_cost, COALESCE(SUM(r.reseller_profit),0) AS reseller_profit, " +
		"COALESCE(SUM(r.admin_profit_micro),0) AS admin_profit_micro, COALESCE(SUM(r.reseller_credited),0) AS reseller_credited, " +
		"COALESCE(SUM(r.admin_credited),0) AS admin_credited").Scan(&totals).Error; err != nil {
		return nil, totals, err
	}
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.PageSize > 100 && f.PageSize != maxRevenueExportRows {
		f.PageSize = 100
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	var rows []RevenueRecordView
	err := f.query().
		Select("r.*, u.username AS username, ru.username AS reseller_username").
		Joins("LEFT JOIN users u ON u.id = r.user_id").
		Joins("LEFT JOIN users ru ON ru.id = r.reseller_id").
		Order("r.id DESC").
		Limit(f.PageSize).Offset((f.Page - 1) * f.PageSize).
		Scan(&rows).Error
	return rows, totals, err
}

// RevenueRecordsForExport returns up to maxRevenueExportRows filtered rows.
func RevenueRecordsForExport(f RevenueRecordFilter) ([]RevenueRecordView, RevenueRecordTotals, bool, error) {
	f.Page, f.PageSize = 1, maxRevenueExportRows
	rows, totals, err := ListRevenueRecords(f)
	return rows, totals, totals.Rows > int64(len(rows)), err
}
