package openaicompletions

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

// TestToolResultWithAuxiliaryContext_Live confirms against the real OpenAI Chat
// Completions API that a user message carrying a tool_result block alongside
// auxiliary text (the shape a PostToolUse hook's AdditionalContext produces) is
// accepted. Before the ordering fix this could surface interleaved tool/user
// messages and be rejected by the API.
func TestToolResultWithAuxiliaryContext_Live(t *testing.T) {
	skipIfNoAPIKey(t)
	ctx := context.Background()
	provider := New()

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
				&llm.ToolUseContent{ID: "call_add_1", Name: "add", Input: []byte(`{"a":567,"b":111}`)},
			},
		},
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.ToolResultContent{ToolUseID: "call_add_1", Content: "678"},
				&llm.TextContent{Text: "(verification note: computed by calculator service v2)"},
			},
		},
	}

	response, err := provider.Generate(ctx,
		llm.WithModel("gpt-4o-mini"),
		llm.WithMessages(messages...),
		llm.WithTools(add),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	text := response.Message().Text()
	assert.True(t, strings.Contains(text, "678"),
		"expected final answer to reflect the tool result, got: %q", text)
}
