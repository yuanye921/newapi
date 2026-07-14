package common

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoMeaningfulOutputState(t *testing.T) {
	info := &RelayInfo{}
	require.False(t, info.HasMeaningfulOutput())

	info.MarkMeaningfulOutput()
	require.True(t, info.HasMeaningfulOutput())
}

func TestRelayInfoHasCleanResponseEnd(t *testing.T) {
	tests := []struct {
		name  string
		info  *RelayInfo
		clean bool
	}{
		{name: "nil relay info", info: nil, clean: false},
		{name: "non-stream success", info: &RelayInfo{}, clean: true},
		{name: "non-stream upstream failure", info: failedRelayInfo(false), clean: false},
		{name: "stream done", info: streamRelayInfo(StreamEndReasonDone, false), clean: true},
		{name: "stream client gone", info: streamRelayInfo(StreamEndReasonClientGone, false), clean: false},
		{name: "stream soft error", info: streamRelayInfo(StreamEndReasonDone, true), clean: false},
		{name: "stream status missing", info: &RelayInfo{IsStream: true}, clean: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.clean, test.info.HasCleanResponseEnd())
		})
	}
}

func failedRelayInfo(stream bool) *RelayInfo {
	info := &RelayInfo{IsStream: stream}
	info.MarkResponseFailed()
	return info
}

func streamRelayInfo(reason StreamEndReason, withError bool) *RelayInfo {
	status := NewStreamStatus()
	status.SetEndReason(reason, nil)
	if withError {
		status.RecordError("invalid upstream event")
	}
	return &RelayInfo{IsStream: true, StreamStatus: status}
}
