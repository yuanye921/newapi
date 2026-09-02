package common

import (
	"encoding/json"
	"strings"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
)

const prefillCompatibilityPlaceholder = "."

// ApplyPrefillCompatibility appends a minimal user turn when a supported text
// request ends with a model turn. The operation is idempotent and deliberately
// skips unfinished tool calls so it does not break the tool-result sequence.
func ApplyPrefillCompatibility(request any) bool {
	switch typed := request.(type) {
	case *dto.GeneralOpenAIRequest:
		return applyOpenAIPrefillCompatibility(typed)
	case *dto.ClaudeRequest:
		return applyClaudePrefillCompatibility(typed)
	case *dto.GeminiChatRequest:
		return applyGeminiPrefillCompatibility(typed)
	case *dto.OpenAIResponsesRequest:
		return applyResponsesPrefillCompatibility(typed)
	default:
		return false
	}
}

// ApplyPrefillCompatibilityJSON applies the same compatibility rule to a raw
// pass-through or final wire request while retaining unrecognized JSON fields.
func ApplyPrefillCompatibilityJSON(data []byte, format types.RelayFormat) ([]byte, bool, error) {
	var root map[string]json.RawMessage
	if err := appcommon.Unmarshal(data, &root); err != nil {
		return nil, false, err
	}

	var changed bool
	switch format {
	case types.RelayFormatOpenAI:
		changed = appendOpenAIUserTurnJSON(root)
	case types.RelayFormatClaude:
		changed = appendClaudeUserTurnJSON(root)
	case types.RelayFormatGemini:
		changed = applyGeminiPrefillCompatibilityJSON(root)
	case types.RelayFormatOpenAIResponses:
		changed = appendResponsesUserTurnJSON(root)
	default:
		return data, false, nil
	}

	if !changed {
		return data, false, nil
	}
	updated, err := appcommon.Marshal(root)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func applyOpenAIPrefillCompatibility(request *dto.GeneralOpenAIRequest) bool {
	if request == nil || len(request.Messages) == 0 {
		return false
	}
	last := &request.Messages[len(request.Messages)-1]
	if last.Role != "assistant" || openAIMessageHasToolCalls(last) {
		return false
	}
	request.Messages = append(request.Messages, dto.Message{
		Role:    "user",
		Content: prefillCompatibilityPlaceholder,
	})
	return true
}

func openAIMessageHasToolCalls(message *dto.Message) bool {
	if message == nil {
		return false
	}
	raw := strings.TrimSpace(string(message.ToolCalls))
	if raw == "" || raw == "null" {
		return false
	}
	var toolCalls []json.RawMessage
	if err := appcommon.Unmarshal(message.ToolCalls, &toolCalls); err != nil {
		return true
	}
	return len(toolCalls) > 0
}

func applyClaudePrefillCompatibility(request *dto.ClaudeRequest) bool {
	if request == nil || len(request.Messages) == 0 {
		return false
	}
	last := &request.Messages[len(request.Messages)-1]
	if last.Role != "assistant" || claudeMessageHasToolUse(last) {
		return false
	}
	request.Messages = append(request.Messages, dto.ClaudeMessage{
		Role:    "user",
		Content: prefillCompatibilityPlaceholder,
	})
	return true
}

func claudeMessageHasToolUse(message *dto.ClaudeMessage) bool {
	if message == nil || message.Content == nil || message.IsStringContent() {
		return false
	}
	content, err := message.ParseContent()
	if err != nil {
		return true
	}
	for _, part := range content {
		if part.Type == "tool_use" || strings.HasSuffix(part.Type, "_tool_use") {
			return true
		}
	}
	return false
}

func applyGeminiPrefillCompatibility(request *dto.GeminiChatRequest) bool {
	if request == nil {
		return false
	}
	changed := appendGeminiUserTurn(&request.Contents)
	for i := range request.Requests {
		if applyGeminiPrefillCompatibility(&request.Requests[i]) {
			changed = true
		}
	}
	return changed
}

func appendGeminiUserTurn(contents *[]dto.GeminiChatContent) bool {
	if contents == nil || len(*contents) == 0 {
		return false
	}
	last := &(*contents)[len(*contents)-1]
	if last.Role != "model" {
		return false
	}
	for _, part := range last.Parts {
		if part.FunctionCall != nil {
			return false
		}
	}
	*contents = append(*contents, dto.GeminiChatContent{
		Role: "user",
		Parts: []dto.GeminiPart{
			{Text: prefillCompatibilityPlaceholder},
		},
	})
	return true
}

func applyResponsesPrefillCompatibility(request *dto.OpenAIResponsesRequest) bool {
	if request == nil {
		return false
	}
	updated, changed := appendResponsesUserTurn(request.Input)
	if changed {
		request.Input = updated
	}
	return changed
}

func appendOpenAIUserTurnJSON(root map[string]json.RawMessage) bool {
	var messages []json.RawMessage
	if err := appcommon.Unmarshal(root["messages"], &messages); err != nil || len(messages) == 0 {
		return false
	}
	var last dto.Message
	if err := appcommon.Unmarshal(messages[len(messages)-1], &last); err != nil ||
		last.Role != "assistant" || openAIMessageHasToolCalls(&last) {
		return false
	}
	return appendRawUserMessage(root, "messages", messages)
}

func appendClaudeUserTurnJSON(root map[string]json.RawMessage) bool {
	var messages []json.RawMessage
	if err := appcommon.Unmarshal(root["messages"], &messages); err != nil || len(messages) == 0 {
		return false
	}
	var last dto.ClaudeMessage
	if err := appcommon.Unmarshal(messages[len(messages)-1], &last); err != nil ||
		last.Role != "assistant" || claudeMessageHasToolUse(&last) {
		return false
	}
	return appendRawUserMessage(root, "messages", messages)
}

func appendRawUserMessage(root map[string]json.RawMessage, field string, messages []json.RawMessage) bool {
	placeholder, err := appcommon.Marshal(map[string]any{
		"role":    "user",
		"content": prefillCompatibilityPlaceholder,
	})
	if err != nil {
		return false
	}
	messages = append(messages, placeholder)
	updated, err := appcommon.Marshal(messages)
	if err != nil {
		return false
	}
	root[field] = updated
	return true
}

func applyGeminiPrefillCompatibilityJSON(root map[string]json.RawMessage) bool {
	changed := appendGeminiUserTurnJSON(root)

	var requests []json.RawMessage
	if err := appcommon.Unmarshal(root["requests"], &requests); err != nil {
		return changed
	}
	requestsChanged := false
	for i := range requests {
		var nested map[string]json.RawMessage
		if err := appcommon.Unmarshal(requests[i], &nested); err != nil {
			continue
		}
		if !applyGeminiPrefillCompatibilityJSON(nested) {
			continue
		}
		updated, err := appcommon.Marshal(nested)
		if err != nil {
			continue
		}
		requests[i] = updated
		requestsChanged = true
	}
	if !requestsChanged {
		return changed
	}
	updated, err := appcommon.Marshal(requests)
	if err != nil {
		return changed
	}
	root["requests"] = updated
	return true
}

func appendGeminiUserTurnJSON(root map[string]json.RawMessage) bool {
	var contents []json.RawMessage
	if err := appcommon.Unmarshal(root["contents"], &contents); err != nil || len(contents) == 0 {
		return false
	}
	var last dto.GeminiChatContent
	if err := appcommon.Unmarshal(contents[len(contents)-1], &last); err != nil || last.Role != "model" {
		return false
	}
	for _, part := range last.Parts {
		if part.FunctionCall != nil {
			return false
		}
	}
	placeholder, err := appcommon.Marshal(dto.GeminiChatContent{
		Role: "user",
		Parts: []dto.GeminiPart{
			{Text: prefillCompatibilityPlaceholder},
		},
	})
	if err != nil {
		return false
	}
	contents = append(contents, placeholder)
	updated, err := appcommon.Marshal(contents)
	if err != nil {
		return false
	}
	root["contents"] = updated
	return true
}

func appendResponsesUserTurnJSON(root map[string]json.RawMessage) bool {
	updated, changed := appendResponsesUserTurn(root["input"])
	if changed {
		root["input"] = updated
	}
	return changed
}

func appendResponsesUserTurn(input json.RawMessage) (json.RawMessage, bool) {
	if appcommon.GetJsonType(input) != "array" {
		return input, false
	}
	var items []json.RawMessage
	if err := appcommon.Unmarshal(input, &items); err != nil || len(items) == 0 {
		return input, false
	}
	var last struct {
		Type string `json:"type,omitempty"`
		Role string `json:"role,omitempty"`
	}
	if err := appcommon.Unmarshal(items[len(items)-1], &last); err != nil || last.Role != "assistant" {
		return input, false
	}
	if last.Type != "" && last.Type != "message" {
		return input, false
	}
	placeholder, err := appcommon.Marshal(map[string]any{
		"role":    "user",
		"content": prefillCompatibilityPlaceholder,
	})
	if err != nil {
		return input, false
	}
	items = append(items, placeholder)
	updated, err := appcommon.Marshal(items)
	if err != nil {
		return input, false
	}
	return updated, true
}
