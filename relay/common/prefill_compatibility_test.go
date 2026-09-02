package common

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyPrefillCompatibilityOpenAI(t *testing.T) {
	tests := []struct {
		name        string
		messages    []dto.Message
		wantChanged bool
	}{
		{name: "empty messages", messages: nil, wantChanged: false},
		{
			name: "user turn already last",
			messages: []dto.Message{
				{Role: "user", Content: "hello"},
			},
			wantChanged: false,
		},
		{
			name: "assistant text turn",
			messages: []dto.Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "prefill"},
			},
			wantChanged: true,
		},
		{
			name: "assistant tool call",
			messages: []dto.Message{
				{
					Role:      "assistant",
					ToolCalls: json.RawMessage(`[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]`),
				},
			},
			wantChanged: false,
		},
		{
			name: "assistant empty tool calls",
			messages: []dto.Message{
				{Role: "assistant", Content: "prefill", ToolCalls: json.RawMessage(`[]`)},
			},
			wantChanged: true,
		},
		{
			name: "assistant malformed tool calls",
			messages: []dto.Message{
				{Role: "assistant", Content: "prefill", ToolCalls: json.RawMessage(`{"unexpected":true}`)},
			},
			wantChanged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.GeneralOpenAIRequest{Messages: tt.messages}
			originalLength := len(request.Messages)

			changed := ApplyPrefillCompatibility(request)

			assert.Equal(t, tt.wantChanged, changed)
			if !tt.wantChanged {
				assert.Len(t, request.Messages, originalLength)
				return
			}
			require.Len(t, request.Messages, originalLength+1)
			assert.Equal(t, "user", request.Messages[originalLength].Role)
			assert.Equal(t, ".", request.Messages[originalLength].Content)
			assert.False(t, ApplyPrefillCompatibility(request), "a second pass must not append twice")
			assert.Len(t, request.Messages, originalLength+1)
		})
	}
}

func TestApplyPrefillCompatibilityClaude(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		request := &dto.ClaudeRequest{}

		assert.False(t, ApplyPrefillCompatibility(request))
		assert.Empty(t, request.Messages)
	})

	t.Run("user turn already last", func(t *testing.T) {
		request := &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "ready"},
		}}

		assert.False(t, ApplyPrefillCompatibility(request))
		assert.Len(t, request.Messages, 1)
	})

	t.Run("assistant text turn", func(t *testing.T) {
		request := &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "prefill"},
		}}

		require.True(t, ApplyPrefillCompatibility(request))
		require.Len(t, request.Messages, 3)
		assert.Equal(t, dto.ClaudeMessage{Role: "user", Content: "."}, request.Messages[2])
		assert.False(t, ApplyPrefillCompatibility(request))
	})

	t.Run("assistant tool use", func(t *testing.T) {
		request := &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "tool_use", Id: "tool_1", Name: "lookup"},
				},
			},
		}}

		assert.False(t, ApplyPrefillCompatibility(request))
		assert.Len(t, request.Messages, 1)
	})

	t.Run("assistant server tool use", func(t *testing.T) {
		request := &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "server_tool_use", Id: "tool_1", Name: "web_search"},
				},
			},
		}}

		assert.False(t, ApplyPrefillCompatibility(request))
		assert.Len(t, request.Messages, 1)
	})
}

func TestApplyPrefillCompatibilityGemini(t *testing.T) {
	request := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: []dto.GeminiPart{{Text: "hello"}}},
			{Role: "model", Parts: []dto.GeminiPart{{Text: "prefill"}}},
		},
		Requests: []dto.GeminiChatRequest{
			{
				Contents: []dto.GeminiChatContent{
					{Role: "model", Parts: []dto.GeminiPart{{Text: "nested prefill"}}},
				},
			},
			{
				Contents: []dto.GeminiChatContent{
					{Role: "user", Parts: []dto.GeminiPart{{Text: "already ready"}}},
				},
			},
		},
	}

	require.True(t, ApplyPrefillCompatibility(request))
	require.Len(t, request.Contents, 3)
	assert.Equal(t, "user", request.Contents[2].Role)
	assert.Equal(t, ".", request.Contents[2].Parts[0].Text)
	require.Len(t, request.Requests[0].Contents, 2)
	assert.Equal(t, "user", request.Requests[0].Contents[1].Role)
	assert.Equal(t, ".", request.Requests[0].Contents[1].Parts[0].Text)
	assert.Len(t, request.Requests[1].Contents, 1)
	assert.False(t, ApplyPrefillCompatibility(request))
}

