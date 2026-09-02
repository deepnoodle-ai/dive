# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

- **Meta Model API provider (Muse Spark)** — new `providers/meta` module serving
  `muse-spark-1.3` and its 1.2/1.1 siblings (1M context, $1.25/$4.25 per MTok;
  a discounted `-contributor` tier trades a lower price for training on your
  data). Targets Meta's Responses API, because Chat Completions redacts
  reasoning for external keys and so cannot carry it across tool turns.
  Verified against the live API, including encrypted-reasoning replay across a
  tool turn; see `docs/design/meta-model-api.md`.

## [1.27.0] - 2026-09-02

### Added

- **Gemini 3.8 Flash** — new `google.ModelGemini38Flash` (1M context,
  $0.75/$3.75 per MTok). It becomes `google.DefaultModel` and takes Gemini 3.7
  Flash's CLI recommendation slot; 3.7 Flash remains catalogued. OpenRouter
  picks up `google/gemini-3.8-flash`.
- **Gemini 3.5 Transcribe, Transcribe Live, and Omni 1.1 Flash** —
  `ModelGemini35Transcribe`, `ModelGemini35TranscribeLive`, and
  `ModelGeminiOmni11Flash` for Google's new speech-to-text and video models.
- **Claude Fable 5.1 and Mythos 5.1** — new `ModelClaudeFable51` and
  `ModelClaudeMythos51` constants (1M context, $10/$50 per MTok). Fable 5.1
  replaces Fable 5 in the CLI recommendations; Fable 5 and Mythos 5 remain
  available.
- **OpenRouter catalog picks up the current frontier ids** —
  `anthropic/claude-fable-5.1`, `openai/gpt-5.6-sol` / `-terra` / `-luna`,
  `google/gemini-3.7-flash`, and `x-ai/grok-4.6`.
- **`ModelCodestral2508`** — the current Codestral release, and
  `ModelCodestralLatest` now carries the Mistral CLI's coding recommendation.

### Fixed

- **Claude Sonnet 5 was priced 50% too high.** Its $2/$10 per MTok launch rate
  became the standard price; Anthropic cancelled the increase to $3/$15 that
  the catalog had pre-applied.
- **Gemini 3.7 and 3.6 Flash were priced 2x too high.** Both now record the
  $0.75/$3.75 per MTok rate billed today rather than the $1.50/$7.50 scheduled
  for 2027-01-01. Catalogs record the price in effect, not a future one.
- **Fable 5.1 and Mythos 5.1 cache reads bill at 0.025x base input**, not the
  0.1x Dive derives for every other Claude model, so the $0.25 per MTok rate is
  stated in the catalog instead of derived 4x too high.
- **OpenAI-compatible error bodies with a bare-string `error` member keep their
  status code and message.** openai-go v3.51 stopped parsing the xAI/Grok shape
  `{"code":...,"error":"<message>"}` into an SDK error at all, surfacing a raw
  `json.UnmarshalTypeError` instead; a 403 for exhausted credits lost both its
  status and its reason, and no longer classified for retry.

### Changed

- **Dive now requires Go 1.26.** All 11 modules moved from `go 1.25.0` to
  `go 1.26.0`, and the pinned toolchain from go1.26.5 to go1.26.8, which clears
  six Go standard library advisories (`net/http`, `crypto/tls`, `net/url`,
  `encoding/asn1`, `encoding/xml`, `html/template`). `govulncheck` now reports
  zero vulnerabilities across every module.
- **Dependencies refreshed across every module.** Notably
  `google.golang.org/genai` v1.68.0 → v1.71.0, `google.golang.org/api` v0.293.0
  → v0.297.0, `openai/openai-go/v3` v3.50.0 → v3.55.0 (the v3.51 hold is
  lifted), `mark3labs/mcp-go` v0.58.0 → v1.0.0, `gobwas/glob` v0.2.3 → v1.0.0,
  `wonton` v0.0.38 → v0.0.39, and OpenTelemetry v1.45.0 → v1.46.0.
- **Mistral's Devstral models are marked deprecated.** Mistral has retired the
  family, and `devstral-small-latest` no longer resolves upstream at all; the
  constants remain for compatibility.

## [1.26.0] - 2026-08-22

### Fixed

- **OpenAI Responses assistant messages keep their `phase`.** The phase
  (`commentary` or `final_answer`) now decodes onto each text block as
  `openai.phase` provider metadata and is replayed on follow-up requests, so
  runtimes that persist and resend history manually no longer silently drop it.
  Unlabeled messages stay unphased.
- **Compaction keeps provider metadata on truncated content.** Shrinking an
  oversized text or `tool_use` block no longer strips replay state such as a
  Google thought signature or an OpenAI message phase.

### Added

- **The experimental `dive` CLI can skip tool approval prompts with
  `--dangerously-skip-permissions`.** The default approval flow is unchanged;
  use the bypass only in an externally sandboxed environment.

### Changed

- **OpenAI streaming closes a text block on its output item's done event**
  rather than on `output_text.done`, so metadata that arrives only with the
  done event — such as a late `phase` label — reaches consumers while the block
  is still open. A response that ends without an item's done event now closes
  every block it left open — text, reasoning, or tool call — before
  `message_delta`, matching the other providers' event order.
- **OpenAI-compatible chat completions streams always terminate cleanly.** A
  stream that reaches `[DONE]` or EOF without a `finish_reason` — as seen
  through Mistral and OpenRouter — now closes its open text and tool-call
  blocks and emits `message_delta` and `message_stop`, instead of ending with
  the message still open.

## [1.25.1] - 2026-08-15

### Fixed

- **Google text generation works on Vertex AI again.** Since v1.21.0 the
  provider sent `serviceTier: "unspecified"` when no tier was requested, which
  Vertex AI rejects with a 400 on every `Generate` and `Stream` call; the field
  is now omitted unless a tier is explicitly set. The Gemini Developer API
  backend was unaffected.

## [1.25.0] - 2026-08-15

### Added

- **The `dive` CLI now loads a `.env` file from the current directory on
  startup** (via `wonton/env`), so provider API keys like `MISTRAL_API_KEY`
  no longer need to be exported into the shell manually. Existing
  environment variables take precedence; a missing `.env` is not an error.
- **Gemini 3.7 Flash** — `gemini-3.7-flash` added to the Google catalog and
  pricing; now the Google default. Google publishes identical list pricing to
  `gemini-3.6-flash`, which remains available. Its thinking parameters were
  verified live against Vertex AI: unlike the rest of the 3.x flash family it
  rejects `MINIMAL` effort and its `thinkingBudget` ceiling is 32768, not
  65535 (see `providers/google/capabilities.go`).
