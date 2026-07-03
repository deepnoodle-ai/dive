package anthropic

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

// TestToolResultWithAuxiliaryContext_Live confirms against the real Anthropic
// Messages API that a user message carrying a tool_result block followed by
// auxiliary text (the shape a PostToolUse hook's AdditionalContext produces) is
// accepted. Anthropic requires tool_result blocks to lead the message; the
// agent-level normalization guarantees that ordering, and this exercises the
// resulting shape end-to-end.
func TestToolResultWithAuxiliaryContext_Live(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping integration test")
	}
	ctx := context.Background()
	provider := New(WithModel(ModelClaudeHaiku4520251001))

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	messages := []*llm.Message{
		llm.NewUserTextMessage("Use the add tool to compute 567 + 111, then tell me the result."),
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{ID: "toolu_add_1", Name: "add", Input: []byte(`{"a":567,"b":111}`)},
			},
		},
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.ToolResultContent{ToolUseID: "toolu_add_1", Content: "678"},
				&llm.TextContent{Text: "(verification note: computed by calculator service v2)"},
			},
		},
	}

	response, err := provider.Generate(ctx,
		llm.WithMessages(messages...),
		llm.WithTools(add),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	text := response.Message().Text()
	assert.True(t, strings.Contains(text, "678"),
		"expected final answer to reflect the tool result, got: %q", text)
}
