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

func TestTransferResellerCustomerQuotaSuccessAndInsufficient(t *testing.T) {
	migrateResellerTables(t)
	const rid, cid = 9321, 9322
	require.NoError(t, DB.Create(&User{Id: rid, Username: "tx-r", Role: common.RoleResellerUser, Quota: 500, AffCode: "txr"}).Error)
	require.NoError(t, DB.Create(&User{Id: cid, Username: "tx-c", ResellerId: rid, Quota: 0, AffCode: "txc"}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id IN (?, ?)", rid, cid)
		DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", rid)
	})

	require.NoError(t, TransferResellerCustomerQuota(rid, cid, 200, ResellerLedger{
		Kind: ResellerLedgerCustomerCredit, OperatorId: rid,
	}))
	var r, c User
	require.NoError(t, DB.First(&r, rid).Error)
	require.NoError(t, DB.First(&c, cid).Error)
	assert.Equal(t, 300, r.Quota)
	assert.Equal(t, 200, c.Quota)

	require.ErrorIs(t, TransferResellerCustomerQuota(rid, cid, 301, ResellerLedger{
		Kind: ResellerLedgerCustomerCredit, OperatorId: rid,
	}), ErrAmountExceedsResellerBalance)
	require.NoError(t, DB.First(&r, rid).Error)
	require.NoError(t, DB.First(&c, cid).Error)
	assert.Equal(t, 300, r.Quota)
	assert.Equal(t, 200, c.Quota)

	require.NoError(t, TransferResellerCustomerQuota(rid, cid, -50, ResellerLedger{
		Kind: ResellerLedgerCustomerDebit, OperatorId: rid,
	}))
	require.NoError(t, DB.First(&r, rid).Error)
	require.NoError(t, DB.First(&c, cid).Error)
	assert.Equal(t, 350, r.Quota)
	assert.Equal(t, 150, c.Quota)

	var n int64
	DB.Model(&ResellerLedger{}).Where("reseller_id = ? AND customer_id = ?", rid, cid).Count(&n)
	assert.Equal(t, int64(2), n)
}

func TestTransferResellerCustomerQuotaClawbackInsufficientCustomer(t *testing.T) {
	migrateResellerTables(t)
	const rid, cid = 9331, 9332
	require.NoError(t, DB.Create(&User{Id: rid, Username: "tx2-r", Role: common.RoleResellerUser, Quota: 100, AffCode: "t2r"}).Error)
	require.NoError(t, DB.Create(&User{Id: cid, Username: "tx2-c", ResellerId: rid, Quota: 10, AffCode: "t2c"}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id IN (?, ?)", rid, cid)
		DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", rid)
	})
	require.ErrorIs(t, TransferResellerCustomerQuota(rid, cid, -11, ResellerLedger{
		Kind: ResellerLedgerCustomerDebit, OperatorId: rid,
	}), ErrInsufficientQuota)
	var r, c User
	require.NoError(t, DB.First(&r, rid).Error)
	require.NoError(t, DB.First(&c, cid).Error)
	assert.Equal(t, 100, r.Quota)
	assert.Equal(t, 10, c.Quota)
}

// seedResellerRedemptionLine creates a reseller (wallet resellerQuota), one of
// its customers, a customer of another reseller and a direct user.
func seedResellerRedemptionLine(t *testing.T, resellerQuota int) (rid, cid, otherCid, directId int) {
	t.Helper()
	migrateResellerTables(t)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	rid, cid, otherCid, directId = 9601, 9602, 9603, 9604
	const otherRid = 9605
	require.NoError(t, DB.Create(&User{Id: rid, Username: "rr-r", Role: common.RoleResellerUser, Status: common.UserStatusEnabled, Quota: resellerQuota, AffCode: "rrr"}).Error)
	require.NoError(t, DB.Create(&User{Id: cid, Username: "rr-c", ResellerId: rid, Status: common.UserStatusEnabled, AffCode: "rrc"}).Error)
	require.NoError(t, DB.Create(&User{Id: otherRid, Username: "rr-r2", Role: common.RoleResellerUser, Status: common.UserStatusEnabled, AffCode: "rrr2"}).Error)
	require.NoError(t, DB.Create(&User{Id: otherCid, Username: "rr-c2", ResellerId: otherRid, Status: common.UserStatusEnabled, AffCode: "rrc2"}).Error)
	require.NoError(t, DB.Create(&User{Id: directId, Username: "rr-admin", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "rra"}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id IN (?, ?, ?, ?, ?)", rid, cid, otherRid, otherCid, directId)
		DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id IN (?, ?)", rid, otherRid)
		DB.Unscoped().Where("reseller_id = ? OR user_id = ?", rid, directId).Delete(&Redemption{})
	})
	return rid, cid, otherCid, directId
}