- **Mistral Medium 3.5** — `mistral-medium-latest` added to the Mistral
  catalog and pricing ($1.50/$7.50 per 1M tokens, 256k context); now the
  Mistral default, replacing `mistral-large-latest` per Mistral's own "becomes
  the default model in Le Chat" announcement. `mistral-large-latest` and
  `mistral-small-latest` also had their published list prices corrected —
  Large fell to $0.50/$1.50 (from a stale $2.00/$6.00) and Small rose to
  $0.15/$0.60 (from a stale $0.10/$0.30) — both now match Mistral's current
  pricing page rather than late-2025 figures.

## [1.24.0] - 2026-08-12

### Fixed

- **Streaming usage no longer double counts the input side.** Anthropic-shaped
  streams repeat cumulative usage in both `message_start` and `message_delta`;
  `ResponseAccumulator` now merges frames by supersession (new `Usage.Absorb`)
  instead of summing, so input, cache, and cost figures are no longer ~2x on
  streamed Anthropic/Ollama traffic. `Usage.Add` keeps its additive semantics
  for cross-request aggregation.

## [1.23.0] - 2026-08-12

### Added

- **Grok 4.6** — `grok-4.6` added to the Grok catalog and pricing; now the
  Grok default, with the same 500k context as `grok-4.5` but higher cached-
  and long-context pricing. Its reasoning-effort ladder is narrower than
  `grok-4.5`'s (`low`/`medium`/`high`/`xhigh`, no `none` or `minimal`). Also
  adds `grok-imagine-image-2.0` ($0.04/image).

## [1.22.0] - 2026-08-11

### Fixed

- **Provider reasoning blocks now survive response copying, streaming, and
  multi-turn replay.** Gemini preserves thought parts and positional thought
  signatures (including signed text and empty parts); Mistral preserves its
  structured thinking chunks; OpenRouter retains plaintext and structured
  `reasoning_details`; and OpenAI Responses retains summaries, raw reasoning
  text, reasoning item IDs, and encrypted content from streamed responses.

## [1.21.0] - 2026-08-10

### Fixed

- **Unknown tool names no longer abort and discard an agent turn.** Dive returns
  a typed `UnknownToolError` result with deterministic suggestions, preserves
  valid sibling calls, and uses the standard tool-iteration limit.
- **`llm.Usage` input buckets are now consistently disjoint.** OpenAI
  Responses, OpenAI Completions, OpenRouter, Mistral, Grok, and Google now
  subtract cache hits from `InputTokens` before pricing. This fixes cached
  tokens being charged at both the normal input rate and the cache-read rate.
  GPT-5.6 cache writes are also decoded into `CacheCreationInputTokens`, so
  they receive the published write rate instead of the normal input rate. To
  reconstruct the pre-fix value, use
  `old input = new input + cache read + cache write`.
- **OpenAI prompt-cache reporting and routing now match GPT-5.6.** Agent
  sessions use a stable hashed `prompt_cache_key`, native OpenAI requests mark
  reusable prefix breakpoints, and the CLI's cache-hit percentage is cache
  reads divided by total input rather than reads divided only by cache
  activity. A small cached prefix can no longer render as `100%`. The CLI's
  model-only skill catalog is also delivered before durable conversation
  history so it remains reusable instead of being rewritten on every turn.
- **Cache-read pricing is populated for every Google and OpenAI text model
  with a published discounted-input rate.** The OpenAI catalog also corrects
  the stale GPT-4o row to the current $2.50 input, $1.25 cached-input, and
  $10 output rates per million tokens. Dated provider model IDs now resolve to
  their stable catalog ID so live OpenAI responses retain `Cost.Total`.
- **Grok agents now send a stable `prompt_cache_key`.** Session-backed agents
  derive a private key from the session ID; stateless agents use an
  agent-instance key and can provide a conversation key per call. The Grok
  catalog also uses xAI's current cached-input rates and applies its input,
  cached-input, and output long-context tier at 200K total input tokens.
- **Gemini usage and cost estimates now reconcile to Google's billing shape.**
  Thinking and tool-use tokens are included, multimodal and Vertex regional
  rates are honored, all Pro rates switch above 200K input, and unsupported
  tiers or incomplete price dimensions remain explicitly unpriced. A provider
  aggregate mismatch preserves completed content and component counts while
  marking the cost estimate unavailable.
- **OpenRouter costs now use the authoritative charge returned in `usage.cost`.**
  This covers arbitrary routed models and provider-specific cache/tool charges
  without depending on a necessarily incomplete static price snapshot. A null
  usage object is treated as missing telemetry rather than a measured zero.

## [1.20.0] - 2026-08-09

### Changed

- **`anthropic.DefaultModel` is now `ModelClaudeSonnet5`** (`claude-sonnet-5`),
  replacing `claude-opus-5`. Pass `WithModel(ModelClaudeOpus5)` to keep Opus 5.
- **`openrouter.DefaultModel` is now `ModelClaudeSonnet5`**
  (`anthropic/claude-sonnet-5`), matching the Anthropic provider default.
- **The CLI's Anthropic default follows `anthropic.DefaultModel`** rather than a
  separate hardcoded `claude-haiku-4-5`.
- **The CLI defaults `--thinking-effort` to `medium`.** Providers now drop the
  parameter on models that have none, so this is safe on every model.
- **Unsupported reasoning and sampling settings are clamped or dropped with a
  warning instead of returning an error.** One `ModelSettings` now survives
  being pointed at a model with a narrower range.
- **New `providers/modelcaps` package** holds the verified per-model capability
  tables shared by the Responses and Chat Completions providers, replacing three
  separate copies of the same prefix switches.
- **New `llm.ClampReasoningEffort`** maps a requested effort onto the closest
  level a model accepts.
- **Gemini thinking control is now wired up.** `WithReasoningEffort` maps to
  `thinkingConfig.thinkingLevel`, or to a clamped `thinkingBudget` on models
  with no thinking level (the 2.5 generation); `WithReasoningBudget` maps to
  `thinkingBudget`, `WithAdaptiveThinking` to a dynamic budget, and
  `WithThinkingDisplay` to `includeThoughts`. All four were previously
  discarded before the request was built.

### Fixed

- **Gemini thought summaries were emitted as answer text.** With thoughts
  enabled, the non-streaming path spliced the model's reasoning into its reply;
  the streaming path dropped it. Both now produce `llm.ThinkingContent`.
- **Reasoning effort was sent to models that have no reasoning parameter**,
  producing a 400 on every request. Affects `gpt-4o` and `gpt-4.1`, plus
  `grok-build`, `grok-code-fast`, and both `grok-4.20-0309` models. The CLI's new
  `medium` default made this fire on every call.
- **`temperature` was sent to models that reject it** on the OpenAI providers:
  `gpt-5`/`-mini`/`-nano`, `gpt-5.5`, `gpt-5.6`, `o3`, and `o4-mini`.
