package dive

import (
	"context"

	"github.com/getstingrai/dive/llm"
)

// Define message types
type messageEvent struct {
	event *Event
}

type messageWork struct {
	task      *Task
	stream    Stream
	publisher *StreamPublisher
}

type messageChat struct {
	ctx     context.Context
	message *llm.Message
	result  chan *llm.Response
	err     chan error
}

type messageStop struct {
	ctx  context.Context
	done chan error
}
