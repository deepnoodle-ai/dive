package a2alib_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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

func (t *suspendingTool) Name() string                       { return "ask" }
func (t *suspendingTool) Description() string                { return "Ask the human for input" }
func (t *suspendingTool) Schema() *dive.Schema               { return nil }
func (t *suspendingTool) Annotations() *dive.ToolAnnotations { return &dive.ToolAnnotations{Title: "Ask"} }
func (t *suspendingTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return dive.NewSuspendResult("Need your approval", map[string]any{"kind": "approval"}), nil
}

type confirmTool struct{}

func (t *confirmTool) Name() string                       { return "confirm" }
func (t *confirmTool) Description() string                { return "Confirm with the human" }
func (t *confirmTool) Schema() *dive.Schema               { return nil }
func (t *confirmTool) Annotations() *dive.ToolAnnotations { return &dive.ToolAnnotations{Title: "Confirm"} }
func (t *confirmTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return dive.NewSuspendResult("Please confirm", nil), nil
}

type authSuspendTool struct{}

func (t *authSuspendTool) Name() string                       { return "auth_gate" }
func (t *authSuspendTool) Description() string                { return "Require authentication" }
func (t *authSuspendTool) Schema() *dive.Schema               { return nil }
func (t *authSuspendTool) Annotations() *dive.ToolAnnotations { return &dive.ToolAnnotations{Title: "AuthGate"} }
func (t *authSuspendTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return dive.NewSuspendResultWithReason("Sign in to continue",
		dive.SuspendReasonAuth, map[string]any{"auth_url": "https://example.com/oauth"}), nil
}

func multiToolCallResponse(calls ...struct{ name, id string }) *llm.Response {
	var content []llm.Content
	for _, c := range calls {
		content = append(content, &llm.ToolUseContent{
			ID:    c.id,
			Name:  c.name,
			Input: json.RawMessage(`{}`),
		})
	}
	return &llm.Response{
		ID:         "resp_tool",
		Model:      "fake-model",
		Role:       llm.Assistant,
		Content:    content,
		Type:       "message",
		StopReason: "tool_use",
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
	}
}

func imageResponse(text string) *llm.Response {
	return &llm.Response{
		ID:   "resp_img",
		Model: "fake-model",
		Role:  llm.Assistant,
		Content: []llm.Content{
			&llm.TextContent{Text: text},
			&llm.ImageContent{Source: &llm.ContentSource{
				Type:      "url",
				MediaType: "image/png",
				URL:       "https://example.com/chart.png",
			}},
		},
		Type:       "message",
		StopReason: "stop",
		Usage:      llm.Usage{InputTokens: 5, OutputTokens: 10},
	}
}

func buildParallelAgent(t *testing.T, model llm.LLM, tools ...dive.Tool) *dive.Agent {
	t.Helper()
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:                  "Test Agent",
		Model:                 model,
		Tools:                 tools,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)
	return agent
}

// sendAndExpectTask is a helper that sends a message and asserts a *Task result.
func sendAndExpectTask(t *testing.T, client *a2aclient.Client, msg *a2a.Message) *a2a.Task {
	t.Helper()
	result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: msg})
	assert.NoError(t, err)
	task, ok := result.(*a2a.Task)
	assert.True(t, ok)
	return task
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

	// Each GET to /.well-known/agent-card.json calls the provider and the
	// description encodes the call count, proving it changes on each request.
	for i := 0; i < 3; i++ {
		resp, err := http.Get(ts.URL + a2alib.WellKnownAgentCardPath)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var card a2a.AgentCard
		assert.NoError(t, json.Unmarshal(body, &card))
		assert.Equal(t, "Dynamic Agent", card.Name)
		assert.Equal(t, fmt.Sprintf("v%d", i+1), card.Description)
	}
	assert.Equal(t, 3, callCount)
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestConcurrentMessageSend(t *testing.T) {
	var counter atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := counter.Add(1)
		return textResponse(fmt.Sprintf("response %d", n)), nil
	}}
	agent := buildAgent(t, model)
	_, client := startServer(t, agent)

	const N = 20
	var wg sync.WaitGroup
	errs := make([]error, N)
	tasks := make([]*a2a.Task, N)

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(fmt.Sprintf("request %d", idx)))
			result, err := client.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: msg})
			errs[idx] = err
			if err == nil {
				tasks[idx], _ = result.(*a2a.Task)
			}
		}(i)
	}
	wg.Wait()

	ids := make(map[a2a.TaskID]bool)
	for i := 0; i < N; i++ {
		assert.NoError(t, errs[i])
		assert.Equal(t, a2a.TaskStateCompleted, tasks[i].Status.State)
		assert.False(t, ids[tasks[i].ID])
		ids[tasks[i].ID] = true
	}
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

