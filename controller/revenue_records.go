package controller

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
)

// AdminRevenueRecords lists every revenue row (real-time profit records) with
// optional reseller / customer / date filters. format=csv|pdf exports.
func AdminRevenueRecords(c *gin.Context) {
	f := revenueRecordFilterFromQuery(c)
	f.Scope = model.RevenueScopeAdmin
	f.ResellerId, _ = strconv.Atoi(c.Query("reseller_id"))
	writeRevenueRecords(c, f)
}

// ResellerRevenueRecords is scoped to the session reseller (its customers'
// calls and its own usage); admin-only fields are stripped.
func ResellerRevenueRecords(c *gin.Context) {
	f := revenueRecordFilterFromQuery(c)
	f.Scope = model.RevenueScopeReseller
	f.ResellerId = c.GetInt("id")
	writeRevenueRecords(c, f)
}

func revenueRecordFilterFromQuery(c *gin.Context) model.RevenueRecordFilter {
	f := model.RevenueRecordFilter{}
	f.From, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	f.To, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	f.CustomerId, _ = strconv.Atoi(c.Query("customer_id"))
	f.Page, _ = strconv.Atoi(c.DefaultQuery("p", "1"))
	f.PageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 {
		f.PageSize = 20
	}
	if f.PageSize > 100 {
		f.PageSize = 100
	}
	return f
}

// revenueRecordOut is the API shape; admin-only fields are omitted for resellers.
type revenueRecordOut struct {
	Id               int64    `json:"id"`
	CreatedAt        int64    `json:"created_at"`
	Kind             string   `json:"kind"`
	PayerType        string   `json:"payer_type"`
	UserId           int      `json:"user_id"`
	Username         string   `json:"username"`
	ResellerId       int      `json:"reseller_id"`
	ResellerUsername string   `json:"reseller_username"`
	ModelName        string   `json:"model_name"`
	RequestId        string   `json:"request_id"`
	CustomerPrice    int64    `json:"customer_price"`
	ResellerCost     int64    `json:"reseller_cost"`
	ResellerProfit   int64    `json:"reseller_profit"`
	ResellerCredited int64    `json:"reseller_credited"`
	CreditState      string   `json:"credit_state"`
	AdminProfit      *float64 `json:"admin_profit,omitempty"` // quota (micro precision)
	AdminCredited    *int64   `json:"admin_credited,omitempty"`
	CostFactor       *float64 `json:"cost_factor,omitempty"`
	Markup           float64  `json:"markup"`
	Flags            string   `json:"flags,omitempty"`
}

func toRevenueRecordOut(r model.RevenueRecordView, internal bool) revenueRecordOut {
	o := revenueRecordOut{
		Id: r.Id, CreatedAt: r.CreatedAt, Kind: r.Kind, PayerType: r.PayerType,
		UserId: r.UserId, Username: r.Username, ResellerId: r.ResellerId, ResellerUsername: r.ResellerUsername,
		ModelName: r.ModelName, RequestId: r.RequestId, CustomerPrice: r.CustomerPrice, ResellerCost: r.ResellerCost,
		ResellerProfit: r.ResellerProfit, ResellerCredited: r.ResellerCredited, CreditState: r.CreditState,
		Markup: r.Markup, Flags: r.Flags,
	}
	if internal {
		ap := float64(r.AdminProfitMicro) / 1e6
		ac := r.AdminCredited
		cf := r.CostFactor
		o.AdminProfit, o.AdminCredited, o.CostFactor = &ap, &ac, &cf
	}
	return o
}

func writeRevenueRecords(c *gin.Context, f model.RevenueRecordFilter) {
	internal := f.Scope == model.RevenueScopeAdmin
	format := strings.TrimSpace(c.Query("format"))
	switch format {
	case "csv", "pdf":
		rows, totals, truncated, err := model.RevenueRecordsForExport(f)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if format == "csv" {
			writeRevenueRecordsCSV(c, f, rows, totals, truncated)
		} else {
			writeRevenueRecordsPDF(c, f, rows, totals, truncated)
		}
		return
	case "", "json":
	default:
		common.ApiErrorMsg(c, "invalid format")
		return
	}
	rows, totals, err := model.ListRevenueRecords(f)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]revenueRecordOut, 0, len(rows))
	for _, r := range rows {
		items = append(items, toRevenueRecordOut(r, internal))
	}
	out := gin.H{"items": items, "total": totals.Rows, "page": f.Page, "page_size": f.PageSize}
	t := gin.H{"rows": totals.Rows, "customer_price": totals.CustomerPrice, "reseller_cost": totals.ResellerCost,
		"reseller_profit": totals.ResellerProfit, "reseller_credited": totals.ResellerCredited}
	if internal {
		t["admin_profit"] = float64(totals.AdminProfitMicro) / 1e6
		t["admin_credited"] = totals.AdminCredited
		if acc, err := model.ProfitAccumulatorTotals(); err == nil {
			out["admin_accumulator"] = gin.H{"user_id": acc.UserId, "accrued_micro": acc.AccruedMicro,
				"credited_quota": acc.CreditedQuota, "remainder_micro": acc.RemainderMicro()}
		}
	}
	out["totals"] = t
	common.ApiSuccess(c, out)
}

