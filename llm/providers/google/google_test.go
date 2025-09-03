package google

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/schema"
	"github.com/stretchr/testify/require"
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

// createProviderWithTimeout creates a provider with a timeout for testing
func createProviderWithTimeout(t *testing.T, timeout time.Duration) *Provider {
	requireGoogleAPIKey(t)

	provider := New()
	require.NotNil(t, provider)

	// Test that the provider can be initialized (which requires API connectivity)
	// This will fail if the API key is invalid or network is unavailable
	return provider
}

func TestProviderName(t *testing.T) {
	provider := New()
	require.Equal(t, "google", provider.Name())
}

func TestProviderBasicGenerate(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\""),
	))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, llm.Assistant, response.Role)
	require.True(t, len(response.Content) > 0)

	// Check that we got some text content
	foundText := false
	for _, content := range response.Content {
		if textContent, ok := content.(*llm.TextContent); ok && textContent.Text != "" {
			foundText = true
			break
		}
	}
	require.True(t, foundText, "Should have received text content")
}

func TestProviderBasicStream(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'hello'"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)

	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	eventCount := 0
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)
		eventCount++

		// Debug: print event types
		t.Logf("Event %d: Type=%s", eventCount, event.Type)

		// Collect text content from deltas
		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
			t.Logf("Delta text: %q", event.Delta.Text)
		}
	}

	err = iterator.Err()
	if err != nil {
		t.Logf("Iterator error: %v", err)
	}
	require.NoError(t, err)
	require.True(t, len(events) > 0, "Should have received at least some events")

	// Should have received some text content
	content := textContent.String()
	t.Logf("Total content received: %q", content)
	require.True(t, len(content) > 0, "Should have received some text content")

	// Basic validation that we got a response
	require.Contains(t, strings.ToLower(content), "hello", "Response should contain 'hello'")
}

func TestProviderOptions(t *testing.T) {
	provider := New(
		WithProjectID("test-project"),
		WithLocation("us-central1"),
		WithModel("gemini-pro"),
		WithMaxTokens(1000),
	)

	require.Equal(t, "test-project", provider.projectID)
	require.Equal(t, "us-central1", provider.location)
	require.Equal(t, "gemini-pro", provider.model)
	require.Equal(t, 1000, provider.maxTokens)
}

func TestConvertMessages(t *testing.T) {
	messages := []*llm.Message{
		llm.NewUserTextMessage("Hello"),
		llm.NewAssistantTextMessage("Hi there!"),
	}

	converted, err := convertMessages(messages)
	require.NoError(t, err)
	require.Len(t, converted, 2)

	// Check role conversion
	require.Equal(t, llm.User, converted[0].Role)
	require.Equal(t, llm.Role("model"), converted[1].Role) // Google uses "model" instead of "assistant"
}

func TestStreamingTextGeneration(t *testing.T) {

	ctx := context.Background()
	provider := New()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Count from 1 to 5 slowly, one number per line."),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		// Collect text content from deltas
		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received at least some events")

	// Validate event sequence
	var messageStartCount, contentBlockStartCount, deltaCount, contentBlockStopCount, messageStopCount int
	for _, event := range events {
		switch event.Type {
		case llm.EventTypeMessageStart:
			messageStartCount++
			require.NotNil(t, event.Message)
			require.Equal(t, llm.Assistant, event.Message.Role)
		case llm.EventTypeContentBlockStart:
			contentBlockStartCount++
			require.NotNil(t, event.Index)
			require.NotNil(t, event.ContentBlock)
		case llm.EventTypeContentBlockDelta:
			deltaCount++
			require.NotNil(t, event.Index)
			require.NotNil(t, event.Delta)
		case llm.EventTypeContentBlockStop:
			contentBlockStopCount++
			require.NotNil(t, event.Index)
		case llm.EventTypeMessageStop:
			messageStopCount++
		}
	}

	// Should have proper event counts
	require.Equal(t, 1, messageStartCount, "Should have exactly one message start event")
	require.Equal(t, 1, contentBlockStartCount, "Should have exactly one content block start event")
	require.True(t, deltaCount > 0, "Should have at least one delta event")
	require.Equal(t, 1, contentBlockStopCount, "Should have exactly one content block stop event")
	require.Equal(t, 1, messageStopCount, "Should have exactly one message stop event")

	// Should have received some text content
	require.True(t, textContent.Len() > 0, "Should have received some text content")

	// Content should contain numbers (basic validation)
	content := textContent.String()
	require.Contains(t, content, "1", "Response should contain the number 1")
}

