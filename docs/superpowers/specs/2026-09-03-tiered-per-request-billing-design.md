# Tiered Per-Request Billing Design

## Goal

Add tiered per-request billing to the existing billing-expression system. A
successful text request is charged exactly once. The charged USD amount is
selected from usage tiers, where a tier may match either input context length
or output token count, and the highest matching tier wins.

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
- Existing token-priced expressions and fixed `ModelPrice` per-request billing
  retain their current meaning and results.

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

Configuration smoke tests include per-request values and reject invalid or
non-finite outcomes before saving. Mixed raw expressions remain possible for
advanced administrators, but the visual editor only round-trips expressions it
can represent without losing information.

## Admin Interface

The existing Expression pricing editor gains a pricing-unit choice:

- Per-token: the current input, output, cache, image, and audio unit prices.
- Per-request: one USD request price for each tier.

In per-request mode, token-price fields are hidden and each tier card displays a
single `$/request` input. Existing tier conditions, AND/OR selection, fallback
tier behavior, tier ordering, raw-expression editing, and request-rule
multipliers remain available.

The visual expression generator wraps each tier price in `request(...)`. The
parser recognizes tiers whose bodies are `request(number)` and restores them as
per-request visual configuration. Expressions that mix request and token costs
without a lossless visual representation stay in raw-expression mode.

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
expression, and `matched_tier`. The existing log-details parser uses
`request(...)` to show the actual tier as a per-request charge. No database
migration or new log column is required.

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
- Old expressions without `request()` remain byte-for-byte compatible in
  meaning and output.
- Pricing synchronization continues to use `ModelBillingMode` and
  `ModelBillingExpr`; no new database setting or migration is introduced.
- Unknown expression syntax is preserved by the raw editor instead of being
  rewritten by the visual editor.

## Test Strategy

Backend tests are written first and cover:

- `request(price)` compilation, execution, zero price, and invalid prices;
- exact USD-to-quota conversion without token multiplication;
- base tier, every threshold boundary, OR semantics, and highest-tier priority;
- pre-consume estimates and actual settlement differences;
- group-ratio scaling, frozen snapshots, and saturation safety;
- unchanged results for existing token-priced expressions.

Frontend tests are written first and cover:

- parsing and generating per-request visual tiers;
- preserving OR conditions and fallback tiers through a round trip;
- refusing lossy visual conversion for mixed expressions;
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
