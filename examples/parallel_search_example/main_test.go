package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/google/uuid"
)

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name, pageURL, tool string
		toolError           bool
		sessionID           string
	}{
		{name: "search", tool: "web_search"},
		{name: "fetch", pageURL: "https://example.com/docs", tool: "web_fetch"},
		{name: "server error", tool: "web_search", toolError: true},
		{name: "reused session", pageURL: "https://example.com/docs", tool: "web_fetch", sessionID: "related-calls-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var methods []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				assert.Equal(t, r.URL.Path, "/mcp")
				assert.Equal(t, r.Header.Get("User-Agent"), "dive-parallel-search-example/1.0")
				assert.Equal(t, r.Header.Get("Authorization"), "")
				assert.Equal(t, r.Header.Get("X-Api-Key"), "")
				var req struct {
					ID     any    `json:"id"`
					Method string `json:"method"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				mu.Lock()
				methods = append(methods, req.Method)
				mu.Unlock()
				var result any
				switch req.Method {
				case "server/discover":
					// Exercise the MCP client's legacy initialization fallback.
					w.Header().Set("Content-Type", "application/json")
					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "Method not found"}}))
					return
				case "initialize":
					result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "fixture", "version": "1"}}
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
					return
				case "tools/list":
					result = map[string]any{"tools": []any{
						map[string]any{"name": "web_search", "inputSchema": map[string]any{"type": "object"}},
						map[string]any{"name": "web_fetch", "inputSchema": map[string]any{"type": "object"}},
					}}
				case "tools/call":
					assert.Equal(t, req.Params.Name, tc.tool)
					if tc.sessionID != "" {
						assert.Equal(t, req.Params.Arguments["session_id"], any(tc.sessionID))
					} else {
						_, err := uuid.Parse(req.Params.Arguments["session_id"].(string))
						assert.NoError(t, err)
					}
					if tc.pageURL == "" {
						assert.Equal(t, req.Params.Arguments["objective"], "Dive MCP docs")
						assert.Equal(t, req.Params.Arguments["search_queries"], any([]any{"Dive MCP docs"}))
					} else {
						assert.Equal(t, req.Params.Arguments["urls"], any([]any{tc.pageURL}))
					}
					result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "https://example.com/docs: MCP documentation excerpt"}}, "isError": tc.toolError}
				default:
					t.Errorf("unexpected method: %s", req.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}))
			}))
			defer server.Close()
			// Keep the shipped headers and transport; replace only the endpoint.
			var config map[string]any
			assert.NoError(t, json.Unmarshal(configJSON, &config))
			config["mcpServers"].(map[string]any)["parallel"].(map[string]any)["url"] = server.URL + "/mcp"
			data, err := json.Marshal(config)
			assert.NoError(t, err)
			var output bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = run(ctx, data, "Dive MCP docs", tc.pageURL, tc.sessionID, &output)
			if tc.toolError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "MCP documentation excerpt")
				assert.Equal(t, output.String(), "")
			} else {
				assert.NoError(t, err)
				assert.Contains(t, output.String(), "https://example.com/docs: MCP documentation excerpt")
			}
			mu.Lock()
			defer mu.Unlock()
			assert.Contains(t, methods, "initialize")
			assert.Contains(t, methods, "tools/list")
			assert.Contains(t, methods, "tools/call")
		})
	}
}

func TestRunCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	assert.Error(t, run(ctx, configJSON, "Dive MCP docs", "", "", &out))
	assert.Equal(t, out.String(), "")
}
