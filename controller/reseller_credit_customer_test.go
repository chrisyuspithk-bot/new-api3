package controller

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupResellerCreditCustomerDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedis
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ResellerProfile{}, &model.ResellerLedger{}))
}

func seedResellerCreditFixture(t *testing.T, resellerQuota int) (resellerID, customerID int) {
	t.Helper()
	resellerID = 9501
	customerID = 9502
	require.NoError(t, model.DB.Create(&model.User{
		Id: resellerID, Username: "cred-r", Role: common.RoleResellerUser,
		Status: common.UserStatusEnabled, Quota: resellerQuota, AffCode: "cr1",
	}).Error)
	require.NoError(t, model.DB.Create(&model.ResellerProfile{
		UserId: resellerID, DefaultMarkup: 1.2, DisplayName: "Cred R",
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: customerID, Username: "cred-c", Role: common.RoleCommonUser,
		ResellerId: resellerID, Status: common.UserStatusEnabled, Quota: 0, AffCode: "cc1",
	}).Error)
	return resellerID, customerID
}

func postResellerCreditCustomer(t *testing.T, resellerID, customerID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", resellerID)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(customerID)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/reseller/customers/"+strconv.Itoa(customerID)+"/credit", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ResellerCreditCustomer(ctx)
	return recorder
}

func TestResellerCreditCustomerRejectsWhenAmountExceedsAvailableBalance(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 1000)

	recorder := postResellerCreditCustomer(t, resellerID, customerID, `{"quota":1001}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, model.ErrAmountExceedsResellerBalance.Error(), resp["message"])

	var customer model.User
	require.NoError(t, model.DB.First(&customer, customerID).Error)
	assert.Equal(t, 0, customer.Quota, "customer quota unchanged on reject")

	var reseller model.User
	require.NoError(t, model.DB.First(&reseller, resellerID).Error)
	assert.Equal(t, 1000, reseller.Quota, "reseller quota unchanged on reject")

	var n int64
	require.NoError(t, model.DB.Model(&model.ResellerLedger{}).Where("customer_id = ?", customerID).Count(&n).Error)
	assert.Equal(t, int64(0), n, "no ledger row on reject")
}

func TestResellerCreditCustomerAllowsWhenAmountWithinAvailableBalance(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 1000)

	recorder := postResellerCreditCustomer(t, resellerID, customerID, `{"quota":1000}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])

	var customer model.User
	require.NoError(t, model.DB.First(&customer, customerID).Error)
	assert.Equal(t, 1000, customer.Quota)

	var reseller model.User
	require.NoError(t, model.DB.First(&reseller, resellerID).Error)
	assert.Equal(t, 0, reseller.Quota, "credit deducts reseller cost balance 1:1")

	var ledger model.ResellerLedger
	require.NoError(t, model.DB.Where("customer_id = ?", customerID).First(&ledger).Error)
	assert.Equal(t, model.ResellerLedgerCustomerCredit, ledger.Kind)
	assert.Equal(t, int64(1000), ledger.QuotaDelta)
	assert.Equal(t, resellerID, ledger.ResellerId)
}

