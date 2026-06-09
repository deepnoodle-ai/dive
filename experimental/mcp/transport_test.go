package mcp

import (
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

func TestBuildRequestHeaders(t *testing.T) {
	t.Run("auth token and custom headers", func(t *testing.T) {
		headers := buildRequestHeaders("secret", map[string]string{"X-Custom": "value"})
		assert.Equal(t, "Bearer secret", headers["Authorization"])
		assert.Equal(t, "value", headers["X-Custom"])
	})

	t.Run("no auth token", func(t *testing.T) {
		headers := buildRequestHeaders("", map[string]string{"X-Custom": "value"})
		_, hasAuth := headers["Authorization"]
		assert.False(t, hasAuth)
		assert.Equal(t, "value", headers["X-Custom"])
	})

	t.Run("does not set Content-Type or Accept", func(t *testing.T) {
		// The mcp-go streamable HTTP transport manages Content-Type and
		// Accept itself, and custom headers override the transport's values.
		// Setting Accept here would break SSE streaming.
		headers := buildRequestHeaders("secret", nil)
		_, hasContentType := headers["Content-Type"]
		_, hasAccept := headers["Accept"]
		assert.False(t, hasContentType)
		assert.False(t, hasAccept)
	})

	t.Run("custom headers can override authorization", func(t *testing.T) {
		headers := buildRequestHeaders("secret", map[string]string{"Authorization": "Custom xyz"})
		assert.Equal(t, "Custom xyz", headers["Authorization"])
	})
}
