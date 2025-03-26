package agent

import (
	"context"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
)

// messageWork represents a task assignment message sent to an agent
type messageWork struct {
	task      dive.Task
	publisher dive.Publisher
}

// messageChat conveys chat message(s) sent to an agent. The agent will process
// this message and respond through the provided channel or stream.
type messageChat struct {
	messages []*llm.Message
	options  dive.GenerateOptions

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
