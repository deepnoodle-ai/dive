package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/diveagents/dive/llm"
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
	require.Contains(t, strings.ToLower(response.Message.Text()), "hello")
}

func TestHelloWorldStream(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, []*llm.Message{
		llm.NewUserMessage("count to 10. respond with the integers only, separated by spaces."),
	})
	require.NoError(t, err)

	var events []*llm.Event
	for iterator.Next() {
		event := iterator.Event()
		events = append(events, event)
	}
	require.NoError(t, iterator.Err())

	var finalResponse *llm.Response
	var finalText string
	var texts []string
	for _, event := range events {
		if event.Response != nil {
			finalResponse = event.Response
		}
		switch event.Type {
		case llm.EventContentBlockDelta:
			if event.Delta.Type == "text_delta" {
				texts = append(texts, event.Delta.Text)
				finalText += event.Delta.Text
			}
		}
	}

	// We don't check the exact output since the model might format it differently
	require.NotEmpty(t, finalText)
	require.NotEmpty(t, texts)

	// Check that the response contains numbers 1-10 in some form
	for i := 1; i <= 10; i++ {
		require.Contains(t, finalText, fmt.Sprintf("%d", i))
	}

	require.NotNil(t, finalResponse)
	require.Equal(t, llm.Assistant, finalResponse.Role)
}

func addFunc(ctx context.Context, input string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", int(params["a"].(float64))+int(params["b"].(float64))), nil
}

func TestToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := []*llm.Message{
		llm.NewUserMessage("add 567 and 111"),
	}

	add := llm.ToolDefinition{
		Name:        "add",
		Description: "Returns the sum of two numbers, \"a\" and \"b\"",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*llm.SchemaProperty{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		},
	}

	response, err := provider.Generate(ctx, messages,
		llm.WithTools(llm.NewTool(&add, addFunc)),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)

	require.Equal(t, 1, len(response.Message.Content))
	content := response.Message.Content[0]
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

	add := llm.ToolDefinition{
		Name:        "add",
		Description: "Returns the sum of two numbers, \"a\" and \"b\"",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*llm.SchemaProperty{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		},
	}

	response, err := provider.Generate(ctx, messages,
		llm.WithTools(llm.NewTool(&add, addFunc)),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)
	require.Equal(t, 2, len(response.Message.Content))

	c1 := response.Message.Content[0]
	require.Equal(t, llm.ContentTypeToolUse, c1.Type)
	require.Equal(t, "add", c1.Name)
	require.Contains(t, string(c1.Input), "567")
	require.Contains(t, string(c1.Input), "111")

	c2 := response.Message.Content[1]
	require.Equal(t, llm.ContentTypeToolUse, c2.Type)
	require.Equal(t, "add", c2.Name)
	require.Contains(t, string(c2.Input), "233")
	require.Contains(t, string(c2.Input), "444")
}

func TestMultipleToolUseStreaming(t *testing.T) {
	ctx := context.Background()
	provider := New()

	messages := []*llm.Message{
		llm.NewUserMessage("Calculate two results for me: add 567 and 111, and add 233 and 444"),
	}

	add := llm.ToolDefinition{
		Name:        "add",
		Description: "Returns the sum of two numbers, \"a\" and \"b\"",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*llm.SchemaProperty{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		},
	}

	iterator, err := provider.Stream(ctx, messages,
		llm.WithTools(llm.NewTool(&add, addFunc)),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)

	var toolCalls []*llm.ToolCall
	for iterator.Next() {
		event := iterator.Event()
		if event.Response != nil {
			// fmt.Printf("response: %+v\n", event.Response)
			// fmt.Println("tool call count:", len(event.Response.ToolCalls()))
			toolCalls = append(toolCalls, event.Response.ToolCalls...)
		}
	}
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

	add := llm.ToolDefinition{
		Name:        "add",
		Description: "Returns the sum of two numbers, \"a\" and \"b\"",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: map[string]*llm.SchemaProperty{
				"a": {Type: "number", Description: "The first number"},
				"b": {Type: "number", Description: "The second number"},
			},
		},
	}

	iterator, err := provider.Stream(ctx, messages,
		llm.WithTools(llm.NewTool(&add, addFunc)),
		llm.WithToolChoice(llm.ToolChoiceAuto),
	)
	require.NoError(t, err)

	var events []*llm.Event
	for iterator.Next() {
		event := iterator.Event()
		events = append(events, event)
	}
	require.NoError(t, iterator.Err())

	var finalResponse *llm.Response
	var jsonParts []string
	for _, event := range events {
		if event.Response != nil {
			finalResponse = event.Response
		}
		switch event.Type {
		case llm.EventContentBlockDelta:
			if event.Delta.Type == "input_json_delta" {
				jsonParts = append(jsonParts, event.Delta.PartialJSON)
			}
		}
	}

	require.NotNil(t, finalResponse)
	require.Equal(t, llm.Assistant, finalResponse.Role)

	// Check that we have at least one tool call
	require.GreaterOrEqual(t, len(finalResponse.ToolCalls), 1)

	// Check that the tool call is for the add function
	toolCall := finalResponse.ToolCalls[0]
	require.Equal(t, "add", toolCall.Name)

	// Check that the arguments contain the numbers
	require.Contains(t, toolCall.Input, "567")
	require.Contains(t, toolCall.Input, "111")

	// Check that we received JSON parts
	require.Greater(t, len(jsonParts), 0)

	// Combine the JSON parts and check that they form valid JSON
	combinedJSON := strings.Join(jsonParts, "")
	var parsedJSON map[string]interface{}
	err = json.Unmarshal([]byte(combinedJSON), &parsedJSON)
	require.NoError(t, err)

	// Check that the parsed JSON contains the correct values
	require.Equal(t, float64(567), parsedJSON["a"])
	require.Equal(t, float64(111), parsedJSON["b"])
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
