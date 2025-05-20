package openai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/schema"
	"github.com/stretchr/testify/require"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, []*llm.Message{
		llm.NewUserMessage("respond with \"hello\""),
	})
	require.NoError(t, err)
	// The model might respond with "Hello!" or other variations, so we check case-insensitive
	require.Contains(t, strings.ToLower(response.Message().Text()), "hello")
}

func TestHelloWorldStream(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, []*llm.Message{
		llm.NewUserMessage("count to 10. respond with the integers only, separated by spaces."),
	})
	require.NoError(t, err)

	accum := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accum.AddEvent(event); err != nil {
			require.NoError(t, err)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, accum.IsComplete())

	response := accum.Response()
	require.NotNil(t, response)
	require.Equal(t, llm.Assistant, response.Role)
	require.Equal(t, "1 2 3 4 5 6 7 8 9 10", response.Message().Text())
}

func TestToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := []*llm.Message{
		llm.NewUserMessage("add 567 and 111"),
	}

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	response, err := provider.Generate(ctx, messages,
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)

	require.Equal(t, 1, len(response.Message().Content))
	content := response.Message().Content[0]
	require.Equal(t, llm.ContentTypeToolUse, content.Type)
	require.Equal(t, "add", content.Name)
	// The exact format of the arguments may vary, so we just check that it contains the numbers
	require.Contains(t, string(content.Input), "567")
	require.Contains(t, string(content.Input), "111")
}

func TestMultipleToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := []*llm.Message{
		llm.NewUserMessage("Calculate two results for me: add 567 and 111, and add 233 and 444"),
	}

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	response, err := provider.Generate(ctx, messages,
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)
	require.Equal(t, 2, len(response.Message().Content))

	c1 := response.Message().Content[0]
	require.Equal(t, llm.ContentTypeToolUse, c1.Type)
	require.Equal(t, "add", c1.Name)
	require.Contains(t, string(c1.Input), "567")
	require.Contains(t, string(c1.Input), "111")

	c2 := response.Message().Content[1]
	require.Equal(t, llm.ContentTypeToolUse, c2.Type)
	require.Equal(t, "add", c2.Name)
	require.Contains(t, string(c2.Input), "233")
	require.Contains(t, string(c2.Input), "444")
}