func TestResellerCreditCustomerPartialTransferUpdatesBothBalances(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 1000)

	recorder := postResellerCreditCustomer(t, resellerID, customerID, `{"quota":400}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"])

	var customer, reseller model.User
	require.NoError(t, model.DB.First(&customer, customerID).Error)
	require.NoError(t, model.DB.First(&reseller, resellerID).Error)
	assert.Equal(t, 400, customer.Quota)
	assert.Equal(t, 600, reseller.Quota)
}

func postResellerDebitCustomer(t *testing.T, resellerID, customerID int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", resellerID)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(customerID)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/reseller/customers/"+strconv.Itoa(customerID)+"/debit", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ResellerDebitCustomer(ctx)
	return recorder
}

func TestResellerDebitCustomerClawbackReturnsQuotaToReseller(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 1000)
	require.Equal(t, http.StatusOK, postResellerCreditCustomer(t, resellerID, customerID, `{"quota":700}`).Code)

	recorder := postResellerDebitCustomer(t, resellerID, customerID, `{"quota":300}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, true, resp["success"])

	var customer, reseller model.User
	require.NoError(t, model.DB.First(&customer, customerID).Error)
	require.NoError(t, model.DB.First(&reseller, resellerID).Error)
	assert.Equal(t, 400, customer.Quota)
	assert.Equal(t, 600, reseller.Quota)

	var ledger model.ResellerLedger
	require.NoError(t, model.DB.Where("customer_id = ? AND kind = ?", customerID, model.ResellerLedgerCustomerDebit).First(&ledger).Error)
	assert.Equal(t, int64(-300), ledger.QuotaDelta)
}

