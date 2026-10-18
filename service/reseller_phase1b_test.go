package service

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedResellerTask(t *testing.T, quota int, markup float64) *model.Task {
	t.Helper()
	task := &model.Task{TaskID: "b2b-task-1", UserId: b2bCustomerId, Quota: quota, Group: "default"}
	task.PrivateData.BillingSource = BillingSourceWallet
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		OriginModelName: "b2b-model", GroupRatio: markup,
		ResellerId: b2bResellerId, ResellerMarkup: markup, ResellerCostFactor: 1,
	}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM tasks WHERE task_id = ?", "b2b-task-1") })
	return task
}

func TestTaskRefundReturnsResellerLeg(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	// submit already charged customer 300 and reseller ceil(300/2)=150
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", b2bResellerId).Update("quota", 850).Error)
	task := seedResellerTask(t, 300, 2)

	require.True(t, RefundTaskQuota(context.Background(), task, "failed"))
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId), "reseller cost refunded")
	assert.Equal(t, 10300, quotaOf(t, b2bCustomerId))
}

func TestTaskRecalculateAdjustsResellerLeg(t *testing.T) {
	seedResellerLine(t, 850, 10000, 2, common.UserStatusEnabled)
	task := seedResellerTask(t, 300, 2)

	RecalculateTaskQuota(context.Background(), task, 501, common.NewMessage("test"))
	// cost 150 -> ceil(501/2)=251: reseller pays 101 more, customer 201 more
	assert.Equal(t, 749, quotaOf(t, b2bResellerId))
	assert.Equal(t, 9799, quotaOf(t, b2bCustomerId))

	RecalculateTaskQuota(context.Background(), task, 100, common.NewMessage("test"))
	assert.Equal(t, 950, quotaOf(t, b2bResellerId), "cost back to 50")
}

func TestMidjourneyResellerLegChargedAndRefunded(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	_, info := newResellerRelayInfo(t)
	require.NoError(t, CheckMidjourneyResellerBilling(info, 400))
	require.ErrorIs(t, CheckMidjourneyResellerBilling(info, 4000), ErrResellerInsufficient)

	info.IsPlayground = true
	task := &model.Midjourney{UserId: b2bCustomerId, MjId: "b2b-mj", ChannelId: 1}
	prepared, err := PrepareMidjourneyTaskBilling(info, task, 400, true)
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { model.DB.Exec("DELETE FROM midjourneys WHERE mj_id = ?", "b2b-mj") })
	applied, err := SettleMidjourneyTaskBilling(info, task, prepared)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, 800, quotaOf(t, b2bResellerId))
	assert.Equal(t, 9600, quotaOf(t, b2bCustomerId))
	assert.Equal(t, 200, task.ResellerCostQuota)

	require.True(t, RefundMidjourneyQuota(context.Background(), task, "failed"))
	assert.Equal(t, 1000, quotaOf(t, b2bResellerId))
	assert.Equal(t, 10000, quotaOf(t, b2bCustomerId))
}

func TestResellerCompletionRatioOverride(t *testing.T) {
	seedResellerLine(t, 1000, 10000, 2, common.UserStatusEnabled)
	cr := 3.5
	require.NoError(t, model.DB.Create(&model.ResellerModelPrice{ResellerId: b2bResellerId, ModelName: "b2b-model", CompletionRatio: &cr}).Error)
	model.InvalidateResellerCache(b2bResellerId)
	info := &relaycommon.RelayInfo{UserId: b2bCustomerId, ResellerId: b2bResellerId}
	got, ok := ResellerCompletionRatio(info, "b2b-model")
	require.True(t, ok)
	assert.Equal(t, 3.5, got)
	_, ok = ResellerCompletionRatio(info, "other-model")
	assert.False(t, ok)
	_, ok = ResellerCompletionRatio(&relaycommon.RelayInfo{UserId: 1}, "b2b-model")
	assert.False(t, ok, "direct users unaffected")
}

// The engine computes charge = round(x*markup) with x = base*costFactor, and
// the reseller cost is derived as round_half_up(charge/markup) = round(x) up to
// the charge rounding: off by at most one quota unit either way (unbiased), and
// the reseller never loses money (charge >= cost for markup >= 1).
func TestResellerCostDerivationMaxError(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 200000; i++ {
		x := rng.Float64() * 1e6
		markup := 1 + rng.Float64()*9
		if i%7 == 0 {
			markup = 0.1 + rng.Float64()*0.9
		}
		charge := int(math.Round(x * markup))
		cost := ResellerCostFromCharge(charge, markup)
		designCost := int(math.Round(x))
		diff := cost - designCost
		if markup >= 1 {
			// round(round(x*m)/m) vs round(x): at most one quota unit apart,
			// in either direction (no systematic bias), except the 1-quota floor.
			assert.True(t, (diff >= -1 && diff <= 1) || (designCost == 0 && cost == 1), "x=%v m=%v charge=%d cost=%d design=%d", x, markup, charge, cost, designCost)
			assert.GreaterOrEqual(t, charge, cost)
		} else {
			// markup < 1 (selling below cost): error bounded by 0.5/markup + 1.5
			assert.LessOrEqual(t, math.Abs(float64(diff)), 0.5/markup+1.5)
		}
		if t.Failed() {
			return
		}
	}
}
