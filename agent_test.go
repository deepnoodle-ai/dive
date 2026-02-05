package dive

import (
	"context"
	"errors"
	"fmt"
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
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "This is a test response"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
		nameFunc: func() string {
			return "test-model"
		},
	}

	// Create a simple agent with the mock LLM
	agent, err := NewAgent(AgentOptions{
		Name:  "TestAgent",
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

		// Verify that the callback was called with the message
		if len(callbackItems) == 0 {
			t.Errorf("Expected callback to be called at least once, got 0 calls")
		}

		// Find the message item in callback items
		foundMessage := false
		for _, item := range callbackItems {
			if item.Type == ResponseItemTypeMessage {
				foundMessage = true
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
		}
		if !foundMessage {
			t.Errorf("Expected to find a message callback item")
		}

		// Also verify the response itself is correct
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		}
	})
}

// TestMessageCopy tests the Message.Copy method
func TestMessageCopy(t *testing.T) {
	original := llm.NewUserTextMessage("Hello, world!")

	copied := original.Copy()

	if copied == original {
		t.Error("Copy should return a new pointer")
	}
	if copied.Role != original.Role {
		t.Errorf("Role mismatch: expected %v, got %v", original.Role, copied.Role)
	}
	if copied.Text() != original.Text() {
		t.Errorf("Text mismatch: expected %q, got %q", original.Text(), copied.Text())
	}
}

// Mock types for testing

type mockLLM struct {
	generateFunc func(ctx context.Context, opts ...llm.Option) (*llm.Response, error)
	nameFunc     func() string
}

func (m *mockLLM) Name() string {
	if m.nameFunc != nil {
		return m.nameFunc()
	}
	return "mock-llm"
}

func (m *mockLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return m.generateFunc(ctx, opts...)
}

// TestHookAbortError tests the HookAbortError functionality across all hook types
func TestHookAbortError(t *testing.T) {
	mockLLM := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_123",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Test response"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
	}

	t.Run("PostGeneration with regular error logs and continues", func(t *testing.T) {
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, state *GenerationState) error {
					return fmt.Errorf("regular error")
				},
			},
		})

		resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
		if err != nil {
			t.Errorf("Expected success despite regular error, got: %v", err)
		}
		if resp == nil {
			t.Error("Expected non-nil response")
		}
	})

	t.Run("PostGeneration with HookAbortError aborts", func(t *testing.T) {
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, state *GenerationState) error {
					return AbortGeneration("safety violation detected")
				},
			},
		})

		resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
		if err == nil {
			t.Error("Expected error from HookAbortError")
		}
		if resp != nil {
			t.Error("Expected nil response when aborted")
		}

		var abortErr *HookAbortError
		if !errors.As(err, &abortErr) {
			t.Errorf("Expected HookAbortError, got: %T", err)
		} else {
			if abortErr.Reason != "safety violation detected" {
				t.Errorf("Expected reason 'safety violation detected', got: %s", abortErr.Reason)
			}
			if abortErr.HookType != "PostGeneration" {
				t.Errorf("Expected HookType 'PostGeneration', got: %s", abortErr.HookType)
			}
		}
	})

	t.Run("PreGeneration with any error aborts", func(t *testing.T) {
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			PreGeneration: []PreGenerationHook{
				func(ctx context.Context, state *GenerationState) error {
					return fmt.Errorf("setup failed")
				},
			},
		})

		resp, err := agent.CreateResponse(context.Background(), WithInput("test"))
		if err == nil {
			t.Error("Expected error from PreGeneration hook")
		}
		if resp != nil {
			t.Error("Expected nil response when PreGeneration fails")
		}
		// PreGeneration wraps errors with "pre-generation hook error: "
		expectedMsg := "pre-generation hook error: setup failed"
		if err.Error() != expectedMsg {
			t.Errorf("Expected error %q, got: %v", expectedMsg, err)
		}
	})

	t.Run("HookAbortError with cause", func(t *testing.T) {
		causeErr := fmt.Errorf("underlying cause")
		agent, _ := NewAgent(AgentOptions{
			Model: mockLLM,
			PostGeneration: []PostGenerationHook{
				func(ctx context.Context, state *GenerationState) error {
					return AbortGenerationWithCause("wrapped error", causeErr)
				},
			},
		})

		_, err := agent.CreateResponse(context.Background(), WithInput("test"))
		if err == nil {
			t.Fatal("Expected error")
		}

		var abortErr *HookAbortError
		if !errors.As(err, &abortErr) {
			t.Fatalf("Expected HookAbortError, got: %T", err)
		}

		if abortErr.Cause != causeErr {
			t.Errorf("Expected cause to be preserved, got: %v", abortErr.Cause)
		}

		if !errors.Is(err, causeErr) {
			t.Error("Expected errors.Is to work with wrapped cause")
		}
	})
}