func revenueRecordsLang(c *gin.Context) string {
	lang := i18n.StatedLang(c)
	if lang == "" {
		lang = i18n.ParseAcceptLanguage(c.GetHeader("Accept-Language"))
	}
	if lang == "" {
		lang = i18n.ParseAcceptLanguage(common.DefaultLanguage)
	}
	return lang
}

func revenueRecordsFilename(f model.RevenueRecordFilter, ext string) string {
	scope := "admin"
	if f.Scope == model.RevenueScopeReseller {
		scope = "reseller"
	}
	return fmt.Sprintf("profit-records-%s-%s.%s", scope, time.Now().In(model.InvoicePeriodLocation()).Format("20060102-150405"), ext)
}

func revenueKindLabel(tr func(string) string, kind, payer string) string {
	k := tr(i18n.MsgRevenueRecordsKindConsume)
	if kind == model.RevenueKindRefund {
		k = tr(i18n.MsgRevenueRecordsKindRefund)
	}
	p := tr(i18n.MsgRevenueRecordsPayerDirect)
	switch payer {
	case model.RevenuePayerCustomer:
		p = tr(i18n.MsgRevenueRecordsPayerCustomer)
	case model.RevenuePayerResellerSelf:
		p = tr(i18n.MsgRevenueRecordsPayerSelf)
	}
	return k + " · " + p
}

func revenueRecordsHeader(tr func(string) string, internal bool) []string {
	h := []string{tr(i18n.MsgRevenueRecordsColTime), tr(i18n.MsgRevenueRecordsColUser)}
	if internal {
		h = append(h, tr(i18n.MsgRevenueRecordsColReseller))
	}
	h = append(h, tr(i18n.MsgRevenueRecordsColModel), tr(i18n.MsgRevenueRecordsColType),
		tr(i18n.MsgRevenueRecordsColPrice), tr(i18n.MsgRevenueRecordsColCost), tr(i18n.MsgRevenueRecordsColResellerProfit))
	if internal {
		h = append(h, tr(i18n.MsgRevenueRecordsColAdminProfit))
	}
	return h
}

func revenueRecordCells(r model.RevenueRecordView, internal bool, tr func(string) string, ts func(int64) string) []string {
	q := func(v int64) string { return strconv.FormatInt(v, 10) }
	cells := []string{ts(r.CreatedAt), fmt.Sprintf("%s (#%d)", r.Username, r.UserId)}
	if internal {
		rs := "-"
		if r.ResellerId > 0 {
			rs = fmt.Sprintf("%s (#%d)", r.ResellerUsername, r.ResellerId)
		}
		cells = append(cells, rs)
	}
	cells = append(cells, r.ModelName, revenueKindLabel(tr, r.Kind, r.PayerType), q(r.CustomerPrice), q(r.ResellerCost), q(r.ResellerProfit))
	if internal {
		cells = append(cells, microToQuotaStr(r.AdminProfitMicro))
	}
	return cells
}

func writeRevenueRecordsCSV(c *gin.Context, f model.RevenueRecordFilter, rows []model.RevenueRecordView, totals model.RevenueRecordTotals, truncated bool) {
	internal := f.Scope == model.RevenueScopeAdmin
	lang := revenueRecordsLang(c)
	tr := func(key string) string { return i18n.Translate(lang, key) }
	loc := model.InvoicePeriodLocation()
	ts := func(v int64) string { return time.Unix(v, 0).In(loc).Format("2006-01-02 15:04:05") }
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", revenueRecordsFilename(f, "csv")))
	_, _ = c.Writer.Write([]byte("\xEF\xBB\xBF")) // BOM so spreadsheet apps read UTF-8 (CJK)
	w := csv.NewWriter(c.Writer)
	header := append([]string{"id"}, revenueRecordsHeader(tr, internal)...)
	header = append(header, tr(i18n.MsgRevenueRecordsColCredited), "request_id")
	_ = w.Write(header)
	for _, r := range rows {
		cells := revenueRecordCells(r, internal, tr, ts)
		for i := range cells {
			cells[i] = csvSafeText(cells[i])
		}
		credited := strconv.FormatInt(r.ResellerCredited, 10)
		if internal {
			credited += " / " + strconv.FormatInt(r.AdminCredited, 10)
		}
		row := append([]string{strconv.FormatInt(r.Id, 10)}, cells...)
		row = append(row, credited, csvSafeText(r.RequestId))
		_ = w.Write(row)
	}
	tot := []string{tr(i18n.MsgRevenueRecordsTotal), strconv.FormatInt(totals.Rows, 10), ""}
	if internal {
		tot = append(tot, "")
	}
	tot = append(tot, "", "", strconv.FormatInt(totals.CustomerPrice, 10), strconv.FormatInt(totals.ResellerCost, 10), strconv.FormatInt(totals.ResellerProfit, 10))
	if internal {
		tot = append(tot, microToQuotaStr(totals.AdminProfitMicro))
	}
	_ = w.Write(tot)
	if truncated {
		_ = w.Write([]string{"# " + tr(i18n.MsgRevenueRecordsTruncated)})
	}
	w.Flush()
}

