package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuotaRefundJournal makes a wallet refund idempotent: the refund and its
// journal row commit together, so a retried refund with the same key is a
// no-op instead of a double credit.
type QuotaRefundJournal struct {
	Id        int64  `gorm:"primaryKey;autoIncrement"`
	RefundKey string `gorm:"type:varchar(128);uniqueIndex;not null"`
	UserId    int    `gorm:"index"`
	Quota     int    `gorm:"not null"`
	CreatedAt int64  `gorm:"index"`
}

func (QuotaRefundJournal) TableName() string { return "quota_refund_journal" }

// refundQuotaHook lets tests inject transient failures.
var refundQuotaHook func(key string) error

// RefundUserQuotaOnce credits quota to userId exactly once per key. It returns
// nil when the key was already applied. Safe to retry on any error.
func RefundUserQuotaOnce(userId int, quota int, key string) error {
	if quota <= 0 {
		return nil
	}
	if key == "" {
		return errors.New("refund key required")
	}
	if refundQuotaHook != nil {
		if err := refundQuotaHook(key); err != nil {
			return err
		}
	}
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "refund_key"}}, DoNothing: true}).
			Create(&QuotaRefundJournal{RefundKey: key, UserId: userId, Quota: quota, CreatedAt: common.GetTimestamp()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // already refunded
		}
		r := tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", quota))
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		applied = true
		return nil
	})
	if err != nil {
		return err
	}
	if applied {
		if cerr := cacheIncrUserQuota(userId, int64(quota)); cerr != nil {
			common.SysLog("failed to sync refunded quota to cache: " + cerr.Error())
			_ = invalidateUserCache(userId)
		}
	}
	return nil
}

// RefundKeyApplied reports whether a refund key was committed. A lookup
// error (e.g. missing table) returns false with the error.
func RefundKeyApplied(key string) (bool, error) {
	var n int64
	if err := DB.Model(&QuotaRefundJournal{}).Where("refund_key = ?", key).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}
