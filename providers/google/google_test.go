package google

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/schema"
	"github.com/deepnoodle-ai/wonton/assert"
)

// hasGoogleAPIKey returns true if a Google API key is available
func hasGoogleAPIKey() bool {
	return os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != ""
}

// requireGoogleAPIKey skips the test if no API key is available
func requireGoogleAPIKey(t *testing.T) {
	if !hasGoogleAPIKey() {
		t.Skip("Skipping test: GOOGLE_API_KEY or GEMINI_API_KEY not set")
	}
}

// createProvider creates a provider with a timeout for testing
func createProvider(t *testing.T) *Provider {
	requireGoogleAPIKey(t)

	provider := New(WithModel(ModelGemini25Flash))
	assert.NotNil(t, provider)

	// Test that the provider can be initialized (which requires API connectivity)
	// This will fail if the API key is invalid or network is unavailable
	return provider
}

func TestProviderName(t *testing.T) {
	provider := New()
	assert.Equal(t, "google", provider.Name())
}

func TestProviderBasicGenerate(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	provider := createProvider(t)

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\" in uppercase"),
	))
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, llm.Assistant, response.Role)
	assert.True(t, len(response.Content) > 0)

	// Check that we got some text content
	foundText := ""
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok && textContent.Text != "" {
			foundText = textContent.Text
			break
		}
	}
	assert.True(t, foundText != "", "Should have received text content")
	assert.Equal(t, "HELLO", foundText)
}

// func TestProviderBasicStream(t *testing.T) {
// 	requireGoogleAPIKey(t)

// 	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
// 	defer cancel()

// 	provider := createProvider(t)

// 	iterator, err := provider.Stream(ctx, llm.WithMessages(
// 		llm.NewUserTextMessage("Say 'hello'"),
// 	))
// 	assert.NoError(t, err)
// 	assert.NotNil(t, iterator)

// 	defer iterator.Close()

// 	var events []*llm.Event
// 	var textContent strings.Builder

// 	// Collect all events
// 	eventCount := 0
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		assert.NotNil(t, event)
// 		events = append(events, event)
// 		eventCount++

// 		// Debug: print event types
// 		t.Logf("Event %d: Type=%s", eventCount, event.Type)

// 		// Collect text content from deltas
// 		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
// 			textContent.WriteString(event.Delta.Text)
// 			t.Logf("Delta text: %q", event.Delta.Text)
// 		}
// 	}

// 	err = iterator.Err()
// 	if err != nil {
// 		t.Logf("Iterator error: %v", err)
// 	}
// 	assert.NoError(t, err)
// 	assert.True(t, len(events) > 0, "Should have received at least some events")

// 	// Should have received some text content
// 	content := textContent.String()
// 	t.Logf("Total content received: %q", content)
// 	assert.True(t, len(content) > 0, "Should have received some text content")

// 	// Basic validation that we got a response
// 	assert.Contains(t, strings.ToLower(content), "hello", "Response should contain 'hello'")
// }

func TestProviderOptions(t *testing.T) {
	provider := New(
		WithProjectID("test-project"),
		WithLocation("us-central1"),
		WithModel("gemini-pro"),
		WithMaxTokens(1000),
	)

	assert.Equal(t, "test-project", provider.projectID)
	assert.Equal(t, "us-central1", provider.location)
	assert.Equal(t, "gemini-pro", provider.model)
	assert.Equal(t, 1000, provider.maxTokens)
}

// func TestConvertMessages(t *testing.T) {
// 	messages := []*llm.Message{
// 		llm.NewUserTextMessage("Hello"),
// 		llm.NewAssistantTextMessage("Hi there!"),
// 	}

// 	converted, err := convertMessages(messages)
// 	assert.NoError(t, err)
// 	assert.Len(t, converted, 2)

// 	// Check role conversion
// 	assert.Equal(t, llm.User, converted[0].Role)
// 	assert.Equal(t, llm.Role("model"), converted[1].Role) // Google uses "model" instead of "assistant"
// }

