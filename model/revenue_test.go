package model

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetRevenueTables(t *testing.T) {
	t.Helper()
	clean := func() {
		DB.Exec("DELETE FROM revenue_records")
		DB.Exec("DELETE FROM revenue_monthly_snapshots")
		DB.Exec("DELETE FROM logs")
		DB.Exec("DELETE FROM users WHERE id IN (801, 802, 803, 804)")
		revenueInsertHook = nil
		pendingRevenueMu.Lock()
		pendingRevenue = nil
		pendingRevenueMu.Unlock()
	}
	clean()
	t.Cleanup(clean)
}

func revCtx(rc RevenueContext) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/", nil)
	SetRevenueContext(c, rc)
	return c
}

func countRevenue(t *testing.T) int64 {
	var n int64
	require.NoError(t, DB.Model(&RevenueRecord{}).Count(&n).Error)
	return n
}

func TestRevenueTxFailureFallsBackAndStillWritesExactlyOnce(t *testing.T) {
	resetRevenueTables(t)
	calls := 0
	revenueInsertHook = func(*RevenueRecord) error {
		calls++
		if calls <= 2 { // in-tx attempt + first separate attempt fail
			return errors.New("injected")
		}
		return nil
	}
	c := revCtx(RevenueContext{PayerType: RevenuePayerDirect})
	RecordConsumeLog(c, 801, RecordConsumeLogParams{ModelName: "m", Quota: 10})
	var logs int64
	DB.Model(&Log{}).Count(&logs)
	assert.Equal(t, int64(1), logs, "log rolled back once, then written alone")
	assert.Equal(t, int64(1), countRevenue(t), "revenue row written by retry")
	var r RevenueRecord
	require.NoError(t, DB.First(&r).Error)
	assert.Greater(t, r.LogId, int64(0))
}

func TestRevenuePersistentFailureQueuesThenFlushes(t *testing.T) {
	resetRevenueTables(t)
	revenueInsertHook = func(*RevenueRecord) error { return errors.New("db down") }
	c := revCtx(RevenueContext{PayerType: RevenuePayerDirect})
	RecordConsumeLog(c, 801, RecordConsumeLogParams{ModelName: "m", Quota: 10})
	assert.Equal(t, int64(0), countRevenue(t))
	assert.Equal(t, 1, PendingRevenueCount())
	assert.Equal(t, 1, FlushPendingRevenue(), "still failing: stays queued")
	revenueInsertHook = nil
	assert.Equal(t, 0, FlushPendingRevenue())
	assert.Equal(t, int64(1), countRevenue(t))
	var r RevenueRecord
	require.NoError(t, DB.First(&r).Error)
	assert.Equal(t, RevenueSourceRetry, r.Source)
	assert.Equal(t, 0, FlushPendingRevenue(), "idempotent")
	assert.Equal(t, int64(1), countRevenue(t))
}

func TestRevenueReconcilerRebuildsMissingRowsIdempotently(t *testing.T) {
	resetRevenueTables(t)
	c := revCtx(RevenueContext{PayerType: RevenuePayerCustomer, ResellerId: 802, CostFactor: 1.25, Markup: 1.2})
	c.Set(ContextKeyLogResellerId, 802)
	c.Set(ContextKeyLogResellerCostQuota, 125)
	RecordConsumeLog(c, 801, RecordConsumeLogParams{ModelName: "m", Quota: 150})
	var orig RevenueRecord
	require.NoError(t, DB.First(&orig).Error)
	// Simulate a lost revenue write.
	DB.Exec("DELETE FROM revenue_records")
	n, err := ReconcileRevenueFromLogs(0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	var rebuilt RevenueRecord
	require.NoError(t, DB.First(&rebuilt).Error)
	assert.Equal(t, RevenueSourceReconcile, rebuilt.Source)
	assert.Equal(t, orig.LogId, rebuilt.LogId)
	assert.Equal(t, orig.CustomerPrice, rebuilt.CustomerPrice)
	assert.Equal(t, orig.ResellerCost, rebuilt.ResellerCost)
	assert.Equal(t, orig.BaseQuotaMicro, rebuilt.BaseQuotaMicro)
	assert.Equal(t, orig.AdminProfitMicro, rebuilt.AdminProfitMicro)
	n, err = ReconcileRevenueFromLogs(0, 100)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "second run creates nothing")
}