- **Adaptive thinking was sent to models that do not support it.** Haiku 4.5,
  Sonnet 4.5, and Opus 4.5 answer "adaptive thinking is not supported on this
  model"; the request now falls back to a manual thinking budget.
- **Codex models bypassed effort clamping entirely**, so `minimal` and `max`
  reached `gpt-5.3-codex`, which rejects both.
- **A reasoning budget and an effort together were rejected client-side** on
  Opus 4.5 and Sonnet 4.6, which accept the combination.
- **Effort ranges corrected against the live API:** `grok-4.5` accepts `xhigh`
  (was downgraded to `high`) and rejects `none`; the Grok multi-agent model
  accepts `max` and `none`; `gpt-5.2-pro` accepts only `medium`/`high`/`xhigh`;
  `gpt-5.3-chat-latest` accepts only `medium`.
- **`claude-opus-5` was missing from every Anthropic reasoning classification**,
  so effort fell through to the legacy thinking-budget path and `max`/`xhigh`
  silently became `high`. It now uses native `output_config.effort`.
- **`temperature` is now dropped on Opus 4.7/4.8**, which reject it with a 400.
- **`xhigh` is no longer downgraded on Sonnet 5**, which supports the full ladder.
- **Forced `tool_choice` no longer fails client-side on Opus 5 and Sonnet 5**,
  which default thinking on but accept an explicit disable.

## [1.19.0] - 2026-08-09

### Added

- **Provider catalogs** — each provider's models, pricing, feature flags, and
  documentation sources now live in `providers/<name>/catalog.json`, generating
  `models_gen.go`, `pricing_gen.go`, and `features_gen.go` via
  `make provider-catalog-generate`. Every provider exposes `Catalog()` and
  `CatalogJSON()`.
- **Provider watch** — a weekly workflow (`scripts/provider_watch.py`) diffs
  upstream provider documentation and APIs against an accepted baseline and
  files a single refreshed issue when something material changes. It also
  reports models published upstream that Dive lacks (`gaps`) and catalog ids
  upstream does not serve (`unverified`), reporting only what is new relative to
  the baseline.
- **`make release-prep VERSION=vX.Y.Z`** points every sub-module's intra-repo
  requirement at the version being released. `make tag-modules` now refuses to
  tag while those requirements are stale.
- **`grok.ModelImagineVideo15`** (`grok-imagine-video-1.5`).

### Deprecated

- **Twelve Grok constants xAI has retired** — `grok-3`, `grok-3-latest`,
  `grok-3-mini`, `grok-4`, `grok-4-latest`, `grok-4-0709`, the four
  `grok-4[-1]-fast-*` slugs, `grok-code-fast-1`, and `grok-imagine-image-pro`.
  The slugs still resolve but redirect to `grok-4.3`, `grok-build-0.1`, or
  `grok-imagine-image-quality`, and bill at the target's rates.

### Fixed

- **Blank error messages from OpenAI-compatible providers** — xAI/Grok returns
  `{"code":…,"error":"<message>"}`, a shape the OpenAI SDK does not parse, so
  failures surfaced as `provider api error (status 403):` with no reason. The
  message now falls back to the raw JSON body.
- **Nine OpenRouter ids used the wrong separator** — OpenRouter serves
  `anthropic/claude-opus-4.7`, not `anthropic/claude-opus-4-7`. Same for Opus
  4.8/4.6/4.5/4.1, Sonnet 4.6/4.5, Haiku 4.5, and one pricing row.
  `openrouter.DefaultModel` was among them, so every default request used an
  unresolvable id; it now points at the new `anthropic/claude-opus-5`.
- **Six Mistral constants pointed at ids Mistral does not serve.**
  `ModelMistralLarge3` → `mistral-large-2512` (`2412` never existed),
  `ModelMinistral3_3B/_8B/_14B` → `ministral-{3b,8b,14b}-2512`,
  `ModelDevstral2` → `devstral-2512`, `ModelDevstralSmall2` →
  `labs-devstral-small-2512`.
- **Grok reasoning-effort clamping skipped `grok-build-0.1`** — it keyed on
  `grok-build-latest`, which xAI does not serve, so `xhigh`/`max` reached the
  API unmapped.
- **Google's embedding pricing listed the shut-down `text-embedding-004`** —
  replaced with `gemini-embedding-001` ($0.15/1M) and `gemini-embedding-2`
  ($0.20/1M text input).
- **Retired Grok slugs are costed at the rate they actually bill.** xAI
  redirects them and bills at the target's rates, so `grok-4[-1]-fast-*` had
  been understating cost more than fivefold ($0.20/$0.50 versus grok-4.3's
  $1.25/$2.50). `grok-3` and `grok-4-0709` were overstating it.

### Changed

- **Every module pins `toolchain go1.26.5`** and refreshes its dependencies
  (openai-go v3.50.0, mcp-go v0.57.0, a2a-go v2.4.0, otel v1.45.0). The minimum
  `go` directive stays at 1.25.0; the pin closes standard-library CVEs.
- **`mistral.DefaultModel` is now `ModelMistralLarge`** (`mistral-large-latest`)
  rather than a pinned dated snapshot, matching the model the CLI recommends.
  Pass `WithModel` explicitly to pin a dated snapshot.
- **`openrouter.ModelMistralLarge3` is now `mistralai/mistral-large-2512`**
  (was `mistral/mistral-large-3`, which is not an id OpenRouter serves). Code
  comparing against the old string — routing tables, stored session metadata —
  needs updating.
- **`openrouter` replaced three retired xAI models** (`x-ai/grok-3`,
  `x-ai/grok-4-fast-reasoning`, `x-ai/grok-4-1-fast-reasoning`) with
  `x-ai/grok-4.5`, `x-ai/grok-4.3`, and `x-ai/grok-build-0.1`.
- **Ollama catalog rebuilt around current model families** — GPT-OSS, Qwen3.6,
  Gemma 4, GLM-4.7 Flash, MiniMax, and DeepSeek-R1. `ollama.DefaultModel` is now
  `ModelGPTOSS_20B` (`gpt-oss:20b`), replacing `llama3.2:3b`. MiniMax M2.7 and
  M3 are Ollama Cloud only (`:cloud` tag, no pricing). Each family also gets an
  untagged constant, and `glm-` now routes to Ollama. Mistral models are absent
  by design — use the `mistral` provider.
- **`anthropic.ModelClaudeOpus5` added and made the default** (`claude-opus-5`,
  1M context, $5/$25 per MTok). `ModelClaudeOpus48` remains but is no longer the
  default.
- **`google.DefaultModel` is now `ModelGemini36Flash`** (`gemini-3.6-flash`),
  replacing `gemini-2.5-pro`.
