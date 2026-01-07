package openaicompletions

import (
	"context"
	"strings"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\""),
	))
	assert.NoError(t, err)
	// The model might respond with "Hello!" or other variations, so we check case-insensitive
	assert.Contains(t, strings.ToLower(response.Message().Text()), "hello")
}

func TestHelloWorldStream(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("count to 10. respond with the integers only, separated by spaces."),
	))
	assert.NoError(t, err)

	accum := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accum.AddEvent(event); err != nil {
			assert.NoError(t, err)
		}
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accum.IsComplete())

	response := accum.Response()
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)
	assert.Equal(t, "1 2 3 4 5 6 7 8 9 10", response.Message().Text())
}

func TestToolUse(t *testing.T) {
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

	response, err := provider.Generate(ctx,
		llm.WithModel("gpt-4o-2024-08-06"),
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	assert.NoError(t, err)

	// Use ToolCalls() which filters for tool_use content (model may also return text)
	toolCalls := response.ToolCalls()
	assert.Equal(t, 1, len(toolCalls))

	toolUse := toolCalls[0]
	assert.Equal(t, "add", toolUse.Name)

	// The exact format of the arguments may vary, so we just check that it contains the numbers
	assert.Contains(t, string(toolUse.Input), "567")
	assert.Contains(t, string(toolUse.Input), "111")
}

func TestMultipleToolUse(t *testing.T) {
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

	response, err := provider.Generate(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Calculate two results for me: add 567 and 111, and add 233 and 444")),
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(response.Message().Content))

	c1 := response.Message().Content[0]
	assert.Equal(t, llm.ContentTypeToolUse, c1.Type())

	toolUse, ok := c1.(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "add", toolUse.Name)
	assert.Contains(t, string(toolUse.Input), "567")
	assert.Contains(t, string(toolUse.Input), "111")

	c2 := response.Message().Content[1]
	assert.Equal(t, llm.ContentTypeToolUse, c2.Type())

	toolUse, ok = c2.(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "add", toolUse.Name)
	assert.Contains(t, string(toolUse.Input), "233")
	assert.Contains(t, string(toolUse.Input), "444")
}

func TestMultipleToolUseStreaming(t *testing.T) {
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

	message := llm.NewUserTextMessage("Calculate two results for me: add 567 and 111, and add 233 and 444")

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(message),
		llm.WithTools(add),
		llm.WithToolChoice(&llm.ToolChoice{Type: llm.ToolChoiceTypeAny}),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accumulator.AddEvent(event); err != nil {
			assert.NoError(t, err)
		}
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	toolCalls := response.ToolCalls()
	assert.Equal(t, 2, len(toolCalls))

	// The two calls can be in any order, so we need to check both

	var c1 *llm.ToolUseContent
	var c2 *llm.ToolUseContent

	if strings.Contains(string(toolCalls[0].Input), "567") {
		c1 = toolCalls[0]
		c2 = toolCalls[1]
	} else {
		c1 = toolCalls[1]
		c2 = toolCalls[0]
	}

	assert.Equal(t, "add", c1.Name)
	assert.Contains(t, string(c1.Input), "567")
	assert.Contains(t, string(c1.Input), "111")

	assert.Equal(t, "add", c2.Name)
	assert.Contains(t, string(c2.Input), "233")
	assert.Contains(t, string(c2.Input), "444")
}

func TestToolUseStream(t *testing.T) {
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

	iterator, err := provider.Stream(ctx,
		llm.WithModel("gpt-4o-2024-08-06"),
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithToolChoice(llm.ToolChoiceAuto),
		llm.WithTools(add),
	)
	assert.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accumulator.AddEvent(event); err != nil {
			assert.NoError(t, err)
		}
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	toolCalls := response.ToolCalls()
	assert.Equal(t, 1, len(toolCalls))

	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)

	// Check that we have at least one tool call
	assert.GreaterOrEqual(t, len(response.ToolCalls()), 1)

	// Check that the tool call is for the add function
	toolCall := response.ToolCalls()[0]
	assert.Equal(t, "add", toolCall.Name)

	// Check that the arguments contain the numbers
	assert.Contains(t, string(toolCall.Input), "567")
	assert.Contains(t, string(toolCall.Input), "111")
}

