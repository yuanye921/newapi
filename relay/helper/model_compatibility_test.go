package helper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClaudeModelRejectsTopK(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "opus 4", model: "claude-opus-4-6", want: true},
		{name: "prefixed sonnet 4", model: "anthropic/claude-sonnet-4-5", want: true},
		{name: "haiku 4", model: "claude-haiku-4-5-20251001", want: true},
		{name: "case insensitive", model: "CLAUDE-OPUS-4-6", want: true},
		{name: "claude 3", model: "claude-3-5-sonnet", want: false},
		{name: "gemini", model: "gemini-3-pro", want: false},
		{name: "gpt", model: "gpt-5.5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClaudeModelRejectsTopK(tt.model))
		})
	}
}

func TestClaudeModelRejectsTemperature(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{name: "opus 4", model: "claude-opus-4-6", want: true},
		{name: "prefixed sonnet 4", model: "anthropic/claude-sonnet-4-5", want: true},
		{name: "aws opus 4", model: "anthropic.claude-opus-4-7-v1:0", want: true},
		{name: "case insensitive", model: "CLAUDE-HAIKU-4-5", want: true},
		{name: "claude 3", model: "claude-3-5-sonnet", want: false},
		{name: "gemini", model: "gemini-3-pro", want: false},
		{name: "gpt", model: "gpt-5.5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClaudeModelRejectsTemperature(tt.model))
		})
	}
}
