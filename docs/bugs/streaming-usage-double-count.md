# Streaming usage is double counted on the input side

**Status:** fixed — [#261](https://github.com/deepnoodle-ai/dive/pull/261), unreleased. This document is kept as the historical record.
**Severity:** high — corrupted every usage-derived number (cost, billing, context gauges) on streamed Anthropic traffic
**Affected:** `llm/stream.go` (`ResponseAccumulator`), `providers/anthropic`, `providers/ollama`, `experimental/cmd/dive`
**Confirmed on:** `v1.23.0`; present from April 2025 until #261 (see [History](#history))
**Originally reported by:** Mobius Cloud team, 2026-08-12

The sections below describe the defect in the present tense as it existed
before the fix.

## Summary

For streaming requests whose provider forwards Anthropic-shaped SSE frames,
`ResponseAccumulator` counts `input_tokens`, `cache_creation_input_tokens`, and
`cache_read_input_tokens` **exactly twice**.

The accumulator seeds `Response.Usage` from `message_start`, then _adds_ the usage carried by
`message_delta`. Anthropic repeats the full cumulative usage object in both frames, so every
input-side bucket lands twice.

`output_tokens` is inflated by the small starting count `message_start` reports (typically 4),
which is why the defect stays invisible in the number people usually sanity-check.
Non-streaming calls are correct.

## Root cause

`llm/stream.go`, `ResponseAccumulator.AddEvent`:

```go
case EventTypeMessageStart:
    if event.Message == nil {
        return errors.New("invalid message start event")
    }
    r.response = event.Message   // seeds Usage from message_start
    return nil
```

```go
// Update usage information if provided
if event.Usage != nil && r.response != nil {
    r.response.Usage.Add(event.Usage)   // adds message_delta's usage on top
```

`Usage.Add` (`llm/usage.go:172`) is a field-wise sum. Anthropic's `message_delta.usage` is
**cumulative for the whole message**, not incremental — the opposite of what `Add` assumes.

The Anthropic provider decodes raw SSE straight into `llm.Event`
(`providers/anthropic/stream_iterator.go`, a pure passthrough), so `message_start` usage arrives
nested under `message` and becomes the seed, while `message_delta` usage arrives at the top level
and becomes an `Add`.

## Reproduction

Feeding real Anthropic frames through the accumulator:

```text
message_start {"input_tokens":14,"cache_creation_input_tokens":8012,"cache_read_input_tokens":120000,"output_tokens":4}
message_delta {"input_tokens":14,"cache_creation_input_tokens":8012,"cache_read_input_tokens":120000,"output_tokens":7}
```

| bucket                        |    true | accumulated |
| ----------------------------- | ------: | ----------: |
| `input_tokens`                |      14 |      **28** |
| `cache_creation_input_tokens` |   8,012 |  **16,024** |
| `cache_read_input_tokens`     | 120,000 | **240,000** |
| `output_tokens`               |       7 |      **11** |
| `TotalInputTokens()`          | 128,026 | **256,052** |

Verified end to end by driving `dive.NewAgent` against an `httptest` server replaying the SSE
transcript above through the real `providers/anthropic` provider.

## Scope

| Provider                                        | Stream path                                                      | Affected               |
| ----------------------------------------------- | ---------------------------------------------------------------- | ---------------------- |
| `anthropic`                                     | passthrough SSE → `llm.Event`                                    | **yes**                |
| `ollama`                                        | embeds `*anthropic.Provider` (Anthropic-compatible Messages API) | **yes**                |
| `openaicompletions` (→ `mistral`, `openrouter`) | own iterator                                                     | **latent** — see below |
| `openai` (Responses API, → `grok`)              | own iterator; `message_start` carries no usage                   | no                     |
| `google`                                        | own iterator; usage emitted once on `message_delta`              | no                     |

`openaicompletions` is not structurally immune. Its `message_start` carries
`Usage: s.usage.toLLMUsage()` (`stream_iterator.go:167`), populated at `:143` from the same
chunk. It is safe only because OpenAI delivers usage in a trailing choices-empty chunk. A
provider that emits usage on a chunk that _also_ carries choices double counts identically —
measured `input=200` against a true `100`. OpenRouter proxies many upstreams and is the most
plausible route to this.

## Impact on the Dive CLI

Yes — the CLI is affected. `Agent.generateStreaming` (`agent.go:2093`) accumulates through the
buggy path, and the resulting `Response.Usage` flows into the CLI via
`agentEventCallback` → `lastUsage` → `usageUpdateEvent`.

Using the reproduction above (`claude-sonnet-5`, list-price estimate):

- **Token rows** (`render.go:191-193`, `app.go:2548-2557`) — input, cache read, cache write and
  the total row all render at 2x.
- **Context gauge** — `compaction.CalculateContextTokens` returns `TotalInputTokens()`, so it
  reports 256,052 against a true 128,026. Compaction and budget guards trip at roughly half the
  real context size.
- **Cost** — `0.132339` against a true `0.066192`, i.e. 1.999x. Output is not doubled, which is
  why it is marginally under exactly 2x.
- **Percentages are correct.** Cache-hit % (`render.go:220-238`) renders 93% either way, because
  numerator and denominator are doubled together. The same cancellation is why
  `logCacheThrash` (`providers/anthropic/anthropic.go:464`) cannot detect this — it compares
  write against read.

## History

The bug is old and was not introduced by the recent billing work.

| Date       | PR   | Effect                                                                          |
| ---------- | ---- | ------------------------------------------------------------------------------- |
| 2025-04-03 | #21  | Added `r.response = event.Message`. Input/output double count begins.           |
| 2025-06-05 | #39  | Added cache-bucket accumulation. Cache double count — the costly part — begins. |
| 2026-08-10 | #251 | Added `TotalInputTokens()`. Did not touch the accumulator.                      |
| 2026-08-10 | #254 | Replaced five explicit `+=` lines with `Usage.Add()`. Semantics unchanged.      |

The same seed-then-add shape is present verbatim at `v1.15.0` and `v1.20.0`, and `CostOf`
already priced cache buckets separately at `v1.20.0`.

`TotalInputTokens()` (new in #251) is the likely reason this surfaced now: it is the first
helper that rolls the doubled cache buckets into a single headline number. A caller previously
summing only `InputTokens` saw an absolute error of 14 tokens.

### Why tests missed it

`experimental/cmd/dive/usage_invariant_test.go` asserts disjoint input buckets across every
provider, but drives each one through `generateFixture`, which decodes a **single non-streaming
JSON body**. The streaming path it was meant to protect is never exercised.

## Fix

Shipped in #261. The guard lives in the accumulator rather than in a per-provider patch: it
covers Anthropic, Ollama, and the latent `openaicompletions` path in one place, and preserves
the `message_start`-only fields that a provider-side strip would drop.

`Usage.Absorb` (`llm/usage.go`) merges cumulative frames by per-field max, and
`ResponseAccumulator.AddEvent` (`llm/stream.go`) calls it in place of `Usage.Add`:

```go
// Usage frames from Anthropic and Google are cumulative for the whole message,
// not incremental: a later frame supersedes an earlier one field by field.
func (u *Usage) Absorb(other *Usage) {
    u.InputTokens = max(u.InputTokens, other.InputTokens)
    u.OutputTokens = max(u.OutputTokens, other.OutputTokens)
    u.CacheCreationInputTokens = max(u.CacheCreationInputTokens, other.CacheCreationInputTokens)
    u.CacheReadInputTokens = max(u.CacheReadInputTokens, other.CacheReadInputTokens)
    u.ReasoningTokens = max(u.ReasoningTokens, other.ReasoningTokens)
    // ModalityTokens merge by per-bucket max; ServiceTier/Speed take the later
    // non-empty value; boolean flags OR-merge; provider Cost replaces rather
    // than sums. See llm/usage.go for the full rule set.
}
```

Per-field max rather than wholesale replacement matters: `message_start` carries fields that
`message_delta` omits (`service_tier`, the `cache_creation` ephemeral 5m/1h breakdown), so a
straight overwrite loses them. It also fixes the output drift for free. Choosing max over
replacement means the rules hold whether or not `message_delta.usage` repeats every key — the
originally reported dumps show exactly four keys in both frames, which suggests the capture
script selected specific keys.

**`Usage.Add` keeps its additive semantics** for the cross-generation aggregation in
`Agent.chat` (`agent.go:1996`), which was and remains correct.

## Regression tests

Landed with the fix:

1. **Accumulator, cumulative frames** — `TestResponseAccumulatorCumulativeUsageFrames`
   (`llm/stream_test.go`). Replays the reproduction frames with both the cache-write and
   cache-read buckets non-zero, asserting the final frame's values.
2. **Output-only `message_delta`** — `TestResponseAccumulatorOutputOnlyDeltaKeepsSeededInputUsage`
   (`llm/stream_test.go`). Pins why max beats wholesale replacement.
3. **Provider-level golden test** — `TestStreamCumulativeUsageNotDoubleCounted`
   (`providers/anthropic/stream_usage_test.go`). Replays a captured SSE transcript through the
   real provider via `httptest`.
4. **`Usage.Absorb` unit tests** (`llm/usage_test.go`) covering the cost-replace, tier/speed,
   modality, and nil-argument rules.

Still open:

1. **Extend `usage_invariant_test.go` to the streaming path** for every provider — the gap that
   let this through. It still drives each provider through a single non-streaming JSON body.
2. **Pin the `openaicompletions` latent case** with a stream whose usage arrives on a chunk that
   also carries choices. `Absorb` makes it correct today, but nothing asserts it.
