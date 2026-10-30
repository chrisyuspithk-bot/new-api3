package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	spChatTokenNamePrefix = "sp-chat-"
	spChatErrorBodyLimit  = 300
)

// ErrSpChatProvisionFailed wraps every provisioning failure so callers can return a
// generic message to end users while the detailed cause stays in server logs.
var ErrSpChatProvisionFailed = errors.New("sp chat provision failed")

var (
	spChatTokenMu sync.Mutex
	spChatToken   string

	spChatUserLocks sync.Map // user id -> *sync.Mutex

	spChatHTTPClient = &http.Client{Timeout: 15 * time.Second}
)

type spChatAddUserResp struct {
	ID string `json:"id"`
}

type spChatGroupResp struct {
	ID string `json:"id"`
}

type spChatSigninResp struct {
	Token string `json:"token"`
}

func SpChatEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(common.GetEnvOrDefaultString("SP_CHAT_PROVISION_ENABLED", "true")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func spChatBaseURL() string {
	return strings.TrimRight(common.GetEnvOrDefaultString("SP_CHAT_BASE_URL", "http://172.17.0.1:8089"), "/")
}

func spChatAdminToken() string {
	spChatTokenMu.Lock()
	defer spChatTokenMu.Unlock()
	if spChatToken == "" {
		spChatToken = strings.TrimSpace(os.Getenv("SP_CHAT_ADMIN_TOKEN"))
	}
	return spChatToken
}

func refreshSpChatAdminToken() error {
	email := strings.TrimSpace(os.Getenv("SP_CHAT_ADMIN_EMAIL"))
	password := os.Getenv("SP_CHAT_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return errors.New("SP_CHAT_ADMIN_TOKEN expired/missing and SP_CHAT_ADMIN_EMAIL/PASSWORD not set")
	}
	var out spChatSigninResp
	if err := spChatSend(http.MethodPost, "/api/v1/auths/signin", "", map[string]string{"email": email, "password": password}, &out); err != nil {
		return err
	}
	if out.Token == "" {
		return errors.New("sp chat signin returned empty token")
	}
	spChatTokenMu.Lock()
	spChatToken = out.Token
	spChatTokenMu.Unlock()
	return nil
}

// spChatSend performs one JSON request against the SP Chat backend. Response bodies
// are truncated in errors so upstream payloads never flood logs.
func spChatSend(method, path, bearer string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := common.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, spChatBaseURL()+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := spChatHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		snippet := string(data[:min(len(data), spChatErrorBodyLimit)])
		return &spChatHTTPError{Status: resp.StatusCode, Msg: fmt.Sprintf("sp chat %s %s -> %d: %s", method, path, resp.StatusCode, snippet)}
	}
	if out != nil && len(data) > 0 {
		if err := common.Unmarshal(data, out); err != nil {
			return fmt.Errorf("sp chat decode %s: %w", path, err)
		}
	}
	return nil
}

type spChatHTTPError struct {
	Status int
	Msg    string
}

func (e *spChatHTTPError) Error() string { return e.Msg }

// spChatAdminDo calls an admin-only SP Chat endpoint, refreshing the service
// admin token once on 401/403.
func spChatAdminDo(method, path string, payload any, out any) error {
	token := spChatAdminToken()
	if token == "" {
		if err := refreshSpChatAdminToken(); err != nil {
			return err
		}
		token = spChatAdminToken()
	}
	err := spChatSend(method, path, token, payload, out)
	var httpErr *spChatHTTPError
	if !errors.As(err, &httpErr) || (httpErr.Status != http.StatusUnauthorized && httpErr.Status != http.StatusForbidden) {
		return err
	}
	if refreshErr := refreshSpChatAdminToken(); refreshErr != nil {
		return fmt.Errorf("%w; token refresh: %v", err, refreshErr)
	}
	return spChatSend(method, path, spChatAdminToken(), payload, out)
}


func spChatNewAPIPublicBaseURL() string {
	return strings.TrimRight(common.GetEnvOrDefaultString("SP_CHAT_NEWAPI_PUBLIC_BASE_URL", "https://api.spit.hk/v1"), "/")
}