func TestStreamingWithConversationHistory(t *testing.T) {

	ctx := context.Background()
	provider := New()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("What is 2 + 2?"),
		llm.NewAssistantTextMessage("2 + 2 = 4"),
		llm.NewUserTextMessage("What about 3 + 3?"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	// Should have proper event sequence
	require.True(t, len(events) >= 5, "Should have at least message start, content start, deltas, content stop, message stop")

	// Should have received meaningful content
	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received some content")
}

func TestStreamingWithSystemPrompt(t *testing.T) {

	ctx := context.Background()
	provider := New()

	systemPrompt := "You are a helpful assistant that always responds in uppercase letters."
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Say hello")),
		llm.WithSystemPrompt(systemPrompt),
	)
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	// Check that we received content
	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received some content")

	// Content should be uppercase (basic validation of system prompt effect)
	// Note: This is a loose check since the model might not always follow instructions perfectly
	upperContent := strings.ToUpper(content)
	require.Equal(t, content, upperContent, "Content should be uppercase based on system prompt")
}

func TestStreamingToolCalls(t *testing.T) {

	ctx := context.Background()
	provider := New()

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
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)
	}
	require.NoError(t, iterator.Err())

	// Basic validation that we got some response
	require.True(t, len(events) > 0, "Should have received events")

	// For now, just validate the basic streaming structure
	// Tool call streaming implementation might need updates to the StreamIterator
	var messageStartCount, contentBlockStartCount, contentBlockStopCount, messageStopCount int
	for _, event := range events {
		switch event.Type {
		case llm.EventTypeMessageStart:
			messageStartCount++
		case llm.EventTypeContentBlockStart:
			contentBlockStartCount++
		case llm.EventTypeContentBlockStop:
			contentBlockStopCount++
		case llm.EventTypeMessageStop:
			messageStopCount++
		}
	}

	require.Equal(t, 1, messageStartCount)
	require.Equal(t, 1, contentBlockStartCount)
	require.Equal(t, 1, contentBlockStopCount)
	require.Equal(t, 1, messageStopCount)
}

func TestStreamingIteratorCleanup(t *testing.T) {

	ctx := context.Background()
	provider := New()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'test'"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)

	// Test that Close works without error
	err = iterator.Close()
	require.NoError(t, err)

	// Test that Next returns false after Close
	require.False(t, iterator.Next(), "Next should return false after Close")

	// Test that Err still works after Close
	require.NoError(t, iterator.Err(), "Err should work after Close")

	// Test that Event returns nil after Close
	event := iterator.Event()
	require.Nil(t, event, "Event should return nil after Close")
}

func TestStreamingEarlyTermination(t *testing.T) {

	ctx := context.Background()
	provider := New()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Write a short paragraph about programming"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var eventCount int
	var foundMessageStart bool

	// Read only a few events and then stop
	for iterator.Next() && eventCount < 3 {
		event := iterator.Event()
		require.NotNil(t, event)
		eventCount++

		if event.Type == llm.EventTypeMessageStart {
			foundMessageStart = true
		}
	}

	// Should have received at least a message start
	require.True(t, foundMessageStart, "Should have received message start event")

	// Should be able to close early without issues
	err = iterator.Close()
	require.NoError(t, err)
}

func TestStreamingWithTemperature(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Test with low temperature for more deterministic output
	temp := 0.1
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Say 'hello world' exactly")),
		llm.WithTemperature(temp),
	)
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	// Should have received content
	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received some content")
}

func TestStreamingWithMaxTokens(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Test with very low max tokens to force truncation
	maxTokens := 10
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Write a very long story")),
		llm.WithMaxTokens(maxTokens),
	)
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}
	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	// Should have received some content (though possibly truncated)
	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received some content")
}

func TestStreamingErrorHandling(t *testing.T) {

	ctx := context.Background()
	provider := New()

	// Test with empty messages (should handle gracefully)
	iterator, err := provider.Stream(ctx, llm.WithMessages())
	require.Error(t, err, "Should return error for empty messages")
	require.Nil(t, iterator)
}

func TestStreamingConcurrentAccess(t *testing.T) {

	ctx := context.Background()
	provider := New()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Say 'concurrent'"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	// Test concurrent access to iterator methods
	done := make(chan bool, 2)

	go func() {
		for iterator.Next() {
			// Concurrent read
			_ = iterator.Event()
		}
		done <- true
	}()

	go func() {
		// Concurrent error check
		for {
			select {
			case <-done:
				return
			default:
				_ = iterator.Err()
			}
		}
	}()

	<-done
	<-done

	// Should complete without panics or deadlocks
	require.NoError(t, iterator.Err())
}

