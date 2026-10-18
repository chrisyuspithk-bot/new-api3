package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
)

// defaultPDFFontDir is where the Docker image installs the Noto Sans TC/SC
// TrueType fonts. PDF_FONT_DIR overrides it; it must hold NotoSansTC.ttf
// and/or NotoSansSC.ttf.
const defaultPDFFontDir = "/usr/share/fonts/truetype/noto"

// ledgerExportLabelKeys maps ledger kinds and invoice scopes to their labels.
var ledgerExportLabelKeys = map[string]string{
	model.ResellerLedgerAdminCredit:          i18n.MsgLedgerExportKindAdminCredit,
	model.ResellerLedgerAdminDebit:           i18n.MsgLedgerExportKindAdminDebit,
	model.ResellerLedgerCustomerCredit:       i18n.MsgLedgerExportKindCustomerCredit,
	model.ResellerLedgerCustomerDebit:        i18n.MsgLedgerExportKindCustomerDebit,
	model.ResellerLedgerRedemptionIssue:      i18n.MsgLedgerExportKindRedemptionIssue,
	model.ResellerLedgerRedemptionRefund:     i18n.MsgLedgerExportKindRedemptionRefund,
	model.ResellerLedgerRedemptionRedeem:     i18n.MsgLedgerExportKindRedemptionRedeem,
	model.ResellerLedgerResellerProfitCredit: i18n.MsgLedgerExportKindResellerProfitCredit,
	model.ResellerLedgerAdminProfitCredit:    i18n.MsgLedgerExportKindAdminProfitCredit,
	model.InvoiceScopeCustomer:               i18n.MsgLedgerExportScopeCustomer,
	model.InvoiceScopeReseller:               i18n.MsgLedgerExportScopeReseller,
	model.InvoiceScopeAdminRollup:            i18n.MsgLedgerExportScopeAdminRollup,
}

// ledgerPDFFont finds the CJK TrueType font for a statement in lang:
// Simplified Chinese prefers Noto Sans SC, every other language Noto Sans TC.
// Either font covers the Latin text and most names in the other script.
func ledgerPDFFont(lang string) (dir, file string, err error) {
	dir = strings.TrimSpace(os.Getenv("PDF_FONT_DIR"))
	if dir == "" {
		dir = defaultPDFFontDir
	}
	candidates := []string{"NotoSansTC.ttf", "NotoSansSC.ttf"}
	if lang == i18n.LangZhCN {
		candidates = []string{"NotoSansSC.ttf", "NotoSansTC.ttf"}
	}
	for _, name := range candidates {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil {
			return dir, name, nil
		}
	}
	return "", "", fmt.Errorf("ledger PDF font not found: put NotoSansTC.ttf or NotoSansSC.ttf in %s (PDF_FONT_DIR), or export with format=csv", dir)
}

