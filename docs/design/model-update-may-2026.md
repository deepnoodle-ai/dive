# Model & Capability Update — May 2026

_Last updated: 2026-06-30_

This document records the latest models and pricing across providers (verified
against official docs) and the changes made to Dive to add the new models and
unlock new capabilities, especially **Claude Opus 4.8**.

## Summary

Dive was already current on Gemini (3.1 Pro) and Grok (4.20). The gaps closed
here:

- **OpenAI**: added GPT-5.5, GPT-5.4 nano, and GPT Image 2; defaulted OpenAI
  text to `gpt-5.5` and OpenAI image generation to `gpt-image-2`.
- **Anthropic**: added Opus 4.7 and 4.8; fixed a bug that made effort/thinking
  unusable on 4.7/4.8; added the native `effort` parameter, adaptive thinking,
  fast mode, thinking-display control, and refusal `stop_details`.
- **Google**: added Gemini 3.5 Flash, 3.1 Flash-Lite (stable), 3 Pro Image
  (Nano Banana Pro), 3.1 Flash Image (Nano Banana 2), 3.1 Flash Live, and the
  custom-tools Pro endpoint.
- **xAI/Grok**: added Grok 4.3 (new default) and grok-build-0.1; corrected
  Grok 4.20 pricing; added Grok Imagine image pricing.

## Latest models & pricing (per 1M tokens, USD)

### Anthropic (Claude API)

| Model | API ID | Input | Output | Context | Max output | Thinking |
|-------|--------|-------|--------|---------|------------|----------|
| **Opus 4.8** | `claude-opus-4-8` | $5 | $25 | 1M (default) | 128k | Adaptive only |
| Opus 4.7 | `claude-opus-4-7` | $5 | $25 | 1M | 128k | Adaptive only |
| Opus 4.6 | `claude-opus-4-6` | $5 | $25 | 1M | 128k | Adaptive + manual (deprecated) |
| Sonnet 4.6 | `claude-sonnet-4-6` | $3 | $15 | 1M | 64k | Adaptive + manual (deprecated) |
| Haiku 4.5 | `claude-haiku-4-5` | $1 | $5 | 200k | 64k | Manual |

Cache (Opus): 5m write $6.25, 1h write $10, hit $0.50. Batch: $2.50 / $12.50.
**Fast mode**: Opus 4.8 $10/$50; Opus 4.6/4.7 $30/$150 (research preview, beta
header `fast-mode-2026-02-01`, requires account access).

### Google (Gemini)

| Model | Input | Output | Notes |
|-------|-------|--------|-------|
| `gemini-3.5-flash` | $1.50 | $9.00 | New stable frontier Flash |
| `gemini-3.1-pro-preview` | $2.00 / $4.00 (>200k) | $12.00 / $18.00 | Flagship preview |
| `gemini-3.1-flash-lite` | $0.25 | $1.50 | Stable; low-latency |
| `gemini-3.1-flash-live-preview` | $0.75 (text) | $4.50 (text) | Live API, audio-to-audio |
| `gemini-3-pro-image` | $2.00 | $120 (img tokens) | Nano Banana Pro, ~$0.134/image |
| `gemini-3.1-flash-lite-image` | $0.30 | $30 (img tokens) | Nano Banana family, ~$0.034/image |
| `gemini-3.1-flash-image` | $0.50 | $60 (img tokens) | Nano Banana 2, ~$0.067/image |

### xAI (Grok)

| Model | Input | Output | Context |
|-------|-------|--------|---------|
| `grok-4.3` | $1.25 | $2.50 | 1M (new flagship/default) |
| `grok-4.20-*` | $1.25 | $2.50 | 1M (corrected from $2/$6) |
| `grok-build-0.1` | $1.00 | $2.00 | 256k (coding) |
| `grok-imagine-image` | $0.02/image | — | — |
| `grok-imagine-image-quality` | $0.05/image | — | — |

### OpenAI

| Model | API ID | Input | Output | Context | Max output | Notes |
|-------|--------|-------|--------|---------|------------|-------|
| **GPT-5.5** | `gpt-5.5` | $5.00 | $30.00 | 1.05M | 128k | New OpenAI default |
| GPT-5.4 | `gpt-5.4` | $2.50 | $15.00 | 1.05M | 128k | More affordable frontier model |
| GPT-5.4 mini | `gpt-5.4-mini` | $0.75 | $4.50 | 400k | 128k | Fast mini model |
| GPT-5.4 nano | `gpt-5.4-nano` | $0.20 | $1.25 | 400k | 128k | Lowest-cost GPT-5.4-class model |
| GPT Image 2 | `gpt-image-2` | — | — | — | — | New OpenAI image-generation default |

## The critical bug (fixed)

Dive mapped `WithReasoningEffort` and `WithReasoningBudget` to
`thinking: {type: "enabled", budget_tokens: N}`. On Opus 4.7/4.8 the Messages
API **rejects manual thinking budgets with a 400 error** — meaning Dive could
not use effort or thinking at all against the new default model.

The provider is now model-aware:

| Model class | Effort | Thinking |
|-------------|--------|----------|
| Opus 4.5+, Sonnet 4.6 | `output_config.effort` (native) | per below |
| Opus 4.6, Sonnet 4.6 | native | adaptive or manual budget (manual deprecated) |
| **Opus 4.7 / 4.8** | native | **adaptive only**; manual budget auto-falls-back to adaptive |
| Older (3.x, 4, 4.5 Sonnet) | emulated via budget (back-compat) | manual budget |

## New capabilities unlocked

Core (`llm` package), provider-neutral where possible:

- `ReasoningEffortXHigh`, `ReasoningEffortMax` effort levels.
- `WithAdaptiveThinking()` / `WithThinking(llm.ThinkingTypeAdaptive)`.
- `WithThinkingDisplay(summarized|omitted)` — note Opus 4.7/4.8 default to
  `omitted`, so set `summarized` if you surface thinking text.
- `WithSpeed(llm.SpeedFast)` — fast mode; adds the beta header automatically.
- `Usage.Speed` — reports which speed served the request.
- `Response.StopDetails` — refusal category on declined requests.

Agent-level: the same knobs are exposed on `dive.ModelSettings`
(`Thinking`, `ThinkingDisplay`, `Speed`).

### Not requiring code changes

- **Mid-conversation system messages** (Opus 4.8): already supported — send a
  message with `llm.System` role in the messages array.
- **1M context on Opus 4.7/4.8**: on by default, no beta header required (unlike
  Opus 4.6, which still needs `context-1m-2025-08-07`).
- **Lower prompt-cache minimum** (1,024 tokens on Opus 4.8): server-side, no
  client change.

## Decisions

- **Default model → Opus 4.8** for the Anthropic and OpenRouter providers
  (matches the "latest and most capable" convention; same standard price as 4.6).
- **`WithReasoningBudget` on Opus 4.7/4.8 auto-converts to adaptive thinking**
  (logs a warning if a logger is set) rather than erroring, so existing
  budget-based callers keep working against the new default.

## Sources

- Claude Opus 4.8 launch notes; Models overview & Pricing
  (platform.claude.com/docs/en/about-claude/models/overview, /pricing).
- Effort, Adaptive thinking, Fast mode docs (platform.claude.com/docs/en/build-with-claude/*).
- Gemini API pricing (ai.google.dev/gemini-api/docs/pricing) and model cards (provided).
- xAI model & pricing tables (provided).
