package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// ReconcileRevenueFromLogs rebuilds revenue rows missing for consume/refund
// logs created since `since` that carry a revenue context. It is idempotent
// (dedupe key log:<id>) and only runs when logs share the main database.
func ReconcileRevenueFromLogs(since int64, limit int) (created int, err error) {
	if !logsShareMainDB() {
		return 0, nil
	}
	if limit <= 0 {
		limit = 1000
	}
	var logs []Log
	err = DB.Table("logs").
		Select("logs.id, logs.user_id, logs.created_at, logs.type, logs.model_name, logs.quota, logs.request_id, logs.reseller_cost_quota, logs.other").
		Joins("LEFT JOIN revenue_records r ON r.log_id = logs.id").
		Where("logs.created_at >= ? AND logs.type IN ? AND r.id IS NULL AND logs.other LIKE ? AND (logs.quota <> 0 OR logs.reseller_cost_quota <> 0)",
			since, []int{LogTypeConsume, LogTypeRefund}, "%\""+logOtherRevenueKey+"\"%").
		Order("logs.id").Limit(limit).Find(&logs).Error
	if err != nil {
		return 0, err
	}
	for _, lg := range logs {
		rc, ok := revenueContextFromOther(lg.Other)
		if !ok {
			continue
		}
		kind, _ := logTypeRevenueKind(lg.Type)
		rec := BuildRevenueRecord(rc, lg.UserId, lg.ModelName, kind, int64(lg.Quota), int64(lg.ResellerCostQuota))
		rec.CreatedAt = lg.CreatedAt
		rec.LogId = int64(lg.Id)
		rec.RequestId = lg.RequestId
		rec.DedupeKey = fmt.Sprintf("log:%d", lg.Id)
		rec.Source = RevenueSourceReconcile
		if err := insertRevenueTx(DB, &rec); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func revenueContextFromOther(other string) (RevenueContext, bool) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(other), &parsed); err != nil {
		return RevenueContext{}, false
	}
	adminRaw, ok := parsed[logOtherAdminInfoKey]
	if !ok {
		return RevenueContext{}, false
	}
	var admin map[string]json.RawMessage
	if err := json.Unmarshal(adminRaw, &admin); err != nil {
		return RevenueContext{}, false
	}
	raw, ok := admin[logOtherRevenueKey]
	if !ok {
		return RevenueContext{}, false
	}
	var rc RevenueContext
	if err := json.Unmarshal(raw, &rc); err != nil || rc.PayerType == "" {
		return RevenueContext{}, false
	}
	return rc, true
}

// StartRevenueWorker flushes the retry queue every 30s and reconciles the
// last 48h of logs every 10 minutes.
func StartRevenueWorker() {
	go func() {
		flush := time.NewTicker(30 * time.Second)
		reconcile := time.NewTicker(10 * time.Minute)
		defer flush.Stop()
		defer reconcile.Stop()
		for {
			select {
			case <-flush.C:
				if PendingRevenueCount() > 0 {
					if left := FlushPendingRevenue(); left > 0 {
						common.SysError(fmt.Sprintf("revenue retry queue still has %d rows", left))
					}
				}
			case <-reconcile.C:
				if n, err := ReconcileRevenueFromLogs(time.Now().Add(-48*time.Hour).Unix(), 5000); err != nil {
					common.SysError(fmt.Sprintf("revenue reconcile failed: %v", err))
				} else if n > 0 {
					common.SysLog(fmt.Sprintf("revenue reconcile rebuilt %d missing rows", n))
				}
			}
		}
	}()
}