// writeLedgerExportPDF writes the export as a statement in the reader's
// language, with the same numbers as the CSV. Customer statements omit
// operator, reference, remark, redemption escrow and cost/profit.
func writeLedgerExportPDF(c *gin.Context, ex *ledgerStatement) {
	lang := i18n.StatedLang(c)
	if lang == "" {
		lang = i18n.ParseAcceptLanguage(common.DefaultLanguage)
	}
	tr := func(key string) string { return i18n.Translate(lang, key) }
	label := func(kind string) string {
		if key, ok := ledgerExportLabelKeys[kind]; ok {
			return tr(key)
		}
		return kind
	}
	fontDir, fontFile, err := ledgerPDFFont(lang)
	if err != nil {
		common.SysError(err.Error())
		common.ApiError(c, err)
		return
	}

	usage, scope := ex.usage, ex.scope
	title := tr(i18n.MsgLedgerExportTitle)
	pdf := fpdf.New("L", "mm", "A4", fontDir)
	pdf.SetTitle(title+" "+ex.period, true)
	pdf.SetCreator("new-api", true)
	pdf.AddUTF8Font("noto", "", fontFile)
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 14)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-10)
		pdf.SetFont("noto", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 5, fmt.Sprintf("%s · %s    %d / {nb}", title, ex.period, pdf.PageNo()), "", 0, "R", false, 0, "")
	})
	pdf.AddPage()

	pdf.SetFont("noto", "", 18)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 10, title, "", 1, "L", false, 0, "")
	kindFilter := tr(i18n.MsgLedgerExportKindFilterAll)
	if ex.rowFilter != "" {
		kindFilter = label(ex.rowFilter)
	}
	pdf.SetFont("noto", "", 10)
	for _, line := range [][2]string{
		{tr(i18n.MsgLedgerExportPeriod), ex.period},
		{tr(i18n.MsgLedgerExportTimezone), model.InvoicePeriodLocation().String()},
		{tr(i18n.MsgLedgerExportScope), label(usage.Scope)},
		{tr(i18n.MsgLedgerExportKindFilter), kindFilter},
	} {
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(32, 6, line[0], "", 0, "L", false, 0, "")
		pdf.SetTextColor(20, 20, 20)
		pdf.CellFormat(0, 6, line[1], "", 1, "L", false, 0, "")
	}
	pdf.SetFont("noto", "", 8.5)
	pdf.SetTextColor(100, 100, 100)
	pdf.MultiCell(0, 5, tr(i18n.MsgLedgerExportNote), "", "L", false)

	quota := func(q int64) string { return strconv.FormatInt(q, 10) }
	count := func(n int64) string { return strconv.FormatInt(n, 10) }
	quotaUsd := func(q int64) string { return usdCell(model.QuotaToUsd(q)) }
	totalWidths := []float64{113, 30, 40, 45, 45}
	totalAligns := "LRRRR"

	ledgerPDFSection(pdf, tr(i18n.MsgLedgerExportSectionSummary))
	summary := [][]string{
		{tr(i18n.MsgLedgerExportCreditIncrease), count(ex.increase.Entries), quota(ex.increase.Quota), quotaUsd(ex.increase.Quota), usdCell(ex.increase.MoneyUsd)},
		{tr(i18n.MsgLedgerExportCreditDecrease), count(ex.decrease.Entries), quota(ex.decrease.Quota), quotaUsd(ex.decrease.Quota), usdCell(ex.decrease.MoneyUsd)},
		{tr(i18n.MsgLedgerExportCreditNet), "", quota(ex.increase.Quota - ex.decrease.Quota), quotaUsd(ex.increase.Quota - ex.decrease.Quota), ""},
		{tr(i18n.MsgLedgerExportActualUsage), count(usage.Requests), quota(usage.ChargeQuota), usdCell(usage.ChargeUsd), ""},
		{tr(i18n.MsgLedgerExportFeeCharge), "", quota(usage.ChargeQuota), usdCell(usage.ChargeUsd), ""},
	}
	if scope.internal {
		summary = append(summary,
			[]string{tr(i18n.MsgLedgerExportFeeCost), "", quota(usage.CostQuota), usdCell(usage.CostUsd), ""},
			[]string{tr(i18n.MsgLedgerExportFeeProfit), "", quota(usage.ProfitQuota), usdCell(usage.ProfitUsd), ""},
			[]string{tr(i18n.MsgLedgerExportRedemptionIssued), count(ex.issued.Entries), quota(ex.issued.Quota), quotaUsd(ex.issued.Quota), ""},
			[]string{tr(i18n.MsgLedgerExportRedemptionRefunded), count(ex.refunded.Entries), quota(ex.refunded.Quota), quotaUsd(ex.refunded.Quota), ""},
			[]string{tr(i18n.MsgLedgerExportRedemptionEscrowNet), "", quota(ex.issued.Quota - ex.refunded.Quota), quotaUsd(ex.issued.Quota - ex.refunded.Quota), ""},
		)
	}
	amountHeader := []string{tr(i18n.MsgLedgerExportColCount), tr(i18n.MsgLedgerExportColQuota), tr(i18n.MsgLedgerExportColAmountUsd), tr(i18n.MsgLedgerExportColPaidUsd)}
	ledgerPDFTable(pdf, totalWidths, totalAligns, append([]string{tr(i18n.MsgLedgerExportColItem)}, amountHeader...), summary)

	ledgerPDFSection(pdf, tr(i18n.MsgLedgerExportSectionByKind))
	byKind := make([][]string, 0, len(ex.kindTotals))
	for _, kt := range ex.kindTotals {
		byKind = append(byKind, []string{label(kt.Kind), count(kt.Entries), quota(kt.Quota), quotaUsd(kt.Quota), usdCell(kt.MoneyUsd)})
	}
	ledgerPDFTable(pdf, totalWidths, totalAligns, append([]string{tr(i18n.MsgLedgerExportColKind)}, amountHeader...), byKind)

	ledgerPDFSection(pdf, tr(i18n.MsgLedgerExportSectionRows))
	header := []string{
		tr(i18n.MsgLedgerExportColDate), tr(i18n.MsgLedgerExportColKind), tr(i18n.MsgLedgerExportColQuota),
		tr(i18n.MsgLedgerExportColAmountUsd), tr(i18n.MsgLedgerExportColPaidUsd),
	}
	widths := []float64{40, 60, 45, 45, 40, 43}
	aligns := "LLRRRL"
	if scope.internal {
		header = append(header, tr(i18n.MsgLedgerExportColReseller), tr(i18n.MsgLedgerExportColCustomer),
			tr(i18n.MsgLedgerExportColOperator), tr(i18n.MsgLedgerExportColRef), tr(i18n.MsgLedgerExportColRemark))
		widths = []float64{30, 36, 22, 28, 26, 28, 28, 24, 25, 26}
		aligns = "LLRRRLLLLL"
	} else {
		header = append(header, tr(i18n.MsgLedgerExportColCustomer))
	}
	accountName := func(name string, id int) string {
		if name != "" {
			return name
		}
		if id > 0 {
			return "#" + strconv.Itoa(id)
		}
		return "-"
	}
	loc := model.InvoicePeriodLocation()
	records := make([][]string, 0, len(ex.rows))
	for _, row := range ex.rows {
		paid := ""
		if row.MoneyUsd != nil {
			paid = usdCell(*row.MoneyUsd)
		}
		record := []string{
			time.Unix(row.CreatedAt, 0).In(loc).Format("2006-01-02 15:04"),
			label(row.Kind),
			quota(row.QuotaDelta),
			quotaUsd(row.QuotaDelta),
			paid,
		}
		if scope.internal {
			record = append(record,
				accountName(row.ResellerUsername, row.ResellerId),
				accountName(row.CustomerUsername, row.CustomerId),
				accountName(row.OperatorUsername, row.OperatorId),
				row.RefId,
				row.Remark,
			)
		} else {
			record = append(record, accountName(row.CustomerUsername, row.CustomerId))
		}
		records = append(records, record)
	}
	ledgerPDFTable(pdf, widths, aligns, header, records)
	if len(records) == 0 {
		pdf.SetFont("noto", "", 9)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 7, tr(i18n.MsgLedgerExportNoRows), "", 1, "L", false, 0, "")
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		common.ApiError(c, err)
		return
	}
	filename := fmt.Sprintf("ledger-%s-%s.pdf", ex.period, usage.Scope)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "application/pdf", buf.Bytes())
}

