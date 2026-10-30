package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupStarterCreditDB(t *testing.T) {
	t.Helper()
	prevDB, prevLog := DB, LOG_DB
	prevRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	common.RedisEnabled = false
	t.Cleanup(func() {
		DB, LOG_DB = prevDB, prevLog
		common.RedisEnabled = prevRedis
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&User{}, &ResellerProfile{}, &ResellerLedger{}))
}

func TestCreateResellerCustomerAppliesStarterCredit(t *testing.T) {
	setupStarterCreditDB(t)
	prevUSD := common.ResellerMinCustomerCreditUSD
	prevQPU := common.QuotaPerUnit
	common.ResellerMinCustomerCreditUSD = 10
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.ResellerMinCustomerCreditUSD = prevUSD
		common.QuotaPerUnit = prevQPU
	})

	rid := 8101
	require.NoError(t, DB.Create(&User{
		Id: rid, Username: "r-start", Role: common.RoleResellerUser,
		Status: common.UserStatusEnabled, Quota: 10_000_000, AffCode: "rs1",
	}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: rid, DefaultMarkup: 1.2}).Error)

	starter, err := ResellerMinCustomerStarterQuota()
	require.NoError(t, err)
	require.Equal(t, 5_000_000, starter)

	user := &User{
		Username: "c-start", Password: "Passw0rd!", Role: common.RoleCommonUser,
		ResellerId: rid, Status: common.UserStatusEnabled, Group: "default",
	}
	require.NoError(t, CreateResellerCustomer(rid, user, starter, rid))
	require.NotZero(t, user.Id)
	require.Equal(t, starter, user.Quota)

	var reseller User
	require.NoError(t, DB.First(&reseller, rid).Error)
	require.Equal(t, 5_000_000, reseller.Quota)

	var n int64
	require.NoError(t, DB.Model(&ResellerLedger{}).Where("customer_id = ? AND kind = ?", user.Id, ResellerLedgerCustomerCredit).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestCreateResellerCustomerRejectsInsufficientResellerBalance(t *testing.T) {
	setupStarterCreditDB(t)
	rid := 8102
	require.NoError(t, DB.Create(&User{
		Id: rid, Username: "r-poor", Role: common.RoleResellerUser,
		Status: common.UserStatusEnabled, Quota: 100, AffCode: "rp1",
	}).Error)
	require.NoError(t, DB.Create(&ResellerProfile{UserId: rid, DefaultMarkup: 1.2}).Error)

	user := &User{
		Username: "c-poor", Password: "Passw0rd!", Role: common.RoleCommonUser,
		ResellerId: rid, Status: common.UserStatusEnabled, Group: "default",
	}
	err := CreateResellerCustomer(rid, user, 5_000_000, rid)
	require.ErrorIs(t, err, ErrAmountExceedsResellerBalance)

	var count int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", "c-poor").Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestResellerMinCustomerStarterQuotaDefault(t *testing.T) {
	prevUSD := common.ResellerMinCustomerCreditUSD
	prevQPU := common.QuotaPerUnit
	common.ResellerMinCustomerCreditUSD = 10
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.ResellerMinCustomerCreditUSD = prevUSD
		common.QuotaPerUnit = prevQPU
	})
	q, err := ResellerMinCustomerStarterQuota()
	require.NoError(t, err)
	require.Equal(t, 5_000_000, q)

	common.ResellerMinCustomerCreditUSD = 0
	q, err = ResellerMinCustomerStarterQuota()
	require.NoError(t, err)
	require.Equal(t, 0, q)
}
