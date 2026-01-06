# Network Proxy Design (Sandbox)

This document proposes a built-in network proxy that enforces domain allowlists for sandboxed commands. It is intended to run in the same Dive process and be used by the sandboxed Bash tool on macOS and Docker/Podman.

## Goals

- Enforce `sandbox.network.allowed_domains` at the transport layer.
- Keep behavior consistent across macOS (Seatbelt) and Docker backends.
- Require minimal user setup (auto-start proxy when configured).
- Be transparent to sandboxed processes by using standard proxy env vars.
- Provide clear audit logging for allowed/blocked requests.

## Non-Goals

- Full traffic inspection (no TLS MITM or content scanning).
- Per-command network rules beyond domain allowlists.
- Replacing OS-level network isolation (`allow_network` remains primary on/off).

## Requirements

- Support HTTP and HTTPS via standard `HTTP_PROXY` / `HTTPS_PROXY`.
- Enforce allowlist on the CONNECT target (HTTPS) and Host header (HTTP).
- Deny all network access when `allow_network=false`.
- Do not change behavior for non-sandboxed processes.

## Architecture

### Components

1) **Proxy Server** (new package: `sandbox/proxy`)
   - Runs in-process with a short-lived listener bound to `127.0.0.1`.
   - Accepts HTTP proxy requests and CONNECT tunnels.
   - Enforces domain allowlist.

2) **Sandbox Manager Integration**
   - When `sandbox.network.allowed_domains` is configured, start proxy automatically.
   - Inject `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` into sandboxed processes.
   - `allow_network` must be `true` or the sandbox rejects startup.

### Data Flow

1) User enables sandbox and sets `network.allowed_domains`.
2) Manager starts proxy and obtains a local port.
3) Manager sets proxy env vars for sandboxed commands.
4) Sandbox backend runs the command.
5) Proxy allows or blocks outbound requests based on domain allowlist.

## Domain Matching

- Match against hostname (case-insensitive).
- Allow suffix matching for subdomains (e.g. `example.com` allows `api.example.com`).
- Support exact match (for simple hosts like `localhost`).
- Optional future: support wildcard syntax (e.g. `*.example.com`).

## HTTPS Handling

- Use standard HTTP proxy CONNECT.
- Enforce allowlist against the CONNECT target host.
- No TLS interception. The proxy only sees the target host and port.

## Configuration

Example:

```json
{
  "sandbox": {
    "enabled": true,
    "allow_network": true,
    "network": {
      "allowed_domains": ["github.com", "proxy.golang.org"]
    }
  }
}
```

Behavior:

- If `allowed_domains` is set and `allow_network` is false, startup fails with a clear error.
- If `allowed_domains` is set and no proxy is configured, the manager starts the in-process proxy and injects env vars automatically.

## Logging

Log events (behind `sandbox.audit_log`):

- Proxy start/stop with chosen port.
- Allowed requests: host, port, method.
- Blocked requests: host, port, reason.

## Failure Modes

- Proxy fails to bind port: sandbox startup fails with error.
- Invalid request: return `403 Forbidden` with a short reason string.
- Allowed domains empty while proxy enabled: treat as deny-all.

## Security Considerations

- The proxy only enforces domains and does not inspect content.
- DNS can still resolve arbitrary domains on the host; enforcement happens at proxy.
- If a sandboxed process bypasses proxy env vars, it can access the network unless `allow_network=false`. The container/Seatbelt layer remains the primary on/off control.

## Testing Plan

- Unit tests for domain matching and allowlist behavior.
- Integration test: run a sandboxed command that attempts `curl` to an allowed and disallowed host.
- Regression test: verify `allow_network=false` still blocks all outbound access.

## Rollout Plan

1) Implement proxy package and integrate with `sandbox.Manager`.
2) Update docs to describe built-in proxy and its limitations.
3) Add tests and basic metrics/logging.
4) Release behind configuration (`network.allowed_domains`) only.

## Open Questions

- Should we expose a fixed port or always choose an ephemeral port?
- Would it be beneficial for `NO_PROXY` to default to localhost and 127.0.0.1?
- Is wildcard syntax support in `allowed_domains` needed from day one?
