package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	InvoiceScopeCustomer    = "customer"
	InvoiceScopeReseller    = "reseller"
	InvoiceScopeAdminRollup = "admin_rollup"
)

// Invoice is a persisted monthly billing snapshot generated from consume/refund logs.
type Invoice struct {
	Id            int64   `json:"id"`
	Period        string  `json:"period" gorm:"type:varchar(7);not null;uniqueIndex:uniq_invoice_scope,priority:1"` // YYYY-MM
	Scope         string  `json:"scope" gorm:"type:varchar(32);not null;uniqueIndex:uniq_invoice_scope,priority:2"`
	ResellerId    int     `json:"reseller_id" gorm:"not null;default:0;uniqueIndex:uniq_invoice_scope,priority:3;index"`
	CustomerId    int     `json:"customer_id" gorm:"not null;default:0;uniqueIndex:uniq_invoice_scope,priority:4;index"`
	ResellerName  string  `json:"reseller_name" gorm:"type:varchar(64);default:''"`
	CustomerName  string  `json:"customer_name" gorm:"type:varchar(64);default:''"`
	Requests      int64   `json:"requests" gorm:"not null;default:0"`
	ChargeQuota   int64   `json:"charge_quota" gorm:"type:bigint;not null;default:0"`
	CostQuota     int64   `json:"cost_quota" gorm:"type:bigint;not null;default:0"`
	ProfitQuota   int64   `json:"profit_quota" gorm:"type:bigint;not null;default:0"`
	ChargeUsd     float64 `json:"charge_usd" gorm:"type:decimal(18,6);not null;default:0"`
	CostUsd       float64 `json:"cost_usd" gorm:"type:decimal(18,6);not null;default:0"`
	ProfitUsd     float64 `json:"profit_usd" gorm:"type:decimal(18,6);not null;default:0"`
	LineItemsJSON string  `json:"line_items_json,omitempty" gorm:"type:text"`
	GeneratedAt   int64   `json:"generated_at" gorm:"bigint;not null;default:0;index"`
	GeneratedBy   int     `json:"generated_by" gorm:"not null;default:0"`
}

func (Invoice) TableName() string { return "invoices" }

// InvoiceLineItem is one model bucket inside an invoice.
type InvoiceLineItem struct {
	Model       string  `json:"model"`
	Requests    int64   `json:"requests"`
	ChargeQuota int64   `json:"charge_quota"`
	CostQuota   int64   `json:"cost_quota"`
	ProfitQuota int64   `json:"profit_quota"`
	ChargeUsd   float64 `json:"charge_usd"`
	CostUsd     float64 `json:"cost_usd"`
	ProfitUsd   float64 `json:"profit_usd"`
}

// InvoiceGenerateResult summarizes a generate run.
type InvoiceGenerateResult struct {
	Period           string `json:"period"`
	CustomerInvoices int    `json:"customer_invoices"`
	ResellerInvoices int    `json:"reseller_invoices"`
	AdminRollup      bool   `json:"admin_rollup"`
	FromUnix         int64  `json:"from_unix"`
	ToUnix           int64  `json:"to_unix"`
}

var (
	ErrInvalidInvoicePeriod = errors.New("period must be YYYY-MM")
	ErrInvoiceNotFound      = errors.New("invoice not found")
)

// ParseInvoicePeriod validates YYYY-MM and returns [from, toExclusive) unix bounds in Asia/Hong_Kong.
func ParseInvoicePeriod(period string) (from, toExclusive int64, err error) {
	if len(period) != 7 || period[4] != '-' {
		return 0, 0, ErrInvalidInvoicePeriod
	}
	loc, locErr := time.LoadLocation("Asia/Hong_Kong")
	if locErr != nil {
		loc = time.FixedZone("HKT", 8*3600)
	}
	start, err := time.ParseInLocation("2006-01", period, loc)
	if err != nil {
		return 0, 0, ErrInvalidInvoicePeriod
	}
	end := start.AddDate(0, 1, 0)
	return start.Unix(), end.Unix(), nil
}

func quotaToUsd(quota int64) float64 {
	if common.QuotaPerUnit <= 0 {
		return 0
	}
	return float64(quota) / common.QuotaPerUnit
}

type invoiceAggRow struct {
	ResellerId  int
	CustomerId  int
	Username    string
	ModelName   string
	Requests    int64
	ChargeQuota int64
	CostQuota   int64
}

