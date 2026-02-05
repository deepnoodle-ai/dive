# MCP Integration Guide

> **Experimental**: This package is in `experimental/mcp/`. The API may change.

The Model Context Protocol (MCP) enables connections between AI applications and external data sources. Dive provides MCP client support for both HTTP/SSE and stdio servers.

## What is MCP?

MCP provides a standardized interface for accessing external tools and data with built-in authentication:

- Access to specialized tools (GitHub, Linear, Slack, databases)
- Consistent tool interface across services
- Built-in security and authentication

## Configuration

Configure MCP servers in `.dive/settings.json`:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "./workspace"]
    },
    "github": {
      "type": "url",
      "url": "https://mcp.github.com/sse",
      "headers": {
        "Authorization": "Bearer ${GITHUB_TOKEN}"
      }
    }
  }
}
```

## Server Types

**Stdio Servers** (local processes):

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path"]
    }
  }
}
```

**URL Servers** (HTTP/SSE):

```json
{
  "mcpServers": {
    "github": {
      "type": "url",
      "url": "https://mcp.github.com/sse",
      "headers": {
        "Authorization": "Bearer ${GITHUB_TOKEN}"
      }
    }
  }
}
```

## Programmatic Usage

```go
import "github.com/deepnoodle-ai/dive/experimental/mcp"

client, err := mcp.NewStdioClient(mcp.StdioClientOptions{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/path"},
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Discover available tools
tools, err := client.ListTools(ctx)
```

## Authentication

### Environment Variables

```bash
export GITHUB_TOKEN="your-token"
```

### OAuth 2.0

The MCP client supports OAuth 2.0 with PKCE for servers that require it.

## Best Practices

1. Secure tokens using environment variables
2. Limit scope to minimal required permissions
3. Test connections before deployment
4. Agents work even if MCP servers are unavailable

For more details, see the [MCP specification](https://spec.modelcontextprotocol.io/).
