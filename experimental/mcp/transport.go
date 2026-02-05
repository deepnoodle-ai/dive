package mcp

import (
	"fmt"
	"net/url"
	"strings"
)

// TransportType represents the type of transport used for MCP communication
type TransportType string

const (
	TransportHTTP  TransportType = "http"
	TransportStdio TransportType = "stdio"
)

// ValidateTransportConfig validates the MCP server transport configuration
func ValidateTransportConfig(serverType, serverURL string) error {
	transportType := TransportType(strings.ToLower(serverType))
	switch transportType {
	case TransportHTTP:
		return validateHTTPConfig(serverURL)
	case TransportStdio:
		return validateStdioConfig(serverURL)
	default:
		return fmt.Errorf("unsupported transport type: %s (supported: http, stdio)", serverType)
	}
}

// validateHTTPConfig validates HTTP transport configuration
func validateHTTPConfig(serverURL string) error {
	if serverURL == "" {
		return fmt.Errorf("URL is required for HTTP transport")
	}
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("HTTP transport requires http:// or https:// URL scheme")
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("HTTP transport requires a valid host in URL")
	}
	return nil
}

// validateStdioConfig validates stdio transport configuration
func validateStdioConfig(command string) error {
	if command == "" {
		return fmt.Errorf("command is required for stdio transport")
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command cannot be empty or only whitespace")
	}
	return nil
}

// ParseStdioCommand parses a stdio command string into components
func ParseStdioCommand(command string) ([]string, error) {
	if err := validateStdioConfig(command); err != nil {
		return nil, err
	}
	// Simple space-based splitting for now
	// This could be enhanced to handle quoted arguments properly
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return nil, fmt.Errorf("no command components found")
	}
	return parts, nil
}

// BuildHTTPHeaders creates HTTP headers for MCP requests
func BuildHTTPHeaders(authToken string, additionalHeaders map[string]string) map[string]string {
	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "application/json"
	if authToken != "" {
		headers["Authorization"] = "Bearer " + authToken
	}
	for key, value := range additionalHeaders {
		headers[key] = value
	}
	return headers
}

// TransportConfig holds configuration for different transport types
type TransportConfig struct {
	Type      TransportType
	URL       string
	Command   []string          // For stdio transport
	Headers   map[string]string // For HTTP transport
	AuthToken string            // For HTTP transport
}

// NewTransportConfig creates a new transport configuration
func NewTransportConfig(serverType, serverURL, authToken string) (*TransportConfig, error) {
	transportType := TransportType(strings.ToLower(serverType))
	if err := ValidateTransportConfig(string(transportType), serverURL); err != nil {
		return nil, err
	}
	config := &TransportConfig{
		Type:      transportType,
		URL:       serverURL,
		AuthToken: authToken,
	}
	switch transportType {
	case TransportHTTP:
		config.Headers = BuildHTTPHeaders(authToken, nil)
	case TransportStdio:
		command, err := ParseStdioCommand(serverURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stdio command: %w", err)
		}
		config.Command = command
	}
	return config, nil
}
