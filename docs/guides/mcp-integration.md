# MCP Integration Guide

The Model Context Protocol (MCP) is an open protocol that enables secure connections between AI applications and external data sources and tools. Dive provides comprehensive MCP support, allowing your agents to connect to any MCP-compatible server and access their tools and resources.

## 📋 Table of Contents

- [What is MCP?](#what-is-mcp)
- [MCP Architecture](#mcp-architecture)
- [Setting up MCP Servers](#setting-up-mcp-servers)
- [Using MCP Tools](#using-mcp-tools)
- [Popular MCP Servers](#popular-mcp-servers)
- [Configuration Options](#configuration-options)
- [Authentication and Security](#authentication-and-security)
- [Error Handling](#error-handling)
- [Custom MCP Servers](#custom-mcp-servers)
- [Best Practices](#best-practices)

## What is MCP?

The Model Context Protocol (MCP) provides:

- **Standardized Interface** - Consistent API for accessing external tools and data
- **Security** - Built-in authentication and permission management
- **Extensibility** - Easy integration with new services and tools
- **Reliability** - Connection management and error recovery
- **Scalability** - Support for multiple concurrent connections

### Benefits of MCP Integration

1. **Access to Specialized Tools** - GitHub, Linear, Slack, databases, and more
2. **No Custom Development** - Use existing MCP servers without writing integration code
3. **Consistent Experience** - Standardized tool interface across all services
4. **Security** - Built-in authentication and permission scoping
5. **Community Ecosystem** - Growing library of MCP server implementations

## MCP Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Dive Agent    │────│  MCP Manager    │────│   MCP Server    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                               │                        │
                               │                ┌───────────────┐
                               └────────────────│ External API  │
                                                └───────────────┘
```

### Components

- **MCP Manager** - Manages connections to multiple MCP servers
- **MCP Server** - External service that implements the MCP protocol
- **Tool Adapter** - Converts MCP tools to Dive's tool interface
- **Authentication** - Handles OAuth, API keys, and other auth methods

## Setting up MCP Servers

### Basic MCP Configuration

```go
package main

import (
    "context"
    "log"
    
    "github.com/diveagents/dive/environment"
    "github.com/diveagents/dive/mcp"
    "github.com/diveagents/dive/agent"
    "github.com/diveagents/dive/llm/providers/anthropic"
)

func main() {
    // Create MCP manager
    mcpManager := mcp.NewManager()
    
    // Configure MCP servers
    mcpServers := []*mcp.ServerConfig{
        {
            Name: "github",
            Type: "url",
            URL:  "https://mcp.github.com/sse",
            AuthorizationToken: os.Getenv("GITHUB_TOKEN"),
        },
        {
            Name: "filesystem",
            Type: "stdio",
            Command: "npx",
            Args: []string{
                "@modelcontextprotocol/server-filesystem",
                "/path/to/workspace",
            },
        },
    }
    
    // Create environment with MCP integration
    env, err := environment.New(environment.Options{
        Name:       "MCP Environment",
        MCPManager: mcpManager,
        MCPServers: mcpServers,
        AutoStart:  true, // Automatically connect to servers
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Create agent that can use MCP tools
    developer, err := agent.New(agent.Options{
        Name: "GitHub Developer",
        Instructions: `You are a developer who can interact with GitHub repositories,
                      manage issues, and work with the filesystem.`,
        Model: anthropic.New(),
        Environment: env,
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Agent automatically has access to all MCP tools
    response, err := developer.CreateResponse(
        context.Background(),
        dive.WithInput("List the open issues in the current repository and create a summary file"),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(response.Text())
}
```

### YAML Configuration

```yaml
# mcp-config.yaml
Name: MCP Development Environment
Description: Environment with GitHub and filesystem access

Config:
  DefaultProvider: anthropic
  DefaultModel: claude-sonnet-4-20250514

MCPServers:
  # GitHub integration
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}
    
  # Linear project management
  - Name: linear
    Type: url  
    URL: https://mcp.linear.app/sse
    AuthorizationToken: ${LINEAR_API_KEY}
    
  # Local filesystem access
  - Name: filesystem
    Type: stdio
    Command: npx
    Args: 
      - "@modelcontextprotocol/server-filesystem"
      - "/Users/developer/projects"
    Environment:
      NODE_ENV: production
      
  # Slack integration
  - Name: slack
    Type: url
    URL: https://mcp.slack.com/sse
    AuthorizationToken: ${SLACK_BOT_TOKEN}
    
Agents:
  - Name: Full Stack Developer
    Instructions: |
      You are a full-stack developer with access to:
      - GitHub for code management
      - Linear for project tracking
      - Slack for team communication
      - Local filesystem for development
    Tools:
      - github:*  # All GitHub tools
      - linear:*  # All Linear tools
      - filesystem:* # All filesystem tools
      - slack:send_message # Specific Slack tool

Workflows:
  - Name: Issue Resolution
    Steps:
      - Name: Get Issue Details
        Agent: Full Stack Developer
        Prompt: "Get details for issue #${inputs.issue_number}"
        Store: issue_details
        
      - Name: Implement Fix
        Agent: Full Stack Developer  
        Prompt: |
          Based on this issue: ${issue_details}
          1. Create a new branch
          2. Implement the fix
          3. Create a pull request
          4. Update the Linear issue
        
      - Name: Notify Team
        Agent: Full Stack Developer
        Prompt: "Send a Slack message about the completed fix"
```

### Loading MCP Configuration

```go
func loadMCPEnvironment() (*environment.Environment, error) {
    cfg, err := config.LoadFromFile("mcp-config.yaml")
    if err != nil {
        return nil, err
    }
    
    env, err := config.BuildEnvironment(cfg)
    if err != nil {
        return nil, err
    }
    
    return env, nil
}
```

## Using MCP Tools

### Discovering Available Tools

```go
func exploreMCPTools(env *environment.Environment) {
    // Get all available MCP tools
    allTools := env.GetMCPTools()
    fmt.Printf("Total MCP tools available: %d\n", len(allTools))
    
    // List tools by server
    servers := []string{"github", "linear", "filesystem", "slack"}
    
    for _, serverName := range servers {
        tools := env.GetMCPToolsByServer(serverName)
        fmt.Printf("\n%s tools: %d\n", serverName, len(tools))
        
        for _, tool := range tools {
            fmt.Printf("  - %s: %s\n", tool.Name(), tool.Description())
        }
    }
    
    // Get server connection status
    serverStatus := env.GetMCPServerStatus()
    fmt.Println("\nServer Status:")
    for server, connected := range serverStatus {
        status := "❌ Disconnected"
        if connected {
            status = "✅ Connected"
        }
        fmt.Printf("  %s: %s\n", server, status)
    }
}
```

### Using Specific MCP Tools

```go
func useMCPToolsDirectly(env *environment.Environment) error {
    // Get specific GitHub tool
    createIssueTool := env.GetMCPTool("github:create_issue")
    if createIssueTool == nil {
        return fmt.Errorf("GitHub create_issue tool not available")
    }
    
    // Use the tool directly
    input := map[string]interface{}{
        "title": "Bug: Authentication not working",
        "body":  "Users cannot login with their credentials",
        "labels": []string{"bug", "authentication"},
    }
    
    inputJSON, _ := json.Marshal(input)
    result, err := createIssueTool.Call(context.Background(), inputJSON)
    if err != nil {
        return err
    }
    
    fmt.Printf("Created issue: %s\n", result.Content[0].Text)
    return nil
}
```

### Agent with Selective MCP Tools

```go
func createSpecializedAgent(env *environment.Environment) (*agent.Agent, error) {
    // Get only specific tools needed for this agent
    githubTools := env.GetMCPToolsByServer("github")
    fileTools := env.GetMCPToolsByServer("filesystem")
    
    // Filter to only needed tools
    var selectedTools []dive.Tool
    for _, tool := range githubTools {
        toolName := tool.Name()
        if strings.Contains(toolName, "issue") || 
           strings.Contains(toolName, "pull_request") ||
           strings.Contains(toolName, "repository") {
            selectedTools = append(selectedTools, tool)
        }
    }
    
    // Add specific filesystem tools
    selectedTools = append(selectedTools, 
        env.GetMCPTool("filesystem:read_file"),
        env.GetMCPTool("filesystem:write_file"),
    )
    
    return agent.New(agent.Options{
        Name: "GitHub Issue Manager",
        Instructions: `You specialize in managing GitHub issues and repositories.
                      You can read and write files, and work with GitHub issues and PRs.`,
        Model: anthropic.New(),
        Tools: selectedTools,
        Environment: env,
    })
}
```

## Popular MCP Servers

### GitHub Server

```bash
# Install GitHub MCP server
npm install -g @modelcontextprotocol/server-github

# Environment variables needed
export GITHUB_TOKEN="your-github-token"
```

```yaml
MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}
```

**Available Tools:**
- `github:create_issue` - Create new issues
- `github:list_issues` - List repository issues
- `github:create_pull_request` - Create pull requests
- `github:list_pull_requests` - List pull requests
- `github:get_repository` - Get repository information
- `github:search_repositories` - Search GitHub repositories

### Linear Server

```bash
# Install Linear MCP server
npm install -g @modelcontextprotocol/server-linear

# Get Linear API key from https://linear.app/settings/api
export LINEAR_API_KEY="your-linear-api-key"
```

```yaml
MCPServers:
  - Name: linear
    Type: url
    URL: https://mcp.linear.app/sse
    AuthorizationToken: ${LINEAR_API_KEY}
```

**Available Tools:**
- `linear:create_issue` - Create new issues
- `linear:list_issues` - List team issues
- `linear:update_issue` - Update issue status/details
- `linear:list_teams` - List organization teams
- `linear:create_project` - Create new projects

### Filesystem Server

```bash
# Install filesystem MCP server
npm install -g @modelcontextprotocol/server-filesystem
```

```yaml
MCPServers:
  - Name: filesystem
    Type: stdio
    Command: npx
    Args: 
      - "@modelcontextprotocol/server-filesystem"
      - "/allowed/directory"  # Restrict access to this directory
```

**Available Tools:**
- `filesystem:read_file` - Read file contents
- `filesystem:write_file` - Write/create files
- `filesystem:list_directory` - List directory contents
- `filesystem:create_directory` - Create directories
- `filesystem:delete_file` - Delete files

### Slack Server

```bash
# Install Slack MCP server
npm install -g @modelcontextprotocol/server-slack

export SLACK_BOT_TOKEN="xoxb-your-bot-token"
```

```yaml
MCPServers:
  - Name: slack
    Type: url
    URL: https://mcp.slack.com/sse
    AuthorizationToken: ${SLACK_BOT_TOKEN}
```

**Available Tools:**
- `slack:send_message` - Send messages to channels
- `slack:list_channels` - List available channels
- `slack:get_channel_history` - Get recent messages
- `slack:upload_file` - Upload files to channels

### Database Servers

```yaml
# PostgreSQL MCP server
MCPServers:
  - Name: postgres
    Type: stdio
    Command: npx
    Args:
      - "@modelcontextprotocol/server-postgres"
    Environment:
      DATABASE_URL: ${POSTGRES_URL}

# SQLite MCP server  
  - Name: sqlite
    Type: stdio
    Command: npx
    Args:
      - "@modelcontextprotocol/server-sqlite"
      - "/path/to/database.sqlite"
```

## Configuration Options

### Server Configuration Types

```go
// HTTP/HTTPS URL-based server
type URLServerConfig struct {
    Name               string            `json:"name"`
    Type               string            `json:"type"` // "url"
    URL                string            `json:"url"`
    AuthorizationToken string            `json:"authorization_token,omitempty"`
    Headers            map[string]string `json:"headers,omitempty"`
    Timeout            time.Duration     `json:"timeout,omitempty"`
}

// Process-based (stdio) server
type StdioServerConfig struct {
    Name        string            `json:"name"`
    Type        string            `json:"type"` // "stdio"
    Command     string            `json:"command"`
    Args        []string          `json:"args,omitempty"`
    WorkingDir  string            `json:"working_dir,omitempty"`
    Environment map[string]string `json:"environment,omitempty"`
}
```

### Advanced Configuration

```yaml
MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}
    Headers:
      User-Agent: "Dive-Agent/1.0"
      X-Custom-Header: "custom-value"
    Timeout: 30s
    
  - Name: custom-service
    Type: stdio
    Command: "/usr/local/bin/my-mcp-server"
    Args: 
      - "--config"
      - "/etc/mcp/config.json"
    WorkingDir: "/var/lib/mcp"
    Environment:
      LOG_LEVEL: "info"
      DATA_PATH: "/var/lib/data"
```

### Connection Management

```go
func configureMCPManager() *mcp.Manager {
    manager := mcp.NewManager(mcp.ManagerOptions{
        MaxConcurrentConnections: 10,
        ConnectionTimeout:        30 * time.Second,
        ReconnectInterval:        5 * time.Second,
        MaxReconnectAttempts:     3,
        
        // Custom error handling
        ErrorHandler: func(serverName string, err error) {
            log.Printf("MCP server %s error: %v", serverName, err)
        },
        
        // Connection lifecycle hooks
        OnConnect: func(serverName string) {
            log.Printf("Connected to MCP server: %s", serverName)
        },
        
        OnDisconnect: func(serverName string) {
            log.Printf("Disconnected from MCP server: %s", serverName)
        },
    })
    
    return manager
}
```

## Authentication and Security

### API Key Authentication

```yaml
MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse
    AuthorizationToken: ${GITHUB_TOKEN}  # Bearer token
    
  - Name: linear
    Type: url
    URL: https://mcp.linear.app/sse
    AuthorizationToken: ${LINEAR_API_KEY}  # API key
```

### OAuth Authentication

```go
func setupOAuthMCPServer() *mcp.ServerConfig {
    // Handle OAuth flow externally, then use access token
    accessToken := getOAuthAccessToken() // Your OAuth implementation
    
    return &mcp.ServerConfig{
        Name:               "oauth-service",
        Type:               "url",
        URL:                "https://api.service.com/mcp",
        AuthorizationToken: accessToken,
    }
}
```

### Custom Authentication Headers

```yaml
MCPServers:
  - Name: custom-api
    Type: url
    URL: https://api.example.com/mcp
    Headers:
      Authorization: "Bearer ${API_TOKEN}"
      X-API-Key: "${API_KEY}"
      X-Client-ID: "${CLIENT_ID}"
```

### Environment Variable Security

```bash
# Use environment variables for sensitive data
export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"
export LINEAR_API_KEY="lin_api_xxxxxxxxxxxxxxxxxxxx"
export SLACK_BOT_TOKEN="xoxb-xxxxxxxxxxxx-xxxxxxxxxxxx-xxxxxxxxxxxxxxxx"

# Or use a .env file (not committed to repo)
echo "GITHUB_TOKEN=your-token-here" >> .env
```

### Permission Scoping

```go
func createRestrictedMCPAgent(env *environment.Environment) (*agent.Agent, error) {
    // Only allow read-only GitHub operations
    allowedTools := []string{
        "github:get_repository",
        "github:list_issues",
        "github:list_pull_requests",
        "github:search_repositories",
    }
    
    var tools []dive.Tool
    for _, toolName := range allowedTools {
        if tool := env.GetMCPTool(toolName); tool != nil {
            tools = append(tools, tool)
        }
    }
    
    return agent.New(agent.Options{
        Name: "Read-Only GitHub Agent",
        Instructions: `You can read GitHub information but cannot create or modify anything.`,
        Model: anthropic.New(),
        Tools: tools,
    })
}
```

## Error Handling

### Connection Error Handling

```go
func handleMCPErrors(env *environment.Environment) {
    // Check server status periodically
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        serverStatus := env.GetMCPServerStatus()
        
        for serverName, connected := range serverStatus {
            if !connected {
                log.Printf("MCP server %s is disconnected, attempting reconnect...", serverName)
                
                // Trigger reconnection
                err := env.ReconnectMCPServer(serverName)
                if err != nil {
                    log.Printf("Failed to reconnect to %s: %v", serverName, err)
                    
                    // Notify agents or fallback to alternative tools
                    handleServerUnavailable(serverName)
                }
            }
        }
    }
}

func handleServerUnavailable(serverName string) {
    switch serverName {
    case "github":
        log.Println("GitHub MCP server unavailable, falling back to direct GitHub API")
        // Implement fallback logic
    case "linear":
        log.Println("Linear MCP server unavailable, tasks will be queued")
        // Queue tasks for later processing
    }
}
```

### Tool Execution Error Handling

```go
func robustMCPToolUse(env *environment.Environment) error {
    createIssueTool := env.GetMCPTool("github:create_issue")
    if createIssueTool == nil {
        return fmt.Errorf("github:create_issue tool not available")
    }
    
    input := map[string]interface{}{
        "title": "Test Issue",
        "body":  "This is a test issue",
    }
    
    // Retry logic for MCP tool calls
    maxRetries := 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        inputJSON, _ := json.Marshal(input)
        result, err := createIssueTool.Call(context.Background(), inputJSON)
        
        if err == nil && !result.IsError {
            fmt.Printf("Issue created successfully: %s\n", result.Content[0].Text)
            return nil
        }
        
        if err != nil {
            log.Printf("Attempt %d failed: %v", attempt+1, err)
        } else {
            log.Printf("Attempt %d returned error: %s", attempt+1, result.Content[0].Text)
        }
        
        if attempt < maxRetries-1 {
            time.Sleep(time.Duration(attempt+1) * time.Second)
        }
    }
    
    return fmt.Errorf("failed to create issue after %d attempts", maxRetries)
}
```

## Custom MCP Servers

### Creating a Custom MCP Server

```python
# custom_mcp_server.py
import asyncio
import json
from mcp import Server, Tool
from mcp.types import TextContent

# Create MCP server instance
server = Server("custom-service")

@server.list_tools()
async def list_tools() -> list[Tool]:
    return [
        Tool(
            name="get_user_data",
            description="Retrieve user data from custom service",
            inputSchema={
                "type": "object", 
                "properties": {
                    "user_id": {"type": "string", "description": "User ID to lookup"}
                },
                "required": ["user_id"]
            }
        ),
        Tool(
            name="send_notification", 
            description="Send notification to user",
            inputSchema={
                "type": "object",
                "properties": {
                    "user_id": {"type": "string"},
                    "message": {"type": "string"}
                },
                "required": ["user_id", "message"]
            }
        )
    ]

@server.call_tool()
async def call_tool(name: str, arguments: dict) -> list[TextContent]:
    if name == "get_user_data":
        user_id = arguments["user_id"]
        # Implement user data retrieval
        user_data = await get_user_from_database(user_id)
        return [TextContent(
            type="text",
            text=json.dumps(user_data, indent=2)
        )]
        
    elif name == "send_notification":
        user_id = arguments["user_id"]
        message = arguments["message"]
        # Implement notification sending
        result = await send_user_notification(user_id, message)
        return [TextContent(
            type="text", 
            text=f"Notification sent to user {user_id}: {result}"
        )]
    
    else:
        raise ValueError(f"Unknown tool: {name}")

if __name__ == "__main__":
    server.run()
```

### Using Custom MCP Server

```yaml
MCPServers:
  - Name: custom-service
    Type: stdio
    Command: python
    Args:
      - "/path/to/custom_mcp_server.py"
    Environment:
      DATABASE_URL: ${DATABASE_URL}
      API_KEY: ${CUSTOM_SERVICE_API_KEY}
```

### Custom MCP Server in Go

```go
// custom_server.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)

func main() {
    // Create MCP server
    s := server.NewStdioServer(
        "custom-service",
        "1.0.0",
        server.WithLogging(log.Default()),
    )
    
    // Register tools
    s.AddTool("database_query", mcp.Tool{
        Name:        "database_query",
        Description: "Execute database queries",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "query": map[string]interface{}{
                    "type":        "string",
                    "description": "SQL query to execute",
                },
            },
            "required": []string{"query"},
        },
    })
    
    // Handle tool calls
    s.SetToolCallHandler(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        switch request.Params.Name {
        case "database_query":
            var args struct {
                Query string `json:"query"`
            }
            
            if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
                return nil, err
            }
            
            // Execute database query
            result, err := executeQuery(args.Query)
            if err != nil {
                return &mcp.CallToolResult{
                    Content: []interface{}{
                        mcp.TextContent{
                            Type: "text",
                            Text: fmt.Sprintf("Query failed: %v", err),
                        },
                    },
                    IsError: true,
                }, nil
            }
            
            return &mcp.CallToolResult{
                Content: []interface{}{
                    mcp.TextContent{
                        Type: "text",
                        Text: result,
                    },
                },
            }, nil
            
        default:
            return nil, fmt.Errorf("unknown tool: %s", request.Params.Name)
        }
    })
    
    // Start server
    if err := s.Serve(); err != nil {
        log.Fatal(err)
    }
}

func executeQuery(query string) (string, error) {
    // Implement database query logic
    return "Query result", nil
}
```

## Best Practices

### 1. Server Selection and Management

```go
// Good: Organize servers by function
func setupProductionMCP() []*mcp.ServerConfig {
    return []*mcp.ServerConfig{
        // Version control
        {Name: "github", Type: "url", URL: "https://mcp.github.com/sse"},
        
        // Project management
        {Name: "linear", Type: "url", URL: "https://mcp.linear.app/sse"},
        
        // Communication
        {Name: "slack", Type: "url", URL: "https://mcp.slack.com/sse"},
        
        // Development environment
        {Name: "filesystem", Type: "stdio", Command: "npx", Args: []string{"@modelcontextprotocol/server-filesystem", "/workspace"}},
    }
}

// Avoid: Too many servers or overlapping functionality
func badMCPSetup() []*mcp.ServerConfig {
    return []*mcp.ServerConfig{
        // Too many similar services
        {Name: "github", Type: "url", URL: "https://mcp.github.com/sse"},
        {Name: "gitlab", Type: "url", URL: "https://mcp.gitlab.com/sse"},
        {Name: "bitbucket", Type: "url", URL: "https://mcp.bitbucket.com/sse"},
        
        // Overlapping functionality
        {Name: "filesystem1", Type: "stdio", Command: "npx", Args: []string{"@modelcontextprotocol/server-filesystem", "/path1"}},
        {Name: "filesystem2", Type: "stdio", Command: "npx", Args: []string{"@modelcontextprotocol/server-filesystem", "/path2"}},
    }
}
```

### 2. Error Handling and Resilience

```go
func resilientMCPSetup(env *environment.Environment) {
    // Monitor server health
    go func() {
        ticker := time.NewTicker(1 * time.Minute)
        defer ticker.Stop()
        
        for range ticker.C {
            status := env.GetMCPServerStatus()
            for server, connected := range status {
                if !connected {
                    log.Printf("MCP server %s is down", server)
                    // Implement alerting or fallback logic
                }
            }
        }
    }()
    
    // Graceful degradation for agents
    agent, _ := agent.New(agent.Options{
        Name: "Resilient Agent",
        Instructions: `You have access to various tools. If a tool is unavailable,
                      explain the limitation and suggest alternatives.`,
        // Don't fail agent creation if MCP servers are down
    })
}
```

### 3. Security and Permissions

```go
// Good: Principle of least privilege
func createSecureMCPAgent(env *environment.Environment) (*agent.Agent, error) {
    // Only grant necessary permissions
    readOnlyTools := []string{
        "github:get_repository",
        "github:list_issues", 
        "linear:list_issues",
        "filesystem:read_file",
    }
    
    var tools []dive.Tool
    for _, toolName := range readOnlyTools {
        if tool := env.GetMCPTool(toolName); tool != nil {
            tools = append(tools, tool)
        }
    }
    
    return agent.New(agent.Options{
        Name: "Read-Only Agent",
        Instructions: "You can read information but cannot make changes.",
        Tools: tools,
    })
}

// Good: Environment-specific configurations
func getEnvironmentMCPServers(env string) []*mcp.ServerConfig {
    base := []*mcp.ServerConfig{
        {Name: "filesystem", Type: "stdio", Command: "npx", Args: []string{"@modelcontextprotocol/server-filesystem"}},
    }
    
    switch env {
    case "development":
        return append(base, &mcp.ServerConfig{
            Name: "github-dev",
            Type: "url",
            URL:  "https://mcp.github.com/sse",
            AuthorizationToken: os.Getenv("GITHUB_DEV_TOKEN"),
        })
        
    case "production":
        return append(base, 
            &mcp.ServerConfig{
                Name: "github-prod",
                Type: "url", 
                URL:  "https://mcp.github.com/sse",
                AuthorizationToken: os.Getenv("GITHUB_PROD_TOKEN"),
            },
            &mcp.ServerConfig{
                Name: "linear-prod",
                Type: "url",
                URL:  "https://mcp.linear.app/sse", 
                AuthorizationToken: os.Getenv("LINEAR_PROD_TOKEN"),
            },
        )
        
    default:
        return base
    }
}
```

### 4. Performance Optimization

```go
// Connection pooling and caching
func optimizedMCPSetup() *mcp.Manager {
    return mcp.NewManager(mcp.ManagerOptions{
        MaxConcurrentConnections: 5,  // Limit concurrent connections
        ConnectionTimeout:        30 * time.Second,
        
        // Enable connection pooling
        PoolConnections: true,
        MaxIdleTime:     10 * time.Minute,
        
        // Cache tool responses where appropriate
        EnableCaching:   true,
        CacheTTL:       5 * time.Minute,
    })
}
```

### 5. Testing MCP Integration

```go
func TestMCPIntegration(t *testing.T) {
    // Use mock MCP servers for testing
    mockServers := []*mcp.ServerConfig{
        {
            Name: "mock-github",
            Type: "mock",
            Tools: []mcp.Tool{
                {
                    Name: "github:create_issue",
                    Handler: func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
                        return &mcp.ToolResult{
                            Content: []mcp.Content{{
                                Type: "text",
                                Text: "Mock issue created: #123",
                            }},
                        }, nil
                    },
                },
            },
        },
    }
    
    env, err := environment.New(environment.Options{
        Name:       "Test Environment",
        MCPServers: mockServers,
    })
    require.NoError(t, err)
    
    // Test MCP tool availability
    tools := env.GetMCPTools()
    assert.Contains(t, tools, "github:create_issue")
    
    // Test tool execution
    tool := env.GetMCPTool("github:create_issue")
    result, err := tool.Call(context.Background(), []byte(`{"title":"Test"}`))
    require.NoError(t, err)
    assert.Contains(t, result.Content[0].Text, "Mock issue created")
}
```

## Next Steps

- [Agent Guide](agents.md) - Learn how agents use MCP tools
- [Environment Guide](environment.md) - Manage MCP servers in environments
- [Tools Guide](tools.md) - Understand the tool system
- [API Reference](../api/core.md) - MCP integration APIs