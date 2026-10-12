package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func migrateInvoiceTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Invoice{}, &Log{}, &User{}))
}

func TestParseInvoicePeriodHKT(t *testing.T) {
	from, to, err := ParseInvoicePeriod("2026-09")
	require.NoError(t, err)
	loc, _ := time.LoadLocation("Asia/Hong_Kong")
	start := time.Unix(from, 0).In(loc)
	end := time.Unix(to, 0).In(loc)
	assert.Equal(t, 2026, start.Year())
	assert.Equal(t, time.September, start.Month())
	assert.Equal(t, 1, start.Day())
	assert.Equal(t, 0, start.Hour())
	assert.Equal(t, time.October, end.Month())
	assert.Equal(t, 1, end.Day())
	_, _, err = ParseInvoicePeriod("2026-9")
	assert.ErrorIs(t, err, ErrInvalidInvoicePeriod)
	_, _, err = ParseInvoicePeriod("not-a-month")
	assert.ErrorIs(t, err, ErrInvalidInvoicePeriod)
}

func TestGenerateMonthlyInvoicesAggregatesAndUpserts(t *testing.T) {
	migrateInvoiceTables(t)
	const (
		rid  = 9401
		cid1 = 9402
		cid2 = 9403
	)
	require.NoError(t, DB.Where("id IN ?", []int{rid, cid1, cid2}).Delete(&User{}).Error)
	require.NoError(t, DB.Create(&User{Id: rid, Username: "inv-reseller", DisplayName: "Brand R", Role: common.RoleResellerUser, AffCode: "ir1"}).Error)
	require.NoError(t, DB.Create(&User{Id: cid1, Username: "inv-c1", ResellerId: rid, AffCode: "ic1"}).Error)
	require.NoError(t, DB.Create(&User{Id: cid2, Username: "inv-c2", ResellerId: rid, AffCode: "ic2"}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM invoices WHERE reseller_id = ? OR period = ?", rid, "2026-08")
		DB.Exec("DELETE FROM logs WHERE reseller_id = ?", rid)
		DB.Exec("DELETE FROM users WHERE id IN ?", []int{rid, cid1, cid2})
	})

	from, toEx, err := ParseInvoicePeriod("2026-08")
	require.NoError(t, err)
	mid := from + 86400

	logs := []Log{
		{UserId: cid1, Username: "inv-c1", ModelName: "gpt-a", Type: LogTypeConsume, Quota: 1000, ResellerId: rid, ResellerCostQuota: 500, CreatedAt: mid},
		{UserId: cid1, Username: "inv-c1", ModelName: "gpt-a", Type: LogTypeConsume, Quota: 500, ResellerId: rid, ResellerCostQuota: 250, CreatedAt: mid + 10},
		{UserId: cid1, Username: "inv-c1", ModelName: "gpt-a", Type: LogTypeRefund, Quota: 200, ResellerId: rid, ResellerCostQuota: 100, CreatedAt: mid + 20},
		{UserId: cid2, Username: "inv-c2", ModelName: "gpt-b", Type: LogTypeConsume, Quota: 3000, ResellerId: rid, ResellerCostQuota: 1000, CreatedAt: mid + 30},
		// Outside period — ignored
		{UserId: cid1, Username: "inv-c1", ModelName: "gpt-a", Type: LogTypeConsume, Quota: 9999, ResellerId: rid, ResellerCostQuota: 9999, CreatedAt: toEx + 10},
		// Direct user — ignored
		{UserId: 1, Username: "direct", ModelName: "gpt-a", Type: LogTypeConsume, Quota: 100, ResellerId: 0, ResellerCostQuota: 0, CreatedAt: mid},
	}
	for i := range logs {
		require.NoError(t, LOG_DB.Create(&logs[i]).Error)
	}

	res, err := GenerateMonthlyInvoices("2026-08", 1)
	require.NoError(t, err)
	assert.Equal(t, "2026-08", res.Period)
	assert.Equal(t, 2, res.CustomerInvoices)
	assert.Equal(t, 1, res.ResellerInvoices)
	assert.True(t, res.AdminRollup)

	// customer1: charge 1000+500-200=1300, cost 500+250-100=650
	var c1 Invoice
	require.NoError(t, DB.Where("period = ? AND scope = ? AND customer_id = ?", "2026-08", InvoiceScopeCustomer, cid1).First(&c1).Error)
	assert.Equal(t, int64(2), c1.Requests)
	assert.Equal(t, int64(1300), c1.ChargeQuota)
	assert.Equal(t, int64(650), c1.CostQuota)
	assert.Equal(t, int64(650), c1.ProfitQuota)
	assert.InDelta(t, 1300/common.QuotaPerUnit, c1.ChargeUsd, 1e-9)
	assert.Equal(t, "Brand R", c1.ResellerName)
	assert.Equal(t, "inv-c1", c1.CustomerName)
	items := c1.InvoiceLineItems()
	require.Len(t, items, 1)
	assert.Equal(t, "gpt-a", items[0].Model)

	var c2 Invoice
	require.NoError(t, DB.Where("period = ? AND scope = ? AND customer_id = ?", "2026-08", InvoiceScopeCustomer, cid2).First(&c2).Error)
	assert.Equal(t, int64(3000), c2.ChargeQuota)
	assert.Equal(t, int64(1000), c2.CostQuota)

	var rInv Invoice
	require.NoError(t, DB.Where("period = ? AND scope = ? AND reseller_id = ?", "2026-08", InvoiceScopeReseller, rid).First(&rInv).Error)
	assert.Equal(t, int64(4300), rInv.ChargeQuota)
	assert.Equal(t, int64(1650), rInv.CostQuota)
	assert.Equal(t, int64(3), rInv.Requests)

	var admin Invoice
	require.NoError(t, DB.Where("period = ? AND scope = ?", "2026-08", InvoiceScopeAdminRollup).First(&admin).Error)
	assert.Equal(t, int64(4300), admin.ChargeQuota)
	assert.Equal(t, int64(1650), admin.CostQuota)

	// Upsert: regenerate with extra log should update totals, not duplicate.
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: cid2, Username: "inv-c2", ModelName: "gpt-b", Type: LogTypeConsume,
		Quota: 100, ResellerId: rid, ResellerCostQuota: 40, CreatedAt: mid + 40,
	}).Error)
	res2, err := GenerateMonthlyInvoices("2026-08", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, res2.CustomerInvoices)
	var count int64
	DB.Model(&Invoice{}).Where("period = ?", "2026-08").Count(&count)
	assert.Equal(t, int64(4), count) // 2 customer + 1 reseller + 1 admin

	require.NoError(t, DB.Where("period = ? AND scope = ? AND customer_id = ?", "2026-08", InvoiceScopeCustomer, cid2).First(&c2).Error)
	assert.Equal(t, int64(3100), c2.ChargeQuota)
	assert.Equal(t, int64(1040), c2.CostQuota)
	assert.Equal(t, 2, c2.GeneratedBy)
}

func TestListInvoicesRestrictReseller(t *testing.T) {
	migrateInvoiceTables(t)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM invoices WHERE period = ?", "2025-01")
	})
	require.NoError(t, DB.Create(&Invoice{Period: "2025-01", Scope: InvoiceScopeCustomer, ResellerId: 1, CustomerId: 10, ChargeQuota: 1}).Error)
	require.NoError(t, DB.Create(&Invoice{Period: "2025-01", Scope: InvoiceScopeReseller, ResellerId: 1, CustomerId: 0, ChargeQuota: 1}).Error)
	require.NoError(t, DB.Create(&Invoice{Period: "2025-01", Scope: InvoiceScopeAdminRollup, ResellerId: 0, CustomerId: 0, ChargeQuota: 1}).Error)
	require.NoError(t, DB.Create(&Invoice{Period: "2025-01", Scope: InvoiceScopeCustomer, ResellerId: 2, CustomerId: 20, ChargeQuota: 1}).Error)

	rows, total, err := ListInvoices(InvoiceListFilter{RestrictResellerId: 1, Num: 50})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	for _, r := range rows {
		assert.Equal(t, 1, r.ResellerId)
		assert.NotEqual(t, InvoiceScopeAdminRollup, r.Scope)
	}
}
