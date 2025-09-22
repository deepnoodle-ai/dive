# Built-in Tools Guide

Dive provides a comprehensive set of built-in tools that extend agent capabilities. This guide covers all available built-in tools and how to use them.

## 📋 Table of Contents

- [Built-in Tools](#built-in-tools)
- [Tool Annotations](#tool-annotations)
- [Using Tools with Agents](#using-tools-with-agents)
- [Best Practices](#best-practices)

For creating custom tools, see the [Custom Tools Guide](custom-tools.md).

## Built-in Tools

Dive provides several built-in tools covering common use cases:

### File System Tools

#### Read File Tool

```go
import "github.com/deepnoodle-ai/dive/toolkit"

readTool := toolkit.NewReadFileTool(toolkit.ReadFileToolOptions{})

// Usage in agent
agent, err := agent.New(agent.Options{
    Name: "File Reader",
    Tools: []dive.Tool{readTool},
})
```

**Capabilities:**

- Read text files from the file system
- Supports various file formats (text, JSON, CSV, etc.)
- Handles encoding detection
- Safe path validation

#### Write File Tool

```go
writeTool := dive.ToolAdapter(toolkit.NewWriteFileTool())
```

**Capabilities:**

- Create new files or overwrite existing ones
- Supports various content types
- Directory creation if needed
- Safe path validation

#### List Directory Tool

```go
listTool := dive.ToolAdapter(toolkit.NewListDirectoryTool())
```

**Capabilities:**

- List files and directories
- Filter by file type or pattern
- Include file metadata (size, modification time)
- Recursive directory traversal option

### Web Tools

#### Web Search Tool

```go
searchTool := dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
    Provider: "google", // or "kagi"
}))
```

**Capabilities:**

- Search the web using Google Custom Search or Kagi
- Configurable result count
- Safe search options
- Result ranking and filtering

**Setup:**

```bash
export GOOGLE_SEARCH_API_KEY="your-google-api-key"
export GOOGLE_SEARCH_CX="your-search-engine-id"
# or
export KAGI_API_KEY="your-kagi-api-key"
```

#### Fetch Tool

```go
fetchTool := dive.ToolAdapter(toolkit.NewFetchTool())
```

**Capabilities:**

- Fetch content from web pages
- Extract clean text content using Firecrawl
- Handle various content types
- Respect robots.txt and rate limits

**Setup:**

```bash
export FIRECRAWL_API_KEY="your-firecrawl-api-key"
```

### System Tools

#### Command Tool

```go
commandTool := dive.ToolAdapter(toolkit.NewCommandTool())
```

**Capabilities:**

- Execute system commands
- Capture stdout and stderr
- Set working directory
- Environment variable control
- Timeout protection

**Security Note:** Use with caution in production environments.

#### Text Editor Tool

```go
editorTool := dive.ToolAdapter(toolkit.NewTextEditorTool())
```

**Capabilities:**

- Advanced file editing operations
- View, create, replace, and insert operations
- Line-based editing with context
- Backup creation before changes

### Creative Tools

#### Generate Image Tool

```go
imageTool := dive.ToolAdapter(toolkit.NewGenerateImageTool())
```

**Capabilities:**

- Generate images using OpenAI's image models
- Configurable image size and quality
- Save images to file system
- Various output formats

**Setup:**

```bash
export OPENAI_API_KEY="your-openai-api-key"
```

## Tool Annotations

Tool annotations provide hints about tool behavior to help agents use them effectively:

```go
type ToolAnnotations struct {
    Title           string  // Human-readable tool name
    ReadOnlyHint    bool    // Tool only reads data
    DestructiveHint bool    // Tool may delete/modify data
    IdempotentHint  bool    // Safe to call multiple times
    OpenWorldHint   bool    // Accesses external resources
}
```

### Annotation Examples

```go
// Read-only web search tool
annotations := dive.ToolAnnotations{
    Title:           "Web Search",
    ReadOnlyHint:    true,
    IdempotentHint:  true,
    OpenWorldHint:   true,
}

// Destructive file operations
annotations := dive.ToolAnnotations{
    Title:           "File Writer",
    ReadOnlyHint:    false,
    DestructiveHint: true,
    IdempotentHint:  false,
    OpenWorldHint:   false,
}

// Safe computational tool
annotations := dive.ToolAnnotations{
    Title:           "Calculator",
    ReadOnlyHint:    true,
    IdempotentHint:  true,
    OpenWorldHint:   false,
}
```

## Using Tools with Agents

### Basic Tool Setup

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/agent"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
    "github.com/deepnoodle-ai/dive/toolkit"
)

func main() {
    // Create agent with multiple tools
    researcher, err := agent.New(agent.Options{
        Name: "Research Agent",
        Instructions: `You are a research assistant with access to web search,
                      file operations, and content fetching capabilities.
                      Use these tools to help users with their research needs.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewFetchTool()),
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Use the agent with tools
    response, err := researcher.CreateResponse(
        context.Background(),
        dive.WithInput("Research the latest developments in quantum computing and save a summary to quantum-research.txt"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.Text())

    // Show which tools were used
    toolCalls := response.ToolCalls()
    fmt.Printf("\nTools used: %d\n", len(toolCalls))
    for _, call := range toolCalls {
        fmt.Printf("- %s\n", call.Name)
    }
}
```

### Tool-Specific Agents

```go
// File management specialist
func createFileManager() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "File Manager",
        Instructions: `You are a file management specialist. You can read, write,
                      organize, and analyze files efficiently.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewListDirectoryTool()),
            dive.ToolAdapter(toolkit.NewTextEditorTool()),
        },
    })
}

// Web researcher specialist
func createWebResearcher() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "Web Researcher",
        Instructions: `You are a web research specialist. You can search the web,
                      fetch content from URLs, and analyze web-based information.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewWebSearchTool(toolkit.WebSearchToolOptions{
                Provider: "google",
            })),
            dive.ToolAdapter(toolkit.NewFetchTool()),
        },
    })
}

// System administrator
func createSysAdmin() (*agent.Agent, error) {
    return agent.New(agent.Options{
        Name: "System Administrator",
        Instructions: `You are a system administrator who can execute commands
                      and manage system resources. Be careful with destructive operations.`,
        Model: anthropic.New(),
        Tools: []dive.Tool{
            dive.ToolAdapter(toolkit.NewCommandTool()),
            dive.ToolAdapter(toolkit.NewReadFileTool()),
            dive.ToolAdapter(toolkit.NewWriteFileTool()),
            dive.ToolAdapter(toolkit.NewListDirectoryTool()),
        },
    })
}
```

For creating custom tools and advanced patterns, see the [Custom Tools Guide](custom-tools.md).
