package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
	// ResellerId 0 = issued by admin (direct users only); >0 = issued by that
	// reseller and redeemable only by its customers.
	ResellerId int `json:"reseller_id" gorm:"not null;default:0;index"`
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, status string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&Redemption{})

	if keyword != "" {
		if id, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
		} else {
			query = query.Where("name LIKE ?", keyword+"%")
		}
	}

	if status != "" {
		now := common.GetTimestamp()
		switch status {
		case "expired":
			query = query.Where(
				"status = ? AND expired_time != 0 AND expired_time < ?",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusEnabled):
			query = query.Where(
				"status = ? AND (expired_time = 0 OR expired_time >= ?)",
				common.RedemptionCodeStatusEnabled,
				now,
			)
		case strconv.Itoa(common.RedemptionCodeStatusDisabled):
			query = query.Where("status = ?", common.RedemptionCodeStatusDisabled)
		case strconv.Itoa(common.RedemptionCodeStatusUsed):
			query = query.Where("status = ?", common.RedemptionCodeStatusUsed)
		}
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, common.NewMessage("ID is empty")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (quota int, err error) {
	if key == "" {
		return 0, errors.New("redemption code not provided")
	}
	if userId == 0 {
		return 0, errors.New("invalid user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := lockForUpdate(tx).Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("invalid redemption code")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("redemption code already used")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("redemption code expired")
		}
		var redeemerResellerId int
		if err := tx.Model(&User{}).Where("id = ?", userId).Select("reseller_id").Scan(&redeemerResellerId).Error; err != nil {
			return err
		}
		if redeemerResellerId != redemption.ResellerId {
			return errors.New("invalid redemption code")
		}
		// A reseller code is redeemable only when its face value was escrowed
		// from the reseller wallet at issue; codes that were never funded
		// (or whose quota no longer matches the escrow) are refused.
		if redemption.ResellerId > 0 {
			issued, err := resellerRedemptionIssuedQuota(tx, redemption)
			if err != nil {
				return err
			}
			if issued != int64(redemption.Quota) {
				return errors.New("reseller redemption code is not funded")
			}
		}
		// Compare-and-swap on status: only the transaction that flips
		// enabled -> used may credit quota, so a concurrent redeem of the
		// same code loses here even without a row lock (e.g. on SQLite).
		result := tx.Model(&Redemption{}).
			Where("id = ? AND status = ?", redemption.Id, common.RedemptionCodeStatusEnabled).
			Updates(map[string]any{
				"redeemed_time": common.GetTimestamp(),
				"status":        common.RedemptionCodeStatusUsed,
				"used_user_id":  userId,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("redemption code already used")
		}
		if err := creditTopUpQuota(tx, userId, redemption.Quota, nil); err != nil {
			return err
		}
		if redemption.ResellerId <= 0 {
			return nil
		}
		return tx.Create(&ResellerLedger{
			ResellerId: redemption.ResellerId,
			CustomerId: userId,
			Kind:       ResellerLedgerRedemptionRedeem,
			QuotaDelta: int64(redemption.Quota),
			OperatorId: userId,
			RefId:      redemptionLedgerRef(redemption.Id),
			CreatedAt:  common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		common.SysError(common.LogText("redemption failed: %s", err.Error()))
		return 0, ErrRedeemFailed
	}
	syncCreditUserQuotaCache(userId, redemption.Quota, common.LogText("redemption"))
	RecordLog(userId, LogTypeTopup, common.NewMessage("Topped up {{quota}} with a redemption code, code ID {{id}}", map[string]any{"quota": logger.FormatQuota(redemption.Quota), "id": redemption.Id}))
	return redemption.Quota, nil
}

func (redemption *Redemption) Insert() error {
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
		return err
	}
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	if redemption.Quota <= 0 {
		return errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(redemption.Quota); err != nil {
		return err
	}
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	var err error
	err = DB.Delete(redemption).Error
	return err
}

// DeleteRedemptionById soft-deletes one code. Deleting an unused
// reseller-issued code refunds its escrow to the reseller (see
// DeleteResellerRedemption); operatorId is recorded on that ledger row.
func DeleteRedemptionById(id int, operatorId int) (err error) {
	if id == 0 {
		return common.NewMessage("ID is empty")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	if redemption.ResellerId > 0 {
		_, err = DeleteResellerRedemption(redemption.ResellerId, id, operatorId)
		return err
	}
	return redemption.Delete()
}

// DeleteInvalidRedemptions soft-deletes used, disabled and expired codes.
// Admin codes and used reseller codes are removed in bulk; unused reseller
// codes go through DeleteResellerRedemption one by one so each refunds its
// escrow exactly once.
func DeleteInvalidRedemptions(operatorId int) (int64, error) {
	now := common.GetTimestamp()
	invalid := DB.Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now)
	var refundable []Redemption
	if err := DB.Model(&Redemption{}).Where(invalid).Where("reseller_id > 0 AND status <> ?", common.RedemptionCodeStatusUsed).Select("id", "reseller_id").Find(&refundable).Error; err != nil {
		return 0, err
	}
	result := DB.Where(invalid).Where("reseller_id = 0 OR status = ?", common.RedemptionCodeStatusUsed).Delete(&Redemption{})
	if result.Error != nil {
		return 0, result.Error
	}
	deleted := result.RowsAffected
	for _, r := range refundable {
		ok, err := DeleteResellerRedemption(r.ResellerId, r.Id, operatorId)
		if err != nil {
			return deleted, err
		}
		if ok {
			deleted++
		}
	}
	return deleted, nil
}

// BatchDeleteRedemptions soft-deletes the selected codes. Admin codes are
// removed in one statement; reseller codes refund their escrow per code.
func BatchDeleteRedemptions(ids []int, operatorId int) (int64, error) {
	if len(ids) == 0 || len(ids) > 1000 {
		return 0, errors.New("select between 1 and 1000 redemption codes")
	}
	for _, id := range ids {
		if id <= 0 {
			return 0, errors.New("redemption IDs must be positive")
		}
	}
	var resellerCodes []Redemption
	if err := DB.Model(&Redemption{}).Where("id IN ? AND reseller_id > 0", ids).Select("id", "reseller_id").Find(&resellerCodes).Error; err != nil {
		return 0, err
	}
	result := DB.Where("id IN ? AND reseller_id = 0", ids).Delete(&Redemption{})
	if result.Error != nil {
		return 0, result.Error
	}
	deleted := result.RowsAffected
	for _, r := range resellerCodes {
		ok, err := DeleteResellerRedemption(r.ResellerId, r.Id, operatorId)
		if err != nil {
			return deleted, err
		}
		if ok {
			deleted++
		}
	}
	return deleted, nil
}

// redemptionLedgerRef is the reseller_ledger ref_id of a redemption code. The
// code id is used instead of the key so ledger views and exports never expose
// a redeemable secret.
func redemptionLedgerRef(id int) string {
	return "redemption:" + strconv.Itoa(id)
}

// resellerRedemptionIssuedQuota returns the face value escrowed when the code
// was issued (0 for codes created without escrow).
func resellerRedemptionIssuedQuota(tx *gorm.DB, redemption *Redemption) (int64, error) {
	var issued int64
	err := tx.Model(&ResellerLedger{}).
		Where("reseller_id = ? AND kind = ? AND ref_id = ?", redemption.ResellerId, ResellerLedgerRedemptionIssue, redemptionLedgerRef(redemption.Id)).
		Select("COALESCE(SUM(-quota_delta),0)").Scan(&issued).Error
	return issued, err
}

// CreateResellerRedemptions issues count prepaid codes of quota each. The
// reseller wallet is debited quota x count and one redemption_issue ledger row
// is written per code in ONE transaction; when the wallet cannot cover the
// total nothing is created.
func CreateResellerRedemptions(resellerId, operatorId int, name string, quota, count int, expiredTime int64) ([]Redemption, error) {
	if resellerId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if count <= 0 || count > 100 {
		return nil, errors.New("count must be between 1 and 100")
	}
	if quota <= 0 {
		return nil, errors.New("redemption quota must be positive")
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return nil, err
	}
	// count <= 100 and quota <= MaxWalletQuota, so the product cannot overflow.
	if err := common.ValidateWalletQuota(quota * count); err != nil {
		return nil, err
	}
	now := common.GetTimestamp()
	codes := make([]Redemption, 0, count)
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := applyQuotaDeltaTx(tx, resellerId, -quota*count, ErrAmountExceedsResellerBalance); err != nil {
			return err
		}
		for range count {
			code := Redemption{
				UserId:      resellerId,
				Name:        name,
				Key:         common.GetUUID(),
				Status:      common.RedemptionCodeStatusEnabled,
				CreatedTime: now,
				Quota:       quota,
				ExpiredTime: expiredTime,
				ResellerId:  resellerId,
			}
			if err := tx.Create(&code).Error; err != nil {
				return err
			}
			if err := tx.Create(&ResellerLedger{
				ResellerId: resellerId,
				Kind:       ResellerLedgerRedemptionIssue,
				QuotaDelta: -int64(quota),
				OperatorId: operatorId,
				RefId:      redemptionLedgerRef(code.Id),
				Remark:     name,
				CreatedAt:  now,
			}).Error; err != nil {
				return err
			}
			codes = append(codes, code)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if cacheErr := invalidateUserCache(resellerId); cacheErr != nil {
		common.SysError(common.LogText("failed to invalidate user cache after redemption issue (user=%d): %s", resellerId, cacheErr.Error()))
	}
	return codes, nil
}

// DeleteResellerRedemption soft-deletes one code of resellerId and, when it
// was never redeemed, refunds its escrow with a redemption_refund ledger row
// in the same transaction. The soft delete is a compare-and-swap (not deleted,
// not used), and Redeem's used-flip skips soft-deleted rows, so a code is
// either redeemed or refunded, never both, and never refunded twice. Codes
// created without escrow are deleted without refund. Returns false when no
// live code matched.
func DeleteResellerRedemption(resellerId, id, operatorId int) (bool, error) {
	if resellerId <= 0 || id <= 0 {
		return false, errors.New("invalid redemption id")
	}
	refunded := false
	deleted := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var code Redemption
		if err := lockForUpdate(tx).Where("id = ? AND reseller_id = ?", id, resellerId).Limit(1).Find(&code).Error; err != nil {
			return err
		}
		if code.Id == 0 {
			return nil
		}
		if code.Status == common.RedemptionCodeStatusUsed {
			res := tx.Where("id = ?", code.Id).Delete(&Redemption{})
			deleted = res.RowsAffected > 0
			return res.Error
		}
		res := tx.Where("id = ? AND status <> ?", code.Id, common.RedemptionCodeStatusUsed).Delete(&Redemption{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		deleted = true
		issued, err := resellerRedemptionIssuedQuota(tx, &code)
		if err != nil {
			return err
		}
		var alreadyRefunded int64
		if err := tx.Model(&ResellerLedger{}).
			Where("reseller_id = ? AND kind = ? AND ref_id = ?", resellerId, ResellerLedgerRedemptionRefund, redemptionLedgerRef(code.Id)).
			Count(&alreadyRefunded).Error; err != nil {
			return err
		}
		if issued <= 0 || alreadyRefunded > 0 {
			return nil
		}
		if err := applyQuotaDeltaTx(tx, resellerId, int(issued), ErrInsufficientQuota); err != nil {
			return err
		}
		refunded = true
		return tx.Create(&ResellerLedger{
			ResellerId: resellerId,
			Kind:       ResellerLedgerRedemptionRefund,
			QuotaDelta: issued,
			OperatorId: operatorId,
			RefId:      redemptionLedgerRef(code.Id),
			Remark:     code.Name,
			CreatedAt:  common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return false, err
	}
	if refunded {
		if cacheErr := invalidateUserCache(resellerId); cacheErr != nil {
			common.SysError(common.LogText("failed to invalidate user cache after redemption refund (user=%d): %s", resellerId, cacheErr.Error()))
		}
	}
	return deleted, nil
}
