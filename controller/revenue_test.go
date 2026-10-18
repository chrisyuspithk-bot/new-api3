package controller

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRevenueDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.RevenueRecord{}, &model.RevenueMonthlySnapshot{}, &model.ProfitCreditAccumulator{}, &model.DirectCreditGrant{}, &model.ResellerLedger{}))
	require.NoError(t, i18n.Init())
	require.NoError(t, db.Create(&model.User{Id: 61, Username: "rA", AffCode: "x1"}).Error)
	require.NoError(t, db.Create(&model.User{Id: 62, Username: "cA", AffCode: "x2"}).Error)
	require.NoError(t, db.Create(&model.User{Id: 63, Username: "rB", AffCode: "x3"}).Error)
	from, _, _ := model.ParseInvoicePeriod("2026-09")
	for i, rec := range []model.RevenueRecord{
		model.BuildRevenueRecord(model.RevenueContext{PayerType: model.RevenuePayerCustomer, ResellerId: 61, CostFactor: 1.25, Markup: 1.2}, 62, "m", model.RevenueKindConsume, 150, 125),
		model.BuildRevenueRecord(model.RevenueContext{PayerType: model.RevenuePayerResellerSelf, ResellerId: 61, CostFactor: 1.2}, 61, "m", model.RevenueKindConsume, 48, 48),
		model.BuildRevenueRecord(model.RevenueContext{PayerType: model.RevenuePayerCustomer, ResellerId: 63, CostFactor: 1, Markup: 2}, 99, "m", model.RevenueKindConsume, 1000, 500),
	} {
		rec.CreatedAt = from + int64(i) + 1
		rec.DedupeKey = "k" + string(rune('a'+i))
		require.NoError(t, db.Create(&rec).Error)
	}
}

func callRevenue(t *testing.T, h gin.HandlerFunc, userId int, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set("id", userId)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/x?"+query, nil)
	h(ctx)
	return rec
}

func TestResellerRevenueStatementIsScopedToSession(t *testing.T) {
	setupRevenueDB(t)
	rec := callRevenue(t, ResellerRevenueStatement, 61, "period=2026-09&reseller_id=63")
	require.Equal(t, 200, rec.Code)
	var resp struct {
		Success bool                   `json:"success"`
		Data    model.RevenueStatement `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, 61, resp.Data.ResellerId, "reseller_id query ignored")
	assert.Equal(t, int64(150+48), resp.Data.Totals.CustomerPrice)
	assert.NotContains(t, rec.Body.String(), "admin_profit_micro", "admin margin never sent to a reseller")
	assert.NotContains(t, rec.Body.String(), "base_quota_micro")
}

func TestRevenueCSVExportsAdminAndResellerViews(t *testing.T) {
	setupRevenueDB(t)
	rec := callRevenue(t, AdminRevenueStatement, 1, "period=2026-09&format=csv")
	require.Equal(t, 200, rec.Code)
	rows, err := csv.NewReader(strings.NewReader(strings.ReplaceAll(rec.Body.String(), "\n\n", "\n"))).ReadAll()
	if err != nil { // sections have different widths
		rows = nil
	}
	_ = rows
	body := rec.Body.String()
	assert.Contains(t, body, "total_customer_price_quota,1198")
	assert.Contains(t, body, "total_admin_revenue_quota,673")
	assert.Contains(t, body, "check_identity_ok,true")
	assert.Contains(t, body, "by_reseller,63,rB")
	assert.Contains(t, body, "reseller_self,61,rA")

	rec = callRevenue(t, ResellerRevenueStatement, 61, "period=2026-09&format=csv")
	body = rec.Body.String()
	assert.Contains(t, body, "by_customer,62,cA")
	assert.NotContains(t, body, "admin_profit")
	assert.NotContains(t, body, "base_quota")
	assert.NotContains(t, body, "rB", "other resellers invisible")
}

func TestRevenuePDFExportLocalized(t *testing.T) {
	setupRevenueDB(t)
	dir := os.Getenv("PDF_FONT_DIR")
	if dir == "" {
		dir = defaultPDFFontDir
	}
	if _, err := os.Stat(dir + "/NotoSansTC.ttf"); err != nil {
		if _, err2 := os.Stat(dir + "/NotoSansSC.ttf"); err2 != nil {
			t.Skip("CJK font not installed")
		}
	}
	for _, lang := range []string{"en", "zh-TW", "zh-CN"} {
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Set("id", 1)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/x?period=2026-09&format=pdf&lang="+lang, nil)
		ctx.Request.Header.Set("Accept-Language", lang)
		AdminRevenueStatement(ctx)
		require.Equal(t, 200, rec.Code, lang)
		assert.True(t, strings.HasPrefix(rec.Body.String(), "%PDF"), lang)
	}
	_ = common.QuotaPerUnit
}

func TestRevenueStatementRejectsBadInput(t *testing.T) {
	setupRevenueDB(t)
	rec := callRevenue(t, AdminRevenueStatement, 1, "period=2026-09&format=xml")
	assert.Contains(t, rec.Body.String(), `"success":false`)
	rec = callRevenue(t, AdminRevenueStatement, 1, "period=bad")
	assert.Contains(t, rec.Body.String(), `"success":false`)
}
