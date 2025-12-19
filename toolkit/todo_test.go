package toolkit

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive/schema"
	"github.com/stretchr/testify/require"
)

func TestTodoWriteTool_Name(t *testing.T) {
	tool := NewTodoWriteTool()
	require.Equal(t, "todo_write", tool.Name())
}

func TestTodoWriteTool_Description(t *testing.T) {
	tool := NewTodoWriteTool()
	desc := tool.Description()
	require.Contains(t, desc, "task list")
	require.Contains(t, desc, "pending")
	require.Contains(t, desc, "in_progress")
	require.Contains(t, desc, "completed")
}

func TestTodoWriteTool_Schema(t *testing.T) {
	tool := NewTodoWriteTool()
	s := tool.Schema()

	require.Equal(t, schema.Object, s.Type)
	require.Contains(t, s.Required, "todos")
	require.Contains(t, s.Properties, "todos")
}

func TestTodoWriteTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateTodos", func(t *testing.T) {
		tool := NewTodoWriteTool()

		input := &TodoWriteInput{
			Todos: []TodoItem{
				{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
				{Content: "Task 2", Status: TodoStatusInProgress, ActiveForm: "Working on Task 2"},
				{Content: "Task 3", Status: TodoStatusCompleted, ActiveForm: "Working on Task 3"},
			},
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, float64(3), response["total"])
		require.Equal(t, float64(1), response["pending"])
		require.Equal(t, float64(1), response["in_progress"])
		require.Equal(t, float64(1), response["completed"])
	})

	t.Run("EmptyTodos", func(t *testing.T) {
		tool := NewTodoWriteTool()

		input := &TodoWriteInput{
			Todos: []TodoItem{},
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.False(t, result.IsError)

		var response map[string]interface{}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Equal(t, float64(0), response["total"])
	})

	t.Run("MissingContent", func(t *testing.T) {
		tool := NewTodoWriteTool()

		input := &TodoWriteInput{
			Todos: []TodoItem{
				{Content: "", Status: TodoStatusPending, ActiveForm: "Working"},
			},
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "content is required")
	})

	t.Run("MissingActiveForm", func(t *testing.T) {
		tool := NewTodoWriteTool()

		input := &TodoWriteInput{
			Todos: []TodoItem{
				{Content: "Task", Status: TodoStatusPending, ActiveForm: ""},
			},
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "activeForm is required")
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		tool := NewTodoWriteTool()

		input := &TodoWriteInput{
			Todos: []TodoItem{
				{Content: "Task", Status: "invalid", ActiveForm: "Working"},
			},
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "status must be")
	})

	t.Run("NilTodos", func(t *testing.T) {
		tool := NewTodoWriteTool()

		input := &TodoWriteInput{
			Todos: nil,
		}

		result, err := tool.Call(ctx, input)
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].Text, "todos array is required")
	})
}

func TestTodoWriteTool_OnUpdateCallback(t *testing.T) {
	ctx := context.Background()

	var callbackTodos []TodoItem
	var mu sync.Mutex

	tool := NewTodoWriteTool(TodoWriteToolOptions{
		OnUpdate: func(todos []TodoItem) {
			mu.Lock()
			defer mu.Unlock()
			callbackTodos = todos
		},
	})

	input := &TodoWriteInput{
		Todos: []TodoItem{
			{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
		},
	}

	_, err := tool.Call(ctx, input)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, callbackTodos, 1)
	require.Equal(t, "Task 1", callbackTodos[0].Content)
}

func TestTodoWriteTool_GetTodos(t *testing.T) {
	ctx := context.Background()

	adapter := NewTodoWriteTool()

	// Access the underlying tool to call GetTodos
	// We need to call through the adapter first to set todos
	input := &TodoWriteInput{
		Todos: []TodoItem{
			{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
			{Content: "Task 2", Status: TodoStatusInProgress, ActiveForm: "Working on Task 2"},
		},
	}

	_, err := adapter.Call(ctx, input)
	require.NoError(t, err)

	// Get the underlying tool using Unwrap
	tool := adapter.Unwrap().(*TodoWriteTool)
	todos := tool.GetTodos()

	require.Len(t, todos, 2)
	require.Equal(t, "Task 1", todos[0].Content)
	require.Equal(t, "Task 2", todos[1].Content)
}

func TestTodoWriteTool_GetCurrentTask(t *testing.T) {
	ctx := context.Background()

	adapter := NewTodoWriteTool()

	t.Run("WithInProgressTask", func(t *testing.T) {
		input := &TodoWriteInput{
			Todos: []TodoItem{
				{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
				{Content: "Task 2", Status: TodoStatusInProgress, ActiveForm: "Working on Task 2"},
			},
		}

		_, err := adapter.Call(ctx, input)
		require.NoError(t, err)

		tool := adapter.Unwrap().(*TodoWriteTool)
		current := tool.GetCurrentTask()

		require.NotNil(t, current)
		require.Equal(t, "Task 2", current.Content)
		require.Equal(t, TodoStatusInProgress, current.Status)
	})

	t.Run("NoInProgressTask", func(t *testing.T) {
		input := &TodoWriteInput{
			Todos: []TodoItem{
				{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
				{Content: "Task 2", Status: TodoStatusCompleted, ActiveForm: "Working on Task 2"},
			},
		}

		_, err := adapter.Call(ctx, input)
		require.NoError(t, err)

		tool := adapter.Unwrap().(*TodoWriteTool)
		current := tool.GetCurrentTask()

		require.Nil(t, current)
	})
}

func TestTodoWriteTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	tool := NewTodoWriteTool()

	input := &TodoWriteInput{
		Todos: []TodoItem{
			{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
			{Content: "Task 2", Status: TodoStatusInProgress, ActiveForm: "Working on Task 2"},
			{Content: "Task 3", Status: TodoStatusCompleted, ActiveForm: "Working on Task 3"},
		},
	}

	preview := tool.PreviewCall(ctx, input)
	require.Contains(t, preview.Summary, "1 pending")
	require.Contains(t, preview.Summary, "1 in progress")
	require.Contains(t, preview.Summary, "1 completed")
}

func TestTodoWriteTool_Annotations(t *testing.T) {
	tool := NewTodoWriteTool()
	annotations := tool.Annotations()

	require.NotNil(t, annotations)
	require.Equal(t, "Todo List", annotations.Title)
	require.False(t, annotations.ReadOnlyHint)
	require.False(t, annotations.DestructiveHint)
}

func TestTodoWriteTool_Concurrency(t *testing.T) {
	ctx := context.Background()
	tool := NewTodoWriteTool()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			input := &TodoWriteInput{
				Todos: []TodoItem{
					{Content: "Task", Status: TodoStatusPending, ActiveForm: "Working"},
				},
			}
			_, err := tool.Call(ctx, input)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()
}
