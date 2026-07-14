package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	emptysetting "github.com/QuantumNous/new-api/setting/empty_response_compensation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func isEmptyResponseCompensationTextRelay(relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo == nil {
		return false
	}
	if relayInfo.RelayFormat == types.RelayFormatClaude {
		return true
	}
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeCompletions,
		relayconstant.RelayModeResponses,
		relayconstant.RelayModeResponsesCompact,
		relayconstant.RelayModeGemini:
		return true
	default:
		return false
	}
}

func TryRecordEmptyResponseCompensation(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, summary textQuotaSummary, settlementErr error) {
	if ctx == nil || relayInfo == nil || settlementErr != nil {
		return
	}
	setting := emptysetting.Get()
	if !setting.Enabled || relayInfo.IsChannelTest || relayInfo.IsPlayground || !isEmptyResponseCompensationTextRelay(relayInfo) {
		return
	}
	if relayInfo.BillingSource != BillingSourceWallet || summary.Quota <= 0 {
		return
	}
	ratio, configured := setting.ModelRatios[relayInfo.OriginModelName]
	if !configured || ratio <= 0 {
		return
	}
	if summary.PromptTokens < setting.InputTokenThreshold || summary.CompletionTokens > setting.OutputTokenThreshold {
		return
	}
	if relayInfo.HasMeaningfulOutput() || !relayInfo.HasCleanResponseEnd() {
		return
	}
	if ctx.Request != nil && ctx.Request.Context().Err() != nil {
		return
	}

	requestId := strings.TrimSpace(relayInfo.RequestId)
	if requestId == "" {
		requestId = strings.TrimSpace(ctx.GetString(common.RequestIdKey))
	}
	if requestId == "" {
		return
	}
	detectionReason := common.GetContextKeyString(ctx, constant.ContextKeyAdminRejectReason)
	_, err := model.CreateEmptyResponseCompensation(model.CreateEmptyResponseCompensationParams{
		UserId:            relayInfo.UserId,
		RequestId:         requestId,
		ModelName:         relayInfo.OriginModelName,
		ChannelId:         relayInfo.ChannelId,
		TokenId:           relayInfo.TokenId,
		PromptTokens:      summary.PromptTokens,
		CompletionTokens:  summary.CompletionTokens,
		OriginalQuota:     summary.Quota,
		CompensationRatio: ratio,
		IsStream:          relayInfo.IsStream,
		DetectionReason:   detectionReason,
		Setting:           setting,
	})
	if err != nil {
		logger.LogError(ctx, "failed to create empty response compensation: "+err.Error())
	}
}
