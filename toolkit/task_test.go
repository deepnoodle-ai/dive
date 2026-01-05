package toolkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// mockTaskAgent implements dive.Agent for testing
type mockTaskAgent struct {
	name     string
	response string
	err      error
	delay    time.Duration
}

func (m *mockTaskAgent) Name() string {
	return m.name
}

func (m *mockTaskAgent) CreateResponse(ctx context.Context, opts ...dive.CreateResponseOption) (*dive.Response, error) {
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
	return &dive.Response{
		Items: []*dive.ResponseItem{
			{
				Type: dive.ResponseItemTypeMessage,
				Message: &llm.Message{
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.TextContent{Text: m.response},
					},
				},
			},
		},
	}, nil
}

func TestTaskRegistry(t *testing.T) {
	registry := NewTaskRegistry()

	t.Run("register and get task", func(t *testing.T) {
		record := &TaskRecord{
			ID:          "task_123",
			Description: "test task",
			Status:      TaskStatusPending,
			StartTime:   time.Now(),
			done:        make(chan struct{}),
		}
		registry.Register(record)

		got, ok := registry.Get("task_123")
		assert.True(t, ok)
		assert.Equal(t, "task_123", got.ID)
		assert.Equal(t, "test task", got.Description)
	})

	t.Run("get non-existent task", func(t *testing.T) {
		_, ok := registry.Get("non_existent")
		assert.False(t, ok)
	})

	t.Run("list tasks", func(t *testing.T) {
		ids := registry.List()
		assert.Contains(t, ids, "task_123")
	})
}

func TestTaskTool(t *testing.T) {
	ctx := context.Background()

	t.Run("synchronous task execution", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return &mockTaskAgent{
					name:     "test-agent",
					response: "Task completed successfully",
				}, nil
			},
			DefaultTimeout: 5 * time.Second,
		})

		result, err := tool.Call(ctx, &TaskToolInput{
			Prompt:       "Do something",
			Description:  "Test task",
			SubagentType: "general-purpose",
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Task completed successfully")
	})

	t.Run("background task execution", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return &mockTaskAgent{
					name:     "test-agent",
					response: "Background task done",
					delay:    100 * time.Millisecond,
				}, nil
			},
		})

		result, err := tool.Call(ctx, &TaskToolInput{
			Prompt:          "Do something in background",
			Description:     "Background task",
			SubagentType:    "Explore",
			RunInBackground: true,
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Task started in background")
		assert.Contains(t, result.Content[0].Text, "task_")

		// Wait for background task to complete
		time.Sleep(200 * time.Millisecond)

		// Verify task completed
		tasks := registry.List()
		assert.Equal(t, 1, len(tasks))
		record, ok := registry.Get(tasks[0])
		assert.True(t, ok)
		assert.Equal(t, TaskStatusCompleted, record.Status)
		assert.Equal(t, "Background task done", record.Output)
	})

	t.Run("task with agent error", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return &mockTaskAgent{
					name: "failing-agent",
					err:  errors.New("agent failed"),
				}, nil
			},
		})

		result, err := tool.Call(ctx, &TaskToolInput{
			Prompt:       "This will fail",
			Description:  "Failing task",
			SubagentType: "general-purpose",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "agent failed")
	})

	t.Run("task with factory error", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return nil, errors.New("factory error")
			},
		})

		result, err := tool.Call(ctx, &TaskToolInput{
			Prompt:       "Do something",
			Description:  "Test task",
			SubagentType: "unknown-type",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "failed to create agent")
	})

	t.Run("missing required fields", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return &mockTaskAgent{name: "test"}, nil
			},
		})

		// Missing prompt
		result, err := tool.Call(ctx, &TaskToolInput{
			Description:  "Test",
			SubagentType: "general-purpose",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "prompt is required")

		// Missing description
		result, err = tool.Call(ctx, &TaskToolInput{
			Prompt:       "Do something",
			SubagentType: "general-purpose",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "description is required")

		// Missing subagent_type
		result, err = tool.Call(ctx, &TaskToolInput{
			Prompt:      "Do something",
			Description: "Test",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "subagent_type is required")
	})

	t.Run("resume non-existent task", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return &mockTaskAgent{name: "test"}, nil
			},
		})

		result, err := tool.Call(ctx, &TaskToolInput{
			Prompt:       "Continue work",
			Description:  "Resume test",
			SubagentType: "general-purpose",
			Resume:       "non_existent_id",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "task non_existent_id not found")
	})
}

