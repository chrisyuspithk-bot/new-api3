package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

type resellerQuotaRequest struct {
	Quota    int      `json:"quota"`
	MoneyUsd *float64 `json:"money_usd"`
	Remark   string   `json:"remark"`
}

// quotaFromRequest resolves a positive quota amount from quota or money_usd (1:1 USD).
func (r resellerQuotaRequest) quotaAmount() (int, error) {
	if r.MoneyUsd != nil {
		if !(*r.MoneyUsd > 0) {
			return 0, errors.New("money_usd must be positive")
		}
		return common.WalletQuotaFromDecimalStrict(decimal.NewFromFloat(*r.MoneyUsd).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	}
	if r.Quota <= 0 {
		return 0, errors.New("quota must be positive")
	}
	if err := common.ValidateWalletQuota(r.Quota); err != nil {
		return 0, err
	}
	return r.Quota, nil
}

func paramId(c *gin.Context, name string) (int, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid id")
		return 0, false
	}
	return id, true
}

func loadReseller(c *gin.Context, id int) (*model.User, bool) {
	var user model.User
	if err := model.DB.Omit("password").Where("id = ? AND role = ?", id, common.RoleResellerUser).First(&user).Error; err != nil {
		common.ApiErrorMsg(c, "reseller not found")
		return nil, false
	}
	return &user, true
}

func resellerView(user *model.User) gin.H {
	profile, _ := model.GetResellerProfile(user.Id)
	groups, _ := model.GetResellerGroups(user.Id)
	customers, _ := model.CountResellerCustomers(user.Id)
	return gin.H{
		"id":             user.Id,
		"username":       user.Username,
		"display_name":   user.DisplayName,
		"status":         user.Status,
		"group":          user.Group,
		"quota":          user.Quota,
		"used_quota":     user.UsedQuota,
		"profile":        profile,
		"groups":         groups,
		"customer_count": customers,
	}
}

// buildSelfResellerInfo adds B2B2C context to /api/user/self. Customers see only
// their reseller's display name; resellers see their commercial profile.
// online_topup_enabled is true for both: customers and resellers may use the
// existing payment gateways. Customer consume still requires reseller cost stock
// (dual-leg billing); top-up only credits the payer's users.quota wallet.
func buildSelfResellerInfo(user *model.User) gin.H {
	if user.Role == common.RoleResellerUser {
		profile, _ := model.GetResellerProfile(user.Id)
		groups, _ := model.GetResellerGroups(user.Id)
		return gin.H{"role": "reseller", "profile": profile, "groups": groups, "online_topup_enabled": true}
	}
	if user.ResellerId > 0 {
		profile, _ := model.GetResellerProfile(user.ResellerId)
		name := ""
		if profile != nil {
			name = profile.DisplayName
		}
		return gin.H{"role": "customer", "reseller_id": user.ResellerId, "reseller_display_name": name, "online_topup_enabled": true}
	}
	return nil
}

