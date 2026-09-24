package mcp

import (
	"context"
	"errors"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestParseServersJSON(t *testing.T) {
	servers, err := ParseServersJSON([]byte(`{
		"model": "ignored",
		"mcpServers": {
			"files": {"command": "npx", "args": ["-y", "server"], "env": {"TOKEN": "${TOKEN}"}},
			"remote": {"type": "HTTP", "url": "https://example.com/mcp", "headers": {"Authorization": "Bearer x"}},
			"stream": {"type": "sse", "url": "https://example.com/sse"},
			"implicit": {"url": "https://example.com/implicit"}
		}
	}`))
	assert.NoError(t, err)
	assert.Len(t, servers, 4)

	files := servers["files"]
	assert.Equal(t, "files", files.Name)
	assert.Equal(t, "stdio", files.Type)
	assert.Equal(t, "npx", files.Command)
	assert.Equal(t, []string{"-y", "server"}, files.Args)
	assert.Equal(t, "${TOKEN}", files.Env["TOKEN"], "expansion happens at connect time")

	assert.Equal(t, "http", servers["remote"].Type)
	assert.Equal(t, "Bearer x", servers["remote"].Headers["Authorization"])
	assert.Equal(t, "sse", servers["stream"].Type)
	assert.Equal(t, "http", servers["implicit"].Type)
}

func TestParseServersJSONInvalidEntries(t *testing.T) {
	servers, err := ParseServersJSON([]byte(`{"mcpServers": {
		"good": {"command": "server"},
		"no-command": {"type": "stdio"},
		"no-url": {"type": "http"},
		"empty": {},
		"weird": {"type": "websocket", "url": "ws://x"}
	}}`))
	assert.Error(t, err)
	assert.Len(t, servers, 1)
	assert.NotNil(t, servers["good"])
	for _, name := range []string{"no-command", "no-url", "empty", "weird"} {
		assert.Contains(t, err.Error(), `"`+name+`"`)
	}
}

func TestParseServersJSONMalformed(t *testing.T) {
	servers, err := ParseServersJSON([]byte(`{"mcpServers": [`))
	assert.Error(t, err)
	assert.Nil(t, servers)

	servers, err = ParseServersJSON([]byte(`{}`))
	assert.NoError(t, err)
	assert.Len(t, servers, 0)
}

func TestLoadServersFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadServersFile(filepath.Join(dir, "missing.json"))
	assert.True(t, errors.Is(err, fs.ErrNotExist))

	path := filepath.Join(dir, ".mcp.json")
	assert.NoError(t, os.WriteFile(path, []byte(`{"mcpServers": {"a": {"command": "x"}}}`), 0o600))
	servers, err := LoadServersFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "x", servers["a"].Command)
}

func TestQualifiedToolName(t *testing.T) {
	assert.Equal(t, "mcp__github__create_issue", QualifiedToolName("github", "create_issue"))
	assert.Equal(t, "mcp__my_server__get-thing_v2", QualifiedToolName("my server", "get-thing.v2"))

	adapter := NewQualifiedToolAdapter(nil, mcp.Tool{Name: "echo"}, "demo")
	assert.Equal(t, "mcp__demo__echo", adapter.Name())
	assert.Equal(t, "demo", adapter.ServerName())
	assert.Equal(t, "echo", NewToolAdapter(nil, mcp.Tool{Name: "echo"}, "demo").Name())
}

// TestClientSSEOutlivesConnectContext connects to an SSE server with a
// context that is cancelled right after Connect returns. The SSE stream must
// not be tied to it, or every later call would fail.
func TestClientSSEOutlivesConnectContext(t *testing.T) {
	srv := mcpserver.NewMCPServer("sse-test", "1.0.0", mcpserver.WithToolCapabilities(true))
	addTestTool(srv, "ping")
	sse := mcpserver.NewSSEServer(srv)
	ts := httptest.NewServer(sse)
	t.Cleanup(ts.Close)
	mcpserver.WithBaseURL(ts.URL)(sse)

	t.Setenv("DIVE_MCP_TEST_SSE_URL", ts.URL+"/sse")
	client, err := NewClient(&ServerConfig{Name: "sse", Type: "sse", URL: "${DIVE_MCP_TEST_SSE_URL}"})
	assert.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	assert.NoError(t, client.Connect(ctx))
	cancel()
	defer client.Close()

	tools, err := client.ListTools(context.Background())
	assert.NoError(t, err)
	assert.Len(t, tools, 1)
	result, err := client.CallTool(context.Background(), "ping", nil)
	assert.NoError(t, err)
	assert.Equal(t, "ok", result.Content[0].(mcp.TextContent).Text)
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("DIVE_MCP_TEST_SET", "value")
	t.Setenv("DIVE_MCP_TEST_EMPTY", "")
	assert.Equal(t, "value", ExpandEnv("${DIVE_MCP_TEST_SET}"))
	assert.Equal(t, "value", ExpandEnv("$DIVE_MCP_TEST_SET"))
	assert.Equal(t, "value", ExpandEnv("${DIVE_MCP_TEST_SET:-fallback}"))
	assert.Equal(t, "fallback", ExpandEnv("${DIVE_MCP_TEST_EMPTY:-fallback}"))
	assert.Equal(t, "fallback", ExpandEnv("${DIVE_MCP_TEST_UNSET_XYZ:-fallback}"))
	assert.Equal(t, "Bearer ", ExpandEnv("Bearer ${DIVE_MCP_TEST_UNSET_XYZ}"))
	assert.Equal(t, "no vars", ExpandEnv("no vars"))
}
