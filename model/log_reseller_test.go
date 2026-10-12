package model

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordConsumeLogCopiesResellerLegFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(ContextKeyLogResellerId, 42)
	c.Set(ContextKeyLogResellerCostQuota, 7)

	RecordConsumeLog(c, 1001, RecordConsumeLogParams{
		ChannelId:        1,
		PromptTokens:     8,
		CompletionTokens: 1,
		ModelName:        "gpt-4o-mini",
		TokenName:        "t",
		Quota:            10,
		TokenId:          1,
		Group:            "default",
		Other:            &LogOther{},
	})

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND model_name = ?", 1001, "gpt-4o-mini").
		Order("id desc").First(&log).Error)
	t.Cleanup(func() { LOG_DB.Delete(&Log{}, log.Id) })

	assert.Equal(t, 42, log.ResellerId)
	assert.Equal(t, 7, log.ResellerCostQuota)
	assert.Equal(t, 10, log.Quota)
	assert.Equal(t, LogTypeConsume, log.Type)
}

func TestRecordConsumeLogOmitsResellerLegWhenUnset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RecordConsumeLog(c, 1002, RecordConsumeLogParams{
		ChannelId: 1,
		ModelName: "gpt-4o-mini-direct",
		Quota:     3,
		Group:     "default",
		Other:     &LogOther{},
	})

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND model_name = ?", 1002, "gpt-4o-mini-direct").
		Order("id desc").First(&log).Error)
	t.Cleanup(func() { LOG_DB.Delete(&Log{}, log.Id) })

	assert.Equal(t, 0, log.ResellerId)
	assert.Equal(t, 0, log.ResellerCostQuota)
}
