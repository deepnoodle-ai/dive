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

		// Verify that the callback was called for init event + final message
		if len(callbackItems) != 2 {
			t.Errorf("Expected callback to be called twice (init + message), got %d calls", len(callbackItems))
		} else {
			// First item should be init event
			initItem := callbackItems[0]
			if initItem.Type != ResponseItemTypeInit {
				t.Errorf("Expected first callback item type to be %s, got %s", ResponseItemTypeInit, initItem.Type)
			}
			if initItem.Init == nil {
				t.Errorf("Expected init item to have Init field")
			} else if initItem.Init.ThreadID == "" {
				t.Errorf("Expected init event to have a thread ID")
			}

			// Second item should be the message
			msgItem := callbackItems[1]
			if msgItem.Type != ResponseItemTypeMessage {
				t.Errorf("Expected second callback item type to be %s, got %s", ResponseItemTypeMessage, msgItem.Type)
			}
			if msgItem.Message == nil {
				t.Errorf("Expected callback item to have a message")
			} else if msgItem.Message.Text() != "This is a test response" {
				t.Errorf("Expected callback message text to be 'This is a test response', got '%s'", msgItem.Message.Text())
			}
			if msgItem.Usage == nil {
				t.Errorf("Expected callback item to have usage information")
			} else {
				if msgItem.Usage.InputTokens != 10 {
					t.Errorf("Expected callback usage InputTokens=10, got %d", msgItem.Usage.InputTokens)
				}
				if msgItem.Usage.OutputTokens != 5 {
					t.Errorf("Expected callback usage OutputTokens=5, got %d", msgItem.Usage.OutputTokens)
				}
			}
		}

		// Also verify the response itself is correct
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		}
	})
}

// TestSessionManagement tests the session management features
func TestSessionManagement(t *testing.T) {
	// Setup a simple mock LLM
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
		nameFunc: func() string {
			return "test-model"
		},
	}

	t.Run("auto-generate ThreadID when not provided", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		resp, err := agent.CreateResponse(context.Background(), WithInput("Hello"))
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if resp.ThreadID == "" {
			t.Error("Expected ThreadID to be auto-generated")
		}
		if len(resp.ThreadID) < 10 {
			t.Errorf("Expected ThreadID to have reasonable length, got %q", resp.ThreadID)
		}
		if resp.ThreadID[:7] != "thread-" {
			t.Errorf("Expected ThreadID to start with 'thread-', got %q", resp.ThreadID)
		}
	})

	t.Run("use provided ThreadID", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		customThreadID := "my-custom-thread-123"
		resp, err := agent.CreateResponse(context.Background(),
			WithThreadID(customThreadID),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if resp.ThreadID != customThreadID {
			t.Errorf("Expected ThreadID %q, got %q", customThreadID, resp.ThreadID)
		}
	})

	t.Run("WithResume is alias for WithThreadID", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// First call creates a thread
		resp1, err := agent.CreateResponse(context.Background(),
			WithThreadID("resume-test-thread"),
			WithInput("First message"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Resume the thread using WithResume
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("resume-test-thread"),
			WithInput("Second message"),
		)
		if err != nil {
			t.Fatalf("Resume CreateResponse failed: %v", err)
		}

		if resp1.ThreadID != resp2.ThreadID {
			t.Errorf("Expected same ThreadID for resumed thread, got %q vs %q", resp1.ThreadID, resp2.ThreadID)
		}

		// Verify thread has accumulated messages
		thread, err := threadRepo.GetThread(context.Background(), "resume-test-thread")
		if err != nil {
			t.Fatalf("Failed to get thread: %v", err)
		}

		// Should have: first input + first response + second input + second response = 4 messages
		if len(thread.Messages) != 4 {
			t.Errorf("Expected 4 messages in thread, got %d", len(thread.Messages))
		}
	})

	t.Run("InitEvent emitted in callback", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		var initEvent *InitEvent
		var callbackThreadID string

		resp, err := agent.CreateResponse(context.Background(),
			WithInput("Hello"),
			WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
				if item.Type == ResponseItemTypeInit && item.Init != nil {
					initEvent = item.Init
					callbackThreadID = item.Init.ThreadID
				}
				return nil
			}),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if initEvent == nil {
			t.Error("Expected InitEvent to be emitted")
		} else if callbackThreadID != resp.ThreadID {
			t.Errorf("InitEvent ThreadID %q doesn't match response ThreadID %q",
				callbackThreadID, resp.ThreadID)
		}
	})
}