// func TestStreamingTextGeneration(t *testing.T) {

// 	ctx := context.Background()
// 	provider := New()

// 	iterator, err := provider.Stream(ctx, llm.WithMessages(
// 		llm.NewUserTextMessage("Count from 1 to 5 slowly, one number per line."),
// 	))
// 	assert.NoError(t, err)
// 	assert.NotNil(t, iterator)
// 	defer iterator.Close()

// 	var events []*llm.Event
// 	var textContent strings.Builder

// 	// Collect all events
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		assert.NotNil(t, event)
// 		events = append(events, event)

// 		// Collect text content from deltas
// 		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
// 			textContent.WriteString(event.Delta.Text)
// 		}
// 	}
// 	assert.NoError(t, iterator.Err())
// 	assert.True(t, len(events) > 0, "Should have received at least some events")

// 	// Validate event sequence
// 	var messageStartCount, contentBlockStartCount, deltaCount, contentBlockStopCount, messageStopCount int
// 	for _, event := range events {
// 		switch event.Type {
// 		case llm.EventTypeMessageStart:
// 			messageStartCount++
// 			assert.NotNil(t, event.Message)
// 			assert.Equal(t, llm.Assistant, event.Message.Role)
// 		case llm.EventTypeContentBlockStart:
// 			contentBlockStartCount++
// 			assert.NotNil(t, event.Index)
// 			assert.NotNil(t, event.ContentBlock)
// 		case llm.EventTypeContentBlockDelta:
// 			deltaCount++
// 			assert.NotNil(t, event.Index)
// 			assert.NotNil(t, event.Delta)
// 		case llm.EventTypeContentBlockStop:
// 			contentBlockStopCount++
// 			assert.NotNil(t, event.Index)
// 		case llm.EventTypeMessageStop:
// 			messageStopCount++
// 		}
// 	}

// 	// Should have proper event counts
// 	assert.Equal(t, 1, messageStartCount, "Should have exactly one message start event")
// 	assert.Equal(t, 1, contentBlockStartCount, "Should have exactly one content block start event")
// 	assert.True(t, deltaCount > 0, "Should have at least one delta event")
// 	assert.Equal(t, 1, contentBlockStopCount, "Should have exactly one content block stop event")
// 	assert.Equal(t, 1, messageStopCount, "Should have exactly one message stop event")

// 	// Should have received some text content
// 	assert.True(t, textContent.Len() > 0, "Should have received some text content")

// 	// Content should contain numbers (basic validation)
// 	content := textContent.String()
// 	assert.Contains(t, content, "1", "Response should contain the number 1")
// }

// func TestStreamingWithConversationHistory(t *testing.T) {

// 	ctx := context.Background()
// 	provider := New()

// 	iterator, err := provider.Stream(ctx, llm.WithMessages(
// 		llm.NewUserTextMessage("What is 2 + 2?"),
// 		llm.NewAssistantTextMessage("2 + 2 = 4"),
// 		llm.NewUserTextMessage("What about 3 + 3?"),
// 	))
// 	assert.NoError(t, err)
// 	assert.NotNil(t, iterator)
// 	defer iterator.Close()

// 	var events []*llm.Event
// 	var textContent strings.Builder

// 	// Collect all events
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		assert.NotNil(t, event)
// 		events = append(events, event)

// 		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
// 			textContent.WriteString(event.Delta.Text)
// 		}
// 	}
// 	assert.NoError(t, iterator.Err())
// 	assert.True(t, len(events) > 0, "Should have received events")

// 	// Should have proper event sequence
// 	assert.True(t, len(events) >= 5, "Should have at least message start, content start, deltas, content stop, message stop")

// 	// Should have received meaningful content
// 	content := textContent.String()
// 	assert.True(t, len(content) > 0, "Should have received some content")
// }

// func TestStreamingWithSystemPrompt(t *testing.T) {

// 	ctx := context.Background()
// 	provider := New()

