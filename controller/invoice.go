package controller

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func invoiceIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

func writeInvoiceCSV(c *gin.Context, inv *model.Invoice) {
	filename := fmt.Sprintf("invoice-%s-%s-%d.csv", inv.Period, inv.Scope, inv.Id)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{
		"period", "scope", "reseller_id", "reseller_name", "customer_id", "customer_name",
		"requests", "charge_quota", "cost_quota", "profit_quota",
		"charge_usd", "cost_usd", "profit_usd", "model",
		"model_requests", "model_charge_quota", "model_cost_quota", "model_profit_quota",
		"model_charge_usd", "model_cost_usd", "model_profit_usd",
	})
	items := inv.InvoiceLineItems()
	if len(items) == 0 {
		_ = w.Write([]string{
			inv.Period, inv.Scope,
			strconv.Itoa(inv.ResellerId), inv.ResellerName,
			strconv.Itoa(inv.CustomerId), inv.CustomerName,
			strconv.FormatInt(inv.Requests, 10),
			strconv.FormatInt(inv.ChargeQuota, 10),
			strconv.FormatInt(inv.CostQuota, 10),
			strconv.FormatInt(inv.ProfitQuota, 10),
			fmt.Sprintf("%.6f", inv.ChargeUsd),
			fmt.Sprintf("%.6f", inv.CostUsd),
			fmt.Sprintf("%.6f", inv.ProfitUsd),
			"", "", "", "", "", "", "", "",
		})
	} else {
		for _, it := range items {
			_ = w.Write([]string{
				inv.Period, inv.Scope,
				strconv.Itoa(inv.ResellerId), inv.ResellerName,
				strconv.Itoa(inv.CustomerId), inv.CustomerName,
				strconv.FormatInt(inv.Requests, 10),
				strconv.FormatInt(inv.ChargeQuota, 10),
				strconv.FormatInt(inv.CostQuota, 10),
				strconv.FormatInt(inv.ProfitQuota, 10),
				fmt.Sprintf("%.6f", inv.ChargeUsd),
				fmt.Sprintf("%.6f", inv.CostUsd),
				fmt.Sprintf("%.6f", inv.ProfitUsd),
				it.Model,
				strconv.FormatInt(it.Requests, 10),
				strconv.FormatInt(it.ChargeQuota, 10),
				strconv.FormatInt(it.CostQuota, 10),
				strconv.FormatInt(it.ProfitQuota, 10),
				fmt.Sprintf("%.6f", it.ChargeUsd),
				fmt.Sprintf("%.6f", it.CostUsd),
				fmt.Sprintf("%.6f", it.ProfitUsd),
			})
		}
	}
	w.Flush()
}

// csvSafeText neutralises spreadsheet formulas in user-entered CSV text.
func csvSafeText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func usdCell(v float64) string { return fmt.Sprintf("%.6f", v) }

var resellerLedgerKinds = []string{
	model.ResellerLedgerAdminCredit,
	model.ResellerLedgerAdminDebit,
	model.ResellerLedgerCustomerCredit,
	model.ResellerLedgerCustomerDebit,
	model.ResellerLedgerRedemptionIssue,
	model.ResellerLedgerRedemptionRefund,
	model.ResellerLedgerRedemptionRedeem,
}

// ledgerExportScope is the role scope of a balance-ledger export.
// internal adds operator/ref/remark columns and the cost/profit fee lines.
type ledgerExportScope struct {
	resellerId int
	customerId int
	internal   bool
}

// ledgerStatement is one month of the balance ledger for a scope: a summary
// (credit increase/decrease over every kind, plus usage and fees from the same
// log aggregation as monthly invoices), per-kind totals, then every ledger
// row. Only the row listing honours the optional kind filter.
type ledgerStatement struct {
	scope      ledgerExportScope
	period     string
	rowFilter  string
	kindTotals []model.LedgerKindTotal
	usage      *model.Invoice
	rows       []model.ResellerLedger
	// increase/decrease are wallet credits; issued/refunded are redemption
	// escrow moves, reported to internal scopes only.
	increase, decrease, issued, refunded model.LedgerKindTotal
}

