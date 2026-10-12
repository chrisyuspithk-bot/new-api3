package controller

import (
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