func TestApplyPrefillCompatibilityGeminiLeavesEmptyAndUserTurnsUnchanged(t *testing.T) {
	tests := []struct {
		name     string
		contents []dto.GeminiChatContent
	}{
		{name: "empty contents"},
		{
			name: "user turn already last",
			contents: []dto.GeminiChatContent{
				{Role: "user", Parts: []dto.GeminiPart{{Text: "ready"}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.GeminiChatRequest{Contents: tt.contents}

			assert.False(t, ApplyPrefillCompatibility(request))
			assert.Equal(t, tt.contents, request.Contents)
		})
	}
}

func TestApplyPrefillCompatibilityGeminiSkipsFunctionCall(t *testing.T) {
	request := &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{
		{
			Role: "model",
			Parts: []dto.GeminiPart{
				{FunctionCall: &dto.FunctionCall{FunctionName: "lookup", Arguments: map[string]any{}}},
			},
		},
	}}

	assert.False(t, ApplyPrefillCompatibility(request))
	assert.Len(t, request.Contents, 1)
}

func TestApplyPrefillCompatibilityResponses(t *testing.T) {
	request := &dto.OpenAIResponsesRequest{
		Input: json.RawMessage(`[{"type":"message","role":"user","content":"hello"},{"type":"message","role":"assistant","content":"prefill"}]`),
	}

	require.True(t, ApplyPrefillCompatibility(request))
	assert.Equal(t, int64(3), gjson.GetBytes(request.Input, "#").Int())
	assert.Equal(t, "user", gjson.GetBytes(request.Input, "2.role").String())
	assert.Equal(t, ".", gjson.GetBytes(request.Input, "2.content").String())
	assert.False(t, ApplyPrefillCompatibility(request))

	toolRequest := &dto.OpenAIResponsesRequest{
		Input: json.RawMessage(`[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]`),
	}
	assert.False(t, ApplyPrefillCompatibility(toolRequest))
	assert.Equal(t, int64(1), gjson.GetBytes(toolRequest.Input, "#").Int())

	stringRequest := &dto.OpenAIResponsesRequest{Input: json.RawMessage(`"hello"`)}
	assert.False(t, ApplyPrefillCompatibility(stringRequest))

	userRequest := &dto.OpenAIResponsesRequest{
		Input: json.RawMessage(`[{"type":"message","role":"user","content":"ready"}]`),
	}
	assert.False(t, ApplyPrefillCompatibility(userRequest))
	assert.Equal(t, int64(1), gjson.GetBytes(userRequest.Input, "#").Int())

	emptyRequest := &dto.OpenAIResponsesRequest{Input: json.RawMessage(`[]`)}
	assert.False(t, ApplyPrefillCompatibility(emptyRequest))
	assert.Equal(t, int64(0), gjson.GetBytes(emptyRequest.Input, "#").Int())
}

func TestApplyPrefillCompatibilityJSONPreservesUnknownFields(t *testing.T) {
	tests := []struct {
		name       string
		format     types.RelayFormat
		input      string
		countPath  string
		rolePath   string
		textPath   string
		vendorPath string
	}{
		{
			name:       "openai",
			format:     types.RelayFormatOpenAI,
			input:      `{"vendor_top":{"keep":true},"messages":[{"role":"assistant","content":"prefill","vendor_message":7}]}`,
			countPath:  "messages.#",
			rolePath:   "messages.1.role",
			textPath:   "messages.1.content",
			vendorPath: "messages.0.vendor_message",
		},
		{
			name:       "claude",
			format:     types.RelayFormatClaude,
			input:      `{"vendor_top":{"keep":true},"messages":[{"role":"assistant","content":"prefill","vendor_message":7}]}`,
			countPath:  "messages.#",
			rolePath:   "messages.1.role",
			textPath:   "messages.1.content",
			vendorPath: "messages.0.vendor_message",
		},
		{
			name:       "gemini",
			format:     types.RelayFormatGemini,
			input:      `{"vendor_top":{"keep":true},"contents":[{"role":"model","parts":[{"text":"prefill"}],"vendor_message":7}]}`,
			countPath:  "contents.#",
			rolePath:   "contents.1.role",
			textPath:   "contents.1.parts.0.text",
			vendorPath: "contents.0.vendor_message",
		},
		{
			name:       "responses",
			format:     types.RelayFormatOpenAIResponses,
			input:      `{"vendor_top":{"keep":true},"input":[{"type":"message","role":"assistant","content":"prefill","vendor_message":7}]}`,
			countPath:  "input.#",
			rolePath:   "input.1.role",
			textPath:   "input.1.content",
			vendorPath: "input.0.vendor_message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, changed, err := ApplyPrefillCompatibilityJSON([]byte(tt.input), tt.format)
			require.NoError(t, err)
			require.True(t, changed)
			assert.Equal(t, int64(2), gjson.GetBytes(updated, tt.countPath).Int())
			assert.Equal(t, "user", gjson.GetBytes(updated, tt.rolePath).String())
			assert.Equal(t, ".", gjson.GetBytes(updated, tt.textPath).String())
			assert.True(t, gjson.GetBytes(updated, "vendor_top.keep").Bool())
			assert.Equal(t, int64(7), gjson.GetBytes(updated, tt.vendorPath).Int())

			second, secondChanged, err := ApplyPrefillCompatibilityJSON(updated, tt.format)
			require.NoError(t, err)
			assert.False(t, secondChanged)
			assert.Equal(t, string(updated), string(second))
		})
	}
}