func userQuota(t *testing.T, id int) int {
	t.Helper()
	var u User
	require.NoError(t, DB.Select("quota").First(&u, id).Error)
	return u.Quota
}

func resellerLedgerSum(t *testing.T, rid int, kind string) (int64, int64) {
	t.Helper()
	var row struct {
		N   int64
		Sum int64
	}
	require.NoError(t, DB.Model(&ResellerLedger{}).Where("reseller_id = ? AND kind = ?", rid, kind).
		Select("COUNT(*) AS n, COALESCE(SUM(quota_delta),0) AS sum").Scan(&row).Error)
	return row.N, row.Sum
}

func TestCreateResellerRedemptionsEscrowsWallet(t *testing.T) {
	rid, _, _, _ := seedResellerRedemptionLine(t, 100)

	_, err := CreateResellerRedemptions(rid, rid, "too-much", 40, 3, 0)
	require.ErrorIs(t, err, ErrAmountExceedsResellerBalance)
	assert.Equal(t, 100, userQuota(t, rid))
	var codes int64
	require.NoError(t, DB.Model(&Redemption{}).Where("reseller_id = ?", rid).Count(&codes).Error)
	assert.Zero(t, codes, "insufficient balance must create no code")
	n, _ := resellerLedgerSum(t, rid, ResellerLedgerRedemptionIssue)
	assert.Zero(t, n)

	issued, err := CreateResellerRedemptions(rid, rid, "batch", 40, 2, 0)
	require.NoError(t, err)
	require.Len(t, issued, 2)
	assert.Equal(t, 20, userQuota(t, rid))
	n, sum := resellerLedgerSum(t, rid, ResellerLedgerRedemptionIssue)
	assert.Equal(t, int64(2), n)
	assert.Equal(t, int64(-80), sum)
	var row ResellerLedger
	require.NoError(t, DB.Where("reseller_id = ? AND kind = ?", rid, ResellerLedgerRedemptionIssue).First(&row).Error)
	assert.Equal(t, rid, row.OperatorId)
	assert.Zero(t, row.CustomerId)
	assert.NotContains(t, row.RefId, issued[0].Key, "ledger must not expose the redeemable key")
}

func TestRedeemResellerCodeOnlyByItsCustomers(t *testing.T) {
	rid, cid, otherCid, directId := seedResellerRedemptionLine(t, 1000)
	issued, err := CreateResellerRedemptions(rid, rid, "scope", 300, 1, 0)
	require.NoError(t, err)
	key := issued[0].Key

	for name, uid := range map[string]int{"other reseller's customer": otherCid, "issuing reseller": rid, "admin": directId} {
		_, err := Redeem(key, uid)
		assert.ErrorIs(t, err, ErrRedeemFailed, name)
	}

	quota, err := Redeem(key, cid)
	require.NoError(t, err)
	assert.Equal(t, 300, quota)
	assert.Equal(t, 300, userQuota(t, cid))
	assert.Equal(t, 700, userQuota(t, rid), "redeem must not debit the reseller again")
	var row ResellerLedger
	require.NoError(t, DB.Where("reseller_id = ? AND kind = ?", rid, ResellerLedgerRedemptionRedeem).First(&row).Error)
	assert.Equal(t, cid, row.CustomerId)
	assert.Equal(t, int64(300), row.QuotaDelta)
	assert.Equal(t, redemptionLedgerRef(issued[0].Id), row.RefId)
}

