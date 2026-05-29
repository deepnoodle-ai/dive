package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/subagent"
	"github.com/deepnoodle-ai/wonton/assert"
)

// mockLLM implements llm.LLM for testing.
type mockLLM struct {
	response string
	err      error
	delay    time.Duration
}

func (m *mockLLM) Name() string { return "mock-llm" }

func (m *mockLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		ID:         "test-resp",
		Model:      "mock-llm",
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: m.response}},
		Type:       "message",
		StopReason: "stop",
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func mockAgent(name, response string, err error, delay time.Duration) (*dive.Agent, error) {
	return dive.NewAgent(dive.AgentOptions{
		Name:  name,
		Model: &mockLLM{response: response, err: err, delay: delay},
	})
}

func testTypes() map[string]*subagent.Definition {
	return map[string]*subagent.Definition{
		"GeneralPurpose": subagent.GeneralPurpose,
		"Explore":        subagent.Explore,
	}
}

// onlyRunID returns the single tracked run id (test helper for deterministic
// assertions on the unexported Runs map).
func onlyRunID(r *Runs) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var id string
	for k := range r.m {
		id = k
	}
	return id
}

func runCount(r *Runs) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}

func TestRuns(t *testing.T) {
	t.Run("stop cancels and removes", func(t *testing.T) {
		r := NewRuns()
		runCtx, cancel := context.WithCancel(context.Background())
		r.add("id1", "desc1", cancel)

		desc, ok := r.stop("id1")
		assert.True(t, ok)
		assert.Equal(t, "desc1", desc)
		select {
		case <-runCtx.Done():
		default:
			t.Fatal("expected stop to cancel the run's context")
		}

		// Second stop finds nothing.
		_, ok = r.stop("id1")
		assert.False(t, ok)
	})

	t.Run("remove drops without a second cancel", func(t *testing.T) {
		r := NewRuns()
		_, cancel := context.WithCancel(context.Background())
		r.add("id2", "desc2", cancel)
		r.remove("id2")
		_, ok := r.stop("id2")
		assert.False(t, ok)
		cancel()
	})

	t.Run("stop on unknown id", func(t *testing.T) {
		r := NewRuns()
		_, ok := r.stop("missing")
		assert.False(t, ok)
	})
}

func TestTaskStopTool(t *testing.T) {
	ctx := context.Background()

	t.Run("cancels a tracked run", func(t *testing.T) {
		r := NewRuns()
		_, cancel := context.WithCancel(context.Background())
		r.add("task_x", "indexing", cancel)

		tool := NewTaskStopTool(TaskStopToolOptions{Runs: r})
		res, err := tool.Call(ctx, &TaskStopToolInput{TaskID: "task_x"})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "cancelled")
		assert.Equal(t, 0, runCount(r))
	})

	t.Run("unknown id", func(t *testing.T) {
		tool := NewTaskStopTool(TaskStopToolOptions{Runs: NewRuns()})
		res, err := tool.Call(ctx, &TaskStopToolInput{TaskID: "nope"})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "not found or already finished")
	})

	t.Run("nil runs", func(t *testing.T) {
		tool := NewTaskStopTool(TaskStopToolOptions{})
		res, err := tool.Call(ctx, &TaskStopToolInput{TaskID: "x"})
		assert.NoError(t, err)
		assert.Contains(t, res.Content[0].Text, "not found or already finished")
	})

	t.Run("missing task_id", func(t *testing.T) {
		tool := NewTaskStopTool(TaskStopToolOptions{Runs: NewRuns()})
		res, err := tool.Call(ctx, &TaskStopToolInput{})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "task_id is required")
	})
}