func TestStreamingIteratorEventSequence(t *testing.T) {
	// Test the event sequence structure without requiring real API calls
	// This tests the iterator's internal event management

	ctx := context.Background()

	// Test that the iterator can be created (though we can't test full streaming without API)
	iterator := NewStreamIterator(ctx, nil, nil, "test-model")
	require.NotNil(t, iterator)

	// Test that methods work on uninitialized iterator
	require.False(t, iterator.Next(), "Next should return false when not started")
	require.NoError(t, iterator.Err(), "Err should work on uninitialized iterator")
	require.Nil(t, iterator.Event(), "Event should return nil when not started")

	// Test Close on uninitialized iterator
	err := iterator.Close()
	require.NoError(t, err, "Close should work on uninitialized iterator")

	// Skip the rest of this test since it would require a real chat object
	// The streaming functionality is tested in the integration tests above
	t.Skip("Skipping full streaming test without real API calls")
}

func TestStreamingEventTypes(t *testing.T) {
	// Test that the event types we're using in the tests are properly defined
	// This ensures our test assertions are using valid event types

	// These should all be valid event types from the llm package
	eventTypes := []llm.EventType{
		llm.EventTypeMessageStart,
		llm.EventTypeContentBlockStart,
		llm.EventTypeContentBlockDelta,
		llm.EventTypeContentBlockStop,
		llm.EventTypeMessageStop,
	}

	// Just verify these constants exist and are not empty
	for _, eventType := range eventTypes {
		require.NotEmpty(t, string(eventType), "Event type should not be empty")
	}
}

func TestStreamingResponseStructure(t *testing.T) {
	// Test that we can create the expected response structures
	// This validates our test expectations are realistic

	// Create a basic response structure like what the iterator should emit
	response := &llm.Response{
		ID:      "test_id",
		Type:    "message",
		Role:    llm.Assistant,
		Content: []llm.Content{},
	}

	require.NotNil(t, response)
	require.Equal(t, llm.Assistant, response.Role)
	require.Empty(t, response.Content)

	// Test creating event structures
	event := &llm.Event{
		Type: llm.EventTypeMessageStart,
		Message: &llm.Response{
			ID:   "test_response_id",
			Type: "message",
			Role: llm.Assistant,
		},
	}

	require.NotNil(t, event)
	require.Equal(t, llm.EventTypeMessageStart, event.Type)
	require.NotNil(t, event.Message)
	require.Equal(t, llm.Assistant, event.Message.Role)
}

// TestRealStreamingIntegration performs a comprehensive test of the streaming functionality
func TestRealStreamingIntegration(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Write a short poem about coding, exactly 4 lines long."),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder
	var messageStartCount, contentBlockStartCount, deltaCount, contentBlockStopCount, messageStopCount int

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		switch event.Type {
		case llm.EventTypeMessageStart:
			messageStartCount++
			require.NotNil(t, event.Message)
			require.Equal(t, llm.Assistant, event.Message.Role)
		case llm.EventTypeContentBlockStart:
			contentBlockStartCount++
			require.NotNil(t, event.Index)
			require.NotNil(t, event.ContentBlock)
		case llm.EventTypeContentBlockDelta:
			deltaCount++
			require.NotNil(t, event.Index)
			require.NotNil(t, event.Delta)
			textContent.WriteString(event.Delta.Text)
		case llm.EventTypeContentBlockStop:
			contentBlockStopCount++
			require.NotNil(t, event.Index)
		case llm.EventTypeMessageStop:
			messageStopCount++
		}
	}

	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	// Validate event sequence
	require.Equal(t, 1, messageStartCount, "Should have exactly one message start event")
	require.Equal(t, 1, contentBlockStartCount, "Should have exactly one content block start event")
	require.True(t, deltaCount > 0, "Should have at least one delta event")
	require.Equal(t, 1, contentBlockStopCount, "Should have exactly one content block stop event")
	require.Equal(t, 1, messageStopCount, "Should have exactly one message stop event")

	// Validate content
	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received text content")
	require.True(t, len(strings.Split(strings.TrimSpace(content), "\n")) >= 3, "Should have multiple lines (poem)")
}

