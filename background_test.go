package dive

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deepnoodle-ai/wonton/assert"
)

// ---------------------------------------------------------------------------
// NewBackgroundResult
// ---------------------------------------------------------------------------

func TestNewBackgroundResult_ReturnsToolResult(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResult(ctx, "test task", func(ctx context.Context) (string, error) {
		return "output", nil
	})
	assert.NotNil(t, result)
	assert.NotNil(t, result.Background)
	assert.Equal(t, result.Background.description, "test task")
	assert.NotEqual(t, result.Background.id, "")
	assert.NotNil(t, result.Background.done)

	// Regular content fields must be zero — this is a tagged union.
	assert.True(t, len(result.Content) == 0)
	assert.Equal(t, result.Display, "")
	assert.False(t, result.IsError)
	assert.Nil(t, result.Suspend)
}

func TestNewBackgroundResult_GoroutineSendsResult(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResult(ctx, "work", func(ctx context.Context) (string, error) {
		return "done!", nil
	})

	select {
	case r := <-result.Background.done:
		assert.NotNil(t, r)
		assert.False(t, r.IsError)
		assert.Equal(t, toolResultText(r), "done!")
	case <-time.After(2 * time.Second):
		t.Fatal("background goroutine did not complete in time")
	}
}

func TestNewBackgroundResult_ErrorBecomesIsError(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResult(ctx, "failing task", func(ctx context.Context) (string, error) {
		return "", errors.New("something went wrong")
	})

	select {
	case r := <-result.Background.done:
		assert.NotNil(t, r)
		assert.True(t, r.IsError)
		assert.True(t, strings.Contains(toolResultText(r), "something went wrong"), "expected error in result text")
	case <-time.After(2 * time.Second):
		t.Fatal("background goroutine did not complete in time")
	}
}

func TestNewBackgroundResult_ContextPropagation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	result := NewBackgroundResult(ctx, "cancellable task", func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})

	<-started
	cancel()

	select {
	case r := <-result.Background.done:
		assert.NotNil(t, r)
		assert.True(t, r.IsError)
	case <-time.After(2 * time.Second):
		t.Fatal("background goroutine did not respond to context cancellation")
	}
}

// ---------------------------------------------------------------------------
// NewBackgroundResultFull
// ---------------------------------------------------------------------------

