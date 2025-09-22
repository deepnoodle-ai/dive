# MCP Integration Guide

The Model Context Protocol (MCP) enables secure connections between AI applications and external data sources. Dive provides comprehensive MCP support.

## What is MCP?

MCP provides a standardized interface for accessing external tools and data with built-in authentication and permission management.

Benefits:

- Access to specialized tools (GitHub, Linear, Slack, databases)
- No custom integration code needed
- Consistent tool interface across services
- Built-in security and authentication

## Quick Setup

### 1. Configure MCP Servers

```yaml
# config.yaml
MCPServers:
  - Name: github
    Type: url
    URL: https://mcp.github.com/sse  # Example URL - replace with actual MCP server
    AuthorizationToken: ${GITHUB_TOKEN}

  - Name: filesystem
    Type: stdio
    Command: npx
    Args: ["@modelcontextprotocol/server-filesystem", "./workspace"]
```

### 2. Use in Code

```go
import (
    "context"

    "github.com/deepnoodle-ai/dive/config"
)

// Load configuration with MCP servers
cfg, err := config.LoadDirectory("./")
if err != nil {
    return err
}

// Create environment (MCP servers auto-connect)
env, err := config.NewEnvironment(ctx, config.EnvironmentOpts{
    Config: cfg,
})
if err != nil {
    return err
}

// MCP tools are now available to all agents through the environment
```

## Server Types

**URL Servers** (HTTP/SSE):

```yaml
- Name: github
  Type: url
  URL: https://mcp.github.com/sse  # Example URL - replace with actual MCP server
  AuthorizationToken: ${GITHUB_TOKEN}
```

**Stdio Servers** (Local processes):

```yaml
- Name: filesystem
  Type: stdio
  Command: npx
  Args: ["@modelcontextprotocol/server-filesystem", "/path"]
```

## Popular MCP Servers

- **GitHub** - Repository management, issues, PRs
- **Linear** - Project management, issue tracking
- **Slack** - Team communication, notifications
- **PostgreSQL** - Database queries and operations
- **Filesystem** - File operations in allowed directories
- **Google Drive** - Document access and management

## Authentication

### Environment Variables

```bash
export GITHUB_TOKEN="your-token"
export LINEAR_API_KEY="your-key"
export SLACK_BOT_TOKEN="your-token"
```

### OAuth (for supported servers)

```yaml
- Name: google-drive
  Type: url
  URL: https://mcp.google.com/drive
  OAuth:
    ClientID: ${GOOGLE_CLIENT_ID}
    Scopes: ["drive.readonly"]
```

## Error Handling

MCP connections are managed automatically:

- Failed connections are retried
- Unavailable servers don't block agent creation
- Connection status is logged

## Best Practices

1. **Secure tokens** - Use environment variables for API keys
2. **Limit scope** - Configure minimal required permissions
3. **Test connections** - Verify MCP servers before deployment
4. **Monitor logs** - Watch for connection issues
5. **Graceful degradation** - Agents work even if MCP servers are unavailable

## Troubleshooting

**Server won't connect:**

- Check token permissions
- Verify server URL/command
- Check network connectivity

**Tools not available:**

- Ensure server is connected
- Check server tool list
- Verify agent has access to MCP tools

For more details, see the [MCP specification](https://spec.modelcontextprotocol.io/).
