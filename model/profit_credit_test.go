package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Financial-audit tests for profit crediting.

const (
	pcRoot     = 811
	pcReseller = 812
	pcCustomer = 813
)

func setupProfitCredit(t *testing.T, withRoot bool) {
	t.Helper()
	resetRevenueTables(t)
	clean := func() {
		DB.Exec("DELETE FROM users WHERE id IN (?, ?, ?)", pcRoot, pcReseller, pcCustomer)
		DB.Exec("DELETE FROM reseller_ledger")
		DB.Exec("DELETE FROM profit_credit_accumulators")
	}
	clean()
	t.Cleanup(clean)
	// Only our root may exist as root user in this test DB.
	DB.Exec("UPDATE users SET role = ? WHERE role = ?", common.RoleAdminUser, common.RoleRootUser)
	if withRoot {
		require.NoError(t, DB.Create(&User{Id: pcRoot, Username: "pcroot", Role: common.RoleRootUser, Status: 1, Quota: 1000, AffCode: "pc0"}).Error)
	}
	require.NoError(t, DB.Create(&User{Id: pcReseller, Username: "pcres", Role: common.RoleResellerUser, Status: 1, Quota: 1000, AffCode: "pc1"}).Error)
	require.NoError(t, DB.Create(&User{Id: pcCustomer, Username: "pccus", Role: common.RoleCommonUser, Status: 1, Quota: 1000, ResellerId: pcReseller, AffCode: "pc2"}).Error)
}

func pcQuota(t *testing.T, id int) int {
	t.Helper()
	var u User
	require.NoError(t, DB.Select("quota").First(&u, id).Error)
	return u.Quota
}

func pcLedger(t *testing.T, kind string) (n int64, sum int64) {
	t.Helper()
	DB.Model(&ResellerLedger{}).Where("kind = ?", kind).Count(&n)
	DB.Model(&ResellerLedger{}).Where("kind = ?", kind).Select("COALESCE(SUM(quota_delta),0)").Scan(&sum)
	return
}

func pcAcc(t *testing.T) ProfitCreditAccumulator {
	t.Helper()
	var a ProfitCreditAccumulator
	DB.Where("user_id = ?", pcRoot).Limit(1).Find(&a)
	return a
}

func customerCtx() RevenueContext {
	return RevenueContext{PayerType: RevenuePayerCustomer, ResellerId: pcReseller, CostFactor: 1.2, Markup: 1.2, ProfitCredit: true}
}

func TestProfitCreditCustomerCallCreditsBothInSameTx(t *testing.T) {
	setupProfitCredit(t, true)
	c := revCtx(customerCtx())
	c.Set(ContextKeyLogResellerId, pcReseller)
	c.Set(ContextKeyLogResellerCostQuota, 73)
	RecordConsumeLog(c, pcCustomer, RecordConsumeLogParams{ModelName: "m", Quota: 87})

	var r RevenueRecord
	require.NoError(t, DB.First(&r).Error)
	assert.Equal(t, int64(14), r.ResellerProfit)
	assert.Equal(t, int64(12_583_333), r.AdminProfitMicro)
	assert.Equal(t, RevenueCreditStateCredited, r.CreditState)
	assert.Equal(t, int64(14), r.ResellerCredited)
	assert.Equal(t, int64(12), r.AdminCredited, "whole units of 12.583333")
	assert.Equal(t, 1000+14, pcQuota(t, pcReseller))
	assert.Equal(t, 1000+12, pcQuota(t, pcRoot))
	acc := pcAcc(t)
	assert.Equal(t, int64(12_583_333), acc.AccruedMicro)
	assert.Equal(t, int64(12), acc.CreditedQuota)
	assert.Equal(t, int64(583_333), acc.RemainderMicro(), "carried, not lost")

	var led []ResellerLedger
	require.NoError(t, DB.Order("id").Find(&led).Error)
	require.Len(t, led, 2)
	assert.Equal(t, ResellerLedgerResellerProfitCredit, led[0].Kind)
	assert.Equal(t, fmt.Sprintf("rev:%d", r.Id), led[0].RefId, "ledger references the revenue row")
	assert.Zero(t, led[0].CustomerId, "never visible in customer-scoped ledgers")
	assert.Zero(t, led[1].CustomerId)
	assert.Equal(t, ResellerLedgerAdminProfitCredit, led[1].Kind)
	assert.Equal(t, pcRoot, led[1].ResellerId)
	assert.Equal(t, int64(12), led[1].QuotaDelta)
}

