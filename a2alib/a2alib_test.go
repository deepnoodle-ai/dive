package a2alib_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/a2alib"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type fakeLLM struct {
	generate func(ctx context.Context, opts ...llm.Option) (*llm.Response, error)
}

func (f *fakeLLM) Name() string { return "fake-llm" }
func (f *fakeLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	return f.generate(ctx, opts...)
}

func textResponse(text string) *llm.Response {
	return &llm.Response{
		ID:         "resp_1",
		Model:      "fake-model",
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: text}},
		Type:       "message",
		StopReason: "stop",
		Usage:      llm.Usage{InputTokens: 5, OutputTokens: 5},
	}
}

func toolCallResponse(toolName, callID string) *llm.Response {
	return &llm.Response{
		ID:    "resp_tool",
		Model: "fake-model",
		Role:  llm.Assistant,
		Content: []llm.Content{&llm.ToolUseContent{
			ID:    callID,
			Name:  toolName,
			Input: json.RawMessage(`{}`),
		}},
		Type:       "message",
		StopReason: "tool_use",
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

type suspendingTool struct{}

func (t *suspendingTool) Name() string                        { return "ask" }
func (t *suspendingTool) Description() string                 { return "Ask the human for input" }
func (t *suspendingTool) Schema() *dive.Schema                { return nil }
func (t *suspendingTool) Annotations() *dive.ToolAnnotations  { return &dive.ToolAnnotations{Title: "Ask"} }
func (t *suspendingTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return dive.NewSuspendResult("Need your approval", map[string]any{"kind": "approval"}), nil
}

func buildAgent(t *testing.T, model llm.LLM, tools ...dive.Tool) *dive.Agent {
	t.Helper()
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:  "Test Agent",
		Model: model,
		Tools: tools,
	})
	assert.NoError(t, err)
	return agent
}

// startServer creates a test HTTP server running the a2alib adapter.
func startServer(t *testing.T, agent *dive.Agent) (*httptest.Server, *a2aclient.Client) {
	t.Helper()
	srv, err := a2alib.NewServer(a2alib.ServerOptions{
		Agent:     agent,
		Transport: "jsonrpc",
	})
	assert.NoError(t, err)

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Create an a2a-go client pointed at the test server.
	card := srv.Card()
	card.SupportedInterfaces = []*a2a.AgentInterface{{
		URL:             ts.URL,
		ProtocolBinding: a2a.TransportProtocolJSONRPC,
		ProtocolVersion: a2a.Version,
	}}

	client, err := a2aclient.NewFromCard(context.Background(), card)
	assert.NoError(t, err)

	return ts, client
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAgentCardServed(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("hello"), nil
	}}
	agent := buildAgent(t, model)

	srv, err := a2alib.NewServer(a2alib.ServerOptions{
		Agent:   agent,
		BaseURL: "http://localhost:8080",
	})
	assert.NoError(t, err)

	card := srv.Card()
	assert.Equal(t, "Test Agent", card.Name)
	assert.True(t, card.Capabilities.Streaming)
	assert.True(t, len(card.Skills) > 0)
	assert.True(t, len(card.SupportedInterfaces) > 0)
}

func TestSendMessageCompletion(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("Paris is the capital of France."), nil
	}}
	agent := buildAgent(t, model)

	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("What is the capital of France?"))
	result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: msg,
	})
	assert.NoError(t, err)

	task, ok := result.(*a2a.Task)
	assert.True(t, ok)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)
	assert.True(t, len(task.Artifacts) > 0)

	// Check the artifact contains the response text.
	art := task.Artifacts[0]
	assert.True(t, len(art.Parts) > 0)
	assert.Equal(t, "Paris is the capital of France.", art.Parts[0].Text())
}

func TestSendMessageSuspend(t *testing.T) {
	callCount := 0
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		callCount++
		if callCount == 1 {
			return toolCallResponse("ask", "call_1"), nil
		}
		return textResponse("Approved! Proceeding."), nil
	}}
	agent := buildAgent(t, model, &suspendingTool{})

	_, client := startServer(t, agent)

	// First message triggers suspend.
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("Do something risky"))
	result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: msg,
	})
	assert.NoError(t, err)

	task, ok := result.(*a2a.Task)
	assert.True(t, ok)
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)

	// The suspension prompt should be in the status message.
	assert.NotNil(t, task.Status.Message)
	assert.True(t, len(task.Status.Message.Parts) > 0)
	assert.Equal(t, "Need your approval", task.Status.Message.Parts[0].Text())

	// Verify the suspension metadata was stored.
	assert.NotNil(t, task.Metadata)
	_, hasSuspension := task.Metadata["dive.suspension"]
	assert.True(t, hasSuspension)
}

func TestSuspendAndResume(t *testing.T) {
	callCount := 0
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		callCount++
		if callCount == 1 {
			return toolCallResponse("ask", "call_1"), nil
		}
		return textResponse("Done! You approved it."), nil
	}}
	agent := buildAgent(t, model, &suspendingTool{})

	_, client := startServer(t, agent)

	// First message triggers suspend.
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("Do something risky"))
	result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: msg,
	})
	assert.NoError(t, err)

	task, ok := result.(*a2a.Task)
	assert.True(t, ok)
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)

	// Resume by sending a follow-up message targeting the same task.
	resumeMsg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("yes, approved"))
	resumeMsg.TaskID = task.ID
	resumeMsg.ContextID = task.ContextID

	result2, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: resumeMsg,
	})
	assert.NoError(t, err)

	task2, ok := result2.(*a2a.Task)
	assert.True(t, ok)
	assert.Equal(t, a2a.TaskStateCompleted, task2.Status.State)
	assert.True(t, len(task2.Artifacts) > 0)
	assert.Equal(t, "Done! You approved it.", task2.Artifacts[0].Parts[0].Text())
}

