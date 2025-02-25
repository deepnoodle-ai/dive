package openai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/tools"
	"github.com/stretchr/testify/require"
)

func TestHelloWorld(t *testing.T) {
	ctx := context.Background()
	provider := New()
	response, err := provider.Generate(ctx, []*llm.Message{
		llm.NewUserMessage("respond with \"hello\""),
	})
	require.NoError(t, err)

	// Check if the response contains "hello" (case insensitive)
	responseText := strings.ToLower(response.Message().Text())
	require.Contains(t, responseText, "hello", "Response should contain 'hello'")
}

func TestToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

	calculator := &tools.MockCalculatorTool{Result: "4"}

	response, err := provider.Generate(ctx, []*llm.Message{
		llm.NewUserMessage("What is 2 + 2?"),
	}, llm.WithTools(calculator))
	require.NoError(t, err)

	require.Len(t, response.ToolCalls(), 1)
	call := response.ToolCalls()[0]
	require.Equal(t, "Calculator", call.Name)
	require.Equal(t, `{"expression":"2 + 2"}`, call.Input)
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
				Text:      "4",
				ToolUseID: "call_123",
			},
			{
				Type:      llm.ContentTypeToolResult,
				Text:      "Found math formulas",
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

// Add a test for mixed content types
func TestConvertMixedContentMessages(t *testing.T) {
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
	converted, err := convertMessages([]*llm.Message{message})
	require.NoError(t, err)

	// Verify the conversion - should be a single message with text and tool call
	require.Len(t, converted, 1)
	require.Equal(t, "assistant", converted[0].Role)
	require.Equal(t, "I'll help you calculate that", converted[0].Content)
	require.Len(t, converted[0].ToolCalls, 1)
	require.Equal(t, "Calculator", converted[0].ToolCalls[0].Function.Name)

	// Create a message with text, tool use, and tool result content blocks
	mixedMessage := &llm.Message{
		Role: llm.Assistant,
		Content: []*llm.Content{
			{
				Type: llm.ContentTypeText,
				Text: "Here's the calculation",
			},
			{
				Type:  llm.ContentTypeToolUse,
				ID:    "call_789",
				Name:  "Calculator",
				Input: json.RawMessage(`{"expression":"3 + 3"}`),
			},
			{
				Type:      llm.ContentTypeToolResult,
				Text:      "6",
				ToolUseID: "call_789",
			},
		},
	}

	// Convert the message
	mixedConverted, err := convertMessages([]*llm.Message{mixedMessage})
	require.NoError(t, err)

	// Verify the conversion - should be two messages:
	// 1. Assistant message with text and tool call
	// 2. Tool message with result
	require.Len(t, mixedConverted, 2)

	// Check first message
	require.Equal(t, "assistant", mixedConverted[0].Role)
	require.Equal(t, "Here's the calculation", mixedConverted[0].Content)
	require.Len(t, mixedConverted[0].ToolCalls, 1)

	// Check second message (tool result)
	require.Equal(t, "tool", mixedConverted[1].Role)
	require.Equal(t, "6", mixedConverted[1].Content)
	require.Equal(t, "call_789", mixedConverted[1].ToolCallID)
}
