package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// R12: reseller-issued redemption codes must be funded from the reseller
// wallet at issue (with a ledger row) and redemption must be traceable; a code
// worth more than the reseller balance cannot be minted.
func TestAuditR12_ResellerRedemptionBypassesLedger(t *testing.T) {
	migrateResellerTables(t)
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	const rid, cid = 9501, 9502
	require.NoError(t, DB.Create(&User{Id: rid, Username: "rd-r", Role: common.RoleResellerUser, Quota: 100, AffCode: "rdr"}).Error)
	require.NoError(t, DB.Create(&User{Id: cid, Username: "rd-c", ResellerId: rid, Quota: 0, Status: common.UserStatusEnabled, AffCode: "rdc"}).Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM users WHERE id IN (?, ?)", rid, cid)
		DB.Exec("DELETE FROM reseller_ledger WHERE reseller_id = ?", rid)
		DB.Unscoped().Where("reseller_id = ?", rid).Delete(&Redemption{})
	})

	_, err := CreateResellerRedemptions(rid, rid, "x", 1_000_000, 1, 0)
	require.ErrorIs(t, err, ErrAmountExceedsResellerBalance, "code worth 10x the reseller balance must not be issued")

	// A code inserted around the escrow (e.g. legacy) cannot be redeemed.
	r := Redemption{UserId: rid, Name: "x", Key: "audit-rd-key-1", CreatedTime: common.GetTimestamp(), Quota: 1_000_000, ResellerId: rid, Status: common.RedemptionCodeStatusEnabled}
	require.NoError(t, r.Insert())
	_, err = Redeem("audit-rd-key-1", cid)
	require.Error(t, err)

	codes, err := CreateResellerRedemptions(rid, rid, "x", 60, 1, 0)
	require.NoError(t, err)
	q, err := Redeem(codes[0].Key, cid)
	require.NoError(t, err)
	var rr, cc User
	require.NoError(t, DB.First(&rr, rid).Error)
	require.NoError(t, DB.First(&cc, cid).Error)
	var ledgerSum int64
	require.NoError(t, DB.Model(&ResellerLedger{}).Where("reseller_id = ?", rid).Select("COALESCE(SUM(quota_delta),0)").Scan(&ledgerSum).Error)
	t.Logf("AUDIT R12 code quota=%d: reseller after=%d customer after=%d ledgerSum=%d", q, rr.Quota, cc.Quota, ledgerSum)
	assert.Equal(t, 40, rr.Quota)
	assert.Equal(t, 60, cc.Quota)
	// issue (-60) + redeem (+60): the ledger nets to zero like a transfer.
	assert.Equal(t, int64(0), ledgerSum)
	assert.Equal(t, 100, rr.Quota+cc.Quota)
}
