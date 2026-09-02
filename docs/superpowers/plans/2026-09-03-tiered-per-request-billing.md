# Tiered Per-Request and Time Billing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:test-driven-development` for every behavior change and `superpowers:verification-before-completion` before reporting completion. Execute tasks in order and preserve the user's existing prefill-compatibility work.

**Goal:** Add a safe, observable per-request tiered billing mode to billing expressions, correct time-window billing, and adapt the selected upstream retry/logging fixes without broadly merging upstream.

**Architecture:** Keep one expression engine and add `request(price)` as a unit conversion primitive. Freeze request-dependent data, including evaluation time, in the existing billing request/snapshot flow; make retries refresh only the selected group's multiplier and reserve; expose structured conditional-rule traces to logs. Extend the existing frontend parser/editor/display rather than creating a second pricing system.

**Tech Stack:** Go 1.22+, Gin, expr-lang/expr, testify, React 19, TypeScript, Bun test, i18next.

**Spec:** `docs/superpowers/specs/2026-09-03-tiered-per-request-billing-design.md`

## Global Constraints

- Follow `pkg/billingexpr/expr.md` and the repository billing-safety rules. All quota conversion must continue through `common.QuotaFromFloatChecked` / existing settlement helpers; do not add bare float-to-int casts.
- Preserve existing token-priced expressions and fixed `ModelPrice` billing byte-for-byte in meaning.
- Preserve the user's unrelated dirty prefill-compatibility files. Stage only the exact paths named by each task. The locale JSON files already contain unrelated changes, so merge keys through the required script and leave those locale files unstaged unless their unrelated hunks can be separated safely.
- Use `common.Marshal` / `common.Unmarshal` for Go JSON work.
- New Go tests use `require` for setup/fatal checks and `assert` for comparisons.
- Use `bun:test`; do not add Vitest or another test dependency.
- The visual editor may round-trip homogeneous `&&` or homogeneous `||` condition groups. Mixed boolean operators remain raw-only rather than being rewritten.
- Request-price expressions produce the engine's existing v1 million-unit cost scale so the established quota conversion path remains unchanged.

---

## Task 1: Add the `request(price)` Billing Primitive

**Files:**

- Create: `pkg/billingexpr/request_price_test.go`
- Create: `setting/billing_setting/tiered_billing_test.go`
- Modify: `pkg/billingexpr/compile.go`
- Modify: `pkg/billingexpr/run.go`
- Modify: `setting/billing_setting/tiered_billing.go`
- Modify: `pkg/billingexpr/expr.md`

### Step 1: Write failing expression and quota tests

Add deterministic tests for the public behavior:

```go
func TestRequestPriceUsesExistingCostScale(t *testing.T) {
    cost, trace, err := RunExpr(`tier("base", request(0.15))`, TokenParams{})
    require.NoError(t, err)
    assert.Equal(t, 150000.0, cost)
    assert.Equal(t, "base", trace.MatchedTier)
}

func TestRequestPriceTierBoundaries(t *testing.T) {
    expr := `len >= 500000 || c >= 20000 ? tier("500k", request(0.5)) :
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
            cost, trace, err := RunExpr(expr, TokenParams{Len: tt.len, C: tt.c})
            require.NoError(t, err)
            assert.Equal(t, tt.cost, cost)
            assert.Equal(t, tt.tier, trace.MatchedTier)
        })
    }
}
```

Also assert that `request(0)` succeeds, `request(-0.01)` returns an error, `request(p)` rejects `TokenParams{P: math.NaN()}` and positive infinity, and a representative existing token expression still returns its current result.

### Step 2: Run the focused test and confirm the red state

Run:

```powershell
go test ./pkg/billingexpr -run 'TestRequestPrice' -count=1
```

Expected: FAIL because `request` is not in the compile/runtime environment.

### Step 3: Register and implement `request(price)`

Add the compile signature in `pkg/billingexpr/compile.go`:

```go
"request": func(float64) (float64, error) { return 0, nil },
```

Add the runtime function in `pkg/billingexpr/run.go`:

```go
"request": func(price float64) (float64, error) {
    if price < 0 || math.IsNaN(price) || math.IsInf(price, 0) {
        return 0, fmt.Errorf("request price must be finite and non-negative")
    }
    scaled := price * 1_000_000
    if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
        return 0, fmt.Errorf("request price is too large")
    }
    return scaled, nil
},
```

Do not perform quota rounding here. `request()` returns expression cost only; settlement continues through the existing checked conversion.

### Step 4: Harden saved-expression smoke validation

In `setting/billing_setting/tiered_billing.go`, reject negative, NaN, and infinite smoke-test outcomes. Add `setting/billing_setting/tiered_billing_test.go` to prove invalid request prices cannot be accepted and a valid reference request-tier expression passes the same smoke validation used by settings updates.

### Step 5: Document the primitive

Update `pkg/billingexpr/expr.md` with:

- `request(price)` syntax and non-negative USD input;
- the v1 million-unit return scale;
- the reference tier expression;
- a note that `request()` and token arithmetic may coexist only in raw mode, while the visual editor intentionally refuses lossy mixed conversion.

### Step 6: Run focused tests and format

Run:

```powershell
gofmt -w pkg\billingexpr\request_price_test.go pkg\billingexpr\compile.go pkg\billingexpr\run.go setting\billing_setting\tiered_billing.go setting\billing_setting\tiered_billing_test.go
go test ./pkg/billingexpr ./setting/billing_setting -run 'TestRequestPrice|Test.*BillingExpr' -count=1
```

Expected: PASS.

### Step 7: Commit only Task 1 files

```powershell
git add -- pkg/billingexpr/request_price_test.go pkg/billingexpr/compile.go pkg/billingexpr/run.go setting/billing_setting/tiered_billing.go setting/billing_setting/tiered_billing_test.go pkg/billingexpr/expr.md
git commit -m "feat: support per-request billing expressions"
```

---

## Task 2: Freeze Evaluation Time and Trace Conditional Multipliers

**Files:**

- Create: `pkg/billingexpr/request_trace_test.go`
- Modify: `relay/helper/billing_expr_request_test.go`
- Create: `service/log_info_generate_test.go`
- Modify: `pkg/billingexpr/types.go`
- Modify: `pkg/billingexpr/compile.go`
- Modify: `pkg/billingexpr/run.go`
- Modify: `pkg/billingexpr/settle.go`
- Modify: `relay/helper/billing_expr_request.go`
- Modify: `service/log_info_generate.go`
- Modify: `pkg/billingexpr/expr.md`

### Step 1: Write failing frozen-time tests

Add a test with a fixed timestamp near a boundary:

```go
request := RequestInput{EvaluatedAt: time.Date(2026, 9, 3, 17, 59, 0, 0, time.FixedZone("CST", 8*60*60))}
cost, _, err := RunExprWithRequest(
    `p * (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18 ? 2 : 1)`,
    TokenParams{PromptTokens: 10},
    request,
)
require.NoError(t, err)
assert.Equal(t, 20.0, cost)
```

Prove the request-input clone used by `relay/helper` retains the exact timestamp. Also test that an omitted timestamp still uses the current-clock fallback without asserting a fragile exact second.

### Step 2: Write failing structured-trace tests

Use an expression with one matching and one non-matching request multiplier. Assert exact trace rows:

```go
assert.Equal(t, []RequestRuleTrace{
    {Cond: `hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18`, Multiplier: 2, Matched: true},
    {Cond: `weekday("Asia/Shanghai") == 0`, Multiplier: 3, Matched: false},
}, trace.RequestRules)
```

Add a log injection test asserting `other["request_rules"]` contains the same ordered structured values. A plain tier expression without conditional request probes must produce no trace rows.

### Step 3: Run the focused tests and confirm the red state

```powershell
go test ./pkg/billingexpr ./relay/helper ./service -run 'Test.*FrozenTime|Test.*RequestRule|Test.*TieredLog' -count=1
```

Expected: FAIL because the timestamp and structured trace fields do not exist.

### Step 4: Add the public trace and timestamp types

In `pkg/billingexpr/types.go` add:

```go
type RequestInput struct {
    Headers     map[string]string
    Body        []byte
    EvaluatedAt time.Time
}