func TestCancelBeforeResume(t *testing.T) {
	var callNum atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := callNum.Add(1)
		if n == 1 {
			return toolCallResponse("ask", "call_1"), nil
		}
		return textResponse("done"), nil
	}}
	agent := buildAgent(t, model, &suspendingTool{})
	_, client := startServer(t, agent)

	// Trigger a suspend.
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("start"))
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)

	// Cancel the suspended task.
	canceled, err := client.CancelTask(context.Background(), &a2a.CancelTaskRequest{ID: task.ID})
	assert.NoError(t, err)
	assert.Equal(t, a2a.TaskStateCanceled, canceled.Status.State)

	// Attempting to resume a canceled task must fail.
	resumeMsg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("too late"))
	resumeMsg.TaskID = task.ID
	_, err = client.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: resumeMsg})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Multi-pending-tool-call resume
// ---------------------------------------------------------------------------

func TestMultiPendingResumeDataPart(t *testing.T) {
	var callNum atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := callNum.Add(1)
		if n == 1 {
			return multiToolCallResponse(
				struct{ name, id string }{"ask", "call_1"},
				struct{ name, id string }{"confirm", "call_2"},
			), nil
		}
		return textResponse("Both answered, thanks."), nil
	}}
	agent := buildParallelAgent(t, model, &suspendingTool{}, &confirmTool{})
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("parallel request"))
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)

	// Resume with structured toolResults DataPart.
	resumeMsg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.NewTextPart("see attached"),
		a2a.NewDataPart(map[string]any{
			"toolResults": map[string]any{
				"call_1": "approved",
				"call_2": "confirmed",
			},
		}),
	)
	resumeMsg.TaskID = task.ID
	resumeMsg.ContextID = task.ContextID

	task2 := sendAndExpectTask(t, client, resumeMsg)
	assert.Equal(t, a2a.TaskStateCompleted, task2.Status.State)
	assert.Equal(t, "Both answered, thanks.", a2alib.ResponseText(task2))
}

func TestMultiPendingResumeTextBroadcast(t *testing.T) {
	var callNum atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := callNum.Add(1)
		if n == 1 {
			return multiToolCallResponse(
				struct{ name, id string }{"ask", "call_1"},
				struct{ name, id string }{"confirm", "call_2"},
			), nil
		}
		return textResponse("Got it."), nil
	}}
	agent := buildParallelAgent(t, model, &suspendingTool{}, &confirmTool{})
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("go"))
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)

	// Resume with plain text — broadcasts to all pending calls.
	resumeMsg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("yes to all"))
	resumeMsg.TaskID = task.ID
	resumeMsg.ContextID = task.ContextID

	task2 := sendAndExpectTask(t, client, resumeMsg)
	assert.Equal(t, a2a.TaskStateCompleted, task2.Status.State)
	assert.Equal(t, "Got it.", a2alib.ResponseText(task2))
}

// ---------------------------------------------------------------------------
// Auth-required suspend
// ---------------------------------------------------------------------------

func TestAuthRequiredSuspend(t *testing.T) {
	var callNum atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := callNum.Add(1)
		if n == 1 {
			return toolCallResponse("auth_gate", "call_auth"), nil
		}
		return textResponse("authenticated"), nil
	}}
	agent := buildAgent(t, model, &authSuspendTool{})
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("access the resource"))
	task := sendAndExpectTask(t, client, msg)
	// Auth-required uses TaskStateInputRequired so the executor terminates and
	// resume works via a new SendMessage. The suspend reason is in metadata.
	assert.Equal(t, a2a.TaskStateInputRequired, task.Status.State)
	assert.NotNil(t, task.Status.Message)

	// Resume with auth result.
	resumeMsg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("token_abc"))
	resumeMsg.TaskID = task.ID
	resumeMsg.ContextID = task.ContextID
	task2 := sendAndExpectTask(t, client, resumeMsg)
	assert.Equal(t, a2a.TaskStateCompleted, task2.Status.State)
}

// ---------------------------------------------------------------------------
// Content projection
// ---------------------------------------------------------------------------