- **Recommended model lists trimmed to one model per class.** Dropped from the
  recommendations, constants intact: `claude-opus-4-8`; `gpt-5.5`, `gpt-5.4`,
  `gpt-5.4-mini`; `gemini-3.5-flash`, `gemini-2.5-pro`; `grok-4.3`,
  `grok-4.20-0309-reasoning`.
- **The CLI no longer falls back to model-family heuristics** for context window
  and label lookup; both come from the embedded catalogs alone. Models the
  catalogs do not list hide the context bar rather than showing a guess.

### Removed

- **`google.ModelGemini25ProLong`** — never a callable model, only a key for
  Gemini 2.5 Pro's over-200K pricing tier, now folded into the `gemini-2.5-pro`
  pricing note.
- **Four constants naming models that do not exist** —
  `grok.ModelGrok45Latest`, `grok.ModelGrokBuildLatest`, `openai.ModelGPT51Mini`,
  and `openai.ModelGPT53CodexSpark`. xAI publishes no `-latest` aliases, and
  OpenAI has no `gpt-5.1-mini` or `gpt-5.3-codex-spark`.
- **`grok.ModelGrok2Vision1212` and `grok.ModelGrok2Image1212`** — xAI no longer
  lists either model; both were already marked deprecated.
- **Ollama constants for retired model families** — Llama 3.x, CodeLlama,
  Gemma/Gemma 2, Qwen 1, Phi-3, and `mistral:7b`/`mistral:nemo`. Any model
  string still works via `WithModel`; only the named constants are gone.

## [1.18.0] - 2026-07-22

### Added

- **Dragged-in files as CLI attachments** — dropping a file onto the terminal
  pastes its path, and the interactive CLI now turns those insertions into
  attachments: images, PDFs, and videos become native content blocks, text
  files are inlined like an `@reference`. Each file is replaced with a
  placeholder (`[Image #1]`) that releases the attachment when deleted.
  Detection is narrow — an inserted run counts as a drop only when it is
  entirely paths to files that exist — so typing and pasted logs can't trigger
  it.
- **Multimodal input support matrix** in the LLM guide.

### Fixed

- **Caller-supplied media across providers** — images and documents were
  failing or silently dropping in several encoders. Google now encodes
  `DocumentContent` instead of dropping it, and its content switch errors by
  default so nothing falls through unnoticed. OpenAI (Responses) accepts
  URL-source and text-source documents. The openaicompletions encoder was
  text-only, so no Chat Completions endpoint could receive images or PDFs; it
  now supports content parts, which also covers Mistral and OpenRouter.
- **Typed blocks in tool results** (e.g. MCP tools returning screenshots) —
  Anthropic emitted image blocks in a shape it does not accept, and OpenAI
  (Responses) JSON-marshaled typed text into the output string; both now encode
  natively. Google and openaicompletions render an explicit placeholder instead
  of dropping non-text blocks. Adds a shared `providers.ToolResultBlocks`
  helper for decoding typed blocks in both in-memory and round-tripped shapes.
- **Empty and nil tool results** — a tool returning no output reached the wire
  as `"content":[]` on Anthropic (rejected), `"null"` on Google, or a hard
  error on the chat completions path. All four providers now substitute a
  shared `providers.EmptyToolResultText` placeholder. Empty strings are left
  alone, since a caller chose that value explicitly.
- **Media sizing during compaction** (experimental) — `estimateTokens` sized
  media by serialized JSON, over-counting by two orders of magnitude, so a
  single attached image tripped mid-turn compaction on the first turn and
  summarized itself away. Media is now sized by actual cost. Also fixes
  `reduceToSummaryBudget` culling attached images regardless of budget.
- **Video content labeling** — videos passed as `@references` were labeled
  `[document: ...]` in the prompt.

## [1.17.0] - 2026-07-21

### Added

- **Gemini 3.6 Flash and 3.5 Flash-Lite** — added stable model constants,
  pricing, 1M-context CLI metadata, and model-picker entries. The CLI now
  defaults to Gemini 3.6 Flash when Google credentials are available, and the
  Google adapter omits deprecated temperature settings for the new request
  generation and logs a warning when a configured temperature is ignored.

### Fixed

- **Google tool results after session replay** — tool result content that
  round-trips through JSON (session persistence, `Message.Copy`) arrives as
  `[]any` rather than typed `[]*dive.ToolResultContent`, so the next turn of a
  multi-turn conversation with tool calls failed with
  `unknown content type: []interface {}`. The Google adapter now decodes the
  round-tripped shape into typed blocks, matching the openaicompletions
  provider.

## [1.16.0] - 2026-07-16

### Fixed

- **Resumed tool results in the response stream** — results supplied via
  `WithToolResults`/`WithResume` are now emitted as `tool_call_result`
  response items (after post-tool hooks, in tool-call order, exactly once
  per result). Previously they were invisible to stream consumers, so
  transcripts built from response items drifted behind the authoritative
  history. See the suspend-resume guide's Streaming section.

### Changed

- **Resume-phase partial-work errors** — failures during resume emission or
  not-started tool execution now carry all items emitted so far in
  `*GenerationError`, matching the generate loop. The session keeps its
  suspended turn, so the resume can be retried.

## [1.15.1] - 2026-07-15

### Fixed

- **Streaming generation retries** — transient failures (rate limits,
  overload, disconnects) that occur before a streaming generation's first
  event are now retried across providers, so a multi-generation turn no
  longer fails outright on a momentary capacity error.
- **Same-name reminder interpretation** — the standing priming rule now keeps
  independent same-name facts and instructions cumulative, using later-wins
  ordering only where two blocks conflict.
- **Skill catalog under the cumulative rule** — the catalog header is now a
  completeness claim ("any skill not listed here is unavailable"), so a later
  catalog conflicts with and replaces a stale one even when skills were
  removed, and evicting a stale catalog when no skills remain emits an
  explicit no-skills notice instead of an empty block (an empty block asserts
  no facts, so nothing would conflict and the stale catalog would survive).

## [1.15.0] - 2026-07-10

### Added

- **Dynamic context-injection demos** — the experimental CLI's repeatable
  `--context-demo` flag offers five focused presets: a live workspace pulse, a
  delivery-pipeline map with automatic Go module and format/test/vet/race
  guidance, verification debt plus observed gate outcomes, failure-specific
  recovery coaching, and security-review triggers for sensitive changes and
  high-impact commands. Interactive runs now trace
  reminder lifecycle events, expose exact latest-turn payloads through `/context`,
  list presets with `dive context-demos`, and warn when the workspace is below
  the Git root. Turn-local ledgers are bounded and deterministic, untrusted
  repository text is excluded from elevated reminders, and verification
  recognizes direct, unmasked check commands. `--context-demo all` enables the
  complete demo set.

### Fixed

- **Failed Read status** — failed Read tool calls now show the actual error
  instead of misleadingly reporting that one line was read.

