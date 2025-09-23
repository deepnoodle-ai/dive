# Claude Code Features Integration Guide

## Overview

This document describes the integration of Claude Code-inspired features into Dive, providing a comprehensive enhancement system for AI agents with memory management, hooks, permissions, subagents, and hierarchical settings.

## Architecture

### Core Components

#### 1. Unified Configuration System (`config/unified_config.go`)

The `UnifiedConfig` struct combines all features into a single configuration system:

```go
type UnifiedConfig struct {
    *Config                              // Base Dive configuration (backwards compatible)
    Settings    *settings.Settings       // Hierarchical settings
    Memory      *MemoryConfig            // Memory system configuration
    Hooks       map[string][]HookConfig  // Hook definitions
    Permissions *PermissionConfig        // Permission rules
    Subagents   map[string]*SubagentConfig // Subagent definitions
    MCPConfig   *MCPConfiguration        // Enhanced MCP configuration
    runtime     *RuntimeState            // Runtime managers (not serialized)
}
```

#### 2. Enhanced Agent (`enhanced/agent.go`)

Wraps standard Dive agents with additional capabilities:

- **Memory Context**: Automatically injects memory from DIVE.md files
- **Hooks**: Executes lifecycle hooks at key points
- **Permissions**: Checks tool permissions before execution
- **Subagents**: Detects and invokes specialized subagents

#### 3. Enhanced Environment (`enhanced/environment.go`)

Extends the base Dive environment with:

```go
type Environment struct {
    BaseEnvironment   BaseEnvironment
    MemoryManager     *memory.Memory
    HookManager       *hooks.HookManager
    PermissionManager *permissions.PermissionManager
    SubagentManager   *subagents.SubagentManager
    SettingsManager   *settings.SettingsManager
    Agents            []dive.Agent
    Tools             map[string]dive.Tool
}
```

#### 4. Agent Factory (`config/factory.go`)

Provides flexible agent creation with enhanced features:

```go
factory, err := NewAgentFactory(ctx, "config.yaml")
agent, err := factory.CreateDefaultAgent()
```

## Feature Descriptions

### Memory System

- **Location**: `memory/memory.go`
- **Purpose**: Maintains context across sessions using DIVE.md files
- **Hierarchy**: Project → User → Enterprise
- **Auto-loading**: Discovers and loads memory files automatically

### Hooks System

- **Location**: `hooks/hooks.go`
- **Events**:
  - `SessionStart/End`: Track conversation lifecycle
  - `UserPromptSubmit`: Pre-process user input
  - `PreToolUse/PostToolUse`: Monitor tool execution
  - `SubagentStop`: Control subagent invocation
- **Actions**: Execute shell commands, modify context, block operations

### Permission System

- **Location**: `permissions/permissions.go`
- **Modes**:
  - `Allow`: Permit without confirmation
  - `Deny`: Block completely
  - `Ask`: Request user confirmation
- **Patterns**: Supports glob patterns for tools and parameters

### Subagents

- **Location**: `subagents/subagents.go`
- **Storage**: Markdown files in `.dive/agents/` or `~/.dive/agents/`
- **Invocation**: Via `@agentname` or `/agent:name` patterns
- **Built-in**: `code-reviewer`, `debugger`, `test-runner`

### Hierarchical Settings

- **Location**: `settings/settings.go`
- **Precedence**: Local → Project → User → Enterprise
- **Format**: JSON files in `.dive/settings/`

## Configuration Example

### Full Enhanced Configuration

```yaml
# enhanced-config.yaml
name: "Enhanced Dive Example"
version: "2.0"

# Memory configuration
memory:
  enabled: true
  autoLoad: true
  customPaths:
    - ./docs/context
    - ./specifications
  excludePatterns:
    - "*.test.md"

# Permission system
permissions:
  defaultMode: normal
  allow:
    - "Read(*.md)"
    - "Bash(ls:*)"
  ask:
    - "Edit(*)"
    - "Write(*)"
  deny:
    - "Read(.env*)"
    - "Bash(rm:*)"

# Hooks
hooks:
  UserPromptSubmit:
    - event: UserPromptSubmit
      actions:
        - type: command
          command: "~/.dive/hooks/prompt-enhancer.sh"
          timeout: 5

  PreToolUse:
    - event: PreToolUse
      matcher: "Bash"
      actions:
        - type: command
          command: "~/.dive/hooks/validate-bash.sh"

# Subagents
subagents:
  code-reviewer:
    name: code-reviewer
    description: "Expert code review specialist"
    tools:
      - Read
      - Grep
    systemPrompt: |
      You are a senior code reviewer.
      Focus on code quality and security.

# Standard Dive configuration
Config:
  DefaultProvider: anthropic
  DefaultModel: claude-3-5-sonnet-20241022
  LogLevel: info

Agents:
  - ID: main
    Name: "Enhanced Assistant"
    Instructions: "You are an AI assistant with enhanced capabilities."
    Model: claude-3-5-sonnet-20241022
```

### Basic Compatible Configuration

For backwards compatibility with the current Dive CLI:

```yaml
# basic-enhanced.yaml
Config:
  DefaultProvider: anthropic
  DefaultModel: claude-3-5-sonnet-20241022
  LogLevel: info

MCPServers:
  - Type: stdio
    Name: filesystem
    Command: npx
    Args:
      - -y
      - "@modelcontextprotocol/server-filesystem"
      - /tmp

Agents:
  - ID: main
    Name: "Enhanced Assistant"
    Instructions: |
      You are an AI assistant with enhanced capabilities.
      Be helpful and concise.
    Provider: anthropic
    Model: claude-3-5-sonnet-20241022
```

