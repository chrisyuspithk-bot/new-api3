package controller

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordsResp struct {
	Success bool `json:"success"`
	Data    struct {
		Items  []map[string]any `json:"items"`
		Total  int64            `json:"total"`
		Totals map[string]any   `json:"totals"`
	} `json:"data"`
}

func decodeRecords(t *testing.T, rec *httptest.ResponseRecorder) recordsResp {
	t.Helper()
	require.Equal(t, 200, rec.Code)
	var r recordsResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &r))
	require.True(t, r.Success, rec.Body.String())
	return r
}

func TestAdminRevenueRecordsListPagingAndFilters(t *testing.T) {
	setupRevenueDB(t)
	r := decodeRecords(t, callRevenue(t, AdminRevenueRecords, 1, "p=1&page_size=2"))
	assert.Equal(t, int64(3), r.Data.Total)
	require.Len(t, r.Data.Items, 2, "paged")
	assert.Equal(t, "rB", r.Data.Items[0]["reseller_username"], "newest first with names")
	assert.Contains(t, r.Data.Items[0], "admin_profit")
	assert.EqualValues(t, 1198, r.Data.Totals["customer_price"], "totals over all filtered rows, not the page")

	r = decodeRecords(t, callRevenue(t, AdminRevenueRecords, 1, "reseller_id=61"))
	assert.Equal(t, int64(2), r.Data.Total)
	r = decodeRecords(t, callRevenue(t, AdminRevenueRecords, 1, "customer_id=62"))
	assert.Equal(t, int64(1), r.Data.Total)
	assert.Equal(t, "cA", r.Data.Items[0]["username"])

	from, _, _ := model.ParseInvoicePeriod("2026-09")
	q := "start_timestamp=" + itoa64(from+2) + "&end_timestamp=" + itoa64(from+3)
	r = decodeRecords(t, callRevenue(t, AdminRevenueRecords, 1, q))
	assert.Equal(t, int64(1), r.Data.Total, "date range: [start, end)")
}

func TestResellerRevenueRecordsScopedAndStripped(t *testing.T) {
	setupRevenueDB(t)
	rec := callRevenue(t, ResellerRevenueRecords, 61, "reseller_id=63")
	r := decodeRecords(t, rec)
	assert.Equal(t, int64(2), r.Data.Total, "own customers + own usage; reseller_id param ignored")
	body := rec.Body.String()
	assert.NotContains(t, body, "admin_profit")
	assert.NotContains(t, body, "admin_credited")
	assert.NotContains(t, body, "cost_factor")
	assert.NotContains(t, body, "admin_accumulator")
	assert.NotContains(t, body, "rB")
}

func TestRevenueRecordsCSVExportReconciles(t *testing.T) {
	setupRevenueDB(t)
	body := callRevenue(t, AdminRevenueRecords, 1, "format=csv").Body.String()
	assert.True(t, strings.HasPrefix(body, "\xEF\xBB\xBF"), "UTF-8 BOM")
	assert.Contains(t, body, "Total,3,,,,,1198,673,525,")
	body = callRevenue(t, ResellerRevenueRecords, 61, "format=csv").Body.String()
	assert.Contains(t, body, "Total,2,,,,198,173,25")
	assert.Contains(t, body, "Total,2,,,,198,173,25,,,0.000396,0.000346,0.000050\n", "USD totals at 6 decimals reconcile with quota totals")
	assert.NotContains(t, body, "rB")
}

func TestRevenueRecordsPDFExportLocalized(t *testing.T) {
	setupRevenueDB(t)
	dir := os.Getenv("PDF_FONT_DIR")
	if dir == "" {
		dir = defaultPDFFontDir
	}
	if _, err := os.Stat(dir + "/NotoSansTC.ttf"); err != nil {
		t.Skip("CJK font not installed")
	}
	for _, lang := range []string{"en", "zh-TW", "zh-CN"} {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Set("id", 61)
		ctx.Request = httptest.NewRequest("GET", "/x?format=pdf", nil)
		ctx.Request.Header.Set("Accept-Language", lang)
		ResellerRevenueRecords(ctx)
		require.Equal(t, 200, rec.Code, lang)
		assert.True(t, strings.HasPrefix(rec.Body.String(), "%PDF"), lang)
	}
}

func TestRevenueUsdCellsTinyCallsVisible(t *testing.T) {
	assert.Equal(t, []string{"0.000124", "0.000104", "0.000020"}, revenueUsdCells(62, 52, 10, 0, false))
	assert.Equal(t, []string{"0.000002", "0.000002", "0.000000", "0.000002"}, revenueUsdCells(1, 1, 0, 1_000_000, true))
	assert.Equal(t, "62 ($0.000124)", quotaUsdPDF("62", "0.000124"))
}
