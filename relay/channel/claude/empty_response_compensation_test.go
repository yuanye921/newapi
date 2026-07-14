package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
)

func TestMarkClaudeMeaningfulOutput(t *testing.T) {
	text := "answer"
	thinking := "reasoning"
	whitespace := "   "
	tests := []struct {
		name       string
		response   *dto.ClaudeResponse
		meaningful bool
	}{
		{name: "empty", response: &dto.ClaudeResponse{}, meaningful: false},
		{name: "whitespace", response: &dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Text: &whitespace}}}, meaningful: false},
		{name: "text", response: &dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Text: &text}}}, meaningful: true},
		{name: "thinking", response: &dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Thinking: &thinking}}}, meaningful: true},
		{name: "tool call", response: &dto.ClaudeResponse{Content: []dto.ClaudeMediaMessage{{Type: "tool_use", Name: "lookup", Input: map[string]any{}}}}, meaningful: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			markClaudeMeaningfulOutput(info, test.response)
			assert.Equal(t, test.meaningful, info.HasMeaningfulOutput())
		})
	}
}