func TestResellerDebitCustomerRejectsWhenCustomerInsufficient(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 1000)
	require.Equal(t, http.StatusOK, postResellerCreditCustomer(t, resellerID, customerID, `{"quota":100}`).Code)

	recorder := postResellerDebitCustomer(t, resellerID, customerID, `{"quota":101}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, model.ErrInsufficientQuota.Error(), resp["message"])

	var customer, reseller model.User
	require.NoError(t, model.DB.First(&customer, customerID).Error)
	require.NoError(t, model.DB.First(&reseller, resellerID).Error)
	assert.Equal(t, 100, customer.Quota)
	assert.Equal(t, 900, reseller.Quota)
}

func TestResellerCreditCustomerRejectsMoneyUsdAboveAvailableBalance(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	old := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = old })

	resellerID, customerID := seedResellerCreditFixture(t, 500000) // $1 available
	recorder := postResellerCreditCustomer(t, resellerID, customerID, `{"money_usd":1.01}`)
	require.Equal(t, http.StatusOK, recorder.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["success"])
	assert.Equal(t, model.ErrAmountExceedsResellerBalance.Error(), resp["message"])
}

func TestAdminListResellerCustomersScopedToPathReseller(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)
	require.NoError(t, model.DB.Create(&model.User{
		Id: 9503, Username: "other-c", ResellerId: 9599, Status: common.UserStatusEnabled, AffCode: "oc1",
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(resellerID)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/admin/resellers/"+strconv.Itoa(resellerID)+"/customers", nil)
	AdminListResellerCustomers(ctx)

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Total int              `json:"total"`
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, 1, resp.Data.Total)
	require.Len(t, resp.Data.Items, 1)
	assert.EqualValues(t, customerID, resp.Data.Items[0]["id"])
}

type ledgerListBody struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Total int `json:"total"`
		Items []struct {
			Kind             string `json:"kind"`
			CustomerId       int    `json:"customer_id"`
			ResellerId       int    `json:"reseller_id"`
			OperatorId       int    `json:"operator_id"`
			OperatorUsername string `json:"operator_username"`
			RefId            string `json:"ref_id"`
			Remark           string `json:"remark"`
		} `json:"items"`
	} `json:"data"`
}

func callLedger(t *testing.T, handler gin.HandlerFunc, userID int, rawURL string) ledgerListBody {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userID)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	handler(ctx)
	var body ledgerListBody
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body
}

func seedMonthLedger(t *testing.T, resellerID, customerID int) (from int64) {
	t.Helper()
	var toEx int64
	var err error
	from, toEx, err = model.ParseInvoicePeriod("2026-09")
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.User{
		Id: 9599, Username: "other-r", Status: common.UserStatusEnabled, AffCode: "or1",
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: 9503, Username: "other-c", ResellerId: resellerID, Status: common.UserStatusEnabled, AffCode: "oc1",
	}).Error)
	rows := []model.ResellerLedger{
		{ResellerId: resellerID, CustomerId: customerID, Kind: model.ResellerLedgerCustomerCredit, QuotaDelta: 10, CreatedAt: from + 1},
		{ResellerId: resellerID, CustomerId: 0, Kind: model.ResellerLedgerAdminCredit, QuotaDelta: 50, CreatedAt: from + 2},
		{ResellerId: resellerID, CustomerId: customerID, Kind: model.ResellerLedgerCustomerDebit, QuotaDelta: -1, CreatedAt: toEx + 1},
		{ResellerId: 9599, CustomerId: 9503, Kind: model.ResellerLedgerCustomerCredit, QuotaDelta: 99, CreatedAt: from + 3},
	}
	require.NoError(t, model.DB.Create(&rows).Error)
	return from
}

func TestResellerGetLedgerPeriodScopesToCallerAndMonth(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)
	seedMonthLedger(t, resellerID, customerID)

	body := callLedger(t, ResellerGetLedger, resellerID, "/api/reseller/ledger?period=2026-09&reseller_id=9599&customer_id=9503")
	require.True(t, body.Success)
	assert.Equal(t, 2, body.Data.Total)
	require.Len(t, body.Data.Items, 2)
	for _, item := range body.Data.Items {
		assert.Equal(t, resellerID, item.ResellerId)
		assert.NotEqual(t, model.ResellerLedgerCustomerDebit, item.Kind)
	}
}

func TestResellerGetLedgerWithoutPeriodKeepsFullCallerHistory(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)
	seedMonthLedger(t, resellerID, customerID)

	body := callLedger(t, ResellerGetLedger, resellerID, "/api/reseller/ledger")
	require.True(t, body.Success)
	assert.Equal(t, 3, body.Data.Total)
	for _, item := range body.Data.Items {
		assert.Equal(t, resellerID, item.ResellerId)
	}
}

func TestCustomerGetLedgerReturnsOnlyCallerRows(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)
	seedMonthLedger(t, resellerID, customerID)

	body := callLedger(t, CustomerGetLedger, customerID, "/api/user/ledger?period=2026-09&reseller_id=9599&customer_id=9503")
	require.True(t, body.Success)
	assert.Equal(t, 1, body.Data.Total)
	require.Len(t, body.Data.Items, 1)
	assert.Equal(t, customerID, body.Data.Items[0].CustomerId)
	assert.Equal(t, model.ResellerLedgerCustomerCredit, body.Data.Items[0].Kind)
}

func TestCustomerGetLedgerHidesOperatorAndRemark(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)
	from := seedMonthLedger(t, resellerID, customerID)
	require.NoError(t, model.DB.Create(&model.ResellerLedger{
		ResellerId: resellerID, CustomerId: customerID, Kind: model.ResellerLedgerCustomerCredit,
		QuotaDelta: 5, OperatorId: resellerID, RefId: "ref-1", Remark: "internal note", CreatedAt: from + 4,
	}).Error)

	customerBody := callLedger(t, CustomerGetLedger, customerID, "/api/user/ledger?period=2026-09")
	require.True(t, customerBody.Success)
	require.Len(t, customerBody.Data.Items, 2)
	for _, item := range customerBody.Data.Items {
		assert.Zero(t, item.OperatorId)
		assert.Empty(t, item.OperatorUsername)
		assert.Empty(t, item.RefId)
		assert.Empty(t, item.Remark)
	}

	resellerBody := callLedger(t, ResellerGetLedger, resellerID, "/api/reseller/ledger?period=2026-09&kind=customer_credit")
	require.True(t, resellerBody.Success)
	require.NotEmpty(t, resellerBody.Data.Items)
	assert.Equal(t, resellerID, resellerBody.Data.Items[0].OperatorId)
	assert.NotEmpty(t, resellerBody.Data.Items[0].OperatorUsername)
	assert.Equal(t, "internal note", resellerBody.Data.Items[0].Remark)
}

func TestLedgerPeriodRejectsInvalidMonth(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)

	resellerBody := callLedger(t, ResellerGetLedger, resellerID, "/api/reseller/ledger?period=2026-9")
	assert.False(t, resellerBody.Success)
	assert.Equal(t, model.ErrInvalidInvoicePeriod.Error(), resellerBody.Message)

	customerBody := callLedger(t, CustomerGetLedger, customerID, "/api/user/ledger?period=nope")
	assert.False(t, customerBody.Success)
	assert.Equal(t, model.ErrInvalidInvoicePeriod.Error(), customerBody.Message)
}

func TestFillResellerNamesSetsNamesOnlyForAffiliatedUsers(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, customerID := seedResellerCreditFixture(t, 0)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", resellerID).Update("display_name", "Reseller R").Error)

	users := []*model.User{{Id: customerID, ResellerId: resellerID}, {Id: 1, ResellerId: 0}}
	require.NoError(t, model.FillResellerNames(users))

	assert.Equal(t, "cred-r", users[0].ResellerUsername)
	assert.Equal(t, "Reseller R", users[0].ResellerDisplayName)
	assert.Empty(t, users[1].ResellerUsername)
	assert.Empty(t, users[1].ResellerDisplayName)
}

// ledgerExport is the parsed balance-ledger export CSV.
type ledgerExport struct {
	summary map[string]string
	header  []string
	rows    [][]string
}

func callLedgerExport(t *testing.T, handler gin.HandlerFunc, userID int, rawURL string) ledgerExport {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userID)
	ctx.Request = httptest.NewRequest(http.MethodGet, rawURL, nil)
	handler(ctx)
	require.True(t, strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/csv"), recorder.Body.String())
	reader := csv.NewReader(recorder.Body)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	require.NoError(t, err)
	out := ledgerExport{summary: map[string]string{}}
	section := ""
	for _, rec := range records {
		if strings.HasPrefix(rec[0], "# ") {
			section = rec[0]
			continue
		}
		switch section {
		case "# SUMMARY":
			if len(rec) == 2 {
				out.summary[rec[0]] = rec[1]
			}
		case "# LEDGER ROWS":
			if out.header == nil {
				out.header = rec
			} else {
				out.rows = append(out.rows, rec)
			}
		}
	}
	return out
}

// seedMonthUsage writes September consume/refund logs: the fixture customer
// nets charge 80 / cost 56 over one request; other rows are out of scope.
func seedMonthUsage(t *testing.T, resellerID, customerID int, from int64) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
	logs := []model.Log{
		{Type: model.LogTypeConsume, UserId: customerID, Username: "cred-c", ModelName: "m", Quota: 100, ResellerId: resellerID, ResellerCostQuota: 70, CreatedAt: from + 10},
		{Type: model.LogTypeRefund, UserId: customerID, Username: "cred-c", ModelName: "m", Quota: 20, ResellerId: resellerID, ResellerCostQuota: 14, CreatedAt: from + 11},
		{Type: model.LogTypeConsume, UserId: 9503, Username: "other-c", ModelName: "m", Quota: 500, ResellerId: 9599, ResellerCostQuota: 400, CreatedAt: from + 12},
		{Type: model.LogTypeConsume, UserId: customerID, Username: "cred-c", ModelName: "m", Quota: 7, ResellerId: resellerID, ResellerCostQuota: 5, CreatedAt: from - 1},
	}
	require.NoError(t, model.DB.Create(&logs).Error)
}

func seedLedgerExportFixture(t *testing.T) (resellerID, customerID int) {
	t.Helper()
	setupResellerCreditCustomerDB(t)
	resellerID, customerID = seedResellerCreditFixture(t, 0)
	from := seedMonthLedger(t, resellerID, customerID)
	money := 2.5
	require.NoError(t, model.DB.Create(&model.ResellerLedger{
		ResellerId: resellerID, CustomerId: customerID, Kind: model.ResellerLedgerCustomerDebit,
		QuotaDelta: -4, MoneyUsd: &money, Remark: "=cmd", CreatedAt: from + 5,
	}).Error)
	seedMonthUsage(t, resellerID, customerID, from)
	return resellerID, customerID
}

func TestResellerExportLedgerSummarisesWholeMonthAndFiltersRowsByKind(t *testing.T) {
	resellerID, customerID := seedLedgerExportFixture(t)

	out := callLedgerExport(t, ResellerExportLedger, resellerID, "/api/reseller/ledger/export?period=2026-09&kind=customer_debit&reseller_id=9599")

	assert.Equal(t, "reseller", out.summary["scope"])
	assert.Equal(t, "customer_debit", out.summary["ledger_rows_kind_filter"])
	assert.Equal(t, "60", out.summary["credit_increase_quota"])
	assert.Equal(t, "4", out.summary["credit_decrease_quota"])
	assert.Equal(t, "2.500000", out.summary["credit_decrease_money_usd"])
	assert.Equal(t, "56", out.summary["credit_net_quota"])
	assert.Equal(t, "1", out.summary["actual_usage_requests"])
	assert.Equal(t, "80", out.summary["actual_usage_quota"])
	assert.Equal(t, "56", out.summary["related_fee_cost_quota"])
	assert.Equal(t, "24", out.summary["related_fee_profit_quota"])
	require.Len(t, out.rows, 1)
	assert.Contains(t, out.header, "remark")
	assert.Equal(t, strconv.Itoa(customerID), out.rows[0][5])
	assert.Equal(t, "-4", out.rows[0][8])
	assert.Equal(t, "'=cmd", out.rows[0][len(out.rows[0])-1])
}

func TestCustomerExportLedgerOnlyCoversCallerAndHidesInternalFields(t *testing.T) {
	_, customerID := seedLedgerExportFixture(t)

	out := callLedgerExport(t, CustomerExportLedger, customerID, "/api/user/ledger/export?period=2026-09&customer_id=9503")

	assert.Equal(t, "customer", out.summary["scope"])
	assert.Equal(t, "10", out.summary["credit_increase_quota"])
	assert.Equal(t, "4", out.summary["credit_decrease_quota"])
	assert.Equal(t, "80", out.summary["related_fee_charge_quota"])
	assert.NotContains(t, out.summary, "related_fee_cost_quota")
	assert.NotContains(t, out.summary, "related_fee_profit_quota")
	assert.NotContains(t, out.header, "remark")
	assert.NotContains(t, out.header, "operator_username")
	require.Len(t, out.rows, 2)
	for _, row := range out.rows {
		assert.Equal(t, strconv.Itoa(customerID), row[5])
	}
}

func TestAdminExportResellerLedgerCoversEveryReseller(t *testing.T) {
	seedLedgerExportFixture(t)

	out := callLedgerExport(t, AdminExportResellerLedger, 1, "/api/admin/reseller_ledger/export?period=2026-09")

	assert.Equal(t, "admin_rollup", out.summary["scope"])
	assert.Equal(t, "all", out.summary["ledger_rows_kind_filter"])
	assert.Equal(t, "159", out.summary["credit_increase_quota"])
	assert.Equal(t, "4", out.summary["credit_decrease_quota"])
	assert.Equal(t, "2", out.summary["actual_usage_requests"])
	assert.Equal(t, "580", out.summary["related_fee_charge_quota"])
	assert.Equal(t, "456", out.summary["related_fee_cost_quota"])
	assert.Len(t, out.rows, 4)
}

func TestLedgerExportRejectsMissingPeriod(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	resellerID, _ := seedResellerCreditFixture(t, 0)

	body := callLedger(t, ResellerExportLedger, resellerID, "/api/reseller/ledger/export")
	assert.False(t, body.Success)
	assert.Equal(t, model.ErrInvalidInvoicePeriod.Error(), body.Message)
}
