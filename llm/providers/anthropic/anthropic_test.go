package anthropic

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
	require.Equal(t, "hello", response.Message.Text())
}

func TestHelloWorldStream(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, []*llm.Message{
		llm.NewUserMessage("count to 10. respond with the integers only, separated by spaces."),
	})
	require.NoError(t, err)
	defer iterator.Close()

	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
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
			numbers := strings.FieldsFunc(event.Delta.Text, func(r rune) bool {
				return r == '\n' || r == ' '
			})
			texts = append(texts, numbers...)
			finalText += event.Delta.Text
		}
	}

	expectedOutput := "1 2 3 4 5 6 7 8 9 10"
	normalizedFinalText := strings.Join(strings.Fields(finalText), " ")
	normalizedTexts := strings.Join(texts, " ")

	require.Equal(t, expectedOutput, normalizedFinalText)
	require.Equal(t, expectedOutput, normalizedTexts)
	require.NotNil(t, finalResponse)
	require.Equal(t, llm.Assistant, finalResponse.Role)

	require.Len(t, finalResponse.Message.Content, 1)
	content := finalResponse.Message.Content[0]
	require.Equal(t, llm.ContentTypeText, content.Type)
	require.Equal(t, finalText, content.Text)

	usage := finalResponse.Usage
	require.NotNil(t, usage)
	require.True(t, usage.OutputTokens > 0)
	require.True(t, usage.InputTokens > 0)
}

func addFunc(ctx context.Context, input string) (string, error) {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", params["a"].(int)+params["b"].(int)), nil
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
		llm.WithToolChoice(llm.ToolChoice{
			Type: "tool",
			Name: "add",
		}),
	)
	require.NoError(t, err)

	require.Equal(t, 1, len(response.Message.Content))
	content := response.Message.Content[0]
	require.Equal(t, llm.ContentTypeToolUse, content.Type)
	require.Equal(t, "add", content.Name)
	require.Equal(t, `{"a":567,"b":111}`, string(content.Input))
}

func TestToolCallStream(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Define a simple calculator tool
	calculatorTool := llm.NewTool(&llm.ToolDefinition{
		Name:        "calculator",
		Description: "Perform a calculation",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"operation", "a", "b"},
			Properties: map[string]*llm.SchemaProperty{
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
		},
	}, func(ctx context.Context, input string) (string, error) {
		return "4", nil // Mock result
	})

	iterator, err := provider.Stream(ctx,
		[]*llm.Message{llm.NewUserMessage("What is 2+2?")},
		llm.WithTools(calculatorTool))

	require.NoError(t, err)
	defer iterator.Close()

	var finalResponse *llm.Response
	for iterator.Next() {
		event := iterator.Event()
		if event.Response != nil {
			finalResponse = event.Response
		}
	}
	require.NoError(t, iterator.Err())
	require.NotNil(t, finalResponse, "Should have received a final response")

	// Check if tool calls were properly processed
	toolCalls := finalResponse.ToolCalls
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
