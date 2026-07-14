package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/assert"
)

func TestIsEmptyResponseCompensationTextRelay(t *testing.T) {
	tests := []struct {
		name    string
		info    *relaycommon.RelayInfo
		allowed bool
	}{
		{name: "nil", info: nil, allowed: false},
		{name: "chat completions", info: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions}, allowed: true},
		{name: "responses", info: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}, allowed: true},
		{name: "gemini native", info: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeGemini}, allowed: true},
		{name: "claude native with unknown mode", info: &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude}, allowed: true},
		{name: "embeddings", info: &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeEmbeddings}, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.allowed, isEmptyResponseCompensationTextRelay(test.info))
		})
	}
}
