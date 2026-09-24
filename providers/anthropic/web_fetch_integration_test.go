//go:build integration

package anthropic

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

// webFetchTool declares Anthropic's server-side web fetch tool.
type webFetchTool struct{}

func (webFetchTool) Name() string           { return "web_fetch" }
func (webFetchTool) Description() string    { return "Anthropic server-side web fetch." }
func (webFetchTool) Schema() *schema.Schema { return nil }
func (webFetchTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: true}
}
func (webFetchTool) Call(context.Context, any) (*dive.ToolResult, error) {
	return nil, errors.New("server-side tool does not implement local calls")
}
func (webFetchTool) ToolConfiguration(string) map[string]any {
	return map[string]any{"type": "web_fetch_20250910", "name": "web_fetch", "max_uses": 2}
}

// A streaming agent with a session uses web fetch on turn one. Turn two
// sends that history back, which Anthropic rejects if the server_tool_use
// block is kept without its web_fetch_tool_result.
func TestIntegrationWebFetchSessionSecondTurn(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sess := session.New("web-fetch-integration")
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:   New(WithModel("claude-haiku-4-5")),
		Tools:   []dive.Tool{webFetchTool{}},
		Session: sess,
	})
	assert.NoError(t, err)

	first, err := agent.CreateResponse(ctx, dive.WithInput(
		"Use the web_fetch tool to fetch https://example.com and tell me the page's title."))
	assert.NoError(t, err)
	assert.NotEqual(t, "", first.OutputText())

	// The fetched page stays in the history, paired with its call.
	messages, err := sess.Messages(ctx)
	assert.NoError(t, err)
	var calls, results int
	for _, message := range messages {
		for _, content := range message.Content {
			switch c := content.(type) {
			case *llm.ServerToolUseContent:
				if c.Name == "web_fetch" {
					calls++
				}
			case *llm.ServerToolResultContent:
				if c.BlockType == llm.ContentTypeWebFetchToolResult {
					results++
				}
			}
		}
	}
	assert.True(t, calls > 0, "expected a web_fetch call in the history")
	assert.Equal(t, calls, results)

	second, err := agent.CreateResponse(ctx, dive.WithInput("Thanks. In one sentence, what is that page for?"))
	assert.NoError(t, err)
	assert.NotEqual(t, "", second.OutputText())
}

// Generate (non-streaming) decodes a web fetch the same way, and its
// history is accepted on the next request.
func TestIntegrationWebFetchGenerateReplay(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	provider := New(WithModel("claude-haiku-4-5"))
	question := llm.NewUserTextMessage("Use the web_fetch tool to fetch https://example.com and tell me the page's title.")
	first, err := provider.Generate(ctx,
		llm.WithMessages(question),
		llm.WithTools(webFetchTool{}),
		llm.WithMaxTokens(1024))
	assert.NoError(t, err)
	var fetched bool
	for _, content := range first.Content {
		if r, ok := content.(*llm.ServerToolResultContent); ok && r.BlockType == llm.ContentTypeWebFetchToolResult {
			fetched = true
		}
	}
	assert.True(t, fetched, "expected a web_fetch_tool_result block")

	_, err = provider.Generate(ctx,
		llm.WithMessages(question, first.Message(), llm.NewUserTextMessage("Thanks. In one sentence, what is that page for?")),
		llm.WithTools(webFetchTool{}),
		llm.WithMaxTokens(1024))
	assert.NoError(t, err)
}
