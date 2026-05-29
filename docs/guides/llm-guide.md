# LLM Guide

Dive supports multiple LLM providers through a unified interface. Each provider is in `providers/<name>` and auto-registers via `init()`.

## Supported Providers

### Anthropic (Claude)

```go
import "github.com/deepnoodle-ai/dive/providers/anthropic"

model := anthropic.New() // defaults to claude-opus-4-5
```

**Env:** `ANTHROPIC_API_KEY`
**Models:** See `providers/anthropic/models.go` for available models.
**Features:** Streaming, tool calling, prompt caching, reasoning control

### OpenAI

```go
import "github.com/deepnoodle-ai/dive/providers/openai"

model := openai.New() // defaults to gpt-5.2
```

**Env:** `OPENAI_API_KEY`
**Models:** See `providers/openai/models.go` for available models.
**Features:** Streaming, tool calling, reasoning budget (o-series)

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

| Setting             | Type              | Description                             |
| ------------------- | ----------------- | --------------------------------------- |
| `Temperature`       | `*float64`        | Creativity vs consistency (0.0-1.0)     |
| `MaxTokens`         | `*int`            | Maximum response length                 |
| `PresencePenalty`   | `*float64`        | Reduce repetition                       |
| `FrequencyPenalty`  | `*float64`        | Encourage topic variety                 |
| `ReasoningBudget`   | `*int`            | Manual thinking budget (o-series, older Claude) |
| `ReasoningEffort`   | `string`          | low, medium, high, xhigh, max (Claude Opus 4.6+) |
| `Thinking`          | `llm.ThinkingType` | adaptive, enabled, or disabled extended thinking |
| `ThinkingDisplay`   | `llm.ThinkingDisplay` | summarized or omitted thinking content      |
| `Speed`             | `llm.Speed`       | fast or standard (Claude fast mode)     |
| `Caching`           | `*bool`           | Enable prompt caching (Claude)          |
| `ParallelToolCalls` | `*bool`           | Allow simultaneous tool calls           |
| `ToolChoice`        | `*llm.ToolChoice` | auto, any, none, or specific tool       |

### Reasoning on Claude Opus 4.7 / 4.8

Opus 4.7 and 4.8 support **only adaptive thinking** — the model decides when and
how much to think — and reject manual `budget_tokens`. Use the `effort`
parameter to guide depth:

```go
ModelSettings: &dive.ModelSettings{
    ReasoningEffort: llm.ReasoningEffortXHigh,   // -> output_config.effort
    Thinking:        llm.ThinkingTypeAdaptive,   // -> thinking: {type: adaptive}
}
```

`WithReasoningBudget` still works against these models for backward
compatibility — it transparently falls back to adaptive thinking. Fast mode
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