// 	systemPrompt := "You are a helpful assistant that always responds in uppercase letters."
// 	iterator, err := provider.Stream(ctx,
// 		llm.WithMessages(llm.NewUserTextMessage("Say hello")),
// 		llm.WithSystemPrompt(systemPrompt),
// 	)
// 	assert.NoError(t, err)
// 	assert.NotNil(t, iterator)
// 	defer iterator.Close()

// 	var events []*llm.Event
// 	var textContent strings.Builder

// 	// Collect all events
// 	for iterator.Next() {
// 		event := iterator.Event()
// 		assert.NotNil(t, event)
// 		events = append(events, event)

// 		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
// 			textContent.WriteString(event.Delta.Text)
// 		}
// 	}
// 	assert.NoError(t, iterator.Err())
// 	assert.True(t, len(events) > 0, "Should have received events")

// 	// Check that we received content
// 	content := textContent.String()
// 	assert.True(t, len(content) > 0, "Should have received some content")

// 	// Content should be uppercase (basic validation of system prompt effect)
// 	// Note: This is a loose check since the model might not always follow instructions perfectly
// 	upperContent := strings.ToUpper(content)
// 	assert.Equal(t, content, upperContent, "Content should be uppercase based on system prompt")
// }

func TestToolCall(t *testing.T) {
	ctx := context.Background()
	provider := createProvider(t)

	tool := llm.NewToolDefinition().
		WithName("get_weather").
		WithDescription("Get weather information").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"location"},
			Properties: map[string]*schema.Property{
				"location": {Type: "string", Description: "The location to get weather for"},
			},
		})

	response, err := provider.Generate(ctx,
		llm.WithMessages(llm.NewUserTextMessage("What's the weather in Tokyo?")),
		llm.WithTools(tool),
	)
	assert.NoError(t, err)

	assert.NotNil(t, response, "Should have received response")
	assert.Equal(t, llm.Assistant, response.Role, "Response should be assistant")
	assert.Len(t, response.Content, 1, "Response should have 1 content")

	calls := response.ToolCalls()
	assert.Len(t, calls, 1, "Response should have 1 tool call")
	assert.Equal(t, "get_weather", calls[0].Name, "Tool call should be for get_weather")
	assert.Contains(t, string(calls[0].Input), "Tokyo", "Tool call should be for Tokyo")
}

func TestToolCallWithResult(t *testing.T) {
	ctx := context.Background()
	provider := createProvider(t)

	// Create a simple tool for testing
	tool := llm.NewToolDefinition().
		WithName("get_weather").
		WithDescription("Get weather information").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"location"},
			Properties: map[string]*schema.Property{
				"location": {Type: "string", Description: "The location to get weather for"},
			},
		})

	// First call - should generate a tool call
	response1, err := provider.Generate(ctx,
		llm.WithMessages(llm.NewUserTextMessage("What's the weather in Tokyo?")),
		llm.WithTools(tool),
	)
	assert.NoError(t, err)
	assert.NotNil(t, response1)

	calls := response1.ToolCalls()
	assert.Len(t, calls, 1, "Should have 1 tool call")
	assert.Equal(t, "get_weather", calls[0].Name)

	// Create a conversation with the tool result
	toolResult := &llm.ToolResultContent{
		ToolUseID: calls[0].ID,
		Content:   `{"temperature": "22°C", "condition": "sunny"}`, // Use string, not []byte
		IsError:   false,
	}

	messages := []*llm.Message{
		llm.NewUserTextMessage("What's the weather in Tokyo?"),
		response1.Message(), // The assistant message with tool call
		llm.NewToolResultMessage(toolResult),
	}

	// Second call - should handle the tool result and continue the conversation
	// This is where the bug was occurring
	response2, err := provider.Generate(ctx,
		llm.WithMessages(messages...),
		llm.WithTools(tool),
	)
	assert.NoError(t, err, "Should handle tool results without error")
	assert.NotNil(t, response2)
	assert.Equal(t, llm.Assistant, response2.Role)

	// The response should contain text describing the weather
	textContent := ""
	for _, content := range response2.Content {
		if text, ok := content.(*llm.TextContent); ok {
			textContent += text.Text
		}
	}
	assert.NotEmpty(t, textContent, "Should have text response after tool result")
	assert.Contains(t, strings.ToLower(textContent), "tokyo", "Response should mention Tokyo")
}

