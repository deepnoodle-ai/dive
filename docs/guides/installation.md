# Installation Guide

This guide covers different ways to install and set up Dive for development and production use.

## 📋 Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Methods](#installation-methods)
- [Environment Setup](#environment-setup)
- [Verification](#verification)
- [Development Setup](#development-setup)
- [Docker Setup](#docker-setup)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### System Requirements

- **Go**: Version 1.23.2 or later
- **Operating System**: Linux, macOS, or Windows
- **Memory**: Minimum 512MB RAM (2GB+ recommended)
- **Storage**: At least 100MB free space

### API Keys (Optional)

Set up API keys for the LLM providers and tools you plan to use:

- **Anthropic**: [Get API key](https://console.anthropic.com/)
- **OpenAI**: [Get API key](https://platform.openai.com/api-keys)
- **Groq**: [Get API key](https://console.groq.com/keys)
- **Google Search**: [Get API key](https://developers.google.com/custom-search/v1/overview)
- **Firecrawl**: [Get API key](https://firecrawl.com/)

## Installation Methods

### Method 1: Go Install (Recommended)

Install Dive as a Go module in your project:

```bash
# Initialize new Go module (if needed)
go mod init your-project-name

# Add Dive dependency
go get github.com/diveagents/dive

# Install CLI (optional)
go install github.com/diveagents/dive/cmd/dive@latest
```

### Method 2: Clone and Build

For development or to use the latest features:

```bash
# Clone the repository
git clone https://github.com/diveagents/dive.git
cd dive

# Build the library
go build ./...

# Build and install CLI
cd cmd/dive
go build -o dive .
go install .

# Verify installation
dive --version
```

### Method 3: Download Binaries

Pre-built binaries will be available for download:

```bash
# Download latest release (when available)
curl -L https://github.com/diveagents/dive/releases/latest/download/dive-$(uname -s)-$(uname -m).tar.gz | tar xz

# Move to PATH
sudo mv dive /usr/local/bin/
```

## Environment Setup

### Required Environment Variables

Set up API keys for the services you plan to use:

```bash
# LLM Providers
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export OPENAI_API_KEY="your-openai-api-key"
export GROQ_API_KEY="your-groq-api-key"

# Tool APIs
export GOOGLE_SEARCH_API_KEY="your-google-search-api-key"
export GOOGLE_SEARCH_CX="your-search-engine-id"
export FIRECRAWL_API_KEY="your-firecrawl-api-key"
export KAGI_API_KEY="your-kagi-api-key"

# Optional: Ollama (if not running locally)
export OLLAMA_HOST="http://localhost:11434"
```

### Using .env Files

Create a `.env` file in your project directory:

```bash
# .env file
ANTHROPIC_API_KEY=your-anthropic-api-key
OPENAI_API_KEY=your-openai-api-key
GROQ_API_KEY=your-groq-api-key
GOOGLE_SEARCH_API_KEY=your-google-search-api-key
GOOGLE_SEARCH_CX=your-search-engine-id
FIRECRAWL_API_KEY=your-firecrawl-api-key
```

Load the environment variables:

```bash
# Using source
source .env

# Or using a tool like direnv
echo "source .env" > .envrc
direnv allow
```

### Shell Configuration

Add to your shell profile (`.bashrc`, `.zshrc`, etc.):

```bash
# Add to ~/.bashrc or ~/.zshrc
export ANTHROPIC_API_KEY="your-anthropic-api-key"
export OPENAI_API_KEY="your-openai-api-key"
# ... other keys

# Reload your shell
source ~/.bashrc  # or ~/.zshrc
```

## Verification

### Test Go Library

Create a simple test file to verify the installation:

```go
// test-dive.go
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
    // Test basic agent creation
    assistant, err := agent.New(agent.Options{
        Name:         "Test Assistant",
        Instructions: "You are a test assistant.",
        Model:        anthropic.New(),
    })
    if err != nil {
        log.Fatal("Failed to create agent:", err)
    }

    fmt.Printf("✅ Successfully created agent: %s\n", assistant.Name())

    // Test basic interaction (requires ANTHROPIC_API_KEY)
    if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
        response, err := assistant.CreateResponse(
            context.Background(),
            dive.WithInput("Hello! Can you confirm you're working?"),
        )
        if err != nil {
            log.Printf("❌ API test failed: %v", err)
        } else {
            fmt.Printf("✅ API test successful: %s\n", response.Text()[:50])
        }
    } else {
        fmt.Println("⚠️  ANTHROPIC_API_KEY not set, skipping API test")
    }
}
```

Run the test:

```bash
go run test-dive.go
```

### Test CLI

Verify the CLI installation:

```bash
# Check version
dive --version

# Test configuration validation
echo 'Name: Test
Config:
  DefaultProvider: anthropic
Agents:
  - Name: Test Agent
    Instructions: Test instructions
Workflows:
  - Name: Test Workflow
    Steps:
      - Name: Test Step
        Agent: Test Agent
        Prompt: Test prompt' > test-workflow.yaml

dive config check test-workflow.yaml
```

## Development Setup

### IDE Configuration

#### VS Code Setup

Install recommended extensions:

```bash
# Install Go extension
code --install-extension golang.Go

# Install YAML extension for configuration files  
code --install-extension redhat.vscode-yaml
```

Create `.vscode/settings.json`:

```json
{
    "go.testFlags": ["-v"],
    "go.testTimeout": "60s",
    "go.lintTool": "golangci-lint",
    "go.formatTool": "goimports",
    "yaml.schemas": {
        "https://raw.githubusercontent.com/diveagents/dive/main/config/schema.json": [
            "*.dive.yaml",
            "workflow*.yaml",
            "agent*.yaml"
        ]
    }
}
```

#### GoLand/IntelliJ Setup

1. Install Go plugin
2. Set GOPATH and GOROOT
3. Enable Go modules support
4. Configure code style to match project

### Git Hooks (Optional)

Set up pre-commit hooks for code quality:

```bash
# Install pre-commit
pip install pre-commit

# Set up hooks (if .pre-commit-config.yaml exists)
pre-commit install
```

### Local Ollama Setup

For local LLM inference:

```bash
# Install Ollama
curl -fsSL https://ollama.ai/install.sh | sh

# Download a model
ollama pull llama3.2:latest

# Verify it's running
ollama list
```

## Docker Setup

### Using Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  dive-app:
    build: .
    environment:
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - GROQ_API_KEY=${GROQ_API_KEY}
    volumes:
      - ./workflows:/app/workflows
      - ./data:/app/data
    ports:
      - "8080:8080"
    depends_on:
      - ollama
      
  ollama:
    image: ollama/ollama:latest
    ports:
      - "11434:11434"
    volumes:
      - ollama-data:/root/.ollama
    environment:
      - OLLAMA_HOST=0.0.0.0

volumes:
  ollama-data:
```

Create `Dockerfile`:

```dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o dive ./cmd/dive

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/dive .
COPY --from=builder /app/examples ./examples

EXPOSE 8080
CMD ["./dive"]
```

Run with Docker:

```bash
# Build and start
docker-compose up --build

# Run CLI commands in container
docker-compose exec dive-app ./dive --help
```

### Standalone Docker

```bash
# Build image
docker build -t dive .

# Run container
docker run -it --rm \
  -e ANTHROPIC_API_KEY="$ANTHROPIC_API_KEY" \
  -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  -v $(pwd)/workflows:/app/workflows \
  dive
```

## Troubleshooting

### Common Issues

#### Go Module Issues

```bash
# Problem: "module not found" errors
# Solution: Initialize Go modules
go mod init your-project
go mod tidy

# Problem: Version conflicts
# Solution: Update to latest versions
go get -u github.com/diveagents/dive
go mod tidy
```

#### API Key Issues

```bash
# Problem: "API key not found" errors
# Solution: Verify environment variables
echo $ANTHROPIC_API_KEY
env | grep API_KEY

# Problem: Invalid API key format
# Solution: Check key format and permissions
curl -H "x-api-key: $ANTHROPIC_API_KEY" https://api.anthropic.com/v1/messages
```

#### CLI Issues

```bash
# Problem: Command not found
# Solution: Check PATH and reinstall
echo $PATH
which dive
go install github.com/diveagents/dive/cmd/dive@latest

# Problem: Permission denied
# Solution: Check file permissions
ls -la $(which dive)
chmod +x $(which dive)
```

#### Network Issues

```bash
# Problem: Connection timeouts
# Solution: Check network connectivity and proxies
curl -I https://api.anthropic.com
curl -I https://api.openai.com

# Problem: Proxy issues
# Solution: Configure Go proxy settings
export GOPROXY=direct
export GOSUMDB=off
```

### Getting Help

1. **Check Documentation**: Review relevant guides and examples
2. **Search Issues**: Look for similar issues on GitHub
3. **Enable Debug Logging**: Set `DIVE_LOG_LEVEL=debug`
4. **Community Support**: Join our [Discord community](https://discord.gg/yrcuURWk)
5. **File an Issue**: Create a detailed bug report on GitHub

### Debug Configuration

Enable detailed logging:

```bash
export DIVE_LOG_LEVEL=debug
export DIVE_LOG_FORMAT=json

# Run with debug output
dive run workflow.yaml --verbose
```

Create debug configuration:

```go
// Enable debug logging in code
import "github.com/diveagents/dive/slogger"

logger := slogger.New(slogger.Options{
    Level:  "debug",
    Format: "text",
    Output: os.Stderr,
})

agent, err := agent.New(agent.Options{
    Name:   "Debug Agent",
    Logger: logger,
    // ... other options
})
```

## Next Steps

- [Quick Start Guide](quick-start.md) - Build your first agent
- [Agent Guide](agents.md) - Learn about creating agents  
- [Workflow Guide](workflows.md) - Create automated processes
- [Examples](../examples/basic.md) - Explore code examples