// writeLedgerExport answers a balance-ledger export as a localized PDF
// statement, or as CSV with format=csv.
func writeLedgerExport(c *gin.Context, scope ledgerExportScope) {
	format := strings.TrimSpace(c.Query("format"))
	if format != "" && format != "pdf" && format != "csv" {
		common.ApiErrorMsg(c, "invalid format")
		return
	}
	period := strings.TrimSpace(c.Query("period"))
	from, toEx, err := model.ParseInvoicePeriod(period)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	kind := strings.TrimSpace(c.Query("kind"))
	if kind != "" && !slices.Contains(resellerLedgerKinds, kind) {
		common.ApiErrorMsg(c, "invalid kind")
		return
	}
	kindTotals, err := model.SumResellerLedgerRange(from, toEx, scope.resellerId, scope.customerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usage, err := model.ComputeInvoiceTotals(period, scope.resellerId, scope.customerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows, _, err := model.ListResellerLedgerRange(from, toEx, scope.resellerId, scope.customerId, kind, 0, -1)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Credit increase/decrease count wallet credits granted to the scope's
	// account holders. A redeemed reseller code credits the customer like
	// customer_credit. Issuing and refunding codes only moves reseller funds
	// into and out of prepaid codes (the reseller side of a transfer), so they
	// are reported separately as redemption escrow.
	ex := ledgerStatement{scope: scope, period: period, rowFilter: kind, kindTotals: kindTotals, usage: usage, rows: rows}
	for _, kt := range kindTotals {
		switch kt.Kind {
		case model.ResellerLedgerRedemptionIssue:
			ex.issued.Entries += kt.Entries
			ex.issued.Quota -= kt.Quota
		case model.ResellerLedgerRedemptionRefund:
			ex.refunded.Entries += kt.Entries
			ex.refunded.Quota += kt.Quota
		case model.ResellerLedgerAdminCredit, model.ResellerLedgerCustomerCredit, model.ResellerLedgerRedemptionRedeem:
			ex.increase.Entries += kt.Entries
			ex.increase.Quota += kt.Quota
			ex.increase.MoneyUsd += kt.MoneyUsd
		case model.ResellerLedgerAdminDebit, model.ResellerLedgerCustomerDebit:
			// Debit rows store a negative quota_delta; report the magnitude.
			ex.decrease.Entries += kt.Entries
			ex.decrease.Quota -= kt.Quota
			ex.decrease.MoneyUsd += kt.MoneyUsd
		}
	}
	if format == "csv" {
		writeLedgerExportCSV(c, &ex)
		return
	}
	writeLedgerExportPDF(c, &ex)
}

// writeLedgerExportCSV writes the export as machine-readable CSV sections.
func writeLedgerExportCSV(c *gin.Context, ex *ledgerStatement) {
	period, usage, scope := ex.period, ex.usage, ex.scope
	increase, decrease, issued, refunded := ex.increase, ex.decrease, ex.issued, ex.refunded
	rowFilter := ex.rowFilter
	if rowFilter == "" {
		rowFilter = "all"
	}

	filename := fmt.Sprintf("ledger-%s-%s.csv", period, usage.Scope)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w := csv.NewWriter(c.Writer)
	summary := [][]string{
		{"# SUMMARY"},
		{"field", "value"},
		{"period", period},
		{"timezone", model.InvoicePeriodLocation().String()},
		{"scope", usage.Scope},
		{"ledger_rows_kind_filter", rowFilter},
		{"note", "credit and usage totals cover the whole period for this scope; the kind filter applies to LEDGER ROWS only"},
		{"credit_increase_entries", strconv.FormatInt(increase.Entries, 10)},
		{"credit_increase_quota", strconv.FormatInt(increase.Quota, 10)},
		{"credit_increase_quota_usd", usdCell(model.QuotaToUsd(increase.Quota))},
		{"credit_increase_money_usd", usdCell(increase.MoneyUsd)},
		{"credit_decrease_entries", strconv.FormatInt(decrease.Entries, 10)},
		{"credit_decrease_quota", strconv.FormatInt(decrease.Quota, 10)},
		{"credit_decrease_quota_usd", usdCell(model.QuotaToUsd(decrease.Quota))},
		{"credit_decrease_money_usd", usdCell(decrease.MoneyUsd)},
		{"credit_net_quota", strconv.FormatInt(increase.Quota-decrease.Quota, 10)},
		{"credit_net_quota_usd", usdCell(model.QuotaToUsd(increase.Quota - decrease.Quota))},
		{"actual_usage_requests", strconv.FormatInt(usage.Requests, 10)},
		{"actual_usage_quota", strconv.FormatInt(usage.ChargeQuota, 10)},
		{"actual_usage_usd", usdCell(usage.ChargeUsd)},
		{"related_fee_charge_quota", strconv.FormatInt(usage.ChargeQuota, 10)},
		{"related_fee_charge_usd", usdCell(usage.ChargeUsd)},
	}
	if scope.internal {
		summary = append(summary,
			[]string{"redemption_issue_entries", strconv.FormatInt(issued.Entries, 10)},
			[]string{"redemption_issue_quota", strconv.FormatInt(issued.Quota, 10)},
			[]string{"redemption_issue_quota_usd", usdCell(model.QuotaToUsd(issued.Quota))},
			[]string{"redemption_refund_entries", strconv.FormatInt(refunded.Entries, 10)},
			[]string{"redemption_refund_quota", strconv.FormatInt(refunded.Quota, 10)},
			[]string{"redemption_refund_quota_usd", usdCell(model.QuotaToUsd(refunded.Quota))},
			[]string{"redemption_escrow_net_quota", strconv.FormatInt(issued.Quota-refunded.Quota, 10)},
			[]string{"related_fee_cost_quota", strconv.FormatInt(usage.CostQuota, 10)},
			[]string{"related_fee_cost_usd", usdCell(usage.CostUsd)},
			[]string{"related_fee_profit_quota", strconv.FormatInt(usage.ProfitQuota, 10)},
			[]string{"related_fee_profit_usd", usdCell(usage.ProfitUsd)},
		)
	}
	_ = w.WriteAll(summary)

	_ = w.Write(nil)
	_ = w.Write([]string{"# LEDGER BY KIND"})
	_ = w.Write([]string{"kind", "entries", "quota_delta", "money_usd"})
	for _, kt := range ex.kindTotals {
		_ = w.Write([]string{csvSafeText(kt.Kind), strconv.FormatInt(kt.Entries, 10), strconv.FormatInt(kt.Quota, 10), usdCell(kt.MoneyUsd)})
	}

	_ = w.Write(nil)
	_ = w.Write([]string{"# LEDGER ROWS"})
	header := []string{"id", "created_at", "created_at_local", "reseller_id", "reseller_username", "customer_id", "customer_username", "kind", "quota_delta", "money_usd"}
	if scope.internal {
		header = append(header, "operator_id", "operator_username", "ref_id", "remark")
	}
	_ = w.Write(header)
	loc := model.InvoicePeriodLocation()
	for _, row := range ex.rows {
		money := ""
		if row.MoneyUsd != nil {
			money = usdCell(*row.MoneyUsd)
		}
		record := []string{
			strconv.FormatInt(row.Id, 10),
			strconv.FormatInt(row.CreatedAt, 10),
			time.Unix(row.CreatedAt, 0).In(loc).Format("2006-01-02 15:04:05"),
			strconv.Itoa(row.ResellerId),
			csvSafeText(row.ResellerUsername),
			strconv.Itoa(row.CustomerId),
			csvSafeText(row.CustomerUsername),
			csvSafeText(row.Kind),
			strconv.FormatInt(row.QuotaDelta, 10),
			money,
		}
		if scope.internal {
			record = append(record,
				strconv.Itoa(row.OperatorId),
				csvSafeText(row.OperatorUsername),
				csvSafeText(row.RefId),
				csvSafeText(row.Remark),
			)
		}
		_ = w.Write(record)
	}
	w.Flush()
}

// AdminExportResellerLedger exports the month for every reseller, or one
// reseller when reseller_id is set.
func AdminExportResellerLedger(c *gin.Context) {
	resellerId, _ := strconv.Atoi(c.Query("reseller_id"))
	writeLedgerExport(c, ledgerExportScope{resellerId: max(resellerId, 0), internal: true})
}

// ResellerExportLedger exports the session reseller's month. Query
// reseller_id / customer_id are ignored.
func ResellerExportLedger(c *gin.Context) {
	writeLedgerExport(c, ledgerExportScope{resellerId: c.GetInt("id"), internal: true})
}

// CustomerExportLedger exports the session customer's month without
// operator/remark columns or reseller cost and profit.
func CustomerExportLedger(c *gin.Context) {
	userId := c.GetInt("id")
	user, err := model.GetUserCache(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user.ResellerId <= 0 {
		common.ApiErrorMsg(c, "forbidden")
		return
	}
	writeLedgerExport(c, ledgerExportScope{customerId: userId})
}

func invoiceDetailResponse(inv *model.Invoice) gin.H {
	return gin.H{
		"invoice":    inv,
		"line_items": inv.InvoiceLineItems(),
	}
}

func listInvoicesHandler(c *gin.Context, f model.InvoiceListFilter) {
	pageInfo := common.GetPageQuery(c)
	f.StartIdx = pageInfo.GetStartIdx()
	f.Num = pageInfo.GetPageSize()
	if period := strings.TrimSpace(c.Query("period")); period != "" {
		f.Period = period
	}
	if scope := strings.TrimSpace(c.Query("scope")); scope != "" {
		f.Scope = scope
	}
	if rid, err := strconv.Atoi(c.Query("reseller_id")); err == nil && rid > 0 {
		f.ResellerId = rid
	}
	if cid, err := strconv.Atoi(c.Query("customer_id")); err == nil && cid > 0 {
		f.CustomerId = cid
	}
	rows, total, err := model.ListInvoices(f)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

// AdminListInvoices lists all persisted monthly invoices.
func AdminListInvoices(c *gin.Context) {
	listInvoicesHandler(c, model.InvoiceListFilter{})
}

// AdminGetInvoice returns one invoice (JSON) or CSV when format=csv.
func AdminGetInvoice(c *gin.Context) {
	id, ok := invoiceIDParam(c)
	if !ok {
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		if errors.Is(err, model.ErrInvoiceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	if c.Query("format") == "csv" {
		writeInvoiceCSV(c, inv)
		return
	}
	common.ApiSuccess(c, invoiceDetailResponse(inv))
}

// AdminGenerateInvoices builds monthly invoices from logs for period YYYY-MM.
func AdminGenerateInvoices(c *gin.Context) {
	var req struct {
		Period string `json:"period"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || strings.TrimSpace(req.Period) == "" {
		common.ApiErrorMsg(c, "period is required (YYYY-MM)")
		return
	}
	result, err := model.GenerateMonthlyInvoices(strings.TrimSpace(req.Period), c.GetInt("id"))
	if err != nil {
		if errors.Is(err, model.ErrInvalidInvoicePeriod) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// ResellerListInvoices lists own reseller invoice + customer invoices.
func ResellerListInvoices(c *gin.Context) {
	listInvoicesHandler(c, model.InvoiceListFilter{RestrictResellerId: c.GetInt("id")})
}

// ResellerGetInvoice returns an invoice belonging to this reseller.
func ResellerGetInvoice(c *gin.Context) {
	id, ok := invoiceIDParam(c)
	if !ok {
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		if errors.Is(err, model.ErrInvoiceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	rid := c.GetInt("id")
	if inv.ResellerId != rid || (inv.Scope != model.InvoiceScopeCustomer && inv.Scope != model.InvoiceScopeReseller) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
		return
	}
	if c.Query("format") == "csv" {
		writeInvoiceCSV(c, inv)
		return
	}
	common.ApiSuccess(c, invoiceDetailResponse(inv))
}

// CustomerListInvoices lists the caller's own customer invoices.
func CustomerListInvoices(c *gin.Context) {
	listInvoicesHandler(c, model.InvoiceListFilter{
		Scope:      model.InvoiceScopeCustomer,
		CustomerId: c.GetInt("id"),
	})
}

// CustomerGetInvoice returns the caller's own customer invoice.
func CustomerGetInvoice(c *gin.Context) {
	id, ok := invoiceIDParam(c)
	if !ok {
		return
	}
	inv, err := model.GetInvoiceById(id)
	if err != nil {
		if errors.Is(err, model.ErrInvoiceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	if inv.Scope != model.InvoiceScopeCustomer || inv.CustomerId != c.GetInt("id") {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "invoice not found"})
		return
	}
	if c.Query("format") == "csv" {
		writeInvoiceCSV(c, inv)
		return
	}
	common.ApiSuccess(c, invoiceDetailResponse(inv))
}
