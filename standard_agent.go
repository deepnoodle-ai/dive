package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/getstingrai/agents/llm"
	"github.com/getstingrai/agents/prompt"
)

var _ Agent = &StandardAgent{}

// Define message types
type messageEvent struct {
	event *Event
}

type messageWork struct {
	task    *Task
	promise *Promise
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

type StandardAgentSpec struct {
	Name  string
	Role  *Role
	Goals []*Goal
	LLM   llm.LLM
}

type StandardAgent struct {
	name    string
	role    *Role
	goals   []*Goal
	llm     llm.LLM
	team    *Team
	running bool

	// Consolidate all message types into a single channel
	mailbox chan interface{}

	mu sync.Mutex
	wg sync.WaitGroup
}

func NewStandardAgent(spec StandardAgentSpec) *StandardAgent {
	return &StandardAgent{
		name:    spec.Name,
		role:    spec.Role,
		goals:   spec.Goals,
		llm:     spec.LLM,
		mailbox: make(chan interface{}, 32),
	}
}

func (a *StandardAgent) Name() string {
	return a.name
}

func (a *StandardAgent) Role() *Role {
	return a.role
}

func (a *StandardAgent) Goals() []*Goal {
	return a.goals
}

func (a *StandardAgent) Join(ctx context.Context, team *Team) error {
	a.team = team
	return nil
}

func (a *StandardAgent) Chat(ctx context.Context, message *llm.Message) (*llm.Response, error) {
	result := make(chan *llm.Response, 1)
	errChan := make(chan error, 1)

	select {
	case a.mailbox <- messageChat{
		ctx:     ctx,
		message: message,
		result:  result,
		err:     errChan,
	}:
		select {
		case resp := <-result:
			return resp, nil
		case err := <-errChan:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *StandardAgent) ChatStream(ctx context.Context, message *llm.Message) (llm.Stream, error) {
	return nil, nil
}

func (a *StandardAgent) Event(ctx context.Context, event *Event) error {
	if !a.IsRunning() {
		return fmt.Errorf("agent is not running")
	}

	select {
	case a.mailbox <- messageEvent{event: event}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *StandardAgent) Work(ctx context.Context, task *Task) (*Promise, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	promise := &Promise{agent: a, ch: make(chan *TaskResult, 1)}
	item := messageWork{task: task, promise: promise}

	select {
	case a.mailbox <- item:
		return promise, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *StandardAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("agent is already running")
	}

	a.running = true
	a.wg.Add(1)
	go a.run()
	return nil
}

func (a *StandardAgent) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}
	done := make(chan error)

	// Send stop message before closing mailbox
	a.mailbox <- messageStop{
		ctx:  ctx,
		done: done,
	}

	// Close mailbox after sending stop message
	close(a.mailbox)
	a.running = false

	select {
	case err := <-done:
		a.wg.Wait()
		return err
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for agent to stop: %w", ctx.Err())
	}
}

func (a *StandardAgent) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *StandardAgent) run() error {
	defer a.wg.Done()
	for {
		select {
		case msg := <-a.mailbox:
			switch m := msg.(type) {
			case messageEvent:
				a.handleEvent(m.event)
			case messageWork:
				a.handleWork(m)
			case messageChat:
				a.handleChat(m)
			case messageStop:
				return a.handleStop(m)
			}
		}
	}
}

func (a *StandardAgent) handleEvent(event *Event) {
	fmt.Printf("event: %+v\n", event)
}

func (a *StandardAgent) handleWork(msg messageWork) {
	task := msg.task

	p, err := prompt.New(
		prompt.WithSystemMessage(task.Description()),
		prompt.WithUserMessage(task.ExpectedOutput()),
	).Build(nil)

	if err != nil {
		msg.promise.ch <- &TaskResult{
			Task:  task,
			Error: err,
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*1)
	defer cancel()

	response, err := a.llm.Generate(ctx, p.Messages,
		llm.WithSystemPrompt(p.System))
	if err != nil {
		msg.promise.ch <- &TaskResult{
			Task:  task,
			Error: err,
		}
		return
	}

	responseText := response.Message().Text()

	msg.promise.ch <- &TaskResult{
		Task:   task,
		Output: TaskOutput{Content: responseText},
	}

	fmt.Println("work complete", task.Name(), responseText)
}

func (a *StandardAgent) handleChat(msg messageChat) {
	response, err := a.llm.Generate(msg.ctx, []*llm.Message{msg.message})
	if err != nil {
		msg.err <- err
	} else {
		msg.result <- response
	}
}

func (a *StandardAgent) handleStop(msg messageStop) error {
	// Cleanup logic here
	msg.done <- nil
	return nil
}