func TestRedeemRejectsUnfundedResellerCode(t *testing.T) {
	rid, cid, _, _ := seedResellerRedemptionLine(t, 100)
	code := Redemption{UserId: rid, Name: "legacy", Key: "rr-unfunded-key", Status: common.RedemptionCodeStatusEnabled, Quota: 1_000_000, ResellerId: rid, CreatedTime: common.GetTimestamp()}
	require.NoError(t, code.Insert())

	_, err := Redeem(code.Key, cid)
	require.ErrorIs(t, err, ErrRedeemFailed)
	assert.Zero(t, userQuota(t, cid))
	n, _ := resellerLedgerSum(t, rid, ResellerLedgerRedemptionRedeem)
	assert.Zero(t, n)

	// Deleting a code that was never escrowed refunds nothing.
	deleted, err := DeleteResellerRedemption(rid, code.Id, rid)
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.Equal(t, 100, userQuota(t, rid))
}

func TestRedeemAdminCodeWritesNoResellerLedger(t *testing.T) {
	_, _, _, directId := seedResellerRedemptionLine(t, 0)
	code := Redemption{UserId: directId, Name: "admin", Key: "rr-admin-code-key", Status: common.RedemptionCodeStatusEnabled, Quota: 50, CreatedTime: common.GetTimestamp()}
	require.NoError(t, code.Insert())

	quota, err := Redeem(code.Key, directId)
	require.NoError(t, err)
	assert.Equal(t, 50, quota)
	var n int64
	require.NoError(t, DB.Model(&ResellerLedger{}).Where("customer_id = ?", directId).Count(&n).Error)
	assert.Zero(t, n)
}

func TestDeleteResellerRedemptionRefundsUnusedOnce(t *testing.T) {
	rid, cid, _, directId := seedResellerRedemptionLine(t, 1000)
	issued, err := CreateResellerRedemptions(rid, rid, "refund", 100, 5, 0)
	require.NoError(t, err)
	require.Equal(t, 500, userQuota(t, rid))

	// Single reseller delete refunds; a second delete is a no-op.
	deleted, err := DeleteResellerRedemption(rid, issued[0].Id, rid)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = DeleteResellerRedemption(rid, issued[0].Id, rid)
	require.NoError(t, err)
	assert.False(t, deleted)
	assert.Equal(t, 600, userQuota(t, rid))

	// A used code is deleted without refund.
	_, err = Redeem(issued[1].Key, cid)
	require.NoError(t, err)
	require.NoError(t, DeleteRedemptionById(issued[1].Id, directId))
	assert.Equal(t, 600, userQuota(t, rid))

	// Admin "delete invalid" refunds a disabled reseller code.
	require.NoError(t, DB.Model(&Redemption{}).Where("id = ?", issued[2].Id).Update("status", common.RedemptionCodeStatusDisabled).Error)
	_, err = DeleteInvalidRedemptions(directId)
	require.NoError(t, err)
	assert.Equal(t, 700, userQuota(t, rid))

	// Admin batch delete refunds unused codes and ignores already deleted ones.
	count, err := BatchDeleteRedemptions([]int{issued[0].Id, issued[3].Id, issued[4].Id}, directId)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	assert.Equal(t, 900, userQuota(t, rid))

	n, sum := resellerLedgerSum(t, rid, ResellerLedgerRedemptionRefund)
	assert.Equal(t, int64(4), n)
	assert.Equal(t, int64(400), sum)
	var adminRows int64
	require.NoError(t, DB.Model(&ResellerLedger{}).Where("reseller_id = ? AND kind = ? AND operator_id = ?", rid, ResellerLedgerRedemptionRefund, directId).Count(&adminRows).Error)
	assert.Equal(t, int64(3), adminRows)
	// Every code is settled: wallet + redeemed value equals the funding.
	assert.Equal(t, 1000, userQuota(t, rid)+userQuota(t, cid))
}

func TestGetResellerProfileDefaultsRedemptionOff(t *testing.T) {
	migrateResellerTables(t)
	profile, err := GetResellerProfile(9699)
	require.NoError(t, err)
	assert.False(t, profile.AllowRedemption)
	assert.InDelta(t, 1.0, profile.DefaultCostFactor, 1e-9)
}