## [1.14.0] - 2026-07-09

### Added

- **Typed context reminders** — reminders are appended with one of two explicit
  lifetimes: `Recorded` reminders enter conversation history, while `ModelOnly`
  reminders disappear after the current response. Operator reminders use a native
  `system` (Anthropic Opus 4.8) or `developer` (OpenAI) role where
  known-supported, falling back to a tagged user message everywhere else. The
  experimental Dive CLI exposes reminders as a demo platform, and
  provider-tagged integration tests exercise live delivery.

### Changed

- **Reminder priming** — agent system prompts now include the reminder
  interpretation rule on every generation, even when no reminder is currently
  present. This keeps prompt caching stable when reminders appear later, with a
  one-time cache-prefix change for existing agents after upgrading.

### Fixed

- **GPT-5.4 mini Chat Completions tools** — requests that combine function
  tools with a non-`none` reasoning effort now preserve the tools, use
  `reasoning_effort: none`, and emit a warning instead of failing with an
  OpenAI HTTP 400 response.

### Security

- **Dependency updates** — bumped `golang.org/x/crypto`, `x/net`, `x/sys`,
  `x/term`, `x/text`, and `x/image` (the latter a direct dependency of
  `media/format.go` and the OpenAI/Grok media decoders) to their latest patch
  releases across every module, including `demos/colosseum`, closing all 20
  Dependabot alerts open against the repo (7 critical, 8 high, 5 moderate).

## [1.13.0] - 2026-07-09

### Added

