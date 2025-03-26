package agent

import (
	"time"

	"github.com/diveagents/dive"
)

var _ dive.Task = &Task{}

type TaskOptions struct {
	Name    string
	Timeout time.Duration
	Prompt  *dive.Prompt
}

type Task struct {
	name    string
	timeout time.Duration
	prompt  *dive.Prompt
}

func (t *Task) Name() string {
	return t.name
}

func (t *Task) Timeout() time.Duration {
	return t.timeout
}

func (t *Task) Prompt() (*dive.Prompt, error) {
	return t.prompt, nil
}

func NewTask(opts TaskOptions) *Task {
	return &Task{
		name:    opts.Name,
		timeout: opts.Timeout,
		prompt:  opts.Prompt,
	}
}