type RequestRuleTrace struct {
    Cond       string  `json:"cond"`
    Multiplier float64 `json:"multiplier"`
    Matched    bool    `json:"matched"`
}
```

Add `RequestRules []RequestRuleTrace` to both `TraceResult` and `TieredResult`.

### Step 5: Instrument request-rule ternaries at compile time

Adapt the behavior from upstream commit `4cf9107` to the local cache/compiler:

- add a `requestRulePatcher` that recognizes multiplier ternaries whose condition contains `param`, `header`, `hour`, `minute`, `weekday`, `month`, or `day`;
- record the condition string and numeric multiplier in cached metadata;
- patch matching float branches to `_trace(index, cond, multiplier)` and integer branches to `_trace_int(...)`;
- reserve `_trace` and `_trace_int` in the compile environment;
- copy cached rule metadata before each run so concurrent requests cannot share mutation.

Do not instrument tier-selection ternaries or arbitrary arithmetic.

### Step 6: Use the frozen time and collect trace matches

In `run.go`, compute once per evaluation:

```go
evaluatedAt := request.EvaluatedAt
if evaluatedAt.IsZero() {
    evaluatedAt = time.Now()
}
```

Change `timeInZone` to accept this time value, and make all five time helpers use it. Runtime `_trace` / `_trace_int` callbacks mark the indexed rule as matched only when the conditional branch is selected.

In `settle.go`, propagate `trace.RequestRules` into `TieredResult` along with the matched tier.

### Step 7: Capture time at billing entry and preserve it in clones

In `relay/helper/billing_expr_request.go`, set `EvaluatedAt: time.Now().UTC()` only when the billing request is first built. The clone path must copy the timestamp unchanged along with defensive copies of headers/body.

### Step 8: Add structured traces to consume logs

In `service/log_info_generate.go`, keep existing `billing_mode`, `expr_b64`, and `matched_tier`, then add `request_rules` only when the result contains rows. Preserve the existing admin-only/log filtering behavior.

### Step 9: Run focused tests and format

```powershell
gofmt -w pkg\billingexpr\types.go pkg\billingexpr\compile.go pkg\billingexpr\run.go pkg\billingexpr\settle.go pkg\billingexpr\request_trace_test.go relay\helper\billing_expr_request.go relay\helper\billing_expr_request_test.go service\log_info_generate.go service\log_info_generate_test.go
go test ./pkg/billingexpr ./relay/helper ./service -run 'Test.*FrozenTime|Test.*RequestRule|Test.*TieredLog' -count=1
```

Expected: PASS, including race-independent ordered trace assertions.

### Step 10: Commit only Task 2 files

```powershell
git add -- pkg/billingexpr/types.go pkg/billingexpr/compile.go pkg/billingexpr/run.go pkg/billingexpr/settle.go pkg/billingexpr/request_trace_test.go relay/helper/billing_expr_request.go relay/helper/billing_expr_request_test.go service/log_info_generate.go service/log_info_generate_test.go pkg/billingexpr/expr.md
git commit -m "fix: freeze and trace dynamic billing rules"
```

---

## Task 3: Refresh Tiered Billing for the Final Retry Group

**Files:**

- Modify: `service/tiered_settle.go`
- Modify: `service/tiered_settle_test.go`
- Modify: `controller/relay.go`
- Modify: `pkg/billingexpr/expr.md`

### Step 1: Write a failing reserve-refresh regression test

Adapt upstream commit `df43f80` to the local `BillingSession` API. Build a recording settler and a frozen snapshot with an initial group ratio, then change `relayInfo.PriceData.GroupRatioInfo.GroupRatio` before retry preparation. Assert:

```go
apiErr := PrepareTieredBillingForSelectedGroup(ctx, relayInfo)
require.Nil(t, apiErr)
assert.Equal(t, expensiveGroupRatio, relayInfo.TieredBillingSnapshot.GroupRatio)
assert.Equal(t, expectedHigherQuota, recordingSettler.LastReservedQuota())
```

Add the inverse case for a cheaper retry and an idempotent same-group call. Confirm the expression hash, request inputs, estimated token counts, and frozen evaluation time remain unchanged.

### Step 2: Run the focused test and confirm the red state

```powershell
go test ./service -run 'TestPrepareTieredBillingForSelectedGroup' -count=1
```

Expected: FAIL because retry preparation does not exist.

### Step 3: Implement group refresh and reserve

In `service/tiered_settle.go` add:

```go
func PrepareTieredBillingForSelectedGroup(c *gin.Context, relayInfo *relaycommon.RelayInfo) *types.NewAPIError
```

The function must:

1. return immediately for non-tiered billing;
2. clone/re-evaluate the existing frozen snapshot with only the currently selected group ratio changed;
3. update `EstimatedQuotaAfterGroup` and `relayInfo.Billing.TieredSnapshot`;
4. call the existing `Billing.Reserve(targetQuota)` so a more expensive retry is funded before sending;
5. return an insufficient-quota API error instead of sending the retry when reserve fails.

At the start of `TryTieredSettle`, refresh the snapshot once more from the selected group so final settlement cannot use a stale first-attempt ratio.

### Step 4: Wire the check into the retry loop

In `controller/relay.go`, call `PrepareTieredBillingForSelectedGroup` after the channel/group has been selected and before `addUsedChannel` and the upstream request. On error, stop retrying and return the billing error.

### Step 5: Run service/controller regression tests

```powershell
gofmt -w service\tiered_settle.go service\tiered_settle_test.go controller\relay.go
go test ./service ./controller -run 'TestPrepareTieredBillingForSelectedGroup|Test.*Tiered.*Retry' -count=1
```

Expected: PASS.

### Step 6: Commit only Task 3 files

```powershell
git add -- service/tiered_settle.go service/tiered_settle_test.go controller/relay.go pkg/billingexpr/expr.md
git commit -m "fix: settle tiered retries with final group"
```

---

## Task 4: Correct and Validate Frontend Time Ranges

**Files:**

- Create: `web/default/src/features/pricing/lib/__tests__/time-rule-expr.test.ts`
- Modify: `web/default/src/features/pricing/lib/billing-expr.ts`

### Step 1: Write failing Bun tests for time semantics

```ts
import { describe, expect, test } from 'bun:test'

