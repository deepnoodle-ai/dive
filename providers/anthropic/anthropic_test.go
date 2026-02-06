package anthropic

import (
	"context"
	"encoding/json"
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
	assert.Equal(t, "hello", response.Message().Text())
}

func TestStreamCountTo10(t *testing.T) {
	ctx := context.Background()
	provider := New()
	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("count to 10. respond with the integers only, separated by spaces."),
	))
	assert.NoError(t, err)
	defer iterator.Close()

	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
	}
	assert.NoError(t, iterator.Err())

	var accumulatedText string
	for _, event := range events {
		switch event.Type {
		case llm.EventTypeContentBlockDelta:
			accumulatedText += event.Delta.Text
		}
	}

	expectedOutput := "1 2 3 4 5 6 7 8 9 10"
	normalizedText := strings.Join(strings.Fields(accumulatedText), " ")
	assert.Equal(t, expectedOutput, normalizedText)
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
		llm.WithMessages(llm.NewUserTextMessage("add 567 and 111")),
		llm.WithTools(add),
		llm.WithToolChoice(&llm.ToolChoice{
			Type: llm.ToolChoiceTypeTool,
			Name: "add",
		}),
	)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(response.Message().Content))
	content := response.Message().Content[0]
	assert.Equal(t, llm.ContentTypeToolUse, content.Type())

	toolUse, ok := content.(*llm.ToolUseContent)
	assert.True(t, ok)
	assert.Equal(t, "add", toolUse.Name)
	assert.Equal(t, `{"a":567,"b":111}`, string(toolUse.Input))
}

func TestToolCallStream(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Define a simple calculator tool
	calculatorTool := llm.NewToolDefinition().
		WithName("calculator").
		WithDescription("Perform a calculation").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"operation", "a", "b"},
			Properties: map[string]*schema.Property{
				"operation": {
					Type:        "string",
					Description: "The operation to perform",
					Enum:        []any{"add", "subtract", "multiply", "divide"},
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

	assert.NoError(t, err)
	defer iterator.Close()

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
	assert.NotNil(t, response, "Should have received a final response")

	// Check if tool calls were properly processed
	toolCalls := response.ToolCalls()
	assert.Equal(t, 1, len(toolCalls))

	toolCall := toolCalls[0]
	assert.NotEmpty(t, toolCall.ID, "Tool call ID should not be empty")
	assert.NotEmpty(t, toolCall.Name, "Tool call name should not be empty")
	assert.NotEmpty(t, toolCall.Input, "Tool call input should not be empty")

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Input), &params); err != nil {
		t.Fatalf("Failed to unmarshal tool call input: %v", err)
	}

	assert.Equal(t, "add", params["operation"])
	assert.Equal(t, 2.0, params["a"])
	assert.Equal(t, 2.0, params["b"])
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
				assert.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				assert.True(t, ok, "Expected DocumentContent, got %T", msg.Content[0])
				assert.Equal(t, "test.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeBase64, docContent.Source.Type)
				assert.Equal(t, "application/pdf", docContent.Source.MediaType)
				assert.Equal(t, "JVBERi0xLjQK...", docContent.Source.Data)
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
				assert.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				assert.True(t, ok)
				assert.Equal(t, "remote.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeURL, docContent.Source.Type)
				assert.Equal(t, "https://example.com/document.pdf", docContent.Source.URL)
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
				assert.Len(t, msg.Content, 1)
				docContent, ok := msg.Content[0].(*llm.DocumentContent)
				assert.True(t, ok)
				assert.Equal(t, "api-file.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeFile, docContent.Source.Type)
				assert.Equal(t, "file-abc123", docContent.Source.FileID)
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
				assert.Len(t, msg.Content, 2)

				// First content should remain as TextContent
				textContent, ok := msg.Content[0].(*llm.TextContent)
				assert.True(t, ok)
				assert.Equal(t, "Please analyze this document:", textContent.Text)

				// Second content should remain as DocumentContent
				docContent, ok := msg.Content[1].(*llm.DocumentContent)
				assert.True(t, ok)
				assert.Equal(t, "report.pdf", docContent.Title)
				assert.NotNil(t, docContent.Source)
				assert.Equal(t, llm.ContentSourceTypeBase64, docContent.Source.Type)
				assert.Equal(t, "application/pdf", docContent.Source.MediaType)
				assert.Equal(t, "JVBERi0xLjQK...", docContent.Source.Data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages := []*llm.Message{tt.input}
			converted, err := convertMessages(messages)
			assert.NoError(t, err)
			assert.Len(t, converted, 1)
			tt.expected(t, converted[0])
		})
	}
}

func TestApplyCacheControlDoesNotMutateOriginal(t *testing.T) {
	// Build an original message with TextContent that has no CacheControl
	original := &llm.TextContent{Text: "hello"}
	messages := []*llm.Message{
		{Role: llm.User, Content: []llm.Content{original}},
	}

	// convertMessages should clone content, so applyCacheControl won't touch the original
	converted, err := convertMessages(messages)
	assert.NoError(t, err)

	config := &llm.Config{}
	applyCacheControl(converted, config)

	// The converted message's content should have cache control set
	setter, ok := converted[0].Content[0].(llm.CacheControlSetter)
	assert.True(t, ok)
	_ = setter // applyCacheControl sets it on the last content of the last message

	// The original content must NOT have been mutated
	assert.Nil(t, original.CacheControl)
}
