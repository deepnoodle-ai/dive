# Quick Start Guide

Get up and running with Dive in just a few minutes! This guide will help you create your first AI agent and run a simple workflow.

## 📋 Table of Contents

- [Before You Begin](#before-you-begin)
- [Your First Agent](#your-first-agent)
- [Adding Tools](#adding-tools)
- [Creating a Workflow](#creating-a-workflow)
- [Interactive Chat](#interactive-chat)
- [Next Steps](#next-steps)

## Before You Begin

Make sure you have:

1. **Go installed** (version 1.23.2 or later)
2. **An API key** from at least one LLM provider:
   - [Anthropic API key](https://console.anthropic.com/)
   - [OpenAI API key](https://platform.openai.com/api-keys)
   - [Groq API key](https://console.groq.com/keys)
   - Or [Ollama](https://ollama.ai/) running locally

Set your API key:
```bash
export ANTHROPIC_API_KEY="your-api-key-here"
# or
export OPENAI_API_KEY="your-api-key-here"
```

## Your First Agent

Let's create a simple AI agent that can answer questions.

### Step 1: Initialize Your Project

```bash
# Create a new directory
mkdir my-dive-project
cd my-dive-project

# Initialize Go module
go mod init my-dive-project

# Add Dive dependency
go get github.com/diveagents/dive
```

### Step 2: Create Your First Agent

Create `main.go`:

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
    // Create an AI agent
    assistant, err := agent.New(agent.Options{
        Name:         "My Assistant",
        Instructions: "You are a helpful AI assistant who provides clear, concise answers.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal("Error creating agent:", err)
    }

    // Ask the agent a question
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithInput("What is artificial intelligence?"),
    )
    if err != nil {
        log.Fatal("Error getting response:", err)
    }

    // Print the response
    fmt.Println("Agent Response:")
    fmt.Println(response.Text())
}
```

### Step 3: Run Your Agent

```bash
go run main.go
```

You should see output like:

```
Agent Response:
Artificial intelligence (AI) refers to computer systems that can perform tasks typically requiring human intelligence, such as learning, reasoning, problem-solving, and decision-making...
```

🎉 **Congratulations!** You've created your first Dive agent!

## Adding Tools

Let's make your agent more powerful by adding tools that let it interact with files and the web.

### Step 1: Install Tool Dependencies

For web search capabilities, get a Google Custom Search API key:

1. Visit [Google Custom Search](https://developers.google.com/custom-search/v1/overview)
2. Create a Custom Search Engine
3. Get your API key and Search Engine ID

```bash
export GOOGLE_SEARCH_API_KEY="your-google-api-key"
export GOOGLE_SEARCH_CX="your-search-engine-id"
```

### Step 2: Create an Agent with Tools

Create `agent-with-tools.go`:

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
    // Create an agent with file and web tools
    assistant, err := agent.New(agent.Options{
        Name: "Research Assistant",
        Instructions: `You are a research assistant who can search the web and work with files.
                      Help users find information and save it to files when requested.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            // File operations
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            
            // Web search
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
        },
    })
    if err != nil {
        log.Fatal("Error creating agent:", err)
    }

    // Ask the agent to research something and save it
    response, err := assistant.CreateResponse(
        context.Background(),
        dive.WithInput("Search for recent developments in quantum computing and save a summary to quantum-research.txt"),
    )
    if err != nil {
        log.Fatal("Error getting response:", err)
    }

    fmt.Println("Agent Response:")
    fmt.Println(response.Text())

    // Show which tools were used
    toolCalls := response.ToolCalls()
    if len(toolCalls) > 0 {
        fmt.Printf("\nTools used: %d\n", len(toolCalls))
        for _, call := range toolCalls {
            fmt.Printf("- %s\n", call.Name)
        }
    }
}
```

### Step 3: Run the Enhanced Agent

```bash
go run agent-with-tools.go
```

Your agent will now search the web and create a file with the research results!

## Creating a Workflow

Workflows let you define multi-step processes that can involve multiple agents and automated actions.

### Step 1: Create a Workflow File

Create `research-workflow.yaml`:

```yaml
Name: Research Pipeline
Description: Research a topic and create a comprehensive report

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info

Agents:
  - Name: Researcher
    Instructions: |
      You are a thorough researcher who finds accurate, up-to-date information.
      Always cite your sources and provide comprehensive analysis.
    Tools:
      - web_search
      
  - Name: Writer
    Instructions: |
      You are a skilled technical writer who creates clear, well-structured reports.
      Organize information logically with proper headings and formatting.
    Tools:
      - write_file

Workflows:
  - Name: Research and Report
    Inputs:
      - Name: topic
        Type: string
        Description: The topic to research
        Required: true
        
      - Name: depth
        Type: string
        Description: Research depth (basic, detailed, comprehensive)
        Default: detailed
        
    Steps:
      - Name: Conduct Research
        Agent: Researcher
        Prompt: |
          Research the following topic with ${inputs.depth} depth:
          Topic: ${inputs.topic}
          
          Provide comprehensive information including:
          - Current state and recent developments
          - Key players and organizations
          - Technical details and implications
          - Future outlook and trends
        Store: research_data
        
      - Name: Create Report
        Agent: Writer  
        Prompt: |
          Create a professional research report based on this information:
          ${research_data}
          
          Structure the report with:
          1. Executive Summary
          2. Background and Context
          3. Current State Analysis
          4. Key Findings
          5. Future Implications
          6. Conclusion
          
          Use clear headings and professional formatting.
        Store: final_report
        
      - Name: Save Report
        Action: Document.Write
        Parameters:
          Path: "reports/${inputs.topic}-research-report.md"
          Content: |
            # Research Report: ${inputs.topic}
            
            *Generated on $(date) by Dive Research Pipeline*
            
            ${final_report}
```

### Step 2: Install and Run with CLI

```bash
# Install Dive CLI
go install github.com/diveagents/dive/cmd/dive@latest

# Run the workflow
dive run research-workflow.yaml --vars "topic=artificial intelligence" --vars "depth=comprehensive"
```

### Step 3: Run Programmatically

Create `workflow-runner.go`:

```go
package main

import (
    "context"
    "log"

    "github.com/diveagents/dive/config"
)

func main() {
    // Load workflow from YAML
    cfg, err := config.LoadFromFile("research-workflow.yaml")
    if err != nil {
        log.Fatal("Error loading workflow:", err)
    }

    // Build environment with agents and workflows
    env, err := config.BuildEnvironment(cfg)
    if err != nil {
        log.Fatal("Error building environment:", err)
    }

    // Start the environment
    err = env.Start(context.Background())
    if err != nil {
        log.Fatal("Error starting environment:", err)
    }
    defer env.Stop(context.Background())

    // Run the workflow
    inputs := map[string]interface{}{
        "topic": "machine learning",
        "depth": "detailed",
    }

    execution, err := env.RunWorkflow(context.Background(), "Research and Report", inputs)
    if err != nil {
        log.Fatal("Error running workflow:", err)
    }

    // Wait for completion
    result, err := execution.Wait(context.Background())
    if err != nil {
        log.Fatal("Workflow execution failed:", err)
    }

    log.Printf("Workflow completed with status: %s", result.Status)
    if result.Outputs != nil {
        log.Printf("Outputs: %+v", result.Outputs)
    }
}
```

Run it:

```bash
go run workflow-runner.go
```

## Interactive Chat

Create an interactive chat session with your agent.

Create `chat.go`:

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

    // Create agent with memory
    assistant, err := agent.New(agent.Options{
        Name: "Chat Assistant",
        Instructions: `You are a helpful AI assistant. Remember our conversation
                      and provide helpful, contextual responses.`,
        Model:            anthropic.New(),
        ThreadRepository: threadRepo,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("🤖 Dive Chat Assistant")
    fmt.Println("Type 'quit' to exit, 'clear' to start new conversation")
    fmt.Println("=" * 50)

    scanner := bufio.NewScanner(os.Stdin)
    threadID := "chat-session-1"

    for {
        fmt.Print("\nYou: ")
        if !scanner.Scan() {
            break
        }

        input := strings.TrimSpace(scanner.Text())
        if input == "" {
            continue
        }

        switch input {
        case "quit", "exit":
            fmt.Println("👋 Goodbye!")
            return
        case "clear":
            threadID = dive.NewID()
            fmt.Println("🔄 Started new conversation")
            continue
        }

        fmt.Print("🤖 Assistant: ")

        // Stream the response for real-time output
        stream, err := assistant.StreamResponse(
            context.Background(),
            dive.WithThreadID(threadID),
            dive.WithInput(input),
        )
        if err != nil {
            fmt.Printf("❌ Error: %v\n", err)
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

Run the interactive chat:

```bash
go run chat.go
```

You can now have a conversation with your agent:

```
🤖 Dive Chat Assistant
Type 'quit' to exit, 'clear' to start new conversation
==================================================

You: Hello! What's your name?
🤖 Assistant: Hello! I'm your Chat Assistant, created using Dive. I'm here to help you with questions, tasks, and conversations. What would you like to talk about or how can I assist you today?

You: Remember that my favorite color is blue
🤖 Assistant: Got it! I'll remember that your favorite color is blue. Is there anything specific about blue that you particularly like, or would you like to talk about something else?

You: What's my favorite color?
🤖 Assistant: Your favorite color is blue! You just told me that in our conversation.
```

## Next Steps

Now that you have the basics working, explore more advanced features:

### 🔧 Learn More About Core Concepts
- [Agents Guide](agents.md) - Deep dive into agent capabilities
- [Workflows Guide](workflows.md) - Master multi-step automation  
- [Tools Guide](tools.md) - Extend agent capabilities
- [Environment Guide](environment.md) - Manage shared resources

### 🚀 Advanced Features
- [LLM Providers](llm-providers.md) - Work with different AI models
- [MCP Integration](mcp-integration.md) - Connect to external services
- [Event Streaming](event-streaming.md) - Real-time monitoring
- [Supervisor Patterns](supervisor-patterns.md) - Multi-agent coordination

### 📚 Examples and Use Cases
- [Basic Examples](../examples/basic.md) - More code examples
- [Advanced Examples](../examples/advanced.md) - Complex scenarios
- [Integration Examples](../examples/integrations.md) - Real-world use cases

### 🛠️ Development
- [Custom Tools](custom-tools.md) - Build your own tools
- [Testing Guide](testing.md) - Test your Dive applications
- [API Reference](../api/core.md) - Detailed API documentation

### 💬 Get Help
- Join our [Discord community](https://discord.gg/yrcuURWk)
- Check out [GitHub Discussions](https://github.com/diveagents/dive/discussions)
- Browse the [GitHub Issues](https://github.com/diveagents/dive/issues)

## Common Next Steps

Here are some popular things to try next:

### 1. Add More Tools
```go
// Add web fetching, command execution, image generation
Tools: []dive.Tool{
    dive.ToolAdapter(toolkit.NewWebSearchTool(...)),
    dive.ToolAdapter(toolkit.NewFetchTool()),
    dive.ToolAdapter(toolkit.NewCommandTool()),
    dive.ToolAdapter(toolkit.NewGenerateImageTool()),
}
```

### 2. Create Specialized Agents
```go
// Code reviewer agent
codeReviewer, err := agent.New(agent.Options{
    Name: "Code Reviewer",
    Instructions: "You are an expert code reviewer...",
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewReadFileTool()),
        dive.ToolAdapter(toolkit.NewWriteFileTool()),
    },
})

// Data analyst agent  
dataAnalyst, err := agent.New(agent.Options{
    Name: "Data Analyst",
    Instructions: "You analyze data and create insights...",
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewReadFileTool()),
        customDataAnalysisTool,
    },
})
```

### 3. Build Multi-Step Workflows
```yaml
Workflows:
  - Name: Full Development Cycle
    Steps:
      - Name: Plan Features
        Agent: Product Manager
      - Name: Write Code
        Agent: Developer  
      - Name: Review Code
        Agent: Code Reviewer
      - Name: Test Code
        Agent: QA Engineer
      - Name: Deploy
        Action: Deploy.ToProduction
```

### 4. Integrate with External Services
```yaml
MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
  - Name: linear
    Type: url
    URL: https://mcp.linear.app/sse
```

Happy building with Dive! 🚀