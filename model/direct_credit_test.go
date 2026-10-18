package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Direct-credit billing mode: financial edge cases.

func setupDirectCredit(t *testing.T, markup, costFactor float64) {
	t.Helper()
	setupProfitCredit(t, true)
	clean := func() {
		DB.Exec("DELETE FROM reseller_profiles WHERE user_id = ?", pcReseller)
		DB.Exec("DELETE FROM direct_credit_grants")
	}
	clean()
	t.Cleanup(clean)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: pcReseller, DefaultMarkup: markup, DefaultCostFactor: costFactor}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Updates(map[string]any{"billing_mode": BillingModeDirectCredit, "quota": 0}).Error)
}

func dcLedger(t *testing.T, kind string) (n int64, sum int64) { return pcLedger(t, kind) }

func TestDirectCreditChrisExample(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	res, err := DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "top-up", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(100), res.ResellerCost)
	assert.Equal(t, int64(20), res.ResellerProfit)
	assert.Equal(t, int64(20_000_000), res.AdminMicro)
	assert.Equal(t, int64(20), res.AdminCredited)
	assert.Equal(t, 120, pcQuota(t, pcCustomer))
	assert.Equal(t, 900, pcQuota(t, pcReseller), "reseller pays cost only")
	assert.Equal(t, 1020, pcQuota(t, pcRoot), "admin profit to root")

	var r RevenueRecord
	require.NoError(t, DB.First(&r, res.RevenueId).Error)
	assert.Equal(t, RevenueKindDCTopup, r.Kind)
	assert.Equal(t, RevenuePayerDirectCredit, r.PayerType)
	assert.Equal(t, int64(120), r.CustomerPrice)
	assert.Equal(t, int64(80_000_000), r.BaseQuotaMicro)
	assert.Zero(t, r.ResellerCredited, "reseller profit is never credited to the wallet")
	assert.True(t, r.IdentityHolds())

	n, sum := dcLedger(t, ResellerLedgerCustomerCredit)
	assert.Equal(t, int64(1), n)
	assert.Equal(t, int64(120), sum)
	_, sum = dcLedger(t, ResellerLedgerDirectCreditCost)
	assert.Equal(t, int64(-100), sum)
	_, sum = dcLedger(t, ResellerLedgerDirectCreditResellerProfit)
	assert.Equal(t, int64(20), sum)
	_, sum = dcLedger(t, ResellerLedgerDirectCreditAdminProfit)
	assert.Equal(t, int64(20), sum)
	var hidden int64
	DB.Model(&ResellerLedger{}).Where("customer_id = ? AND kind <> ?", pcCustomer, ResellerLedgerCustomerCredit).Count(&hidden)
	assert.Zero(t, hidden, "cost/profit rows never visible to the customer")
}

func TestDirectCreditRoundingAndTinyAmounts(t *testing.T) {
	for _, tc := range []struct {
		x                    int64
		markup, cf           float64
		cost, profit, admMic int64
	}{
		{1, 1.2, 1.25, 1, 0, 0},            // 0.83 -> 1 (min 1), base round(0.8)=1
		{7, 1.2, 1.25, 6, 1, 1_000_000},    // 5.83 -> 6, base round(4.8)=5
		{3, 2, 1, 2, 1, 0},                 // 1.5 rounds half up -> 2
		{1, 1000, 1.2, 1, 0, 0},            // tiny: cost min 1
		{120, 1.2, 1, 100, 20, 0},          // cf 1: no admin margin
		{100, 1, 0.8, 100, 0, -25_000_000}, // cf < 1: admin margin negative (base 125)
	} {
		cost, profit, _, adm, err := DirectCreditSplit(tc.x, tc.markup, tc.cf)
		require.NoError(t, err)
		assert.Equal(t, tc.cost, cost, "%+v", tc)
		assert.Equal(t, tc.profit, profit, "%+v", tc)
		assert.Equal(t, tc.admMic, adm, "%+v", tc)
	}
	_, _, _, _, err := DirectCreditSplit(0, 1.2, 1)
	assert.ErrorIs(t, err, ErrDirectCreditAmount)
	_, _, _, _, err = DirectCreditSplit(10, 1.2, 0)
	assert.ErrorIs(t, err, ErrInvalidCostFactor)
	_, _, _, _, err = DirectCreditSplit(10, 0, 1)
	assert.ErrorIs(t, err, ErrInvalidMarkup)
}