- **OpenAI GPT-5.6** — `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, and
  the `gpt-5.6` alias are now in the OpenAI catalogs with pricing, context
  metadata, and `max` reasoning effort support. The Responses provider and CLI
  now default to `gpt-5.6-sol`, and `openai-go` is pinned to the GPT-5.6 SDK
  commit until the `v3.42.0` tag is available.

## [1.12.0] - 2026-07-09

### Added

- **Grok 4.5** — `grok-4.5` (with `grok-4.5-latest` and `grok-build-latest`
  aliases) added to the Grok catalog and pricing; now the Grok default, with
  500k context and cached-input cost estimates.
- **Anthropic summarized thinking** — `ThinkingDisplaySummarized` requests
  visible summarized thinking on Claude models that otherwise omit it (Sonnet 5,
  Opus 4.7/4.8, Fable/Mythos 5), and `Usage.ReasoningTokens` now counts
  Anthropic's reported thinking tokens. The CLI gains `--show-thinking` and
  `--thinking-effort` (env `DIVE_SHOW_THINKING` / `DIVE_THINKING_EFFORT`).

## [1.11.1] - 2026-07-03

### Fixed

- **OpenAI tool-result ordering** — hook `AdditionalContext` text mixed with
  `tool_result` blocks no longer breaks the OpenAI providers. Tool results are
  now emitted before the auxiliary text on both Chat Completions and the
  Responses API (which previously errored; also covers Grok), and
  durable-storage tool results decode correctly.

## [1.11.0] - 2026-06-30

### Added

- **Claude Sonnet 5** — `claude-sonnet-5` added to the Anthropic provider
  catalog, pricing, and capabilities, and to the CLI.
- **Nano Banana image models** — Gemini image-generation models added to the
  Google provider catalog and pricing.

### Fixed

- **OpenRouter SSE keep-alive comments** — the OpenAI-completions stream parser
  now skips SSE comment lines (`: OPENROUTER PROCESSING` keep-alives), which
  previously failed with `invalid character ':'` on slower models.

## [1.10.1] - 2026-06-29

### Fixed

- **Google thought signatures** — Gemini 3 returns an opaque `thought_signature`
  on each function-call part and rejects later requests (HTTP 400) if it is not
  echoed back. Tool calls now carry this signature on a new
  `llm.ToolUseContent.Metadata` field (type `llm.ProviderMetadata`, an opaque
  per-provider round-trip bag namespaced by provider key) and the Google
  provider replays it on subsequent turns. Preserved across both streaming and
  non-streaming responses and through session serialization.

## [1.10.0] - 2026-06-29

### Added

- **Google Vertex AI backend** — `google.WithVertexAI(location)` routes a single
  Google provider instance through the Vertex AI backend using Application
  Default Credentials, independent of the process-wide
  `GOOGLE_GENAI_USE_VERTEXAI` environment variable. An empty location is
  resolved by the genai SDK from `GOOGLE_CLOUD_LOCATION`/`GOOGLE_CLOUD_REGION`
  before defaulting to `global`. The provider's client initialization now
  selects the backend explicitly: the Vertex path passes only project/location
  (the genai SDK treats an API key as mutually exclusive with them), while the
  Gemini API path passes only the API key.

## [1.9.0] - 2026-06-20

### Added

- **Anthropic hybrid prompt caching** — the single tail-only cache breakpoint
  is replaced with a 4-slot automatic + explicit strategy that fixes the
  full-prefix rewrites and 20-block lookback overruns behind the prior cost
  incident. Slot 2 puts an explicit breakpoint on the last system block
  (caching tools + system); slot 1 lets the first-party endpoint's top-level
  automatic `cache_control` own the moving tail, with a portability fallback to
  an explicit tail breakpoint on Bedrock/Vertex/custom endpoints; slots 3–4
  walk backward keeping ≤15 blocks between breakpoints within the remaining
  budget. `ToolUseContent` is now anchorable so an anchor can land inside a
  parallel tool-call fan-out turn. `CacheControl` gains a `TTL` field with
  `CacheTTL5m`/`CacheTTL1h` constants (1h applied to the stable prefix only
  when `FeatureExtendedCache` is on; the tail stays 5m), and a cache-thrash
  warning fires when cache writes dominate reads.
- **Per-call usage cost estimation** — monetary cost is now a first-class part
  of every generation, computed where the tokens are produced so per-call
  costs sum correctly across model/speed changes. `llm.PricingInfo` gains
  `CacheReadPrice`/`CacheWritePrice` and a `Cost` breakdown via
  `PricingInfo.CostOf(usage)`; `llm.Usage` carries `Cost *Cost` (nil = unknown,
  distinct from a known $0) with cost-aware `Add`/`Copy`. A cost-resolver hook
  (`SetCostResolver`/`PopulateCost`) lets `llm` price usage without importing
  providers, and the streaming accumulator attaches cost at message completion
  for all providers. The providers registry adds
  `RegisterPricing`/`PricingFor`, populated from each provider's `init()`,
  wiring up the previously unused per-provider pricing tables across Anthropic,
  OpenAI, Google, Grok, Mistral, Ollama, OpenRouter, and openaicompletions.
- **CLI token/cost visibility** — the ambiguous "in / cache / out" status line
  is replaced with a labeled per-scope table (input, cache read, cache write,
  output, hit rate, cost) that colors the hit rate by health so cache thrash is
  immediately visible, with a fast-mode badge and a reasoning column when
  present. A new `/usage` command (alias `/cost`) renders a persistent,
  fully-labeled breakdown per scope with a legend. The cost column appears only
  when pricing is known; "—" marks an unknown per-scope cost, and cost is
  labeled an estimate at list prices, not a bill.

### Changed

- **`wonton` upgraded to v0.0.36** across all 11 modules, with call sites
  adapted to the updated API: CLI option constructors drop the empty short-flag
  argument, the image example's binary fetch moves to `fetch.Download`,
  firecrawl maps API errors onto the new `fetch.Error` struct, and the
  firecrawl/google/kagi web-search adapters return `[]web.SearchItem` value
  slices. Adds locking around the focused-`InputField` key-routing contract
  introduced in wonton ≥ v0.0.35.
- **Anthropic request shape** — `Request.System` is now `[]*SystemBlock`
  instead of a string, and a top-level `Request.CacheControl` supports
  automatic caching. The GA `prompt-caching-2024-07-31` beta header is no
  longer sent by default and the invalid `CacheControlTypePersistent` constant
  is removed.

## [1.8.1] - 2026-06-09

### Fixed

- **Background task cancellation** — background goroutines spawned by tools
  were prematurely cancelled when their tool batch completed, because they
  inherited the batch-scoped `childCtx`. They now receive the outer
  `CreateResponse` context via a new `withBackgroundCtx`/`backgroundCtxFrom`
  helper, so background tasks live for the full agent turn.
- **CLI temperature zero-value** — the `--temperature` flag was always written
  to `ModelSettings.Temperature` (even when not set), forcing every request to
  `temperature=0`. The CLI now uses `ctx.IsSet("temperature")` and only sets
  the field when the flag is explicitly provided.
- **Claude 5 temperature rejection** — Fable 5 and Mythos 5 reject the
  temperature parameter at the API level. The Anthropic provider now skips
  setting temperature for these models, and logs a warning when a non-nil
  temperature is silently dropped.

## [1.8.0] - 2026-06-09

### Added

- **Claude Fable 5 / Mythos 5** — new `ModelClaudeFable5` and
  `ModelClaudeMythos5` constants with pricing (1M context / 128k output),
  adaptive-thinking and native-effort support (all five levels including
  `xhigh`/`max`), an OpenRouter `anthropic/claude-fable-5` ID, and a CLI
  model-picker entry for Fable 5. The Anthropic default stays
  `claude-opus-4-8`.
- **`SequentialOnlyHint` tool annotation** — a tool that mutates shared state
  can opt out of parallel execution; any batch containing such a tool runs
  sequentially even when `ParallelToolExecution` is enabled.
- **Scoped session permission grants** — `AllowToolForSession(tool, pattern)`
  grants (tool, specifier)-scoped session approvals. Dialog approvals now
  grant the exact approved command/path (or WebFetch domain) instead of a
  whole tool category; `AllowForSession`/`IsSessionAllowed` are deprecated but
  still honored.
- **Partial-work error reporting** — a mid-turn LLM failure now returns
  `*GenerationError` carrying the accumulated `Usage`, `OutputMessages`, and
  `Items` (recover via `errors.As`). New sentinels: `ErrReentrantSession` (a
  tool calling back into its own session fails fast instead of deadlocking)
  and `ErrSessionNotSuspended` (resume against a non-suspended session is
  detected before the LLM call, not after).
- **Demos** — `demos/colosseum` (agent tournament arena with A2A remote
  players) and `demos/noodleville` (agent-driven village simulation).

### Security

- **Permission deny rules are now absolute** — deny rules evaluate before the
  session allowlist, and specifier-bearing deny rules fail closed when no
  specifier can be extracted. Bash patterns match per command segment,
  quote-aware (`Bash(go test *)` no longer authorizes
  `go test ./...; rm -rf /`; command substitution never matches an allow
  rule). File path specifiers are cleaned and segment-aware (`*` no longer
  crosses `/`; `..` traversal can't escape an allowed prefix), and WebFetch
  patterns match the real URL host so lookalike domains don't pass. Matching
  dispatches per tool through `DefaultSpecifierMatchers`, overridable via
  `Config.SpecifierMatchers`.
- **Skill hardening** — `!{...}` shell expansion runs against the raw template
  before argument substitution, so model-controlled arguments can't inject
  commands (`$1`–`$9`/`$ARGUMENTS` are passed to the shell as data); the Skill
  tool can no longer invoke user-invocable-only commands hidden from the
  catalog; `allowed-tools` is documented as informational only.
- **Toolkit fail-closed** — the file tools now return their workspace
  validator construction error instead of silently granting unrestricted
  filesystem access; Fetch's SSRF protection validates the IP actually dialed
  after DNS resolution (closing the DNS-rebinding window); Glob/Grep default
  excludes now match top-level directories like `node_modules/`.

### Changed

- **A2A final-answer extraction** — the server emits a single final artifact
  built from the last renderable assistant message (previously one artifact
  per message), and `RemoteAgent` extracts the latest artifact, so
  `TaskResult.Text` is the final answer rather than a tool-use preamble.
  Intermediate messages still stream as `working` status updates.
- **FileStore session aliasing** — `FileStore` caches the live `*Session` per
  ID, so every `Open` of the same ID returns the same shared, synchronized
  instance (fixes double-Open divergence that could silently delete turns on
  disk).
- **`settings.local.json` deep-merges** with `settings.json` instead of
  shadowing it: objects merge per key (local wins), arrays replace wholesale,
  and scalar keys present in the local file win.
- **OpenAI `WithMaxRetries`** is now the single retry knob: it configures
  Dive's retry loop and SDK-internal retries are disabled, eliminating
  double-retry amplification (up to 9 requests per persistent error). Also
  applies to Grok.

### Fixed

- **Agent loop** — data race on the response-item accumulator under parallel
  tool execution; extension `PostBackgroundToolUse` hooks were silently
  dropped; a PostToolUse hook setting `Result = nil` no longer panics or
  orphans the tool_use block; injected background-results messages are now
  persisted to the session; hook `Messages`/`Iteration` refresh every loop
  iteration; SessionStart hook `Values` are visible to later hooks; the
  per-session lock is context-aware.
- **Suspend/resume** — `WithResume` on a session-backed agent no longer drops
  prior history; suspend-phase usage now accumulates into `TotalUsage()`;
  sessions deep-copy messages on ingestion so later caller mutations can't
  rewrite stored history; resume tool-result merges are deterministic and
  survive message-replacing PreGeneration hooks.
- **Session durability** — a torn final JSONL line no longer makes a FileStore
  session permanently unreadable (healed on open); removed the 1 MB read cap
  that broke sessions containing large events; fixed `Put`/`List` races; an
  unrecognized header line no longer triggers a destructive overwrite.
- **Providers** — openaicompletions streaming no longer reports zero token
  usage (Mistral, OpenRouter); OpenAI stream content-block indices are no
  longer off by one; Anthropic web-search error results decode instead of
  failing the whole response; the Google stream iterator now emits usage, stop
  reason, sequential indices, and parallel tool calls, with unique synthetic
  tool-call IDs and no stdout debug logging; 502/529 are retryable while
  permanent errors are no longer retried in openaicompletions; cached and
  reasoning token details are parsed across providers; Anthropic no longer
  mutates caller-owned messages; registry routing of `/`-containing model IDs
  reaches the OpenRouter matcher.
- **Toolkit** — Grep `offset` and `-n` are honored (working pagination and
  line numbers on both search paths); Bash scanner failures return an error
  instead of silently truncating output; ReadFile offset/limit reads handle
  long lines; TextEditor's unbounded, racy file history was removed.
- **MCP (experimental)** — `Client.Close` actually closes the underlying
  client (no more subprocess leaks); the HTTP transport sends configured auth
  headers; a second OAuth flow no longer panics and the configured token store
  is honored; `RefreshTools` cleans up server-side-removed tools.

## [1.7.0] - 2026-05-29

### Added

- **Text-to-speech and transcription** — new `media.TextToSpeech` and
  `media.Transcribe` functions backed by `TextToSpeechProvider` /
  `TranscriptionProvider` interfaces, an `AudioFormat` type
  (mp3/opus/aac/flac/wav/pcm), and options for voice, voice instructions,
  speech speed, audio format, language, and transcription prompt. Supported on
  OpenAI (`gpt-4o-mini-tts`, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`,
  `gpt-4o-transcribe-diarize`, `whisper-1`) and Google
  (`gemini-2.5-flash-preview-tts`, `gemini-2.5-pro-preview-tts`,
  `gemini-3.1-flash-tts-preview`), with new text-to-speech and transcription
  examples.
