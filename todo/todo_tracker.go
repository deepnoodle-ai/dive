package todo

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/deepnoodle-ai/dive"
)

// TodoTracker provides real-time tracking and display of todo list progress.
//
// TodoTracker can be used as an event callback to monitor todo updates during
// agent execution. It maintains the current state of the todo list and provides
// methods for displaying progress.
type TodoTracker struct {
	mu    sync.RWMutex
	todos []TodoItem
}

// NewTodoTracker creates a new TodoTracker.
func NewTodoTracker() *TodoTracker {
	return &TodoTracker{}
}

// HandleEvent is a dive.EventCallback that tracks todo updates.
// Use this as the callback passed to dive.WithEventCallback.
func (t *TodoTracker) HandleEvent(ctx context.Context, item *dive.ResponseItem) error {
	if item.Type == ItemType {
		if evt, ok := item.Extension.(*TodoEvent); ok && evt != nil {
			t.mu.Lock()
			t.todos = make([]TodoItem, len(evt.Todos))
			copy(t.todos, evt.Todos)
			t.mu.Unlock()
		}
	}
	return nil
}

// Todos returns a copy of the current todo list.
func (t *TodoTracker) Todos() []TodoItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]TodoItem, len(t.todos))
	copy(result, t.todos)
	return result
}

// CurrentTask returns the currently in-progress task, if any.
func (t *TodoTracker) CurrentTask() *TodoItem {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, todo := range t.todos {
		if todo.Status == TodoStatusInProgress {
			todoCopy := todo
			return &todoCopy
		}
	}
	return nil
}

// Progress returns the count of completed, in-progress, and total tasks.
func (t *TodoTracker) Progress() (completed, inProgress, total int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	total = len(t.todos)
	for _, todo := range t.todos {
		switch todo.Status {
		case TodoStatusCompleted:
			completed++
		case TodoStatusInProgress:
			inProgress++
		}
	}
	return
}

// DisplayProgress writes a formatted progress display to the given writer.
func (t *TodoTracker) DisplayProgress(w io.Writer) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.todos) == 0 {
		return
	}

	completed, inProgress, total := 0, 0, len(t.todos)
	for _, todo := range t.todos {
		switch todo.Status {
		case TodoStatusCompleted:
			completed++
		case TodoStatusInProgress:
			inProgress++
		}
	}

	fmt.Fprintf(w, "\nProgress: %d/%d completed\n", completed, total)
	fmt.Fprintf(w, "Currently working on: %d task(s)\n\n", inProgress)

	for i, todo := range t.todos {
		icon := "❌"
		text := todo.Content
		if todo.Status == TodoStatusCompleted {
			icon = "✅"
		} else if todo.Status == TodoStatusInProgress {
			icon = "🔧"
			text = todo.ActiveForm
		}
		fmt.Fprintf(w, "%d. %s %s\n", i+1, icon, text)
	}
}

// FormatProgress returns a formatted string of the current progress.
func (t *TodoTracker) FormatProgress() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.todos) == 0 {
		return ""
	}

	completed, inProgress := 0, 0
	var currentTask string
	for _, todo := range t.todos {
		switch todo.Status {
		case TodoStatusCompleted:
			completed++
		case TodoStatusInProgress:
			inProgress++
			if currentTask == "" {
				currentTask = todo.ActiveForm
			}
		}
	}

	progress := fmt.Sprintf("%d/%d", completed, len(t.todos))
	if currentTask != "" {
		return fmt.Sprintf("%s • %s", currentTask, progress)
	}
	return progress
}

// ChainCallback returns a dive.EventCallback that calls the tracker's HandleEvent
// and then calls the provided callback. This allows chaining multiple handlers.
func (t *TodoTracker) ChainCallback(next dive.EventCallback) dive.EventCallback {
	return func(ctx context.Context, item *dive.ResponseItem) error {
		if err := t.HandleEvent(ctx, item); err != nil {
			return err
		}
		if next != nil {
			return next(ctx, item)
		}
		return nil
	}
}
