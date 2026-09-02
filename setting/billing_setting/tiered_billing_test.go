package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmokeTestExprAcceptsRequestPricing(t *testing.T) {
	err := SmokeTestExpr(`len >= 200000 || c >= 8000 ? tier("large", request(0.2)) : tier("base", request(0.15))`)

	require.NoError(t, err)
}

func TestSmokeTestExprRejectsInvalidResults(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{name: "negative request price", expr: `request(-0.01)`},
		{name: "positive infinity", expr: `1e308 * 1e308`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SmokeTestExpr(tt.expr)
			assert.Error(t, err)
		})
	}
}
