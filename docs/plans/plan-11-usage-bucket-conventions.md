---
Title: Define llm.Usage input buckets as disjoint and fix the subset-convention decoders
Author: Claude, with Curtis Myzie
Status: Implemented
Last Updated: 2026-08-10
---

# Define llm.Usage input buckets as disjoint and fix the subset-convention decoders

**Workflow:** Standard-tier spec, Dive team feedback, then implementation.

## Context

`llm.Usage` carries four token buckets and a priced `Cost`. Nothing on the
type states how `InputTokens` relates to `CacheReadInputTokens`
(`llm/usage.go:6-15` — the only documented relationship is that
`ReasoningTokens` is a subset of `OutputTokens`). In practice Dive's
providers populate the input-side buckets under two incompatible
conventions, and the pricing formula assumes only one of them.

`PricingInfo.CostOf` (`llm/pricing.go`) prices every bucket independently
and sums them:

```go
c := Cost{
    Input:      float64(u.InputTokens) * p.InputPrice / perMillion,
    Output:     float64(u.OutputTokens) * p.OutputPrice / perMillion,
    CacheRead:  float64(u.CacheReadInputTokens) * p.CacheReadPrice / perMillion,
    CacheWrite: float64(u.CacheCreationInputTokens) * p.CacheWritePrice / perMillion,
}
c.Total = c.Input + c.Output + c.CacheRead + c.CacheWrite
```

That formula is correct exactly when the buckets are **disjoint** — every
token counted once, in one bucket. Anthropic's direct decoder uses that
convention, and Ollama inherits it through its embedded Anthropic provider.

This was found during a downstream consumer's capability-gap review of
Dive: that deployment enforces spend budgets on `Cost.Total` and persists
it as its durable spend record, so the overstatement below reaches budget
refusals and money reporting, not just logs.

## Current behavior and root cause

Per-provider population of the input-side buckets:

| Provider                | Convention                                                                                                                                                                                                                                                                                                                                                                               | Mapping sites                                                              |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `anthropic`             | **Disjoint** — the API reports `input_tokens` excluding cached; read/creation are separate additive fields                                                                                                                                                                                                                                                                               | decoder passes them through unchanged                                      |
| `openai` (Responses)    | **Subset** — `InputTokens` is the full prompt; `CachedTokens` and GPT-5.6 `CacheWriteTokens` are parts of it                                                                                                                                                                                                                                                                             | `providers/openai/decode.go`, `providers/openai/stream_iterator.go`        |
| `openaicompletions`     | **Subset** — `CachedTokens` and `CacheWriteTokens` are parts of the wire `PromptTokens`, not additive                                                                                                                                                                                                                                                                                    | `providers/openaicompletions/types.go`                                     |
| `grok`                  | **Subset** — embeds the OpenAI Responses provider (`providers/grok/grok.go:20-24`); verified `grok.go` only embeds and configures it, with no usage mapping of its own                                                                                                                                                                                                                   | inherited                                                                  |
| `google`                | **Subset** — `CachedContentTokenCount` is part of `PromptTokenCount` per the Gemini API contract                                                                                                                                                                                                                                                                                         | `providers/google/util.go:89-97`                                           |
| `openrouter`, `mistral` | **Subset today, by inheritance** — both embed `*openaic.Provider` (`providers/openrouter/openrouter.go:35`, `providers/mistral/mistral.go:34`) with no usage code of their own, so their mapping _is_ `toLLMUsage`. Whenever the served endpoint reports `prompt_tokens_details.cached_tokens` — OpenRouter passes it through for many upstreams — they are on the broken arithmetic now | shared `toLLMUsage`; fixed automatically by the `openaicompletions` change |
| `ollama`                | **Disjoint by construction** — embeds the Anthropic provider (`providers/ollama/ollama.go:34`) and speaks its Anthropic-compatible Messages convention                                                                                                                                                                                                                                   | none needed                                                                |

On every subset-convention path, `CostOf` charges a cached token at
`InputPrice + CacheReadPrice` instead of `CacheReadPrice`. When
`CacheReadPrice` is nonzero, the ratio of the incorrect cached component to
the correct one is `1 + InputPrice/CacheReadPrice`: exactly **11x** on the
current GPT-5.6 tier (`providers/openai/pricing_gen.go`: input 5.00, cache
read 0.50 per million). The excess charge itself is `InputPrice`, or **10x**
the correct cached component at those rates. When `CacheReadPrice` is zero,
that ratio is undefined; the missing-rate problem is addressed separately
below. The better a consumer's prompt caching works, the more its reported
cost _rises_.

The root cause is that the type never chose a convention, so each decoder
faithfully preserved its provider's wire shape and the shared pricing code
silently assumed Anthropic's.

