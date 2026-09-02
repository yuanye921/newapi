package service

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestInjectTieredBillingInfoIncludesRequestRuleTrace(t *testing.T) {
	rules := []billingexpr.RequestRuleTrace{
		{Cond: `param("service_tier") == "fast"`, Multiplier: 2, Matched: true},
		{Cond: `hour("Asia/Shanghai") >= 14`, Multiplier: 1.5, Matched: false},
	}
	other := map[string]interface{}{}
	relayInfo := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base", request(0.15))`},
	}

	InjectTieredBillingInfo(other, relayInfo, &billingexpr.TieredResult{
		MatchedTier:  "base",
		RequestRules: rules,
	})

	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, "base", other["matched_tier"])
	assert.Equal(t, rules, other["request_rules"])
}

func TestInjectTieredBillingInfoOmitsEmptyRequestRuleTrace(t *testing.T) {
	other := map[string]interface{}{}
	relayInfo := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("base", request(0.15))`},
	}

	InjectTieredBillingInfo(other, relayInfo, &billingexpr.TieredResult{MatchedTier: "base"})

	_, exists := other["request_rules"]
	assert.False(t, exists)
}