test('same-day ranges use AND', () => {
  const expr = buildRequestRuleExpr([{
    conditions: [{
      source: 'time', timeFunc: 'hour', timezone: 'Asia/Shanghai',
      mode: MATCH_RANGE, value: '', rangeStart: '14', rangeEnd: '18',
    }],
    multiplier: '2',
  }])
  expect(expr).toBe(
    '(hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18 ? 2 : 1)',
  )
})

test('overnight ranges use OR', () => {
  const expr = buildRequestRuleExpr([{
    conditions: [{
      source: 'time', timeFunc: 'hour', timezone: 'Asia/Shanghai',
      mode: MATCH_RANGE, value: '', rangeStart: '22', rangeEnd: '6',
    }],
    multiplier: '2',
  }])
  expect(expr).toBe(
    '(hour("Asia/Shanghai") >= 22 || hour("Asia/Shanghai") < 6 ? 2 : 1)',
  )
})
```

Add cases for equal bounds (never match), hours `-1`/`24`, minutes `-1`/`60`, weekday `-1`/`7`, month `0`/`13`, day `0`/`32`, valid boundary values, and parse-build-parse stability for both `&&` and `||` ranges.

### Step 2: Run the focused test and confirm the red state

```powershell
Set-Location web\default
bun test src/features/pricing/lib/__tests__/time-rule-expr.test.ts
```

Expected: FAIL because same-day ranges currently generate an always-true OR expression and range-domain validation is absent.

### Step 3: Port the scoped time fix

Adapt the exact behavior of upstream commit `ac381ac` in `billing-expr.ts`:

- declare valid domains for `hour`, `minute`, `weekday`, `month`, and `day`;
- reject raw comparison values outside the selected function's domain;
- parse both `start <= value && value < end` and `start <= value || value < end` forms;
- merge compatible scalar bounds into the existing range condition model;
- generate `&&` when start is before end;
- generate `||` when start is after end;
- generate an always-false conjunction when bounds are equal.

Do not accept partially parsed or lossy rules.

### Step 4: Run focused tests and frontend type checking

```powershell
bun test src/features/pricing/lib/__tests__/time-rule-expr.test.ts
bun run typecheck
Set-Location ..\..
```

Expected: PASS.

### Step 5: Commit only Task 4 files

```powershell
git add -- web/default/src/features/pricing/lib/billing-expr.ts web/default/src/features/pricing/lib/__tests__/time-rule-expr.test.ts
git commit -m "fix: handle same-day and overnight billing ranges"
```

---

## Task 5: Round-Trip Per-Request Tiers and OR Conditions

**Files:**

- Create: `web/default/src/features/pricing/lib/__tests__/tier-expr.test.ts`
- Modify: `web/default/src/features/pricing/lib/billing-expr.ts`
- Modify: `web/default/src/features/pricing/lib/tier-expr.ts`

### Step 1: Write failing parser/generator tests

Define the intended visual model in the test:

```ts
const config: VisualConfig = {
  pricing_unit: 'request',
  tiers: [
    {
      label: '500k',
      condition_logic: 'or',
      conditions: [
        { variable: 'len', operator: '>=', value: 500000 },
        { variable: 'c', operator: '>=', value: 20000 },
      ],
      request_price: 0.5,
    },
    { label: 'base', conditions: [], request_price: 0.15 },
  ],
}
```

Assert:

- generation emits `tier("500k", request(0.5), len >= 500000 || c >= 20000)`;
- parse(generate(config)) deeply equals the normalized config;
- a token-priced config still emits its prior expression;
- homogeneous AND groups remain AND;
- mixed `&&` and `||` inside one tier return `null` from visual parsing;
- `parseTiersFromExpr` returns `requestPrice: 0.5` and does not mistake nested `request(...)` parentheses for the end of the tier call.

### Step 2: Run the focused test and confirm the red state

```powershell
Set-Location web\default
bun test src/features/pricing/lib/__tests__/tier-expr.test.ts
```

Expected: FAIL because visual tiers have no pricing unit/request price and the current parser only supports `&&` token expressions.

### Step 3: Extend shared parsed-tier types and parsing

In `billing-expr.ts`:

```ts
export type TierConditionLogic = 'and' | 'or'

