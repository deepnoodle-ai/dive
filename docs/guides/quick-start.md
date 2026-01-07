# Quick Start Guide

Get up and running with Dive in just a few minutes! This guide will help you create your first AI agent.

## 📋 Table of Contents

- [Before You Begin](#before-you-begin)
- [Your First Agent](#your-first-agent)
- [Adding Tools](#adding-tools)
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
go get github.com/deepnoodle-ai/dive
```

### Step 2: Create Your First Agent

Create `main.go`:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
)

func main() {
    // Create an AI agent
    assistant, err := dive.NewAgent(dive.AgentOptions{
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

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
    "github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
    // Create an agent with file and web tools
    assistant, err := dive.NewAgent(dive.AgentOptions{
        Name: "Research Assistant",
        Instructions: `You are a research assistant who can search the web and work with files.
                      Help users find information and save it to files when requested.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            // File operations
            dive.ToolAdapter(toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})),
            dive.ToolAdapter(toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{})),

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

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

func main() {
    // Create session repository for conversation memory
    sessionRepo := dive.NewMemorySessionRepository()

    // Create agent with memory
    assistant, err := dive.NewAgent(dive.AgentOptions{
        Name: "Chat Assistant",
        Instructions: `You are a helpful AI assistant. Remember our conversation
                      and provide helpful, contextual responses.`,
        Model:             anthropic.New(),
        SessionRepository: sessionRepo,
    })
    if err != nil {
        panic(err)
    }

    fmt.Println("🤖 Dive Chat Assistant")
    fmt.Println("Type 'quit' to exit, 'clear' to start new conversation")
    fmt.Println(strings.Repeat("=", 50))

    scanner := bufio.NewScanner(os.Stdin)
    sessionID := "chat-session-1"

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
            sessionID = "" // Empty triggers auto-generation of new ID
            fmt.Println("🔄 Started new conversation")
            continue
        }

        fmt.Print("🤖 Assistant: ")

        // Stream the response for real-time output
        stream, err := assistant.StreamResponse(
            context.Background(),
            dive.WithSessionID(sessionID),
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
- [Tools Guide](tools.md) - Extend agent capabilities with built-in and custom tools
- [Custom Tools](custom-tools.md) - Build your own tools

### 🚀 Advanced Features

- [LLM Guide](llm-guide.md) - Work with different AI models and providers
- [MCP Integration](mcp-integration.md) - Connect to external services

### 🛠️ Development

- [API Reference](../api/core.md) - Detailed API documentation
- [CLI Reference](../reference.md) - Complete command-line interface guide

### 💬 Get Help

- Join our [Discord community](https://discord.gg/yrcuURWk)
- Check out [GitHub Discussions](https://github.com/deepnoodle-ai/dive/discussions)
- Browse the [GitHub Issues](https://github.com/deepnoodle-ai/dive/issues)

## Common Next Steps

Here are some popular things to try next:

### 1. Add More Tools

```go
// Add web fetching, command execution, image generation
Tools: []dive.Tool{
    dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
        Provider: "google",
    })),
    dive.ToolAdapter(toolkit.NewFetchTool(toolkit.FetchToolOptions{})),
    dive.ToolAdapter(toolkit.NewCommandTool(toolkit.CommandToolOptions{})),
    dive.ToolAdapter(toolkit.NewImageGenerationTool(toolkit.ImageGenerationToolOptions{})),
}
```

### 2. Create Specialized Agents

```go
// Code reviewer agent
codeReviewer, err := dive.NewAgent(dive.AgentOptions{
    Name: "Code Reviewer",
    Instructions: "You are an expert code reviewer...",
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})),
        dive.ToolAdapter(toolkit.NewWriteFileTool(toolkit.WriteFileToolOptions{})),
    },
})

// Data analyst agent
dataAnalyst, err := dive.NewAgent(dive.AgentOptions{
    Name: "Data Analyst",
    Instructions: "You analyze data and create insights...",
    Tools: []dive.Tool{
        dive.ToolAdapter(toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})),
        customDataAnalysisTool,
    },
})
```

Happy building with Dive! 🚀