// ledgerPDFSection starts a statement section with a heading, on a new page
// when the heading and the first table rows would not fit.
func ledgerPDFSection(pdf *fpdf.Fpdf, heading string) {
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	if pdf.GetY()+30 > pageHeight-bottomMargin {
		pdf.AddPage()
	} else {
		pdf.Ln(4)
	}
	pdf.SetFont("noto", "", 12)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 8, heading, "", 1, "L", false, 0, "")
}

// ledgerPDFTable draws a striped table, repeating the header on each new
// page and shortening cell text that does not fit its column.
func ledgerPDFTable(pdf *fpdf.Fpdf, widths []float64, aligns string, header []string, rows [][]string) {
	const rowHeight = 6.5
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	drawHeader := func() {
		pdf.SetFont("noto", "", 9)
		pdf.SetFillColor(232, 236, 241)
		pdf.SetTextColor(40, 40, 40)
		pdf.SetDrawColor(210, 214, 220)
		for i, text := range header {
			pdf.CellFormat(widths[i], rowHeight+0.5, ledgerPDFFit(pdf, text, widths[i]), "B", 0, aligns[i:i+1], true, 0, "")
		}
		pdf.Ln(-1)
	}
	drawHeader()
	for n, row := range rows {
		if pdf.GetY()+rowHeight > pageHeight-bottomMargin {
			pdf.AddPage()
			drawHeader()
		}
		pdf.SetFont("noto", "", 8.5)
		pdf.SetTextColor(30, 30, 30)
		pdf.SetFillColor(247, 248, 250)
		for i, text := range row {
			align := aligns[i : i+1]
			if align == "R" {
				// Amounts are never cut: shrink the font until the number fits.
				size := ledgerPDFNumberFontSize(pdf, text, widths[i], 8.5)
				pdf.SetFontSize(size)
				pdf.CellFormat(widths[i], rowHeight, ledgerPDFFit(pdf, text, widths[i]), "", 0, align, n%2 == 1, 0, "")
				pdf.SetFontSize(8.5)
				continue
			}
			pdf.CellFormat(widths[i], rowHeight, ledgerPDFFit(pdf, text, widths[i]), "", 0, align, n%2 == 1, 0, "")
		}
		pdf.Ln(-1)
	}
}

// ledgerPDFFit shortens text with an ellipsis to fit a cell of width w.
// ledgerPDFNumberFontSize returns the largest size <= base (min 4.5) at which
// text fits the cell; numeric cells use it instead of truncation.
func ledgerPDFNumberFontSize(pdf *fpdf.Fpdf, text string, w, base float64) float64 {
	limit := w - 2*pdf.GetCellMargin()
	size := base
	for size > 4.5 {
		pdf.SetFontSize(size)
		if pdf.GetStringWidth(text) <= limit {
			break
		}
		size -= 0.25
	}
	pdf.SetFontSize(base)
	return size
}

func ledgerPDFFit(pdf *fpdf.Fpdf, text string, w float64) string {
	limit := w - 2*pdf.GetCellMargin()
	if pdf.GetStringWidth(text) <= limit {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && pdf.GetStringWidth(string(runes)+"…") > limit {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