func TestConvertMessages(t *testing.T) {
	// Create a message with two ContentTypeToolUse content blocks
	message := &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.ToolUseContent{
				ID:    "call_123",
				Name:  "Calculator",
				Input: []byte(`{"expression":"2 + 2"}`),
			},
			&llm.ToolUseContent{
				ID:    "call_456",
				Name:  "GoogleSearch",
				Input: []byte(`{"query":"math formulas"}`),
			},
		},
	}

	// Convert the message
	converted, err := convertMessages([]*llm.Message{message})
	assert.NoError(t, err)

	// Verify the conversion - should be a single message with multiple tool calls
	assert.Len(t, converted, 1)

	// Check the message has both tool calls
	assert.Equal(t, "assistant", converted[0].Role)
	assert.Len(t, converted[0].ToolCalls, 2)

	// Check first tool call
	assert.Equal(t, "call_123", converted[0].ToolCalls[0].ID)
	assert.Equal(t, "function", converted[0].ToolCalls[0].Type)
	assert.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"expression":"2 + 2"}`, converted[0].ToolCalls[0].Function.Arguments)

	// Check second tool call
	assert.Equal(t, "call_456", converted[0].ToolCalls[1].ID)
	assert.Equal(t, "function", converted[0].ToolCalls[1].Type)
	assert.Equal(t, "GoogleSearch", converted[0].ToolCalls[1].Function.Name)
	assert.Equal(t, `{"query":"math formulas"}`, converted[0].ToolCalls[1].Function.Arguments)
}

// Add a test for tool results
func TestConvertToolResultMessages(t *testing.T) {
	// Create a message with two ContentTypeToolResult content blocks
	message := &llm.Message{
		Role: "tool",
		Content: []llm.Content{
			&llm.ToolResultContent{
				Content:   "4",
				ToolUseID: "call_123",
			},
			&llm.ToolResultContent{
				Content:   "Found math formulas",
				ToolUseID: "call_456",
			},
		},
	}

	// Convert the message
	converted, err := convertMessages([]*llm.Message{message})
	assert.NoError(t, err)

	// Verify the conversion - should be two separate messages
	assert.Len(t, converted, 2)

	// Check first tool result message
	assert.Equal(t, "tool", converted[0].Role)
	assert.Equal(t, "4", converted[0].Content)
	assert.Equal(t, "call_123", converted[0].ToolCallID)

	// Check second tool result message
	assert.Equal(t, "tool", converted[1].Role)
	assert.Equal(t, "Found math formulas", converted[1].Content)
	assert.Equal(t, "call_456", converted[1].ToolCallID)
}

// Test for messages containing both text and tool use content
func TestConvertTextAndToolUseMessage(t *testing.T) {
	// Create a message with both text and tool use content blocks
	message := &llm.Message{
		Role: llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{
				Text: "I'll help you calculate that",
			},
			&llm.ToolUseContent{
				ID:    "call_123",
				Name:  "Calculator",
				Input: []byte(`{"expression":"2 + 2"}`),
			},
		},
	}

	// Convert the message
	converted, err := convertMessages(llm.Messages{message})
	assert.NoError(t, err)

	// Verify the conversion - should be a single message with text and tool call
	assert.Len(t, converted, 1)
	assert.Equal(t, "assistant", converted[0].Role)
	assert.Equal(t, "I'll help you calculate that", converted[0].Content)
	assert.Len(t, converted[0].ToolCalls, 1)
	assert.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)
}

// Test for tool use followed by tool result
func TestConvertToolUseAndResultMessages(t *testing.T) {
	// Create sequence of messages: first the assistant's tool use, then the tool result
	messages := []*llm.Message{
		{
			Role: llm.Assistant,
			Content: []llm.Content{
				&llm.ToolUseContent{
					ID:    "call_111",
					Name:  "Calculator",
					Input: []byte(`{"expression":"1 + 1"}`),
				},
				&llm.ToolUseContent{
					ID:    "call_999",
					Name:  "Calculator",
					Input: []byte(`{"expression":"2 + 2"}`),
				},
			},
		},
		{
			Role: llm.User,
			Content: []llm.Content{
				&llm.ToolResultContent{
					Content:   "1",
					ToolUseID: "call_111",
				},
				&llm.ToolResultContent{
					Content:   "2",
					ToolUseID: "call_999",
				},
			},
		},
	}

	// Convert the messages. The tool result content blocks are split across
	// two messages (how OpenAI does it).
	converted, err := convertMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, converted, 3)

	assert.Equal(t, "assistant", converted[0].Role)
	assert.Len(t, converted[0].ToolCalls, 2)
	assert.Equal(t, "call_111", converted[0].ToolCalls[0].ID)
	assert.Equal(t, "call_999", converted[0].ToolCalls[1].ID)

	assert.Equal(t, "tool", converted[1].Role)
	assert.Equal(t, "1", converted[1].Content)
	assert.Equal(t, "call_111", converted[1].ToolCallID)

	assert.Equal(t, "tool", converted[2].Role)
	assert.Equal(t, "2", converted[2].Content)
	assert.Equal(t, "call_999", converted[2].ToolCallID)

}
