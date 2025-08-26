# Model Configuration Guide

This guide covers how to fine-tune LLM behavior in Dive through model configuration options, provider-specific settings, and advanced features like reasoning control and caching.

## 📋 Table of Contents

- [Overview](#overview)
- [Basic Configuration](#basic-configuration)
- [Model Settings](#model-settings)
- [Provider-Specific Configuration](#provider-specific-configuration)
- [Advanced Features](#advanced-features)
- [Performance Optimization](#performance-optimization)
- [Best Practices](#best-practices)

## Overview

Model configuration in Dive allows you to control:

- **Temperature and Creativity** - Control randomness and creativity
- **Token Limits** - Set input and output token constraints
- **Reasoning Control** - Configure reasoning effort and budget
- **Tool Behavior** - Control when and how tools are used
- **Caching** - Enable prompt caching for performance
- **Provider Features** - Access provider-specific capabilities

### Configuration Hierarchy

```
Environment Config → Agent Config → ModelSettings → Provider Defaults
```

Settings are applied in order, with more specific settings overriding general ones.

## Basic Configuration

### Agent-Level Configuration

```go
package main

import (
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
    agent, err := agent.New(agent.Options{
        Name:         "Configured Assistant",
        Instructions: "You are a helpful assistant.",
        Model:        anthropic.New(),
        
        // Basic model settings
        ModelSettings: &agent.ModelSettings{
            Temperature:     &[]float64{0.7}[0],    // Creativity level (0.0-1.0)
            MaxTokens:       &[]int{2048}[0],       // Response length limit
            PresencePenalty: &[]float64{0.0}[0],    // Reduce repetition
            FrequencyPenalty: &[]float64{0.0}[0],   // Reduce word frequency
        },
    })
    if err != nil {
        panic(err)
    }
}
```

### YAML Configuration

```yaml
# agent-config.yaml
Name: Smart Assistant
Instructions: You are an intelligent assistant with customized behavior.

ModelSettings:
  Temperature: 0.7
  MaxTokens: 4000
  ReasoningEffort: high
  ReasoningBudget: 50000
  ParallelToolCalls: true
  Caching: true

Config:
  Provider: anthropic
  Model: claude-sonnet-4-20250514
```

## Model Settings

### Temperature Control

Temperature controls the randomness and creativity of responses:

```go
// Conservative, factual responses
agent.ModelSettings{
    Temperature: &[]float64{0.1}[0], // Low creativity, factual
}

// Balanced responses
agent.ModelSettings{
    Temperature: &[]float64{0.7}[0], // Moderate creativity
}

// Creative, varied responses
agent.ModelSettings{
    Temperature: &[]float64{0.9}[0], // High creativity
}
```

**Use Cases:**
- **0.0-0.3**: Code generation, factual Q&A, data analysis
- **0.4-0.7**: General conversation, content creation, problem solving
- **0.8-1.0**: Creative writing, brainstorming, artistic tasks

### Token Limits

Control input and output token usage:

```go
agent.ModelSettings{
    MaxTokens: &[]int{4096}[0], // Maximum response tokens
    
    // Some providers support input token limits
    MaxInputTokens: &[]int{100000}[0], // Maximum input context
}
```

### Presence and Frequency Penalties

Reduce repetitive content:

```go
agent.ModelSettings{
    // Presence penalty: reduce likelihood of repeating topics
    PresencePenalty: &[]float64{0.6}[0], // Range: -2.0 to 2.0
    
    // Frequency penalty: reduce likelihood of repeating tokens
    FrequencyPenalty: &[]float64{0.3}[0], // Range: -2.0 to 2.0
}
```

### Tool Control

Configure tool calling behavior:

```go
agent.ModelSettings{
    // Allow parallel tool execution
    ParallelToolCalls: &[]bool{true}[0],
    
    // Control tool choice
    ToolChoice: &llm.ToolChoice{
        Type: "auto", // "auto", "none", or specific tool
    },
}
```

## Provider-Specific Configuration

### Anthropic (Claude) Configuration

```go
import "github.com/diveagents/dive/llm/providers/anthropic"

// Basic Anthropic setup
model := anthropic.New(
    anthropic.WithModel("claude-sonnet-4-20250514"),
    anthropic.WithMaxTokens(4096),
    anthropic.WithTemperature(0.7),
)

// Advanced Anthropic features
agent, err := agent.New(agent.Options{
    Model: model,
    ModelSettings: &agent.ModelSettings{
        // Reasoning control (Claude Sonnet/Opus)
        ReasoningBudget: &[]int{50000}[0], // Token budget for reasoning
        ReasoningEffort: "high",           // "low", "medium", "high"
        
        // Enable prompt caching
        Caching: &[]bool{true}[0],
        
        // Provider-specific features
        Features: []string{"computer_use"}, // Enable computer use
        
        // Custom headers
        RequestHeaders: http.Header{
            "X-Custom-Header": []string{"custom-value"},
        },
    },
})
```

### OpenAI Configuration

```go
import "github.com/diveagents/dive/llm/providers/openai"

// GPT-4 configuration
model := openai.New(
    openai.WithModel("gpt-4o"),
    openai.WithTemperature(0.7),
    openai.WithMaxTokens(4000),
)

// o1/o3 reasoning models
reasoningModel := openai.New(
    openai.WithModel("o1"),
    // Note: Reasoning models have limited parameter support
)

agent, err := agent.New(agent.Options{
    Model: reasoningModel,
    ModelSettings: &agent.ModelSettings{
        // Reasoning budget for o1/o3 models
        ReasoningBudget: &[]int{100000}[0],
        ReasoningEffort: "high",
        
        // Response format control
        ResponseFormat: &llm.ResponseFormat{
            Type: "json_object", // Force JSON responses
        },
    },
})
```

### Groq Configuration

```go
import "github.com/diveagents/dive/llm/providers/groq"

// High-performance Groq setup
model := groq.New(
    groq.WithModel("deepseek-r1-distill-llama-70b"),
    groq.WithTimeout(time.Second * 30), // Fast timeout for speed
)

agent, err := agent.New(agent.Options{
    Model: model,
    ModelSettings: &agent.ModelSettings{
        Temperature: &[]float64{0.3}[0], // Consistent outputs
        MaxTokens:   &[]int{1024}[0],    // Shorter responses for speed
    },
})
```

### Ollama Configuration

```go
import "github.com/diveagents/dive/llm/providers/ollama"

// Local Ollama setup
model := ollama.New(
    ollama.WithModel("llama3.2:7b"),
    ollama.WithHost("http://localhost:11434"),
    ollama.WithKeepAlive("10m"), // Keep model loaded
    ollama.WithOptions(map[string]interface{}{
        "num_gpu":      2,      // Use GPUs
        "num_thread":   8,      // CPU threads
        "temperature":  0.7,    // Model temperature
        "top_p":       0.9,     // Nucleus sampling
        "top_k":       40,      // Top-k sampling
    }),
)
```

## Advanced Features

### Reasoning Control

For models that support reasoning (Claude Sonnet/Opus, OpenAI o1/o3):

```go
// Different reasoning levels for different tasks
func createReasoningAgents() {
    // Fast, light reasoning for simple tasks
    quickAgent, _ := agent.New(agent.Options{
        Name: "Quick Assistant",
        ModelSettings: &agent.ModelSettings{
            ReasoningEffort: "low",
            ReasoningBudget: &[]int{5000}[0],
        },
    })
    
    // Deep reasoning for complex problems
    deepAgent, _ := agent.New(agent.Options{
        Name: "Deep Thinker",
        ModelSettings: &agent.ModelSettings{
            ReasoningEffort: "high",
            ReasoningBudget: &[]int{100000}[0],
        },
    })
    
    // Balanced reasoning for general use
    balancedAgent, _ := agent.New(agent.Options{
        Name: "Balanced Assistant",
        ModelSettings: &agent.ModelSettings{
            ReasoningEffort: "medium", 
            ReasoningBudget: &[]int{25000}[0],
        },
    })
}
```

### Prompt Caching

Enable caching to reduce costs and improve performance:

```go
agent, err := agent.New(agent.Options{
    ModelSettings: &agent.ModelSettings{
        Caching: &[]bool{true}[0], // Enable prompt caching
    },
})

// Caching is automatically managed:
// - System prompts are cached
// - Long conversation histories are cached
// - Cache control headers are set appropriately
```

### Response Format Control

Control output format for structured data:

```go
// JSON response format (OpenAI)
agent, err := agent.New(agent.Options{
    Model: openai.New(),
    ModelSettings: &agent.ModelSettings{
        ResponseFormat: &llm.ResponseFormat{
            Type: "json_object",
            Schema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "analysis": {"type": "string"},
                    "confidence": {"type": "number"},
                    "recommendations": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                },
                "required": []string{"analysis", "confidence"},
            },
        },
    },
})
```

### Custom Request Headers

Add custom headers for monitoring or authentication:

```go
agent, err := agent.New(agent.Options{
    ModelSettings: &agent.ModelSettings{
        RequestHeaders: http.Header{
            "X-Organization-ID": []string{"org-123"},
            "X-Project-ID":     []string{"proj-456"},
            "X-User-Agent":     []string{"MyApp/1.0"},
        },
    },
})
```

## Performance Optimization

### Caching Strategy

```go
// Enable caching for production agents
func createProductionAgent() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Production Assistant",
        ModelSettings: &agent.ModelSettings{
            Caching:     &[]bool{true}[0],  // Enable caching
            Temperature: &[]float64{0.3}[0], // Lower temperature for consistency
        },
    })
}
```

### Token Optimization

```go
// Optimize token usage for cost efficiency
func createCostOptimizedAgent() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Cost Optimized",
        Instructions: "Be concise and direct in your responses.",
        ModelSettings: &agent.ModelSettings{
            MaxTokens:   &[]int{1000}[0],    // Limit response length
            Temperature: &[]float64{0.1}[0], // Reduce randomness
            
            // Use shorter reasoning budget
            ReasoningBudget: &[]int{10000}[0],
            ReasoningEffort: "medium",
        },
    })
}
```

### Parallel Tool Calls

Enable parallel tool execution for faster responses:

```go
agent, err := agent.New(agent.Options{
    ModelSettings: &agent.ModelSettings{
        ParallelToolCalls: &[]bool{true}[0], // Execute tools concurrently
    },
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewWebSearchTool(...)),
        dive.ToolAdapter(toolkit.NewReadFileTool()),
        dive.ToolAdapter(toolkit.NewWriteFileTool()),
    },
})
```

## Best Practices

### 1. Task-Specific Configuration

```go
// Different configurations for different use cases

// Code generation - precise and deterministic
func createCodeAgent() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Code Generator",
        Instructions: "You generate clean, efficient code with proper error handling.",
        ModelSettings: &agent.ModelSettings{
            Temperature:       &[]float64{0.1}[0], // Low creativity
            MaxTokens:         &[]int{4000}[0],    // Longer responses
            PresencePenalty:   &[]float64{0.2}[0], // Reduce repetition
            FrequencyPenalty:  &[]float64{0.1}[0], // Slight frequency penalty
        },
    })
}

// Creative writing - high creativity and variation
func createWriterAgent() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Creative Writer",
        Instructions: "You are a creative writer who produces engaging, original content.",
        ModelSettings: &agent.ModelSettings{
            Temperature:     &[]float64{0.9}[0], // High creativity
            MaxTokens:       &[]int{4000}[0],    // Longer responses
            PresencePenalty: &[]float64{0.0}[0], // Allow topic repetition
        },
    })
}

// Customer service - balanced and helpful
func createServiceAgent() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Customer Service",
        Instructions: "You provide helpful, professional customer service.",
        ModelSettings: &agent.ModelSettings{
            Temperature:  &[]float64{0.5}[0], // Balanced creativity
            MaxTokens:    &[]int{1500}[0],    // Moderate response length
            ReasoningEffort: "medium",        // Thoughtful responses
        },
    })
}

// Data analysis - factual and precise
func createAnalystAgent() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Data Analyst",
        Instructions: "You analyze data objectively and provide factual insights.",
        ModelSettings: &agent.ModelSettings{
            Temperature:     &[]float64{0.0}[0], // Completely deterministic
            ReasoningEffort: "high",             // Deep analysis
            ReasoningBudget: &[]int{75000}[0],   // Large reasoning budget
        },
    })
}
```

### 2. Environment-Specific Settings

```go
// Development vs Production configurations
func createEnvironmentSpecificAgent(env string) (*agent.Agent, error) {
    var settings *agent.ModelSettings
    
    switch env {
    case "development":
        settings = &agent.ModelSettings{
            Temperature: &[]float64{0.8}[0], // More creative for experimentation
            MaxTokens:   &[]int{2000}[0],    // Moderate token usage
            Caching:     &[]bool{false}[0],  // Disable caching for testing
        }
        
    case "staging":
        settings = &agent.ModelSettings{
            Temperature: &[]float64{0.5}[0], // Balanced for testing
            MaxTokens:   &[]int{2000}[0],    
            Caching:     &[]bool{true}[0],   // Enable caching
        }
        
    case "production":
        settings = &agent.ModelSettings{
            Temperature: &[]float64{0.3}[0], // Consistent responses
            MaxTokens:   &[]int{1500}[0],    // Cost optimization
            Caching:     &[]bool{true}[0],   // Enable caching
            
            // Production monitoring headers
            RequestHeaders: http.Header{
                "X-Environment": []string{"production"},
                "X-Version":     []string{"1.0.0"},
            },
        }
        
    default:
        settings = &agent.ModelSettings{
            Temperature: &[]float64{0.7}[0], // Default settings
        }
    }
    
    return agent.New(agent.Options{
        Name:          "Environment Agent",
        ModelSettings: settings,
    })
}
```

### 3. Configuration Management

```go
// Centralized configuration management
type AgentConfig struct {
    Name         string
    Instructions string
    Temperature  float64
    MaxTokens    int
    ReasoningConfig struct {
        Effort string
        Budget int
    }
    Features []string
}

func loadAgentConfig(configPath string) (*AgentConfig, error) {
    // Load from YAML, JSON, or other config file
    data, err := ioutil.ReadFile(configPath)
    if err != nil {
        return nil, err
    }
    
    var config AgentConfig
    err = yaml.Unmarshal(data, &config)
    if err != nil {
        return nil, err
    }
    
    return &config, nil
}

func createAgentFromConfig(config *AgentConfig) (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name:         config.Name,
        Instructions: config.Instructions,
        ModelSettings: &agent.ModelSettings{
            Temperature:     &config.Temperature,
            MaxTokens:       &config.MaxTokens,
            ReasoningEffort: config.ReasoningConfig.Effort,
            ReasoningBudget: &config.ReasoningConfig.Budget,
            Features:        config.Features,
        },
    })
}
```

### 4. A/B Testing Different Configurations

```go
// Test different configurations to find optimal settings
func testConfigurations() {
    configs := []agent.ModelSettings{
        {
            Temperature: &[]float64{0.3}[0],
            MaxTokens:   &[]int{1000}[0],
        },
        {
            Temperature: &[]float64{0.7}[0],
            MaxTokens:   &[]int{1500}[0],
        },
        {
            Temperature: &[]float64{0.9}[0],
            MaxTokens:   &[]int{2000}[0],
        },
    }
    
    testPrompts := []string{
        "Explain quantum computing",
        "Write a creative story about AI",
        "Analyze this data: [1,2,3,4,5]",
    }
    
    for i, config := range configs {
        fmt.Printf("Testing configuration %d:\n", i+1)
        
        agent, err := agent.New(agent.Options{
            Name:          fmt.Sprintf("Test Agent %d", i+1),
            ModelSettings: &config,
        })
        if err != nil {
            continue
        }
        
        for _, prompt := range testPrompts {
            response, err := agent.CreateResponse(
                context.Background(),
                dive.WithInput(prompt),
            )
            if err == nil {
                fmt.Printf("  Prompt: %s\n", prompt)
                fmt.Printf("  Response length: %d tokens\n", 
                          response.Usage.OutputTokens)
                fmt.Printf("  Response: %s...\n", 
                          response.Text()[:min(100, len(response.Text()))])
            }
        }
        fmt.Println()
    }
}
```

## Next Steps

- [LLM Providers Guide](llm-providers.md) - Learn about different providers
- [Agent Guide](agents.md) - Understand how agents use model settings
- [Performance Guide](performance.md) - Optimize your applications
- [API Reference](../api/llm.md) - Detailed API documentation