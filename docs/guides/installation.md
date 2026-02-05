# Installation Guide

Quick setup guide for Dive.

## Prerequisites

- **Go** 1.25 or later
- **API keys** for LLM providers (optional):
  - [Anthropic](https://console.anthropic.com/)
  - [OpenAI](https://platform.openai.com/api-keys)

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
```

## Verification

Test your setup:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/deepnoodle-ai/dive"
    "github.com/deepnoodle-ai/dive/providers/anthropic"
)

func main() {
    agent, err := dive.NewAgent(dive.AgentOptions{
        Name:         "Test Agent",
        SystemPrompt: "You are a helpful assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal(err)
    }

    response, err := agent.CreateResponse(
        context.Background(),
        dive.WithInput("Hello!"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.OutputText())
}
```

## Troubleshooting

**Import errors:**

- Run `go mod tidy`
- Ensure Go version 1.25+

**API key issues:**

- Verify key format and permissions
- Check environment variable names

## Next Steps

- [Quick Start Guide](quick-start.md) - Build your first agent
- [Agent Guide](agents.md) - Learn about agent capabilities