func scanInvoiceAggs(from, toExclusive int64) ([]invoiceAggRow, error) {
	query := LOG_DB.Model(&Log{}).
		Select("reseller_id, user_id AS customer_id, username, model_name, "+
			"COALESCE(SUM(CASE WHEN type = ? THEN 1 ELSE 0 END),0) AS requests, "+
			"COALESCE(SUM(CASE WHEN type = ? THEN -quota ELSE quota END),0) AS charge_quota, "+
			"COALESCE(SUM(CASE WHEN type = ? THEN -reseller_cost_quota ELSE reseller_cost_quota END),0) AS cost_quota",
			LogTypeConsume, LogTypeRefund, LogTypeRefund).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("reseller_id > 0").
		Where("created_at >= ? AND created_at < ?", from, toExclusive).
		Group("reseller_id, user_id, username, model_name")
	var raw []invoiceAggRow
	if err := query.Scan(&raw).Error; err != nil {
		return nil, err
	}
	return raw, nil
}

func fillUsd(inv *Invoice) {
	inv.ProfitQuota = inv.ChargeQuota - inv.CostQuota
	inv.ChargeUsd = quotaToUsd(inv.ChargeQuota)
	inv.CostUsd = quotaToUsd(inv.CostQuota)
	inv.ProfitUsd = quotaToUsd(inv.ProfitQuota)
}