func writeRevenueRecordsPDF(c *gin.Context, f model.RevenueRecordFilter, rows []model.RevenueRecordView, totals model.RevenueRecordTotals, truncated bool) {
	internal := f.Scope == model.RevenueScopeAdmin
	lang := revenueRecordsLang(c)
	tr := func(key string) string { return i18n.Translate(lang, key) }
	fontDir, fontFile, err := ledgerPDFFont(lang)
	if err != nil {
		common.SysError(err.Error())
		common.ApiError(c, err)
		return
	}
	loc := model.InvoicePeriodLocation()
	ts := func(v int64) string { return time.Unix(v, 0).In(loc).Format("2006-01-02 15:04:05") }
	title := tr(i18n.MsgRevenueRecordsTitleAdmin)
	if !internal {
		title = tr(i18n.MsgRevenueRecordsTitleReseller)
	}
	pdf := fpdf.New("L", "mm", "A4", fontDir)
	pdf.SetTitle(title, true)
	pdf.SetCreator("new-api", true)
	pdf.AddUTF8Font("noto", "", fontFile)
	pdf.SetMargins(10, 12, 10)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("noto", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 5, fmt.Sprintf("%s    %d / {nb}", title, pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()
	pdf.SetFont("noto", "", 18)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 10, title, "", 1, "L", false, 0, "")
	rng := func(v int64, open string) string {
		if v <= 0 {
			return open
		}
		return ts(v)
	}
	meta := [][2]string{
		{tr(i18n.MsgRevenueRecordsRange), rng(f.From, "…") + " — " + rng(f.To, "…")},
		{tr(i18n.MsgRevenueExportTimezone), loc.String()},
		{tr(i18n.MsgRevenueExportGeneratedAt), ts(time.Now().Unix())},
		{tr(i18n.MsgRevenueExportRows), strconv.FormatInt(totals.Rows, 10)},
	}
	var filters []string
	if internal && f.ResellerId > 0 {
		filters = append(filters, fmt.Sprintf("%s #%d", tr(i18n.MsgRevenueRecordsColReseller), f.ResellerId))
	}
	if f.CustomerId > 0 {
		filters = append(filters, fmt.Sprintf("%s #%d", tr(i18n.MsgRevenueRecordsColUser), f.CustomerId))
	}
	if len(filters) > 0 {
		meta = append(meta, [2]string{tr(i18n.MsgRevenueRecordsFilters), strings.Join(filters, ", ")})
	}
	pdf.SetFont("noto", "", 10)
	for _, line := range meta {
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(42, 6, line[0], "", 0, "L", false, 0, "")
		pdf.SetTextColor(20, 20, 20)
		pdf.CellFormat(0, 6, line[1], "", 1, "L", false, 0, "")
	}
	if truncated {
		pdf.SetTextColor(180, 60, 60)
		pdf.MultiCell(0, 5, tr(i18n.MsgRevenueRecordsTruncated), "", "L", false)
	}
	var widths []float64
	var aligns string
	if internal {
		widths = []float64{32, 34, 34, 40, 34, 22, 22, 22, 37}
		aligns = "LLLLLRRRR"
	} else {
		widths = []float64{36, 46, 52, 44, 33, 33, 33}
		aligns = "LLLLRRR"
	}
	var body [][]string
	for _, r := range rows {
		body = append(body, revenueRecordCells(r, internal, tr, ts))
	}
	tot := []string{tr(i18n.MsgRevenueRecordsTotal), strconv.FormatInt(totals.Rows, 10)}
	if internal {
		tot = append(tot, "")
	}
	tot = append(tot, "", "", strconv.FormatInt(totals.CustomerPrice, 10), strconv.FormatInt(totals.ResellerCost, 10), strconv.FormatInt(totals.ResellerProfit, 10))
	if internal {
		tot = append(tot, microToQuotaStr(totals.AdminProfitMicro))
	}
	body = append(body, tot)
	ledgerPDFSection(pdf, title)
	ledgerPDFTable(pdf, widths, aligns, revenueRecordsHeader(tr, internal), body)
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", revenueRecordsFilename(f, "pdf")))
	if err := pdf.Output(c.Writer); err != nil {
		common.SysError("revenue records pdf: " + err.Error())
	}
}
