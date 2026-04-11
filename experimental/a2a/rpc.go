package a2a

import (
	"encoding/json"
)

// Well-known JSON-RPC method names supported by the server adapter.
const (
	MethodMessageSend   = "message/send"
	MethodMessageStream = "message/stream"
	MethodTasksGet      = "tasks/get"
	MethodTasksCancel   = "tasks/cancel"
)

// DefaultAgentCardPath is the canonical well-known URL path at which an
// A2A agent card is served, per the current A2A spec.
const DefaultAgentCardPath = "/.well-known/agent-card.json"

// LegacyAgentCardPath is the previous A2A well-known URL path. The server
// adapter serves the same card body at this path so older clients keep
// working; new clients should use DefaultAgentCardPath.
const LegacyAgentCardPath = "/.well-known/agent.json"

// JSON-RPC error codes reserved by the A2A spec. Values in the
// -32000..-32099 range are server-defined in JSON-RPC 2.0; A2A carves out
// -32001..-32099 for protocol-level errors.
const (
	ErrorCodeTaskNotFound        = -32001
	ErrorCodeTaskNotCancelable   = -32002
	ErrorCodePushNotifUnsupported = -32003
	ErrorCodeUnsupportedOperation = -32004
	ErrorCodeContentTypeMismatch = -32005
	ErrorCodeInvalidAgentResp    = -32006

	// Standard JSON-RPC 2.0 codes.
	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

// RPCRequest is a JSON-RPC 2.0 request envelope. Params and ID are carried
// as raw JSON so the dispatcher can decode them on a per-method basis.
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is a JSON-RPC 2.0 response envelope. Exactly one of Result
// and Error is populated on any successful dispatch.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error implements the error interface on RPCError so the dispatcher can
// return it directly from handler functions.
func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// newRPCError is a short constructor used by the dispatcher.
func newRPCError(code int, msg string, data any) *RPCError {
	return &RPCError{Code: code, Message: msg, Data: data}
}