export type ParsedTier = {
  label: string
  conditions: TierCondition[]
  conditionLogic: TierConditionLogic
  requestPrice?: number
  [field: string]: unknown
}
```

Replace the flat tier-call regular expression with a balanced-parenthesis scan so nested `request(number)` calls are parsed correctly. Recognize exact finite non-negative `request(number)` bodies. Split conditions only when all top-level separators are the same operator; otherwise mark the expression as non-visual.

### Step 4: Extend the visual config and generator

In `tier-expr.ts` add:

```ts
export type PricingUnit = 'token' | 'request'

export type VisualTier = {
  label: string
  conditions: TierConditionInput[]
  input_unit_cost: number
  output_unit_cost: number
  cache_mode: CacheMode
  cache_read_unit_cost?: number
  cache_create_unit_cost?: number
  cache_create_1h_unit_cost?: number
  image_unit_cost?: number
  image_output_unit_cost?: number
  audio_input_unit_cost?: number
  audio_output_unit_cost?: number
  condition_logic?: TierConditionLogic
  request_price?: number
  [field: string]: unknown
}

export type VisualConfig = {
  pricing_unit: PricingUnit
  tiers: VisualTier[]
}
```

Generate per-request bodies as `request(price)`. Join conditions with `&&` for `and` and `||` for `or`. Keep missing `condition_logic` backward-compatible as `and`. Add `request: (price) => price * 1_000_000` to the local preview environment.

### Step 5: Run tests and type checking

```powershell
bun test src/features/pricing/lib/__tests__/tier-expr.test.ts src/features/pricing/lib/__tests__/time-rule-expr.test.ts
bun run typecheck
Set-Location ..\..
```

Expected: PASS.

### Step 6: Commit only Task 5 files

```powershell
git add -- web/default/src/features/pricing/lib/billing-expr.ts web/default/src/features/pricing/lib/tier-expr.ts web/default/src/features/pricing/lib/__tests__/tier-expr.test.ts
git commit -m "feat: round-trip request-priced visual tiers"
```

---

## Task 6: Add Pricing Unit and Condition Logic to the Admin Editor

**Files:**

- Modify: `web/default/src/features/system-settings/models/tiered-pricing-editor.tsx`
- Modify: `web/default/src/features/system-settings/models/model-pricing-sheet.tsx`

### Step 1: Add unit selection without creating a second editor

At the top of the visual editor, add an accessible selector bound to `config.pricing_unit`:

```tsx
<Select value={config.pricing_unit} onValueChange={setPricingUnit}>
  <SelectItem value='token'>{t('Per-token tiers')}</SelectItem>
  <SelectItem value='request'>{t('Per-request tiers')}</SelectItem>