func TestStreamingToolCalls(t *testing.T) {
	ctx := context.Background()
	provider := createProvider(t)

	// Create a simple tool for testing
	tool := llm.NewToolDefinition().
		WithName("get_weather").
		WithDescription("Get weather information").
		WithSchema(&schema.Schema{
			Type:     "object",
			Required: []string{"location"},
			Properties: map[string]*schema.Property{
				"location": {Type: "string", Description: "The location to get weather for"},
			},
		})

	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("What's the weather in Tokyo?")),
		llm.WithTools(tool),
	)
	assert.NoError(t, err)
	assert.NotNil(t, iterator)
	defer iterator.Close()

	accum := llm.NewResponseAccumulator()
	for iterator.Next() {
		accum.AddEvent(iterator.Event())
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accum.IsComplete(), "Should have received complete response")

	response := accum.Response()
	assert.NotNil(t, response, "Should have received response")
	assert.Equal(t, llm.Assistant, response.Role, "Response should be assistant")
	assert.Len(t, response.Content, 1, "Response should have 1 content")

	calls := response.ToolCalls()
	assert.Len(t, calls, 1, "Response should have 1 tool call")
	assert.Equal(t, "get_weather", calls[0].Name, "Tool call should be for get_weather")
	assert.Contains(t, string(calls[0].Input), "Tokyo", "Tool call should be for Tokyo")
}

func TestStreamingIteratorCleanup(t *testing.T) {
	ctx := context.Background()
	provider := createProvider(t)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'test'"),
	))
	assert.NoError(t, err)
	assert.NotNil(t, iterator)

	err = iterator.Close()
	assert.NoError(t, err)

	assert.False(t, iterator.Next(), "Next should return false after Close")
	assert.NoError(t, iterator.Err(), "Err should work after Close")
	assert.Nil(t, iterator.Event(), "Event should return nil after Close")
}

func TestStreamingErrorHandling(t *testing.T) {
	ctx := context.Background()
	provider := createProvider(t)

	// Test with empty messages (should handle gracefully)
	iterator, err := provider.Stream(ctx, llm.WithMessages())
	assert.Error(t, err, "Should return error for empty messages")
	assert.Nil(t, iterator)
}

func TestStreamingIntegration(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	provider := createProvider(t)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Write a short poem about coding, exactly 4 lines long."),
	))
	assert.NoError(t, err)
	assert.NotNil(t, iterator)
	defer iterator.Close()

	// Accumulate the response
	accum := llm.NewResponseAccumulator()
	for iterator.Next() {
		event := iterator.Event()
		assert.NotNil(t, event)
		accum.AddEvent(event)
	}
	assert.NoError(t, iterator.Err())
	assert.True(t, accum.IsComplete(), "Should have received complete response")

	response := accum.Response()
	assert.NotNil(t, response, "Should have received response")
	assert.Len(t, response.Content, 1, "Response should have 1 content")

	content, ok := response.Content[0].(*llm.TextContent)
	assert.True(t, ok, "Content should be text")
	assert.Equal(t, llm.Assistant, response.Role, "Response should be assistant")
	assert.Equal(t, llm.ContentTypeText, content.Type(), "Response should be text")

	lines := strings.Split(content.Text, "\n")
	assert.True(t, len(lines) >= 4, "Response should be at least 4 lines")
	hasKeyword := false
	for _, keyword := range []string{"logic", "line", "digital", "dreams", "ideas", "art", "code", "program", "think"} {
		if strings.Contains(strings.ToLower(content.Text), keyword) {
			hasKeyword = true
			break
		}
	}
	assert.True(t, hasKeyword, "Response should contain one of the keywords")
}