// configureSpChatUserLLMConnection signs in as the newly created OWUI user and
// writes settings.directConnections so model requests use this company's New API
// token (one token per sp_company) instead of the shared instance provider key.
func configureSpChatUserLLMConnection(email, plainPassword, tokenKey string) error {
	var signin spChatSigninResp
	if err := spChatSend(http.MethodPost, "/api/v1/auths/signin", "", map[string]string{
		"email": email, "password": plainPassword,
	}, &signin); err != nil {
		return fmt.Errorf("signin new owui user: %w", err)
	}
	if signin.Token == "" {
		return errors.New("signin new owui user returned empty token")
	}
	base := spChatNewAPIPublicBaseURL()
	payload := map[string]any{
		"directConnections": map[string]any{
			"OPENAI_API_BASE_URLS": []string{base},
			"OPENAI_API_KEYS":      []string{tokenKey},
			"OPENAI_API_CONFIGS": map[string]any{
				"0": map[string]any{
					"enable":    true,
					"tags":      []map[string]string{{"name": "New API"}},
					"prefix_id": "newapi",
				},
			},
		},
	}
	if err := spChatSend(http.MethodPost, "/api/v1/users/user/settings/update", signin.Token, payload, nil); err != nil {
		return fmt.Errorf("set owui directConnections: %w", err)
	}
	return nil
}

func orgAdminPermissions() map[string]any {
	return map[string]any{
		"workspace": map[string]any{
			"models": true, "knowledge": true, "prompts": true,
			"tools": false, "skills": false,
			"models_import": false, "models_export": false,
			"prompts_import": true, "prompts_export": true,
			"tools_import": false, "tools_export": false,
			"skills_import": false, "skills_export": false,
		},
		"sharing": map[string]any{
			"models": true, "knowledge": true, "prompts": true,
			"tools": false, "skills": false, "notes": true, "folders": true,
			"public_models": false, "public_knowledge": false, "public_prompts": false,
			"public_tools": false, "public_skills": false, "public_notes": false,
			"public_chats": false, "open_chats": false, "public_calendars": false,
		},
		"access_grants": map[string]any{"allow_users": true, "allow_groups": false},
		"chat": map[string]any{
			"controls": true, "file_upload": true, "delete": true, "edit": true,
			"share": true, "export": true, "import": true, "web_upload": true,
		},
		"features": map[string]any{
			"api_keys": false, "notes": true, "folders": true, "channels": false,
			"web_search": true, "image_generation": true, "code_interpreter": true,
		},
	}
}

