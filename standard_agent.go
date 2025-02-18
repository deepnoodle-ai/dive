package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/getstingrai/agents/llm"
)

var _ Agent = &StandardAgent{}

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

	eventChan chan *Event
	workChan  chan workItem
	stopChan  chan struct{}
	mu        sync.Mutex
	wg        sync.WaitGroup

	currentWork     *workItem
	currentWorkDone chan struct{}
}

type workItem struct {
	task    *Task
	promise *Promise
}

func NewStandardAgent(spec StandardAgentSpec) *StandardAgent {
	return &StandardAgent{
		name:            spec.Name,
		role:            spec.Role,
		goals:           spec.Goals,
		llm:             spec.LLM,
		eventChan:       make(chan *Event, 32),
		workChan:        make(chan workItem, 32),
		stopChan:        make(chan struct{}),
		currentWorkDone: make(chan struct{}),
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
	response, err := a.llm.Generate(ctx, []*llm.Message{message})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (a *StandardAgent) ChatStream(ctx context.Context, message *llm.Message) (llm.Stream, error) {
	return nil, nil
}

func (a *StandardAgent) Event(ctx context.Context, event *Event) error {
	if !a.IsRunning() {
		return fmt.Errorf("agent is not running")
	}
	select {
	case a.eventChan <- event:
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
	item := workItem{task: task, promise: promise}

	select {
	case a.workChan <- item:
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

	a.running = false
	close(a.stopChan)

	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
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
		case event := <-a.eventChan:
			fmt.Printf("event: %+v\n", event)

		case work := <-a.workChan:
			go func() {
				result := a.processWork(work.task)
				work.promise.ch <- result
				close(work.promise.ch)
			}()

		case <-a.stopChan:
			return nil
		}
	}
}

func (a *StandardAgent) processWork(task *Task) *TaskResult {
	return nil
}
