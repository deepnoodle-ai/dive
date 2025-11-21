package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
)

// TestAgentCreateResponse demonstrates using the CreateResponse API
func TestAgentCreateResponse(t *testing.T) {
	// Setup a simple mock LLM
	mockLLM := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_123",
				Model:      "test-model", // This is the model name that will be used
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "This is a test response"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
		nameFunc: func() string {
			return "test-model" // Make sure this matches the model in the response
		},
	}

	// Create a simple agent with the mock LLM
	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
		Goal:  "To test the CreateResponse API",
		Model: mockLLM,
	})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	t.Run("CreateResponse with input string", func(t *testing.T) {
		// Test with a simple string input
		resp, err := agent.CreateResponse(context.Background(), WithInput("Hello, agent!"))
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		// Check if items exist and the message has the expected text
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		} else {
			found := false
			for _, item := range resp.Items {
				if item.Type == ResponseItemTypeMessage && item.Message != nil {
					text := item.Message.Text()
					if text == "This is a test response" {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Expected to find 'This is a test response' in response items")
			}
		}

		if resp.Usage == nil {
			t.Errorf("Expected non-nil Usage")
		} else {
			if resp.Usage.InputTokens != 10 {
				t.Errorf("Expected InputTokens=10, got %d", resp.Usage.InputTokens)
			}
			if resp.Usage.OutputTokens != 5 {
				t.Errorf("Expected OutputTokens=5, got %d", resp.Usage.OutputTokens)
			}
		}
	})

	t.Run("CreateResponse with messages", func(t *testing.T) {
		// Test with explicit messages
		messages := []*llm.Message{
			llm.NewUserTextMessage("Here's a more complex message"),
		}

		resp, err := agent.CreateResponse(context.Background(), WithMessages(messages...))
		if err != nil {
			t.Fatalf("CreateResponse with messages failed: %v", err)
		}

		// Check if items exist and the message has the expected text
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		} else {
			found := false
			for _, item := range resp.Items {
				if item.Type == ResponseItemTypeMessage && item.Message != nil {
					text := item.Message.Text()
					if text == "This is a test response" {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Expected to find 'This is a test response' in response items")
			}
		}
	})

	t.Run("CreateResponse with callback for final message", func(t *testing.T) {
		// Track callback invocations
		var callbackItems []*ResponseItem
		eventCallback := func(ctx context.Context, item *ResponseItem) error {
			callbackItems = append(callbackItems, item)
			return nil
		}

		// Create a response with callback
		resp, err := agent.CreateResponse(
			context.Background(),
			WithInput("Hello, agent!"),
			WithEventCallback(eventCallback),
		)
		if err != nil {
			t.Fatalf("CreateResponse with callback failed: %v", err)
		}

		// Verify that the callback was called for the final message
		if len(callbackItems) != 1 {
			t.Errorf("Expected callback to be called once for the final message, got %d calls", len(callbackItems))
		} else {
			item := callbackItems[0]
			if item.Type != ResponseItemTypeMessage {
				t.Errorf("Expected callback item type to be %s, got %s", ResponseItemTypeMessage, item.Type)
			}
			if item.Message == nil {
				t.Errorf("Expected callback item to have a message")
			} else if item.Message.Text() != "This is a test response" {
				t.Errorf("Expected callback message text to be 'This is a test response', got '%s'", item.Message.Text())
			}
			if item.Usage == nil {
				t.Errorf("Expected callback item to have usage information")
			} else {
				if item.Usage.InputTokens != 10 {
					t.Errorf("Expected callback usage InputTokens=10, got %d", item.Usage.InputTokens)
				}
				if item.Usage.OutputTokens != 5 {
					t.Errorf("Expected callback usage OutputTokens=5, got %d", item.Usage.OutputTokens)
				}
			}
		}

		// Also verify the response itself is correct
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		}
	})
}

// Mock types for testing

type mockLLM struct {
	generateFunc func(ctx context.Context, opts ...llm.Option) (*llm.Response, error)
	nameFunc     func() string
}

func (m *mockLLM) Name() string {
	return "mock-llm"
}

func (m *mockLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return m.generateFunc(ctx, opts...)
}
