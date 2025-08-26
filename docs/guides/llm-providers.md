# LLM Providers Guide

Dive provides a unified interface for working with multiple Large Language Model providers, allowing you to switch between different models and providers without changing your application code.

## 📋 Table of Contents

- [Overview](#overview)
- [Supported Providers](#supported-providers)
- [Provider Configuration](#provider-configuration)
- [Model Selection](#model-selection)
- [Advanced Features](#advanced-features)
- [Provider-Specific Features](#provider-specific-features)
- [Best Practices](#best-practices)

## Overview

The LLM abstraction layer in Dive provides:

- **Unified Interface** - Same API across all providers
- **Model Flexibility** - Easy switching between models
- **Feature Parity** - Tool calling, streaming, and advanced features
- **Automatic Retries** - Built-in error handling and retry logic
- **Usage Tracking** - Token counting and cost monitoring
- **Provider Detection** - Automatic provider selection based on environment

## Supported Providers

### Anthropic (Claude)

Full-featured provider with excellent tool calling support.

```go
import "github.com/diveagents/dive/llm/providers/anthropic"

// Basic setup
model := anthropic.New()

// With specific model
model := anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))

// With advanced options
model := anthropic.New(
    anthropic.WithModel("claude-opus-4-20250514"),
    anthropic.WithAPIKey("your-api-key"),
    anthropic.WithBaseURL("https://api.anthropic.com"),
)
```

**Supported Models:**
- `claude-sonnet-4-20250514` (recommended)
- `claude-opus-4-20250514`
- `claude-3-7-sonnet-20250219`
- `claude-3-5-sonnet-20241022`
- `claude-3-5-haiku-20241022`

### OpenAI (GPT)

Comprehensive provider supporting GPT models and reasoning models.

```go
import "github.com/diveagents/dive/llm/providers/openai"

// Basic setup
model := openai.New()

// With specific model
model := openai.New(openai.WithModel("gpt-4o"))

// With advanced options
model := openai.New(
    openai.WithModel("o1"),
    openai.WithAPIKey("your-api-key"),
    openai.WithOrganization("your-org-id"),
)
```

**Supported Models:**
- `gpt-4o` (recommended for general use)
- `gpt-4.5-preview`
- `o1` (reasoning model)
- `o1-mini`
- `o3-mini` (latest reasoning model)

### Groq

High-performance inference provider with fast response times.

```go
import "github.com/diveagents/dive/llm/providers/groq"

// Basic setup
model := groq.New()

// With specific model
model := groq.New(groq.WithModel("deepseek-r1-distill-llama-70b"))

// With options
model := groq.New(
    groq.WithModel("llama-3.3-70b-versatile"),
    groq.WithAPIKey("your-api-key"),
)
```

**Supported Models:**
- `deepseek-r1-distill-llama-70b` (recommended)
- `llama-3.3-70b-versatile`
- `qwen-2.5-32b`

### Ollama (Local Models)

Run models locally with Ollama.

```go
import "github.com/diveagents/dive/llm/providers/ollama"

// Basic setup (connects to local Ollama)
model := ollama.New()

// With specific model and host
model := ollama.New(
    ollama.WithModel("llama3.2:latest"),
    ollama.WithHost("http://localhost:11434"),
)
```

**Supported Models:**
- `llama3.2:*` (with tool support)
- `mistral:*` (basic support)
- Any model available in your Ollama installation

## Provider Configuration

### Environment Variables

Set up API keys and configuration through environment variables:

```bash
# Anthropic
export ANTHROPIC_API_KEY="your-anthropic-key"

# OpenAI  
export OPENAI_API_KEY="your-openai-key"
export OPENAI_ORG_ID="your-org-id"  # Optional

# Groq
export GROQ_API_KEY="your-groq-key"

# Ollama (if not running locally)
export OLLAMA_HOST="http://your-ollama-server:11434"
```

### Programmatic Configuration

```go
// Anthropic with custom configuration
anthropicModel := anthropic.New(
    anthropic.WithAPIKey("key-from-config"),
    anthropic.WithModel("claude-sonnet-4-20250514"),
    anthropic.WithBaseURL("https://custom-api.example.com"),
    anthropic.WithMaxRetries(5),
    anthropic.WithTimeout(time.Minute * 2),
)

// OpenAI with organization and project
openaiModel := openai.New(
    openai.WithAPIKey("key-from-config"),
    openai.WithOrganization("org-123"),
    openai.WithProject("proj-456"),
    openai.WithModel("gpt-4o"),
)

// Groq with performance tuning
groqModel := groq.New(
    groq.WithAPIKey("key-from-config"),
    groq.WithModel("llama-3.3-70b-versatile"),
    groq.WithTimeout(time.Second * 30), // Fast responses
)
```

### Provider Detection

Dive can automatically detect available providers:

```go
import "github.com/diveagents/dive/agent"

// Auto-detect provider based on environment variables
agent, err := agent.New(agent.Options{
    Name:         "Assistant",
    Instructions: "You are a helpful assistant",
    // Model will be auto-detected from available providers
})

// Manual provider selection with fallback
var model llm.LLM
if anthropicKey := os.Getenv("ANTHROPIC_API_KEY"); anthropicKey != "" {
    model = anthropic.New()
} else if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
    model = openai.New()
} else {
    model = ollama.New() // Local fallback
}
```

## Model Selection

### Choosing the Right Model

**For General Use:**
```go
// Balanced performance and cost
model := anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))
// or
model := openai.New(openai.WithModel("gpt-4o"))
```

**For Complex Reasoning:**
```go
// Best reasoning capabilities
model := openai.New(openai.WithModel("o1"))
// or
model := anthropic.New(anthropic.WithModel("claude-opus-4-20250514"))
```

**For Fast Responses:**
```go
// Optimized for speed
model := groq.New(groq.WithModel("llama-3.3-70b-versatile"))
// or
model := anthropic.New(anthropic.WithModel("claude-3-5-haiku-20241022"))
```

**For Cost-Effective Solutions:**
```go
// Local inference (no API costs)
model := ollama.New(ollama.WithModel("llama3.2:latest"))
```

### Model Capabilities Matrix

| Provider | Model | Tool Calling | Streaming | Reasoning | Speed | Cost |
|----------|-------|--------------|-----------|-----------|-------|------|
| Anthropic | claude-sonnet-4 | ✅ | ✅ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| Anthropic | claude-opus-4 | ✅ | ✅ | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐ |
| Anthropic | claude-3-5-haiku | ✅ | ✅ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| OpenAI | gpt-4o | ✅ | ✅ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| OpenAI | o1 | ✅ | ❌ | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐ |
| OpenAI | o3-mini | ✅ | ✅ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| Groq | deepseek-r1 | ✅ | ✅ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Groq | llama-3.3-70b | ✅ | ✅ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Ollama | llama3.2 | ✅ | ✅ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

## Advanced Features

### Streaming Responses

```go
// Enable streaming for real-time responses
agent, err := agent.New(agent.Options{
    Name:  "Streaming Assistant",
    Model: anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514")),
})

stream, err := agent.StreamResponse(
    context.Background(),
    dive.WithInput("Write a short story about AI"),
)

for event := range stream.Events() {
    switch event.Type {
    case dive.EventTypeLLMEvent:
        if event.Item.Event.Type == "content_block_delta" {
            fmt.Print(event.Item.Event.Delta.Text)
        }
    }
}
```

### Tool Calling

```go
import "github.com/diveagents/dive/toolkit"

// Agent with tools
agent, err := agent.New(agent.Options{
    Name: "Tool-Using Assistant",
    Model: anthropic.New(), // Supports tool calling
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
            Provider: "google",
        })),
        dive.ToolAdapter(toolkit.NewCalculatorTool()),
    },
})

response, err := agent.CreateResponse(
    context.Background(),
    dive.WithInput("Search for recent AI news and calculate 15% of 1000"),
)
```

### Advanced Model Settings

```go
agent, err := agent.New(agent.Options{
    Name: "Fine-tuned Assistant",
    Model: anthropic.New(),
    ModelSettings: &agent.ModelSettings{
        Temperature:       &[]float64{0.7}[0],     // Creativity level
        MaxTokens:         &[]int{2048}[0],         // Response length limit
        ReasoningBudget:   &[]int{50000}[0],        // Reasoning token budget
        ReasoningEffort:   "high",                  // Reasoning intensity
        ParallelToolCalls: &[]bool{true}[0],        // Concurrent tool execution
        Caching:           &[]bool{true}[0],        // Enable prompt caching
        
        // Provider-specific features
        Features: []string{"computer_use"},         // Enable computer use (Anthropic)
        
        // Custom headers
        RequestHeaders: http.Header{
            "X-Custom-Header": []string{"custom-value"},
        },
    },
})
```

### Usage Tracking

```go
response, err := agent.CreateResponse(ctx, dive.WithInput("Hello"))
if err != nil {
    panic(err)
}

// Access usage information
usage := response.Usage
fmt.Printf("Input tokens: %d\n", usage.InputTokens)
fmt.Printf("Output tokens: %d\n", usage.OutputTokens)
fmt.Printf("Cache creation tokens: %d\n", usage.CacheCreationInputTokens)
fmt.Printf("Cache read tokens: %d\n", usage.CacheReadInputTokens)
fmt.Printf("Total tokens: %d\n", usage.TotalTokens())
```

## Provider-Specific Features

### Anthropic Features

```go
model := anthropic.New(
    anthropic.WithModel("claude-sonnet-4-20250514"),
    anthropic.WithBetaFeatures([]string{"computer-use-2024-10-22"}),
    anthropic.WithMaxTokens(4096),
)

// Computer use capability
agent, err := agent.New(agent.Options{
    Model: model,
    ModelSettings: &agent.ModelSettings{
        Features: []string{"computer_use"},
    },
})
```

### OpenAI Features

```go
model := openai.New(
    openai.WithModel("gpt-4o"),
    openai.WithResponseFormat(&llm.ResponseFormat{
        Type: "json_object", // Force JSON responses
    }),
)

// Reasoning models (o1, o3)
reasoningModel := openai.New(
    openai.WithModel("o1"),
    // Note: o1 models have different parameter support
)

agent, err := agent.New(agent.Options{
    Model: reasoningModel,
    ModelSettings: &agent.ModelSettings{
        ReasoningBudget: &[]int{100000}[0], // Higher budget for complex reasoning
        ReasoningEffort: "high",
    },
})
```

### Groq Optimizations

```go
// Optimized for high-throughput scenarios
model := groq.New(
    groq.WithModel("llama-3.3-70b-versatile"),
    groq.WithTimeout(time.Second * 15), // Fast timeout
)

// Batch processing setup
agent, err := agent.New(agent.Options{
    Model: model,
    ModelSettings: &agent.ModelSettings{
        Temperature: &[]float64{0.1}[0], // Consistent outputs
        MaxTokens:   &[]int{1024}[0],    // Shorter responses
    },
})
```

### Ollama Configuration

```go
// Local model with custom parameters
model := ollama.New(
    ollama.WithModel("llama3.2:7b"),
    ollama.WithHost("http://gpu-server:11434"),
    ollama.WithKeepAlive("10m"), // Keep model loaded
    ollama.WithOptions(map[string]interface{}{
        "num_gpu":      2,      // Use 2 GPUs
        "num_thread":   8,      // CPU threads
        "temperature":  0.7,    // Model temperature
    }),
)
```

## Best Practices

### 1. Provider Selection Strategy

```go
// Production-ready provider fallback
func createModel() llm.LLM {
    // Primary: High-quality commercial provider
    if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
        return anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))
    }
    
    // Secondary: Alternative commercial provider
    if key := os.Getenv("OPENAI_API_KEY"); key != "" {
        return openai.New(openai.WithModel("gpt-4o"))
    }
    
    // Fallback: Local inference
    return ollama.New(ollama.WithModel("llama3.2:latest"))
}
```

### 2. Model Configuration by Use Case

```go
// Research and analysis
func createResearchAgent() *agent.Agent {
    model := anthropic.New(anthropic.WithModel("claude-opus-4-20250514"))
    agent, _ := agent.New(agent.Options{
        Model: model,
        ModelSettings: &agent.ModelSettings{
            Temperature:     &[]float64{0.1}[0], // Factual responses
            ReasoningBudget: &[]int{100000}[0],   // Deep thinking
            ReasoningEffort: "high",
        },
    })
    return agent
}

// Creative writing
func createWriterAgent() *agent.Agent {
    model := openai.New(openai.WithModel("gpt-4o"))
    agent, _ := agent.New(agent.Options{
        Model: model,
        ModelSettings: &agent.ModelSettings{
            Temperature: &[]float64{0.9}[0], // Creative responses
            MaxTokens:   &[]int{4096}[0],    // Longer outputs
        },
    })
    return agent
}

// Fast customer service
func createServiceAgent() *agent.Agent {
    model := groq.New(groq.WithModel("llama-3.3-70b-versatile"))
    agent, _ := agent.New(agent.Options{
        Model: model,
        ResponseTimeout: time.Second * 30, // Quick responses
        ModelSettings: &agent.ModelSettings{
            Temperature: &[]float64{0.3}[0], // Consistent responses
            MaxTokens:   &[]int{1024}[0],    // Concise answers
        },
    })
    return agent
}
```

### 3. Error Handling and Retries

```go
// Robust model creation with retry logic
func createRobustAgent() (*agent.Agent, error) {
    models := []func() llm.LLM{
        func() llm.LLM { return anthropic.New() },
        func() llm.LLM { return openai.New() },
        func() llm.LLM { return groq.New() },
        func() llm.LLM { return ollama.New() },
    }
    
    for _, createModel := range models {
        model := createModel()
        agent, err := agent.New(agent.Options{
            Name:  "Robust Assistant",
            Model: model,
        })
        
        // Test the model with a simple request
        ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
        _, err = agent.CreateResponse(ctx, dive.WithInput("Hello"))
        cancel()
        
        if err == nil {
            return agent, nil
        }
        log.Printf("Model %T failed: %v, trying next...", model, err)
    }
    
    return nil, fmt.Errorf("no working model providers available")
}
```

### 4. Cost Management

```go
// Cost-aware model selection
func selectModelByBudget(budget string) llm.LLM {
    switch budget {
    case "premium":
        return anthropic.New(anthropic.WithModel("claude-opus-4-20250514"))
    case "standard":  
        return anthropic.New(anthropic.WithModel("claude-sonnet-4-20250514"))
    case "budget":
        return groq.New(groq.WithModel("llama-3.3-70b-versatile"))
    case "free":
        return ollama.New(ollama.WithModel("llama3.2:latest"))
    default:
        return anthropic.New() // Default to balanced option
    }
}

// Usage monitoring
func monitorUsage(agent *agent.Agent) *agent.Agent {
    return agent // Add usage tracking middleware
}
```

### 5. Development vs Production

```go
// Development setup with local models
func createDevAgent() *agent.Agent {
    model := ollama.New(ollama.WithModel("llama3.2:latest"))
    agent, _ := agent.New(agent.Options{
        Name:  "Dev Assistant",
        Model: model,
        ModelSettings: &agent.ModelSettings{
            Temperature: &[]float64{0.5}[0],
        },
    })
    return agent
}

// Production setup with commercial providers
func createProdAgent() *agent.Agent {
    model := anthropic.New(
        anthropic.WithModel("claude-sonnet-4-20250514"),
        anthropic.WithMaxRetries(3),
    )
    agent, _ := agent.New(agent.Options{
        Name:            "Production Assistant",
        Model:           model,
        ResponseTimeout: time.Minute * 2,
        ModelSettings: &agent.ModelSettings{
            Temperature: &[]float64{0.3}[0],
            Caching:     &[]bool{true}[0],
        },
    })
    return agent
}
```

## Next Steps

- [Model Configuration](model-configuration.md) - Fine-tune model behavior
- [MCP Integration](mcp-integration.md) - Connect to external tool servers
- [Agent Guide](agents.md) - Use providers with agents
- [API Reference](../api/llm.md) - Detailed LLM API documentation