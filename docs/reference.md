# Dive Reference

Complete reference for Dive CLI commands and configuration.

## CLI Commands

### Installation

```bash
# Install from source
git clone https://github.com/deepnoodle-ai/dive.git
cd dive/cmd/dive
go install .
```

### Commands

#### ask

Ask a question and get an immediate response.

```bash
dive ask [message] [options]

Options:
  --provider         LLM provider (anthropic, openai, groq, grok, ollama, google)
  --model           Model name
  --config          Configuration file path
  --agent           Agent name from config
  --system-prompt   System prompt override
  --thread-id       Thread ID for conversation continuity
```

#### chat

Start an interactive chat session with an agent.

```bash
dive chat [options]

Options:
  --provider    LLM provider (anthropic, openai, groq, grok, ollama, google)
  --model       Model name
  --config      Configuration file path
  --agent       Agent name from config
  --thread-id   Thread ID for conversation continuity
```

#### classify

Classify text into categories with confidence scores.

```bash
dive classify [text] --labels label1,label2,label3

Options:
  --labels      Comma-separated list of classification labels
  --provider    LLM provider
  --model       Model name
  --threshold   Confidence threshold (0.0-1.0)
```

#### diff

Generate semantic diff analysis between two files or texts.

```bash
dive diff <file1> <file2> [options]

Options:
  --explain-changes    Provide detailed explanation of changes
  --provider           LLM provider
  --model              Model name
```

#### compare

Compare outputs between different LLM providers.

```bash
dive compare --provider1 anthropic --provider2 openai --input "prompt"

Options:
  --provider1   First LLM provider
  --provider2   Second LLM provider
  --model1      First model name
  --model2      Second model name
  --input       Input prompt to compare
```

#### extract

Extract structured data from text.

```bash
dive extract [text] --schema schema.json

Options:
  --schema      JSON schema for extraction
  --provider    LLM provider
  --model       Model name
```

#### summarize

Summarize text content.

```bash
dive summarize [text] [options]

Options:
  --provider    LLM provider
  --model       Model name
  --length      Summary length (short, medium, long)
```

#### embed

Generate text embeddings.

```bash
dive embed [text] [options]

Options:
  --provider    Embedding provider (openai, google)
  --model       Embedding model name
```

#### image

Generate or analyze images.

```bash
dive image generate --prompt "description"
dive image analyze --file image.jpg

Options:
  --prompt      Text prompt for image generation
  --file        Image file to analyze
  --provider    Image provider
```

#### threads

Manage conversation threads.

```bash
dive threads list
dive threads show <thread-id>
dive threads delete <thread-id>
```

#### mcp

Manage Model Context Protocol servers.

```bash
dive mcp list
dive mcp auth <server-name>
```

#### config

Manage configuration files.

```bash
dive config check <file>    # Validate configuration file
```

#### Global Options

- `--help, -h` - Show help
- `--version, -v` - Show version

## Configuration Files

### Basic Structure

```yaml
Name: My Environment
Description: Production AI environment

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info
  ConfirmationMode: if-destructive

Agents:
  - Name: Assistant
    Instructions: You are a helpful assistant.
    Tools: [web_search, read_file]

MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}
```

### Environment Configuration

```yaml
Config:
  DefaultProvider: anthropic # Default LLM provider
  DefaultModel: claude-sonnet-4 # Default model
  LogLevel: info # debug, info, warn, error
  ConfirmationMode: if-destructive # always, if-destructive, never
  MaxConcurrency: 10 # Max parallel operations
```

### Agent Configuration

```yaml
Agents:
  - Name: Research Assistant
    ID: researcher # Optional unique ID
    Goal: Research and analyze topics
    Instructions: |
      You are an expert researcher who provides
      thorough analysis of any topic.
    Provider: anthropic # Override default
    Model: claude-sonnet-4 # Override default
    Tools: # Available tools
      - web_search
      - read_file
      - write_file
    IsSupervisor: false # Can delegate to other agents
    Subordinates: [] # Available subordinate agents
    DateAwareness: auto # Include current date
    SystemPrompt: "" # Additional system instructions

    ModelSettings: # Model configuration
      Temperature: 0.7
      MaxTokens: 4000
      ReasoningBudget: 5000
      Caching: true
      ParallelToolCalls: true
      ToolChoice: auto

    Context: # Static context files
      - Type: file
        Path: ./context/guidelines.md
      - Type: file
        Path: ./context/examples.txt
```

### MCP Server Configuration

```yaml
MCPServers:
  # URL-based server (HTTP/SSE)
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}

  # Stdio server (local process)
  - Name: filesystem
    Type: stdio
    Command: npx
    Args:
      - "@modelcontextprotocol/server-filesystem"
      - "./workspace"
    Env:
      FILESYSTEM_ROOT: ./workspace
```

### Provider Configuration

```yaml
Config:
  Providers:
    - Name: anthropic
      Caching: true
      Features: [tool_calling, streaming]
      RequestHeaders:
        X-Custom-Header: value

    - Name: openai
      Features: [reasoning, tool_calling]
```

### Tool Configuration

```yaml
Tools:
  - Name: web_search
    Config:
      Provider: google # google, kagi
      MaxResults: 10

  - Name: fetch
    Config:
      Provider: firecrawl
      Format: markdown
```

## Environment Variables

### Required API Keys

```bash
# LLM Providers
export ANTHROPIC_API_KEY="your-key"
export OPENAI_API_KEY="your-key"
export GROQ_API_KEY="your-key"

# Tools
export GOOGLE_SEARCH_API_KEY="your-key"
export GOOGLE_SEARCH_CX="your-cx-id"
export FIRECRAWL_API_KEY="your-key"
export KAGI_API_KEY="your-key"

# MCP Servers
export GITHUB_TOKEN="your-token"
export LINEAR_API_KEY="your-key"
export SLACK_BOT_TOKEN="your-token"
```

### Optional Settings

```bash
export DIVE_LOG_LEVEL="info"
export DIVE_CONFIG_PATH="./dive.yaml"
export DIVE_CONFIRMATION_MODE="if-destructive"
```

## File Discovery

Dive looks for configuration files in this order:

1. `--config` flag path
2. `./dive.yaml`
3. `./dive.yml`
4. `./config.yaml`
5. `./config.yml`
6. `~/.dive/config.yaml`

## Examples

### Simple Agent Config

```yaml
Agents:
  - Name: Assistant
    Instructions: You are a helpful assistant.
    Provider: anthropic
    Model: claude-sonnet-4-20250514
```

### Multi-Agent Setup

```yaml
Agents:
  - Name: Supervisor
    Instructions: You coordinate work between team members.
    IsSupervisor: true
    Subordinates: [Researcher, Writer]
    Tools: [assign_work]

  - Name: Researcher
    Instructions: You research topics thoroughly.
    Tools: [web_search, read_file]

  - Name: Writer
    Instructions: You write clear, engaging content.
    Tools: [write_file, text_editor]
```

### Development Config

```yaml
Config:
  DefaultProvider: ollama
  DefaultModel: llama3.1
  LogLevel: debug
  ConfirmationMode: always

Agents:
  - Name: DevAssistant
    Instructions: You help with development tasks.
    Tools: [read_file, write_file, command]
```
