# MCP Integration Guide

> **Experimental**: This package is in `experimental/mcp/`. The API may change.

The Model Context Protocol (MCP) enables connections between AI applications and external data sources. Dive provides MCP client support for stdio, streamable HTTP, and SSE servers.

## What is MCP?

MCP provides a standardized interface for accessing external tools and data with built-in authentication:

- Access to specialized tools (GitHub, Linear, Slack, databases)
- Consistent tool interface across services
- Built-in security and authentication

## Configuration Format

Dive reads Claude Code's `mcpServers` format:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."],
      "env": { "LOG_LEVEL": "${LOG_LEVEL:-info}" }
    },
    "github": {
      "type": "http",
      "url": "https://api.githubcopilot.com/mcp/",
      "headers": { "Authorization": "Bearer ${GITHUB_TOKEN}" }
    },
    "legacy": {
      "type": "sse",
      "url": "https://example.com/sse"
    }
  }
}
```

- `type` is `stdio`, `http` (streamable HTTP), or `sse`. When it is omitted, an entry with a `command` is `stdio` and one with a `url` is `http`.
- `${VAR}` and `${VAR:-default}` are expanded from the environment in `command`, `args`, `env`, `url`, and `headers` when the server connects.
- A stdio server inherits Dive's environment; `env` adds to it.

## Using MCP in the Dive CLI

The CLI (`experimental/cmd/dive`) starts MCP servers at launch, in both interactive and `-p` print mode, and stops them on exit.

### Where servers come from

| Source | Scope |
| --- | --- |
| `"mcpServers"` in `~/.dive/settings.json` | Your servers, in every workspace |
| `.mcp.json` in the workspace directory | Project servers, checked in with the repo; must be approved |
| `--mcp-config <file>` (repeatable) | Servers for this run |

When the same server name appears in more than one source, the later row wins: `--mcp-config` beats `.mcp.json`, which beats `~/.dive/settings.json`. Among several `--mcp-config` files, the last one wins.

### Approving project servers

A `.mcp.json` arrives with the repository, and a stdio server runs a command on your machine. As in Claude Code, Dive starts a project server only once you approve it in `~/.dive/settings.json`:

```json
{
  "enabledMcpjsonServers": ["github", "filesystem"],
  "enableAllProjectMcpServers": false,
  "disabledMcpjsonServers": ["noisy"]
}
```

`disabledMcpjsonServers` wins over both enable settings. Approvals are by server name and apply in every workspace. Passing the file yourself with `--mcp-config .mcp.json` also counts as approval. A project's own `.dive/settings.json` cannot approve its servers.

### Tools and permissions

Each MCP tool is named `mcp__<server>__<tool>`, as in Claude Code. Characters other than letters, digits, `_`, and `-` become `_`. A tool whose full name is longer than 64 characters, or that repeats a name already taken, is skipped, and `/mcp` shows why.

In interactive mode, MCP tools go through the normal permission prompt. A server's `readOnlyHint` does not auto-approve its tools, because the hint is only the server's own claim. To stop being asked, choose the option in the prompt that allows the tool for the rest of the session. Programs that embed Dive can allow a whole server with a rule such as `permission.AllowRule("mcp__github__*")`. With `--dangerously-skip-permissions`, MCP tools run without prompts like every other tool. In `-p` mode, the CLI runs tools without prompts, as it does for built-in tools.

MCP tools are given to the main agent only. Subagents don't get them.

### Startup, failures, and `/mcp`

- Servers connect in parallel. Each one has 30 seconds to start and list its tools. Set `MCP_TIMEOUT` in milliseconds to change this.
- A server that fails does not stop the CLI. In interactive mode, the intro line counts connected, failed, and unapproved servers. In `-p` mode, each problem is printed to stderr as a warning.
- `/mcp` lists each server with its source, transport, and tools, plus any errors, unapproved project servers, and config warnings.

Example:

```bash
dive --mcp-config ./mcp.json -p "Use the github tools to list my open PRs"
```

## Programmatic Usage

```go
import "github.com/deepnoodle-ai/dive/experimental/mcp"

// Stdio server
client, err := mcp.NewClient(&mcp.ServerConfig{
    Name:    "filesystem",
    Type:    "stdio",
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/path"},
})
if err != nil {
    log.Fatal(err)
}
if err := client.Connect(ctx); err != nil {
    log.Fatal(err)
}
defer client.Close()

// Discover tools and adapt them for a Dive agent, named mcp__filesystem__<tool>
mcpTools, err := client.ListTools(ctx)
var tools []dive.Tool
for _, t := range mcpTools {
    tools = append(tools, mcp.NewQualifiedToolAdapter(client, t, "filesystem"))
}
```

`NewToolAdapter` exposes the bare MCP tool name instead.

To load a config file in the format above:

```go
servers, err := mcp.LoadServersFile(".mcp.json") // map[string]*mcp.ServerConfig
```

`ParseServersJSON` does the same for bytes. It returns the valid servers even when some entries are invalid; the error describes the invalid ones.

## Manager

The `Manager` connects to several servers and tracks their tools. It keys tools by their bare MCP names and rejects duplicates across servers:

```go
manager := mcp.NewManager()
err := manager.InitializeServers(ctx, []*mcp.ServerConfig{
    {
        Name:    "filesystem",
        Type:    "stdio",
        Command: "npx",
        Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/path"},
    },
})
defer manager.Close()
```

## Authentication

### Environment Variables

Reference secrets with `${VAR}` in `env`, `headers`, or `url` rather than writing them into config files:

```bash
export GITHUB_TOKEN="your-token"
```

### OAuth 2.0

The MCP client supports OAuth 2.0 with PKCE for HTTP servers that require it. Configure it with the `OAuth` field on `ServerConfig`.

## Best Practices

1. Keep tokens in environment variables.
2. Limit each token to the permissions the server needs.
3. Approve project servers by name, not with `enableAllProjectMcpServers`, unless you trust every repository you open.
4. Agents keep working when an MCP server is unavailable.

For more details, see the [MCP specification](https://spec.modelcontextprotocol.io/).
