package dive

import (
	"context"

	"github.com/getstingrai/dive/llm"
)

type messageWork struct {
	task      *Task
	publisher *StreamPublisher
}

type messageChat struct {
	message    *llm.Message
	resultChan chan *llm.Response
	errChan    chan error
}

type messageStop struct {
	ctx  context.Context
	done chan error
}
