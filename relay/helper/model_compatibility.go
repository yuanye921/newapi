package helper

import (
	"strconv"
	"strings"
)

// Claude models released after Opus 4.6 reject temperature, top_p, and top_k.
func ClaudeModelRejectsSamplingParams(model string) bool {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(normalizedModel, "claude-mythos") {
		return true
	}

	claudeIndex := strings.Index(normalizedModel, "claude-")
	if claudeIndex < 0 {
		return false
	}
	parts := strings.Split(normalizedModel[claudeIndex+len("claude-"):], "-")
	if len(parts) == 0 {
		return false
	}

	versionIndex := 0
	major, err := strconv.Atoi(parts[versionIndex])
	if err != nil {
		versionIndex = 1
		if len(parts) <= versionIndex {
			return false
		}
		major, err = strconv.Atoi(parts[versionIndex])
		if err != nil {
			return false
		}
	}

	minor := 0
	if len(parts) > versionIndex+1 {
		minor, _ = strconv.Atoi(parts[versionIndex+1])
	}
	return major > 4 || (major == 4 && minor >= 7)
}
