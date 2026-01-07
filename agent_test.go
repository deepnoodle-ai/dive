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
			} else if initItem.Init.SessionID == "" {
				t.Errorf("Expected init event to have a session ID")
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

	t.Run("auto-generate SessionID when not provided", func(t *testing.T) {
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

		if resp.SessionID == "" {
			t.Error("Expected SessionID to be auto-generated")
		}
		if len(resp.SessionID) < 10 {
			t.Errorf("Expected SessionID to have reasonable length, got %q", resp.SessionID)
		}
		if resp.SessionID[:8] != "session-" {
			t.Errorf("Expected SessionID to start with 'session-', got %q", resp.SessionID)
		}
	})

	t.Run("use provided SessionID", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		customSessionID := "my-custom-session-123"
		resp, err := agent.CreateResponse(context.Background(),
			WithSessionID(customSessionID),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if resp.SessionID != customSessionID {
			t.Errorf("Expected SessionID %q, got %q", customSessionID, resp.SessionID)
		}
	})

	t.Run("WithResume is alias for WithSessionID", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// First call creates a session
		resp1, err := agent.CreateResponse(context.Background(),
			WithSessionID("resume-test-session"),
			WithInput("First message"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Resume the session using WithResume
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("resume-test-session"),
			WithInput("Second message"),
		)
		if err != nil {
			t.Fatalf("Resume CreateResponse failed: %v", err)
		}

		if resp1.SessionID != resp2.SessionID {
			t.Errorf("Expected same SessionID for resumed session, got %q vs %q", resp1.SessionID, resp2.SessionID)
		}

		// Verify session has accumulated messages
		session, err := sessionRepo.GetSession(context.Background(), "resume-test-session")
		if err != nil {
			t.Fatalf("Failed to get session: %v", err)
		}

		// Should have: first input + first response + second input + second response = 4 messages
		if len(session.Messages) != 4 {
			t.Errorf("Expected 4 messages in session, got %d", len(session.Messages))
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
		var callbackSessionID string

		resp, err := agent.CreateResponse(context.Background(),
			WithInput("Hello"),
			WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
				if item.Type == ResponseItemTypeInit && item.Init != nil {
					initEvent = item.Init
					callbackSessionID = item.Init.SessionID
				}
				return nil
			}),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if initEvent == nil {
			t.Error("Expected InitEvent to be emitted")
		} else if callbackSessionID != resp.SessionID {
			t.Errorf("InitEvent SessionID %q doesn't match response SessionID %q",
				callbackSessionID, resp.SessionID)
		}
	})
}

