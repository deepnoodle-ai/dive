# Basic Examples

This guide provides simple, practical examples to help you get started with Dive. Each example builds on the previous ones, introducing new concepts gradually.

## 📋 Table of Contents

- [Hello World Agent](#hello-world-agent)
- [Agent with Tools](#agent-with-tools)
- [Simple Workflow](#simple-workflow)
- [Interactive Chat](#interactive-chat)
- [File Processing](#file-processing)
- [Web Research](#web-research)
- [Data Analysis](#data-analysis)

## Hello World Agent

The simplest possible Dive agent that responds to text input.

### Code Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
    // Create a basic agent
    assistant, err := agent.New(agent.Options{
        Name:         "Hello Assistant",
        Instructions: "You are a friendly AI assistant who gives helpful responses.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal(err)
    }

    // Send a message to the agent
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithInput("Hello! What can you help me with?"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Print the response
    fmt.Println("Agent response:")
    fmt.Println(response.Text())
    
    // Show usage information
    if response.Usage != nil {
        fmt.Printf("\nToken usage: %d input, %d output\n", 
                   response.Usage.InputTokens, 
                   response.Usage.OutputTokens)
    }
}
```

### Environment Setup

```bash
# Set up API key
export ANTHROPIC_API_KEY="your-anthropic-api-key"

# Run the example
go run hello-world.go
```

### Expected Output

```
Agent response:
Hello! I'm here to help you with a wide variety of tasks. I can assist with:

- Answering questions and providing information
- Writing and editing text
- Problem solving and analysis
- Creative tasks like writing stories or brainstorming ideas
- Code review and programming help
- Research and summarization
- And much more!

What would you like help with today?

Token usage: 89 input, 156 output
```

## Agent with Tools

Adding tools to give your agent the ability to interact with external systems.

### Code Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

func main() {
    // Create agent with file and web tools
    assistant, err := agent.New(agent.Options{
        Name: "Tool Assistant",
        Instructions: `You are a helpful assistant with access to files and web search. 
                      Use these tools to help users with their requests.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            // File operations
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewListDirectoryTool()),
            
            // Web search
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google", // Requires GOOGLE_SEARCH_API_KEY
            })),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Ask agent to search and save results
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithInput("Search for recent news about artificial intelligence and save the results to ai-news.txt"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Agent response:")
    fmt.Println(response.Text())
    
    // Show what tools were used
    toolCalls := response.ToolCalls()
    if len(toolCalls) > 0 {
        fmt.Printf("\nTools used: %d\n", len(toolCalls))
        for _, call := range toolCalls {
            fmt.Printf("- %s\n", call.Name)
        }
    }
}
```

### Required Environment Variables

```bash
export ANTHROPIC_API_KEY="your-anthropic-key"
export GOOGLE_SEARCH_API_KEY="your-google-search-key"
export GOOGLE_SEARCH_CX="your-search-engine-id"
```

## Simple Workflow

A YAML-defined workflow that orchestrates multiple steps.

### Workflow File (research.yaml)

```yaml
Name: Simple Research
Description: Research a topic and create a summary

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info

Agents:
  - Name: Researcher
    Instructions: You are a thorough researcher who finds accurate information.
    Tools:
      - web_search
      
  - Name: Writer
    Instructions: You are a skilled writer who creates clear, engaging summaries.

Workflows:
  - Name: Research and Summarize
    Inputs:
      - Name: topic
        Type: string
        Description: The topic to research
        Required: true
        
    Steps:
      - Name: Conduct Research
        Agent: Researcher
        Prompt: |
          Research the following topic thoroughly:
          Topic: ${inputs.topic}
          
          Find the most current and relevant information.
        Store: research_data
        
      - Name: Create Summary
        Agent: Writer
        Prompt: |
          Create a comprehensive but concise summary based on this research:
          ${research_data}
          
          Make it engaging and easy to understand.
        Store: final_summary
        
      - Name: Save Results
        Action: Document.Write
        Parameters:
          Path: "research-${inputs.topic}.md"
          Content: |
            # Research Summary: ${inputs.topic}
            
            ${final_summary}
            
            ---
            *Generated by Dive Research Workflow*
```

### Running the Workflow

```bash
# Using the CLI
dive run research.yaml --vars "topic=renewable energy"

# Programmatically
```go
package main

import (
    "context"
    "log"
    
    "github.com/diveagents/dive/config"
    "github.com/diveagents/dive/environment"
)

func main() {
    // Load workflow from YAML
    cfg, err := config.LoadFromFile("research.yaml")
    if err != nil {
        log.Fatal(err)
    }
    
    // Build environment
    env, err := config.BuildEnvironment(cfg)
    if err != nil {
        log.Fatal(err)
    }
    
    // Run workflow
    inputs := map[string]interface{}{
        "topic": "renewable energy",
    }
    
    execution, err := env.RunWorkflow(context.Background(), "Research and Summarize", inputs)
    if err != nil {
        log.Fatal(err)
    }
    
    // Wait for completion
    result, err := execution.Wait(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Workflow completed: %s", result.Status)
}
```

## Interactive Chat

Building an interactive chat interface with an agent.

### Code Example

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "strings"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/objects"
)

func main() {
    // Create thread repository for conversation memory
    threadRepo := objects.NewInMemoryThreadRepository()
    
    // Create agent with thread support
    assistant, err := agent.New(agent.Options{
        Name: "Chat Assistant",
        Instructions: `You are a helpful AI assistant. 
                      Remember our conversation history and provide helpful responses.`,
        Model:            anthropic.New(),
        ThreadRepository: threadRepo,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("=== Dive Chat Assistant ===")
    fmt.Println("Type 'quit' to exit, 'clear' to start new conversation")
    fmt.Println()

    scanner := bufio.NewScanner(os.Stdin)
    threadID := "chat-session-1"

    for {
        fmt.Print("You: ")
        if !scanner.Scan() {
            break
        }

        input := strings.TrimSpace(scanner.Text())
        if input == "" {
            continue
        }
        
        // Handle special commands
        switch input {
        case "quit", "exit":
            fmt.Println("Goodbye!")
            return
        case "clear":
            threadID = dive.NewID()
            fmt.Println("Started new conversation.")
            continue
        }

        // Send message to agent
        fmt.Print("Assistant: ")
        
        // Use streaming for real-time response
        stream, err := assistant.StreamResponse(
            context.Background(),
            dive.WithThreadID(threadID),
            dive.WithInput(input),
        )
        if err != nil {
            fmt.Printf("Error: %v\n", err)
            continue
        }

        // Print streaming response
        for event := range stream.Events() {
            if event.Type == dive.EventTypeLLMEvent && 
               event.Item.Event.Type == "content_block_delta" {
                fmt.Print(event.Item.Event.Delta.Text)
            }
        }
        fmt.Println() // New line after response
        
        stream.Close()
    }
}
```

## File Processing

Processing files with an agent using built-in file tools.

### Code Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

func main() {
    // Create agent with file processing capabilities
    processor, err := agent.New(agent.Options{
        Name: "File Processor",
        Instructions: `You are a file processing assistant. You can read, analyze, and modify files.
                      Always provide clear summaries of what you find and any changes you make.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewListDirectoryTool()),
            dive.ToolAdapter(toolkit.NewTextEditorTool()),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create sample data file
    sampleData := `Name,Age,City
Alice,25,New York
Bob,30,San Francisco
Carol,28,Chicago
David,35,Boston`

    fmt.Println("Creating sample data file...")
    _, err = processor.CreateResponse(
        context.Background(),
        dive.WithInput(fmt.Sprintf(`Create a file called "sample.csv" with this content:
%s`, sampleData)),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Process the file
    fmt.Println("\nProcessing the file...")
    response, err := processor.CreateResponse(
        context.Background(),
        dive.WithInput(`Analyze the sample.csv file and:
1. Count how many records it has
2. Calculate the average age  
3. List all unique cities
4. Create a summary report and save it as "analysis.txt"`),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Processing complete!")
    fmt.Println("Agent response:")
    fmt.Println(response.Text())
}
```

## Web Research

Performing web research and compiling results.

### Code Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

func main() {
    // Create research agent
    researcher, err := agent.New(agent.Options{
        Name: "Web Researcher",
        Instructions: `You are a professional researcher. When researching:
                      1. Use multiple sources for comprehensive coverage
                      2. Verify information across sources
                      3. Cite your sources clearly
                      4. Organize findings logically`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewFetchTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Research topic
    topic := "latest developments in quantum computing 2024"
    
    fmt.Printf("Researching: %s\n", topic)
    fmt.Println("This may take a few moments...")
    
    response, err := researcher.CreateResponse(
        context.Background(),
        dive.WithInput(fmt.Sprintf(`Research the topic: %s

Please:
1. Search for the most recent and relevant information
2. Fetch content from at least 3 high-quality sources
3. Create a comprehensive research report
4. Save the report as "research-report.md"

Include:
- Executive summary
- Key developments
- Major players/companies involved
- Technical breakthroughs
- Future implications
- Source citations`, topic)),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("\nResearch completed!")
    fmt.Println("Report saved to research-report.md")
    fmt.Println("\nAgent summary:")
    fmt.Println(response.Text())
}
```

### Required Environment Variables

```bash
export ANTHROPIC_API_KEY="your-anthropic-key"
export GOOGLE_SEARCH_API_KEY="your-google-search-key"  
export GOOGLE_SEARCH_CX="your-search-engine-id"
export FIRECRAWL_API_KEY="your-firecrawl-key"
```

## Data Analysis

Analyzing data with computational capabilities.

### Code Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "math"
    "math/rand"
    "time"
    
    "github.com/diveagents/dive"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
    "github.com/diveagents/dive/toolkit"
)

// Custom analysis tool
type DataAnalysisTool struct{}

type AnalysisInput struct {
    Data []float64 `json:"data" description:"Array of numbers to analyze"`
}

func (t *DataAnalysisTool) Name() string {
    return "analyze_data"
}

func (t *DataAnalysisTool) Description() string {
    return "Perform statistical analysis on numerical data"
}

func (t *DataAnalysisTool) Schema() dive.Schema {
    return dive.Schema{
        Type: "object",
        Properties: map[string]dive.Property{
            "data": {
                Type:        "array",
                Description: "Array of numbers to analyze",
                Items: &dive.Property{
                    Type: "number",
                },
            },
        },
        Required: []string{"data"},
    }
}

func (t *DataAnalysisTool) Annotations() dive.ToolAnnotations {
    return dive.ToolAnnotations{
        Title:          "Data Analysis Tool",
        ReadOnlyHint:   true,
        IdempotentHint: true,
    }
}

func (t *DataAnalysisTool) Call(ctx context.Context, input *AnalysisInput) (*dive.ToolResult, error) {
    data := input.Data
    if len(data) == 0 {
        return &dive.ToolResult{
            Content: []*dive.ToolResultContent{{
                Type: dive.ToolResultContentTypeText,
                Text: "No data provided",
            }},
            IsError: true,
        }, nil
    }

    // Calculate statistics
    sum := 0.0
    min := data[0]
    max := data[0]
    
    for _, v := range data {
        sum += v
        if v < min {
            min = v
        }
        if v > max {
            max = v
        }
    }
    
    mean := sum / float64(len(data))
    
    // Calculate standard deviation
    variance := 0.0
    for _, v := range data {
        variance += math.Pow(v-mean, 2)
    }
    variance /= float64(len(data))
    stdDev := math.Sqrt(variance)
    
    result := fmt.Sprintf(`Statistical Analysis Results:
- Count: %d
- Mean: %.2f
- Min: %.2f
- Max: %.2f
- Range: %.2f
- Standard Deviation: %.2f
- Variance: %.2f`, 
        len(data), mean, min, max, max-min, stdDev, variance)
    
    return &dive.ToolResult{
        Content: []*dive.ToolResultContent{{
            Type: dive.ToolResultContentTypeText,
            Text: result,
        }},
    }, nil
}

func main() {
    // Create analyst agent with custom tool
    analyst, err := agent.New(agent.Options{
        Name: "Data Analyst",
        Instructions: `You are a data analyst with statistical analysis capabilities.
                      Use the analyze_data tool for numerical computations and provide insights.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(&DataAnalysisTool{}),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Generate sample data
    rand.Seed(time.Now().UnixNano())
    sampleData := make([]float64, 100)
    for i := range sampleData {
        sampleData[i] = rand.NormFloat64()*10 + 50 // Normal distribution around 50
    }

    fmt.Println("Analyzing sample dataset...")
    
    response, err := analyst.CreateResponse(
        context.Background(),
        dive.WithInput(fmt.Sprintf(`I have a dataset of 100 measurements. Please:
1. Analyze this data: %v
2. Interpret the statistical results
3. Identify any patterns or outliers
4. Create a summary report and save it as "analysis-report.txt"

Provide insights about what this data might represent and any notable characteristics.`, sampleData)),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("\nAnalysis completed!")
    fmt.Println("Agent response:")
    fmt.Println(response.Text())
}
```

## Running the Examples

### Prerequisites

```bash
# Install Go dependencies
go mod init dive-examples
go get github.com/diveagents/dive

# Set up environment variables
export ANTHROPIC_API_KEY="your-key"
export GOOGLE_SEARCH_API_KEY="your-key"  # For web search examples
export GOOGLE_SEARCH_CX="your-cx"        # For web search examples
export FIRECRAWL_API_KEY="your-key"      # For content fetching
```

### Running Examples

```bash
# Hello World
go run hello-world.go

# Tools example
go run tools-example.go

# Workflow (using CLI)
dive run research.yaml --vars "topic=AI"

# Interactive chat
go run chat-example.go

# File processing
go run file-processing.go

# Web research
go run web-research.go

# Data analysis
go run data-analysis.go
```

## Common Patterns

### Error Handling

```go
response, err := agent.CreateResponse(ctx, dive.WithInput(input))
if err != nil {
    log.Printf("Agent error: %v", err)
    return
}

// Check for tool errors in response
for _, result := range response.ToolResults() {
    if result.Result.IsError {
        log.Printf("Tool %s failed: %s", 
                   result.Name, 
                   result.Result.Content[0].Text)
    }
}
```

### Event Handling

```go
callback := func(ctx context.Context, event *dive.ResponseEvent) error {
    switch event.Type {
    case dive.EventTypeResponseToolCall:
        fmt.Printf("Calling tool: %s\n", event.Item.ToolCall.Name)
    case dive.EventTypeResponseInProgress:
        // Handle partial response
    case dive.EventTypeResponseCompleted:
        fmt.Println("Response completed")
    }
    return nil
}

response, err := agent.CreateResponse(
    ctx,
    dive.WithInput(input),
    dive.WithEventCallback(callback),
)
```

### Resource Management

```go
// Always use context with timeout
ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
defer cancel()

// Close streams properly
stream, err := agent.StreamResponse(ctx, opts...)
if err != nil {
    return err
}
defer stream.Close()

for event := range stream.Events() {
    // Handle events
}
```

## Next Steps

- [Advanced Examples](advanced.md) - Complex multi-agent scenarios
- [Integration Examples](integrations.md) - Real-world use cases
- [Agent Guide](../guides/agents.md) - Learn more about agents
- [Workflow Guide](../guides/workflows.md) - Master workflow creation