func seedRevenueRow(t *testing.T, createdAt int64, rc RevenueContext, user int, kind string, charge, cost int64, key string) {
	t.Helper()
	r := BuildRevenueRecord(rc, user, "m", kind, charge, cost)
	r.CreatedAt = createdAt
	r.DedupeKey = key
	require.NoError(t, insertRevenueTx(DB, &r))
}

func TestRevenueStatementsAggregateAndReconcile(t *testing.T) {
	resetRevenueTables(t)
	for _, u := range []User{{Id: 801, Username: "custA", AffCode: "ra1"}, {Id: 802, Username: "resellerR", AffCode: "ra2"}, {Id: 803, Username: "direct1", AffCode: "ra3"}, {Id: 804, Username: "resellerS", AffCode: "ra4"}} {
		require.NoError(t, DB.Create(&u).Error)
	}
	from, _, err := ParseInvoicePeriod("2026-09")
	require.NoError(t, err)
	cust := RevenueContext{PayerType: RevenuePayerCustomer, ResellerId: 802, CostFactor: 1.25, Markup: 1.2}
	self := RevenueContext{PayerType: RevenuePayerResellerSelf, ResellerId: 802, CostFactor: 1.25}
	other := RevenueContext{PayerType: RevenuePayerCustomer, ResellerId: 804, CostFactor: 1.1, Markup: 1.5}
	seedRevenueRow(t, from+10, cust, 801, RevenueKindConsume, 150, 125, "a")
	seedRevenueRow(t, from+20, cust, 801, RevenueKindConsume, 81, 68, "b")
	seedRevenueRow(t, from+30, cust, 801, RevenueKindRefund, 150, 125, "c")
	seedRevenueRow(t, from+40, self, 802, RevenueKindConsume, 48, 48, "d")
	seedRevenueRow(t, from+50, RevenueContext{PayerType: RevenuePayerDirect}, 803, RevenueKindConsume, 500, 0, "e")
	seedRevenueRow(t, from+60, other, 999, RevenueKindConsume, 330, 220, "f")
	seedRevenueRow(t, from-1, cust, 801, RevenueKindConsume, 9999, 8333, "prev-month")

	adm, err := ComputeRevenueStatement("2026-09", RevenueScopeAdmin, 0)
	require.NoError(t, err)
	assert.True(t, adm.Reconciliation.IdentityOK, "Σcharge = Σbase + Σadmin_profit + Σreseller_profit")
	assert.True(t, adm.Reconciliation.SectionsOK)
	assert.Equal(t, int64(150+81-150+48+500+330), adm.Totals.CustomerPrice)
	assert.Equal(t, int64(125+68-125+220), adm.CustomerSum.ResellerCost)
	assert.Equal(t, int64(68+48+500+220), adm.Totals.AdminRevenue)
	assert.Equal(t, int64(48), adm.SelfSum.CustomerPrice)
	assert.Equal(t, int64(500), adm.DirectSum.CustomerPrice)
	require.Len(t, adm.ByReseller, 2)
	assert.Equal(t, "resellerS", adm.ByReseller[0].Name)
	assert.Equal(t, int64(110), adm.ByReseller[0].ResellerProfit)
	assert.Equal(t, int64(13), adm.ByReseller[1].ResellerProfit, "81-68")
	assert.Equal(t, int64(2), adm.ByReseller[1].Requests)
	assert.Equal(t, int64(1), adm.ByReseller[1].Refunds)
	require.Len(t, adm.ResellerSelf, 1)
	require.Len(t, adm.Direct, 1)
	assert.Equal(t, "direct1", adm.Direct[0].Name)
	assert.NotZero(t, adm.Totals.AdminProfitMicro)

	rs, err := ComputeRevenueStatement("2026-09", RevenueScopeReseller, 802)
	require.NoError(t, err)
	assert.Equal(t, int64(81+48), rs.Totals.CustomerPrice, "only its customers + own use")
	assert.Equal(t, int64(13), rs.CustomerSum.ResellerProfit)
	assert.Equal(t, int64(48), rs.SelfSum.ResellerCost)
	assert.Zero(t, rs.Totals.BaseQuotaMicro, "admin list price hidden from reseller")
	assert.Zero(t, rs.Totals.AdminProfitMicro, "admin margin hidden from reseller")
	require.Len(t, rs.ByCustomer, 1)
	assert.Equal(t, "custA", rs.ByCustomer[0].Name)
	assert.Equal(t, "resellerR", rs.ResellerName)
	assert.True(t, rs.Reconciliation.IdentityOK)
}