func TestStreamMessage(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("streamed response"), nil
	}}
	agent := buildAgent(t, model)

	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello"))
	var events []a2a.Event
	for event, err := range client.SendStreamingMessage(context.Background(), &a2a.SendMessageRequest{
		Message: msg,
	}) {
		assert.NoError(t, err)
		events = append(events, event)
	}

	// Should have at least: submitted task, working status, artifact, completed status.
	assert.True(t, len(events) >= 3)

	// Last event should be a completed status or task.
	last := events[len(events)-1]
	switch v := last.(type) {
	case *a2a.TaskStatusUpdateEvent:
		assert.Equal(t, a2a.TaskStateCompleted, v.Status.State)
	case *a2a.Task:
		assert.Equal(t, a2a.TaskStateCompleted, v.Status.State)
	default:
		assert.True(t, false, "unexpected last event type: %T", last)
	}
}

func TestRemoteAgentSendText(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("Hello from the remote agent!"), nil
	}}
	agent := buildAgent(t, model)
	ts, _ := startServer(t, agent)

	// Create a RemoteAgent from a card.
	card := &a2a.AgentCard{
		Name:        "Test Agent",
		Description: "test",
		SupportedInterfaces: []*a2a.AgentInterface{{
			URL:             ts.URL,
			ProtocolBinding: a2a.TransportProtocolJSONRPC,
			ProtocolVersion: a2a.Version,
		}},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}
	remote, err := a2alib.NewRemoteAgentFromCard(context.Background(), card)
	assert.NoError(t, err)

	task, err := remote.SendText(context.Background(), "hi there")
	assert.NoError(t, err)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)

	// ResponseText should extract the answer.
	text := a2alib.ResponseText(task)
	assert.Equal(t, "Hello from the remote agent!", text)

	// ContextID should be set after the first call.
	assert.True(t, remote.ContextID() != "")
}

func TestRemoteAgentSendTextOnTask(t *testing.T) {
	callCount := 0
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		callCount++
		if callCount == 1 {
			return toolCallResponse("ask", "call_1"), nil
		}
		return textResponse("Resumed successfully."), nil
	}}
	agent := buildAgent(t, model, &suspendingTool{})
	ts, _ := startServer(t, agent)

	card := &a2a.AgentCard{
		Name:        "Test Agent",
		Description: "test",
		SupportedInterfaces: []*a2a.AgentInterface{{
			URL:             ts.URL,
			ProtocolBinding: a2a.TransportProtocolJSONRPC,
			ProtocolVersion: a2a.Version,
		}},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
	}
	remote, err := a2alib.NewRemoteAgentFromCard(context.Background(), card)
	assert.NoError(t, err)

	// First call suspends.
	task, err := remote.SendText(context.Background(), "do something")
	assert.NoError(t, err)
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)

	// Resume on the same task.
	task2, err := remote.SendTextOnTask(context.Background(), task.ID, "yes")
	assert.NoError(t, err)
	assert.Equal(t, a2a.TaskStateCompleted, task2.Status.State)
	assert.Equal(t, "Resumed successfully.", a2alib.ResponseText(task2))
}

func TestResponseTextFallsBackToHistory(t *testing.T) {
	// Task with no artifacts but an agent message in history.
	task := &a2a.Task{
		Status: a2a.TaskStatus{State: a2a.TaskStateCompleted},
		History: []*a2a.Message{
			a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hi")),
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("hello from history")),
		},
	}
	assert.Equal(t, "hello from history", a2alib.ResponseText(task))

	// Nil task.
	assert.Equal(t, "", a2alib.ResponseText(nil))
}

func TestGetTask(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("stored response"), nil
	}}
	agent := buildAgent(t, model)

	_, client := startServer(t, agent)

	// Send a message first.
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello"))
	result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{
		Message: msg,
	})
	assert.NoError(t, err)

	task, ok := result.(*a2a.Task)
	assert.True(t, ok)

	// Retrieve it.
	fetched, err := client.GetTask(context.Background(), &a2a.GetTaskRequest{ID: task.ID})
	assert.NoError(t, err)
	assert.Equal(t, task.ID, fetched.ID)
	assert.Equal(t, a2a.TaskStateCompleted, fetched.Status.State)
}

func TestAgentCardProvider(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("hello"), nil
	}}
	agent := buildAgent(t, model)

	callCount := 0
	provider := func(ctx context.Context) (*a2a.AgentCard, error) {
		callCount++
		return &a2a.AgentCard{
			Name:               "Dynamic Agent",
			Description:        "v" + string(rune('0'+callCount)),
			DefaultInputModes:  []string{"text/plain"},
			DefaultOutputModes: []string{"text/plain"},
		}, nil
	}

	srv, err := a2alib.NewServer(a2alib.ServerOptions{
		Agent:        agent,
		CardProvider: provider,
	})
	assert.NoError(t, err)

	// Card() returns nil when a provider is set.
	assert.True(t, srv.Card() == nil)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Each GET to /.well-known/agent-card.json calls the provider.
	for i := 0; i < 3; i++ {
		resp, err := http.Get(ts.URL + a2alib.WellKnownAgentCardPath)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var card a2a.AgentCard
		assert.NoError(t, json.Unmarshal(body, &card))
		assert.Equal(t, "Dynamic Agent", card.Name)
	}
	assert.Equal(t, 3, callCount)
}