func lineItemsFromModelRows(rows []invoiceAggRow) string {
	// Merge by model name (caller may pass unmerged rows).
	merged := map[string]*InvoiceLineItem{}
	order := make([]string, 0)
	for _, r := range rows {
		key := r.ModelName
		if item := merged[key]; item == nil {
			profit := r.ChargeQuota - r.CostQuota
			merged[key] = &InvoiceLineItem{
				Model:       key,
				Requests:    r.Requests,
				ChargeQuota: r.ChargeQuota,
				CostQuota:   r.CostQuota,
				ProfitQuota: profit,
				ChargeUsd:   quotaToUsd(r.ChargeQuota),
				CostUsd:     quotaToUsd(r.CostQuota),
				ProfitUsd:   quotaToUsd(profit),
			}
			order = append(order, key)
		} else {
			item.Requests += r.Requests
			item.ChargeQuota += r.ChargeQuota
			item.CostQuota += r.CostQuota
			item.ProfitQuota = item.ChargeQuota - item.CostQuota
			item.ChargeUsd = quotaToUsd(item.ChargeQuota)
			item.CostUsd = quotaToUsd(item.CostQuota)
			item.ProfitUsd = quotaToUsd(item.ProfitQuota)
		}
	}
	items := make([]InvoiceLineItem, 0, len(order))
	for _, k := range order {
		items = append(items, *merged[k])
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func upsertInvoice(inv *Invoice) error {
	var existing Invoice
	err := DB.Where("period = ? AND scope = ? AND reseller_id = ? AND customer_id = ?",
		inv.Period, inv.Scope, inv.ResellerId, inv.CustomerId).First(&existing).Error
	if err == nil {
		inv.Id = existing.Id
		return DB.Model(&existing).Select(
			"reseller_name", "customer_name", "requests",
			"charge_quota", "cost_quota", "profit_quota",
			"charge_usd", "cost_usd", "profit_usd",
			"line_items_json", "generated_at", "generated_by",
		).Updates(inv).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DB.Create(inv).Error
	}
	return err
}

// GenerateMonthlyInvoices builds customer, reseller, and admin_rollup invoices for period.
// Existing rows for the same unique key are replaced. generatedBy is the admin user id.
func GenerateMonthlyInvoices(period string, generatedBy int) (*InvoiceGenerateResult, error) {
	from, toExclusive, err := ParseInvoicePeriod(period)
	if err != nil {
		return nil, err
	}
	now := common.GetTimestamp()

	customerRows, err := scanInvoiceAggs(from, toExclusive)
	if err != nil {
		return nil, err
	}

	type custKey struct{ ResellerId, CustomerId int }
	type custBucket struct {
		Username string
		Requests int64
		Charge   int64
		Cost     int64
		Models   []invoiceAggRow
	}
	customers := map[custKey]*custBucket{}
	resellerTotals := map[int]*custBucket{}
	var rollupRequests, rollupCharge, rollupCost int64
	var rollupModels []invoiceAggRow

	for _, r := range customerRows {
		ck := custKey{r.ResellerId, r.CustomerId}
		b := customers[ck]
		if b == nil {
			b = &custBucket{Username: r.Username}
			customers[ck] = b
		}
		if b.Username == "" && r.Username != "" {
			b.Username = r.Username
		}
		b.Requests += r.Requests
		b.Charge += r.ChargeQuota
		b.Cost += r.CostQuota
		b.Models = append(b.Models, r)

		rb := resellerTotals[r.ResellerId]
		if rb == nil {
			rb = &custBucket{}
			resellerTotals[r.ResellerId] = rb
		}
		rb.Requests += r.Requests
		rb.Charge += r.ChargeQuota
		rb.Cost += r.CostQuota
		rb.Models = append(rb.Models, r)

		rollupRequests += r.Requests
		rollupCharge += r.ChargeQuota
		rollupCost += r.CostQuota
		rollupModels = append(rollupModels, r)
	}

	resellerNames := map[int]string{}
	if len(resellerTotals) > 0 {
		ids := make([]int, 0, len(resellerTotals))
		for id := range resellerTotals {
			ids = append(ids, id)
		}
		var users []User
		if err := DB.Select("id, username, display_name").Where("id IN ?", ids).Find(&users).Error; err == nil {
			for _, u := range users {
				name := u.DisplayName
				if name == "" {
					name = u.Username
				}
				resellerNames[u.Id] = name
			}
		}
	}

	result := &InvoiceGenerateResult{Period: period, FromUnix: from, ToUnix: toExclusive - 1}

	for ck, b := range customers {
		inv := &Invoice{
			Period:        period,
			Scope:         InvoiceScopeCustomer,
			ResellerId:    ck.ResellerId,
			CustomerId:    ck.CustomerId,
			ResellerName:  resellerNames[ck.ResellerId],
			CustomerName:  b.Username,
			Requests:      b.Requests,
			ChargeQuota:   b.Charge,
			CostQuota:     b.Cost,
			LineItemsJSON: lineItemsFromModelRows(b.Models),
			GeneratedAt:   now,
			GeneratedBy:   generatedBy,
		}
		fillUsd(inv)
		if err := upsertInvoice(inv); err != nil {
			return nil, fmt.Errorf("customer invoice %d/%d: %w", ck.ResellerId, ck.CustomerId, err)
		}
		result.CustomerInvoices++
	}

	for rid, b := range resellerTotals {
		inv := &Invoice{
			Period:        period,
			Scope:         InvoiceScopeReseller,
			ResellerId:    rid,
			CustomerId:    0,
			ResellerName:  resellerNames[rid],
			Requests:      b.Requests,
			ChargeQuota:   b.Charge,
			CostQuota:     b.Cost,
			LineItemsJSON: lineItemsFromModelRows(b.Models),
			GeneratedAt:   now,
			GeneratedBy:   generatedBy,
		}
		fillUsd(inv)
		if err := upsertInvoice(inv); err != nil {
			return nil, fmt.Errorf("reseller invoice %d: %w", rid, err)
		}
		result.ResellerInvoices++
	}

	adminInv := &Invoice{
		Period:        period,
		Scope:         InvoiceScopeAdminRollup,
		ResellerId:    0,
		CustomerId:    0,
		Requests:      rollupRequests,
		ChargeQuota:   rollupCharge,
		CostQuota:     rollupCost,
		LineItemsJSON: lineItemsFromModelRows(rollupModels),
		GeneratedAt:   now,
		GeneratedBy:   generatedBy,
	}
	fillUsd(adminInv)
	if err := upsertInvoice(adminInv); err != nil {
		return nil, fmt.Errorf("admin rollup: %w", err)
	}
	result.AdminRollup = true
	return result, nil
}

// InvoiceListFilter selects persisted invoices.
type InvoiceListFilter struct {
	Period             string
	Scope              string
	ResellerId         int
	CustomerId         int
	RestrictResellerId int
	StartIdx           int
	Num                int
}

// ListInvoices returns a page of invoices matching the filter.
func ListInvoices(f InvoiceListFilter) ([]Invoice, int64, error) {
	query := DB.Model(&Invoice{})
	if f.Period != "" {
		query = query.Where("period = ?", f.Period)
	}
	if f.Scope != "" {
		query = query.Where("scope = ?", f.Scope)
	}
	if f.ResellerId > 0 {
		query = query.Where("reseller_id = ?", f.ResellerId)
	}
	if f.CustomerId > 0 {
		query = query.Where("customer_id = ?", f.CustomerId)
	}
	if f.RestrictResellerId > 0 {
		query = query.Where("reseller_id = ? AND scope IN ?", f.RestrictResellerId,
			[]string{InvoiceScopeCustomer, InvoiceScopeReseller})
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Num <= 0 {
		f.Num = 20
	}
	var rows []Invoice
	err := query.Order("period desc, scope asc, reseller_id asc, customer_id asc").
		Offset(f.StartIdx).Limit(f.Num).Find(&rows).Error
	return rows, total, err
}

// GetInvoiceById loads one invoice by primary key.
func GetInvoiceById(id int64) (*Invoice, error) {
	var inv Invoice
	if err := DB.First(&inv, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvoiceNotFound
		}
		return nil, err
	}
	return &inv, nil
}

// InvoiceLineItems parses LineItemsJSON.
func (inv *Invoice) InvoiceLineItems() []InvoiceLineItem {
	if inv.LineItemsJSON == "" {
		return nil
	}
	var items []InvoiceLineItem
	_ = json.Unmarshal([]byte(inv.LineItemsJSON), &items)
	return items
}
