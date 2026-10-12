package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func setupResellerTopupPolicyDB(t *testing.T) {
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.ResellerProfile{}))
}

func TestBuildSelfResellerInfoEnablesOnlineTopup(t *testing.T) {
	setupResellerTopupPolicyDB(t)
	const resellerID = 9401
	require.NoError(t, model.DB.Create(&model.User{
		Id: resellerID, Username: "pol-r", Role: common.RoleResellerUser, AffCode: "pr1",
	}).Error)
	require.NoError(t, model.DB.Create(&model.ResellerProfile{
		UserId: resellerID, DisplayName: "Acme", DefaultMarkup: 1.5,
	}).Error)
	require.NoError(t, model.DB.Create(&model.User{
		Id: 9402, Username: "pol-c", Role: common.RoleCommonUser, ResellerId: resellerID, AffCode: "pc1",
	}).Error)

	resellerInfo := buildSelfResellerInfo(&model.User{Id: resellerID, Role: common.RoleResellerUser})
	require.NotNil(t, resellerInfo)
	assert.Equal(t, "reseller", resellerInfo["role"])
	assert.Equal(t, true, resellerInfo["online_topup_enabled"])

	customerInfo := buildSelfResellerInfo(&model.User{Id: 9402, Role: common.RoleCommonUser, ResellerId: resellerID})
	require.NotNil(t, customerInfo)
	assert.Equal(t, "customer", customerInfo["role"])
	assert.Equal(t, true, customerInfo["online_topup_enabled"])
	assert.Equal(t, "Acme", customerInfo["reseller_display_name"])

	assert.Nil(t, buildSelfResellerInfo(&model.User{Id: 1, Role: common.RoleCommonUser, ResellerId: 0}))
}

func TestBlockResellerCustomerStillBlocksNonTopupRoutes(t *testing.T) {
	setupResellerTopupPolicyDB(t)
	const resellerID = 9411
	require.NoError(t, model.DB.Create(&model.User{
		Id: resellerID, Username: "blk-r", Role: common.RoleResellerUser, AffCode: "br1",
	}).Error)
	customer := &model.User{
		Id: 9412, Username: "blk-c", Role: common.RoleCommonUser, ResellerId: resellerID, AffCode: "bc1", Status: common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(customer).Error)
	direct := &model.User{
		Id: 9413, Username: "blk-d", Role: common.RoleCommonUser, ResellerId: 0, AffCode: "bd1", Status: common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(direct).Error)

	gin.SetMode(gin.TestMode)
	mw := BlockResellerCustomer()

	run := func(userID int) (aborted bool, code int, body map[string]any) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/stripe/pay", nil)
		c.Set("id", userID)
		mw(c)
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return c.IsAborted(), rec.Code, body
	}

	// Customer is blocked (subscriptions stay reseller-managed).
	aborted, code, body := run(customer.Id)
	assert.True(t, aborted)
	assert.Equal(t, http.StatusForbidden, code)
	assert.Equal(t, false, body["success"])

	// Direct user and reseller (reseller_id=0) pass through — online top-up
	// uses the same gateways; only non-topup routes keep this middleware.
	aborted, code, _ = run(direct.Id)
	assert.False(t, aborted)
	assert.NotEqual(t, http.StatusForbidden, code)
	aborted, code, _ = run(resellerID)
	assert.False(t, aborted)
	assert.NotEqual(t, http.StatusForbidden, code)
}

func TestOnlinePayRoutesNoLongerMountBlockResellerCustomer(t *testing.T) {
	raw, err := os.ReadFile("../router/api-router.go")
	require.NoError(t, err)
	src := string(raw)
	for _, route := range []string{
		`selfRoute.POST("/pay"`,
		`selfRoute.POST("/amount"`,
		`selfRoute.POST("/stripe/pay"`,
		`selfRoute.POST("/stripe/amount"`,
		`selfRoute.POST("/creem/pay"`,
		`selfRoute.POST("/waffo/amount"`,
		`selfRoute.POST("/waffo/pay"`,
		`selfRoute.POST("/waffo-pancake/amount"`,
		`selfRoute.POST("/waffo-pancake/pay"`,
	} {
		idx := strings.Index(src, route)
		require.GreaterOrEqual(t, idx, 0, route)
		lineEnd := strings.Index(src[idx:], "\n")
		line := src[idx : idx+lineEnd]
		assert.NotContains(t, line, "BlockResellerCustomer", route+" must allow reseller customers to top up")
	}
	// Non-topup customer restrictions remain.
	assert.Contains(t, src, `selfRoute.POST("/aff_transfer", controller.BlockResellerCustomer()`)
	assert.Contains(t, src, `selfRoute.POST("/checkin", controller.BlockResellerCustomer()`)
	assert.Contains(t, src, `subscriptionRoute.POST("/stripe/pay", controller.BlockResellerCustomer()`)
}
