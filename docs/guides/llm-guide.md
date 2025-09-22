# LLM Guide

Complete guide to using Large Language Models with Dive.

## Supported Providers

### Anthropic (Claude)
```go
import "github.com/deepnoodle-ai/dive/llm/providers/anthropic"

model := anthropic.New()
```

**Models:** claude-sonnet-4-20250514, claude-opus-4-20250514, claude-3-5-sonnet-20241022, claude-3-5-haiku-20241022, claude-3-7-sonnet-20250219
**Features:** Tool calling, streaming, prompt caching, reasoning control

### OpenAI
```go
import "github.com/deepnoodle-ai/dive/llm/providers/openai"

model := openai.New()
```

**Models:** gpt-5-2025-08-07 (default), gpt-4o, gpt-4o-mini, o1, o3-mini, o3
**Features:** Tool calling, streaming, reasoning budget (o1/o3)

### Groq
```go
import "github.com/deepnoodle-ai/dive/llm/providers/groq"

model := groq.New()
```

**Models:** llama-3.3-70b-versatile, deepseek-r1-distill-llama-70b
**Features:** High-speed inference, streaming

### Grok (X.AI)
```go
import "github.com/deepnoodle-ai/dive/llm/providers/grok"

model := grok.New()
```

**Models:** grok-2, grok-2-mini, grok-3
**Features:** Real-time X (Twitter) integration, reasoning

### OpenRouter
```go
import "github.com/deepnoodle-ai/dive/llm/providers/openrouter"

model := openrouter.New()
```

**Models:** Access to 200+ models from multiple providers
**Features:** Unified access to diverse models, cost optimization

### Google (Gemini)
```go
import "github.com/deepnoodle-ai/dive/llm/providers/google"

model := google.New()
```

**Models:** gemini-2.0-flash-exp, gemini-1.5-pro, gemini-1.5-flash
**Features:** Multimodal capabilities, large context windows

### Ollama (Local)
```go
import "github.com/deepnoodle-ai/dive/llm/providers/ollama"

model := ollama.New()
```

**Models:** Any locally installed model
**Features:** Local inference, privacy, custom models

## Quick Setup

### Environment Variables
```bash
export ANTHROPIC_API_KEY="your-key"
export OPENAI_API_KEY="your-key"
export GROQ_API_KEY="your-key"
export GROK_API_KEY="your-key"
export OPENROUTER_API_KEY="your-key"
export GEMINI_API_KEY="your-key"  # For Google
# Ollama runs locally - no key needed
```

### Basic Usage
```go
agent, err := agent.New(agent.Options{
    Name:         "Assistant",
    Instructions: "You are a helpful assistant.",
    Model:        anthropic.New(),
})
```

## Model Configuration

### Agent-Level Settings
```go
agent, err := agent.New(agent.Options{
    Name:  "Assistant",
    Model: anthropic.New(),
    ModelSettings: &agent.ModelSettings{
        Temperature:     0.7,
        MaxTokens:      2000,
        ReasoningBudget: 5000,  // For o1 models
        Caching:        true,   // For Claude
    },
})
```

### YAML Configuration
```yaml
Agents:
  - Name: Creative Writer
    Provider: anthropic
    Model: claude-sonnet-4-20250514
    ModelSettings:
      Temperature: 0.9
      MaxTokens: 4000
      Caching: true

  - Name: Reasoner
    Provider: openai
    Model: o1
    ModelSettings:
      ReasoningBudget: 10000
      ReasoningEffort: high
```

## Model Settings Reference

### Core Settings
- **Temperature** (0.0-1.0): Creativity vs consistency
- **MaxTokens**: Maximum response length
- **PresencePenalty**: Reduce repetition
- **FrequencyPenalty**: Encourage topic variety

### Provider-Specific
- **ReasoningBudget** (OpenAI o1/o3): Tokens for reasoning
- **ReasoningEffort** (OpenAI o1/o3): low, medium, high
- **Caching** (Anthropic): Cache prompts for speed

### Tool Settings
- **ParallelToolCalls**: Allow simultaneous tool use
- **ToolChoice**: auto, required, none, or specific tool

## Model Selection Tips

### Choose by Use Case

**Creative Tasks:**
- Claude Sonnet (high creativity)
- Temperature: 0.7-0.9

**Factual/Analysis:**
- GPT-4o (reliability)
- Temperature: 0.1-0.3

**Complex Reasoning:**
- OpenAI o1/o3 (reasoning)
- High reasoning budget

**Fast Response:**
- Claude Haiku (speed)
- Groq models (fastest)

**Local/Private:**
- Ollama (privacy)
- Any local model

### Configuration Examples

**Creative Writer:**
```yaml
ModelSettings:
  Temperature: 0.9
  MaxTokens: 4000
  PresencePenalty: 0.1
```

**Code Analyst:**
```yaml
ModelSettings:
  Temperature: 0.2
  MaxTokens: 2000
  ReasoningBudget: 8000  # If using o1
```

**Fast Chat:**
```yaml
Provider: groq
Model: llama-3.3-70b
ModelSettings:
  Temperature: 0.5
  MaxTokens: 1000
```

## Performance Tips

1. **Use caching** (Claude) for repeated prompts
2. **Set reasonable token limits** to control costs
3. **Choose appropriate models** for your use case
4. **Monitor usage** with built-in token tracking
5. **Use local models** (Ollama) for development

## Error Handling

All providers have automatic retry logic and consistent error handling:

```go
response, err := agent.CreateResponse(ctx, dive.WithInput("Hello"))
if err != nil {
    // Handle rate limits, API errors, etc.
    log.Printf("Error: %v", err)
}
```

Common errors are handled automatically:
- Rate limits (automatic retry with backoff)
- Network timeouts (configurable retry)
- API errors (clear error messages)

## Best Practices

1. **Test with multiple providers** - Each has strengths
2. **Use appropriate temperatures** - Low for facts, high for creativity
3. **Set reasonable token limits** - Balance quality and cost
4. **Enable caching** when using repeated prompts
5. **Monitor costs** - Track token usage across providers
6. **Use local models** for development and privacy-sensitive tasks