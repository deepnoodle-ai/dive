# LLM Guide

Dive supports multiple LLM providers through a unified interface. Each provider is in `providers/<name>` and auto-registers via `init()`.

## Supported Providers

### Anthropic (Claude)

```go
import "github.com/deepnoodle-ai/dive/providers/anthropic"

model := anthropic.New() // defaults to claude-opus-4-8
```

**Env:** `ANTHROPIC_API_KEY`
**Models:** See `providers/anthropic/models.go` for available models.
**Features:** Streaming, tool calling, prompt caching, reasoning control

### OpenAI

```go
import "github.com/deepnoodle-ai/dive/providers/openai"

model := openai.New() // defaults to gpt-5.6-sol
```

**Env:** `OPENAI_API_KEY`
**Models:** See `providers/openai/models.go` for available models.
**Features:** Streaming, tool calling, vision input, reasoning effort

### Google (Gemini)

```go
import "github.com/deepnoodle-ai/dive/providers/google"

model := google.New() // defaults to gemini-2.5-pro
```

**Env:** `GEMINI_API_KEY` or `GOOGLE_API_KEY`
**Models:** See `providers/google/models.go` for available models.
**Features:** Streaming, tool calling, multimodal

### Grok (X.AI)

```go
import "github.com/deepnoodle-ai/dive/providers/grok"

model := grok.New()
```

**Env:** `GROK_API_KEY`
**Models:** See `providers/grok/models.go` for available models.

### Mistral

```go
import "github.com/deepnoodle-ai/dive/providers/mistral"

model := mistral.New()
```

**Env:** `MISTRAL_API_KEY`
**Models:** See `providers/mistral/models.go` for available models.

### Ollama (Local)

```go
import "github.com/deepnoodle-ai/dive/providers/ollama"

model := ollama.New()
```

No API key needed. Requires Ollama running locally.
Use any model available in your local Ollama installation.

### OpenRouter

```go
import "github.com/deepnoodle-ai/dive/providers/openrouter"

model := openrouter.New()
```

**Env:** `OPENROUTER_API_KEY`
**Features:** Access to 200+ models from multiple providers

## Multimodal Input

Messages can carry images and documents alongside text using
`llm.ImageContent` and `llm.DocumentContent` blocks:

```go
message := llm.NewUserMessage(
    &llm.TextContent{Text: "Summarize this report."},
    &llm.DocumentContent{
        Title: "report.pdf",
        Source: &llm.ContentSource{
            Type:      llm.ContentSourceTypeBase64,
            MediaType: "application/pdf",
            Data:      pdfBase64,
        },
    },
)
```

Each provider encodes these blocks into its native request format. Supported
content sources by provider:

| Provider                                 | Images                | Documents                              |
| ---------------------------------------- | --------------------- | -------------------------------------- |
| anthropic                                | base64, URL, file ID  | base64, URL, file ID, text             |
| openai (Responses)                       | base64, URL, file ID  | base64, URL, file ID, text             |
| grok                                     | base64, URL, file ID  | same as openai (server support varies) |
| google                                   | base64, URL/file URI  | base64, URL/file URI, text             |
| openaicompletions, mistral, openrouter   | base64, URL           | base64, file ID, text (no URL)         |
| ollama                                   | base64 (model-dependent) | model-dependent                     |

Notes:

- Base64 sources require `MediaType`. Providers convert to the wire format
  they need (e.g. data URLs for OpenAI-style APIs, inline bytes for Gemini).
- Text-source documents are sent natively on Anthropic and inlined as plain
  text on providers without a text-document type.
- A content block a provider cannot encode is a request-building error, never
  a silent drop.

Tool results can also carry media (e.g. an MCP tool returning a screenshot).
Anthropic and OpenAI (Responses) receive tool-result images natively; on
providers whose tool messages are text-only (google, openaicompletions,
mistral, openrouter), non-text blocks are replaced with a
`[image content omitted]` placeholder so the model knows content was elided.
A tool result with nothing to render is sent as `(no output)` rather than an
empty block or empty array, which are variously rejected or ambiguous.

## Provider Options

All providers accept variadic options. For example, to specify a model:

```go
provider := anthropic.New(anthropic.WithModel("claude-sonnet-4-5"))
```

## Model Settings

Configure LLM behavior per agent via `ModelSettings`:

