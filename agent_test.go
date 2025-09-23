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
