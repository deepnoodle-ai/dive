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
	Name      string
	Role      *Role
	Goals     []*Goal
	LLM       llm.LLM
	Tools     []llm.Tool
	IsManager bool
	IsWorker  bool
}

type StandardAgent struct {
	name      string
	role      *Role
	goals     []*Goal
	llm       llm.LLM
	team      *Team
	running   bool
	tools     []llm.Tool
	isManager bool
	isWorker  bool

	// Consolidate all message types into a single channel
	mailbox chan interface{}

	mu sync.Mutex
	wg sync.WaitGroup
}

func NewStandardAgent(spec StandardAgentSpec) *StandardAgent {
	return &StandardAgent{
		name:      spec.Name,
		role:      spec.Role,
		goals:     spec.Goals,
		llm:       spec.LLM,
		tools:     spec.Tools,
		isManager: spec.IsManager,
		isWorker:  spec.IsWorker,
		mailbox:   make(chan interface{}, 32),
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
				result, err := a.handleTask(m.task)
				if err != nil {
					m.promise.ch <- NewTaskResultError(m.task, err)
				} else {
					m.promise.ch <- result
				}
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

func (a *StandardAgent) getTools() []llm.Tool {
	results := []llm.Tool{}
	for _, tool := range a.tools {
		results = append(results, tool)
	}
	if a.isManager {
		// results = append(results, a.team.Tools()...)
	}
	return results
}

func (a *StandardAgent) systemPrompt() (string, error) {
	return ExecuteTemplate(agentSystemPromptTemplate, a.TemplateData())
}

func (a *StandardAgent) handleTask(task *Task) (*TaskResult, error) {
	timeout := task.Timeout()
	if timeout == 0 {
		timeout = time.Minute * 3
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	systemPrompt, err := a.systemPrompt()
	if err != nil {
		return nil, err
	}

	fmt.Println("==== systemPrompt ====")
	fmt.Println(systemPrompt)
	fmt.Println("==== /systemPrompt ====")

	p, err := prompt.New(
		prompt.WithSystemMessage(systemPrompt),
		prompt.WithUserMessage(task.PromptText()),
	).Build()

	if err != nil {
		return nil, err
	}

	response, err := a.llm.Generate(ctx,
		p.Messages,
		llm.WithSystemPrompt(p.System),
		llm.WithTools(a.getTools()...),
	)
	if err != nil {
		return nil, err
	}

	responseText := response.Message().Text()

	fmt.Println("work complete", task.Name(), responseText)

	return &TaskResult{
		Task:   task,
		Output: TaskOutput{Content: responseText},
	}, nil
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

func NewTaskResultError(task *Task, err error) *TaskResult {
	return &TaskResult{
		Task:  task,
		Error: err,
	}
}

type AgentTemplateData struct {
	Name      string
	Role      string
	Goals     []*Goal
	Team      *Team
	IsManager bool
	IsWorker  bool
}

func (a *StandardAgent) TemplateData() *AgentTemplateData {
	return &AgentTemplateData{
		Name:      a.name,
		Role:      a.role.Description,
		Goals:     a.goals,
		Team:      a.team,
		IsManager: a.isManager,
		IsWorker:  a.isWorker,
	}
}