</Select>
```

Changing units should initialize missing fields but must not silently convert monetary values. If the current expression cannot be represented in the target unit, preserve raw mode and show the existing raw-edit fallback.

### Step 2: Render request price fields and correct units

In `VisualTierCard`:

- token mode keeps all existing token/cache/media inputs and `$/1M tokens` labels;
- request mode hides those fields and shows one finite, non-negative `request_price` input;
- the visible suffix is `$/request`;
- the fallback tier works identically to threshold tiers.

### Step 3: Add the all/any condition selector

When a tier has at least two conditions, show:

```tsx
<Select value={tier.condition_logic ?? 'and'} onValueChange={setConditionLogic}>
  <SelectItem value='and'>{t('All conditions (AND)')}</SelectItem>
  <SelectItem value='or'>{t('Any condition (OR)')}</SelectItem>
</Select>
```

Keep the selector hidden for zero/one condition because it has no semantic effect. The screenshot's input-length/output-token thresholds use `or`.

### Step 4: Clarify the time control

Rename the visual label from an overnight-only concept to `Time range`, and add concise guidance that start before end means the same day while start after end crosses midnight. Do not expose backend implementation details.

### Step 5: Verify compile and focused parser behavior

```powershell
Set-Location web\default
bun test src/features/pricing/lib/__tests__/tier-expr.test.ts src/features/pricing/lib/__tests__/time-rule-expr.test.ts
bun run typecheck
bun x oxfmt --check src/features/system-settings/models/tiered-pricing-editor.tsx src/features/system-settings/models/model-pricing-sheet.tsx
Set-Location ..\..
```

Expected: PASS.

### Step 6: Commit only Task 6 files

```powershell
git add -- web/default/src/features/system-settings/models/tiered-pricing-editor.tsx web/default/src/features/system-settings/models/model-pricing-sheet.tsx
git commit -m "feat: edit request-priced tiers visually"
```

---

## Task 7: Display Per-Request Prices and Matched Rules

**Files:**

- Create: `web/default/src/features/pricing/lib/__tests__/dynamic-price.test.ts`
- Modify: `web/default/src/features/pricing/lib/dynamic-price.ts`
- Modify: `web/default/src/features/pricing/components/dynamic-pricing-breakdown.tsx`
- Modify: `web/default/src/features/pricing/components/model-card.tsx`
- Modify: `web/default/src/features/pricing/components/model-details.tsx`
- Modify: `web/default/src/features/usage-logs/types.ts`
- Modify: `web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx`
- Modify: `web/default/src/features/usage-logs/lib/format.ts`

### Step 1: Write failing public-price tests

In `dynamic-price.test.ts`, parse the reference request-tier expression and assert five entries with exact request prices and `unit: 'request'`. Assert group ratio/recharge conversion scales the monetary amount once and never divides by one million tokens. Keep a token-expression regression assertion.

Use this target shape:

```ts
export type DynamicPriceEntry = {
  key: string
  field: string
  label: string
  shortLabel: string
  value: number
  formatted: string
  variable: BillingVar | null
  unit: 'token' | 'request'
}
```

### Step 2: Run the focused test and confirm the red state

```powershell
Set-Location web\default
bun test src/features/pricing/lib/__tests__/dynamic-price.test.ts
```

Expected: FAIL because request-tier bodies are not emitted as request-price entries.

### Step 3: Add unit-aware public pricing data

In `dynamic-price.ts`:

- emit one request entry when `ParsedTier.requestPrice` is present;
- use `variable: null` and `unit: 'request'`;
- apply group ratio and recharge rate exactly once;
- keep all existing token field entries and formatting unchanged;
- include `requestPrice` in the fields considered by dynamic summaries.

Update model cards/details only where they currently hard-code `/1M tokens`; use the entry's unit to render `$/request`.

### Step 4: Prefer backend trace data in usage logs

In `usage-logs/types.ts` add:

```ts
export type RequestRuleTrace = {
  cond: string
  multiplier: number
  matched: boolean
}