func TestApplyPrefillCompatibilityJSONGeminiBatch(t *testing.T) {
	input := []byte(`{"requests":[{"vendor_nested":true,"contents":[{"role":"model","parts":[{"text":"prefill"}]}]},{"contents":[{"role":"user","parts":[{"text":"ready"}]}]}]}`)

	updated, changed, err := ApplyPrefillCompatibilityJSON(input, types.RelayFormatGemini)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, int64(2), gjson.GetBytes(updated, "requests.0.contents.#").Int())
	assert.Equal(t, "user", gjson.GetBytes(updated, "requests.0.contents.1.role").String())
	assert.Equal(t, ".", gjson.GetBytes(updated, "requests.0.contents.1.parts.0.text").String())
	assert.True(t, gjson.GetBytes(updated, "requests.0.vendor_nested").Bool())
	assert.Equal(t, int64(1), gjson.GetBytes(updated, "requests.1.contents.#").Int())
}

func TestApplyPrefillCompatibilityJSONSkipsPendingTools(t *testing.T) {
	tests := []struct {
		name   string
		format types.RelayFormat
		input  string
	}{
		{
			name:   "openai tool call",
			format: types.RelayFormatOpenAI,
			input:  `{"messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}]}`,
		},
		{
			name:   "claude tool use",
			format: types.RelayFormatClaude,
			input:  `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}]}]}`,
		},
		{
			name:   "gemini function call",
			format: types.RelayFormatGemini,
			input:  `{"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{}}}]}]}`,
		},
		{
			name:   "responses function call",
			format: types.RelayFormatOpenAIResponses,
			input:  `{"input":[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, changed, err := ApplyPrefillCompatibilityJSON([]byte(tt.input), tt.format)
			require.NoError(t, err)
			assert.False(t, changed)
			assert.JSONEq(t, tt.input, string(updated))
		})
	}
}

func TestApplyPrefillCompatibilityJSONRejectsInvalidRoot(t *testing.T) {
	_, _, err := ApplyPrefillCompatibilityJSON([]byte(`{"messages":`), types.RelayFormatOpenAI)
	require.Error(t, err)
}
