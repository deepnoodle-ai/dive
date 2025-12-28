package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestContextManagementRequest(t *testing.T) {
	// Create context management config
	cmConfig := &llm.ContextManagementConfig{
		Edits: []llm.ContextManagementEdit{
			{
				Type: "clear_tool_uses_20250919",
				Trigger: &llm.ContextManagementTrigger{
					Type:  "input_tokens",
					Value: 30000,
				},
				Keep: &llm.ContextManagementKeep{
					Type:  "tool_uses",
					Value: float64(3),
				},
			},
		},
	}

	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Check beta header for context management
		betaHeader := r.Header.Get("anthropic-beta")
		assert.Contains(t, betaHeader, FeatureContextManagement, "anthropic-beta header should contain context management feature")

		// Parse request body
		var req Request
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)

		// Check context management config
		assert.NotNil(t, req.ContextManagement)
		assert.Len(t, req.ContextManagement.Edits, 1)

		// Convert keep value to float64 for comparison, to match json unmarshal behavior
		expectedEdit := cmConfig.Edits[0]
		if val, ok := expectedEdit.Keep.Value.(int); ok {
			expectedEdit.Keep.Value = float64(val)
		}

		assert.Equal(t, expectedEdit, req.ContextManagement.Edits[0])

		// Return mock response
		resp := llm.Response{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-sonnet-4-5",
			Content: []llm.Content{
				&llm.TextContent{Text: "Context cleared."},
			},
			Usage: llm.Usage{
				InputTokens:  1000,
				OutputTokens: 10,
			},
			ContextManagement: &llm.ContextManagementResponse{
				OriginalInputTokens: 35000,
				AppliedEdits: []llm.AppliedContextEdit{
					{
						Type:               "clear_tool_uses_20250919",
						ClearedToolUses:    5,
						ClearedInputTokens: 2000,
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Configure provider with mock server
	provider := New(
		WithEndpoint(server.URL),
		WithAPIKey("test-key"),
	)

	// Make request
	resp, err := provider.Generate(context.Background(),
		llm.WithMessages(llm.NewUserTextMessage("test")),
		llm.WithContextManagement(cmConfig),
	)

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.NotNil(t, resp.ContextManagement)
	assert.Equal(t, 35000, resp.ContextManagement.OriginalInputTokens)
	assert.Len(t, resp.ContextManagement.AppliedEdits, 1)

	applied := resp.ContextManagement.AppliedEdits[0]
	assert.Equal(t, "clear_tool_uses_20250919", applied.Type)
	assert.Equal(t, 5, applied.ClearedToolUses)
	assert.Equal(t, 2000, applied.ClearedInputTokens)
}
