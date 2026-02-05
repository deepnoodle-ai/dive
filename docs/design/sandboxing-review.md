# Sandboxing Design Review

> **Historical**: This review was written prior to implementation. Some recommendations have since been implemented (e.g. `MountCloudCredentials`, network proxy filtering). See `experimental/sandbox/` for current state.

## Executive Summary

The proposed sandboxing design for Dive (`docs/design/sandboxing.md`) presents a pragmatic, defense-in-depth approach leveraging native OS capabilities: **Seatbelt (sandbox-exec)** for macOS and **Docker/Podman** for Linux/Windows.

The architecture is sound and aligns well with similar industry implementations, specifically the **Gemini CLI**. However, a comparison with Gemini CLI's production codebase reveals several opportunities to improve robustness, usability, and security before implementation.

## Key Findings & Comparison

| Feature                      | Gemini CLI Implementation                                                                             | Dive Design Proposal                                                       | Recommendation                                                                                                      |
| :--------------------------- | :---------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------ |
| **macOS Profile Generation** | Static `.sb` files with `-D` param injection. Hard limit of 5 `INCLUDE_DIR`s padded with `/dev/null`. | Static `.sb` files with `-D` param injection. Same limit/padding proposed. | **Upgrade:** Use Go `text/template` for dynamic profile generation to remove arbitrary limits and clean up padding. |
| **macOS System Calls**       | Explicitly allows `sysctl-read` and `mach-lookup`.                                                    | Not mentioned.                                                             | **Critical:** Must add these permissions to support Go runtime and system tools like `ps`.                          |
| **Linux User Mapping**       | Complex script injection (`groupadd`, `useradd`). Limits usage to Debian/Ubuntu via OS detection.     | Shell script injection proposed.                                           | **Refine:** The proposed script is fragile on Alpine/Distroless. Make this opt-in or robustly detect supported OSs. |
| **Network Isolation**        | Complex "Proxy Container" approach.                                                                   | "None" or "Host" network via flag.                                         | **Keep Simple:** Dive's binary toggle is appropriate for a v1.                                                      |
| **Read Access Model**        | Container sees only mounted paths.                                                                    | macOS Seatbelt allows `file-read*` everywhere.                             | **Clarify:** Document or configure the cross-platform read-access policy to avoid surprises.                        |

## Critical Recommendations (Must Implement)

### 1. Dynamic Seatbelt Profiles (macOS)

The design proposes a static profile with fixed slots (`ALLOWED_DIR_0`...`ALLOWED_DIR_5`). This is brittle.

- **Change:** Use Go's `text/template` engine to generate the `.sb` profile string at runtime.
- **Benefit:** Removes the arbitrary 5-path limit and eliminates the need to pad unused slots with `/dev/null`.

### 2. Missing macOS Permissions

The draft `restrictive.sb` is likely too tight for a Go-based agent or common shell tools.

- **Change:** Add the following to the Seatbelt profile:

  ```scheme
  ;; Essential for 'ps', 'top', 'pgrep' (process listing)
  (allow mach-lookup (global-name "com.apple.sysmond"))

  ;; Essential for Go runtime initialization (hardware capabilities)
  (allow sysctl-read)
  ```

### 3. Safer Linux User Mapping

The proposed `groupadd`/`useradd` injection assumes a standard glibc/shadow-utils environment (like Ubuntu/Debian). It will fail on Alpine (uses `adduser`) or Distroless (no shell/tools).

- **Change:**
  - Default to running as `root` (or the image's default user) for unknown images.
  - Add a config flag `SandboxUserMapping` (default `false` or auto-detect) to enable the UID/GID mapping logic.
  - Document that custom images require standard user management tools if mapping is enabled.

### 4. Cancellation and Cleanup

The current design wraps `BashSession` with `context.Background()` and does not guarantee cleanup of temporary Seatbelt profiles.

- **Change:**
  - Thread the caller's `context.Context` into `WrapCommand` so cancellation terminates sandboxed commands.
  - For Docker, use `--cidfile` + `docker rm -f` (or `podman rm -f`) on cancellation to avoid leaking containers.
  - Ensure Seatbelt backend deletes the temporary `.sb` file (e.g., `defer os.Remove`).

## Enhancement Recommendations (High Value)

### 1. Usability: Standard Credential Mounting

Agents frequently need to interact with cloud resources. Without credentials, they are blocked.

- **Proposal:** Add `MountCloudCredentials bool` to `SandboxConfig`.
- **Implementation:** Auto-detect and mount these paths as **Read-Only**:
  - `~/.config/gcloud`
  - `~/.aws/credentials`
  - Target of `GOOGLE_APPLICATION_CREDENTIALS`

### 2. Security: Read-Only Root Filesystem

For the Docker backend, the agent only needs write access to the workspace and `/tmp`.

- **Proposal:** Add `--read-only` to the Docker run command.
- **Benefit:** Prevents the agent from permanently modifying the container's system binaries (defense against persistence or environment corruption).

### 3. Robustness: Lifecycle Management

- **Cleanup:** Ensure the Seatbelt backend explicitly deletes the temporary `.sb` file using `defer os.Remove(...)` to avoid polluting the user's temp dir.
- **Cancellation:** Ensure `WrapCommand` properly handles `context.Context` cancellation to kill the container/process if the user aborts a long-running tool.

### 4. Configuration: Environment Allow-list

Gemini CLI explicitly passes through `TERM`, `COLORTERM`, and project-specific env vars.

- **Proposal:** Define a default allow-list of environment variables to pass into the sandbox so the terminal environment behaves as expected (e.g., colors, encoding).

### 5. Path Normalization

Relative paths or symlinks in `WorkDir`/`AllowedWritePaths` can undermine intended restrictions or create inconsistent behavior across backends.

- **Proposal:** Canonicalize paths (`abs` + `eval symlinks`) before generating Seatbelt profiles or volume mounts, and document that normalized paths are used for enforcement.

## Conclusion

The design is structurally sound. By adopting **dynamic templates for macOS**, **guardrails for Linux user mapping**, **cancellation/cleanup**, and **clarified read-access + path normalization**, Dive's sandboxing will be significantly more robust and developer-friendly than a naive implementation.
