package billingexpr

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestPriceUsesExistingCostScale(t *testing.T) {
	cost, trace, err := RunExpr(`tier("base", request(0.15))`, TokenParams{})

	require.NoError(t, err)
	assert.Equal(t, 150000.0, cost)
	assert.Equal(t, "base", trace.MatchedTier)
}

func TestRequestPriceTierBoundaries(t *testing.T) {
	exprStr := `
len >= 500000 || c >= 20000 ? tier("500k", request(0.5)) :
len >= 400000 || c >= 16000 ? tier("400k", request(0.4)) :
len >= 300000 || c >= 12000 ? tier("300k", request(0.3)) :
len >= 200000 || c >= 8000 ? tier("200k", request(0.2)) :
tier("base", request(0.15))`

	tests := []struct {
		name string
		len  float64
		c    float64
		tier string
		cost float64
	}{
		{name: "base", len: 199999, c: 7999, tier: "base", cost: 150000},
		{name: "200k by input", len: 200000, c: 0, tier: "200k", cost: 200000},
		{name: "300k by output", len: 0, c: 12000, tier: "300k", cost: 300000},
		{name: "400k boundary", len: 400000, c: 0, tier: "400k", cost: 400000},
		{name: "500k priority", len: 500000, c: 20000, tier: "500k", cost: 500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, trace, err := RunExpr(exprStr, TokenParams{Len: tt.len, C: tt.c})

			require.NoError(t, err)
			assert.Equal(t, tt.cost, cost)
			assert.Equal(t, tt.tier, trace.MatchedTier)
		})
	}
}

func TestRequestPriceRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		params TokenParams
	}{
		{name: "negative", expr: `request(-0.01)`},
		{name: "nan", expr: `request(p)`, params: TokenParams{P: math.NaN()}},
		{name: "positive infinity", expr: `request(p)`, params: TokenParams{P: math.Inf(1)}},
		{name: "negative infinity", expr: `request(p)`, params: TokenParams{P: math.Inf(-1)}},
		{name: "scale overflow", expr: `request(p)`, params: TokenParams{P: math.MaxFloat64}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := RunExpr(tt.expr, tt.params)
			assert.Error(t, err)
		})
	}
}

func TestRequestPriceAllowsZero(t *testing.T) {
	cost, _, err := RunExpr(`request(0)`, TokenParams{})

	require.NoError(t, err)
	assert.Zero(t, cost)
}

func TestRequestPriceDoesNotChangeTokenExpressions(t *testing.T) {
	cost, trace, err := RunExpr(
		`p <= 200000 ? tier("standard", p * 1.5 + c * 7.5) : tier("long", p * 3 + c * 11.25)`,
		TokenParams{P: 100000, C: 5000},
	)

	require.NoError(t, err)
	assert.Equal(t, 187500.0, cost)
	assert.Equal(t, "standard", trace.MatchedTier)
}
