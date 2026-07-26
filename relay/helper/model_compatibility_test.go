package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaudeModelRejectsSamplingParams(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "opus 4.7", model: "claude-opus-4-7", want: true},
		{name: "bedrock opus 4.8", model: "anthropic.claude-opus-4-8", want: true},
		{name: "custom prefixed opus 5", model: "[特价aws]claude-opus-5", want: true},
		{name: "sonnet 5", model: "claude-sonnet-5", want: true},
		{name: "future 4.x", model: "claude-sonnet-4-10", want: true},
		{name: "mythos preview", model: "claude-mythos-preview", want: true},
		{name: "case insensitive", model: "CLAUDE-OPUS-4-7", want: true},
		{name: "opus 4.6", model: "claude-opus-4-6", want: false},
		{name: "sonnet 4.6", model: "anthropic/claude-sonnet-4-6", want: false},
		{name: "haiku 4.5", model: "claude-haiku-4-5-20251001", want: false},
		{name: "claude 3", model: "claude-3-5-sonnet", want: false},
		{name: "gemini", model: "gemini-3-pro", want: false},
		{name: "gpt", model: "gpt-5.5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClaudeModelRejectsSamplingParams(tt.model))
		})
	}
}
