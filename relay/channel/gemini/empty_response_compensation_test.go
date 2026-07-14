package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/assert"
)

func TestMarkGeminiMeaningfulOutput(t *testing.T) {
	tests := []struct {
		name       string
		part       dto.GeminiPart
		meaningful bool
	}{
		{name: "empty", part: dto.GeminiPart{}, meaningful: false},
		{name: "whitespace", part: dto.GeminiPart{Text: "   "}, meaningful: false},
		{name: "text", part: dto.GeminiPart{Text: "answer"}, meaningful: true},
		{name: "function call", part: dto.GeminiPart{FunctionCall: &dto.FunctionCall{FunctionName: "lookup"}}, meaningful: true},
		{name: "function response", part: dto.GeminiPart{FunctionResponse: &dto.GeminiFunctionResponse{Name: "lookup"}}, meaningful: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			response := &dto.GeminiChatResponse{
				Candidates: []dto.GeminiChatCandidate{{
					Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{test.part}},
				}},
			}
			markGeminiMeaningfulOutput(info, response)
			assert.Equal(t, test.meaningful, info.HasMeaningfulOutput())
		})
	}
}
