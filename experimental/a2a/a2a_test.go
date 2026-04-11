package a2a_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/a2a"
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

func (t *suspendingTool) Name() string                  { return "ask" }
func (t *suspendingTool) Description() string           { return "Ask the human for input" }
func (t *suspendingTool) Schema() *dive.Schema          { return nil }
func (t *suspendingTool) Annotations() *dive.ToolAnnotations {
	return &dive.ToolAnnotations{Title: "Ask"}
}
func (t *suspendingTool) Call(ctx context.Context, input any) (*dive.ToolResult, error) {
	return dive.NewSuspendResult("Need your approval", map[string]any{"kind": "approval"}), nil
}

// ---------------------------------------------------------------------------
// Types / encoding
// ---------------------------------------------------------------------------

func TestTaskStateIsTerminal(t *testing.T) {
	terminal := []a2a.TaskState{
		a2a.TaskStateCompleted,
		a2a.TaskStateCanceled,
		a2a.TaskStateFailed,
		a2a.TaskStateRejected,
	}
	for _, s := range terminal {
		assert.True(t, s.IsTerminal())
	}
	nonTerminal := []a2a.TaskState{
		a2a.TaskStateSubmitted,
		a2a.TaskStateWorking,
		a2a.TaskStateInputRequired,
		a2a.TaskStateAuthRequired,
	}
	for _, s := range nonTerminal {
		assert.False(t, s.IsTerminal())
	}
}

func TestMessageJSONOmitsNullParts(t *testing.T) {
	msg := a2a.Message{MessageID: "m1", Role: a2a.RoleUser}
	b, err := json.Marshal(msg)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(string(b), `"parts":[]`))
}

func TestSendMessageParamsValidate(t *testing.T) {
	valid := &a2a.SendMessageParams{
		Message: &a2a.Message{
			MessageID: "m1",
			Role:      a2a.RoleUser,
			Parts:     []a2a.Part{a2a.NewTextPart("hello")},
		},
	}
	assert.NoError(t, valid.Validate())

	cases := []*a2a.SendMessageParams{
		nil,
		{},
		{Message: &a2a.Message{Role: a2a.RoleUser, Parts: []a2a.Part{a2a.NewTextPart("x")}}},
		{Message: &a2a.Message{MessageID: "m", Parts: []a2a.Part{a2a.NewTextPart("x")}}},
		{Message: &a2a.Message{MessageID: "m", Role: a2a.RoleAgent, Parts: []a2a.Part{a2a.NewTextPart("x")}}},
		{Message: &a2a.Message{MessageID: "m", Role: a2a.RoleUser}},
	}
	for i, c := range cases {
		err := c.Validate()
		assert.Error(t, err)
		_ = i
	}
}

// ---------------------------------------------------------------------------
// Server: well-known agent card
// ---------------------------------------------------------------------------

func buildAgent(t *testing.T, model llm.LLM, tools ...dive.Tool) *dive.Agent {
	t.Helper()
	agent, err := dive.NewAgent(dive.AgentOptions{
		Name:  "Research Assistant",
		Model: model,
		Tools: tools,
	})
	assert.NoError(t, err)
	return agent
}

func TestServerAgentCard(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("ok"), nil
	}}
	agent := buildAgent(t, model)

	server, err := a2a.NewServer(a2a.ServerOptions{
		Agent:   agent,
		BaseURL: "https://agent.example.com",
	})
	assert.NoError(t, err)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent.json")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	var card a2a.AgentCard
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&card))
	assert.Equal(t, card.Name, "Research Assistant")
	assert.True(t, card.Capabilities.Streaming)
	assert.True(t, strings.HasPrefix(card.URL, "https://agent.example.com"))
}

// ---------------------------------------------------------------------------
// Server: message/send happy path
// ---------------------------------------------------------------------------

func TestServerMessageSendCompletion(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("Paris is the capital of France."), nil
	}}
	agent := buildAgent(t, model)

	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)

	task, err := client.SendMessage(context.Background(), &a2a.Message{
		MessageID: "m1",
		Role:      a2a.RoleUser,
		Parts:     []a2a.Part{a2a.NewTextPart("What is the capital of France?")},
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, task.Status.State, a2a.TaskStateCompleted)
	assert.Equal(t, a2a.ResponseText(task), "Paris is the capital of France.")
	assert.NotEqual(t, task.ID, "")
	assert.NotEqual(t, task.ContextID, "")
}

// ---------------------------------------------------------------------------
// Server: suspend → input-required → resume
// ---------------------------------------------------------------------------