```go
agent, _ := dive.NewAgent(dive.AgentOptions{
    SystemPrompt: "You are a creative writer.",
    Model:        anthropic.New(),
    ModelSettings: &dive.ModelSettings{
        Temperature:       dive.Ptr(0.7),
        MaxTokens:         dive.Ptr(2000),
        ReasoningBudget:   dive.Ptr(5000),
        ReasoningEffort:   "high",
        Caching:           dive.Ptr(true),
        ParallelToolCalls: dive.Ptr(true),
    },
})
```

### Settings Reference

| Setting             | Type                  | Description                                      |
| ------------------- | --------------------- | ------------------------------------------------ |
| `Temperature`       | `*float64`            | Creativity vs consistency (0.0-1.0)              |
| `MaxTokens`         | `*int`                | Maximum response length                          |
| `PresencePenalty`   | `*float64`            | Reduce repetition                                |
| `FrequencyPenalty`  | `*float64`            | Encourage topic variety                          |
| `ReasoningBudget`   | `*int`                | Manual thinking budget (o-series, older Claude)  |
| `ReasoningEffort`   | `llm.ReasoningEffort` | none, minimal, low, medium, high, xhigh, max     |
| `Thinking`          | `llm.ThinkingType`    | adaptive, enabled, or disabled extended thinking |
| `ThinkingDisplay`   | `llm.ThinkingDisplay` | summarized or omitted thinking content           |
| `Speed`             | `llm.Speed`           | fast or standard (Claude fast mode)              |
| `Caching`           | `*bool`               | Enable prompt caching (Claude)                   |
| `ParallelToolCalls` | `*bool`               | Allow simultaneous tool calls                    |
| `ToolChoice`        | `*llm.ToolChoice`     | auto, any, none, or specific tool                |

Provider implementations normalize `ReasoningEffort` only where the model
family is known. Unsupported providers may omit the option or pass it through
for compatibility with custom OpenAI-compatible endpoints.

### Reasoning And Summarized Thinking On Claude

Newer Claude models prefer **adaptive thinking** — the model decides when and how
much to think — with `effort` guiding depth. Opus 4.7, Opus 4.8, Sonnet 5, Fable
5, and Mythos 5 reject manual `budget_tokens`; Dive keeps older callers working
by mapping `ReasoningBudget` to adaptive thinking on those models.

```go
ModelSettings: &dive.ModelSettings{
    ReasoningEffort: llm.ReasoningEffortXHigh,   // -> output_config.effort
    Thinking:        llm.ThinkingTypeAdaptive,   // -> thinking: {type: adaptive}
    ThinkingDisplay: llm.ThinkingDisplaySummarized,
}
```

`ThinkingDisplaySummarized` requests visible summarized thinking blocks. This is
important on Sonnet 5, Fable 5, Mythos 5, Opus 4.7, and Opus 4.8, where Claude
defaults to `omitted` display and returns an empty `thinking` field plus an
encrypted signature. Dive preserves both normal `thinking` blocks and
`redacted_thinking` blocks in responses; when continuing a tool-use turn, pass
the assistant message content back unchanged so Anthropic can verify the
signatures.

In the Dive CLI, pass `--show-thinking` (or set `DIVE_SHOW_THINKING=true`) to
request adaptive summarized thinking and render visible thinking summaries in
the interactive transcript. Add `--thinking-effort high` (or set
`DIVE_THINKING_EFFORT=high`) to choose the thinking effort level. Supported
provider-neutral values are `none`, `minimal`, `low`, `medium`, `high`, `xhigh`,
and `max`; provider-specific values pass through to compatible backends.

When thinking is active, Anthropic only allows `tool_choice` `auto` or `none`.
Dive returns a request-building error for forced tool choices (`any` or a
specific tool), prefilled assistant responses, and manual thinking budgets that
are not less than `MaxTokens` unless interleaved thinking is enabled.
Temperature is ignored on thinking requests because Anthropic rejects sampling
changes with extended thinking.

`Usage.ReasoningTokens` includes Anthropic's reported
`output_tokens_details.thinking_tokens` value when it is present. Fast mode
(`Speed: llm.SpeedFast`) requires fast-mode access on your account and applies
the `fast-mode-2026-02-01` beta header automatically.

## Provider Registry

The `providers` package provides a registry for creating models by name:

```go
import "github.com/deepnoodle-ai/dive/providers"

model := providers.CreateModel("claude-sonnet-4-5", "")
```

This is useful for CLI tools or configuration-driven model selection.

## Best Practices

1. **Use local models for development** - Ollama avoids API costs during dev
2. **Enable caching** with Claude for repeated prompts
3. **Set reasonable token limits** to control costs
4. **Choose the right model** for your use case: fast (Haiku, Flash) vs capable (Opus, GPT-5)
