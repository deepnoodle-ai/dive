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

func TestDocumentContentHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    *llm.Message
		expected func(*testing.T, *llm.Message)
	}{
		{
			name: "DocumentContent with base64 data",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.DocumentContent{
						Title: "test.pdf",
						Source: &llm.ContentSource{
							Type:      llm.ContentSourceTypeBase64,
							MediaType: "application/pdf",
							Data:      "JVBERi0xLjQK...",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				require.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				require.True(t, ok, "Expected DocumentContent, got %T", msg.Content[0])
				require.Equal(t, "test.pdf", docContent.Title)
				require.NotNil(t, docContent.Source)
				require.Equal(t, llm.ContentSourceTypeBase64, docContent.Source.Type)
				require.Equal(t, "application/pdf", docContent.Source.MediaType)
				require.Equal(t, "JVBERi0xLjQK...", docContent.Source.Data)
			},
		},
		{
			name: "DocumentContent with URL",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.DocumentContent{
						Title: "remote.pdf",
						Source: &llm.ContentSource{
							Type: llm.ContentSourceTypeURL,
							URL:  "https://example.com/document.pdf",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				require.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				require.True(t, ok)
				require.Equal(t, "remote.pdf", docContent.Title)
				require.NotNil(t, docContent.Source)
				require.Equal(t, llm.ContentSourceTypeURL, docContent.Source.Type)
				require.Equal(t, "https://example.com/document.pdf", docContent.Source.URL)
			},
		},
		{
			name: "DocumentContent with file ID",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.DocumentContent{
						Title: "api-file.pdf",
						Source: &llm.ContentSource{
							Type:   llm.ContentSourceTypeFile,
							FileID: "file-abc123",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				require.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				require.True(t, ok)
				require.Equal(t, "api-file.pdf", docContent.Title)
				require.NotNil(t, docContent.Source)
				require.Equal(t, llm.ContentSourceTypeFile, docContent.Source.Type)
				require.Equal(t, "file-abc123", docContent.Source.FileID)
			},
		},
		{
			name: "Mixed content with DocumentContent and TextContent",
			input: &llm.Message{
				Role: llm.User,
				Content: []llm.Content{
					&llm.TextContent{Text: "Please analyze this document:"},
					&llm.DocumentContent{
						Title: "report.pdf",
						Source: &llm.ContentSource{
							Type:      llm.ContentSourceTypeBase64,
							MediaType: "application/pdf",
							Data:      "JVBERi0xLjQK...",
						},
					},
				},
			},
			expected: func(t *testing.T, msg *llm.Message) {
				require.Len(t, msg.Content, 2)

				// First content should remain as TextContent
				textContent, ok := msg.Content[0].(*llm.TextContent)
				require.True(t, ok)
				require.Equal(t, "Please analyze this document:", textContent.Text)

				// Second content should remain as DocumentContent
				docContent, ok := msg.Content[1].(*llm.DocumentContent)
				require.True(t, ok)
				require.Equal(t, "report.pdf", docContent.Title)
				require.NotNil(t, docContent.Source)
				require.Equal(t, llm.ContentSourceTypeBase64, docContent.Source.Type)
				require.Equal(t, "application/pdf", docContent.Source.MediaType)
				require.Equal(t, "JVBERi0xLjQK...", docContent.Source.Data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []*llm.Message{tt.input}
			converted, err := convertMessages(messages)
			require.NoError(t, err)
			require.Len(t, converted, 1)
			tt.expected(t, converted[0])
		})
	}
}
