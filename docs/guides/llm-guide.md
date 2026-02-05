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

### Groq

```go
import "github.com/deepnoodle-ai/dive/providers/groq"

model := groq.New()
```

**Env:** `GROQ_API_KEY`
**Models:** See `providers/groq/models.go` for available models.
**Features:** High-speed inference, streaming

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
| `ReasoningBudget`   | `*int`            | Tokens for reasoning (o-series, Claude) |
| `ReasoningEffort`   | `string`          | low, medium, high                       |
| `Caching`           | `*bool`           | Enable prompt caching (Claude)          |
| `ParallelToolCalls` | `*bool`           | Allow simultaneous tool calls           |
| `ToolChoice`        | `*llm.ToolChoice` | auto, any, none, or specific tool       |

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