func TestTaskOutputTool(t *testing.T) {
	ctx := context.Background()

	t.Run("get completed task output", func(t *testing.T) {
		registry := NewTaskRegistry()
		done := make(chan struct{})
		close(done)

		record := &TaskRecord{
			ID:          "task_abc",
			Description: "completed task",
			Status:      TaskStatusCompleted,
			Output:      "The result is 42",
			StartTime:   time.Now().Add(-5 * time.Second),
			EndTime:     time.Now(),
			done:        done,
		}
		registry.Register(record)

		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})
		result, err := tool.Call(ctx, &TaskOutputToolInput{
			TaskID: "task_abc",
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		text := result.Content[0].Text
		assert.Contains(t, text, "task_abc")
		assert.Contains(t, text, "completed")
		assert.Contains(t, text, "The result is 42")
	})

	t.Run("get non-existent task", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})

		result, err := tool.Call(ctx, &TaskOutputToolInput{
			TaskID: "non_existent",
		})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "task non_existent not found")
	})

	t.Run("non-blocking status check", func(t *testing.T) {
		registry := NewTaskRegistry()
		record := &TaskRecord{
			ID:          "task_running",
			Description: "running task",
			Status:      TaskStatusRunning,
			StartTime:   time.Now(),
			done:        make(chan struct{}), // not closed
		}
		registry.Register(record)

		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})
		block := false
		result, err := tool.Call(ctx, &TaskOutputToolInput{
			TaskID: "task_running",
			Block:  &block,
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		text := result.Content[0].Text
		assert.Contains(t, text, "task_running")
		assert.Contains(t, text, "running")
	})

	t.Run("blocking with timeout", func(t *testing.T) {
		registry := NewTaskRegistry()
		record := &TaskRecord{
			ID:          "task_slow",
			Description: "slow task",
			Status:      TaskStatusRunning,
			StartTime:   time.Now(),
			done:        make(chan struct{}), // not closed
		}
		registry.Register(record)

		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})
		block := true
		result, err := tool.Call(ctx, &TaskOutputToolInput{
			TaskID:  "task_slow",
			Block:   &block,
			Timeout: 100, // 100ms
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		// Should return after timeout with current status
		text := result.Content[0].Text
		assert.Contains(t, text, "task_slow")
		assert.Contains(t, text, "running")
	})

	t.Run("missing task_id", func(t *testing.T) {
		registry := NewTaskRegistry()
		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})

		result, err := tool.Call(ctx, &TaskOutputToolInput{})
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "task_id is required")
	})

	t.Run("task with error", func(t *testing.T) {
		registry := NewTaskRegistry()
		done := make(chan struct{})
		close(done)

		record := &TaskRecord{
			ID:          "task_failed",
			Description: "failed task",
			Status:      TaskStatusFailed,
			Output:      "Task failed: connection timeout",
			Error:       errors.New("connection timeout"),
			StartTime:   time.Now().Add(-2 * time.Second),
			EndTime:     time.Now(),
			done:        done,
		}
		registry.Register(record)

		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})
		result, err := tool.Call(ctx, &TaskOutputToolInput{
			TaskID: "task_failed",
		})
		assert.NoError(t, err)
		assert.False(t, result.IsError) // TaskOutput itself succeeds
		text := result.Content[0].Text
		assert.Contains(t, text, "failed")
		assert.Contains(t, text, "connection timeout")
	})
}

func TestToolMetadata(t *testing.T) {
	registry := NewTaskRegistry()

	t.Run("TaskTool metadata", func(t *testing.T) {
		tool := NewTaskTool(TaskToolOptions{
			Registry: registry,
			AgentFactory: func(ctx context.Context, name string, def *dive.SubagentDefinition, parentTools []dive.Tool) (dive.Agent, error) {
				return nil, nil
			},
		})

		assert.Equal(t, "Task", tool.Name())
		assert.NotEqual(t, "", tool.Description())
		assert.True(t, tool.ShouldReturnResult())

		schema := tool.Schema()
		assert.Equal(t, "object", string(schema.Type))
		assert.Contains(t, schema.Required, "prompt")
		assert.Contains(t, schema.Required, "description")
		assert.Contains(t, schema.Required, "subagent_type")

		annotations := tool.Annotations()
		assert.Equal(t, "Task", annotations.Title)
		assert.True(t, annotations.OpenWorldHint)
	})

	t.Run("TaskOutputTool metadata", func(t *testing.T) {
		tool := NewTaskOutputTool(TaskOutputToolOptions{Registry: registry})

		assert.Equal(t, "TaskOutput", tool.Name())
		assert.NotEqual(t, "", tool.Description())
		assert.True(t, tool.ShouldReturnResult())

		schema := tool.Schema()
		assert.Equal(t, "object", string(schema.Type))
		assert.Contains(t, schema.Required, "task_id")

		annotations := tool.Annotations()
		assert.Equal(t, "TaskOutput", annotations.Title)
		assert.True(t, annotations.ReadOnlyHint)
		assert.True(t, annotations.IdempotentHint)
	})
}