## Testing Guide

### 1. Build the CLI

```bash
go build ./cmd/dive
```

### 2. Test with Basic Configuration

```bash
# Test simple query
./dive ask --config examples/basic-enhanced.yaml "What is 2+2?"

# Expected output: "4"
```

### 3. Test Memory System

```bash
# Create a DIVE.md file
echo "# Project Context
This project is about building AI agents.
Important: Always be concise." > DIVE.md

# Memory will be auto-loaded
./dive ask "What is this project about?"
```

### 4. Test Hooks

```bash
# Create a hook script
mkdir -p ~/.dive/hooks
cat > ~/.dive/hooks/log-prompt.sh << 'EOF'
#!/bin/bash
echo "User asked: $DIVE_PROMPT" >> ~/.dive/prompts.log
EOF
chmod +x ~/.dive/hooks/log-prompt.sh

# Create settings with hook
cat > .dive/settings/local.json << 'EOF'
{
  "hooks": {
    "UserPromptSubmit": [{
      "hooks": [{
        "type": "command",
        "command": "~/.dive/hooks/log-prompt.sh"
      }]
    }]
  }
}
EOF

# Test - check ~/.dive/prompts.log after
./dive ask "Hello, how are you?"
```

### 5. Test Subagents

```bash
# Create a subagent
mkdir -p .dive/agents
cat > .dive/agents/calculator.md << 'EOF'
---
name: calculator
description: Math calculation specialist
tools:
  - calculate
model: inherit
---

You are a mathematical calculation specialist.
Show your work step by step.
Always verify your calculations.
EOF

# Invoke subagent (when integrated with CLI)
./dive ask "@calculator What is 15% of 240?"
```

### 6. Test Permissions

```bash
# Create permission settings
cat > .dive/settings/permissions.json << 'EOF'
{
  "permissions": {
    "defaultMode": "normal",
    "deny": ["Write(/etc/*)", "Bash(rm:*)"],
    "ask": ["Write(*)", "Edit(*)"]
  }
}
EOF

# This should be denied
./dive ask "Delete /etc/passwd"

# This should prompt for confirmation
./dive ask "Write a file to test.txt"
```

### 7. Test Enhanced Interactive Mode

```bash
# Start enhanced interactive session
./dive interactive

# In the session, try:
> /memory show
> /settings list
> /subagent list
> @code-reviewer Review the last changes
> /help
```

### 8. Test MCP Integration

```bash
# Config with MCP server (see basic-enhanced.yaml)
./dive ask --config examples/basic-enhanced.yaml "List files in /tmp"

# Should use the MCP filesystem tools
```

## Integration Status

### ✅ Completed

- Unified configuration system
- Enhanced agent wrapper
- Memory management integration
- Hooks system integration
- Permission system integration
- Subagents system integration
- Settings hierarchy
- Factory pattern for agent creation
- Circular dependency resolution

### 🔄 Partially Integrated

- CLI commands (ask command uses fallback for old config)
- Interactive mode (needs CLI update to use new system)

### 📋 TODO

- Update all CLI commands to use `AgentFactory`
- Add CLI flags for enhanced features
- Create migration tool for old configs
- Add comprehensive test suite
- Update documentation

## Code Flow

1. **Configuration Loading**:
   ```
   LoadUnifiedConfig() → Load base config → Load settings →
   Initialize managers → Merge configurations
   ```

2. **Agent Creation**:
   ```
   NewAgentFactory() → LoadUnifiedConfig() → CreateEnhancedEnvironment() →
   CreateAgent() → EnhanceAgent() → Wrapped Agent
   ```

3. **Request Processing**:
   ```
   User Input → Hooks (UserPromptSubmit) → Memory Context Injection →
   Subagent Detection → Permission Check → Tool Execution →
   Hooks (Pre/PostToolUse) → Response
   ```

## File Structure

```
dive/
├── config/
│   ├── unified_config.go    # Unified configuration system
│   └── factory.go            # Agent factory
├── enhanced/
│   ├── agent.go              # Enhanced agent wrapper
│   └── environment.go        # Enhanced environment
├── memory/
│   └── memory.go             # Memory management
├── hooks/
│   └── hooks.go              # Hooks system
├── permissions/
│   └── permissions.go        # Permission system
├── subagents/
│   └── subagents.go          # Subagent management
├── settings/
│   └── settings.go           # Hierarchical settings
└── examples/
    ├── enhanced-config.yaml  # Full configuration example
    └── basic-enhanced.yaml   # Basic compatible config
```

## Troubleshooting

### Circular Import Errors

If you encounter circular import errors:
1. Check that `enhanced` package doesn't import `config`
2. Ensure `subagents` doesn't import `config`
3. Use interfaces to break cycles

### Configuration Not Loading

1. Check YAML syntax (especially @ symbols need quotes)
2. For old CLI, use only recognized fields
3. Check file permissions on config files

### Hooks Not Executing

1. Ensure hook scripts are executable (`chmod +x`)
2. Check hook event names match exactly
3. Verify settings.json syntax

### Memory Not Loading

1. Check DIVE.md file exists in project root
2. Verify no syntax errors in markdown
3. Check exclude patterns aren't blocking files

## Next Steps

1. Complete CLI integration for all commands
2. Add unit tests for enhanced features
3. Create migration documentation
4. Build example hook scripts library
5. Develop standard subagent templates