func TestImagePartProjectedToImageContent(t *testing.T) {
	var hasImage bool
	var imageURL string
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		cfg := &llm.Config{}
		cfg.Apply(opts...)
		for _, msg := range cfg.Messages {
			if msg.Role != llm.User {
				continue
			}
			for _, c := range msg.Content {
				if img, ok := c.(*llm.ImageContent); ok {
					hasImage = true
					if img.Source != nil {
						imageURL = img.Source.URL
					}
				}
			}
		}
		return textResponse("nice picture"), nil
	}}
	agent := buildAgent(t, model)
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.NewTextPart("What's in this image?"),
		a2a.NewFileURLPart("https://example.com/photo.png", "image/png"),
	)
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)
	assert.True(t, hasImage)
	assert.Equal(t, "https://example.com/photo.png", imageURL)
}

func TestDocumentPartProjectedToDocumentContent(t *testing.T) {
	var hasDocument bool
	var docURL string
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		cfg := &llm.Config{}
		cfg.Apply(opts...)
		for _, msg := range cfg.Messages {
			if msg.Role != llm.User {
				continue
			}
			for _, c := range msg.Content {
				if doc, ok := c.(*llm.DocumentContent); ok {
					hasDocument = true
					if doc.Source != nil {
						docURL = doc.Source.URL
					}
				}
			}
		}
		return textResponse("got it"), nil
	}}
	agent := buildAgent(t, model)
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.NewTextPart("Summarize this PDF."),
		a2a.NewFileURLPart("https://example.com/invoice.pdf", "application/pdf"),
	)
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)
	assert.True(t, hasDocument)
	assert.Equal(t, "https://example.com/invoice.pdf", docURL)
}

func TestResponseArtifactsIncludeImageParts(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return imageResponse("Here is the chart"), nil
	}}
	agent := buildAgent(t, model)
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("Make me a chart"))
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)
	assert.True(t, len(task.Artifacts) > 0)

	art := task.Artifacts[0]
	assert.True(t, len(art.Parts) >= 2)

	var hasText, hasImage bool
	for _, p := range art.Parts {
		if p.Text() != "" {
			hasText = true
		}
		if p.URL() != "" {
			hasImage = true
		}
	}
	assert.True(t, hasText)
	assert.True(t, hasImage)
}

func TestDataPartRenderedAsJSONBlock(t *testing.T) {
	var textContent string
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		cfg := &llm.Config{}
		cfg.Apply(opts...)
		for _, msg := range cfg.Messages {
			if msg.Role != llm.User {
				continue
			}
			for _, c := range msg.Content {
				if tc, ok := c.(*llm.TextContent); ok {
					textContent += tc.Text
				}
			}
		}
		return textResponse("got it"), nil
	}}
	agent := buildAgent(t, model)
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser,
		a2a.NewTextPart("Summarize the order."),
		a2a.NewDataPart(map[string]any{"orderId": "ABC-123", "total": 4299}),
	)
	task := sendAndExpectTask(t, client, msg)
	assert.Equal(t, a2a.TaskStateCompleted, task.Status.State)
	assert.True(t, len(textContent) > 0)
	assert.True(t, containsSubstring(textContent, "orderId") || containsSubstring(textContent, "ABC-123"))
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}

// ---------------------------------------------------------------------------
// Streaming artifact emission
// ---------------------------------------------------------------------------

func TestStreamingEmitsArtifacts(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return imageResponse("chart description"), nil
	}}
	agent := buildAgent(t, model)
	_, client := startServer(t, agent)

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("stream chart"))
	var artifactEvents []*a2a.TaskArtifactUpdateEvent
	var allEvents []a2a.Event
	for event, err := range client.SendStreamingMessage(context.Background(), &a2a.SendMessageRequest{Message: msg}) {
		assert.NoError(t, err)
		allEvents = append(allEvents, event)
		if v, ok := event.(*a2a.TaskArtifactUpdateEvent); ok {
			artifactEvents = append(artifactEvents, v)
		}
	}
	assert.True(t, len(artifactEvents) > 0)

	// Last event should be a completed status.
	last := allEvents[len(allEvents)-1]
	switch v := last.(type) {
	case *a2a.TaskStatusUpdateEvent:
		assert.Equal(t, a2a.TaskStateCompleted, v.Status.State)
	case *a2a.Task:
		assert.Equal(t, a2a.TaskStateCompleted, v.Status.State)
	}

	// The artifact should include both text and image parts.
	art := artifactEvents[0].Artifact
	var hasText, hasURL bool
	for _, p := range art.Parts {
		if p.Text() != "" {
			hasText = true
		}
		if p.URL() != "" {
			hasURL = true
		}
	}
	assert.True(t, hasText)
	assert.True(t, hasURL)
}