func lockSpChatUser(userId int) func() {
	mu, _ := spChatUserLocks.LoadOrStore(userId, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// ProvisionSpChatOrg creates one New API token per company, an SP Chat org-admin
// account (OWUI role "user", never global admin) and a company group tagged
// data.sp_company (including llm_api_key). It is idempotent per user: an already linked user is a no-op,
// and any resource created by a failed attempt is removed before returning so a
// retry starts clean. Callers must only pass an email that has been verified or
// asserted by an administrator. Returned errors wrap ErrSpChatProvisionFailed.
func ProvisionSpChatOrg(userId int, plainPassword, companyName string) (err error) {
	companyName = strings.TrimSpace(companyName)
	if userId == 0 || companyName == "" || !SpChatEnabled() {
		return nil
	}
	unlock := lockSpChatUser(userId)
	defer unlock()

	user, err := model.GetUserById(userId, false)
	if err != nil {
		return fmt.Errorf("%w: load user %d: %v", ErrSpChatProvisionFailed, userId, err)
	}
	if user.SpChatProvisioned == 1 && user.OwuiGroupId != "" {
		return nil
	}
	email := strings.TrimSpace(user.Email)
	if email == "" || plainPassword == "" {
		return fmt.Errorf("%w: user %d needs an email and password", ErrSpChatProvisionFailed, userId)
	}

	var tokenId int
	var owuiUserId, groupId string
	defer func() {
		if err == nil {
			return
		}
		// Undo in reverse order; failures here are logged and left for manual cleanup.
		if groupId != "" {
			if cleanupErr := spChatAdminDo(http.MethodDelete, "/api/v1/groups/id/"+groupId+"/delete", nil, nil); cleanupErr != nil {
				common.SysError(fmt.Sprintf("sp chat cleanup group %s for user %d: %v", groupId, userId, cleanupErr))
			}
		}
		if owuiUserId != "" {
			if cleanupErr := spChatAdminDo(http.MethodDelete, "/api/v1/users/"+owuiUserId, nil, nil); cleanupErr != nil {
				common.SysError(fmt.Sprintf("sp chat cleanup owui user %s for user %d: %v", owuiUserId, userId, cleanupErr))
			}
		}
		if tokenId != 0 {
			if cleanupErr := model.DeleteTokenById(tokenId, userId); cleanupErr != nil {
				common.SysError(fmt.Sprintf("sp chat cleanup token %d for user %d: %v", tokenId, userId, cleanupErr))
			}
		}
		err = fmt.Errorf("%w: user %d: %v", ErrSpChatProvisionFailed, userId, err)
	}()

	key, err := common.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate token key: %w", err)
	}
	token := model.Token{
		UserId:         user.Id,
		Name:           fmt.Sprintf("%s%d", spChatTokenNamePrefix, user.Id),
		Key:            key,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    -1,
		RemainQuota:    user.Quota,
		UnlimitedQuota: user.Quota <= 0,
	}
	if err = token.Insert(); err != nil {
		return fmt.Errorf("insert new-api token: %w", err)
	}
	tokenId = token.Id

	displayName := user.DisplayName
	if displayName == "" {
		displayName = user.Username
	}
	var owuiUser spChatAddUserResp
	if err = spChatAdminDo(http.MethodPost, "/api/v1/auths/add", map[string]any{
		"name":     displayName,
		"email":    email,
		"password": plainPassword,
		"role":     "user",
	}, &owuiUser); err != nil {
		return fmt.Errorf("create owui user: %w", err)
	}
	if owuiUser.ID == "" {
		return errors.New("owui add user returned empty id")
	}
	owuiUserId = owuiUser.ID

	var group spChatGroupResp
	if err = spChatAdminDo(http.MethodPost, "/api/v1/groups/create", map[string]any{
		"name":        companyName,
		"description": fmt.Sprintf("SP Chat org for New API user %d", user.Id),
		"permissions": orgAdminPermissions(),
		"data": map[string]any{
			"sp_company": map[string]any{
				"newapi_user_id":       user.Id,
				"company_name":         companyName,
				"newapi_token_id":      token.Id,
				"admin_user_id":        owuiUserId,
				"newapi_base_url":      spChatNewAPIPublicBaseURL(),
				"llm_connection":       "directConnections",
				// Company-level New API token (one per org). OWUI injects this for
				// all company members; do not mint a distinct token per member.
				"llm_api_key":          key,
			},
		},
	}, &group); err != nil {
		return fmt.Errorf("create owui group: %w", err)
	}
	if group.ID == "" {
		return errors.New("owui create group returned empty id")
	}
	groupId = group.ID

	if err = spChatAdminDo(http.MethodPost, "/api/v1/groups/id/"+groupId+"/users/add", map[string]any{
		"user_ids": []string{owuiUserId},
	}, nil); err != nil {
		return fmt.Errorf("add owui user to group: %w", err)
	}

	if err = configureSpChatUserLLMConnection(email, plainPassword, key); err != nil {
		return fmt.Errorf("configure owui llm connection: %w", err)
	}

	if err = model.DB.Model(&model.User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"company_name":           companyName,
		"owui_group_id":          groupId,
		"owui_admin_user_id":     owuiUserId,
		"newapi_token_id":        token.Id,
		"sp_chat_provisioned":    1,
		"sp_chat_provisioned_at": time.Now().Unix(),
	}).Error; err != nil {
		return fmt.Errorf("save provision link fields: %w", err)
	}

	common.SysLog(fmt.Sprintf("sp chat provisioned user=%d group=%s owui=%s token=%d", user.Id, groupId, owuiUserId, token.Id))
	return nil
}

// SyncSpChatPassword pushes a New API password change to the linked SP Chat
// account. Unlinked users are a no-op.
func SyncSpChatPassword(userId int, plainPassword string) error {
	if userId == 0 || plainPassword == "" || !SpChatEnabled() {
		return nil
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return err
	}
	if user.SpChatProvisioned != 1 || user.OwuiAdminUserId == "" {
		return nil
	}
	if err := spChatAdminDo(http.MethodPost, "/api/v1/users/"+user.OwuiAdminUserId+"/update", map[string]any{
		"password": plainPassword,
	}, nil); err != nil {
		return fmt.Errorf("sync sp chat password for user %d: %w", userId, err)
	}
	common.SysLog(fmt.Sprintf("sp chat password synced for user %d owui=%s", userId, user.OwuiAdminUserId))
	return nil
}

// SyncSpChatPasswordInBackground runs SyncSpChatPassword off the request path so
// an unreachable SP Chat never blocks a New API password change.
func SyncSpChatPasswordInBackground(userId int, plainPassword string) {
	gopool.Go(func() {
		if err := SyncSpChatPassword(userId, plainPassword); err != nil {
			common.SysError(err.Error())
		}
	})
}