// TestSessionForking tests the session forking functionality
func TestSessionForking(t *testing.T) {
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

	t.Run("fork creates new session with copied messages", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial conversation
		resp1, err := agent.CreateResponse(context.Background(),
			WithSessionID("original-session"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Fork the session
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("original-session"),
			WithFork(true),
			WithInput("Let's try something different"),
		)
		if err != nil {
			t.Fatalf("Fork CreateResponse failed: %v", err)
		}

		// Forked session should have different ID
		if resp2.SessionID == resp1.SessionID {
			t.Error("Expected forked session to have different ID")
		}

		// Original session should be unchanged
		originalSession, err := sessionRepo.GetSession(context.Background(), "original-session")
		if err != nil {
			t.Fatalf("Failed to get original session: %v", err)
		}
		// Original should have: input + response = 2 messages
		if len(originalSession.Messages) != 2 {
			t.Errorf("Expected original session to have 2 messages, got %d", len(originalSession.Messages))
		}

		// Forked session should have original messages plus new ones
		forkedSession, err := sessionRepo.GetSession(context.Background(), resp2.SessionID)
		if err != nil {
			t.Fatalf("Failed to get forked session: %v", err)
		}
		// Forked should have: copied messages (2) + new input + new response = 4 messages
		if len(forkedSession.Messages) != 4 {
			t.Errorf("Expected forked session to have 4 messages, got %d", len(forkedSession.Messages))
		}
	})

	t.Run("ForkSession repository method", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()

		// Create a session with messages
		originalSession := &Session{
			ID:        "original",
			UserID:    "user-1",
			AgentID:   "agent-1",
			AgentName: "Test Agent",
			Title:     "Test Session",
			Messages: []*llm.Message{
				llm.NewUserTextMessage("Hello"),
				llm.NewAssistantTextMessage("Hi there!"),
			},
			Metadata: map[string]interface{}{"key": "value"},
		}
		if err := sessionRepo.PutSession(context.Background(), originalSession); err != nil {
			t.Fatalf("Failed to put original session: %v", err)
		}

		// Fork the session
		forked, err := sessionRepo.ForkSession(context.Background(), "original")
		if err != nil {
			t.Fatalf("ForkSession failed: %v", err)
		}

		// Verify forked session
		if forked.ID == originalSession.ID {
			t.Error("Forked session should have different ID")
		}
		if forked.ID[:8] != "session-" {
			t.Errorf("Forked session ID should start with 'session-', got %q", forked.ID)
		}
		if forked.UserID != originalSession.UserID {
			t.Errorf("UserID mismatch: expected %q, got %q", originalSession.UserID, forked.UserID)
		}
		if forked.AgentID != originalSession.AgentID {
			t.Errorf("AgentID mismatch: expected %q, got %q", originalSession.AgentID, forked.AgentID)
		}
		if forked.Title != originalSession.Title {
			t.Errorf("Title mismatch: expected %q, got %q", originalSession.Title, forked.Title)
		}
		if len(forked.Messages) != len(originalSession.Messages) {
			t.Errorf("Expected %d messages, got %d", len(originalSession.Messages), len(forked.Messages))
		}
		if forked.Metadata["key"] != "value" {
			t.Error("Metadata not copied correctly")
		}

		// Verify original is unchanged
		original, err := sessionRepo.GetSession(context.Background(), "original")
		if err != nil {
			t.Fatalf("Failed to get original: %v", err)
		}
		if len(original.Messages) != 2 {
			t.Errorf("Original messages changed unexpectedly")
		}

		// Verify forked is stored
		storedForked, err := sessionRepo.GetSession(context.Background(), forked.ID)
		if err != nil {
			t.Fatalf("Forked session not stored: %v", err)
		}
		if storedForked.ID != forked.ID {
			t.Error("Stored forked session ID mismatch")
		}
	})

	t.Run("ForkSession returns error for non-existent session", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()

		_, err := sessionRepo.ForkSession(context.Background(), "non-existent")
		if err != ErrSessionNotFound {
			t.Errorf("Expected ErrSessionNotFound, got %v", err)
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

	t.Run("unique SessionIDs generated each time", func(t *testing.T) {
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

		if resp1.SessionID == resp2.SessionID {
			t.Error("Expected unique SessionIDs for different conversations")
		}
	})

	t.Run("empty SessionID triggers auto-generation", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		resp, err := agent.CreateResponse(context.Background(),
			WithSessionID(""),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		if resp.SessionID == "" {
			t.Error("Expected SessionID to be auto-generated when empty string provided")
		}
		if len(resp.SessionID) < 8 || resp.SessionID[:8] != "session-" {
			t.Errorf("Expected auto-generated SessionID format, got %q", resp.SessionID)
		}
	})

	t.Run("WithFork false does not fork", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial session
		resp1, err := agent.CreateResponse(context.Background(),
			WithSessionID("no-fork-session"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// WithFork(false) should continue same session
		resp2, err := agent.CreateResponse(context.Background(),
			WithResume("no-fork-session"),
			WithFork(false),
			WithInput("Continue"),
		)
		if err != nil {
			t.Fatalf("Second CreateResponse failed: %v", err)
		}

		if resp1.SessionID != resp2.SessionID {
			t.Errorf("Expected same SessionID with WithFork(false), got %q vs %q",
				resp1.SessionID, resp2.SessionID)
		}

		// Should only have one session
		sessions, err := sessionRepo.ListSessions(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(sessions.Items) != 1 {
			t.Errorf("Expected 1 session, got %d", len(sessions.Items))
		}
	})

	t.Run("fork without SessionRepository works without error", func(t *testing.T) {
		agent, err := NewAgent(AgentOptions{
			Name:  "TestAgent",
			Model: mockLLM,
			// No SessionRepository
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// WithFork should not cause an error even without repository
		resp, err := agent.CreateResponse(context.Background(),
			WithSessionID("some-session"),
			WithFork(true),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse with fork but no repository failed: %v", err)
		}

		// Should still have a session ID
		if resp.SessionID == "" {
			t.Error("Expected SessionID even without repository")
		}
	})

	t.Run("multiple forks from same session", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial session
		resp1, err := agent.CreateResponse(context.Background(),
			WithSessionID("multi-fork-original"),
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
		if resp1.SessionID == resp2.SessionID {
			t.Error("Fork 1 should have different ID from original")
		}
		if resp1.SessionID == resp3.SessionID {
			t.Error("Fork 2 should have different ID from original")
		}
		if resp2.SessionID == resp3.SessionID {
			t.Error("Fork 1 and Fork 2 should have different IDs")
		}

		// Original should still have 2 messages
		original, err := sessionRepo.GetSession(context.Background(), "multi-fork-original")
		if err != nil {
			t.Fatalf("Failed to get original: %v", err)
		}
		if len(original.Messages) != 2 {
			t.Errorf("Original should have 2 messages, got %d", len(original.Messages))
		}

		// Both forks should have 4 messages each
		fork1, err := sessionRepo.GetSession(context.Background(), resp2.SessionID)
		if err != nil {
			t.Fatalf("Failed to get fork 1: %v", err)
		}
		if len(fork1.Messages) != 4 {
			t.Errorf("Fork 1 should have 4 messages, got %d", len(fork1.Messages))
		}

		fork2, err := sessionRepo.GetSession(context.Background(), resp3.SessionID)
		if err != nil {
			t.Fatalf("Failed to get fork 2: %v", err)
		}
		if len(fork2.Messages) != 4 {
			t.Errorf("Fork 2 should have 4 messages, got %d", len(fork2.Messages))
		}
	})

	t.Run("InitEvent has correct SessionID after fork", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Create initial session
		_, err = agent.CreateResponse(context.Background(),
			WithSessionID("init-event-fork-test"),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("First CreateResponse failed: %v", err)
		}

		// Fork and capture InitEvent
		var initSessionID string
		resp, err := agent.CreateResponse(context.Background(),
			WithResume("init-event-fork-test"),
			WithFork(true),
			WithInput("Forking"),
			WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
				if item.Type == ResponseItemTypeInit && item.Init != nil {
					initSessionID = item.Init.SessionID
				}
				return nil
			}),
		)
		if err != nil {
			t.Fatalf("Fork CreateResponse failed: %v", err)
		}

		// InitEvent should have the FORKED session ID, not the original
		if initSessionID == "init-event-fork-test" {
			t.Error("InitEvent should have forked SessionID, not original")
		}
		if initSessionID != resp.SessionID {
			t.Errorf("InitEvent SessionID %q doesn't match response SessionID %q",
				initSessionID, resp.SessionID)
		}
	})

	t.Run("forking non-existent session creates new session", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		// Try to fork a non-existent session
		resp, err := agent.CreateResponse(context.Background(),
			WithResume("non-existent-session"),
			WithFork(true),
			WithInput("Hello"),
		)
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		// Should still work and create a session
		if resp.SessionID == "" {
			t.Error("Expected SessionID to be set")
		}
	})

	t.Run("message accumulation across multiple calls", func(t *testing.T) {
		sessionRepo := NewMemorySessionRepository()
		agent, err := NewAgent(AgentOptions{
			Name:              "TestAgent",
			Model:             mockLLM,
			SessionRepository: sessionRepo,
		})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		sessionID := "accumulation-test"

		// Make 5 calls to the same session
		for i := 1; i <= 5; i++ {
			_, err := agent.CreateResponse(context.Background(),
				WithSessionID(sessionID),
				WithInput("Message"),
			)
			if err != nil {
				t.Fatalf("CreateResponse %d failed: %v", i, err)
			}
		}

		// Session should have 10 messages (5 inputs + 5 responses)
		session, err := sessionRepo.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("Failed to get session: %v", err)
		}
		if len(session.Messages) != 10 {
			t.Errorf("Expected 10 messages after 5 calls, got %d", len(session.Messages))
		}

		// Verify alternating roles
		for i, msg := range session.Messages {
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

// TestSessionForkingDeepCopy tests that forked messages are truly independent
func TestSessionForkingDeepCopy(t *testing.T) {
	sessionRepo := NewMemorySessionRepository()

	// Create original session with a message
	original := &Session{
		ID: "deep-copy-test",
		Messages: []*llm.Message{
			llm.NewUserTextMessage("Original message"),
		},
	}
	if err := sessionRepo.PutSession(context.Background(), original); err != nil {
		t.Fatalf("Failed to put original: %v", err)
	}

	// Fork the session
	forked, err := sessionRepo.ForkSession(context.Background(), "deep-copy-test")
	if err != nil {
		t.Fatalf("ForkSession failed: %v", err)
	}

	// Modify the forked message
	if len(forked.Messages) > 0 {
		forked.Messages[0] = llm.NewUserTextMessage("Modified in fork")
	}

	// Save the modified fork
	if err := sessionRepo.PutSession(context.Background(), forked); err != nil {
		t.Fatalf("Failed to save modified fork: %v", err)
	}

	// Original should be unchanged
	originalReloaded, err := sessionRepo.GetSession(context.Background(), "deep-copy-test")
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
	forkedReloaded, err := sessionRepo.GetSession(context.Background(), forked.ID)
	if err != nil {
		t.Fatalf("Failed to reload fork: %v", err)
	}

	forkedText := forkedReloaded.Messages[0].Text()
	if forkedText != "Modified in fork" {
		t.Errorf("Forked message was not updated: got %q, expected %q",
			forkedText, "Modified in fork")
	}
}

// TestSessionRepositoryOperations tests all SessionRepository interface methods
func TestSessionRepositoryOperations(t *testing.T) {
	t.Run("PutSession and GetSession", func(t *testing.T) {
		repo := NewMemorySessionRepository()

		session := &Session{
			ID:        "test-session",
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

		err := repo.PutSession(context.Background(), session)
		if err != nil {
			t.Fatalf("PutSession failed: %v", err)
		}

		retrieved, err := repo.GetSession(context.Background(), "test-session")
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}

		if retrieved.ID != session.ID {
			t.Errorf("ID mismatch: expected %q, got %q", session.ID, retrieved.ID)
		}
		if retrieved.UserID != session.UserID {
			t.Errorf("UserID mismatch: expected %q, got %q", session.UserID, retrieved.UserID)
		}
		if retrieved.AgentID != session.AgentID {
			t.Errorf("AgentID mismatch: expected %q, got %q", session.AgentID, retrieved.AgentID)
		}
		if retrieved.AgentName != session.AgentName {
			t.Errorf("AgentName mismatch: expected %q, got %q", session.AgentName, retrieved.AgentName)
		}
		if retrieved.Title != session.Title {
			t.Errorf("Title mismatch: expected %q, got %q", session.Title, retrieved.Title)
		}
		if len(retrieved.Messages) != 1 {
			t.Errorf("Expected 1 message, got %d", len(retrieved.Messages))
		}
		if retrieved.Metadata["key1"] != "value1" {
			t.Errorf("Metadata key1 mismatch: expected %q, got %v", "value1", retrieved.Metadata["key1"])
		}
	})

	t.Run("GetSession returns error for non-existent", func(t *testing.T) {
		repo := NewMemorySessionRepository()

		_, err := repo.GetSession(context.Background(), "non-existent")
		if err != ErrSessionNotFound {
			t.Errorf("Expected ErrSessionNotFound, got %v", err)
		}
	})

	t.Run("DeleteSession", func(t *testing.T) {
		repo := NewMemorySessionRepository()

		session := &Session{ID: "delete-test"}
		_ = repo.PutSession(context.Background(), session)

		err := repo.DeleteSession(context.Background(), "delete-test")
		if err != nil {
			t.Fatalf("DeleteSession failed: %v", err)
		}

		_, err = repo.GetSession(context.Background(), "delete-test")
		if err != ErrSessionNotFound {
			t.Errorf("Session should be deleted, got err: %v", err)
		}
	})

	t.Run("DeleteSession non-existent is not an error", func(t *testing.T) {
		repo := NewMemorySessionRepository()

		err := repo.DeleteSession(context.Background(), "non-existent")
		if err != nil {
			t.Errorf("DeleteSession should not error for non-existent: %v", err)
		}
	})

	t.Run("ListSessions with pagination", func(t *testing.T) {
		repo := NewMemorySessionRepository()

		// Create 5 sessions
		for i := 0; i < 5; i++ {
			session := &Session{ID: string(rune('a' + i))}
			_ = repo.PutSession(context.Background(), session)
		}

		// List all
		all, err := repo.ListSessions(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(all.Items) != 5 {
			t.Errorf("Expected 5 sessions, got %d", len(all.Items))
		}

		// List with limit
		limited, err := repo.ListSessions(context.Background(), &ListSessionsInput{Limit: 2})
		if err != nil {
			t.Fatalf("ListSessions with limit failed: %v", err)
		}
		if len(limited.Items) != 2 {
			t.Errorf("Expected 2 sessions with limit, got %d", len(limited.Items))
		}

		// List with offset
		offset, err := repo.ListSessions(context.Background(), &ListSessionsInput{Offset: 3})
		if err != nil {
			t.Fatalf("ListSessions with offset failed: %v", err)
		}
		if len(offset.Items) != 2 {
			t.Errorf("Expected 2 sessions with offset 3, got %d", len(offset.Items))
		}

		// Offset beyond range
		empty, err := repo.ListSessions(context.Background(), &ListSessionsInput{Offset: 10})
		if err != nil {
			t.Fatalf("ListSessions with large offset failed: %v", err)
		}
		if len(empty.Items) != 0 {
			t.Errorf("Expected 0 sessions with offset 10, got %d", len(empty.Items))
		}
	})

	t.Run("WithSessions initializer", func(t *testing.T) {
		sessions := []*Session{
			{ID: "init-1"},
			{ID: "init-2"},
			{ID: "init-3"},
		}

		repo := NewMemorySessionRepository().WithSessions(sessions)

		all, err := repo.ListSessions(context.Background(), nil)
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(all.Items) != 3 {
			t.Errorf("Expected 3 sessions, got %d", len(all.Items))
		}

		// Verify we can get each one
		for _, session := range sessions {
			retrieved, err := repo.GetSession(context.Background(), session.ID)
			if err != nil {
				t.Errorf("Failed to get session %s: %v", session.ID, err)
			}
			if retrieved.ID != session.ID {
				t.Errorf("ID mismatch for %s", session.ID)
			}
		}
	})

	t.Run("ForkSession copies all fields", func(t *testing.T) {
		repo := NewMemorySessionRepository()

		original := &Session{
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
		_ = repo.PutSession(context.Background(), original)

		forked, err := repo.ForkSession(context.Background(), "fork-all-fields")
		if err != nil {
			t.Fatalf("ForkSession failed: %v", err)
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