// TestThreadForking tests the thread forking functionality
func TestThreadForking(t *testing.T) {
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
		nameFunc: func() string { return "test-model" },
	}

	t.Run("fork creates new thread with copied messages", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial conversation
		resp1, err := agent.CreateResponse(context.Background(),
			WithThreadID("original-thread"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Fork the thread
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("original-thread"),
			WithFork(true),
			WithInput("Let's try something different"),
		)
		if err != nil {
			t.Fatalf("Fork CreateResponse failed: %v", err)
		}

		// Forked thread should have different ID
		if resp2.ThreadID == resp1.ThreadID {
			t.Error("Expected forked thread to have different ID")
		}

		// Original thread should be unchanged
		originalThread, err := threadRepo.GetThread(context.Background(), "original-thread")
		if err != nil {
			t.Fatalf("Failed to get original thread: %v", err)
		}
		// Original should have: input + response = 2 messages
		if len(originalThread.Messages) != 2 {
			t.Errorf("Expected original thread to have 2 messages, got %d", len(originalThread.Messages))
		}

		// Forked thread should have original messages plus new ones
		forkedThread, err := threadRepo.GetThread(context.Background(), resp2.ThreadID)
		if err != nil {
			t.Fatalf("Failed to get forked thread: %v", err)
		}
		// Forked should have: copied messages (2) + new input + new response = 4 messages
		if len(forkedThread.Messages) != 4 {
			t.Errorf("Expected forked thread to have 4 messages, got %d", len(forkedThread.Messages))
		}
	})

	t.Run("ForkThread repository method", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()

		// Create a thread with messages
		originalThread := &Thread{
			ID:        "original",
			UserID:    "user-1",
			AgentID:   "agent-1",
			AgentName: "Test Agent",
			Title:     "Test Thread",
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
				llm.NewAssistantTextMessage("Hi there!"),
			},
			Metadata: map[string]interface{}{"key": "value"},
		}
		if err := threadRepo.PutThread(context.Background(), originalThread); err != nil {
			t.Fatalf("Failed to put original thread: %v", err)
		}

		// Fork the thread
		forked, err := threadRepo.ForkThread(context.Background(), "original")
		if err != nil {
			t.Fatalf("ForkThread failed: %v", err)
		}

		// Verify forked thread
		if forked.ID == originalThread.ID {
			t.Error("Forked thread should have different ID")
		}
		if forked.ID[:7] != "thread-" {
			t.Errorf("Forked thread ID should start with 'thread-', got %q", forked.ID)
		}
		if forked.UserID != originalThread.UserID {
			t.Errorf("UserID mismatch: expected %q, got %q", originalThread.UserID, forked.UserID)
		}
		if forked.AgentID != originalThread.AgentID {
			t.Errorf("AgentID mismatch: expected %q, got %q", originalThread.AgentID, forked.AgentID)
		}
		if forked.Title != originalThread.Title {
			t.Errorf("Title mismatch: expected %q, got %q", originalThread.Title, forked.Title)
		}
		if len(forked.Messages) != len(originalThread.Messages) {
			t.Errorf("Expected %d messages, got %d", len(originalThread.Messages), len(forked.Messages))
		}
		if forked.Metadata["key"] != "value" {
			t.Error("Metadata not copied correctly")
		}

		// Verify original is unchanged
		original, err := threadRepo.GetThread(context.Background(), "original")
		if err != nil {
			t.Fatalf("Failed to get original: %v", err)
		}
		if len(original.Messages) != 2 {
			t.Errorf("Original messages changed unexpectedly")
		}

		// Verify forked is stored
		storedForked, err := threadRepo.GetThread(context.Background(), forked.ID)
		if err != nil {
			t.Fatalf("Forked thread not stored: %v", err)
		}
		if storedForked.ID != forked.ID {
			t.Error("Stored forked thread ID mismatch")
		}
	})

	t.Run("ForkThread returns error for non-existent thread", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()

		_, err := threadRepo.ForkThread(context.Background(), "non-existent")
		if err != ErrThreadNotFound {
			t.Errorf("Expected ErrThreadNotFound, got %v", err)
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

// TestSessionManagementEdgeCases tests edge cases and advanced scenarios
func TestSessionManagementEdgeCases(t *testing.T) {
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
		nameFunc: func() string { return "test-model" },
	}

	t.Run("unique ThreadIDs generated each time", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		resp1, err := agent.CreateResponse(context.Background(), WithInput("Hello"))
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		resp2, err := agent.CreateResponse(context.Background(), WithInput("Hello again"))
		if err != nil {
			t.Fatalf("Second CreateResponse failed: %v", err)
		}

		if resp1.ThreadID == resp2.ThreadID {
			t.Error("Expected unique ThreadIDs for different conversations")
		}
	})

	t.Run("empty ThreadID triggers auto-generation", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		resp, err := agent.CreateResponse(context.Background(),
			WithThreadID(""),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if resp.ThreadID == "" {
			t.Error("Expected ThreadID to be auto-generated when empty string provided")
		}
		if len(resp.ThreadID) < 7 || resp.ThreadID[:7] != "thread-" {
			t.Errorf("Expected auto-generated ThreadID format, got %q", resp.ThreadID)
		}
	})

	t.Run("WithFork false does not fork", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial thread
		resp1, err := agent.CreateResponse(context.Background(),
			WithThreadID("no-fork-thread"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// WithFork(false) should continue same thread
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("no-fork-thread"),
			WithFork(false),
			WithInput("Continue"),
		)
		if err != nil {
			t.Fatalf("Second CreateResponse failed: %v", err)
		}

		if resp1.ThreadID != resp2.ThreadID {
			t.Errorf("Expected same ThreadID with WithFork(false), got %q vs %q",
				resp1.ThreadID, resp2.ThreadID)
		}

		// Should only have one thread
		threads, err := threadRepo.ListThreads(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListThreads failed: %v", err)
		}
		if len(threads.Items) != 1 {
			t.Errorf("Expected 1 thread, got %d", len(threads.Items))
		}
	})

	t.Run("fork without ThreadRepository works without error", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			// No ThreadRepository
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// WithFork should not cause an error even without repository
		resp, err := agent.CreateResponse(context.Background(),
			WithThreadID("some-thread"),
			WithFork(true),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse with fork but no repository failed: %v", err)
		}

		// Should still have a thread ID
		if resp.ThreadID == "" {
			t.Error("Expected ThreadID even without repository")
		}
	})

	t.Run("multiple forks from same thread", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial thread
		resp1, err := agent.CreateResponse(context.Background(),
			WithThreadID("multi-fork-original"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Fork twice from the same original
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("multi-fork-original"),
			WithFork(true),
			WithInput("Fork 1"),
		)
		if err != nil {
			t.Fatalf("First fork failed: %v", err)
		}

		resp3, err := agent.CreateResponse(context.Background(),
			WithResume("multi-fork-original"),
			WithFork(true),
			WithInput("Fork 2"),
		)
		if err != nil {
			t.Fatalf("Second fork failed: %v", err)
		}

		// All three should have different IDs
		if resp1.ThreadID == resp2.ThreadID {
			t.Error("Fork 1 should have different ID from original")
		}
		if resp1.ThreadID == resp3.ThreadID {
			t.Error("Fork 2 should have different ID from original")
		}
		if resp2.ThreadID == resp3.ThreadID {
			t.Error("Fork 1 and Fork 2 should have different IDs")
		}

		// Original should still have 2 messages
		original, err := threadRepo.GetThread(context.Background(), "multi-fork-original")
		if err != nil {
			t.Fatalf("Failed to get original: %v", err)
		}
		if len(original.Messages) != 2 {
			t.Errorf("Original should have 2 messages, got %d", len(original.Messages))
		}

		// Both forks should have 4 messages each
		fork1, err := threadRepo.GetThread(context.Background(), resp2.ThreadID)
		if err != nil {
			t.Fatalf("Failed to get fork 1: %v", err)
		}
		if len(fork1.Messages) != 4 {
			t.Errorf("Fork 1 should have 4 messages, got %d", len(fork1.Messages))
		}

		fork2, err := threadRepo.GetThread(context.Background(), resp3.ThreadID)
		if err != nil {
			t.Fatalf("Failed to get fork 2: %v", err)
		}
		if len(fork2.Messages) != 4 {
			t.Errorf("Fork 2 should have 4 messages, got %d", len(fork2.Messages))
		}
	})

	t.Run("InitEvent has correct ThreadID after fork", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial thread
		_, err = agent.CreateResponse(context.Background(),
			WithThreadID("init-event-fork-test"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Fork and capture InitEvent
		var initThreadID string
		resp, err := agent.CreateResponse(context.Background(),
			WithResume("init-event-fork-test"),
			WithFork(true),
			WithInput("Forking"),
			WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
				if item.Type == ResponseItemTypeInit && item.Init != nil {
					initThreadID = item.Init.ThreadID
				}
				return nil
			}),
		)
		if err != nil {
			t.Fatalf("Fork CreateResponse failed: %v", err)
		}

		// InitEvent should have the FORKED thread ID, not the original
		if initThreadID == "init-event-fork-test" {
			t.Error("InitEvent should have forked ThreadID, not original")
		}
		if initThreadID != resp.ThreadID {
			t.Errorf("InitEvent ThreadID %q doesn't match response ThreadID %q",
				initThreadID, resp.ThreadID)
		}
	})

	t.Run("forking non-existent thread creates new thread", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Try to fork a non-existent thread
		resp, err := agent.CreateResponse(context.Background(),
			WithResume("non-existent-thread"),
			WithFork(true),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		// Should still work and create a thread
		if resp.ThreadID == "" {
			t.Error("Expected ThreadID to be set")
		}
	})

	t.Run("message accumulation across multiple calls", func(t *testing.T) {
		threadRepo := NewMemoryThreadRepository()
		agent, err := NewAgent(AgentOptions{
			Name:             "TestAgent",
			Model:            mockLLM,
			ThreadRepository: threadRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		threadID := "accumulation-test"

		// Make 5 calls to the same thread
		for i := 1; i <= 5; i++ {
			_, err := agent.CreateResponse(context.Background(),
				WithThreadID(threadID),
				WithInput("Message"),
			)
			if err != nil {
				t.Fatalf("CreateResponse %d failed: %v", i, err)
			}
		}

		// Thread should have 10 messages (5 inputs + 5 responses)
		thread, err := threadRepo.GetThread(context.Background(), threadID)
		if err != nil {
			t.Fatalf("Failed to get thread: %v", err)
		}
		if len(thread.Messages) != 10 {
			t.Errorf("Expected 10 messages after 5 calls, got %d", len(thread.Messages))
		}

		// Verify alternating roles
		for i, msg := range thread.Messages {
			expectedRole := llm.User
			if i%2 == 1 {
				expectedRole = llm.Assistant
			}
			if msg.Role != expectedRole {
				t.Errorf("Message %d has wrong role: expected %v, got %v", i, expectedRole, msg.Role)
			}
		}
	})
}

// TestThreadForkingDeepCopy tests that forked messages are truly independent
func TestThreadForkingDeepCopy(t *testing.T) {
	threadRepo := NewMemoryThreadRepository()

	// Create original thread with a message
	original := &Thread{
		ID: "deep-copy-test",
		Messages: []*llm.Message{
			llm.NewUserTextMessage("Original message"),
		},
	}
	if err := threadRepo.PutThread(context.Background(), original); err != nil {
		t.Fatalf("Failed to put original: %v", err)
	}

	// Fork the thread
	forked, err := threadRepo.ForkThread(context.Background(), "deep-copy-test")
	if err != nil {
		t.Fatalf("ForkThread failed: %v", err)
	}

	// Modify the forked message
	if len(forked.Messages) > 0 {
		forked.Messages[0] = llm.NewUserTextMessage("Modified in fork")
	}

	// Save the modified fork
	if err := threadRepo.PutThread(context.Background(), forked); err != nil {
		t.Fatalf("Failed to save modified fork: %v", err)
	}

	// Original should be unchanged
	originalReloaded, err := threadRepo.GetThread(context.Background(), "deep-copy-test")
	if err != nil {
		t.Fatalf("Failed to reload original: %v", err)
	}

	if len(originalReloaded.Messages) == 0 {
		t.Fatal("Original has no messages")
	}

	originalText := originalReloaded.Messages[0].Text()
	if originalText != "Original message" {
		t.Errorf("Original message was modified: got %q, expected %q",
			originalText, "Original message")
	}

	// Forked should have the new message
	forkedReloaded, err := threadRepo.GetThread(context.Background(), forked.ID)
	if err != nil {
		t.Fatalf("Failed to reload fork: %v", err)
	}

	forkedText := forkedReloaded.Messages[0].Text()
	if forkedText != "Modified in fork" {
		t.Errorf("Forked message was not updated: got %q, expected %q",
			forkedText, "Modified in fork")
	}
}

// TestThreadRepositoryOperations tests all ThreadRepository interface methods
func TestThreadRepositoryOperations(t *testing.T) {
	t.Run("PutThread and GetThread", func(t *testing.T) {
		repo := NewMemoryThreadRepository()

		thread := &Thread{
			ID:        "test-thread",
			UserID:    "user-1",
			AgentID:   "agent-1",
			AgentName: "Test Agent",
			Title:     "Test Title",
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
			},
			Metadata: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
		}

		err := repo.PutThread(context.Background(), thread)
		if err != nil {
			t.Fatalf("PutThread failed: %v", err)
		}

		retrieved, err := repo.GetThread(context.Background(), "test-thread")
		if err != nil {
			t.Fatalf("GetThread failed: %v", err)
		}

		if retrieved.ID != thread.ID {
			t.Errorf("ID mismatch: expected %q, got %q", thread.ID, retrieved.ID)
		}
		if retrieved.UserID != thread.UserID {
			t.Errorf("UserID mismatch: expected %q, got %q", thread.UserID, retrieved.UserID)
		}
		if retrieved.AgentID != thread.AgentID {
			t.Errorf("AgentID mismatch: expected %q, got %q", thread.AgentID, retrieved.AgentID)
		}
		if retrieved.AgentName != thread.AgentName {
			t.Errorf("AgentName mismatch: expected %q, got %q", thread.AgentName, retrieved.AgentName)
		}
		if retrieved.Title != thread.Title {
			t.Errorf("Title mismatch: expected %q, got %q", thread.Title, retrieved.Title)
		}
		if len(retrieved.Messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(retrieved.Messages))
		}
		if retrieved.Metadata["key1"] != "value1" {
			t.Errorf("Metadata key1 mismatch: expected %q, got %v", "value1", retrieved.Metadata["key1"])
		}
	})

	t.Run("GetThread returns error for non-existent", func(t *testing.T) {
		repo := NewMemoryThreadRepository()

		_, err := repo.GetThread(context.Background(), "non-existent")
		if err != ErrThreadNotFound {
			t.Errorf("Expected ErrThreadNotFound, got %v", err)
		}
	})

	t.Run("DeleteThread", func(t *testing.T) {
		repo := NewMemoryThreadRepository()

		thread := &Thread{ID: "delete-test"}
		_ = repo.PutThread(context.Background(), thread)

		err := repo.DeleteThread(context.Background(), "delete-test")
		if err != nil {
			t.Fatalf("DeleteThread failed: %v", err)
		}

		_, err = repo.GetThread(context.Background(), "delete-test")
		if err != ErrThreadNotFound {
			t.Errorf("Thread should be deleted, got err: %v", err)
		}
	})

	t.Run("DeleteThread non-existent is not an error", func(t *testing.T) {
		repo := NewMemoryThreadRepository()

		err := repo.DeleteThread(context.Background(), "non-existent")
		if err != nil {
			t.Errorf("DeleteThread should not error for non-existent: %v", err)
		}
	})

	t.Run("ListThreads with pagination", func(t *testing.T) {
		repo := NewMemoryThreadRepository()

		// Create 5 threads
		for i := 0; i < 5; i++ {
			thread := &Thread{ID: string(rune('a' + i))}
			_ = repo.PutThread(context.Background(), thread)
		}

		// List all
		all, err := repo.ListThreads(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListThreads failed: %v", err)
		}
		if len(all.Items) != 5 {
			t.Errorf("Expected 5 threads, got %d", len(all.Items))
		}

		// List with limit
		limited, err := repo.ListThreads(context.Background(), &ListThreadsInput{Limit: 2})
		if err != nil {
			t.Fatalf("ListThreads with limit failed: %v", err)
		}
		if len(limited.Items) != 2 {
			t.Errorf("Expected 2 threads with limit, got %d", len(limited.Items))
		}

		// List with offset
		offset, err := repo.ListThreads(context.Background(), &ListThreadsInput{Offset: 3})
		if err != nil {
			t.Fatalf("ListThreads with offset failed: %v", err)
		}
		if len(offset.Items) != 2 {
			t.Errorf("Expected 2 threads with offset 3, got %d", len(offset.Items))
		}

		// Offset beyond range
		empty, err := repo.ListThreads(context.Background(), &ListThreadsInput{Offset: 10})
		if err != nil {
			t.Fatalf("ListThreads with large offset failed: %v", err)
		}
		if len(empty.Items) != 0 {
			t.Errorf("Expected 0 threads with offset 10, got %d", len(empty.Items))
		}
	})

	t.Run("WithThreads initializer", func(t *testing.T) {
		threads := []*Thread{
			{ID: "init-1"},
			{ID: "init-2"},
			{ID: "init-3"},
		}

		repo := NewMemoryThreadRepository().WithThreads(threads)

		all, err := repo.ListThreads(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListThreads failed: %v", err)
		}
		if len(all.Items) != 3 {
			t.Errorf("Expected 3 threads, got %d", len(all.Items))
		}

		// Verify we can get each one
		for _, thread := range threads {
			retrieved, err := repo.GetThread(context.Background(), thread.ID)
			if err != nil {
				t.Errorf("Failed to get thread %s: %v", thread.ID, err)
			}
			if retrieved.ID != thread.ID {
				t.Errorf("ID mismatch for %s", thread.ID)
			}
		}
	})

	t.Run("ForkThread copies all fields", func(t *testing.T) {
		repo := NewMemoryThreadRepository()

		original := &Thread{
			ID:        "fork-all-fields",
			UserID:    "user-123",
			AgentID:   "agent-456",
			AgentName: "My Agent",
			Title:     "Important Conversation",
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
				llm.NewAssistantTextMessage("Hi there!"),
				llm.NewUserTextMessage("How are you?"),
			},
			Metadata: map[string]interface{}{
				"priority": "high",
				"tags":     []string{"test", "fork"},
			},
		}
		_ = repo.PutThread(context.Background(), original)

		forked, err := repo.ForkThread(context.Background(), "fork-all-fields")
		if err != nil {
			t.Fatalf("ForkThread failed: %v", err)
		}

		// Verify all fields are copied correctly
		if forked.ID == original.ID {
			t.Error("Forked ID should be different")
		}
		if forked.UserID != original.UserID {
			t.Errorf("UserID not copied: expected %q, got %q", original.UserID, forked.UserID)
		}
		if forked.AgentID != original.AgentID {
			t.Errorf("AgentID not copied: expected %q, got %q", original.AgentID, forked.AgentID)
		}
		if forked.AgentName != original.AgentName {
			t.Errorf("AgentName not copied: expected %q, got %q", original.AgentName, forked.AgentName)
		}
		if forked.Title != original.Title {
			t.Errorf("Title not copied: expected %q, got %q", original.Title, forked.Title)
		}
		if len(forked.Messages) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(forked.Messages))
		}
		if forked.Metadata["priority"] != "high" {
			t.Error("Metadata not copied correctly")
		}

		// Verify messages content
		if forked.Messages[0].Text() != "Hello" {
			t.Error("First message not copied correctly")
		}
		if forked.Messages[1].Text() != "Hi there!" {
			t.Error("Second message not copied correctly")
		}
		if forked.Messages[2].Text() != "How are you?" {
			t.Error("Third message not copied correctly")
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
