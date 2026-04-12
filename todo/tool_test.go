package todo

import (
	"context"
	"sync"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/wonton/assert"
	"github.com/deepnoodle-ai/wonton/schema"
)

func TestTool_Name(t *testing.T) {
	tool := NewTool()
	assert.Equal(t, "TodoWrite", tool.Name())
}

func TestTool_Description(t *testing.T) {
	tool := NewTool()
	desc := tool.Description()
	assert.Contains(t, desc, "task list")
	assert.Contains(t, desc, "pending")
	assert.Contains(t, desc, "in_progress")
	assert.Contains(t, desc, "completed")
}

func TestTool_Schema(t *testing.T) {
	tool := NewTool()
	s := tool.Schema()

	assert.Equal(t, schema.Object, s.Type)
	assert.Contains(t, s.Required, "todos")
	assert.Contains(t, s.Properties, "todos")
}

func TestTool_Call(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateTodos", func(t *testing.T) {
		tool := NewTool()

		input := &WriteInput{
			Todos: []TodoItem{
				{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
				{Content: "Task 2", Status: TodoStatusInProgress, ActiveForm: "Working on Task 2"},
				{Content: "Task 3", Status: TodoStatusCompleted, ActiveForm: "Working on Task 3"},
			},
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		text := result.Content[0].Text
		assert.Contains(t, text, "Todos have been modified successfully")
		assert.Contains(t, text, "1 completed, 1 in progress, 1 pending (3 total)")
	})

	t.Run("EmptyTodos", func(t *testing.T) {
		tool := NewTool()

		input := &WriteInput{Todos: []TodoItem{}}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		text := result.Content[0].Text
		assert.Contains(t, text, "Todos have been modified successfully")
		assert.NotContains(t, text, "Progress:")
	})

	t.Run("AllCompletedRetainsListAndNudges", func(t *testing.T) {
		tool := NewTool()

		input := &WriteInput{
			Todos: []TodoItem{
				{Content: "Task 1", Status: TodoStatusCompleted, ActiveForm: "Working on Task 1"},
				{Content: "Task 2", Status: TodoStatusCompleted, ActiveForm: "Working on Task 2"},
			},
		}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.False(t, result.IsError)

		text := result.Content[0].Text
		assert.Contains(t, text, "All tasks complete")
		assert.Contains(t, text, "2 completed, 0 in progress, 0 pending")
	})

	t.Run("MissingContent", func(t *testing.T) {
		tool := NewTool()
		input := &WriteInput{Todos: []TodoItem{
			{Content: "", Status: TodoStatusPending, ActiveForm: "Working"},
		}}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "content is required")
	})

	t.Run("MissingActiveForm", func(t *testing.T) {
		tool := NewTool()
		input := &WriteInput{Todos: []TodoItem{
			{Content: "Task", Status: TodoStatusPending, ActiveForm: ""},
		}}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "activeForm is required")
	})

	t.Run("InvalidStatus", func(t *testing.T) {
		tool := NewTool()
		input := &WriteInput{Todos: []TodoItem{
			{Content: "Task", Status: "invalid", ActiveForm: "Working"},
		}}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "status must be")
	})

	t.Run("NilTodos", func(t *testing.T) {
		tool := NewTool()
		input := &WriteInput{Todos: nil}

		result, err := tool.Call(ctx, input)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "todos array is required")
	})
}

func TestTool_OnUpdateCallback(t *testing.T) {
	ctx := context.Background()

	var (
		mu            sync.Mutex
		callbackTodos []TodoItem
	)

	tool := NewTool(WithOnUpdate(func(todos []TodoItem) {
		mu.Lock()
		defer mu.Unlock()
		callbackTodos = todos
	}))

	input := &WriteInput{Todos: []TodoItem{
		{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
	}}

	_, err := tool.Call(ctx, input)
	assert.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, callbackTodos, 1)
	assert.Equal(t, "Task 1", callbackTodos[0].Content)
}

func TestTool_PreviewCall(t *testing.T) {
	ctx := context.Background()
	tool := NewTool()

	input := &WriteInput{
		Todos: []TodoItem{
			{Content: "Task 1", Status: TodoStatusPending, ActiveForm: "Working on Task 1"},
			{Content: "Task 2", Status: TodoStatusInProgress, ActiveForm: "Working on Task 2"},
			{Content: "Task 3", Status: TodoStatusCompleted, ActiveForm: "Working on Task 3"},
		},
	}

	preview := tool.PreviewCall(ctx, input)
	assert.Contains(t, preview.Summary, "1 pending")
	assert.Contains(t, preview.Summary, "1 in progress")
	assert.Contains(t, preview.Summary, "1 completed")
}

func TestTool_Annotations(t *testing.T) {
	tool := NewTool()
	annotations := tool.Annotations()

	assert.NotNil(t, annotations)
	assert.Equal(t, "TodoWrite", annotations.Title)
	assert.False(t, annotations.ReadOnlyHint)
	assert.False(t, annotations.DestructiveHint)
}

func TestTool_StatelessAcrossCalls(t *testing.T) {
	// The tool stores no list state — independent calls share no memory.
	// (The model's list lives in conversation history, not on the tool.)
	ctx := context.Background()
	tool := NewTool()

	_, err := tool.Call(ctx, &WriteInput{Todos: []TodoItem{
		{Content: "Setup", Status: TodoStatusInProgress, ActiveForm: "Setting up"},
	}})
	assert.NoError(t, err)

	// A second tool instance must behave identically — there is no
	// process-wide store to bleed across.
	other := NewTool()
	res, err := other.Call(ctx, &WriteInput{Todos: []TodoItem{
		{Content: "Other", Status: TodoStatusPending, ActiveForm: "Doing other"},
	}})
	assert.NoError(t, err)
	assert.False(t, res.IsError)
}

func TestTool_Concurrency(t *testing.T) {
	ctx := context.Background()
	tool := NewTool()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := tool.Call(ctx, &WriteInput{Todos: []TodoItem{
				{Content: "Task", Status: TodoStatusPending, ActiveForm: "Working"},
			}})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

// Compile-time check: Tool implements dive.TypedTool[*WriteInput].
var _ dive.TypedTool[*WriteInput] = (*Tool)(nil)
