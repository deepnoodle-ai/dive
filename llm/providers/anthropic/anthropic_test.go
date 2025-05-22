package anthropic

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
	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\""),
	))
	require.NoError(t, err)
	require.Equal(t, "hello", response.Message().Text())
}

func TestStreamCountTo10(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("count to 10. respond with the integers only, separated by spaces."),
	))
	require.NoError(t, err)
	defer iterator.Close()

	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
	}
	require.NoError(t, iterator.Err())

	var accumulatedText string
	for _, event := range events {
		switch event.Type {
		case llm.EventTypeContentBlockDelta:
			accumulatedText += event.Delta.Text
		}
	}

	expectedOutput := "1 2 3 4 5 6 7 8 9 10"
	normalizedText := strings.Join(strings.Fields(accumulatedText), " ")
	require.Equal(t, expectedOutput, normalizedText)
}

func TestToolUse(t *testing.T) {
	ctx := context.Background()
	provider := New()

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

	response, err := provider.Generate(ctx,
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithTools(add),
		llm.WithToolChoice("tool"),
		llm.WithToolChoiceName("add"),
	)
	require.NoError(t, err)

	require.Equal(t, 1, len(response.Message().Content))
	content := response.Message().Content[0]
	require.Equal(t, llm.ContentTypeToolUse, content.Type())

	toolUse, ok := content.(*llm.ToolUseContent)
	require.True(t, ok)
	require.Equal(t, "add", toolUse.Name)
	require.Equal(t, `{"a":567,"b":111}`, string(toolUse.Input))
}

func TestToolCallStream(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Define a simple calculator tool
	calculatorTool := llm.NewToolDefinition().
		WithName("calculator").
		WithDescription("Perform a calculation").
		WithSchema(schema.Schema{
			Type:     "object",
			Required: []string{"operation", "a", "b"},
			Properties: map[string]*schema.Property{
				"operation": {
					Type:        "string",
					Description: "The operation to perform",
					Enum:        []string{"add", "subtract", "multiply", "divide"},
				},
				"a": {
					Type:        "number",
					Description: "The first operand",
				},
				"b": {
					Type:        "number",
					Description: "The second operand",
				},
			},
		})

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("What is 2+2?")),
		llm.WithTools(calculatorTool),
	)

	require.NoError(t, err)
	defer iterator.Close()

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
	require.NotNil(t, response, "Should have received a final response")

	// Check if tool calls were properly processed
	toolCalls := response.ToolCalls()
	require.Equal(t, 1, len(toolCalls))

	toolCall := toolCalls[0]
	require.NotEmpty(t, toolCall.ID, "Tool call ID should not be empty")
	require.NotEmpty(t, toolCall.Name, "Tool call name should not be empty")
	require.NotEmpty(t, toolCall.Input, "Tool call input should not be empty")

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Input), &params); err != nil {
		t.Fatalf("Failed to unmarshal tool call input: %v", err)
	}

	require.Equal(t, "add", params["operation"])
	require.Equal(t, 2.0, params["a"])
	require.Equal(t, 2.0, params["b"])
}
