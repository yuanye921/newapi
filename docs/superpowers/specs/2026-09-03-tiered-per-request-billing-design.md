# Tiered Per-Request Billing Design

## Goal

Add tiered per-request billing to the existing billing-expression system and
make time-based multipliers safe and observable. A successful text request is
charged exactly once. The charged USD amount is selected from usage tiers,
where a tier may match either input context length or output token count, and
the highest matching tier wins. Optional time windows can then multiply that
selected request price.

The reference rule is:

| Tier | Match condition | Price |
| --- | --- | ---: |
| Tier 5 | `len >= 500000 || c >= 20000` | $0.50/request |
| Tier 4 | `len >= 400000 || c >= 16000` | $0.40/request |
| Tier 3 | `len >= 300000 || c >= 12000` | $0.30/request |
| Tier 2 | `len >= 200000 || c >= 8000` | $0.20/request |
| Base | No higher tier matches | $0.15/request |

## Selected Approach

Extend the existing expression language with `request(price)`, where `price`
is a non-negative USD amount for one request. Keep `tier(name, value)` as the
single source of truth for tier selection, pre-consume, settlement, logs, and
public pricing display.

The reference rule is stored as:

```text
len >= 500000 || c >= 20000 ? tier("tier_5", request(0.5)) :
len >= 400000 || c >= 16000 ? tier("tier_4", request(0.4)) :
len >= 300000 || c >= 12000 ? tier("tier_3", request(0.3)) :
len >= 200000 || c >= 8000  ? tier("tier_2", request(0.2)) :
tier("base", request(0.15))
```

Internally, `request(price)` converts the request price into the expression's
existing v1 cost scale. The centralized quota conversion remains unchanged,
so group ratios, quota rounding, saturation auditing, frozen snapshots, and
settlement error handling continue to use the established path.

## Alternatives Rejected

### A separate `tiered_per_request` billing mode

This would make the stored configuration explicit, but it would duplicate the
pre-consume, settlement, logging, synchronization, and frontend display paths.
The two tiered modes could drift and calculate different amounts.

### Manually multiplying every request price by one million

This requires fewer backend changes, but exposes implementation units to
administrators, produces confusing expressions, and makes pricing mistakes
more likely. It also prevents the visual editor and public price page from
reliably identifying per-request amounts.

## Billing Semantics

- One completed text request produces one tiered request charge.
- `len` is the full input context length already computed by the tiered billing
  pipeline.
- `c` is the actual output token count during final settlement.
- Tier conditions are evaluated from highest to lowest. The first matching tier
  is charged, which makes the highest matching threshold win.
- Conditions within the reference tiers use logical OR: reaching either the
  context threshold or output threshold is sufficient.
- If no threshold matches, the fallback tier is charged.
- Group ratios and request-rule multipliers apply to the selected request price
  exactly as they apply to existing tiered expressions.
- A request price of zero is allowed for deliberately free tiers. Negative,
  NaN, and infinite request prices are rejected.
- Existing token-price arithmetic and fixed `ModelPrice` per-request billing
  retain their current meaning. Time rules use the corrected range and frozen
  request-time semantics described below.

## Pre-Consume and Settlement Flow

Pre-consume evaluates the same frozen expression that final settlement uses:

1. Use the counted input tokens for `len` and `p`.
2. Use the request's declared maximum output tokens for `c`.
3. If the request omits that limit, retain the existing 8192-token fallback.
4. Evaluate the expression, apply the group ratio, and pre-consume the estimated
   quota.
5. Freeze the expression, request-dependent inputs, group ratio, estimated
   tier, and quota in the existing billing snapshot.

After the upstream response provides usage, settlement evaluates the frozen
expression with actual input and output counts. It then refunds or consumes the
difference from the pre-consumed amount. A configuration change made while a
request is running cannot change that request's bill.

If expression evaluation fails during settlement, the existing safe fallback
charges the frozen pre-consumed amount. Quota conversion continues to use the
project's checked rounding and saturation-audit path.

## Expression Runtime and Validation

The v1 compile and runtime environments expose:

```text
request(price) -> float64
```

The function accepts a finite, non-negative USD amount and returns a value in
the existing expression cost scale. It can be wrapped by `tier()` and multiplied
by existing request rules. Existing expressions compile and run unchanged.

## Time-Based Multipliers

Time rules remain request-rule multipliers appended to the base tier
expression. They apply equally to token-priced tiers and the new per-request
tiers. For example, doubling the selected request price from 09:00 until 12:00
in Shanghai uses:

```text
(tier("base", request(0.15))) *
(hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12 ? 2 : 1)
```

Time ranges have explicit boundary behavior:

- `start < end` is a within-day window and uses `current >= start && current < end`.
- `start > end` crosses midnight and uses `current >= start || current < end`.
- `start == end` matches no time, preventing an accidental all-day multiplier.
- Hour, minute, weekday, month, and day values must be integers inside their
  natural domains: hour 0-23, minute 0-59, weekday 0-6, month 1-12, and day
  1-31.
- Multiple rule groups retain the existing multiplicative behavior. If two
  windows overlap and both match, both multipliers apply.

The pricing time is captured when the request enters billing and reused during
settlement. `billingexpr.RequestInput` carries this evaluation timestamp, and
the runtime time functions read it instead of calling the clock again. Direct
callers that omit the timestamp retain the current-clock fallback. A response
crossing a time boundary therefore cannot change price between pre-consume and
final settlement.

