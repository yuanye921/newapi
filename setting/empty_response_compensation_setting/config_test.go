package empty_response_compensation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateModelRatiosJSON(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty map", value: `{}`, wantErr: false},
		{name: "valid boundaries", value: `{"model-a":1,"model-b":100}`, wantErr: false},
		{name: "invalid json", value: `{`, wantErr: true},
		{name: "empty model", value: `{"":50}`, wantErr: true},
		{name: "ratio too low", value: `{"model-a":0}`, wantErr: true},
		{name: "ratio too high", value: `{"model-a":101}`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateModelRatiosJSON(test.value)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateOption(t *testing.T) {
	require.NoError(t, ValidateOption("enabled", "true"))
	require.NoError(t, ValidateOption("daily_claim_limit", "0"))
	require.NoError(t, ValidateOption("claim_window_days", "1"))
	require.Error(t, ValidateOption("enabled", "yes"))
	require.Error(t, ValidateOption("input_token_threshold", "-1"))
	require.Error(t, ValidateOption("claim_window_days", "0"))
}

func TestGetReturnsIndependentModelRatioMap(t *testing.T) {
	first := Get()
	first.ModelRatios["mutated-only-in-test"] = 50

	second := Get()
	_, exists := second.ModelRatios["mutated-only-in-test"]
	assert.False(t, exists)
}
