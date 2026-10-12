package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func migrateResellerTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&ResellerProfile{}, &ResellerLedger{}))
}

func TestAdjustQuotaWithLedgerIsAtomic(t *testing.T) {
	migrateResellerTables(t)
	const uid = 9301
	require.NoError(t, DB.Create(&User{Id: uid, Username: "ledger-u", Quota: 100, AffCode: "lg1"}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id = ?", uid)
		DB.Exec("DELETE FROM reseller_ledger WHERE customer_id = ?", uid)
	})
	require.NoError(t, AdjustQuotaWithLedger(uid, 50, ResellerLedger{ResellerId: 1, CustomerId: uid, Kind: ResellerLedgerCustomerCredit}))
	require.ErrorIs(t, AdjustQuotaWithLedger(uid, -500, ResellerLedger{ResellerId: 1, CustomerId: uid, Kind: ResellerLedgerCustomerDebit}), ErrInsufficientQuota)
	var u User
	require.NoError(t, DB.First(&u, uid).Error)
	assert.Equal(t, 150, u.Quota)
	var n int64
	DB.Model(&ResellerLedger{}).Where("customer_id = ?", uid).Count(&n)
	assert.Equal(t, int64(1), n, "failed debit writes no ledger row")
}

func TestCreateResellerCustomerEnforcesLimit(t *testing.T) {
	migrateResellerTables(t)
	const rid = 9311
	require.NoError(t, DB.Create(&User{Id: rid, Username: "lim-r", Role: common.RoleResellerUser, AffCode: "lr1"}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: rid, DefaultMarkup: 1, MaxCustomers: 1}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id = ? OR reseller_id = ?", rid, rid)
		DB.Exec("DELETE FROM reseller_profiles WHERE user_id = ?", rid)
	})
	first := &User{Username: "lim-c1", Password: "password123", ResellerId: rid, Group: "default"}
	require.NoError(t, CreateResellerCustomer(rid, first))
	assert.Equal(t, 0, first.Quota)
	second := &User{Username: "lim-c2", Password: "password123", ResellerId: rid, Group: "default"}
	require.ErrorIs(t, CreateResellerCustomer(rid, second), ErrCustomerLimitReached)
}