The visual editor labels this control as a general time range rather than an
overnight-only range, explains the within-day versus across-midnight behavior,
and preserves valid ranges through visual-to-expression round trips. Invalid
raw ranges remain in raw mode instead of being silently rewritten.

Configuration smoke tests include per-request values and reject invalid or
non-finite outcomes before saving. Mixed raw expressions remain possible for
advanced administrators, but the visual editor only round-trips expressions it
can represent without losing information.

## Admin Interface

The existing Expression pricing editor gains a pricing-unit choice:

- Per-token: the current input, output, cache, image, and audio unit prices.
- Per-request: one USD request price for each tier.

In per-request mode, token-price fields are hidden and each tier card displays a
single `$/request` input. Because the current visual editor only joins multiple
conditions with `&&`, each non-fallback tier gains an explicit condition
operator selector: "all conditions" generates `&&`, while "any condition"
generates `||`. Fallback tier behavior, tier ordering, raw-expression editing,
and request-rule multipliers remain available.

The visual expression generator wraps each tier price in `request(...)`. The
parser recognizes tiers whose bodies are `request(number)` and restores them as
per-request visual configuration. Homogeneous `&&` and `||` condition groups
round-trip without changing their meaning. Expressions that mix request and
token costs, or mix `&&` and `||` inside one tier without a lossless visual
representation, stay in raw-expression mode.

All new labels, descriptions, validation messages, and price units use the
project i18n workflow for English, Simplified Chinese, Traditional Chinese,
French, Japanese, Russian, and Vietnamese.

## Public Pricing and Logs

The public model price page recognizes request-priced tier expressions and
renders:

- the fallback price as `$amount / request`;
- every threshold tier as `$amount / request`;
- the existing dynamic-pricing notice;
- readable input-length and output-token conditions.

The consume log continues to record `billing_mode`, the encoded frozen
expression, and `matched_tier`. It also records the detected conditional
multiplier rules and whether each one matched. The log-details view highlights
the time or request conditions that actually changed the charge. The existing
log-details parser uses `request(...)` to show the actual tier as a per-request
charge. No database migration or new log column is required.

Because pricing and settlement are shared above provider adapters, ordinary
text requests from OpenAI Chat, Claude Messages, Gemini, and OpenAI Responses
inherit the feature without channel-specific billing branches. Non-text task,
image, audio, embedding, rerank, and realtime billing flows are outside this
change.

## Error Handling and Compatibility

- Invalid request prices prevent the billing expression from being saved.
- Invalid visual drafts show an administrator-facing validation error and do
  not overwrite the last valid expression.
- Settlement failures use the frozen pre-consume fallback already established
  by tiered billing.
- Old expressions without `request()` continue to compile. Their token-price
  arithmetic is unchanged; only malformed visual time ranges and requests that
  cross a time boundary receive the intentional corrections described above.
- Pricing synchronization continues to use `ModelBillingMode` and
  `ModelBillingExpr`; no new database setting or migration is introduced.
- Unknown expression syntax is preserved by the raw editor instead of being
  rewritten by the visual editor.

## Selected Upstream Billing Fixes

Only the following upstream billing changes are adapted to this customized
branch. Their behavior is retained while paths and types are adjusted to the
current local architecture:

1. [`ac381ac`](https://github.com/QuantumNous/new-api/commit/ac381acf4bf41204b97bb26b4c58c83275877a2e)
   fixes within-day time ranges that previously generated an
   always-true OR expression. It also adds time-domain validation, parser
   round-trip handling, UI guidance, translations, and regression tests.
2. [`df43f80`](https://github.com/QuantumNous/new-api/commit/df43f80)
   refreshes group-dependent tiered billing state before every
   upstream retry. A retry that moves to a more expensive group raises the
   reservation before sending; a move from a free group to a paid group starts
   billing before that attempt. Final settlement uses the last selected group.
3. [`4cf9107`](https://github.com/QuantumNous/new-api/commit/4cf9107)
   records conditional multiplier traces and highlights matched rules in usage
   logs.

The repository is not wholesale-upgraded to the latest upstream release as
part of this feature. Experimental task plugins, broad quota-type migrations,
database migrations, and unrelated provider changes remain outside scope.

## Test Strategy

Backend tests are written first and cover:

- `request(price)` compilation, execution, zero price, and invalid prices;
- exact USD-to-quota conversion without token multiplication;
- base tier, every threshold boundary, OR semantics, and highest-tier priority;
- pre-consume estimates and actual settlement differences;
- group-ratio scaling, frozen snapshots, and saturation safety;
- a time captured at request start remaining stable across settlement;
- same-day, overnight, equal-bound, invalid-bound, and multi-window time rules;
- tiered retries reserving and settling against the final selected group;
- matched and unmatched conditional multiplier traces in consume logs;
- unchanged results for existing token-priced expressions.

Frontend tests are written first and cover:

- parsing and generating per-request visual tiers;
- preserving OR conditions and fallback tiers through a round trip;
- refusing lossy visual conversion for mixed expressions;
- same-day and overnight time-range generation, parsing, validation, and
  round-trip stability;
- matched time-rule presentation in usage-log details;
- public pricing breakdowns and `$ / request` formatting.

Verification runs focused Go and frontend tests first, followed by `go test
./...`, frontend type checking, lint for changed files, i18n synchronization,
and the production build.

## Out of Scope

- Custom currencies or a configurable request-price unit.
- Progressive or cumulative charging across several tiers in one request.
- Per-channel copies of the same pricing rule.
- Changes to non-text billing flows.
- A database migration or a second tiered billing configuration store.
- A wholesale upstream release merge or unrelated upstream fixes.