func TestServerSuspendMapsToInputRequired(t *testing.T) {
	var callNum atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := callNum.Add(1)
		switch n {
		case 1:
			return toolCallResponse("ask", "call_1"), nil
		case 2:
			return textResponse("Thanks, I have what I need."), nil
		}
		return textResponse("unexpected"), nil
	}}
	agent := buildAgent(t, model, &suspendingTool{})

	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)

	task, err := client.SendMessage(context.Background(), &a2a.Message{
		MessageID: "m1",
		Role:      a2a.RoleUser,
		Parts:     []a2a.Part{a2a.NewTextPart("Please proceed")},
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, task.Status.State, a2a.TaskStateInputRequired)
	assert.NotNil(t, task.Status.Message)
	assert.Equal(t, task.Status.Message.TextContent(), "Need your approval")

	// Resume by sending a new message targeting the same task.
	resumed, err := client.SendMessage(context.Background(), &a2a.Message{
		MessageID: "m2",
		Role:      a2a.RoleUser,
		Parts:     []a2a.Part{a2a.NewTextPart("approved")},
		TaskID:    task.ID,
		ContextID: task.ContextID,
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, resumed.Status.State, a2a.TaskStateCompleted)
	assert.Equal(t, a2a.ResponseText(resumed), "Thanks, I have what I need.")
}

// ---------------------------------------------------------------------------
// Server: tasks/get and tasks/cancel
// ---------------------------------------------------------------------------

func TestServerTasksGetAndCancel(t *testing.T) {
	var callNum atomic.Int32
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		n := callNum.Add(1)
		if n == 1 {
			return toolCallResponse("ask", "call_1"), nil
		}
		return textResponse("done"), nil
	}}
	agent := buildAgent(t, model, &suspendingTool{})

	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)

	task, err := client.SendMessage(context.Background(), &a2a.Message{
		MessageID: "m1",
		Role:      a2a.RoleUser,
		Parts:     []a2a.Part{a2a.NewTextPart("Please proceed")},
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, task.Status.State, a2a.TaskStateInputRequired)

	got, err := client.GetTask(context.Background(), task.ID)
	assert.NoError(t, err)
	assert.Equal(t, got.ID, task.ID)
	assert.Equal(t, got.Status.State, a2a.TaskStateInputRequired)

	canceled, err := client.CancelTask(context.Background(), task.ID)
	assert.NoError(t, err)
	assert.Equal(t, canceled.Status.State, a2a.TaskStateCanceled)

	// Cancelling again should fail with TaskNotCancelable.
	_, err = client.CancelTask(context.Background(), task.ID)
	assert.Error(t, err)
	rpcErr, ok := err.(*a2a.RPCError)
	assert.True(t, ok)
	assert.Equal(t, rpcErr.Code, a2a.ErrorCodeTaskNotCancelable)
}

// ---------------------------------------------------------------------------
// Server: tasks/get for unknown id
// ---------------------------------------------------------------------------

func TestServerTasksGetUnknown(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("ok"), nil
	}}
	agent := buildAgent(t, model)
	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)

	_, err = client.GetTask(context.Background(), "does-not-exist")
	assert.Error(t, err)
	rpcErr, ok := err.(*a2a.RPCError)
	assert.True(t, ok)
	assert.Equal(t, rpcErr.Code, a2a.ErrorCodeTaskNotFound)
}

// ---------------------------------------------------------------------------
// Server: streaming
// ---------------------------------------------------------------------------

func TestServerMessageStream(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("streamed hello"), nil
	}}
	agent := buildAgent(t, model)
	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)

	var (
		statusEvents   []*a2a.TaskStatusUpdateEvent
		artifactEvents []*a2a.TaskArtifactUpdateEvent
	)
	err = client.StreamMessage(context.Background(), &a2a.Message{
		MessageID: "m1",
		Role:      a2a.RoleUser,
		Parts:     []a2a.Part{a2a.NewTextPart("stream please")},
	}, nil, func(ev *a2a.StreamEvent) error {
		switch {
		case ev.StatusUpdate != nil:
			statusEvents = append(statusEvents, ev.StatusUpdate)
		case ev.ArtifactUpdate != nil:
			artifactEvents = append(artifactEvents, ev.ArtifactUpdate)
		}
		return nil
	})
	assert.NoError(t, err)
	assert.True(t, len(statusEvents) > 0)
	final := statusEvents[len(statusEvents)-1]
	assert.Equal(t, final.Status.State, a2a.TaskStateCompleted)
	assert.True(t, final.Final)
	assert.Len(t, artifactEvents, 1)
	assert.Equal(t, artifactEvents[0].Artifact.Parts[0].Text, "streamed hello")
}

// ---------------------------------------------------------------------------
// RemoteAgent wrapper
// ---------------------------------------------------------------------------

func TestRemoteAgentSendText(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("remote response"), nil
	}}
	agent := buildAgent(t, model)
	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client, err := a2a.NewClient(a2a.ClientOptions{Endpoint: ts.URL + "/"})
	assert.NoError(t, err)
	remote := a2a.NewRemoteAgent(client)

	task, err := remote.SendText(context.Background(), "ping")
	assert.NoError(t, err)
	assert.Equal(t, a2a.ResponseText(task), "remote response")
	assert.NotEqual(t, remote.ContextID(), "")

	// Second call should reuse the contextId.
	prevCtxID := remote.ContextID()
	task2, err := remote.SendText(context.Background(), "again")
	assert.NoError(t, err)
	assert.Equal(t, task2.ContextID, prevCtxID)
}

// ---------------------------------------------------------------------------
// JSON-RPC error cases
// ---------------------------------------------------------------------------

func TestServerInvalidMethod(t *testing.T) {
	model := &fakeLLM{generate: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
		return textResponse("ok"), nil
	}}
	agent := buildAgent(t, model)
	server, err := a2a.NewServer(a2a.ServerOptions{Agent: agent})
	assert.NoError(t, err)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"mystery/do","params":{}}`
	resp, err := http.Post(ts.URL+"/", "application/json", strings.NewReader(body))
	assert.NoError(t, err)
	defer resp.Body.Close()
	var env struct {
		Error *a2a.RPCError `json:"error"`
	}
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	assert.NotNil(t, env.Error)
	assert.Equal(t, env.Error.Code, a2a.ErrorCodeMethodNotFound)
}
