package mcp

import (
	"errors"
	"fmt"
)

var (
	// ErrNotConnected is returned when attempting to use a disconnected client
	ErrNotConnected = errors.New("mcp client not connected")

	// ErrServerNotFound is returned when a server is not found
	ErrServerNotFound = errors.New("mcp server not found")

	// ErrToolNotFound is returned when a tool is not found
	ErrToolNotFound = errors.New("mcp tool not found")

	// ErrResourceNotFound is returned when a resource is not found
	ErrResourceNotFound = errors.New("mcp resource not found")

	// ErrUnsupportedOperation is returned when the server doesn't support an operation
	ErrUnsupportedOperation = errors.New("mcp operation not supported by server")

	// ErrInitializationFailed is returned when client initialization fails
	ErrInitializationFailed = errors.New("mcp client initialization failed")
)

// MCPError wraps MCP-specific errors with additional context
type MCPError struct {
	Operation  string
	ServerName string
	Err        error
}

func (e *MCPError) Error() string {
	if e.ServerName != "" {
		return fmt.Sprintf("MCP %s failed for server %s: %v", e.Operation, e.ServerName, e.Err)
	}
	return fmt.Sprintf("MCP %s failed: %v", e.Operation, e.Err)
}

func (e *MCPError) Unwrap() error {
	return e.Err
}

// NewMCPError creates a new MCPError
func NewMCPError(operation, serverName string, err error) *MCPError {
	return &MCPError{
		Operation:  operation,
		ServerName: serverName,
		Err:        err,
	}
}

// IsNotConnectedError checks if an error is a not connected error
func IsNotConnectedError(err error) bool {
	return errors.Is(err, ErrNotConnected)
}

// IsUnsupportedOperationError checks if an error is an unsupported operation error
func IsUnsupportedOperationError(err error) bool {
	return errors.Is(err, ErrUnsupportedOperation)
}

// IsResourceNotFoundError checks if an error is a resource not found error
func IsResourceNotFoundError(err error) bool {
	return errors.Is(err, ErrResourceNotFound)
}
