package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	emptysetting "github.com/QuantumNous/new-api/setting/empty_response_compensation_setting"

	"github.com/gin-gonic/gin"
)

type emptyResponseCompensationClaimRequest struct {
	Ids []int `json:"ids"`
}

func emptyResponseCompensationRules(setting emptysetting.Setting) gin.H {
	return gin.H{
		"min_qualification_amount": setting.MinQualificationAmount,
		"input_token_threshold":    setting.InputTokenThreshold,
		"output_token_threshold":   setting.OutputTokenThreshold,
		"claim_window_days":        setting.ClaimWindowDays,
		"daily_claim_limit":        setting.DailyClaimLimit,
		"announcement":             setting.Announcement,
	}
}

func GetUserEmptyResponseCompensation(c *gin.Context) {
	setting := emptysetting.Get()
	pageInfo := common.GetPageQuery(c)
	userId := c.GetInt("id")
	summary, err := model.GetEmptyResponseCompensationSummary(userId, setting)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	records, total, err := model.GetUserEmptyResponseCompensations(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, gin.H{
		"enabled": setting.Enabled,
		"rules":   emptyResponseCompensationRules(setting),
		"summary": summary,
		"records": pageInfo,
	})
}

func ClaimUserEmptyResponseCompensation(c *gin.Context) {
	setting := emptysetting.Get()
	if !setting.Enabled {
		common.ApiErrorI18n(c, i18n.MsgFeatureDisabled)
		return
	}
	var request emptyResponseCompensationClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(request.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	result, err := model.ClaimEmptyResponseCompensations(c.GetInt("id"), request.Ids, setting)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgOperationFailed)
		return
	}
	common.ApiSuccess(c, result)
}

func GetAdminEmptyResponseCompensations(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId, _ := strconv.Atoi(c.Query("user_id"))
	startTime, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	records, total, err := model.GetAdminEmptyResponseCompensations(model.EmptyResponseCompensationAdminFilter{
		UserId:    userId,
		ModelName: c.Query("model_name"),
		Status:    c.Query("status"),
		StartTime: startTime,
		EndTime:   endTime,
		Offset:    pageInfo.GetStartIdx(),
		Limit:     pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	summary, err := model.GetEmptyResponseCompensationAdminSummary()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgDatabaseError)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	setting := emptysetting.Get()
	common.ApiSuccess(c, gin.H{
		"summary": summary,
		"records": pageInfo,
		"rules":   emptyResponseCompensationRules(setting),
	})
}
