package service

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProvisionSpChatOrg(t *testing.T) {
	previousDB, previousRedis := model.DB, common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}))
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
	})

	var mu sync.Mutex
	var calls []string
	failGroupCreate := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		fail := failGroupCreate
		mu.Unlock()
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/auths/add":
			_, _ = w.Write([]byte(`{"id":"owui-1"}`))
		case "/api/v1/groups/create":
			if fail {
				http.Error(w, `{"detail":"boom"}`, http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"id":"group-1"}`))
		default:
			_, _ = w.Write([]byte(`true`))
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("SP_CHAT_PROVISION_ENABLED", "true")
	t.Setenv("SP_CHAT_BASE_URL", server.URL)
	t.Setenv("SP_CHAT_ADMIN_TOKEN", "service-token")
	spChatTokenMu.Lock()
	spChatToken = ""
	spChatTokenMu.Unlock()

	user := model.User{Username: "acme", Email: "owner@acme.test", Password: "x", CompanyName: "Acme"}
	require.NoError(t, db.Create(&user).Error)

	// A failed group create must roll back the OWUI user and the New API token.
	err = ProvisionSpChatOrg(user.Id, "Passw0rd!", "Acme")
	require.ErrorIs(t, err, ErrSpChatProvisionFailed)
	assert.Contains(t, calls, "DELETE /api/v1/users/owui-1")
	var tokenCount int64
	require.NoError(t, db.Model(&model.Token{}).Where("user_id = ?", user.Id).Count(&tokenCount).Error)
	assert.Zero(t, tokenCount)
	stored, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Zero(t, stored.SpChatProvisioned)

	// A retry provisions cleanly and links the user.
	failGroupCreate = false
	require.NoError(t, ProvisionSpChatOrg(user.Id, "Passw0rd!", "Acme"))
	stored, err = model.GetUserById(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 1, stored.SpChatProvisioned)
	assert.Equal(t, "group-1", stored.OwuiGroupId)
	assert.Equal(t, "owui-1", stored.OwuiAdminUserId)
	assert.NotZero(t, stored.NewapiTokenId)

	// A provisioned user is a no-op: no further SP Chat calls.
	callsBefore := len(calls)
	require.NoError(t, ProvisionSpChatOrg(user.Id, "Passw0rd!", "Acme"))
	assert.Len(t, calls, callsBefore)
}
