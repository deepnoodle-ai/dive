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
//   - agent card discovery via /.well-known/agent-card.json (canonical)
//     and /.well-known/agent.json (legacy alias)
//   - message/send and tasks/get JSON-RPC methods
//   - tasks/cancel with cancellation propagation to in-flight LLM calls
//   - message/stream for Server-Sent Events streaming of task progress
//   - mapping Dive's ResponseStatusSuspended to A2A input-required state
//   - multi-pending-tool-call resume via structured DataPart or text broadcast
//   - faithful content projection: text, image, document, and refusal
//     content types are mapped to A2A parts in artifacts and history
//   - flattening non-text input parts (DataPart, FilePart) into the
//     agent prompt so structured A2A messages round-trip usefully
//   - TaskStore.List for user-implemented expiration and cleanup
//
// tasks/resubscribe, the tasks/pushNotificationConfig/* family, and
// agent/getAuthenticatedExtendedCard are recognized by the dispatcher
// but respond with -32004 UnsupportedOperation (rather than -32601
// MethodNotFound) so peers get a meaningful signal when probing for
// them.
//
// # Library philosophy
//
// This package handles protocol fidelity and Dive runtime integration.
// Deployment concerns — authentication, rate limiting, timeouts, graceful
// shutdown, durable storage, and observability — belong in your HTTP server
// and middleware. The package provides the right seams: Server.Handler
// returns an http.Handler you can wrap, TaskStore and SessionProvider are
// pluggable interfaces, and in-flight turn contexts propagate cancellation
// from your server's shutdown path.
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
// The package targets the A2A v1.0 JSON-RPC binding: method names like
// "message/send"/"message/stream"/"tasks/get"/"tasks/cancel",
// SCREAMING_SNAKE_CASE task state strings ("TASK_STATE_INPUT_REQUIRED",
// "TASK_STATE_COMPLETED"), content-based Part discrimination (text, raw,
// data, url), and StreamResponse field-name discrimination for streaming
// events. Custom MarshalJSON implementations on the types fill in safe
// defaults so manually constructed values still validate against strict
// A2A clients.
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
