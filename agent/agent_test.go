package agent

import (
	"context"
	"testing"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/stretchr/testify/require"
)

func TestAgent(t *testing.T) {
	agent, err := New(Options{
		Name:         "Testing Agent",
		Goal:         "Test the agent",
		Instructions: "You are a testing agent",
		IsSupervisor: false,
		Model:        anthropic.New(),
	})
	require.NoError(t, err)
	require.NotNil(t, agent)
}

func TestAgentChat(t *testing.T) {
	// Use a mock LLM instead of Anthropic to prevent timing out
	mockLLM := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_mock",
				Model:      "test-model",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Hello there! How can I help you today?"}},
				Type:       "message",
				StopReason: "stop",
				Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
			}, nil
		},
	}

	agent, err := New(Options{
		Name:  "Testing Agent",
		Model: mockLLM,
	})
	require.NoError(t, err)

	stream, err := agent.StreamResponse(context.Background(), dive.WithInput("Hello, world!"))
	require.NoError(t, err)

	var foundResponse bool
	for stream.Next(context.Background()) {
		event := stream.Event()
		if event.Type == dive.EventTypeResponseCompleted && event.Response != nil {
			foundResponse = true
			break
		}
	}
	require.True(t, foundResponse, "Expected to find a completed response event")
}

// func TestAgentChatWithTools(t *testing.T) {
// 	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
// 	defer cancel()

// 	echoToolDef := llm.ToolDefinition{
// 		Name:        "echo",
// 		Description: "Echoes back the input",
// 		Parameters: llm.Schema{
// 			Type:     "object",
// 			Required: []string{"text"},
// 			Properties: map[string]*llm.SchemaProperty{
// 				"text": {Type: "string", Description: "The text to echo back"},
// 			},
// 		},
// 	}

// 	var echoInput string

// 	echoFunc := func(ctx context.Context, input string) (string, error) {
// 		var m map[string]interface{}
// 		err := json.Unmarshal([]byte(input), &m)
// 		require.NoError(t, err)
// 		echoInput = m["text"].(string)
// 		return input, nil
// 	}

// 	agent, err := New(Options{
// 		Model: anthropic.New(),
// 		Tools: []llm.Tool{llm.NewTool(&echoToolDef, echoFunc)},
// 	})
// 	require.NoError(t, err)

// 	err = agent.Start(ctx)
// 	require.NoError(t, err)
// 	defer agent.Stop(ctx)

// 	stream, err := agent.Chat(ctx, llm.Messages{llm.NewUserMessage("Please use the echo tool to echo 'hello world'")})
// 	require.NoError(t, err)

// 	messages, err := dive.ReadMessages(ctx, stream)
// 	require.NoError(t, err)
// 	require.Greater(t, len(messages), 0)

// 	lastMessage := messages[len(messages)-1]
// 	text := strings.ToLower(lastMessage.Text())
// 	require.Contains(t, text, "echo")
// 	require.Equal(t, "hello world", echoInput)

// 	err = agent.Stop(ctx)
// 	require.NoError(t, err)
// }

func TestAgentChatSystemPrompt(t *testing.T) {
	agent, err := New(Options{
		Name:         "TestAgent",
		Goal:         "Help research a topic.",
		Instructions: "You are a research assistant.",
		IsSupervisor: false,
		Model:        anthropic.New(),
	})
	require.NoError(t, err)

	// Get the chat system prompt
	chatSystemPrompt, err := agent.buildSystemPrompt()
	require.NoError(t, err)

	// Verify that the chat system prompt doesn't contain the status section
	require.NotContains(t, chatSystemPrompt, "<status>")
	require.NotContains(t, chatSystemPrompt, "active")
	require.NotContains(t, chatSystemPrompt, "completed")
	require.NotContains(t, chatSystemPrompt, "paused")
	require.NotContains(t, chatSystemPrompt, "blocked")
	require.NotContains(t, chatSystemPrompt, "error")
}

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
	agent, err := New(Options{
		Name:  "TestAgent",
		Goal:  "To test the CreateResponse API",
		Model: mockLLM,
	})
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	t.Run("CreateResponse with input string", func(t *testing.T) {
		// Test with a simple string input
		resp, err := agent.CreateResponse(context.Background(), dive.WithInput("Hello, agent!"))
		if err != nil {
			t.Fatalf("CreateResponse failed: %v", err)
		}

		// Check if items exist and the message has the expected text
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		} else {
			found := false
			for _, item := range resp.Items {
				if item.Type == dive.ResponseItemTypeMessage && item.Message != nil {
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

		resp, err := agent.CreateResponse(context.Background(), dive.WithMessages(messages))
		if err != nil {
			t.Fatalf("CreateResponse with messages failed: %v", err)
		}

		// Check if items exist and the message has the expected text
		if len(resp.Items) == 0 {
			t.Errorf("Expected response to have items, got none")
		} else {
			found := false
			for _, item := range resp.Items {
				if item.Type == dive.ResponseItemTypeMessage && item.Message != nil {
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
