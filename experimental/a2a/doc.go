// Package a2a provides experimental support for the A2A (Agent-to-Agent)
// protocol so that Dive agents can be exposed as remote A2A agents and can
// call remote A2A agents from Go code.
//
// # Status
//
// This package is experimental. Its API may change without notice. It lives
// under experimental/ because the A2A protocol, its Go SDK story, and the
// mapping between Dive's local runtime and A2A task semantics are all still
// evolving.
//
// # Scope
//
// The first shipping phase of A2A support is deliberately small:
//
//   - agent card discovery via /.well-known/agent.json
//   - message/send and tasks/get JSON-RPC methods
//   - tasks/cancel for in-flight task cancellation
//   - message/stream for Server-Sent Events streaming of task progress
//   - mapping Dive's ResponseStatusSuspended to A2A input-required state
//
// See docs/prds/prd-05-a2a-support.md for the full motivation, goals, and
// out-of-scope items. See docs/guides/experimental/a2a.md for usage.
//
// # Architectural boundaries
//
// Dive core remains the authoritative local runtime. The A2A layer is an
// adapter that projects Dive responses onto A2A task state. No A2A types or
// protocol concerns leak into dive.Agent, dive.Response, or dive.Session.
//
// # Wire format
//
// The package targets the current A2A JSON-RPC binding: method names like
// "message/send"/"message/stream"/"tasks/get"/"tasks/cancel", hyphenated
// lowercase task state strings ("input-required", "completed"), and "kind"
// discriminators on Part, Message, Task, TaskStatusUpdateEvent, and
// TaskArtifactUpdateEvent. Custom MarshalJSON implementations on the
// types fill in safe defaults so manually constructed values still
// validate against strict A2A clients.
//
// The agent card is served at the canonical
// /.well-known/agent-card.json path; the legacy /.well-known/agent.json
// path is also served for older clients. The client fetches the
// canonical path and falls back to the legacy path on 404.
//
// Phase 1 has been cross-validated against the official a2a-python SDK in
// both directions; see experimental/a2a/interop_test.go (build tag
// "interop") and docs/guides/experimental/a2a.md for details.
package a2a