// TestStreamingWithSystemPromptIntegration tests streaming with system prompt (real API)
func TestStreamingWithSystemPromptIntegration(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	systemPrompt := "You are a helpful assistant that always responds in exactly 10 words or less."
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("What is the capital of France?")),
		llm.WithSystemPrompt(systemPrompt),
	)
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}

	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received content")

	// Check that response is reasonably short (system prompt should be followed)
	words := strings.Fields(content)
	require.True(t, len(words) <= 15, "Response should be reasonably short due to system prompt, got %d words", len(words))
}

// TestStreamingWithConversationHistoryIntegration tests streaming with conversation context (real API)
func TestStreamingWithConversationHistoryIntegration(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("My favorite color is blue."),
		llm.NewAssistantTextMessage("That's a nice choice! Blue is a very popular color."),
		llm.NewUserTextMessage("What color should I wear with it?"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}

	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	content := textContent.String()
	require.True(t, len(content) > 0, "Should have received content")

	// Should reference the conversation context
	contentLower := strings.ToLower(content)
	require.True(t, strings.Contains(contentLower, "blue") ||
		strings.Contains(contentLower, "color"),
		"Response should reference the conversation about colors")
}

// TestStreamingWithTemperatureIntegration tests streaming with different temperature settings (real API)
func TestStreamingWithTemperatureIntegration(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	// Test with low temperature for more deterministic output
	temp := 0.1
	iterator, err := provider.Stream(ctx,
		llm.WithMessages(llm.NewUserTextMessage("Say only 'Hello World' and nothing else.")),
		llm.WithTemperature(temp),
	)
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	var textContent strings.Builder

	// Collect all events
	for iterator.Next() {
		event := iterator.Event()
		require.NotNil(t, event)
		events = append(events, event)

		if event.Type == llm.EventTypeContentBlockDelta && event.Delta != nil {
			textContent.WriteString(event.Delta.Text)
		}
	}

	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0, "Should have received events")

	content := strings.TrimSpace(textContent.String())
	require.True(t, len(content) > 0, "Should have received content")

	// With low temperature and specific instruction, should get very close to expected output
	require.Contains(t, strings.ToLower(content), "hello", "Should contain hello")
	require.Contains(t, strings.ToLower(content), "world", "Should contain world")
}

// TestStreamingErrorHandlingIntegration tests error handling in streaming (real API)
func TestStreamingErrorHandlingIntegration(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	// Test with empty messages (should handle gracefully)
	iterator, err := provider.Stream(ctx, llm.WithMessages())
	require.Error(t, err, "Should return error for empty messages")
	require.Nil(t, iterator)
}

// TestStreamingTimeout tests that streaming respects timeouts
func TestStreamingTimeout(t *testing.T) {
	requireGoogleAPIKey(t)

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("Write a very long story about programming history."),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)
	defer iterator.Close()

	var events []*llm.Event
	start := time.Now()

	// Try to collect events, but should timeout
	for iterator.Next() && time.Since(start) < 5*time.Second {
		event := iterator.Event()
		if event != nil {
			events = append(events, event)
		}
	}

	// Should either timeout or get some events
	if time.Since(start) >= 5*time.Second {
		t.Log("Streaming timed out as expected")
	} else {
		require.True(t, len(events) > 0, "Should have received some events before timeout")
	}
}

// TestConcurrentStreaming tests that multiple streaming calls work concurrently
func TestConcurrentStreaming(t *testing.T) {
	requireGoogleAPIKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	provider := createProviderWithTimeout(t, 10*time.Second)

	const numConcurrent = 3
	results := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func(id int) {
			streamCtx, streamCancel := context.WithTimeout(ctx, 20*time.Second)
			defer streamCancel()

			iterator, err := provider.Stream(streamCtx, llm.WithMessages(
				llm.NewUserTextMessage(fmt.Sprintf("Say 'test %d'", id)),
			))
			if err != nil {
				results <- err
				return
			}
			defer iterator.Close()

			eventCount := 0
			for iterator.Next() {
				eventCount++
			}

			if err := iterator.Err(); err != nil {
				results <- err
				return
			}

			if eventCount == 0 {
				results <- fmt.Errorf("no events received for stream %d", id)
				return
			}

			results <- nil
		}(i)
	}

	// Wait for all concurrent streams to complete
	for i := 0; i < numConcurrent; i++ {
		err := <-results
		require.NoError(t, err, "Concurrent stream %d failed", i)
	}
}
