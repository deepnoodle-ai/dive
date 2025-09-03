package google

import (
	"context"
	"os"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/stretchr/testify/require"
)

func TestProviderName(t *testing.T) {
	provider := New()
	require.Equal(t, "google", provider.Name())
}

func TestProviderBasicGenerate(t *testing.T) {
	// Skip if no credentials are available
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" || os.Getenv("GOOGLE_CLOUD_LOCATION") == "" {
		t.Skip("Skipping Google provider test - no credentials")
	}

	ctx := context.Background()
	provider := New()

	response, err := provider.Generate(ctx, llm.WithMessages(
		llm.NewUserTextMessage("respond with \"hello\""),
	))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, llm.Assistant, response.Role)
	require.True(t, len(response.Content) > 0)
}

func TestProviderBasicStream(t *testing.T) {
	// Skip if no credentials are available
	if os.Getenv("GOOGLE_CLOUD_PROJECT") == "" || os.Getenv("GOOGLE_CLOUD_LOCATION") == "" {
		t.Skip("Skipping Google provider test - no credentials")
	}

	ctx := context.Background()
	provider := New()

	iterator, err := provider.Stream(ctx, llm.WithMessages(
		llm.NewUserTextMessage("count to 3"),
	))
	require.NoError(t, err)
	require.NotNil(t, iterator)

	defer iterator.Close()

	var events []*llm.Event
	for iterator.Next() {
		events = append(events, iterator.Event())
	}
	require.NoError(t, iterator.Err())
	require.True(t, len(events) > 0)
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
