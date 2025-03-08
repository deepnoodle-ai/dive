package agent

import (
	"context"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/llm"
)

// messageWork represents a task assignment message sent to an agent
type messageWork struct {
	task      dive.Task
	publisher dive.Publisher
}

// messageChat represents a direct chat message sent to an agent
// The agent will process this message immediately and respond through
// the provided channels, without converting it to a task
type messageChat struct {
	message *llm.Message
	options dive.GenerateOptions

	// For synchronous responses:
	resultChan chan *llm.Response
	errChan    chan error

	// For streaming responses:
	stream dive.Stream
}

// messageStop represents a request to stop the agent
type messageStop struct {
	ctx  context.Context
	done chan error
}
