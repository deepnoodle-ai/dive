# CLI Reference

The Dive CLI provides a powerful command-line interface for running workflows, chatting with agents, and managing configurations. This reference covers all available commands and options.

## 📋 Table of Contents

- [Installation](#installation)
- [Global Options](#global-options)
- [Commands](#commands)
  - [run](#run)
  - [chat](#chat)
  - [config](#config)
- [Configuration](#configuration)
- [Environment Variables](#environment-variables)
- [Examples](#examples)

## Installation

### Building from Source

```bash
# Clone the repository
git clone https://github.com/diveagents/dive.git
cd dive

# Build and install the CLI
cd cmd/dive
go install .

# Verify installation
dive --help
```

### Using Go Install (when available)

```bash
go install github.com/diveagents/dive/cmd/dive@latest
```

## Global Options

These options are available for all commands:

```bash
dive [global options] command [command options] [arguments...]
```

### Global Flags

- `--help, -h` - Show help information
- `--version, -v` - Show version information

## Commands

### run

Execute a workflow from a YAML configuration file.

#### Usage

```bash
dive run [options] <workflow-file>
```

#### Options

- `--vars, -v <key=value>` - Set workflow input variables (can be used multiple times)
- `--workflow, -w <name>` - Specify which workflow to run (if multiple in file)
- `--env <file>` - Load environment variables from file
- `--dry-run` - Validate workflow without executing it
- `--output, -o <format>` - Output format: `text`, `json`, `yaml` (default: `text`)
- `--verbose` - Enable verbose output
- `--timeout <duration>` - Set execution timeout (default: `30m`)

#### Examples

```bash
# Run a simple workflow
dive run workflow.yaml

# Run with input variables
dive run workflow.yaml --vars "topic=AI research" --vars "format=markdown"

# Run specific workflow from multi-workflow file
dive run workflows.yaml --workflow "Data Analysis"

# Dry run for validation
dive run workflow.yaml --dry-run

# JSON output for programmatic use
dive run workflow.yaml --output json

# With custom timeout
dive run long-workflow.yaml --timeout 1h
```

#### Workflow File Format

```yaml
Name: Research Pipeline
Description: Automated research and analysis

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514
  LogLevel: info

Agents:
  - Name: Researcher
    Instructions: You are a thorough researcher
    Tools: [web_search, fetch]

Workflows:
  - Name: Research
    Inputs:
      - Name: topic
        Type: string
        Required: true
    Steps:
      - Name: Research Topic
        Agent: Researcher
        Prompt: "Research: ${inputs.topic}"
```

### chat

Start an interactive chat session with an agent.

#### Usage

```bash
dive chat [options]
```

#### Options

- `--provider <provider>` - LLM provider: `anthropic`, `openai`, `groq`, `ollama` (required)
- `--model <model>` - Specific model to use (optional, uses provider default)
- `--agent <file>` - Load agent configuration from YAML file
- `--name <name>` - Agent name (default: generated)
- `--instructions <text>` - Agent instructions (default: helpful assistant)
- `--tools <tool1,tool2>` - Comma-separated list of tools to enable
- `--thread <id>` - Continue existing conversation thread
- `--temperature <float>` - Model temperature (0.0-1.0)
- `--max-tokens <int>` - Maximum response tokens
- `--system-prompt <text>` - Custom system prompt
- `--verbose` - Show detailed information
- `--no-tools` - Disable all tools

#### Examples

```bash
# Basic chat with Anthropic
dive chat --provider anthropic --model claude-sonnet-4-20250514

# Chat with tools enabled
dive chat --provider openai --model gpt-4o --tools web_search,read_file,write_file

# Load agent from configuration file
dive chat --agent ./agents/researcher.yaml

# Continue previous conversation
dive chat --provider anthropic --thread user-123

# Creative writing agent
dive chat --provider openai --model gpt-4o --temperature 0.9 --instructions "You are a creative writing assistant"

# Local chat with Ollama
dive chat --provider ollama --model llama3.2:latest

# Chat with custom system prompt
dive chat --provider anthropic --system-prompt "You are a helpful coding assistant specializing in Go"
```

#### Agent Configuration File

```yaml
Name: Research Assistant
Instructions: |
  You are an expert research assistant who helps analyze topics thoroughly.
  Always cite your sources and provide comprehensive analysis.

Tools:
  - web_search
  - fetch
  - read_file
  - write_file

ModelSettings:
  Temperature: 0.3
  MaxTokens: 4000
  ReasoningEffort: high

Config:
  Provider: anthropic
  Model: claude-sonnet-4-20250514
```

#### Interactive Commands

During chat sessions, you can use special commands:

- `/help` - Show available commands
- `/exit` or `/quit` - Exit the chat session
- `/clear` - Clear the current conversation
- `/save <filename>` - Save conversation to file
- `/load <filename>` - Load conversation from file
- `/tools` - Show available tools
- `/model` - Show current model information
- `/thread <id>` - Switch to different thread
- `/reset` - Reset to new conversation

### config

Manage and validate Dive configurations.

#### Usage

```bash
dive config <subcommand> [options] [arguments...]
```

#### Subcommands

##### check

Validate a configuration file for syntax and logical errors.

```bash
dive config check <file>
```

**Options:**
- `--strict` - Enable strict validation mode
- `--schema` - Show configuration schema
- `--format <format>` - Output format: `text`, `json` (default: `text`)

**Examples:**
```bash
# Validate workflow file
dive config check workflow.yaml

# Strict validation
dive config check workflow.yaml --strict

# JSON output for CI/CD
dive config check workflow.yaml --format json
```

##### show

Display configuration information.

```bash
dive config show [options]
```

**Options:**
- `--providers` - Show available LLM providers
- `--models` - Show available models
- `--tools` - Show available tools
- `--schema` - Show configuration schema

**Examples:**
```bash
# Show all available providers and models
dive config show --providers --models

# Show available tools
dive config show --tools

# Show configuration schema
dive config show --schema
```

##### init

Initialize a new configuration file.

```bash
dive config init [options] [filename]
```

**Options:**
- `--type <type>` - Configuration type: `workflow`, `agent` (default: `workflow`)
- `--template <name>` - Use specific template
- `--provider <provider>` - Default LLM provider
- `--interactive, -i` - Interactive configuration

**Examples:**
```bash
# Create basic workflow
dive config init workflow.yaml

# Create agent configuration
dive config init --type agent my-agent.yaml

# Interactive setup
dive config init --interactive

# Use specific template
dive config init --template research-pipeline research.yaml
```

## Configuration

### Configuration File Locations

Dive looks for configuration files in the following order:

1. Command-line specified file
2. `./dive.yaml` (current directory)
3. `~/.dive/config.yaml` (user home directory)
4. `/etc/dive/config.yaml` (system-wide)

### Global Configuration Format

```yaml
# ~/.dive/config.yaml
Default:
  Provider: anthropic
  Model: claude-sonnet-4-20250514
  LogLevel: info
  
Providers:
  anthropic:
    APIKey: ${ANTHROPIC_API_KEY}
    BaseURL: https://api.anthropic.com
    
  openai:
    APIKey: ${OPENAI_API_KEY}
    Organization: ${OPENAI_ORG_ID}
    
  groq:
    APIKey: ${GROQ_API_KEY}
    
Tools:
  web_search:
    google:
      APIKey: ${GOOGLE_SEARCH_API_KEY}
      SearchEngineID: ${GOOGLE_SEARCH_CX}
    kagi:
      APIKey: ${KAGI_API_KEY}
      
  fetch:
    firecrawl:
      APIKey: ${FIRECRAWL_API_KEY}

Defaults:
  timeout: 30m
  max_retries: 3
  temperature: 0.7
```

## Environment Variables

### Required Variables

Set these environment variables for the providers you want to use:

```bash
# LLM Providers
export ANTHROPIC_API_KEY="your-anthropic-key"
export OPENAI_API_KEY="your-openai-key" 
export GROQ_API_KEY="your-groq-key"

# Tool APIs
export GOOGLE_SEARCH_API_KEY="your-google-key"
export GOOGLE_SEARCH_CX="your-search-engine-id"
export FIRECRAWL_API_KEY="your-firecrawl-key"
export KAGI_API_KEY="your-kagi-key"
```

### Optional Variables

```bash
# OpenAI Organization (optional)
export OPENAI_ORG_ID="your-org-id"

# Ollama (if not running locally)
export OLLAMA_HOST="http://your-ollama-server:11434"

# Dive Configuration
export DIVE_LOG_LEVEL="debug"
export DIVE_CONFIG_FILE="/path/to/config.yaml"
export DIVE_DEFAULT_PROVIDER="anthropic"
```

### Loading from .env Files

```bash
# Load environment from file
dive run workflow.yaml --env .env.production

# .env file format
ANTHROPIC_API_KEY=sk-...
OPENAI_API_KEY=sk-...
GOOGLE_SEARCH_API_KEY=AI...
```

## Examples

### Basic Workflow Execution

```bash
# research.yaml
Name: Research Assistant
Agents:
  - Name: Researcher
    Instructions: Research topics thoroughly
    Tools: [web_search]
Workflows:
  - Name: Research
    Inputs:
      - Name: topic
        Type: string
    Steps:
      - Name: Research
        Agent: Researcher
        Prompt: "Research: ${inputs.topic}"

# Run the workflow
dive run research.yaml --vars "topic=quantum computing"
```

### Multi-Step Data Pipeline

```bash
# pipeline.yaml  
Name: Data Analysis Pipeline
Agents:
  - Name: Fetcher
    Tools: [fetch, read_file]
  - Name: Analyzer  
    Tools: [write_file]
Workflows:
  - Name: Analyze
    Steps:
      - Name: Fetch Data
        Agent: Fetcher
        Prompt: "Fetch data from ${inputs.source}"
        Store: raw_data
      - Name: Analyze Data
        Agent: Analyzer  
        Prompt: "Analyze: ${raw_data}"
        Store: analysis
      - Name: Save Results
        Action: Document.Write
        Parameters:
          Path: "results.md"
          Content: "${analysis}"

# Execute pipeline
dive run pipeline.yaml --vars "source=https://api.example.com/data"
```

### Interactive Agent Configuration

```bash
# Create agent interactively
dive config init --type agent --interactive

# Results in agent.yaml:
Name: Code Reviewer
Instructions: |
  You are an expert code reviewer. Analyze code for:
  - Security vulnerabilities
  - Performance issues  
  - Best practices
  - Documentation completeness
Tools:
  - read_file
  - write_file
ModelSettings:
  Temperature: 0.2
  MaxTokens: 4000

# Use the agent
dive chat --agent agent.yaml
```

### Batch Processing

```bash
# Process multiple files
for file in data/*.json; do
  dive run process.yaml --vars "input_file=$file" --output json >> results.jsonl
done

# Parallel execution
find data/ -name "*.json" | xargs -P 4 -I {} dive run process.yaml --vars "input_file={}"
```

### CI/CD Integration

```bash
#!/bin/bash
# ci-workflow.sh

# Validate all workflow files
for workflow in workflows/*.yaml; do
  echo "Validating $workflow..."
  dive config check "$workflow" --format json
  if [ $? -ne 0 ]; then
    echo "Validation failed for $workflow"
    exit 1
  fi
done

# Run test workflows
dive run test-workflow.yaml --vars "env=test" --output json > test-results.json

echo "All workflows validated and tested successfully"
```

### Development Workflow

```bash
# Development with local models (no API keys required)
export DIVE_DEFAULT_PROVIDER=ollama

# Quick agent testing
dive chat --provider ollama --model llama3.2:latest --tools read_file,write_file

# Workflow development with dry-run
dive run workflow.yaml --dry-run --verbose

# Test with different providers
dive run workflow.yaml --vars "provider=anthropic"
dive run workflow.yaml --vars "provider=openai"  
dive run workflow.yaml --vars "provider=groq"
```

## Exit Codes

The CLI uses standard exit codes:

- `0` - Success
- `1` - General error
- `2` - Configuration error
- `3` - Validation error
- `4` - Runtime error
- `130` - Interrupted by user (Ctrl+C)

## Next Steps

- [Quick Start Guide](../guides/quick-start.md) - Get started with Dive
- [Workflow Guide](../guides/workflows.md) - Learn workflow syntax
- [Configuration Reference](configuration.md) - Detailed configuration options
- [Examples](../examples/) - Real-world usage examples