func TestMultipleToolUseStreaming(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := llm.Messages{
		llm.NewUserMessage("Calculate two results for me: add 567 and 111, and add 233 and 444"),
	}

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	iterator, err := provider.Stream(ctx, messages,
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accumulator.AddEvent(event); err != nil {
			require.NoError(t, err)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	toolCalls := response.ToolCalls()
	require.Equal(t, 2, len(toolCalls))

	// The two calls can be in any order, so we need to check both

	var c1 *llm.ToolCall
	var c2 *llm.ToolCall

	if strings.Contains(string(toolCalls[0].Input), "567") {
		c1 = toolCalls[0]
		c2 = toolCalls[1]
	} else {
		c1 = toolCalls[1]
		c2 = toolCalls[0]
	}

	require.Equal(t, "add", c1.Name)
	require.Contains(t, string(c1.Input), "567")
	require.Contains(t, string(c1.Input), "111")

	require.Equal(t, "add", c2.Name)
	require.Contains(t, string(c2.Input), "233")
	require.Contains(t, string(c2.Input), "444")
}

func TestToolUseStream(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := []*llm.Message{
		llm.NewUserMessage("add 567 and 111"),
	}

	add := llm.NewToolDefinition().
		WithName("add").
		WithDescription("Returns the sum of two numbers, \"a\" and \"b\"").
		WithSchema(schema.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*schema.Property{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		})

	iterator, err := provider.Stream(ctx, messages,
		llm.WithTools(add),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)

	accumulator := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		if err := accumulator.AddEvent(event); err != nil {
			require.NoError(t, err)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, accumulator.IsComplete())

	response := accumulator.Response()
	toolCalls := response.ToolCalls()
	require.Equal(t, 1, len(toolCalls))

	require.NotNil(t, response)
	require.Equal(t, llm.Assistant, response.Role)

	// Check that we have at least one tool call
	require.GreaterOrEqual(t, len(response.ToolCalls()), 1)

	// Check that the tool call is for the add function
	toolCall := response.ToolCalls()[0]
	require.Equal(t, "add", toolCall.Name)

	// Check that the arguments contain the numbers
	require.Contains(t, toolCall.Input, "567")
	require.Contains(t, toolCall.Input, "111")
}

func TestConvertMessages(t *testing.T) {
	// Create a message with two ContentTypeToolUse content blocks
	message := &llm.Message{
		Role: llm.Assistant,
		Content: []*llm.Content{
			{
				Type:  llm.ContentTypeToolUse,
				ID:    "call_123",
				Name:  "Calculator",
				Input: json.RawMessage(`{"expression":"2 + 2"}`),
			},
			{
				Type:  llm.ContentTypeToolUse,
				ID:    "call_456",
				Name:  "GoogleSearch",
				Input: json.RawMessage(`{"query":"math formulas"}`),
			},
		},
	}

	// Convert the message
	converted, err := convertMessages([]*llm.Message{message})
	require.NoError(t, err)

	// Verify the conversion - should be a single message with multiple tool calls
	require.Len(t, converted, 1)

	// Check the message has both tool calls
	require.Equal(t, "assistant", converted[0].Role)
	require.Len(t, converted[0].ToolCalls, 2)

	// Check first tool call
	require.Equal(t, "call_123", converted[0].ToolCalls[0].ID)
	require.Equal(t, "function", converted[0].ToolCalls[0].Type)
	require.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)
	require.Equal(t, `{"expression":"2 + 2"}`, converted[0].ToolCalls[0].Function.Arguments)

	// Check second tool call
	require.Equal(t, "call_456", converted[0].ToolCalls[1].ID)
	require.Equal(t, "function", converted[0].ToolCalls[1].Type)
	require.Equal(t, "GoogleSearch", converted[0].ToolCalls[1].Function.Name)
	require.Equal(t, `{"query":"math formulas"}`, converted[0].ToolCalls[1].Function.Arguments)
}

// Add a test for tool results
func TestConvertToolResultMessages(t *testing.T) {
	// Create a message with two ContentTypeToolResult content blocks
	message := &llm.Message{
		Role: "tool",
		Content: []*llm.Content{
			{
				Type:      llm.ContentTypeToolResult,
				Content:   "4",
				ToolUseID: "call_123",
			},
			{
				Type:      llm.ContentTypeToolResult,
				Content:   "Found math formulas",
				ToolUseID: "call_456",
			},
		},
	}

	// Convert the message
	converted, err := convertMessages([]*llm.Message{message})
	require.NoError(t, err)

	// Verify the conversion - should be two separate messages
	require.Len(t, converted, 2)

	// Check first tool result message
	require.Equal(t, "tool", converted[0].Role)
	require.Equal(t, "4", converted[0].Content)
	require.Equal(t, "call_123", converted[0].ToolCallID)

	// Check second tool result message
	require.Equal(t, "tool", converted[1].Role)
	require.Equal(t, "Found math formulas", converted[1].Content)
	require.Equal(t, "call_456", converted[1].ToolCallID)
}

// Test for messages containing both text and tool use content
func TestConvertTextAndToolUseMessage(t *testing.T) {
	// Create a message with both text and tool use content blocks
	message := &llm.Message{
		Role: llm.Assistant,
		Content: []*llm.Content{
			{
				Type: llm.ContentTypeText,
				Text: "I'll help you calculate that",
			},
			{
				Type:  llm.ContentTypeToolUse,
				ID:    "call_123",
				Name:  "Calculator",
				Input: json.RawMessage(`{"expression":"2 + 2"}`),
			},
		},
	}

	// Convert the message
	converted, err := convertMessages(llm.Messages{message})
	require.NoError(t, err)

	// Verify the conversion - should be a single message with text and tool call
	require.Len(t, converted, 1)
	require.Equal(t, "assistant", converted[0].Role)
	require.Equal(t, "I'll help you calculate that", converted[0].Content)
	require.Len(t, converted[0].ToolCalls, 1)
	require.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)
}

// Test for tool use followed by tool result
func TestConvertToolUseAndResultMessages(t *testing.T) {
	// Create sequence of messages: first the assistant's tool use, then the tool result
	messages := []*llm.Message{
		{
			Role: llm.Assistant,
			Content: []*llm.Content{
				{
					Type:  llm.ContentTypeToolUse,
					ID:    "call_111",
					Name:  "Calculator",
					Input: json.RawMessage(`{"expression":"1 + 1"}`),
				},
				{
					Type:  llm.ContentTypeToolUse,
					ID:    "call_999",
					Name:  "Calculator",
					Input: json.RawMessage(`{"expression":"2 + 2"}`),
				},
			},
		},
		{
			Role: llm.User,
			Content: []*llm.Content{
				{
					Type:      llm.ContentTypeToolResult,
					Content:   "1",
					ToolUseID: "call_111",
				},
				{
					Type:      llm.ContentTypeToolResult,
					Content:   "2",
					ToolUseID: "call_999",
				},
			},
		},
	}

	// Convert the messages. The tool result content blocks are split across
	// two messages (how OpenAI does it).
	converted, err := convertMessages(messages)
	require.NoError(t, err)
	require.Len(t, converted, 3)

	require.Equal(t, "assistant", converted[0].Role)
	require.Len(t, converted[0].ToolCalls, 2)
	require.Equal(t, "call_111", converted[0].ToolCalls[0].ID)
	require.Equal(t, "call_999", converted[0].ToolCalls[1].ID)

	require.Equal(t, "tool", converted[1].Role)
	require.Equal(t, "1", converted[1].Content)
	require.Equal(t, "call_111", converted[1].ToolCallID)

	require.Equal(t, "tool", converted[2].Role)
	require.Equal(t, "2", converted[2].Content)
	require.Equal(t, "call_999", converted[2].ToolCallID)

}