- **Latest OpenAI models** — added `gpt-5.5`, `gpt-5.4-mini`, `gpt-5.4-nano`,
  and `gpt-image-2`; OpenAI defaults now use `gpt-5.5` for text and
  `gpt-image-2` for image generation. Reasoning effort is now normalized for
  known OpenAI, Grok, and Anthropic model families without tightening the
  public `llm.ReasoningEffort` string type.

## [1.6.0] - 2026-05-29

### Added

- **Claude Opus 4.8 / 4.7** model constants and pricing; Anthropic and
  OpenRouter now default to Opus 4.8. Added a `FastModeTextPricing` table.
- **Native Anthropic effort** — `WithReasoningEffort` maps to
  `output_config.effort` on Opus 4.5+/Sonnet 4.6 (older models keep the legacy
  effort→budget mapping). New levels `ReasoningEffortMinimal` (matches OpenAI
  gpt-5), `ReasoningEffortXHigh`, and `ReasoningEffortMax`.
- **Adaptive thinking** — `WithAdaptiveThinking()`, `WithThinking(...)`, and
  `WithThinkingDisplay(...)`; on adaptive-only models (Opus 4.7/4.8) a manual
  `WithReasoningBudget` falls back to adaptive thinking. `dive.ModelSettings`
  gains `Thinking`, `ThinkingDisplay`, and `Speed`.
- **Fast mode** — `WithSpeed(llm.SpeedFast)` sets the `fast-mode-2026-02-01`
  beta header; `Usage.Speed` reports the speed used.
- **Grok server-side tools** — `CodeExecutionTool` (sandboxed Python) and
  `CollectionsSearchTool` (`file_search`); `WebSearchTool` gains
  `EnableImageSearch`. New `docs/guides/grok.md` and examples. Adds `grok-4.3`
  (new default) plus new Gemini/Grok models and pricing.
- **Reasoning token usage** — `llm.Usage.ReasoningTokens` reports reasoning
  output tokens (OpenAI Responses, Grok); the streaming path now fills
  `CacheReadInputTokens`. `ResponsesIncludeProvider` lets a server-side tool
  request response `include` data.
- **Structured tool progress** — tools emit typed snapshots via
  `dive.ReportProgress(ctx, *dive.ToolProgress)`, delivered as
  `ResponseItemTypeToolProgress` events (latest-wins, distinct from the
  text-delta `StreamOutput`). Bash now reports line/byte/elapsed progress.
- **`SessionStartHook`** — fires once at the start of a fresh conversation to
  seed it from external state, returning a `*SessionStartResult` (durable or
  ephemeral via `Persist`).
- **Model-judgment hook helpers** — `PromptStopHook` and `PromptToolGate` let a
  model make a hook decision via a forced structured verdict.
- **Refusal `stop_details`** on `llm.Response`.

### Changed

- **Subagents promoted to stable** — the subagent layer moves out of
  experimental to core `subagent/` and `toolkit/orchestration/`, aligned with
  Claude Code's tool model. Adds built-in read-only `Explore`/`Plan` personas;
  `DisallowedTools` is now enforced in `FilterTools` and parseable from the
  `disallowed-tools` key in `.dive/agents/*.md` frontmatter. The
  subagent-spawner tool is `Agent` (renamed from the experimental `Task`), with
  the `Task*` prefix reserved for background control (`TaskStop`/`Monitor`).
- **Non-destructive compaction** — `Compact()` inserts a checkpoint instead of
  collapsing the log. `Messages()` returns the active window (latest summary +
  everything after); new `AllMessages()` returns the full transcript, and
  `CompactionHistory()` returns one `CompactionRecord` per checkpoint.

### Fixed

- Effort/thinking requests no longer 400 on Opus 4.7/4.8 (which reject manual
  thinking budgets); `ReasoningEffort` with `Thinking: disabled` on a legacy
  model now errors instead of silently re-enabling thinking.
- Corrected Grok 4.20 pricing ($1.25/$2.50 per 1M tokens) and the X-search
  handle limit (now 20, was capped at 10).
- `file_search` / collections-search responses now decode instead of returning
  an "unsupported" error.

## [1.5.0] - 2026-05-15

### Added

- **`Extension` interface** for composable agent capabilities. Extensions bundle
  tools, hooks, and system prompt rules and are merged during `NewAgent` via
  `AgentOptions.Extensions`.
- **Agent suspend/resume** for out-of-process tool results. A tool returns
  `NewSuspendResult` to pause the agent; the response comes back with
  `Status == ResponseStatusSuspended` and a `Suspension` state for later
  resumption via `WithToolResults` or `WithResume`. `SuspendableSession` adds
  auto-persistence; the `OnSuspend` hook fires before persistence.
