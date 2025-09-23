# Installation Guide

Quick setup guide for Dive.

## Prerequisites

- **Go** 1.23.2 or later
- **API keys** for LLM providers (optional):
  - [Anthropic](https://console.anthropic.com/)
  - [OpenAI](https://platform.openai.com/api-keys)
  - [Groq](https://console.groq.com/keys)

## Installation

### As a Library

```bash
# Initialize Go module
go mod init your-project

# Add Dive dependency
go get github.com/deepnoodle-ai/dive
```

### CLI Tool

```bash
# Install from source
git clone https://github.com/deepnoodle-ai/dive.git
cd dive/cmd/dive
go install .
```

## Environment Setup

Set your API keys:

```bash
export ANTHROPIC_API_KEY="your-key-here"
export OPENAI_API_KEY="your-key-here"
export GROQ_API_KEY="your-key-here"

# For tools (optional)
export GOOGLE_SEARCH_API_KEY="your-key"
export GOOGLE_SEARCH_CX="your-cx-id"
export FIRECRAWL_API_KEY="your-key"
```

## Verification

Test your setup:

```go
package main

import (
    "context"
    "fmt"
    "github.com/deepnoodle-ai/dive/agent"
    "github.com/deepnoodle-ai/dive/llm/providers/anthropic"
    "github.com/deepnoodle-ai/dive"
)

func main() {
    agent, err := agent.New(agent.Options{
        Name:         "Test Agent",
        Instructions: "You are a helpful assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        panic(err)
    }

    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("Hello!"),
    )
    if err != nil {
        panic(err)
    }

    fmt.Println(response.Text())
}
```

## Troubleshooting

**Import errors:**

- Run `go mod tidy`
- Ensure Go version 1.23.2+

**API key issues:**

- Verify key format and permissions
- Check environment variable names

**Network issues:**

- Configure proxy if needed
- Check firewall settings

## Next Steps

- [Quick Start Guide](quick-start.md) - Build your first agent
- [Agent Guide](agents.md) - Learn about agent capabilities