func TestProfitCreditRemainderAccumulatesAcrossCalls(t *testing.T) {
	setupProfitCredit(t, true)
	// 7 rows of 0.3 quota admin margin = 2.1 -> 2 credited, 0.1 carried.
	for i := 0; i < 7; i++ {
		rec := RevenueRecord{DedupeKey: fmt.Sprintf("t:%d", i), Kind: RevenueKindConsume, PayerType: RevenuePayerResellerSelf,
			UserId: pcReseller, ResellerId: pcReseller, CustomerPrice: 1, ResellerCost: 1, AdminRevenue: 1,
			BaseQuotaMicro: 700_000, AdminProfitMicro: 300_000, creditEligible: true}
		require.NoError(t, insertRevenueTx(DB, &rec))
	}
	acc := pcAcc(t)
	assert.Equal(t, int64(2_100_000), acc.AccruedMicro)
	assert.Equal(t, int64(2), acc.CreditedQuota)
	assert.Equal(t, int64(100_000), acc.RemainderMicro())
	assert.Equal(t, 1002, pcQuota(t, pcRoot))
	n, sum := pcLedger(t, ResellerLedgerAdminProfitCredit)
	assert.Equal(t, int64(2), n, "a ledger entry only when a whole unit is crossed")
	assert.Equal(t, int64(2), sum)
	var credited int64
	DB.Model(&RevenueRecord{}).Select("COALESCE(SUM(admin_credited),0)").Scan(&credited)
	assert.Equal(t, int64(2), credited, "row-level credited sums to the ledger")
	n2, _ := pcLedger(t, ResellerLedgerResellerProfitCredit)
	assert.Zero(t, n2, "self-calls have no reseller profit")
}

func TestProfitCreditIdempotentSameDedupeKey(t *testing.T) {
	setupProfitCredit(t, true)
	mk := func() RevenueRecord {
		r := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindConsume, 87, 73)
		r.DedupeKey = "log:4242"
		return r
	}
	for i := 0; i < 3; i++ { // live tx + retry queue + reconciler all reuse the key
		r := mk()
		require.NoError(t, insertRevenueTx(DB, &r))
	}
	enqueuePendingRevenue(mk())
	FlushPendingRevenue()
	assert.Equal(t, int64(1), countRevenue(t))
	assert.Equal(t, 1014, pcQuota(t, pcReseller), "credited exactly once")
	assert.Equal(t, 1012, pcQuota(t, pcRoot))
	n, _ := pcLedger(t, ResellerLedgerResellerProfitCredit)
	assert.Equal(t, int64(1), n)
}

func TestProfitCreditRefundReversesWithNegativeLedger(t *testing.T) {
	setupProfitCredit(t, true)
	c := revCtx(customerCtx())
	c.Set(ContextKeyLogResellerId, pcReseller)
	c.Set(ContextKeyLogResellerCostQuota, 73)
	RecordConsumeLog(c, pcCustomer, RecordConsumeLogParams{ModelName: "m", Quota: 87})
	ref := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindRefund, -87, -73)
	ref.DedupeKey = "refund:1"
	require.NoError(t, insertRevenueTx(DB, &ref))
	assert.Equal(t, 1000, pcQuota(t, pcReseller), "reseller profit reversed")
	assert.Equal(t, 1000, pcQuota(t, pcRoot), "admin whole units reversed")
	acc := pcAcc(t)
	assert.Equal(t, int64(0), acc.AccruedMicro)
	assert.Equal(t, int64(0), acc.CreditedQuota)
	_, rsum := pcLedger(t, ResellerLedgerResellerProfitCredit)
	_, asum := pcLedger(t, ResellerLedgerAdminProfitCredit)
	assert.Zero(t, rsum)
	assert.Zero(t, asum)
	var led ResellerLedger
	require.NoError(t, DB.Where("kind = ? AND quota_delta < 0", ResellerLedgerResellerProfitCredit).First(&led).Error)
	assert.Equal(t, int64(-14), led.QuotaDelta)
}

func TestProfitCreditNegativeRemainderFloors(t *testing.T) {
	setupProfitCredit(t, true)
	// +0.4 then refund -0.6: accrued -0.2 -> floor -1 credited, remainder 0.8.
	for i, m := range []int64{400_000, -600_000} {
		kind := RevenueKindConsume
		if m < 0 {
			kind = RevenueKindRefund
		}
		rec := RevenueRecord{DedupeKey: fmt.Sprintf("n:%d", i), Kind: kind, PayerType: RevenuePayerResellerSelf,
			UserId: pcReseller, ResellerId: pcReseller, AdminProfitMicro: m, creditEligible: true}
		require.NoError(t, insertRevenueTx(DB, &rec))
	}
	acc := pcAcc(t)
	assert.Equal(t, int64(-200_000), acc.AccruedMicro)
	assert.Equal(t, int64(-1), acc.CreditedQuota)
	assert.Equal(t, int64(800_000), acc.RemainderMicro())
	assert.Equal(t, 999, pcQuota(t, pcRoot))
}

func TestProfitCreditNotEligibleBooksWithoutCredit(t *testing.T) {
	setupProfitCredit(t, true)
	rc := customerCtx()
	rc.ProfitCredit = false // e.g. pre-deploy task refund or violation fee
	ref := BuildRevenueRecord(rc, pcCustomer, "m", RevenueKindRefund, -87, -73)
	ref.DedupeKey = "legacy:1"
	require.NoError(t, insertRevenueTx(DB, &ref))
	var r RevenueRecord
	require.NoError(t, DB.First(&r).Error)
	assert.Equal(t, RevenueCreditStateNotCredited, r.CreditState)
	assert.Equal(t, 1000, pcQuota(t, pcReseller), "never reverse a credit that was never given")
	assert.Equal(t, 1000, pcQuota(t, pcRoot))
	n, _ := pcLedger(t, ResellerLedgerResellerProfitCredit)
	assert.Zero(t, n)
}