- **`Tracer` interface** for agent observability (tracing, metrics, audit
  logging) with `StartAgentRun` / `StartChat` / `StartToolCall`. `NopTracer`
  and `MultiTracer` ship in core; the OpenTelemetry adapter lives in the
  promoted `dive/otel` module.
- **A2A (Agent-to-Agent) support** as a stable submodule (`a2a/`), built on the
  official `a2a-go/v2` SDK. `Server` exposes a Dive agent as JSON-RPC or REST;
  `RemoteAgent` calls remote A2A endpoints. Suspend/resume maps to the A2A
  `input-required` state. Static and dynamic agent cards supported.
- **Background tool execution** — tools can opt into running in the background
  while the agent continues, with results returned later.
- **Skill system** as a stable package (`skill/`) — unified skills and slash
  commands implementing the `Extension` interface, with provider-based loading
  (filesystem, `.agents/skills/`), variable expansion, and trigger matching.
  Supports agentskills.io standard frontmatter fields in `SkillConfig`.
- **Media generation tools** for images and videos with path traversal
  protection, duration schema, and aspect ratio controls.
- **CLI enhancements**: `models` command, interactive model switcher, status
  line in the input area, hanging indent for assistant messages, and broad UI
  polish.

### Changed

- **Subagent reliability** improvements with auto-retrieval of nested agent
  results.
- **Ollama provider** switched to the Anthropic Messages API; adds
  `provider/model` syntax for unambiguous routing.
- **Skip retrying permanent errors** in provider retry loops (auth failures,
  4xx client errors).
- **Promoted out of experimental**: `dive/otel`, `a2a` (renamed from `a2alib`),
  and `toolkit/firecrawl` are now stable submodules.
- **Upgraded dependencies**: OpenTelemetry 1.40→1.41, wonton 0.0.29→0.0.34.

## [1.4.0] - 2026-03-25

### Added

- **Grok provider** as a standalone Go submodule (`providers/grok/`). Built on the
  OpenAI Responses API with support for Grok 4.20 models (reasoning, non-reasoning,
  multi-agent).
- **Server-side tools for Grok**: `WebSearchTool` (web search with domain filters and
  image understanding) and `XSearchTool` (X/Twitter search with handle filters, date
  ranges, and media understanding).
- **Prompt caching for Grok** via `WithPromptCacheKey(key)` option for cache reuse
  across requests.
- **OpenAI provider extensions**: `ResponsesToolProvider` interface for custom tool
  types and `WithExtraRequestOptions` for per-request SDK options.

### Changed

- **Upgraded dependencies**: grpc v1.79.3, jsonparser v1.1.2 (DoS fix),
  openai-go v3.29.0, genai v1.51.0.

## [1.3.0] - 2026-03-12

### Changed

- **Stream parallel tool results as they complete.** `ToolCallResult` events and
  `PostToolUse` hooks now fire as soon as each tool finishes, rather than waiting
  for all parallel tools to complete. Callbacks remain single-threaded via a channel
  consumer. Result events now arrive in completion order, not declaration order.

## [1.2.0] - 2026-03-11

### Added

- **Parallel tool execution** via `AgentOptions.ParallelToolExecution` (defaults to `false`).
  When enabled and the LLM returns multiple tool calls in one message, they execute
  concurrently via goroutines. Hooks and event callbacks remain serialized for safety.
  Three-phase design: pre-hooks run sequentially, tools execute in parallel, post-hooks
  run sequentially.

## [1.1.0] - 2026-03-10

### Changed

- **Upgrade OpenAI Go SDK from v1 to v3** (`openai-go` v1.12.0 → v3.26.0). All SDK
  migration handled internally in `providers/openai`; Dive's public API is unchanged.
  Streaming reasoning deduplicated and per-summary-part tracking added.
- **Update provider models and features for March 2026.** Anthropic: claude-sonnet-4-6,
  new beta features. OpenAI: gpt-5.4 (new default), gpt-5.3, gpt-5.1-mini, o3-mini.
  Google: gemini-3.1-pro/flash variants. Grok: removed deprecated grok-2 models.
- **Upgrade all dependencies to latest versions.** Key bumps: mcp-go v0.43→v0.45,
  golang.org/x/net v0.50→v0.51, googleapis/gax-go v2.17→v2.18, opentelemetry
  v1.40→v1.42, grpc v1.78→v1.79, genai SDK v1.46→v1.49.

## [1.0.1] - 2025-02-09

### Changed

- **Decoupled root module** from provider and experimental sub-modules. The root
  `go.mod` no longer depends on `providers/google`, `providers/openai`, or
  `experimental/mcp`, significantly reducing transitive dependencies for consumers
  who only need the core library.
- **Added `examples/` module** with its own `go.mod` to hold example code separately
  from the core module.
- **Added `tag-modules` Makefile target** for tagging all sub-modules in one step
  (`make tag-modules VERSION=v1.0.0`).
- **Added `examples` to `tidy-all`** module list in Makefile.

## [1.0.0] - 2025-02-09

First official stable release. Major architectural overhaul from v0.0.x with a simpler
agent API, a new hook system, and clearly separated core vs experimental packages.

### Added

- **Hook system** with 7 hook types (`PreGeneration`, `PostGeneration`,
  `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PreIteration`, `Stop`)
  and shared `*HookContext`. Built-in hooks for context injection, compaction,
  and usage logging.
- **Session package** (`session/`) with `FileStore` and `MemoryStore` backends,
  plus fork and compact operations.
- **Permission package** (`permission/`) promoted to core. Rule-based tool
  permissions with modes, specifier patterns, and session allowlists.
- **`FuncTool[T]`** for creating tools from functions with auto-generated schemas.
- **`Toolset` interface** for dynamic per-request tool resolution.
- **Provider registry** with self-registering providers via `init()`.
- **Gemini 3 models** (`gemini-3-pro-preview`, `gemini-3-flash-preview`).
- **Tool panic recovery**, `OutputMessages` on Response, `llms.txt`.

### Changed

- **Agent is a concrete struct**, not an interface. `SystemPrompt` replaces
  `Instructions`. `AgentOptions` streamlined with `Hooks`, `Toolsets`, `Session`.
- **Toolkit constructors** return `*dive.TypedToolAdapter[T]` with variadic options.
- **CLI moved** to `experimental/cmd/dive/`.
- **Provider defaults updated**: Anthropic `claude-opus-4-6`, OpenAI `gpt-5.2`,
  Google `gemini-2.5-pro`. Pricing updated across all providers.
- **Experimental boundary**: MCP, sandbox, skills, slash commands, subagents,
  compaction, todo, settings, and extended toolkit moved under `experimental/`.

### Removed

- **Groq provider**.
- **Thread system** replaced by `dive.Session` interface.
- **Interactor and Confirmer** replaced by hooks and the permission package.
- **Subagent loader and compaction** from core (available in `experimental/`).
