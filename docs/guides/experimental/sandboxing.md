# Sandboxing Guide

> **Experimental**: This package is in `experimental/sandbox/`. The API may change.

Sandboxing provides secure isolation for AI agent tool execution, restricting what sandboxed processes can do at the OS level.

## Overview

When an AI agent executes shell commands, there is a risk of damage to the host system. Sandboxing mitigates this with:

- **Filesystem Isolation**: Restricting write access to the project directory and temp files
- **Network Isolation**: Optionally disabling or restricting outbound network access
- **Process Isolation**: Preventing processes from escaping the sandbox

## Platform Support

| Platform    | Backend                   | Requirement           |
| ----------- | ------------------------- | --------------------- |
| **macOS**   | Seatbelt (`sandbox-exec`) | None (native)         |
| **Linux**   | Docker or Podman          | Docker/Podman daemon  |
| **Windows** | Docker                    | Docker Desktop (WSL2) |

## Configuration

Configure in `.dive/settings.json`:

```json
{
  "sandbox": {
    "enabled": true,
    "allow_network": false,
    "network": {
      "allowed_domains": ["github.com", "proxy.golang.org"]
    },
    "allowed_write_paths": ["/Users/me/.cache/go-build"],
    "seatbelt": {
      "profile": "restrictive"
    }
  }
}
```

### Configuration Options

| Option                    | Type       | Description                                     |
| ------------------------- | ---------- | ----------------------------------------------- |
| `enabled`                 | `bool`     | Enable sandboxing                               |
| `mode`                    | `string`   | `regular` or `auto` (auto-allow sandboxed Bash) |
| `allow_network`           | `bool`     | Permit outbound network (default: false)        |
| `network.allowed_domains` | `[]string` | Domains allowed via proxy                       |
| `work_dir`                | `string`   | Writable directory (defaults to workspace)      |
| `allowed_write_paths`     | `[]string` | Additional writable paths                       |
| `excluded_commands`       | `[]string` | Commands that run outside sandbox               |
| `mount_cloud_credentials` | `bool`     | Mount ~/.aws, ~/.config/gcloud (read-only)      |
| `seatbelt.profile`        | `string`   | `restrictive` or `permissive`                   |

## Programmatic Usage

```go
import (
    "github.com/deepnoodle-ai/dive/experimental/sandbox"
    "github.com/deepnoodle-ai/dive/toolkit"
)

sandboxCfg := &sandbox.Config{
    Enabled:      true,
    WorkDir:      "/path/to/project",
    AllowNetwork: false,
}

bashTool := toolkit.NewBashTool(toolkit.BashToolOptions{
    WorkspaceDir:  "/path/to/project",
    SandboxConfig: sandboxCfg,
})
```

## Network Filtering

When `allowed_domains` is set, Dive automatically starts a built-in proxy to enforce the allowlist. You don't need to configure `http_proxy`/`https_proxy` manually.

## Best Practices

1. Keep `allow_network: false` unless the agent needs network access
2. Use `allowed_domains` over unrestricted `allow_network: true`
3. Mount cloud credentials only when needed (they're read-only)
4. Use `.dive/settings.local.json` for machine-specific paths

## Troubleshooting

- **"Operation not permitted" (macOS)**: Add the path to `allowed_write_paths`
- **Docker/Podman not found**: Ensure the daemon is installed and running
- **Permission issues on Linux**: Enable `docker.enable_user_mapping: true`

For design details, see the [sandboxing design docs](../../design/sandboxing.md).
