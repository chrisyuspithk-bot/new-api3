package controller

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
)

// Revenue (earnings) statements: admin scope (per reseller, reseller self-use,
// direct users) and reseller scope (per customer + own usage). JSON for the
// UI, CSV and localized PDF for export; closed months are frozen snapshots.

func AdminRevenueStatement(c *gin.Context) {
	writeRevenueStatement(c, model.RevenueScopeAdmin, 0)
}

// ResellerRevenueStatement is scoped to the session reseller; query params
// naming another reseller are ignored.
func ResellerRevenueStatement(c *gin.Context) {
	writeRevenueStatement(c, model.RevenueScopeReseller, c.GetInt("id"))
}

// AdminRevenueReconcile flushes the retry queue and rebuilds revenue rows
// missing for recent logs (default 2 days, max 62).
func AdminRevenueReconcile(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "2"))
	if days <= 0 || days > 62 {
		days = 2
	}
	pendingLeft := model.FlushPendingRevenue()
	created, err := model.ReconcileRevenueFromLogs(time.Now().Add(-time.Duration(days)*24*time.Hour).Unix(), 20000)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"rebuilt": created, "pending_retry": pendingLeft, "days": days})
}

func writeRevenueStatement(c *gin.Context, scope string, resellerId int) {
	format := strings.TrimSpace(c.Query("format"))
	if format != "" && format != "json" && format != "pdf" && format != "csv" {
		common.ApiErrorMsg(c, "invalid format")
		return
	}
	period := strings.TrimSpace(c.Query("period"))
	if period == "" {
		period = time.Now().In(model.InvoicePeriodLocation()).Format("2006-01")
	}
	st, err := model.GetRevenueStatement(period, scope, resellerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	switch format {
	case "csv":
		writeRevenueCSV(c, st)
	case "pdf":
		writeRevenuePDF(c, st)
	default:
		common.ApiSuccess(c, st)
	}
}

func microToQuotaStr(m int64) string {
	neg := m < 0
	if neg {
		m = -m
	}
	s := fmt.Sprintf("%d.%06d", m/1_000_000, m%1_000_000)
	if neg {
		s = "-" + s
	}
	return s
}

func microToUsd(m int64) float64 {
	if common.QuotaPerUnit <= 0 {
		return 0
	}
	return float64(m) / 1_000_000 / common.QuotaPerUnit
}

func revenueFilename(st *model.RevenueStatement, ext string) string {
	if st.Scope == model.RevenueScopeReseller {
		return fmt.Sprintf("revenue-%s-reseller-%d.%s", st.Period, st.ResellerId, ext)
	}
	return fmt.Sprintf("revenue-%s-admin.%s", st.Period, ext)
}

func writeRevenueCSV(c *gin.Context, st *model.RevenueStatement) {
	internal := st.Scope == model.RevenueScopeAdmin
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", revenueFilename(st, "csv")))
	w := csv.NewWriter(c.Writer)
	q := func(v int64) string { return strconv.FormatInt(v, 10) }
	u := func(v int64) string { return usdCell(model.QuotaToUsd(v)) }
	_ = w.Write([]string{"# SUMMARY"})
	_ = w.Write([]string{"field", "value"})
	meta := [][]string{
		{"period", st.Period}, {"timezone", st.Timezone}, {"scope", st.Scope},
		{"frozen", strconv.FormatBool(st.Frozen)}, {"frozen_at", q(st.FrozenAt)},
		{"row_count", q(st.RowCount)}, {"checksum_sha256", st.Checksum},
		{"late_rows", q(st.LateRows)}, {"checksum_mismatch", strconv.FormatBool(st.ChecksumMismatch)},
	}
	if !internal {
		meta = append(meta, []string{"reseller_id", strconv.Itoa(st.ResellerId)}, []string{"reseller", csvSafeText(st.ResellerName)})
	}
	sums := []struct {
		name string
		a    model.RevenueAgg
	}{{"total", st.Totals}, {"customer", st.CustomerSum}, {"reseller_self", st.SelfSum}, {"direct_credit", st.DirectCreditSum}}
	if internal {
		sums = append(sums, struct {
			name string
			a    model.RevenueAgg
		}{"direct", st.DirectSum})
	}
	for _, s := range sums {
		meta = append(meta,
			[]string{s.name + "_requests", q(s.a.Requests)},
			[]string{s.name + "_refunds", q(s.a.Refunds)},
			[]string{s.name + "_customer_price_quota", q(s.a.CustomerPrice)},
			[]string{s.name + "_customer_price_usd", u(s.a.CustomerPrice)},
			[]string{s.name + "_reseller_cost_quota", q(s.a.ResellerCost)},
			[]string{s.name + "_reseller_cost_usd", u(s.a.ResellerCost)},
			[]string{s.name + "_reseller_profit_quota", q(s.a.ResellerProfit)},
			[]string{s.name + "_reseller_profit_usd", u(s.a.ResellerProfit)},
		)
		if internal {
			meta = append(meta,
				[]string{s.name + "_admin_revenue_quota", q(s.a.AdminRevenue)},
				[]string{s.name + "_admin_revenue_usd", u(s.a.AdminRevenue)},
				[]string{s.name + "_base_quota", microToQuotaStr(s.a.BaseQuotaMicro)},
				[]string{s.name + "_base_usd", usdCell(microToUsd(s.a.BaseQuotaMicro))},
				[]string{s.name + "_admin_profit_quota", microToQuotaStr(s.a.AdminProfitMicro)},
				[]string{s.name + "_admin_profit_usd", usdCell(microToUsd(s.a.AdminProfitMicro))},
			)
		}
	}
	r := st.Reconciliation
	meta = append(meta,
		[]string{"check_sections_ok", strconv.FormatBool(r.SectionsOK)},
		[]string{"check_log_compared", strconv.FormatBool(r.LogCompared)},
		[]string{"check_log_charge_diff_quota", q(r.LogChargeDiff)},
		[]string{"check_log_reseller_cost_diff_quota", q(r.LogResellerCostDif)},
		[]string{"check_flagged_rows", q(r.FlaggedRows)},
		[]string{"check_pending_retry", strconv.Itoa(r.PendingRetry)},
	)
	if internal {
		meta = append(meta, []string{"check_identity_ok", strconv.FormatBool(r.IdentityOK)}, []string{"check_identity_diff_micro", q(r.IdentityDiffMicro)})
	}
	_ = w.WriteAll(meta)

	header := []string{"section", "id", "name", "requests", "refunds", "customer_price_quota", "reseller_cost_quota", "reseller_profit_quota"}
	if internal {
		header = append(header, "admin_revenue_quota", "base_quota", "admin_profit_quota")
	}
	writeLines := func(section string, lines []model.RevenueAgg) {
		for _, a := range lines {
			rec := []string{section, strconv.Itoa(a.Id), csvSafeText(a.Name), q(a.Requests), q(a.Refunds), q(a.CustomerPrice), q(a.ResellerCost), q(a.ResellerProfit)}
			if internal {
				rec = append(rec, q(a.AdminRevenue), microToQuotaStr(a.BaseQuotaMicro), microToQuotaStr(a.AdminProfitMicro))
			}
			_ = w.Write(rec)
		}
	}
	_ = w.Write(nil)
	_ = w.Write([]string{"# LINES"})
	_ = w.Write(header)
	if internal {
		writeLines("by_reseller", st.ByReseller)
		writeLines("reseller_self", st.ResellerSelf)
		writeLines("direct", st.Direct)
	} else {
		writeLines("by_customer", st.ByCustomer)
		own := st.SelfSum
		own.Id, own.Name = st.ResellerId, st.ResellerName
		writeLines("own_usage", []model.RevenueAgg{own})
	}

	// Per-call detail rows (live rows; for frozen months see checksum/late_rows).
	rows, err := model.RevenueRowsForExport(st.Period, st.Scope, st.ResellerId)
	if err == nil {
		_ = w.Write(nil)
		_ = w.Write([]string{"# RECORDS"})
		rh := []string{"id", "created_at", "created_at_local", "kind", "payer_type", "user_id", "reseller_id", "model", "request_id", "markup", "customer_price_quota", "reseller_cost_quota", "reseller_profit_quota"}
		if internal {
			rh = append(rh, "cost_factor", "base_quota", "admin_revenue_quota", "admin_profit_quota", "log_id", "source", "flags")
		}
		_ = w.Write(rh)
		loc := model.InvoicePeriodLocation()
		for _, x := range rows {
			rec := []string{q(x.Id), q(x.CreatedAt), time.Unix(x.CreatedAt, 0).In(loc).Format("2006-01-02 15:04:05"), x.Kind, x.PayerType,
				strconv.Itoa(x.UserId), strconv.Itoa(x.ResellerId), csvSafeText(x.ModelName), csvSafeText(x.RequestId),
				strconv.FormatFloat(x.Markup, 'f', -1, 64), q(x.CustomerPrice), q(x.ResellerCost), q(x.ResellerProfit)}
			if internal {
				rec = append(rec, strconv.FormatFloat(x.CostFactor, 'f', -1, 64), microToQuotaStr(x.BaseQuotaMicro), q(x.AdminRevenue),
					microToQuotaStr(x.AdminProfitMicro), q(x.LogId), x.Source, x.Flags)
			}
			_ = w.Write(rec)
		}
	}
	w.Flush()
}

func writeRevenuePDF(c *gin.Context, st *model.RevenueStatement) {
	internal := st.Scope == model.RevenueScopeAdmin
	lang := i18n.StatedLang(c)
	if lang == "" {
		lang = i18n.ParseAcceptLanguage(c.GetHeader("Accept-Language"))
	}
	if lang == "" {
		lang = i18n.ParseAcceptLanguage(common.DefaultLanguage)
	}
	tr := func(key string) string { return i18n.Translate(lang, key) }
	fontDir, fontFile, err := ledgerPDFFont(lang)
	if err != nil {
		common.SysError(err.Error())
		common.ApiError(c, err)
		return
	}
	title := tr(i18n.MsgRevenueExportTitleAdmin)
	if !internal {
		title = tr(i18n.MsgRevenueExportTitleReseller)
	}
	pdf := fpdf.New("L", "mm", "A4", fontDir)
	pdf.SetTitle(title+" "+st.Period, true)
	pdf.SetCreator("new-api", true)
	pdf.AddUTF8Font("noto", "", fontFile)
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("noto", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 5, fmt.Sprintf("%s · %s    %d / {nb}", title, st.Period, pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()
	pdf.SetFont("noto", "", 18)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 10, title, "", 1, "L", false, 0, "")

	loc := model.InvoicePeriodLocation()
	ts := func(v int64) string {
		if v <= 0 {
			return "-"
		}
		return time.Unix(v, 0).In(loc).Format("2006-01-02 15:04:05")
	}
	status := tr(i18n.MsgRevenueExportStatusOpen)
	if st.Frozen {
		status = tr(i18n.MsgRevenueExportStatusFrozen)
	}
	meta := [][2]string{
		{tr(i18n.MsgRevenueExportPeriod), st.Period},
		{tr(i18n.MsgRevenueExportTimezone), st.Timezone},
	}
	if !internal {
		meta = append(meta, [2]string{tr(i18n.MsgRevenueExportReseller), fmt.Sprintf("%s (#%d)", st.ResellerName, st.ResellerId)})
	}
	meta = append(meta,
		[2]string{tr(i18n.MsgRevenueExportStatus), status},
		[2]string{tr(i18n.MsgRevenueExportFrozenAt), ts(st.FrozenAt)},
		[2]string{tr(i18n.MsgRevenueExportGeneratedAt), ts(st.GeneratedAt)},
		[2]string{tr(i18n.MsgRevenueExportRows), strconv.FormatInt(st.RowCount, 10)},
		[2]string{tr(i18n.MsgRevenueExportChecksum), st.Checksum},
	)
	pdf.SetFont("noto", "", 10)
	for _, line := range meta {
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(42, 6, line[0], "", 0, "L", false, 0, "")
		pdf.SetTextColor(20, 20, 20)
		pdf.CellFormat(0, 6, line[1], "", 1, "L", false, 0, "")
	}
	pdf.SetFont("noto", "", 8.5)
	pdf.SetTextColor(100, 100, 100)
	note := tr(i18n.MsgRevenueExportNoteAdmin)
	if !internal {
		note = tr(i18n.MsgRevenueExportNoteReseller)
	}
	pdf.MultiCell(0, 5, note, "", "L", false)

	qs := func(v int64) string { return strconv.FormatInt(v, 10) }
	us := func(v int64) string { return usdCell(model.QuotaToUsd(v)) }
	mq := func(v int64) string { return microToQuotaStr(v) }
	mu := func(v int64) string { return usdCell(microToUsd(v)) }

	ledgerPDFSection(pdf, tr(i18n.MsgRevenueExportSectionSummary))
	var summary [][]string
	if internal {
		summary = [][]string{
			{tr(i18n.MsgRevenueExportItemTotalBilled), qs(st.Totals.Requests), qs(st.Totals.CustomerPrice), us(st.Totals.CustomerPrice)},
			{tr(i18n.MsgRevenueExportItemCustomerSales), qs(st.CustomerSum.Requests), qs(st.CustomerSum.CustomerPrice), us(st.CustomerSum.CustomerPrice)},
			{tr(i18n.MsgRevenueExportItemResellerCost), "", qs(st.CustomerSum.ResellerCost), us(st.CustomerSum.ResellerCost)},
			{tr(i18n.MsgRevenueExportItemResellerProfit), "", qs(st.CustomerSum.ResellerProfit), us(st.CustomerSum.ResellerProfit)},
			{tr(i18n.MsgRevenueExportItemSelfUse), qs(st.SelfSum.Requests), qs(st.SelfSum.CustomerPrice), us(st.SelfSum.CustomerPrice)},
			{tr(i18n.MsgRevenueExportItemDirect), qs(st.DirectSum.Requests), qs(st.DirectSum.CustomerPrice), us(st.DirectSum.CustomerPrice)},
			{tr(i18n.MsgRevenueRecordsKindDCTopup), "", qs(st.DirectCreditSum.CustomerPrice), us(st.DirectCreditSum.CustomerPrice)},
			{tr(i18n.MsgLedgerExportKindDirectCreditCost), "", qs(st.DirectCreditSum.ResellerCost), us(st.DirectCreditSum.ResellerCost)},
			{tr(i18n.MsgRevenueExportItemAdminRevenue), "", qs(st.Totals.AdminRevenue), us(st.Totals.AdminRevenue)},
			{tr(i18n.MsgRevenueExportItemBase), "", mq(st.Totals.BaseQuotaMicro), mu(st.Totals.BaseQuotaMicro)},
			{tr(i18n.MsgRevenueExportItemAdminProfit), "", mq(st.Totals.AdminProfitMicro), mu(st.Totals.AdminProfitMicro)},
		}
	} else {
		summary = [][]string{
			{tr(i18n.MsgRevenueExportItemYourSales), qs(st.CustomerSum.Requests), qs(st.CustomerSum.CustomerPrice), us(st.CustomerSum.CustomerPrice)},
			{tr(i18n.MsgRevenueExportItemYourCost), "", qs(st.CustomerSum.ResellerCost), us(st.CustomerSum.ResellerCost)},
			{tr(i18n.MsgRevenueExportItemYourProfit), "", qs(st.CustomerSum.ResellerProfit), us(st.CustomerSum.ResellerProfit)},
			{tr(i18n.MsgRevenueExportItemOwnUsage), qs(st.SelfSum.Requests), qs(st.SelfSum.ResellerCost), us(st.SelfSum.ResellerCost)},
			{tr(i18n.MsgRevenueRecordsKindDCTopup), "", qs(st.DirectCreditSum.CustomerPrice), us(st.DirectCreditSum.CustomerPrice)},
			{tr(i18n.MsgLedgerExportKindDirectCreditCost), "", qs(st.DirectCreditSum.ResellerCost), us(st.DirectCreditSum.ResellerCost)},
			{tr(i18n.MsgRevenueExportItemTotalPaid), "", qs(st.CustomerSum.ResellerCost + st.SelfSum.ResellerCost + st.DirectCreditSum.ResellerCost), us(st.CustomerSum.ResellerCost + st.SelfSum.ResellerCost + st.DirectCreditSum.ResellerCost)},
		}
	}
	ledgerPDFTable(pdf, []float64{140, 30, 50, 53}, "LRRR",
		[]string{tr(i18n.MsgRevenueExportColItem), tr(i18n.MsgRevenueExportColRequests), tr(i18n.MsgRevenueExportColQuota), tr(i18n.MsgRevenueExportColUsd)}, summary)

	lineTable := func(heading string, lines []model.RevenueAgg) {
		ledgerPDFSection(pdf, heading)
		var header []string
		var widths []float64
		var aligns string
		if internal {
			header = []string{tr(i18n.MsgRevenueExportColName), tr(i18n.MsgRevenueExportColRequests), tr(i18n.MsgRevenueExportColRefunds),
				tr(i18n.MsgRevenueExportColCustomerPrice) + " USD", tr(i18n.MsgRevenueExportColResellerCost) + " USD", tr(i18n.MsgRevenueExportColResellerProfit) + " USD",
				tr(i18n.MsgRevenueExportColBase) + " USD", tr(i18n.MsgRevenueExportColAdminProfit) + " USD"}
			widths = []float64{53, 20, 20, 36, 36, 36, 36, 36}
			aligns = "LRRRRRRR"
		} else {
			header = []string{tr(i18n.MsgRevenueExportColName), tr(i18n.MsgRevenueExportColRequests), tr(i18n.MsgRevenueExportColRefunds),
				tr(i18n.MsgRevenueExportColCustomerPrice) + " USD", tr(i18n.MsgRevenueExportColYourCost) + " USD", tr(i18n.MsgRevenueExportColYourProfit) + " USD"}
			widths = []float64{73, 25, 25, 50, 50, 50}
			aligns = "LRRRRR"
		}
		rows := make([][]string, 0, len(lines))
		for _, a := range lines {
			name := a.Name
			if name == "" {
				name = "#" + strconv.Itoa(a.Id)
			}
			row := []string{name, qs(a.Requests), qs(a.Refunds), us(a.CustomerPrice), us(a.ResellerCost), us(a.ResellerProfit)}
			if internal {
				row = append(row, mu(a.BaseQuotaMicro), mu(a.AdminProfitMicro))
			}
			rows = append(rows, row)
		}
		ledgerPDFTable(pdf, widths, aligns, header, rows)
		if len(rows) == 0 {
			pdf.SetFont("noto", "", 9)
			pdf.SetTextColor(120, 120, 120)
			pdf.CellFormat(0, 7, tr(i18n.MsgRevenueExportNoRows), "", 1, "L", false, 0, "")
		}
	}
	if internal {
		lineTable(tr(i18n.MsgRevenueExportSectionByReseller), st.ByReseller)
		lineTable(tr(i18n.MsgRevenueExportSectionResellerSelf), st.ResellerSelf)
		lineTable(tr(i18n.MsgRevenueExportSectionDirect), st.Direct)
	} else {
		lineTable(tr(i18n.MsgRevenueExportSectionByCustomer), st.ByCustomer)
	}

	ledgerPDFSection(pdf, tr(i18n.MsgRevenueExportSectionReconciliation))
	okStr := func(ok bool) string {
		if ok {
			return tr(i18n.MsgRevenueExportOk)
		}
		return tr(i18n.MsgRevenueExportMismatch)
	}
	r := st.Reconciliation
	var checks [][]string
	if internal {
		checks = append(checks, []string{tr(i18n.MsgRevenueExportCheckIdentity), mq(r.IdentityDiffMicro), okStr(r.IdentityOK)})
	}
	checks = append(checks, []string{tr(i18n.MsgRevenueExportCheckSections), "", okStr(r.SectionsOK)})
	if r.LogCompared {
		checks = append(checks,
			[]string{tr(i18n.MsgRevenueExportCheckLogCharge), qs(r.LogChargeDiff), okStr(r.LogChargeDiff == 0)},
			[]string{tr(i18n.MsgRevenueExportCheckLogCost), qs(r.LogResellerCostDif), okStr(r.LogResellerCostDif == 0)})
	} else {
		checks = append(checks, []string{tr(i18n.MsgRevenueExportCheckLogCharge), "", tr(i18n.MsgRevenueExportNotChecked)})
	}
	checks = append(checks,
		[]string{tr(i18n.MsgRevenueExportCheckFlagged), qs(r.FlaggedRows), okStr(r.FlaggedRows == 0)},
		[]string{tr(i18n.MsgRevenueExportCheckLateRows), qs(st.LateRows), okStr(st.LateRows == 0 && !st.ChecksumMismatch)},
		[]string{tr(i18n.MsgRevenueExportCheckPending), strconv.Itoa(r.PendingRetry), okStr(r.PendingRetry == 0)},
	)
	ledgerPDFTable(pdf, []float64{150, 70, 53}, "LRL",
		[]string{tr(i18n.MsgRevenueExportColCheck), tr(i18n.MsgRevenueExportColValue), tr(i18n.MsgRevenueExportColResult)}, checks)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", revenueFilename(st, "pdf")))
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}