request_rules?: RequestRuleTrace[]
```

Pass this value from `details-dialog.tsx` into `DynamicPricingBreakdown`. Add a prop:

```ts
requestRules?: RequestRuleTrace[]
```

When present, render the structured rows in their backend order and visually distinguish matched from unmatched rules. When absent, retain expression-only parsing for old logs. Never re-evaluate time in the browser to decide whether an old request matched.

### Step 5: Run focused tests, types, and changed-file lint

```powershell
bun test src/features/pricing/lib/__tests__/dynamic-price.test.ts src/features/pricing/lib/__tests__/tier-expr.test.ts
bun run typecheck
bun x oxlint src/features/pricing/lib/dynamic-price.ts src/features/pricing/components/dynamic-pricing-breakdown.tsx src/features/pricing/components/model-card.tsx src/features/pricing/components/model-details.tsx src/features/usage-logs/types.ts src/features/usage-logs/components/dialogs/details-dialog.tsx src/features/usage-logs/lib/format.ts
Set-Location ..\..
```

Expected: PASS.

### Step 6: Commit only changed Task 7 files

Use `git diff --name-only` to omit optional files that did not need edits, then stage only Task 7 implementation/test paths:

```powershell
git add -- web/default/src/features/pricing/lib/dynamic-price.ts web/default/src/features/pricing/lib/__tests__/dynamic-price.test.ts web/default/src/features/pricing/components/dynamic-pricing-breakdown.tsx web/default/src/features/pricing/components/model-card.tsx web/default/src/features/pricing/components/model-details.tsx web/default/src/features/usage-logs/types.ts web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx web/default/src/features/usage-logs/lib/format.ts
git commit -m "feat: show request tiers and matched billing rules"
```

---

## Task 8: Add Seven-Language UI Copy Through the Project Script

**Files:**

- Temporarily create and then delete: `web/default/scripts/add-tiered-request-i18n.mjs`
- Modify through the script only: `web/default/src/i18n/locales/en.json`
- Modify through the script only: `web/default/src/i18n/locales/zh.json`
- Modify through the script only: `web/default/src/i18n/locales/zh-TW.json`
- Modify through the script only: `web/default/src/i18n/locales/fr.json`
- Modify through the script only: `web/default/src/i18n/locales/ja.json`
- Modify through the script only: `web/default/src/i18n/locales/ru.json`
- Modify through the script only: `web/default/src/i18n/locales/vi.json`
- Modify through sync: `web/default/src/i18n/locales/_reports/_sync-report.json`

### Step 1: Collect exact new English keys

Run `rg "t\('"` over the changed UI files and add this exact key set when it is not already present:

- `Pricing unit`
- `Per-token tiers`
- `Per-request tiers`
- `Request price`
- `$/request`
- `All conditions (AND)`
- `Any condition (OR)`
- `Time range`
- `Start before end stays within the day; start after end crosses midnight`
- `Matched condition`
- `Condition not matched`

### Step 2: Add translations only through the required script

Create `web/default/scripts/add-tiered-request-i18n.mjs` that imports and calls the repository's `add-missing-keys.mjs` workflow with explicit translations for all seven locales. Do not edit locale JSON by hand.

Run:

```powershell
Set-Location web\default
bun scripts/add-tiered-request-i18n.mjs
bun run i18n:sync
Set-Location ..\..
```

Then delete the temporary script with `apply_patch`.

### Step 3: Verify no fallback or missing keys

Inspect `_sync-report.json` and run:

```powershell
Set-Location web\default
bun run i18n:sync
bun run typecheck
Set-Location ..\..
```

Expected: sync exits successfully and all new keys have translations in `en`, `zh`, `zh-TW`, `fr`, `ja`, `ru`, and `vi`.

### Step 4: Protect pre-existing locale edits

Compare each locale file against the pre-Task-8 worktree. The files already contain unrelated prefill-compatibility translations. Do not stage or commit those unrelated changes. If clean hunk separation is not safe for JSON, leave all locale outputs unstaged and report them in the final handoff; the implementation remains in the working tree.

If and only if the locale diffs contain no unrelated work, commit:

```powershell
git add -- web/default/src/i18n/locales/en.json web/default/src/i18n/locales/zh.json web/default/src/i18n/locales/zh-TW.json web/default/src/i18n/locales/fr.json web/default/src/i18n/locales/ja.json web/default/src/i18n/locales/ru.json web/default/src/i18n/locales/vi.json web/default/src/i18n/locales/_reports/_sync-report.json
git commit -m "i18n: translate request-tier billing controls"
```

---

## Task 9: End-to-End Billing Regression and Final Verification

**Files:**

- Create: `service/tiered_billing_flow_test.go`
- Modify: `pkg/billingexpr/request_price_test.go`
- Modify: `web/default/src/features/pricing/lib/__tests__/tier-expr.test.ts`

### Step 1: Add the end-to-end backend scenario

Create `service/tiered_billing_flow_test.go`. The test must use the reference expression and prove this full path:

1. a request with input below 200K and `max_tokens` below 8K pre-consumes the `$0.15` fallback at the first group ratio;
2. retry selection changes to a higher group ratio and increases the reservation before sending;
3. actual output crosses the 8K threshold and settlement selects `$0.20`;
4. the final quota is non-negative, checked through the existing conversion helper, and uses the final group ratio;
5. the consume-log payload records the matched tier and structured matched multiplier trace.

Keep the fixture deterministic with a fixed `EvaluatedAt`.

### Step 2: Run all focused tests

```powershell
go test ./pkg/billingexpr ./relay/helper ./service ./controller -count=1
Set-Location web\default
bun test src/features/pricing/lib/__tests__
Set-Location ..\..
```

Expected: PASS.

### Step 3: Run the full repository verification

```powershell
go test ./...
Set-Location web\default
bun run i18n:sync
bun run typecheck
bun run lint
bun run build
Set-Location ..\..
```

Expected: every command exits 0. If an unrelated pre-existing failure appears, record the exact command/output and still run every remaining independent check.

### Step 4: Review the final diff and invariants

Run:

```powershell
git diff --check
git status --short
git diff --stat
rg -n "TODO|TBD|FIXME|<replace-me>" pkg/billingexpr relay/helper service controller web/default/src/features/pricing web/default/src/features/system-settings/models web/default/src/features/usage-logs docs/superpowers
```

Manually confirm:

- `request()` rejects negative/non-finite input;
- no bare quota conversion or direct `OtherRatios` writes were introduced;
- old token expressions return unchanged results;
- same-day time windows use `&&`, overnight windows use `||`, equal bounds never match;
- request time is frozen through pre-consume, retry, and settlement;
- higher-cost retry groups reserve before sending and final settlement uses the last selected group;
- request rules appear in logs and old logs still render;
- public/admin UI consistently uses `$/request`;
- unknown/mixed expressions remain raw rather than being rewritten;
- all unrelated prefill-compatibility work remains intact and unstaged unless it was already the user's staged work.

### Step 5: Commit only final test/doc corrections

Stage the exact Task 9 test files and commit:

```powershell
git add -- service/tiered_billing_flow_test.go pkg/billingexpr/request_price_test.go web/default/src/features/pricing/lib/__tests__/tier-expr.test.ts
git commit -m "test: cover tiered request billing flow"
```

### Step 6: Report completion with evidence

Provide the user:

- the implemented behavior in plain language;
- the selected upstream fixes adapted, with commit links;
- focused and full verification commands with pass/fail status;
- any pre-existing unrelated failures clearly separated;
- the list of intentionally untouched/uncommitted prefill-compatibility files.