### The pricing tables have their own gap

`CacheReadPrice` is populated only for the GPT-5.6 family and Grok's
catalog. Google has no cache-read price on any model, and neither do
GPT-5.5/5.4 and earlier — models that do report cached tokens. This is
invisible today because the subset double-count overwhelms it, but it
determines the _direction_ of error after the decoder fix: see proposal
step 3.

There is also a fourth mapping surface easy to miss: `llm.Usage`'s own
`UnmarshalJSON` (`llm/usage.go:26-50`) advertises accepting provider-native
usage shapes and maps `output_tokens_details` onto `ReasoningTokens`, but
ignores input-side details. A provider-native payload with
`input_tokens_details`/`prompt_tokens_details` parsed through it would
reintroduce the subset convention behind the decoders' backs.

## Goals

- `llm.Usage`'s input-side buckets (`InputTokens`,
  `CacheCreationInputTokens`, `CacheReadInputTokens`) are **disjoint by
  documented contract**, for every provider, on both the non-streaming and
  streaming paths.
- `CostOf` is correct as written, for every provider, with no per-provider
  arithmetic anywhere outside decoders.
- Consumers that need the full prompt size can derive it without
  re-learning per-provider conventions.

## Non-goals

- No change to Anthropic's decoder or cache-write pricing. OpenAI GPT-5.6's
  separately reported `cache_write_tokens` now populates the existing
  `CacheCreationInputTokens` bucket.
- No change to `ReasoningTokens`, which stays a documented subset of
  `OutputTokens` and is deliberately not priced as its own bucket —
  output-side subset with documentation is a coherent, existing design.
- No renaming of any field; no wire-format changes toward providers.
- No attempt to reconstruct historical usage for downstream consumers
  (the reporting consumer has decided its own cutover is forward-only).
  Provider adapters decode typed response structs rather than raw JSON into
  `llm.Usage`, while session storage emits Dive's canonical usage shape without
  provider-native input-detail objects. Historical raw provider payloads
  therefore do not pass through `Usage.UnmarshalJSON` during this migration.

## Proposal

1. **State the contract on the type.** Document on `llm.Usage`
   (`llm/usage.go`) that `InputTokens`, `CacheCreationInputTokens`, and
   `CacheReadInputTokens` are mutually disjoint: `InputTokens` counts
   uncached, unwritten prompt tokens only, and the full prompt is the sum
   of the three. Note the contrast with `ReasoningTokens`' documented
   subset-of-output convention so the asymmetry is explicit rather than
   surprising.
2. **Fix the subset decoders at their mapping sites.** Every subset decoder
   uses the same normalization before constructing `llm.Usage`:

   ```go
   prompt := max(0, wirePrompt)
   cached := min(max(0, wireCached), prompt)
   written := min(max(0, wireWritten), prompt - cached)
   InputTokens = prompt - cached - written
   CacheReadInputTokens = cached
   CacheCreationInputTokens = written
   ```

   For a valid provider payload (`0 <= wireCached <= wirePrompt`),
   `TotalInputTokens()` therefore equals the raw wire prompt count. For an
   invalid payload, tests assert the exact clamped buckets against the
   normalized prompt instead of treating the invalid raw fields as an
   invariant oracle. Apply that rule at every subset mapping site:

   - `providers/openai/decode.go:29` and
     `providers/openai/stream_iterator.go:452,475`:
     normalize `InputTokens` with `InputTokensDetails.CachedTokens` and
     `CacheWriteTokens`.
   - `providers/openaicompletions/types.go` `toLLMUsage`: same subtraction
     from `PromptTokens`, including `cache_write_tokens`; update the
     `PromptTokensDetails` comment to say
     the subset relationship is a _wire_ fact that the conversion resolves.
     This one site also fixes `openrouter` and `mistral`, which embed the
     provider and have no usage code of their own.
   - `providers/google/util.go` `convertUsageMetadata`: subtract
     `CachedContentTokenCount` from `PromptTokenCount`.
   - `grok` inherits the OpenAI Responses fix — verified: `grok.go` only
     embeds and configures the Responses provider.
   - `llm/usage.go` `UnmarshalJSON`: apply the same map-and-clamp for
     provider-native input-side read and write details, mirroring the existing
     `output_tokens_details` handling. Precedence is based on JSON key
     presence, not zero values: if either canonical
     `cache_read_input_tokens` or `cache_creation_input_tokens` is present,
     the canonical disjoint fields win and native detail objects are ignored;
     otherwise `input_tokens_details.cached_tokens` paired with
     `input_tokens` wins over the Chat Completions fallback of
     `prompt_tokens_details.cached_tokens` paired with `prompt_tokens`.
     Whenever that native normalization changes token fields, clear `Cost`
     because `UnmarshalJSON` has no pricing snapshot from which to recompute
     it. When no normalization occurs, preserve the alias-unmarshaled `Cost`
     exactly — including an existing `Cost.Total` that does not equal its
     components — so canonical historical data is not silently rewritten.

