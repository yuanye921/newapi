package relay

import (
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func prefillCompatibilityEnabledForRequest(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil || !info.ChannelSetting.PrefillCompatibilityEnabled {
		return false
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		return info.RelayMode == relayconstant.RelayModeChatCompletions
	case types.RelayFormatClaude:
		return true
	case types.RelayFormatGemini:
		return info.RelayMode == relayconstant.RelayModeGemini &&
			!strings.Contains(strings.ToLower(info.RequestURLPath), "counttokens")
	case types.RelayFormatOpenAIResponses:
		return info.RelayMode == relayconstant.RelayModeResponses
	default:
		return false
	}
}

func applyFinalPrefillCompatibility(info *relaycommon.RelayInfo, data []byte) ([]byte, error) {
	if !prefillCompatibilityEnabledForRequest(info) {
		return data, nil
	}
	updated, _, err := relaycommon.ApplyPrefillCompatibilityJSON(data, info.GetFinalRequestRelayFormat())
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func preparePassThroughRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, io.Closer, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, nil, err
	}
	info.UpstreamRequestBodySize = storage.Size()
	if !prefillCompatibilityEnabledForRequest(info) {
		return common.ReaderOnly(storage), nil, nil
	}

	data, err := storage.Bytes()
	if err != nil {
		return nil, nil, err
	}
	updated, changed, err := relaycommon.ApplyPrefillCompatibilityJSON(data, info.RelayFormat)
	if err != nil {
		return nil, nil, err
	}
	if !changed {
		return common.ReaderOnly(storage), nil, nil
	}

	body, size, closer, err := relaycommon.NewOutboundJSONBody(updated)
	if err != nil {
		return nil, nil, err
	}
	info.UpstreamRequestBodySize = size
	return body, closer, nil
}
