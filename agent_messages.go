package dive

import (
	"context"

	"github.com/getstingrai/dive/llm"
)

// messageWork represents a task assignment message sent to an agent
type messageWork struct {
	task      *Task
	publisher *StreamPublisher
}

// messageChat represents a direct chat message sent to an agent
// The agent will process this message immediately and respond through
// the provided channels, without converting it to a task
type messageChat struct {
	message    *llm.Message
	resultChan chan *llm.Response
	errChan    chan error
}

// messageStop represents a request to stop the agent
type messageStop struct {
	ctx  context.Context
	done chan error
}
