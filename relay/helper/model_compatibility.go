package helper

import "strings"

func isClaude4Model(model string) bool {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(normalizedModel, "claude-opus-4") ||
		strings.Contains(normalizedModel, "claude-sonnet-4") ||
		strings.Contains(normalizedModel, "claude-haiku-4")
}

// Claude 4 models reject the legacy top_k sampling parameter.
func ClaudeModelRejectsTopK(model string) bool {
	return isClaude4Model(model)
}

// Claude 4 models use the provider default and reject the deprecated temperature parameter.
func ClaudeModelRejectsTemperature(model string) bool {
	return isClaude4Model(model)
}
