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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callReseller(t *testing.T, h gin.HandlerFunc, method string, resellerID, customerID int, body string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set("id", resellerID)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(customerID)}}
	ctx.Request = httptest.NewRequest(method, "/api/reseller/customers/"+strconv.Itoa(customerID), strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	h(ctx)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func dcQuota(t *testing.T, id int) int {
	var u model.User
	require.NoError(t, model.DB.Select("quota").First(&u, id).Error)
	return u.Quota
}

func TestResellerDirectCreditCustomerEndToEnd(t *testing.T) {
	setupResellerCreditCustomerDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RevenueRecord{}, &model.DirectCreditGrant{}, &model.ProfitCreditAccumulator{}))
	resellerID, customerID := seedResellerCreditFixture(t, 1000)
	require.NoError(t, model.DB.Model(&model.ResellerProfile{}).Where("user_id = ?", resellerID).Update("default_cost_factor", 1.25).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 9500, Username: "dc-root", Role: common.RoleRootUser, Status: 1, Quota: 0, AffCode: "dr0"}).Error)

	// Default mode is markup.
	resp := callReseller(t, ResellerGetCustomer, http.MethodGet, resellerID, customerID, "")
	require.Equal(t, true, resp["success"], resp)
	assert.Equal(t, "markup", resp["data"].(map[string]any)["billing_mode"])

	resp = callReseller(t, ResellerUpdateCustomer, http.MethodPut, resellerID, customerID, `{"billing_mode":"direct_credit"}`)
	require.Equal(t, true, resp["success"], resp)
	assert.Equal(t, "direct_credit", resp["data"].(map[string]any)["billing_mode"])

	resp = callReseller(t, ResellerCreditCustomer, http.MethodPost, resellerID, customerID, `{"quota":120}`)
	require.Equal(t, true, resp["success"], resp)
	assert.Equal(t, 120, dcQuota(t, customerID))
	assert.Equal(t, 900, dcQuota(t, resellerID), "reseller pays 120/1.2")
	assert.Equal(t, 20, dcQuota(t, 9500), "admin profit 100-80 to root")

	resp = callReseller(t, ResellerUpdateCustomer, http.MethodPut, resellerID, customerID, `{"billing_mode":"markup"}`)
	assert.Equal(t, false, resp["success"], "switch rejected with nonzero balance")
	assert.Equal(t, model.ErrBillingModeSwitchNonZero.Error(), resp["message"])

	resp = callReseller(t, ResellerDebitCustomer, http.MethodPost, resellerID, customerID, `{"quota":120}`)
	require.Equal(t, true, resp["success"], resp)
	assert.Equal(t, 0, dcQuota(t, customerID))
	assert.Equal(t, 1000, dcQuota(t, resellerID))
	assert.Equal(t, 0, dcQuota(t, 9500))

	resp = callReseller(t, ResellerUpdateCustomer, http.MethodPut, resellerID, customerID, `{"billing_mode":"markup"}`)
	require.Equal(t, true, resp["success"], resp)
	resp = callReseller(t, ResellerUpdateCustomer, http.MethodPut, resellerID, customerID, `{"billing_mode":"weird"}`)
	assert.Equal(t, false, resp["success"])
}
