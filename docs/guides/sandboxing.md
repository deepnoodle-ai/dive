# Sandboxing Guide

Sandboxing in Dive provides secure isolation for AI agent tool execution. By restricting what sandboxed processes can do at the OS level, Dive adds a critical layer of defense-in-depth against malicious or buggy commands.

## Overview

When an AI agent executes shell commands (e.g., via the `Bash` tool), there is a risk that commands could damage the host system, exfiltrate data, or cause unintended side effects. Sandboxing mitigates these risks by:

- **Filesystem Isolation**: Restricting write access to the project directory and temporary files.
- **Network Isolation**: Optionally disabling or restricting outbound network access.
- **Process Isolation**: Preventing processes from escaping the sandbox.
- **Root Filesystem Protection**: Ensuring the system's core files cannot be modified.

## Platform Support

Dive uses native or standard container-based technologies for sandboxing depending on your operating system:

| Platform    | Backend                   | Requirement           |
| :---------- | :------------------------ | :-------------------- |
| **macOS**   | Seatbelt (`sandbox-exec`) | None (native)         |
| **Linux**   | Docker or Podman          | Docker/Podman daemon  |
| **Windows** | Docker                    | Docker Desktop (WSL2) |

### macOS (Seatbelt)

On macOS, Dive uses **Seatbelt**, the same technology that powers the Mac App Store sandbox. It is lightweight, native, and does not require any additional software like Docker.

Dive dynamically generates Seatbelt profiles at runtime, ensuring that:

- Writes are only allowed to your project directory (`WorkDir`) and `/tmp`.
- Common system tools (`ps`, `top`, `pgrep`) continue to work via specialized permissions.
- Network access is toggled based on your configuration.

### Linux & Windows (Docker/Podman)

On Linux and Windows, Dive uses containerization. It prefers **Podman** on Linux (for rootless execution) but falls back to **Docker** if Podman is not found.

Key container features:

- **Read-Only Root**: The container's system files are mounted as read-only.
- **Volume Mounts**: Only your project directory, `/tmp`, and specific allowed paths are mounted read-write.
- **User Mapping**: Optional mapping of your host UID/GID into the container to prevent file permission issues.
- **Automatic Cleanup**: Containers are automatically removed (`--rm`) and cleaned up via CID files even if Dive is interrupted.

## Configuration

Sandboxing is configured through the `sandbox` section in your project's `.dive/settings.json` or `.dive/settings.local.json`.

### Basic Configuration

To enable the sandbox with default settings (restrictive, no network):

```json
{
  "sandbox": {
    "enabled": true
  }
}
```

### Advanced Configuration

You can customize the sandbox behavior for more complex workflows:

```json
{
  "sandbox": {
    "enabled": true,
    "allow_network": false,
    "mount_cloud_credentials": true,
    "allowed_write_paths": ["/Users/me/.cache/go-build", "/Users/me/.npm"],
    "docker": {
      "image": "golang:1.23-alpine",
      "enable_user_mapping": true
    },
    "seatbelt": {
      "profile": "restrictive"
    }
  }
}
```

### Configuration Options

| Option                       | Type       | Description                                                                            |
| :--------------------------- | :--------- | :------------------------------------------------------------------------------------- |
| `enabled`                    | `bool`     | Enables or disables sandboxing.                                                        |
| `allow_network`              | `bool`     | Permits outbound network access. Default: `false`.                                     |
| `work_dir`                   | `string`   | The directory where the agent can write. Defaults to current workspace.                |
| `allowed_write_paths`        | `[]string` | Additional host paths the agent is allowed to write to.                                |
| `mount_cloud_credentials`    | `bool`     | Mounts `~/.aws`, `~/.config/gcloud`, and `GOOGLE_APPLICATION_CREDENTIALS` (read-only). |
| `docker.image`               | `string`   | The container image to use. Default: `ubuntu:22.04`.                                   |
| `docker.enable_user_mapping` | `bool`     | (Linux) Maps host UID/GID to container user.                                           |
| `seatbelt.profile`           | `string`   | `restrictive` or `permissive`. Default: `restrictive`.                                 |

## Programmatic Usage

If you are using Dive as a library, you can configure the sandbox when creating the `BashTool`:

```go
import (
    "github.com/deepnoodle-ai/dive/sandbox"
    "github.com/deepnoodle-ai/dive/toolkit"
)

sandboxCfg := &sandbox.Config{
    Enabled:      true,
    WorkDir:      "/path/to/project",
    AllowNetwork: false,
    MountCloudCredentials: true,
}

bashTool := toolkit.NewBashTool(toolkit.BashToolOptions{
    WorkspaceDir:  "/path/to/project",
    SandboxConfig: sandboxCfg,
})
```

## Best Practices

1. **Use Restrictive by Default**: Keep `allow_network: false` unless the agent specifically needs to download dependencies or call external APIs.
2. **Mount Credentials Sparingly**: Only set `mount_cloud_credentials: true` if the agent needs to run `aws` or `gcloud` commands. These are mounted as **Read-Only** for safety.
3. **Specify Tool-Specific Images**: If your project is in Go, Node, or Python, use a Docker image that already contains the necessary compilers (`golang:1.23`, `node:20`, etc.) to avoid the agent trying to install tools at runtime.
4. **Use `.dive/settings.local.json`**: For machine-specific paths (like your home directory's cache), use `settings.local.json` so these paths don't get committed to the project's repository.

## Troubleshooting

### "Operation not permitted" (macOS)

This usually means the agent tried to write to a path outside the `WorkDir` or `/tmp`. Check if you need to add the path to `allowed_write_paths`.

### Docker/Podman not found

Ensure the Docker or Podman daemon is installed and running. If using Docker on Linux, ensure your user is in the `docker` group or that rootless Docker is configured.

### Permissions issues on Linux

If files created by the agent are owned by `root`, try enabling `enable_user_mapping: true` in the `docker` configuration. Note that the image must have `shadow-utils` (`useradd`/`groupadd`) installed for this to work.