func assertNoDirectCreditWrites(t *testing.T) {
	t.Helper()
	assert.Equal(t, 0, pcQuota(t, pcCustomer))
	assert.Equal(t, 1000, pcQuota(t, pcRoot))
	var n int64
	DB.Model(&DirectCreditGrant{}).Count(&n)
	assert.Zero(t, n)
	DB.Model(&RevenueRecord{}).Count(&n)
	assert.Zero(t, n)
	DB.Model(&ResellerLedger{}).Count(&n)
	assert.Zero(t, n)
}

func TestDirectCreditInsufficientResellerBalance(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcReseller).Update("quota", 99).Error)
	_, err := DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	assert.ErrorIs(t, err, ErrAmountExceedsResellerBalance)
	assert.Equal(t, 99, pcQuota(t, pcReseller))
	assertNoDirectCreditWrites(t)
	// Exactly enough succeeds.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcReseller).Update("quota", 100).Error)
	_, err = DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, pcQuota(t, pcReseller))
}

func TestDirectCreditFailsClosedOnFactorsAndMode(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	require.NoError(t, DB.Model(&ResellerProfile{}).Where("user_id = ?", pcReseller).Update("default_cost_factor", 0).Error)
	_, err := DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	assert.ErrorIs(t, err, ErrInvalidCostFactor)
	require.NoError(t, DB.Model(&ResellerProfile{}).Where("user_id = ?", pcReseller).Update("default_cost_factor", -1).Error)
	_, err = DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	assert.ErrorIs(t, err, ErrInvalidCostFactor)
	DB.Exec("DELETE FROM reseller_profiles WHERE user_id = ?", pcReseller)
	_, err = DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	assert.ErrorIs(t, err, ErrDirectCreditNoResellerSetup, "no silent default cost factor")
	assertNoDirectCreditWrites(t)

	require.NoError(t, DB.Create(&ResellerProfile{UserId: pcReseller, DefaultMarkup: 1.2, DefaultCostFactor: 1.25}).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Update("billing_mode", "").Error)
	_, err = DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	assert.ErrorIs(t, err, ErrNotDirectCreditCustomer)
	_, err = DirectCreditCustomer(pcReseller+100, pcCustomer, 120, 1, "", nil)
	assert.Error(t, err, "other reseller's customer")
	_, err = DirectCreditCustomer(pcReseller, pcCustomer, 0, 1, "", nil)
	assert.ErrorIs(t, err, ErrDirectCreditAmount)
	assertNoDirectCreditWrites(t)
}

func TestDirectCreditCustomerMarkupOverride(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	m := 1.5
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Update("markup", m).Error)
	res, err := DirectCreditCustomer(pcReseller, pcCustomer, 150, 1, "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(100), res.ResellerCost, "customer markup wins over reseller default")
}

func TestBillingModeSwitchRequiresZeroBalance(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	require.NoError(t, SetCustomerBillingMode(pcCustomer, "direct_credit"), "same mode is a no-op")
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Update("quota", 5).Error)
	assert.ErrorIs(t, SetCustomerBillingMode(pcCustomer, "markup"), ErrBillingModeSwitchNonZero)
	var u User
	DB.Select("billing_mode").First(&u, pcCustomer)
	assert.Equal(t, BillingModeDirectCredit, u.BillingMode)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Update("quota", 0).Error)
	require.NoError(t, SetCustomerBillingMode(pcCustomer, "markup"))
	DB.Select("billing_mode").First(&u, pcCustomer)
	assert.Equal(t, BillingModeMarkup, u.BillingMode)
	assert.ErrorIs(t, SetCustomerBillingMode(pcCustomer, "bogus"), ErrInvalidBillingMode)
	assert.Error(t, SetCustomerBillingMode(pcRoot, "direct_credit"), "only reseller customers")
}

func TestDirectCreditReversalProportionalAndNeverNegative(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	_, err := DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	require.NoError(t, err)
	// Partial: 60 of 120 -> cost 50, admin 10.
	res, err := ReverseDirectCredit(pcReseller, pcCustomer, 60, 1, "remove", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(-50), res.ResellerCost)
	assert.Equal(t, int64(-10), res.ResellerProfit)
	assert.Equal(t, int64(-10), res.AdminCredited)
	assert.Equal(t, 60, pcQuota(t, pcCustomer))
	assert.Equal(t, 950, pcQuota(t, pcReseller))
	assert.Equal(t, 1010, pcQuota(t, pcRoot))
	// Rest: exact remainder.
	_, err = ReverseDirectCredit(pcReseller, pcCustomer, 60, 1, "", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, pcQuota(t, pcCustomer))
	assert.Equal(t, 1000, pcQuota(t, pcReseller), "fully restored")
	assert.Equal(t, 1000, pcQuota(t, pcRoot))
	_, err = ReverseDirectCredit(pcReseller, pcCustomer, 1, 1, "", nil)
	assert.ErrorIs(t, err, ErrInsufficientQuota)
	var revs []RevenueRecord
	DB.Where("kind = ?", RevenueKindDCReversal).Find(&revs)
	require.Len(t, revs, 2)
	for _, r := range revs {
		assert.True(t, r.IdentityHolds())
	}
	var sum struct{ P, C, A int64 }
	DB.Model(&RevenueRecord{}).Select("SUM(customer_price) AS p, SUM(reseller_cost) AS c, SUM(admin_profit_micro) AS a").Scan(&sum)
	assert.Equal(t, struct{ P, C, A int64 }{0, 0, 0}, sum, "top-up + reversals net to zero")
}