3. **Backfill cache-read prices where cached tokens are actually
   reported.** The decoder fix changes the effective formula to
   `uncached × InputPrice + cached × CacheReadPrice`, and a zero
   `CacheReadPrice` then prices cache hits at $0. Google (no cache-read
   prices at all) and OpenAI models before the 5.6 tier would flip from
   overstating cost to _understating_ it — the worse direction for the
   budget-enforcement use case that motivated this plan, since budgets trip
   late instead of early. Treat `providers/google/catalog.json` and
   `providers/openai/catalog.json` as the authoritative inputs. In the
   implementation diff, enumerate every `pricing.text` model for which the
   provider's official pricing page publishes a cache-read/context-caching
   rate, and record `cache_read_price_per_1m_tokens`, `updated_at`, and the
   exact official pricing URL in the catalog's `sources`. Validate the rest of
   each row against that same source rather than grafting a cache rate onto a
   stale input/output row. Models with no published cache discount stay zero
   and are listed explicitly in the pricing test's exclusions with the source
   reason.

   The OpenAI catalog generates both `providers/openai/pricing_gen.go` and the
   filtered `providers/openaicompletions/pricing_gen.go`; the Google catalog
   generates `providers/google/pricing_gen.go`. Do not edit generated pricing
   files directly. Run `make provider-catalog-generate`, then
   `make provider-catalog-check`, and make the pricing regression table name
   every cache-capable model so a missing row cannot hide behind iteration over
   only the entries that already have nonzero prices. Backfill Google and
   older OpenAI rates in the same change so the correction never ships through
   an understating intermediate state.

4. **Add the derived total.** A method on `Usage`, `TotalInputTokens() int`,
   returning `InputTokens + CacheCreationInputTokens + CacheReadInputTokens`,
   so consumers that want the provider's full prompt count (context-size
   tracking, display) have one blessed answer. The CLI already hand-rolls
   exactly this sum (`experimental/cmd/dive/app.go:2550`) — double-counting
   cached tokens on subset providers today — and becomes the helper's first
   caller. Audit Dive's other internal consumers of `InputTokens` (CLI
   display in `render.go`, accumulators; Anthropic's `logCacheThrash` is
   unaffected since that path doesn't change) and move any that meant "full
   prompt" onto it.
5. **Say it loudly in the changelog.** For consumers on the
   subset-convention paths — OpenAI Responses, `openaicompletions` and its
   embedders OpenRouter and Mistral, Grok, Google — this changes reported
   values: `InputTokens` drops by the cached amount and `Cost.Total` drops
   accordingly. That is the bug fix, but anyone comparing across the
   upgrade will see the discontinuity; the release notes must state it as a
   semantic correction with the one-line formula for reconstructing the old
   value (`old input = new input + cache read`).

## Downstream impact

- **The reporting consumer** enforces budgets and persists spend from
  `Cost.Total` unreconciled; its own tracking work pins the Dive release
  carrying this fix, declares each provider's convention at registration,
  and validates the disjoint invariant per served provider — Dive
  computes, the consumer verifies. It needs a tagged release to pin.
- Any other consumer summing buckets or comparing usage across providers
  gets correct, comparable numbers for the first time. Consumers that were
  independently compensating for the subset convention (none known) would
  double-correct — one more reason the changelog entry must be explicit.
- The pricing backfill (step 3) is part of the correction, not a side
  quest: with it, subset-path estimates drop to the true discounted price;
  without it they would drop _below_ it, and a budget enforced on
  `Cost.Total` would refuse too late instead of too early.

## Alternatives considered

- **Standardize on the subset convention instead.** Rejected: it makes the
  four-field struct ambiguous (a cached token lives in two buckets), forces
  the subtraction into `CostOf` and every other consumer forever, and
  contradicts the additive semantics `Usage.Add` and `Cost.Add` already
  assume.
- **Keep per-provider conventions and price per provider.** Rejected: it
  spreads wire knowledge from decoders into shared pricing code and leaves
  every downstream consumer with the comparability problem this plan
  exists to remove.
- **Fix only pricing and leave the token counts as reported.** Rejected:
  cost would be right while cross-provider token sums stayed meaningless,
  and every consumer doing its own arithmetic on tokens (budgeting per
  token, analytics) would still be wrong.

## Tests