func TestAgentTool(t *testing.T) {
	ctx := context.Background()

	t.Run("synchronous execution returns output", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("test-agent", "Task completed successfully", nil, 0)
			},
			DefaultTimeout: 5 * time.Second,
		})
		res, err := tool.Call(ctx, &AgentToolInput{
			Prompt:       "Do something",
			Description:  "Test task",
			SubagentType: "GeneralPurpose",
		})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "Task completed successfully")
	})

	t.Run("unknown subagent type", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("a", "ok", nil, 0)
			},
		})
		res, err := tool.Call(ctx, &AgentToolInput{Prompt: "x", Description: "t", SubagentType: "nope"})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "unknown subagent type")
	})

	t.Run("agent error", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("failing", "", errors.New("agent failed"), 0)
			},
		})
		res, err := tool.Call(ctx, &AgentToolInput{Prompt: "x", Description: "t", SubagentType: "GeneralPurpose"})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "agent failed")
	})

	t.Run("factory error", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return nil, errors.New("factory boom")
			},
		})
		res, err := tool.Call(ctx, &AgentToolInput{Prompt: "x", Description: "t", SubagentType: "GeneralPurpose"})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "failed to create agent")
	})

	t.Run("missing required fields", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("a", "ok", nil, 0)
			},
		})
		res, _ := tool.Call(ctx, &AgentToolInput{Description: "t", SubagentType: "GeneralPurpose"})
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "prompt is required")

		res, _ = tool.Call(ctx, &AgentToolInput{Prompt: "x", SubagentType: "GeneralPurpose"})
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "description is required")

		res, _ = tool.Call(ctx, &AgentToolInput{Prompt: "x", Description: "t"})
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "subagent_type is required")
	})

	t.Run("nil factory returns an error instead of panicking", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{Subagents: testTypes()})
		res, err := tool.Call(ctx, &AgentToolInput{Prompt: "x", Description: "t", SubagentType: "GeneralPurpose"})
		assert.NoError(t, err)
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "misconfigured")
	})

	t.Run("catalog is copied defensively", func(t *testing.T) {
		types := map[string]*subagent.Definition{"x": {Description: "d", Prompt: "p"}}
		tool := NewAgentTool(AgentToolOptions{
			Subagents: types,
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("a", "ok", nil, 0)
			},
		})
		// Mutating the caller's map after construction must not affect the tool.
		delete(types, "x")
		res, err := tool.Call(ctx, &AgentToolInput{Prompt: "p", Description: "d", SubagentType: "x"})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
	})

	t.Run("background spawn is dispatched and tracked", func(t *testing.T) {
		runs := NewRuns()
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			Runs:      runs,
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("bg", "done", nil, 2*time.Second)
			},
		})
		res, err := tool.Call(ctx, &AgentToolInput{
			Prompt:          "Do something",
			Description:     "Background task",
			SubagentType:    "GeneralPurpose",
			RunInBackground: true,
		})
		assert.NoError(t, err)
		assert.NotNil(t, res.Background)
		assert.Equal(t, 0, len(res.Content))

		// Registered while in flight, keyed by a task_ id.
		assert.Equal(t, 1, runCount(runs))
		id := onlyRunID(runs)
		assert.True(t, strings.HasPrefix(id, "task_"))

		// TaskStop cancels and untracks it.
		stop := NewTaskStopTool(TaskStopToolOptions{Runs: runs})
		sres, _ := stop.Call(ctx, &TaskStopToolInput{TaskID: id})
		assert.Contains(t, sres.Content[0].Text, "cancelled")
		assert.Equal(t, 0, runCount(runs))
	})

	t.Run("synchronous timeout returns quickly", func(t *testing.T) {
		tool := NewAgentTool(AgentToolOptions{
			Subagents: testTypes(),
			AgentFactory: func(ctx context.Context, name string, def *subagent.Definition, parentTools []dive.Tool) (*dive.Agent, error) {
				return mockAgent("slow", "should not appear", nil, 5*time.Second)
			},
			DefaultTimeout: 200 * time.Millisecond,
		})
		start := time.Now()
		res, err := tool.Call(ctx, &AgentToolInput{Prompt: "slow", Description: "Timeout test", SubagentType: "GeneralPurpose"})
		elapsed := time.Since(start)
		assert.NoError(t, err)
		assert.True(t, res.IsError)
		assert.True(t, elapsed < 2*time.Second)
	})
}

func TestAgentToolMetadata(t *testing.T) {
	tool := NewAgentTool(AgentToolOptions{
		Subagents: map[string]*subagent.Definition{"GeneralPurpose": subagent.GeneralPurpose},
	})
	assert.Equal(t, "Agent", tool.Name())

	s := tool.Schema()
	assert.Contains(t, s.Required, "prompt")
	assert.Contains(t, s.Required, "description")
	assert.Contains(t, s.Required, "subagent_type")

	desc := tool.Description()
	assert.Contains(t, desc, "Available subagent types:")
	assert.Contains(t, desc, "GeneralPurpose")
}

func TestMonitorTool(t *testing.T) {
	ctx := context.Background()

	t.Run("validation", func(t *testing.T) {
		tool := NewMonitorTool(MonitorToolOptions{})
		res, _ := tool.Call(ctx, &MonitorToolInput{Description: "d"})
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "command is required")

		res, _ = tool.Call(ctx, &MonitorToolInput{Command: "echo hi"})
		assert.True(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "description is required")
	})

	t.Run("registers a stoppable run", func(t *testing.T) {
		runs := NewRuns()
		tool := NewMonitorTool(MonitorToolOptions{Runs: runs})
		res, err := tool.Call(ctx, &MonitorToolInput{Command: "sleep 30", Description: "sleeping"})
		assert.NoError(t, err)
		assert.False(t, res.IsError)
		assert.Contains(t, res.Content[0].Text, "Monitor started")

		id := onlyRunID(runs)
		assert.True(t, strings.HasPrefix(id, "monitor_"))

		stop := NewTaskStopTool(TaskStopToolOptions{Runs: runs})
		sres, _ := stop.Call(ctx, &TaskStopToolInput{TaskID: id})
		assert.Contains(t, sres.Content[0].Text, "cancelled")
	})
}
