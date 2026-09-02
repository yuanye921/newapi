package relay

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrefillCompatibilityEnabledForRequest(t *testing.T) {
	tests := []struct {
		name    string
		info    *relaycommon.RelayInfo
		enabled bool
	}{
		{
			name: "openai chat enabled",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI,
				RelayMode:   relayconstant.RelayModeChatCompletions,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: true,
		},
		{
			name: "channel switch disabled",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI,
				RelayMode:   relayconstant.RelayModeChatCompletions,
				ChannelMeta: &relaycommon.ChannelMeta{},
			},
			enabled: false,
		},
		{
			name: "legacy completions excluded",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI,
				RelayMode:   relayconstant.RelayModeCompletions,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: false,
		},
		{
			name: "claude enabled",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatClaude,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: true,
		},
		{
			name: "gemini enabled",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatGemini,
				RelayMode:   relayconstant.RelayModeGemini,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: true,
		},
		{
			name: "gemini count tokens excluded",
			info: &relaycommon.RelayInfo{
				RelayFormat:    types.RelayFormatGemini,
				RelayMode:      relayconstant.RelayModeGemini,
				RequestURLPath: "/v1beta/models/gemini-2.5-pro:countTokens",
				ChannelMeta:    &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: false,
		},
		{
			name: "responses enabled",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAIResponses,
				RelayMode:   relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: true,
		},
		{
			name: "responses compact excluded",
			info: &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAIResponsesCompaction,
				RelayMode:   relayconstant.RelayModeResponsesCompact,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
			},
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.enabled, prefillCompatibilityEnabledForRequest(tt.info))
		})
	}
}

func TestPreparePassThroughRequestBodyAppliesPrefillCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := `{"model":"test","vendor_top":{"keep":true},"messages":[{"role":"assistant","content":"prefill","vendor_message":7}]}`
	request := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(original))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true}},
	}

	body, closer, err := preparePassThroughRequestBody(c, info)
	require.NoError(t, err)
	require.NotNil(t, closer)
	t.Cleanup(func() { _ = closer.Close() })
	updated, err := io.ReadAll(body)
	require.NoError(t, err)

	assert.Equal(t, int64(2), gjson.GetBytes(updated, "messages.#").Int())
	assert.Equal(t, "user", gjson.GetBytes(updated, "messages.1.role").String())
	assert.Equal(t, ".", gjson.GetBytes(updated, "messages.1.content").String())
	assert.True(t, gjson.GetBytes(updated, "vendor_top.keep").Bool())
	assert.Equal(t, int64(7), gjson.GetBytes(updated, "messages.0.vendor_message").Int())
	assert.Equal(t, int64(len(updated)), info.UpstreamRequestBodySize)
}

func TestPreparePassThroughRequestBodyKeepsDisabledRequestByteExact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := "{ \"messages\": [ { \"role\": \"assistant\", \"content\": \"prefill\" } ] }"
	request := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(original))
	request.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	body, closer, err := preparePassThroughRequestBody(c, info)
	require.NoError(t, err)
	assert.Nil(t, closer)
	unchanged, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, original, string(unchanged))
	assert.Equal(t, int64(len(original)), info.UpstreamRequestBodySize)
}

func TestApplyFinalPrefillCompatibilityUsesConvertedFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RelayMode:              relayconstant.RelayModeChatCompletions,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{PrefillCompatibilityEnabled: true},
		},
	}
	convertedAfterOverride := []byte(`{"input":[{"type":"message","role":"assistant","content":"overridden prefill"}]}`)

	updated, err := applyFinalPrefillCompatibility(info, convertedAfterOverride)
	require.NoError(t, err)
	assert.Equal(t, int64(2), gjson.GetBytes(updated, "input.#").Int())
	assert.Equal(t, "user", gjson.GetBytes(updated, "input.1.role").String())
	assert.Equal(t, ".", gjson.GetBytes(updated, "input.1.content").String())
}