func TestDirectCreditReversalOddSplitsAndLots(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	_, err := DirectCreditCustomer(pcReseller, pcCustomer, 7, 1, "", nil) // cost 6, admin 1
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ResellerProfile{}).Where("user_id = ?", pcReseller).Update("default_markup", 2).Error)
	_, err = DirectCreditCustomer(pcReseller, pcCustomer, 10, 1, "", nil) // cost 5, base 4, admin 1
	require.NoError(t, err)
	assert.Equal(t, 1000-11, pcQuota(t, pcReseller))
	// 11 = 10 from newest lot (exact rest) + 1 from the first lot at its ratio.
	res, err := ReverseDirectCredit(pcReseller, pcCustomer, 11, 1, "", nil)
	require.NoError(t, err)
	assert.Equal(t, int64(-(5 + 1)), res.ResellerCost, "1/7 of 6 rounds to 1")
	for i := 0; i < 6; i++ {
		_, err = ReverseDirectCredit(pcReseller, pcCustomer, 1, 1, "", nil)
		require.NoError(t, err)
	}
	assert.Equal(t, 0, pcQuota(t, pcCustomer))
	assert.Equal(t, 1000, pcQuota(t, pcReseller), "rounding never leaks across lots")
	assert.Equal(t, 1000, pcQuota(t, pcRoot))
	var g []DirectCreditGrant
	DB.Find(&g)
	for _, x := range g {
		assert.Zero(t, x.Remaining)
		assert.Zero(t, x.CostRemaining)
		assert.Zero(t, x.AdminMicroRemaining)
	}
}

func TestDirectCreditReversalLimitsAndRootFloor(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	_, err := DirectCreditCustomer(pcReseller, pcCustomer, 120, 1, "", nil)
	require.NoError(t, err)
	// Customer spent 100 (calls): only 20 left to remove.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Update("quota", 20).Error)
	_, err = ReverseDirectCredit(pcReseller, pcCustomer, 21, 1, "", nil)
	assert.ErrorIs(t, err, ErrInsufficientQuota)
	// Extra admin-granted balance is not reversible direct credit.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcCustomer).Update("quota", 500).Error)
	_, err = ReverseDirectCredit(pcReseller, pcCustomer, 121, 1, "", nil)
	assert.ErrorIs(t, err, ErrDirectCreditExceedsGrants)
	// Root cannot cover the admin-profit reversal -> whole reversal rejected.
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pcRoot).Update("quota", 5).Error)
	_, err = ReverseDirectCredit(pcReseller, pcCustomer, 120, 1, "", nil)
	assert.ErrorIs(t, err, ErrInsufficientQuota)
	assert.Equal(t, 500, pcQuota(t, pcCustomer), "rolled back")
	assert.Equal(t, 900, pcQuota(t, pcReseller))
	assert.Equal(t, 5, pcQuota(t, pcRoot))
	var g DirectCreditGrant
	DB.First(&g)
	assert.Equal(t, int64(120), g.Remaining)
}

func TestDirectCreditCallsWriteNoRevenueRow(t *testing.T) {
	setupDirectCredit(t, 1.2, 1.25)
	c := revCtx(RevenueContext{PayerType: RevenuePayerNone})
	_, ok := GetRevenueContext(c)
	assert.False(t, ok, "none context is never set")
	RecordConsumeLog(c, pcCustomer, RecordConsumeLogParams{ModelName: "m", Quota: 50})
	rc := RevenueContext{PayerType: RevenuePayerNone}
	createLogWithRevenue(&Log{UserId: pcCustomer, Type: LogTypeConsume, Quota: 50}, &rc, true)
	assert.Zero(t, countRevenue(t))
	var n int64
	DB.Model(&Log{}).Where("other LIKE ?", "%revenue_ctx%").Count(&n)
	assert.Zero(t, n, "reconciler has nothing to rebuild")
	created, err := ReconcileRevenueFromLogs(0, 100)
	require.NoError(t, err)
	assert.Zero(t, created)
}