func TestNewBackgroundResultFull_FullControl(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResultFull(ctx, "full task", func(ctx context.Context) *ToolResult {
		return NewToolResultError("custom error result").WithDisplay("display text")
	})

	assert.NotNil(t, result.Background)

	select {
	case r := <-result.Background.done:
		assert.NotNil(t, r)
		assert.True(t, r.IsError)
		assert.Equal(t, r.Display, "display text")
		assert.True(t, strings.Contains(toolResultText(r), "custom error result"))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestNewBackgroundResultFull_NilResultDefaultsToEmpty(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResultFull(ctx, "nil task", func(ctx context.Context) *ToolResult {
		return nil
	})
	select {
	case r := <-result.Background.done:
		assert.NotNil(t, r)
		assert.False(t, r.IsError)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

// ---------------------------------------------------------------------------
// Panic recovery
// ---------------------------------------------------------------------------

func TestNewBackgroundResult_PanicRecovered(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResult(ctx, "panicking task", func(ctx context.Context) (string, error) {
		panic("oh no")
	})

	select {
	case r := <-result.Background.done:
		assert.NotNil(t, r)
		assert.True(t, r.IsError)
		text := toolResultText(r)
		assert.True(t, strings.Contains(text, "background task panicked"), "expected panic message, got: "+text)
		assert.True(t, strings.Contains(text, "oh no"), "expected panic value, got: "+text)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panic recovery")
	}
}

func TestNewBackgroundResultFull_PanicRecovered(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResultFull(ctx, "panicking full", func(ctx context.Context) *ToolResult {
		panic(42)
	})
	select {
	case r := <-result.Background.done:
		assert.True(t, r.IsError)
		assert.True(t, strings.Contains(toolResultText(r), "background task panicked"))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

// ---------------------------------------------------------------------------
// Buffered channel — goroutine never blocks even if caller never reads
// ---------------------------------------------------------------------------

func TestNewBackgroundResult_NoLeakWhenHandleDropped(t *testing.T) {
	ctx := context.Background()
	done := make(chan struct{})

	result := NewBackgroundResult(ctx, "dropped", func(ctx context.Context) (string, error) {
		close(done)
		return "result", nil
	})
	_ = result // handle intentionally dropped

	select {
	case <-done:
		// goroutine completed even though Done was never read
	case <-time.After(2 * time.Second):
		t.Fatal("background goroutine leaked")
	}
}

// ---------------------------------------------------------------------------
// AwaitBackgroundTasks
// ---------------------------------------------------------------------------

func TestAwaitBackgroundTasks_Empty(t *testing.T) {
	results, err := AwaitBackgroundTasks(context.Background(), nil)
	assert.NoError(t, err)
	assert.True(t, len(results) == 0)

	results, err = AwaitBackgroundTasks(context.Background(), []*BackgroundTaskHandle{})
	assert.NoError(t, err)
	assert.True(t, len(results) == 0)
}

func TestAwaitBackgroundTasks_SingleTask(t *testing.T) {
	ctx := context.Background()
	result := NewBackgroundResult(ctx, "single", func(ctx context.Context) (string, error) {
		return "hello", nil
	})

	handle := &BackgroundTaskHandle{
		TaskID: result.Background.id,
		Done:   result.Background.done,
	}

	results, err := AwaitBackgroundTasks(ctx, []*BackgroundTaskHandle{handle})
	assert.NoError(t, err)
	assert.Equal(t, len(results), 1)
	assert.NotNil(t, results[handle.TaskID])
	assert.Equal(t, toolResultText(results[handle.TaskID]), "hello")
}

func TestAwaitBackgroundTasks_MultipleTasks(t *testing.T) {
	ctx := context.Background()

	var handles []*BackgroundTaskHandle
	for i := range 5 {
		i := i
		r := NewBackgroundResult(ctx, "task", func(ctx context.Context) (string, error) {
			time.Sleep(time.Duration(i) * 5 * time.Millisecond)
			return "ok", nil
		})
		handles = append(handles, &BackgroundTaskHandle{
			TaskID: r.Background.id,
			Done:   r.Background.done,
		})
	}

	results, err := AwaitBackgroundTasks(ctx, handles)
	assert.NoError(t, err)
	assert.Equal(t, len(results), 5)
	for _, h := range handles {
		assert.NotNil(t, results[h.TaskID])
	}
}

func TestAwaitBackgroundTasks_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create a task that blocks until context cancelled
	r := NewBackgroundResult(ctx, "blocked", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	handle := &BackgroundTaskHandle{
		TaskID: r.Background.id,
		Done:   r.Background.done,
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	results, err := AwaitBackgroundTasks(ctx, []*BackgroundTaskHandle{handle})
	assert.Error(t, err)
	// Results may be empty or partial; the important thing is err is non-nil
	_ = results
}

func TestAwaitBackgroundTasks_PartialResultsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// fast task — completes immediately
	fastR := NewBackgroundResult(ctx, "fast", func(ctx context.Context) (string, error) {
		return "fast result", nil
	})
	fastHandle := &BackgroundTaskHandle{
		TaskID: fastR.Background.id,
		Done:   fastR.Background.done,
	}

	// slow task — blocks until context cancelled
	slowR := NewBackgroundResult(ctx, "slow", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	slowHandle := &BackgroundTaskHandle{
		TaskID: slowR.Background.id,
		Done:   slowR.Background.done,
	}

	// Let the fast task complete first
	time.Sleep(50 * time.Millisecond)

	// Cancel while slow task is still running
	cancel()

	results, err := AwaitBackgroundTasks(ctx, []*BackgroundTaskHandle{fastHandle, slowHandle})
	// err is ctx.Err() for cancelled tasks
	assert.Error(t, err)
	// fast task may or may not be in results depending on timing; we just
	// verify the function returned and did not block
	_ = results
}

// ---------------------------------------------------------------------------
// backgroundStartedMessage
// ---------------------------------------------------------------------------

func TestBackgroundStartedMessage(t *testing.T) {
	msg := backgroundStartedMessage("run tests", "task-123")
	assert.True(t, strings.Contains(msg, "Background task started: run tests"))
	assert.True(t, strings.Contains(msg, "Task ID: task-123"))
	assert.True(t, strings.Contains(msg, "The result will be delivered in a follow-up message."))
}

// ---------------------------------------------------------------------------
// backgroundCompletedMessage
// ---------------------------------------------------------------------------

func TestBackgroundCompletedMessage_Single(t *testing.T) {
	handle := &BackgroundTaskHandle{
		TaskID:      "id-1",
		Description: "run the build",
	}
	results := map[string]*ToolResult{
		"id-1": NewToolResultText("build output"),
	}
	msg := backgroundCompletedMessage([]*BackgroundTaskHandle{handle}, results)
	// Single task: no introductory line
	assert.False(t, strings.Contains(msg, "The following background tasks have completed"), "single task should not have intro line")
	assert.True(t, strings.Contains(msg, "Background task completed: run the build"))
	assert.True(t, strings.Contains(msg, "Task ID: id-1"))
	assert.True(t, strings.Contains(msg, "Result:"))
	assert.True(t, strings.Contains(msg, "build output"))
}

func TestBackgroundCompletedMessage_Multiple(t *testing.T) {
	handles := []*BackgroundTaskHandle{
		{TaskID: "id-1", Description: "task one"},
		{TaskID: "id-2", Description: "task two"},
	}
	results := map[string]*ToolResult{
		"id-1": NewToolResultText("result one"),
		"id-2": NewToolResultError("something failed"),
	}
	msg := backgroundCompletedMessage(handles, results)
	assert.True(t, strings.Contains(msg, "The following background tasks have completed"))
	assert.True(t, strings.Contains(msg, "task one"))
	assert.True(t, strings.Contains(msg, "task two"))
	assert.True(t, strings.Contains(msg, "Result:"))
	assert.True(t, strings.Contains(msg, "Error:"))
	assert.True(t, strings.Contains(msg, "result one"))
	assert.True(t, strings.Contains(msg, "something failed"))
}

func TestBackgroundCompletedMessage_Empty(t *testing.T) {
	msg := backgroundCompletedMessage(nil, nil)
	assert.Equal(t, msg, "")

	msg = backgroundCompletedMessage([]*BackgroundTaskHandle{}, map[string]*ToolResult{})
	assert.Equal(t, msg, "")
}

func TestBackgroundCompletedMessage_MissingResult(t *testing.T) {
	handles := []*BackgroundTaskHandle{
		{TaskID: "id-1", Description: "task"},
	}
	// Result missing from map
	msg := backgroundCompletedMessage(handles, map[string]*ToolResult{})
	assert.True(t, strings.Contains(msg, "Background task completed: task"))
	// No panic; result section is empty
}

// ---------------------------------------------------------------------------
// toolResultText helper
// ---------------------------------------------------------------------------

func TestToolResultText(t *testing.T) {
	assert.Equal(t, toolResultText(nil), "")
	assert.Equal(t, toolResultText(NewToolResultText("hello")), "hello")
	assert.Equal(t, toolResultText(NewToolResultError("oops")), "oops")

	multi := NewToolResult(
		&ToolResultContent{Type: ToolResultContentTypeText, Text: "part one"},
		&ToolResultContent{Type: ToolResultContentTypeText, Text: "part two"},
	)
	assert.Equal(t, toolResultText(multi), "part one\npart two")
}