func TestRevenueClosedMonthIsFrozenAndLateRowsFlagged(t *testing.T) {
	resetRevenueTables(t)
	from, _, _ := ParseInvoicePeriod("2026-08")
	cust := RevenueContext{PayerType: RevenuePayerCustomer, ResellerId: 802, CostFactor: 1.2, Markup: 1.2}
	seedRevenueRow(t, from+10, cust, 801, RevenueKindConsume, 144, 120, "x1")
	st, err := GetRevenueStatement("2026-08", RevenueScopeAdmin, 0)
	require.NoError(t, err)
	assert.True(t, st.Frozen)
	assert.Equal(t, int64(144), st.Totals.CustomerPrice)
	// A row backfilled after the freeze does not change the frozen invoice ...
	seedRevenueRow(t, from+20, cust, 801, RevenueKindConsume, 72, 60, "x2")
	again, err := GetRevenueStatement("2026-08", RevenueScopeAdmin, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(144), again.Totals.CustomerPrice, "frozen totals unchanged")
	assert.Equal(t, st.Checksum, again.Checksum)
	// ... but is flagged for the auditor.
	assert.Equal(t, int64(1), again.LateRows)
	assert.True(t, again.ChecksumMismatch)

	// The current month is never frozen.
	now := time.Now().In(InvoicePeriodLocation()).Format("2006-01")
	cur, err := GetRevenueStatement(now, RevenueScopeAdmin, 0)
	require.NoError(t, err)
	assert.False(t, cur.Frozen)
	var snaps int64
	DB.Model(&RevenueMonthlySnapshot{}).Where("period = ?", now).Count(&snaps)
	assert.Zero(t, snaps)
}

func TestRevenueStatementCrossChecksLogs(t *testing.T) {
	resetRevenueTables(t)
	c := revCtx(RevenueContext{PayerType: RevenuePayerCustomer, ResellerId: 802, CostFactor: 1.25, Markup: 1.2})
	c.Set(ContextKeyLogResellerId, 802)
	c.Set(ContextKeyLogResellerCostQuota, 125)
	RecordConsumeLog(c, 801, RecordConsumeLogParams{ModelName: "m", Quota: 150})
	period := time.Now().In(InvoicePeriodLocation()).Format("2006-01")
	st, err := ComputeRevenueStatement(period, RevenueScopeAdmin, 0)
	require.NoError(t, err)
	require.True(t, st.Reconciliation.LogCompared)
	assert.Zero(t, st.Reconciliation.LogChargeDiff)
	assert.Zero(t, st.Reconciliation.LogResellerCostDif)
	// A log whose revenue row went missing shows up as a difference.
	DB.Exec("DELETE FROM revenue_records")
	st, err = ComputeRevenueStatement(period, RevenueScopeAdmin, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(-150), st.Reconciliation.LogChargeDiff)
	_ = fmt.Sprint(common.QuotaPerUnit)
}

func TestRevenueInvalidScopeRejected(t *testing.T) {
	_, err := ComputeRevenueStatement("2026-09", "bogus", 0)
	assert.Error(t, err)
	_, err = ComputeRevenueStatement("2026-09", RevenueScopeReseller, 0)
	assert.Error(t, err)
	_, err = ComputeRevenueStatement("2026-13", RevenueScopeAdmin, 0)
	assert.Error(t, err)
}
