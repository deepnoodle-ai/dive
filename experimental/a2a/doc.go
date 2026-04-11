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
// The prototype targets A2A wire format v0.2 (JSON-RPC method names like
// "message/send", hyphenated task state strings like "input-required", and
// the "kind" part discriminator). This is the form most deployed clients
// speak today. Later revisions of this package may add v1.0 aliases.
package a2a