func TestProfitCreditLedgerFailureRollsBackRowAndCredits(t *testing.T) {
	setupProfitCredit(t, true)
	require.NoError(t, DB.Migrator().RenameTable("reseller_ledger", "reseller_ledger_off"))
	t.Cleanup(func() { _ = DB.Migrator().RenameTable("reseller_ledger_off", "reseller_ledger") })
	rec := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindConsume, 87, 73)
	rec.DedupeKey = "fail:1"
	require.Error(t, insertRevenueTx(DB, &rec))
	assert.Zero(t, countRevenue(t), "row rolled back with its credit")
	assert.Equal(t, 1000, pcQuota(t, pcReseller), "no partial credit")
	require.NoError(t, DB.Migrator().RenameTable("reseller_ledger_off", "reseller_ledger"))
	rec2 := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindConsume, 87, 73)
	rec2.DedupeKey = "fail:1"
	require.NoError(t, insertRevenueTx(DB, &rec2), "retry with the same key succeeds once")
	assert.Equal(t, 1014, pcQuota(t, pcReseller))
}

func TestProfitCreditNoRootAccruesForLater(t *testing.T) {
	setupProfitCredit(t, false)
	rec := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindConsume, 87, 73)
	rec.DedupeKey = "noroot:1"
	require.NoError(t, insertRevenueTx(DB, &rec))
	var r RevenueRecord
	require.NoError(t, DB.First(&r).Error)
	assert.Equal(t, RevenueCreditStateNoRoot, r.CreditState)
	assert.Equal(t, 1014, pcQuota(t, pcReseller), "reseller still credited")
	var a ProfitCreditAccumulator
	DB.Where("user_id = 0").Find(&a)
	assert.Equal(t, int64(12_583_333), a.AccruedMicro, "admin share accrued, not lost")
	DB.Exec("DELETE FROM profit_credit_accumulators WHERE user_id = 0")
}

func TestProfitCreditConcurrentNoLostOrDoubleCredit(t *testing.T) {
	setupProfitCredit(t, true)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindConsume, 87, 73)
			rec.DedupeKey = fmt.Sprintf("c:%d", i%20) // each key raced twice
			_ = insertRevenueTx(DB, &rec)
		}(i)
	}
	wg.Wait()
	assert.Equal(t, int64(20), countRevenue(t))
	assert.Equal(t, 1000+20*14, pcQuota(t, pcReseller))
	acc := pcAcc(t)
	assert.Equal(t, int64(20*12_583_333), acc.AccruedMicro)
	assert.Equal(t, floorDivMicro(acc.AccruedMicro), acc.CreditedQuota)
	assert.Equal(t, 1000+int(acc.CreditedQuota), pcQuota(t, pcRoot))
	_, asum := pcLedger(t, ResellerLedgerAdminProfitCredit)
	assert.Equal(t, acc.CreditedQuota, asum, "ledger == accumulator == wallet")
}

func TestProfitCreditSkippedWhenResellerLegFailed(t *testing.T) {
	setupProfitCredit(t, true)
	for i, cost := range []int64{0, 50} { // failed leg (0) and short leg
		r := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindConsume, 87, cost)
		r.DedupeKey = fmt.Sprintf("leg:%d", i)
		require.NoError(t, insertRevenueTx(DB, &r))
		assert.Equal(t, RevenueCreditStateLegMismatch, r.CreditState)
	}
	assert.Equal(t, 1000, pcQuota(t, pcReseller), "no payout of the whole price")
	assert.Equal(t, 1000, pcQuota(t, pcRoot), "root not debited")
	var n int64
	DB.Model(&ResellerLedger{}).Count(&n)
	assert.Zero(t, n)
	// Refund with a failed leg is not reversed either.
	r := BuildRevenueRecord(customerCtx(), pcCustomer, "m", RevenueKindRefund, 87, 0)
	r.DedupeKey = "leg:refund"
	require.NoError(t, insertRevenueTx(DB, &r))
	assert.Equal(t, 1000, pcQuota(t, pcReseller))
}

func TestProfitCreditInvalidFactorsNotCredited(t *testing.T) {
	setupProfitCredit(t, true)
	rc := customerCtx()
	rc.Markup = 0
	r := BuildRevenueRecord(rc, pcCustomer, "m", RevenueKindConsume, 87, 0)
	r.DedupeKey = "inv:1"
	require.NoError(t, insertRevenueTx(DB, &r))
	assert.Equal(t, RevenueCreditStateNotCredited, r.CreditState)
	assert.Equal(t, 1000, pcQuota(t, pcReseller))
}
