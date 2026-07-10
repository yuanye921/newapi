package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestDropsTopPForClaudeLikeModelWhenTemperatureIsSet(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "[aws]claude-opus-4-6",
		Temperature: common.GetPointer(0.7),
		TopP:        common.GetPointer(0.9),
	}
	info := &relaycommon.RelayInfo{
		UpstreamModelName: "[aws]claude-opus-4-6",
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 0.7, *convertedRequest.Temperature)
	require.Nil(t, convertedRequest.TopP)
}

func TestConvertOpenAIRequestKeepsTopPForNonClaudeModel(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "gpt-4o",
		Temperature: common.GetPointer(0.7),
		TopP:        common.GetPointer(0.9),
	}
	info := &relaycommon.RelayInfo{
		UpstreamModelName: "gpt-4o",
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)

	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, convertedRequest.TopP)
	require.Equal(t, 0.9, *convertedRequest.TopP)
}
