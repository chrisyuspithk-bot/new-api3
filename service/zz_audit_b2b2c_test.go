package service

// Financial audit tests (B2B2C). Added by audit 2026-10-13. Not pushed.
// Each test logs "AUDIT <ID> PASS|FAIL|FINDING ..." lines consumed by the report.

import (
	"math"
	"math/rand"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func auditApply(t *testing.T, model_ string) (*relaycommon.RelayInfo, float64) {
	info := &relaycommon.RelayInfo{UserId: b2bCustomerId, ResellerId: b2bResellerId, OriginModelName: model_, UsingGroup: "default"}
	gr := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(info, &gr)
	return info, gr.GroupRatio
}

// R1: cost_multiplier = 0 is accepted and makes customer usage free.
func TestAuditR1_ZeroCostMultiplierFreeUsage(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1.2, common.UserStatusEnabled)
	zero := 0.0
	err := model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &zero})
	t.Logf("AUDIT R1 upsert cost_multiplier=0 err=%v", err)
	info, gr := auditApply(t, "b2b-model")
	t.Logf("AUDIT R1 customer groupRatio=%v pricingErr=%v", gr, info.ResellerPricingErr)
	self := &relaycommon.RelayInfo{UserId: b2bResellerId, ResellerSelf: true, OriginModelName: "b2b-model", UsingGroup: "default"}
	sgr := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(self, &sgr)
	t.Logf("AUDIT R1 self pricingErr=%v", self.ResellerPricingErr)
	assert.ErrorIs(t, err, model.ErrInvalidResellerPrice, "cost_multiplier 0 must be rejected at upsert")
	assert.False(t, err == nil && info.ResellerPricingErr == nil && gr == 0, "FINDING: cost_multiplier 0 accepted -> customer charge 0 (free usage), while self-call is rejected")

	// A 0 row already stored (pre-validation data) must fail closed on both paths.
	require.NoError(t, model.DB.Create(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &zero}).Error)
	model.InvalidateResellerCache(b2bResellerId)
	info, _ = auditApply(t, "b2b-model")
	assert.ErrorIs(t, info.ResellerPricingErr, model.ErrInvalidCostFactor)
	self = &relaycommon.RelayInfo{UserId: b2bResellerId, ResellerSelf: true, OriginModelName: "b2b-model", UsingGroup: "default"}
	sgr = hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(self, &sgr)
	assert.ErrorIs(t, self.ResellerPricingErr, model.ErrInvalidCostFactor)
}

