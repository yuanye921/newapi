package billingexpr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestInputFreezesTimeHelpers(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		want float64
	}{
		{name: "early", time: time.Date(2026, 9, 3, 3, 10, 0, 0, time.UTC), want: 3},
		{name: "late", time: time.Date(2026, 9, 3, 19, 10, 0, 0, time.UTC), want: 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, _, err := RunExprWithRequest(
				`hour("UTC") * 1.0`,
				TokenParams{},
				RequestInput{EvaluatedAt: tt.time},
			)

			require.NoError(t, err)
			assert.Equal(t, tt.want, cost)
		})
	}
}

func TestRequestRuleTraceIncludesMatchedAndUnmatchedFactors(t *testing.T) {
	exprStr := `(tier("base", p * 2)) * (param("service_tier") == "fast" ? 2 : 1) * (has(header("anthropic-beta"), "fast-mode") ? 2.5 : 1)`
	cost, trace, err := RunExprWithRequest(
		exprStr,
		TokenParams{P: 10},
		RequestInput{Body: []byte(`{"service_tier":"fast"}`)},
	)

	require.NoError(t, err)
	assert.Equal(t, 40.0, cost)
	assert.Equal(t, "base", trace.MatchedTier)
	assert.Equal(t, []RequestRuleTrace{
		{Cond: `param("service_tier") == "fast"`, Multiplier: 2, Matched: true},
		{Cond: `has(header("anthropic-beta"), "fast-mode")`, Multiplier: 2.5, Matched: false},
	}, trace.RequestRules)
}

func TestRequestRuleTracePreservesIntegerConditionalType(t *testing.T) {
	cost, trace, err := RunExprWithRequest(
		`5 % (param("service_tier") == "fast" ? 2 : 1)`,
		TokenParams{},
		RequestInput{Body: []byte(`{"service_tier":"fast"}`)},
	)

	require.NoError(t, err)
	assert.Equal(t, 1.0, cost)
	assert.Equal(t, []RequestRuleTrace{
		{Cond: `param("service_tier") == "fast"`, Multiplier: 2, Matched: true},
	}, trace.RequestRules)
}

func TestRequestRuleTraceIgnoresNonUnitFallback(t *testing.T) {
	cost, trace, err := RunExprWithRequest(
		`10 * (param("service_tier") == "fast" ? 2 : 1.5)`,
		TokenParams{},
		RequestInput{Body: []byte(`{"service_tier":"standard"}`)},
	)

	require.NoError(t, err)
	assert.Equal(t, 15.0, cost)
	assert.Empty(t, trace.RequestRules)
}

func TestRequestRuleInternalTraceFunctionsAreReserved(t *testing.T) {
	for _, exprStr := range []string{`_trace(0, true, 5.0)`, `_trace_int(0, true, 5)`} {
		_, err := CompileFromCache(exprStr)
		assert.ErrorContains(t, err, "reserved for internal use")
	}
}

func TestTieredSettlementCarriesRequestRuleTrace(t *testing.T) {
	exprStr := `tier("base", request(0.2)) * (param("service_tier") == "fast" ? 2 : 1)`
	snapshot := &BillingSnapshot{
		ExprString:    exprStr,
		ExprHash:      ExprHashString(exprStr),
		GroupRatio:    1,
		EstimatedTier: "base",
		QuotaPerUnit:  500000,
		ExprVersion:   1,
	}

	result, err := ComputeTieredQuotaWithRequest(
		snapshot,
		TokenParams{},
		RequestInput{Body: []byte(`{"service_tier":"fast"}`)},
	)

	require.NoError(t, err)
	assert.Equal(t, 200000.0, result.ActualQuotaBeforeGroup)
	assert.Equal(t, 200000, result.ActualQuotaAfterGroup)
	assert.Equal(t, []RequestRuleTrace{
		{Cond: `param("service_tier") == "fast"`, Multiplier: 2, Matched: true},
	}, result.RequestRules)
}
