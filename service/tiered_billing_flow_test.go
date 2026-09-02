package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTieredRequestBillingFlowUsesFinalGroupAndLogsRuleTrace(t *testing.T) {
	const exprStr = `(
len >= 500000 || c >= 20000 ? tier("500k", request(0.5)) :
len >= 400000 || c >= 16000 ? tier("400k", request(0.4)) :
len >= 300000 || c >= 12000 ? tier("300k", request(0.3)) :
len >= 200000 || c >= 8000 ? tier("200k", request(0.2)) :
tier("base", request(0.15))
) * (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18 ? 2 : 1)`

	evaluatedAt := time.Date(2026, 9, 3, 7, 30, 0, 0, time.UTC)
	requestInput := billingexpr.RequestInput{EvaluatedAt: evaluatedAt}
	estimatedParams := billingexpr.TokenParams{Len: 199_999, C: 7_999}
	estimatedCost, estimatedTrace, err := billingexpr.RunExprWithRequest(
		exprStr,
		estimatedParams,
		requestInput,
	)
	require.NoError(t, err)
	require.Equal(t, "base", estimatedTrace.MatchedTier)
	require.Equal(t, 300_000.0, estimatedCost)

	const quotaPerUnit = 500_000.0
	estimatedQuotaBeforeGroup := estimatedCost / 1_000_000 * quotaPerUnit
	estimatedQuotaAfterGroup, err := billingexpr.QuotaRoundStrict(
		estimatedQuotaBeforeGroup,
	)
	require.NoError(t, err)
	require.Equal(t, 150_000, estimatedQuotaAfterGroup)

	billing := &recordingBillingSettler{
		preConsumedQuota: estimatedQuotaAfterGroup,
	}
	relayInfo := &relaycommon.RelayInfo{
		Billing:               billing,
		FinalPreConsumedQuota: estimatedQuotaAfterGroup,
		BillingRequestInput:   &requestInput,
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:               "tiered_expr",
			ExprString:                exprStr,
			ExprHash:                  billingexpr.ExprHashString(exprStr),
			GroupRatio:                1,
			EstimatedPromptTokens:     int(estimatedParams.Len),
			EstimatedCompletionTokens: int(estimatedParams.C),
			EstimatedQuotaBeforeGroup: estimatedQuotaBeforeGroup,
			EstimatedQuotaAfterGroup:  estimatedQuotaAfterGroup,
			EstimatedTier:             estimatedTrace.MatchedTier,
			QuotaPerUnit:              quotaPerUnit,
		},
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.5},
		},
	}

	apiErr := PrepareTieredBillingForSelectedGroup(nil, relayInfo)
	require.Nil(t, apiErr)
	assert.Equal(t, []int{225_000}, billing.reserveTargets)
	assert.Equal(t, 225_000, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, 1.5, relayInfo.TieredBillingSnapshot.GroupRatio)
	assert.Equal(t, evaluatedAt, relayInfo.BillingRequestInput.EvaluatedAt)

	ok, finalQuota, result := TryTieredSettle(
		relayInfo,
		billingexpr.TokenParams{Len: 199_999, C: 8_000},
	)
	require.True(t, ok)
	require.NotNil(t, result)
	assert.Equal(t, 300_000, finalQuota)
	assert.GreaterOrEqual(t, finalQuota, 0)
	assert.Equal(t, "200k", result.MatchedTier)
	assert.True(t, result.CrossedTier)
	assert.Nil(t, result.Clamp)
	assert.Equal(t, 200_000.0, result.ActualQuotaBeforeGroup)
	assert.Equal(t, 1.5, relayInfo.TieredBillingSnapshot.GroupRatio)

	other := map[string]interface{}{}
	InjectTieredBillingInfo(other, relayInfo, result)
	assert.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, "200k", other["matched_tier"])
	assert.Equal(t, []billingexpr.RequestRuleTrace{
		{
			Cond:       `hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18`,
			Multiplier: 2,
			Matched:    true,
		},
	}, other["request_rules"])
}