// BlockResellerCustomer rejects B2B2C customers (reseller_id > 0) from routes
// that remain reseller-managed in phase 1: subscriptions, aff transfer, check-in.
// Online wallet top-up (epay/Stripe/Creem/Waffo/Waffo-Pancake) is intentionally
// allowed for customers and resellers via the same payment pipelines.
func BlockResellerCustomer() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := model.GetUserCache(c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			c.Abort()
			return
		}
		if user.ResellerId > 0 {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "This feature is not available for reseller customers"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func validateGroupsExist(groups []string) error {
	for _, g := range groups {
		if !ratio_setting.ContainsGroupRatio(g) {
			return errors.New("unknown group: " + g)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Admin APIs
// ---------------------------------------------------------------------------

type adminResellerRequest struct {
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	DisplayName     string   `json:"display_name"`
	UserId          int      `json:"user_id"` // upgrade an existing direct common user
	Group           string   `json:"group"`
	Groups          []string `json:"groups"`
	DefaultMarkup   *float64 `json:"default_markup"`
	MaxCustomers    *int     `json:"max_customers"`
	AllowRedemption *bool    `json:"allow_redemption"`
	BrandName       *string  `json:"brand_name"`
	Status          *int     `json:"status"`
}

func applyResellerProfile(id int, req adminResellerRequest) error {
	profile, err := model.GetResellerProfile(id)
	if err != nil {
		return err
	}
	profile.UserId = id
	if req.DefaultMarkup != nil {
		profile.DefaultMarkup = *req.DefaultMarkup
	}
	if req.MaxCustomers != nil {
		profile.MaxCustomers = max(*req.MaxCustomers, 0)
	}
	if req.AllowRedemption != nil {
		profile.AllowRedemption = *req.AllowRedemption
	}
	if req.BrandName != nil {
		profile.DisplayName = *req.BrandName
	}
	return model.SaveResellerProfile(profile)
}

func AdminListResellers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var users []model.User
	var total int64
	query := model.DB.Model(&model.User{}).Omit("password").Where("role = ?", common.RoleResellerUser)
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(users))
	for i := range users {
		items = append(items, resellerView(&users[i]))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func AdminGetReseller(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	user, ok := loadReseller(c, id)
	if !ok {
		return
	}
	common.ApiSuccess(c, resellerView(user))
}

// AdminListResellerCustomers lists the customers owned by reseller :id
// (admin read-only view of ResellerListCustomers).
func AdminListResellerCustomers(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if _, ok := loadReseller(c, id); !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	var users []model.User
	var total int64
	query := model.DB.Model(&model.User{}).Omit("password").Where("reseller_id = ?", id)
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("username LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(users))
	for i := range users {
		items = append(items, customerView(&users[i]))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func AdminCreateReseller(c *gin.Context) {
	var req adminResellerRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	if req.DefaultMarkup != nil && !model.ValidMarkup(*req.DefaultMarkup) {
		common.ApiError(c, model.ErrInvalidMarkup)
		return
	}
	if err := validateGroupsExist(req.Groups); err != nil {
		common.ApiError(c, err)
		return
	}
	var id int
	if req.UserId > 0 {
		var existing model.User
		if err := model.DB.Where("id = ?", req.UserId).First(&existing).Error; err != nil {
			common.ApiErrorMsg(c, "user not found")
			return
		}
		if existing.Role != common.RoleCommonUser || existing.ResellerId != 0 {
			common.ApiErrorMsg(c, "only direct common users can be upgraded to reseller")
			return
		}
		if err := model.DB.Model(&model.User{}).Where("id = ?", existing.Id).Update("role", common.RoleResellerUser).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		if _, err := model.BumpUserAuthVersion(existing.Id); err != nil {
			common.SysError(common.LogText("failed to bump auth version: %s", err.Error()))
		}
		id = existing.Id
	} else {
		username := strings.TrimSpace(req.Username)
		if username == "" || len(req.Password) < 8 {
			common.ApiErrorMsg(c, "username and password (min 8 chars) are required")
			return
		}
		user := model.User{Username: username, Password: req.Password, DisplayName: req.DisplayName, Role: common.RoleResellerUser}
		if user.DisplayName == "" {
			user.DisplayName = username
		}
		if req.Group != "" {
			user.Group = req.Group
		}
		if err := common.Validate.Struct(&user); err != nil {
			common.ApiError(c, err)
			return
		}
		if err := model.DB.Transaction(func(tx *gorm.DB) error { return user.InsertWithTx(tx, 0) }); err != nil {
			common.ApiError(c, err)
			return
		}
		// Reseller cost balance starts at 0 and is only funded by admin credit.
		if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 0).Error; err != nil {
			common.ApiError(c, err)
			return
		}
		id = user.Id
	}
	if err := applyResellerProfile(id, req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Groups != nil {
		if err := model.SetResellerGroups(id, req.Groups, req.Group); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	model.InvalidateResellerCache(id)
	user, ok := loadReseller(c, id)
	if !ok {
		return
	}
	common.ApiSuccess(c, resellerView(user))
}

func AdminUpdateReseller(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if _, ok := loadReseller(c, id); !ok {
		return
	}
	var req adminResellerRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	if req.DefaultMarkup != nil && !model.ValidMarkup(*req.DefaultMarkup) {
		common.ApiError(c, model.ErrInvalidMarkup)
		return
	}
	if err := applyResellerProfile(id, req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.DisplayName != "" {
		if err := model.DB.Model(&model.User{}).Where("id = ?", id).Update("display_name", req.DisplayName).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if req.Status != nil {
		if *req.Status != common.UserStatusEnabled && *req.Status != common.UserStatusDisabled {
			common.ApiErrorMsg(c, "invalid status")
			return
		}
		// Disabling a reseller freezes the whole line: TokenAuth and billing
		// both check the reseller status (cache TTL <= 30s on other nodes).
		if err := model.SetUserStatusAndRevoke(id, *req.Status); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	model.InvalidateResellerCache(id)
	user, _ := loadReseller(c, id)
	common.ApiSuccess(c, resellerView(user))
}

// AdminDeleteReseller disables a reseller (soft); customer lines are frozen.
func AdminDeleteReseller(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if _, ok := loadReseller(c, id); !ok {
		return
	}
	if err := model.SetUserStatusAndRevoke(id, common.UserStatusDisabled); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func adminAdjustReseller(c *gin.Context, sign int) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if _, ok := loadReseller(c, id); !ok {
		return
	}
	var req resellerQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	amount, err := req.quotaAmount()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	kind := model.ResellerLedgerAdminCredit
	if sign < 0 {
		kind = model.ResellerLedgerAdminDebit
	}
	entry := model.ResellerLedger{ResellerId: id, Kind: kind, MoneyUsd: req.MoneyUsd, OperatorId: c.GetInt("id"), Remark: req.Remark}
	if err := model.AdjustQuotaWithLedger(id, sign*amount, entry); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(id, model.LogTypeResellerAdminCredit, common.NewMessage("Admin adjusted reseller balance by {{quota}}", map[string]any{"quota": signedQuota(sign, amount)}))
	common.ApiSuccess(c, gin.H{"quota_delta": sign * amount})
}

func signedQuota(sign int, amount int) string {
	if sign < 0 {
		return "-" + logger.FormatQuota(amount)
	}
	return logger.FormatQuota(amount)
}

func AdminCreditReseller(c *gin.Context) { adminAdjustReseller(c, 1) }
func AdminDebitReseller(c *gin.Context)  { adminAdjustReseller(c, -1) }

func AdminGetResellerGroups(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	groups, err := model.GetResellerGroups(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, groups)
}

func AdminSetResellerGroups(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	reseller, ok := loadReseller(c, id)
	if !ok {
		return
	}
	var req struct {
		Groups        []string `json:"groups"`
		FallbackGroup string   `json:"fallback_group"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	if err := validateGroupsExist(req.Groups); err != nil {
		common.ApiError(c, err)
		return
	}
	fallback := req.FallbackGroup
	if fallback == "" {
		fallback = reseller.Group
	}
	if err := model.SetResellerGroups(id, req.Groups, fallback); err != nil {
		common.ApiError(c, err)
		return
	}
	groups, _ := model.GetResellerGroups(id)
	common.ApiSuccess(c, groups)
}

func AdminListResellerModelPrices(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	var rows []model.ResellerModelPrice
	if err := model.DB.Where("reseller_id = ?", id).Order("model_name").Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func AdminUpsertResellerModelPrice(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if _, ok := loadReseller(c, id); !ok {
		return
	}
	var row model.ResellerModelPrice
	if err := common.DecodeJson(c.Request.Body, &row); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	if m := c.Param("model"); m != "" {
		row.ModelName = m
	}
	row.Id = 0
	row.ResellerId = id
	if strings.TrimSpace(row.ModelName) == "" {
		common.ApiErrorMsg(c, "model_name is required")
		return
	}
	if err := model.UpsertResellerModelPrice(&row); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func AdminDeleteResellerModelPrice(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if err := model.DeleteResellerModelPrice(id, c.Param("model")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminGetResellerLedger(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	rows, total, err := model.GetResellerLedger(id, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), c.Query("kind"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

// AdminListResellerLedger lists credit/debit ledger rows across all resellers.
// Query: period=YYYY-MM (preferred) or from/to unix; optional kind, reseller_id.
func AdminListResellerLedger(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var from, toEx int64
	if period := c.Query("period"); period != "" {
		var err error
		from, toEx, err = model.ParseInvoicePeriod(period)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		from, _ = strconv.ParseInt(c.Query("from"), 10, 64)
		toEx, _ = strconv.ParseInt(c.Query("to"), 10, 64)
	}
	resellerId, _ := strconv.Atoi(c.Query("reseller_id"))
	rows, total, err := model.ListResellerLedgerRange(from, toEx, resellerId, 0, c.Query("kind"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func grossProfit(c *gin.Context, resellerId int) {
	from, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	to, _ := strconv.ParseInt(c.Query("to"), 10, 64)
	rows, err := model.GetResellerGrossProfit(resellerId, from, to, c.Query("group_by"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func AdminGrossProfitReport(c *gin.Context) {
	resellerId, _ := strconv.Atoi(c.Query("reseller_id"))
	grossProfit(c, resellerId)
}

// ---------------------------------------------------------------------------
// Reseller APIs (ResellerAuth). Every query is scoped to reseller_id = me.
// ---------------------------------------------------------------------------

func ResellerGetSelf(c *gin.Context) {
	user, ok := loadReseller(c, c.GetInt("id"))
	if !ok {
		return
	}
	common.ApiSuccess(c, resellerView(user))
}

func ResellerUpdateMarkup(c *gin.Context) {
	var req struct {
		DefaultMarkup float64 `json:"default_markup"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || !model.ValidMarkup(req.DefaultMarkup) {
		common.ApiError(c, model.ErrInvalidMarkup)
		return
	}
	markup := req.DefaultMarkup
	if err := applyResellerProfile(c.GetInt("id"), adminResellerRequest{DefaultMarkup: &markup}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"default_markup": markup})
}

func customerView(user *model.User) gin.H {
	return gin.H{
		"id":            user.Id,
		"username":      user.Username,
		"display_name":  user.DisplayName,
		"status":        user.Status,
		"group":         user.Group,
		"quota":         user.Quota,
		"used_quota":    user.UsedQuota,
		"request_count": user.RequestCount,
		"markup":        user.Markup,
		"remark":        user.Remark,
		"created_at":    user.CreatedAt,
	}
}

func ResellerListCustomers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var users []model.User
	var total int64
	query := model.DB.Model(&model.User{}).Omit("password").Where("reseller_id = ?", c.GetInt("id"))
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		query = query.Where("username LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&users).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(users))
	for i := range users {
		items = append(items, customerView(&users[i]))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func loadCustomer(c *gin.Context) (*model.User, bool) {
	id, ok := paramId(c, "id")
	if !ok {
		return nil, false
	}
	user, err := model.GetResellerCustomer(c.GetInt("id"), id)
	if err != nil {
		// Same response for "missing" and "belongs to another reseller" (no IDOR oracle).
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "customer not found"})
		return nil, false
	}
	return user, true
}

func ResellerGetCustomer(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	common.ApiSuccess(c, customerView(user))
}

type resellerCustomerRequest struct {
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	DisplayName string   `json:"display_name"`
	Group       string   `json:"group"`
	Markup      *float64 `json:"markup"`
	ClearMarkup bool     `json:"clear_markup"`
	Remark      *string  `json:"remark"`
}

// AdminCreateResellerCustomer creates a customer under reseller :id (same body as ResellerCreateCustomer).
func AdminCreateResellerCustomer(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	if _, ok := loadReseller(c, id); !ok {
		return
	}
	var req resellerCustomerRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	createResellerCustomer(c, id, req)
}

func ResellerCreateCustomer(c *gin.Context) {
	resellerId := c.GetInt("id")
	var req resellerCustomerRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	createResellerCustomer(c, resellerId, req)
}

// createResellerCustomer validates and creates a customer for the given reseller.
func createResellerCustomer(c *gin.Context, resellerId int, req resellerCustomerRequest) {
	cfg, err := model.GetResellerPricingConfig(resellerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	group := req.Group
	if group == "" && len(cfg.Groups) > 0 {
		group = cfg.Groups[0]
		if reseller, ok := loadReseller(c, resellerId); ok && cfg.AllowsGroup(reseller.Group) {
			group = reseller.Group
		} else if !ok {
			return
		}
	}
	if !cfg.AllowsGroup(group) {
		common.ApiError(c, model.ErrGroupNotAllowed)
		return
	}
	if req.Markup != nil && !model.ValidMarkup(*req.Markup) {
		common.ApiError(c, model.ErrInvalidMarkup)
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || len(req.Password) < 8 {
		common.ApiErrorMsg(c, "username and password (min 8 chars) are required")
		return
	}
	user := model.User{
		Username:    username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Role:        common.RoleCommonUser,
		Group:       group,
		ResellerId:  resellerId,
		Markup:      req.Markup,
	}
	if user.DisplayName == "" {
		user.DisplayName = username
	}
	if req.Remark != nil {
		user.Remark = *req.Remark
	}
	if err := common.Validate.Struct(&user); err != nil {
		common.ApiError(c, err)
		return
	}
	// max_customers check + insert + zero quota in one transaction.
	if err := model.CreateResellerCustomer(resellerId, &user); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, customerView(&user))
}

func ResellerUpdateCustomer(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	var req resellerCustomerRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	fields := map[string]any{}
	if req.Group != "" {
		cfg, err := model.GetResellerPricingConfig(user.ResellerId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if !cfg.AllowsGroup(req.Group) {
			common.ApiError(c, model.ErrGroupNotAllowed)
			return
		}
		fields["group"] = req.Group
	}
	if req.ClearMarkup {
		fields["markup"] = nil
	} else if req.Markup != nil {
		if !model.ValidMarkup(*req.Markup) {
			common.ApiError(c, model.ErrInvalidMarkup)
			return
		}
		fields["markup"] = *req.Markup
	}
	if req.DisplayName != "" {
		fields["display_name"] = req.DisplayName
	}
	if req.Remark != nil {
		fields["remark"] = *req.Remark
	}
	if err := model.UpdateResellerCustomerFields(user.Id, fields); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, _ := model.GetResellerCustomer(user.ResellerId, user.Id)
	common.ApiSuccess(c, customerView(updated))
}

func ResellerSetCustomerStatus(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	var req struct {
		Status int `json:"status"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || (req.Status != common.UserStatusEnabled && req.Status != common.UserStatusDisabled) {
		common.ApiErrorMsg(c, "invalid status")
		return
	}
	if err := model.SetUserStatusAndRevoke(user.Id, req.Status); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"status": req.Status})
}

func resellerAdjustCustomer(c *gin.Context, sign int) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	var req resellerQuotaRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	amount, err := req.quotaAmount()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	kind := model.ResellerLedgerCustomerCredit
	if sign < 0 {
		kind = model.ResellerLedgerCustomerDebit
	}
	entry := model.ResellerLedger{
		ResellerId: user.ResellerId,
		CustomerId: user.Id,
		Kind:       kind,
		MoneyUsd:   req.MoneyUsd,
		OperatorId: c.GetInt("id"),
		Remark:     req.Remark,
	}
	// Atomic transfer: credit → +customer −reseller; debit/clawback → −customer +reseller.
	// Insufficient reseller funds (credit) or customer funds (debit) reject with no writes.
	if err := model.TransferResellerCustomerQuota(user.ResellerId, user.Id, sign*amount, entry); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(user.Id, model.LogTypeResellerCredit, common.NewMessage("Reseller adjusted balance by {{quota}}", map[string]any{"quota": signedQuota(sign, amount)}))
	common.ApiSuccess(c, gin.H{"quota_delta": sign * amount})
}

func ResellerCreditCustomer(c *gin.Context) { resellerAdjustCustomer(c, 1) }
func ResellerDebitCustomer(c *gin.Context)  { resellerAdjustCustomer(c, -1) }

type markupRequest struct {
	ModelName string  `json:"model_name"`
	Markup    float64 `json:"markup"`
}

func decodeMarkup(c *gin.Context) (markupRequest, bool) {
	var req markupRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return req, false
	}
	if m := c.Param("model"); m != "" {
		req.ModelName = m
	}
	if strings.TrimSpace(req.ModelName) == "" {
		common.ApiErrorMsg(c, "model_name is required")
		return req, false
	}
	if !model.ValidMarkup(req.Markup) {
		common.ApiError(c, model.ErrInvalidMarkup)
		return req, false
	}
	return req, true
}

func ResellerListModelMarkups(c *gin.Context) {
	var rows []model.ResellerModelMarkup
	if err := model.DB.Where("reseller_id = ?", c.GetInt("id")).Order("model_name").Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func ResellerUpsertModelMarkup(c *gin.Context) {
	req, ok := decodeMarkup(c)
	if !ok {
		return
	}
	if err := model.UpsertResellerModelMarkup(c.GetInt("id"), req.ModelName, req.Markup); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, req)
}

func ResellerDeleteModelMarkup(c *gin.Context) {
	if err := model.DeleteResellerModelMarkup(c.GetInt("id"), c.Param("model")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ResellerListCustomerModelMarkups(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	var rows []model.CustomerModelMarkup
	if err := model.DB.Where("reseller_id = ? AND customer_id = ?", user.ResellerId, user.Id).Order("model_name").Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func ResellerUpsertCustomerModelMarkup(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	req, ok := decodeMarkup(c)
	if !ok {
		return
	}
	if err := model.UpsertCustomerModelMarkup(user.ResellerId, user.Id, req.ModelName, req.Markup); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, req)
}

func ResellerDeleteCustomerModelMarkup(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	if err := model.DeleteCustomerModelMarkup(user.ResellerId, user.Id, c.Param("model")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ResellerListRedemptions(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var rows []model.Redemption
	var total int64
	query := model.DB.Model(&model.Redemption{}).Where("reseller_id = ?", c.GetInt("id"))
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&rows).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func ResellerCreateRedemptions(c *gin.Context) {
	resellerId := c.GetInt("id")
	profile, err := model.GetResellerProfile(resellerId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !profile.AllowRedemption {
		common.ApiErrorMsg(c, "redemption codes are disabled for this reseller")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Quota       int    `json:"quota"`
		Count       int    `json:"count"`
		ExpiredTime int64  `json:"expired_time"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	if req.Count <= 0 || req.Count > 100 {
		common.ApiErrorMsg(c, "count must be between 1 and 100")
		return
	}
	keys := make([]string, 0, req.Count)
	for range req.Count {
		redemption := model.Redemption{
			UserId:      resellerId,
			Name:        req.Name,
			Key:         common.GetUUID(),
			CreatedTime: common.GetTimestamp(),
			Quota:       req.Quota,
			ExpiredTime: req.ExpiredTime,
			ResellerId:  resellerId,
		}
		if err := redemption.Insert(); err != nil {
			common.ApiError(c, err)
			return
		}
		keys = append(keys, redemption.Key)
	}
	common.ApiSuccess(c, keys)
}

func ResellerDeleteRedemption(c *gin.Context) {
	id, ok := paramId(c, "id")
	if !ok {
		return
	}
	result := model.DB.Where("id = ? AND reseller_id = ?", id, c.GetInt("id")).Delete(&model.Redemption{})
	if result.Error != nil {
		common.ApiError(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "redemption not found"})
		return
	}
	common.ApiSuccess(c, nil)
}

func ResellerGetLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	var logs []model.Log
	var total int64
	query := model.LOG_DB.Model(&model.Log{}).Where("reseller_id = ? AND type = ?", c.GetInt("id"), model.LogTypeConsume)
	if err := query.Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := query.Order("id desc").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&logs).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		items = append(items, gin.H{
			"id": l.Id, "created_at": l.CreatedAt, "user_id": l.UserId, "username": l.Username,
			"model_name": l.ModelName, "token_name": l.TokenName, "group": l.Group,
			"prompt_tokens": l.PromptTokens, "completion_tokens": l.CompletionTokens,
			"quota": l.Quota, "reseller_cost_quota": l.ResellerCostQuota, "profit_quota": l.Quota - l.ResellerCostQuota,
		})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func ResellerGrossProfitReport(c *gin.Context) {
	grossProfit(c, c.GetInt("id"))
}

func ResellerGetLedger(c *gin.Context) {
	// Session reseller only. Query reseller_id / customer_id are ignored.
	ledgerPage(c, c.GetInt("id"), 0)
}

func CustomerGetLedger(c *gin.Context) {
	// Session user only. Query reseller_id / customer_id are ignored.
	ledgerPage(c, 0, c.GetInt("id"))
}

// ledgerPage lists balance-ledger rows for one role scope.
// A positive resellerId returns that reseller's own and customer rows.
// A positive customerId returns only that customer's rows.
func ledgerPage(c *gin.Context, resellerId, customerId int) {
	if resellerId <= 0 && customerId <= 0 {
		common.ApiErrorMsg(c, "unauthorized")
		return
	}
	pageInfo := common.GetPageQuery(c)
	kind := c.Query("kind")
	var (
		rows  []model.ResellerLedger
		total int64
		err   error
	)
	if period := strings.TrimSpace(c.Query("period")); period != "" {
		var from, toEx int64
		from, toEx, err = model.ParseInvoicePeriod(period)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		rows, total, err = model.ListResellerLedgerRange(from, toEx, resellerId, customerId, kind, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	} else if customerId > 0 {
		rows, total, err = model.ListResellerLedgerRange(0, 0, 0, customerId, kind, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	} else {
		rows, total, err = model.GetResellerLedger(resellerId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), kind)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

// ResellerDeleteCustomer disables (soft "deletes") a customer and its tokens.
func ResellerDeleteCustomer(c *gin.Context) {
	user, ok := loadCustomer(c)
	if !ok {
		return
	}
	if err := model.DisableResellerCustomer(user.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": user.Id, "status": common.UserStatusDisabled})
}

// ---------------------------------------------------------------------------
// reseller-managed customer tokens (scoped: customer must belong to me)
// ---------------------------------------------------------------------------

type resellerTokenRequest struct {
	Name           string  `json:"name"`
	RemainQuota    *int    `json:"remain_quota"`
	UnlimitedQuota *bool   `json:"unlimited_quota"`
	ExpiredTime    *int64  `json:"expired_time"`
	Group          *string `json:"group"`
	ModelLimits    *string `json:"model_limits"`
	AllowIps       *string `json:"allow_ips"`
	Status         *int    `json:"status"`
}

func resellerTokenView(t *model.Token) gin.H {
	return gin.H{
		"id": t.Id, "user_id": t.UserId, "name": t.Name, "key": t.GetMaskedKey(), "status": t.Status,
		"created_time": t.CreatedTime, "accessed_time": t.AccessedTime, "expired_time": t.ExpiredTime,
		"remain_quota": t.RemainQuota, "unlimited_quota": t.UnlimitedQuota, "used_quota": t.UsedQuota,
		"group": t.Group, "model_limits_enabled": t.ModelLimitsEnabled, "model_limits": t.ModelLimits,
	}
}

// applyResellerTokenRequest validates and copies request fields onto t.
func applyResellerTokenRequest(c *gin.Context, customer *model.User, t *model.Token, req resellerTokenRequest) bool {
	if req.Name != "" {
		if len(req.Name) > 50 {
			common.ApiErrorMsg(c, "token name is too long")
			return false
		}
		t.Name = req.Name
	}
	if req.UnlimitedQuota != nil {
		t.UnlimitedQuota = *req.UnlimitedQuota
	}
	if req.RemainQuota != nil {
		if *req.RemainQuota < 0 {
			common.ApiErrorMsg(c, "remain_quota cannot be negative")
			return false
		}
		t.RemainQuota = *req.RemainQuota
	}
	if req.ExpiredTime != nil {
		t.ExpiredTime = *req.ExpiredTime
	}
	if req.Group != nil {
		group := strings.TrimSpace(*req.Group)
		if group == "auto" {
			common.ApiErrorMsg(c, "auto group is not supported for reseller-managed tokens")
			return false
		}
		if group != "" {
			cfg, err := model.GetResellerPricingConfig(customer.ResellerId)
			if err != nil {
				common.ApiError(c, err)
				return false
			}
			if !cfg.AllowsGroup(group) {
				common.ApiError(c, model.ErrGroupNotAllowed)
				return false
			}
		}
		t.Group = group
	}
	if req.ModelLimits != nil {
		t.ModelLimits = *req.ModelLimits
		t.ModelLimitsEnabled = strings.TrimSpace(*req.ModelLimits) != ""
	}
	if req.AllowIps != nil {
		ips := *req.AllowIps
		t.AllowIps = &ips
	}
	if req.Status != nil {
		if *req.Status != common.TokenStatusEnabled && *req.Status != common.TokenStatusDisabled {
			common.ApiErrorMsg(c, "invalid status")
			return false
		}
		t.Status = *req.Status
	}
	return true
}

func loadCustomerToken(c *gin.Context) (*model.User, *model.Token, bool) {
	customer, ok := loadCustomer(c)
	if !ok {
		return nil, nil, false
	}
	tokenId, ok := paramId(c, "token_id")
	if !ok {
		return nil, nil, false
	}
	token, err := model.GetTokenByIds(tokenId, customer.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "token not found"})
		return nil, nil, false
	}
	return customer, token, true
}

func ResellerListCustomerTokens(c *gin.Context) {
	customer, ok := loadCustomer(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(customer.Id, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(customer.Id)
	items := make([]gin.H, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, resellerTokenView(t))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func ResellerCreateCustomerToken(c *gin.Context) {
	customer, ok := loadCustomer(c)
	if !ok {
		return
	}
	var req resellerTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	count, err := model.CountUserTokens(customer.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= operation_setting.GetMaxUserTokens() {
		common.ApiErrorMsg(c, "maximum number of tokens reached")
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorMsg(c, "failed to generate token")
		return
	}
	now := common.GetTimestamp()
	token := model.Token{UserId: customer.Id, Name: "default", Key: key, Status: common.TokenStatusEnabled,
		CreatedTime: now, AccessedTime: now, ExpiredTime: -1}
	if !applyResellerTokenRequest(c, customer, &token, req) {
		return
	}
	if err := token.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	view := resellerTokenView(&token)
	view["key"] = token.GetFullKey() // shown once so the reseller can hand it over
	common.ApiSuccess(c, view)
}

func ResellerUpdateCustomerToken(c *gin.Context) {
	customer, token, ok := loadCustomerToken(c)
	if !ok {
		return
	}
	var req resellerTokenRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	if !applyResellerTokenRequest(c, customer, token, req) {
		return
	}
	if err := token.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, resellerTokenView(token))
}

func ResellerDeleteCustomerToken(c *gin.Context) {
	customer, token, ok := loadCustomerToken(c)
	if !ok {
		return
	}
	if err := model.DeleteTokenById(token.Id, customer.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": token.Id})
}

// AdminReconcileResellers lists (read-only) negative reseller balances and
// consume logs with an inconsistent reseller leg. ?from=<unix> limits the log scan.
func AdminReconcileResellers(c *gin.Context) {
	from, _ := strconv.ParseInt(c.Query("from"), 10, 64)
	issues, err := model.ReconcileResellers(from)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"issues": issues, "count": len(issues)})
}