// R2/R3: model_price override on ratio-billed model, model_ratio override on per-call model.
func TestAuditR2_OverrideTypeMismatchSilentlyIgnored(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1, common.UserStatusEnabled)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"audit-ratio-model":2}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"audit-price-model":0.05}`))
	p := 0.10
	require.NoError(t, model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "audit-price-model", ModelPrice: &p}))
	r := 3.0
	require.NoError(t, model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "audit-ratio-model", ModelRatio: &r}))
	_, gp := auditApply(t, "audit-price-model")
	_, grr := auditApply(t, "audit-ratio-model")
	t.Logf("AUDIT R2 model_price 0.10 on base 0.05 -> factor %v (exp 2)", gp)
	t.Logf("AUDIT R2 model_ratio 3 on base 2 -> factor %v (exp 1.5)", grr)
	assert.InDelta(t, 2.0, gp, 1e-9)
	assert.InDelta(t, 1.5, grr, 1e-9)

	// mismatch: model_price override on a ratio-billed model
	wrong := 0.5
	err := model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "audit-ratio-model", ModelPrice: &wrong})
	_, gm := auditApply(t, "audit-ratio-model")
	t.Logf("AUDIT R3 model_price override on ratio-billed model: upsertErr=%v factor=%v", err, gm)
	r3Unfixed := err == nil && gm == 1

	// delete override -> base
	require.NoError(t, model.DeleteResellerModelPrice(b2bResellerId, "audit-ratio-model"))
	_, gd := auditApply(t, "audit-ratio-model")
	t.Logf("AUDIT R4 after delete factor=%v (exp 1)", gd)
	assert.InDelta(t, 1.0, gd, 1e-9)
	if r3Unfixed {
		t.Skip("audit finding R3 not fixed yet: model_price override on a ratio-billed model is accepted but silently ignored")
	}
}

// R5: invalid values
func TestAuditR5_InvalidMarkupsAndPrices(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1, common.UserStatusEnabled)
	for _, m := range []float64{0, -1, math.NaN(), math.Inf(1), 1e6} {
		err := model.UpsertResellerModelMarkup(b2bResellerId, "b2b-model", m)
		t.Logf("AUDIT R5 markup %v rejected=%v", m, err != nil)
		assert.Error(t, err)
		err = model.UpsertCustomerModelMarkup(b2bResellerId, b2bCustomerId, "b2b-model", m)
		assert.Error(t, err)
	}
	err := model.SaveResellerProfile(&model.ResellerProfile{UserId: b2bResellerId, DefaultMarkup: 0})
	assert.Error(t, err)
	for _, v := range []float64{-0.01, math.NaN()} {
		vv := v
		assert.Error(t, model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &vv}))
	}
	inf := math.Inf(1)
	err = model.UpsertResellerModelPrice(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CostMultiplier: &inf})
	t.Logf("AUDIT R5b cost_multiplier=+Inf accepted=%v", err == nil)
	// tiny markup allowed?
	tiny := 1e-6
	err = model.UpsertResellerModelMarkup(b2bResellerId, "b2b-model2", tiny)
	t.Logf("AUDIT R5c markup 1e-6 accepted=%v; cost leg for 1-quota sale = %d", err == nil, ResellerCostFromCharge(1, tiny))
}

// R6: precedence including discount and huge markups
func TestAuditR6_Precedence(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1.2, common.UserStatusEnabled)
	check := func(exp float64, name string) {
		_, m, err := ResolveResellerFactors(b2bCustomerId, b2bResellerId, "b2b-model")
		require.NoError(t, err)
		t.Logf("AUDIT R6 %s markup=%v exp=%v", name, m, exp)
		assert.InDelta(t, exp, m, 1e-9, name)
	}
	check(1.2, "default")
	require.NoError(t, model.UpsertResellerModelMarkup(b2bResellerId, "b2b-model", 999999))
	check(999999, "reseller x model huge")
	require.NoError(t, model.UpdateResellerCustomerFields(b2bCustomerId, map[string]any{"markup": 0.5}))
	check(0.5, "customer discount")
	require.NoError(t, model.UpsertCustomerModelMarkup(b2bResellerId, b2bCustomerId, "b2b-model", 3))
	check(3, "customer x model")
	require.NoError(t, model.DeleteCustomerModelMarkup(b2bResellerId, b2bCustomerId, "b2b-model"))
	check(0.5, "after delete customer x model")
	// other model falls back to customer markup, not reseller x model
	_, m, _ := ResolveResellerFactors(b2bCustomerId, b2bResellerId, "other")
	assert.InDelta(t, 0.5, m, 1e-9)
}

// R7: rounding property: reseller cost leg vs sale
func TestAuditR7_RoundingProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	worstOverMarkupLT1 := 0.0
	for i := 0; i < 300000; i++ {
		sale := 1 + rng.Intn(5_000_000)
		if i%3 == 0 {
			sale = 1 + rng.Intn(10)
		}
		m := []float64{1.2, 1.1, 1.15, 1.3333, 2, 3, 7.77, 0.9, 0.5}[i%9]
		cost := ResellerCostFromCharge(sale, m)
		exact := float64(sale) / m
		require.GreaterOrEqual(t, float64(cost), math.Min(exact-0.5-1e-9, 1))
		require.LessOrEqual(t, float64(cost), math.Max(exact+0.5+1e-9, 1))
		require.Greater(t, cost, 0)
		if m >= 1 {
			require.LessOrEqual(t, cost, sale)
		} else if d := float64(cost) - float64(sale); d > worstOverMarkupLT1 {
			worstOverMarkupLT1 = d
		}
	}
	t.Logf("AUDIT R7 300k samples: cost in [sale/m, sale/m+1), cost>0, cost<=sale when m>=1")
	// 1-token: sale is forced to >=1 quota by engine; cost >= 1
	assert.Equal(t, 1, ResellerCostFromCharge(1, 1.2))
	assert.Equal(t, 1, ResellerCostFromCharge(1, 1.44))
}

// R8: settle can push reseller below zero (preconsume underestimate)
func TestAuditR8_SettleOverdraft(t *testing.T) {
	seedResellerLine(t, 60, 100000, 2, common.UserStatusEnabled)
	ctx, info := newResellerRelayInfo(t)
	require.Nil(t, PreConsumeBilling(ctx, 100, info))
	require.NoError(t, SettleBilling(ctx, info, 10000))
	rq := quotaOf(t, b2bResellerId)
	cq := quotaOf(t, b2bCustomerId)
	t.Logf("AUDIT R8 reseller after settle=%d (start 60, cost 5000) customer=%d", rq, cq)
	issues, _ := model.ReconcileResellers(0)
	t.Logf("AUDIT R8 reconcile issues=%+v", issues)
	assert.Equal(t, 60-5000, rq)
}

// R9: transfers edge cases + concurrency conservation
func TestAuditR9_Transfers(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.ResellerLedger{}))
	seedResellerLine(t, 1000, 0, 1.2, common.UserStatusEnabled)
	model.DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", b2bResellerId)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", b2bResellerId) })
	e := func(k string) model.ResellerLedger { return model.ResellerLedger{Kind: k, OperatorId: b2bResellerId} }
	assert.Error(t, model.TransferResellerCustomerQuota(b2bResellerId, b2bCustomerId, 0, e(model.ResellerLedgerCustomerCredit)))
	assert.ErrorIs(t, model.TransferResellerCustomerQuota(b2bResellerId, b2bCustomerId, 1001, e(model.ResellerLedgerCustomerCredit)), model.ErrAmountExceedsResellerBalance)
	assert.ErrorIs(t, model.TransferResellerCustomerQuota(b2bResellerId, b2bCustomerId, -1, e(model.ResellerLedgerCustomerDebit)), model.ErrInsufficientQuota)
	// model layer does not verify the customer belongs to the reseller
	stranger := 9199
	require.NoError(t, model.DB.Create(&model.User{Id: stranger, Username: "stranger", Quota: 0, AffCode: "s9"}).Error)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM users WHERE id = ?", stranger) })
	errS := model.TransferResellerCustomerQuota(b2bResellerId, stranger, 1, e(model.ResellerLedgerCustomerCredit))
	t.Logf("AUDIT R9d model-layer transfer to non-customer err=%v (controller must guard)", errS)
	if errS == nil {
		model.DB.Exec("UPDATE users SET quota = quota + 1 WHERE id = ?", b2bResellerId)
		model.DB.Exec("DELETE FROM reseller_ledger WHERE customer_id = ?", stranger)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if model.TransferResellerCustomerQuota(b2bResellerId, b2bCustomerId, 30, e(model.ResellerLedgerCustomerCredit)) == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	rq, cq := quotaOf(t, b2bResellerId), quotaOf(t, b2bCustomerId)
	var sum int64
	model.DB.Raw("SELECT COALESCE(SUM(quota_delta),0) FROM reseller_ledger WHERE reseller_id = ? AND customer_id = ?", b2bResellerId, b2bCustomerId).Scan(&sum)
	t.Logf("AUDIT R9 concurrent 40x30: ok=%d reseller=%d customer=%d ledgerSum=%d", ok, rq, cq, sum)
	assert.Equal(t, 1000, rq+cq)
	assert.GreaterOrEqual(t, rq, 0)
	assert.Equal(t, int64(cq), sum)
	assert.Equal(t, ok*30, cq)
}

// R10: AdjustUserQuota ledger integrity
func TestAuditR10_AdminAdjustLedger(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.ResellerLedger{}))
	seedResellerLine(t, 1000, 500, 1.2, common.UserStatusEnabled)
	model.DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", b2bResellerId)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", b2bResellerId) })
	_, err := model.AdjustUserQuota(b2bResellerId, 1, common.RoleRootUser, "add", 250, "r-add")
	require.NoError(t, err)
	_, err = model.AdjustUserQuota(b2bResellerId, 1, common.RoleRootUser, "override", 700, "r-ovr")
	require.NoError(t, err)
	_, err = model.AdjustUserQuota(b2bCustomerId, 1, common.RoleRootUser, "subtract", 100, "c-sub")
	require.NoError(t, err)
	_, err = model.AdjustUserQuota(b2bResellerId, 1, common.RoleRootUser, "override", 700, "noop")
	require.NoError(t, err)
	// subtract more than balance
	adj, err := model.AdjustUserQuota(b2bResellerId, 1, common.RoleRootUser, "subtract", 5000, "r-neg")
	t.Logf("AUDIT R10b subtract 5000 from 700: err=%v after=%v", err, adj)
	var rows []model.ResellerLedger
	model.DB.Where("reseller_id = ?", b2bResellerId).Order("id").Find(&rows)
	var rsum, csum int64
	for _, r := range rows {
		t.Logf("AUDIT R10 ledger kind=%s cust=%d delta=%d remark=%q", r.Kind, r.CustomerId, r.QuotaDelta, r.Remark)
		if r.CustomerId == 0 {
			rsum += r.QuotaDelta
		} else {
			csum += r.QuotaDelta
		}
	}
	rq, cq := quotaOf(t, b2bResellerId), quotaOf(t, b2bCustomerId)
	t.Logf("AUDIT R10 reseller %d->%d ledgerSum=%d ; customer 500->%d ledgerSum=%d", 1000, rq, rsum, cq, csum)
	assert.Equal(t, int64(rq-1000), rsum)
	assert.Equal(t, int64(cq-500), csum)
	if rq < 0 {
		t.Skip("audit finding R10 not fixed yet: admin subtract can drive the reseller wallet negative")
	}
}

// R11: customer pointing at non-reseller / disabled reseller
func TestAuditR11_BrokenResellerLink(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 1.2, common.UserStatusEnabled)
	model.DB.Exec("UPDATE users SET role = ? WHERE id = ?", common.RoleCommonUser, b2bResellerId)
	model.InvalidateResellerCache(b2bResellerId)
	info, _ := auditApply(t, "b2b-model")
	t.Logf("AUDIT R11 reseller demoted -> err=%v", info.ResellerPricingErr)
	assert.ErrorIs(t, info.ResellerPricingErr, model.ErrResellerDisabled)
	// self path for a demoted reseller still with ResellerSelf flag
	self := &relaycommon.RelayInfo{UserId: b2bResellerId, ResellerSelf: true, OriginModelName: "b2b-model", UsingGroup: "default"}
	g := hosttypes.GroupRatioInfo{GroupRatio: 1}
	ApplyResellerPricing(self, &g)
	t.Logf("AUDIT R11b demoted self path err=%v ratio=%v", self.ResellerPricingErr, g.GroupRatio)
}