**Thorough post-implementation testing is a hard requirement of this plan,
not a nicety.** This change alters money-bearing numbers on every
non-Anthropic path. The implementation is not done until every fixture
below is in place and green, and the live per-provider verification in the
Rollout section has actually been run — decoded buckets and `Cost.Total`
reconciled against each provider's own reported usage on real calls with
observed cache hits.

- Per-decoder unit tests from wire fixtures with cached tokens present:
  assert disjoint output for valid payloads and the exact upper/lower clamp for
  invalid cached counts, on all three decode paths for OpenAI
  Responses (non-streaming, `ResponseCompletedEvent`, and
  `ResponseIncompleteEvent`).
- A cross-provider invariant test over valid usage fixtures, with the expected
  prompt total computed from provider-specific raw metadata before decoding:
  OpenAI Responses and Grok use `usage.input_tokens`; OpenAI-compatible,
  OpenRouter, and Mistral use `usage.prompt_tokens`; Google uses
  `usageMetadata.promptTokenCount`; Anthropic and Ollama sum the wire
  `input_tokens`, `cache_creation_input_tokens`, and
  `cache_read_input_tokens`. Compare that independent oracle with
  `TotalInputTokens()`, and assert that `CostOf` equals the disjoint formula.
  Invalid fixtures use the normalized-prompt expectation defined in step 2
  instead of the raw-field invariant.
- A pricing regression test with a subset-shaped fixture proving the old
  double-count can no longer be produced.
- A streamed end-to-end case: a subset-shaped fixture through the stream
  iterator and `ResponseAccumulator`, asserting the cost attached by
  `PopulateCost` at stream end (`llm/stream.go:224`) is the disjoint cost.
- Separate inheritance canaries at both the OpenRouter and Mistral provider
  surfaces pin that each produces disjoint usage via the shared
  `toLLMUsage`; the inheritance is load-bearing and otherwise invisible.
- `Usage.UnmarshalJSON` tests cover Responses-native and
  Chat-Completions-native input details, both detail shapes together, mixed
  canonical/native fields, explicit canonical zero buckets, invalid cached
  counts, and a native payload whose stale `Cost` must be cleared. A canonical
  round trip preserves its serialized `Cost`, including an intentionally
  inconsistent `Cost.Total`, proving alias unmarshaling remains lossless when
  no token normalization occurs.
- Pricing backfill: a table enumerates every cache-capable Google and OpenAI
  model expected from the authoritative catalogs (including the
  OpenAI-completions generated view), asserts `CacheReadPrice > 0`, and proves
  cached usage produces `Cost.CacheRead > 0`. A future catalog regeneration
  that drops any listed rate therefore fails instead of silently zeroing
  cache-hit cost.

## Rollout and verification

1. Land the contract docs, decoder fixes, pricing backfill, helper, and tests
   in one change. Generate and verify the catalog outputs with
   `make provider-catalog-generate` and `make provider-catalog-check`; review
   the catalog inputs rather than hand-editing `pricing_gen.go`.
2. Before tagging, run Dive-side live smoke calls with observed cache hits
   against the subset providers (OpenAI, Google, and Grok at minimum;
   OpenRouter and Mistral where keys allow), reconciling decoded buckets
   and `Cost.Total` against each provider's own reported totals.
3. Tag a release; changelog states the semantic correction and the
   reconstruction formula.
4. The reporting consumer bumps its pin and runs its live qualification
   per subset provider — one real call with observed cache reads per
   provider, confirming decoded disjoint counts against the provider's own
   reported totals. Treat that as the field verification for this plan
   too.

## Resolved questions

- **Method name:** `TotalInputTokens()`. The struct's vocabulary is the
  Input family (`InputTokens`, `CacheReadInputTokens`,
  `CacheCreationInputTokens`); "prompt" would import the completions wire
  vocabulary into a type that deliberately doesn't use it.
  (`TotalPromptTokens` and `PromptTokens` were the earlier candidates.)
- **Streaming-accumulator assertion: no — because it is impossible, which
  is a stronger answer than "not worth it."** Disjointness is not
  observable from a decoded `Usage`: nothing in the struct records whether
  `InputTokens` already includes the cached count, and the accumulator
  (`llm/stream.go:204-209`) just sums whatever decoders emit. The only
  place the invariant can be checked is at decode time against wire
  fixtures — which is exactly the cross-provider fixture test above.
  Recorded here so nobody re-proposes the runtime assert later.
- **OpenRouter comment-vs-test: moot.** OpenRouter has no independent
  usage-mapping site — it embeds `openaicompletions` and inherits
  `toLLMUsage`, so it is covered by the fix today, not "when its decoder
  maps them." Same for Mistral. Separate inheritance canaries pin both
  provider surfaces.
