package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestAdminInvoiceCSVRequiresBearer reproduces the dashboard CSV download:
// a plain <a href> navigation sends no Authorization header and is rejected,
// while the SPA's Bearer-authenticated blob fetch receives the CSV.
func TestAdminInvoiceCSVRequiresBearer(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Invoice{}))
	operator := createQuotaTestOperator(t, db, common.RoleRootUser)
	require.NoError(t, model.PublishUserAuthCache(operator.Id))
	inv := model.Invoice{Period: "2026-09", Scope: "admin_rollup", Requests: 3}
	require.NoError(t, db.Create(&inv).Error)
	bundle, err := service.CreateLoginSession(operator.Id, "password", "127.0.0.1", "invoice-csv-test")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/invoices/:id", middleware.AdminAuth(), AdminGetInvoice)
	url := "/api/admin/invoices/" + itoa64(inv.Id) + "?format=csv"

	anon := httptest.NewRecorder()
	router.ServeHTTP(anon, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusUnauthorized, anon.Code)
	require.Contains(t, anon.Body.String(), "AUTH_UNAUTHORIZED")

	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+bundle.AccessToken)
	ok := httptest.NewRecorder()
	router.ServeHTTP(ok, req)
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	require.True(t, strings.HasPrefix(ok.Header().Get("Content-Type"), "text/csv"))
	require.Contains(t, ok.Header().Get("Content-Disposition"), "attachment")
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
