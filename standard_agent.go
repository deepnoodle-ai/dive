package agents

import (
	"context"
	"fmt"
	"regexp"
	"strings"
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

type TaskState struct {
	task           *Task
	promise        *Promise
	status         TaskStatus
	priority       int
	started        time.Time
	output         string
	reasoning      string
	reportedStatus string
	messages       []*llm.Message
	suspended      bool
	chatResult     chan *llm.Response
	chatError      chan error
}

func (s *TaskState) Task() *Task {
	return s.task
}

func (s *TaskState) Output() string {
	return s.output
}

func (s *TaskState) Reasoning() string {
	return s.reasoning
}

func (s *TaskState) Status() TaskStatus {
	return s.status
}

func (s *TaskState) ReportedStatus() string {
	return s.reportedStatus
}

func (s *TaskState) Messages() []*llm.Message {
	return s.messages
}

func (s *TaskState) String() string {
	text, err := ExecuteTemplate(taskStatePromptTemplate, s)
	if err != nil {
		panic(err)
	}
	return text
}

type Document struct {
	Name    string
	Content string
	Format  OutputFormat
}

type StandardAgentSpec struct {
	Name           string
	Role           *Role
	Goals          []*Goal
	LLM            llm.LLM
	Tools          []llm.Tool
	IsManager      bool
	IsWorker       bool
	MaxActiveTasks int
	TickFrequency  time.Duration
	CacheControl   string
}

type StandardAgent struct {
	name           string
	role           *Role
	goals          []*Goal
	llm            llm.LLM
	team           *Team
	running        bool
	tools          []llm.Tool
	toolsByName    map[string]llm.Tool
	isManager      bool
	isWorker       bool
	maxActiveTasks int
	tickFrequency  time.Duration
	cacheControl   string
	taskQueue      []*TaskState
	activeTask     *TaskState
	workspace      []*Document
	ticker         *time.Ticker
	completedTasks []*TaskState

	// Consolidate all message types into a single channel
	mailbox chan interface{}

	mu sync.Mutex
	wg sync.WaitGroup
}

func NewStandardAgent(spec StandardAgentSpec) *StandardAgent {
	if spec.MaxActiveTasks == 0 {
		spec.MaxActiveTasks = 1
	}
	if spec.TickFrequency == 0 {
		spec.TickFrequency = time.Millisecond * 250
	}
	a := &StandardAgent{
		name:           spec.Name,
		role:           spec.Role,
		goals:          spec.Goals,
		llm:            spec.LLM,
		tools:          spec.Tools,
		isManager:      spec.IsManager,
		isWorker:       spec.IsWorker,
		maxActiveTasks: spec.MaxActiveTasks,
		tickFrequency:  spec.TickFrequency,
		cacheControl:   spec.CacheControl,
		mailbox:        make(chan interface{}, 64),
		toolsByName:    make(map[string]llm.Tool),
	}
	for _, tool := range spec.Tools {
		a.toolsByName[tool.Definition().Name] = tool
	}
	return a
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

	select {
	case a.mailbox <- messageWork{
		task:    task,
		promise: promise,
	}:
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
	a.ticker = time.NewTicker(a.tickFrequency)
	defer a.ticker.Stop()

	for {
		select {
		case <-a.ticker.C:
			a.processTaskQueue()
		case msg := <-a.mailbox:
			switch m := msg.(type) {
			case messageEvent:
				a.handleEvent(m.event)
			case messageWork:
				taskState := &TaskState{
					task:     m.task,
					promise:  m.promise,
					status:   TaskStatusQueued,
					priority: m.task.Priority(),
				}
				// Insert into queue maintaining priority order
				inserted := false
				for i, existing := range a.taskQueue {
					if taskState.priority > existing.priority {
						a.taskQueue = append(a.taskQueue[:i], append([]*TaskState{taskState}, a.taskQueue[i:]...)...)
						inserted = true
						break
					}
				}
				if !inserted {
					a.taskQueue = append(a.taskQueue, taskState)
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

func parseStructuredResponse(responseText string) (string, string, string) {
	var response, thinking, reportedStatus string

	// Split on <think> tag
	if strings.Contains(responseText, "<think>") {
		parts := strings.Split(responseText, "<think>")
		if len(parts) > 1 {
			// Find the end of think section
			thinkParts := strings.Split(parts[1], "</think>")
			if len(thinkParts) > 1 {
				thinking = strings.TrimSpace(thinkParts[0])
				response = strings.TrimSpace(thinkParts[1])
			}
		}
	} else {
		response = responseText
	}

	// Extract status if present
	if strings.Contains(response, "<status>") {
		parts := strings.Split(response, "<status>")
		if len(parts) > 1 {
			statusParts := strings.Split(parts[1], "</status>")
			if len(statusParts) > 1 {
				reportedStatus = strings.TrimSpace(statusParts[0])
				response = strings.TrimSpace(parts[0])
			}
		}
	}

	return response, thinking, reportedStatus
}

func (a *StandardAgent) handleTask(state *TaskState) error {
	task := state.task
	timeout := task.Timeout()
	if timeout == 0 {
		timeout = time.Minute * 3
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	systemPrompt, err := a.systemPrompt()
	if err != nil {
		return err
	}

	messages := []*llm.Message{}

	if len(state.Messages()) == 0 {
		if len(a.completedTasks) > 0 {
			messages = append(messages, llm.NewUserMessage(
				fmt.Sprintf("For reference, here is an overview of other recent tasks we completed:\n\n%s", a.getTaskHistory())))
		}
		messages = append(messages, llm.NewUserMessage(task.PromptText()))
	} else {
		messages = append(messages, state.Messages()...)
		messages = append(messages, llm.NewUserMessage("Continue working on the task."))
	}

	var response *llm.Response

	for i := 0; i < 3; i++ {
		p, err := prompt.New(
			prompt.WithSystemMessage(systemPrompt),
			prompt.WithMessage(messages...),
		).Build()
		if err != nil {
			return err
		}
		response, err = a.llm.Generate(ctx,
			p.Messages,
			llm.WithSystemPrompt(p.System),
			llm.WithTools(a.getTools()...),
			llm.WithCacheControl(a.cacheControl),
		)
		if err != nil {
			return err
		}
		messages = append(messages, response.Message())
		if len(response.ToolCalls()) > 0 {
			var toolResults []*llm.ToolResult
			for _, toolCall := range response.ToolCalls() {
				tool, ok := a.toolsByName[toolCall.Name]
				if !ok {
					return fmt.Errorf("tool not found: %s", toolCall.Name)
				} else {
					result, err := tool.Call(ctx, toolCall.Input)
					if err != nil {
						return fmt.Errorf("tool error: %w", err)
					} else {
						toolResults = append(toolResults, &llm.ToolResult{
							ID:     toolCall.ID,
							Result: result,
						})
					}
				}
			}
			if len(toolResults) > 0 {
				messages = append(messages, llm.NewToolResultMessage(toolResults))
			}
		} else {
			break
		}
	}

	state.messages = messages
	finalOutput, thinking, reportedStatus := parseStructuredResponse(response.Message().Text())
	if state.output == "" {
		state.output = finalOutput
	} else {
		state.output = fmt.Sprintf("%s\n\n%s", state.output, finalOutput)
	}
	state.reasoning = thinking
	state.reportedStatus = reportedStatus
	return nil
}

// Add new constructor for chat tasks
func NewChatTask(message *llm.Message) *Task {
	return &Task{
		name:        "",
		kind:        "chat",
		description: fmt.Sprintf("Generate a response to user message: %q", message.Text()),
		// promptText:  message.Text(),
		priority: 10, // High priority for chat responses
		timeout:  time.Minute * 1,
	}
}

func (a *StandardAgent) handleChat(m messageChat) {
	// Create a task from the chat message with the response channels
	task := NewChatTask(m.message)

	// Create task state and add to queue with high priority
	taskState := &TaskState{
		task:       task,
		promise:    &Promise{agent: a, ch: make(chan *TaskResult, 1)},
		status:     TaskStatusQueued,
		priority:   task.Priority(),
		messages:   []*llm.Message{m.message},
		chatResult: m.result,
		chatError:  m.err,
	}

	// Insert into queue maintaining priority order
	inserted := false
	for i, existing := range a.taskQueue {
		if taskState.priority > existing.priority {
			a.taskQueue = append(a.taskQueue[:i], append([]*TaskState{taskState}, a.taskQueue[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		a.taskQueue = append(a.taskQueue, taskState)
	}
}

func (a *StandardAgent) handleStop(msg messageStop) error {
	// Cleanup logic here
	msg.done <- nil
	return nil
}

func (a *StandardAgent) processTaskQueue() {
	// Check if we should preempt current task
	if a.activeTask != nil && len(a.taskQueue) > 0 {
		nextTask := a.taskQueue[0]
		if nextTask.priority > a.activeTask.priority {
			// Suspend current task and move it back to queue
			a.activeTask.suspended = true
			a.activeTask.status = TaskStatusQueued
			// Insert suspended task back into queue maintaining priority order
			inserted := false
			for i, existing := range a.taskQueue {
				if a.activeTask.priority > existing.priority {
					a.taskQueue = append(a.taskQueue[:i], append([]*TaskState{a.activeTask}, a.taskQueue[i:]...)...)
					inserted = true
					break
				}
			}
			if !inserted {
				a.taskQueue = append(a.taskQueue, a.activeTask)
			}
			a.activeTask = nil
		}
	}

	// If no active task and queue not empty, activate next task
	if a.activeTask == nil && len(a.taskQueue) > 0 {
		a.activeTask = a.taskQueue[0]
		a.taskQueue = a.taskQueue[1:]
		a.activeTask.status = TaskStatusActive
		if !a.activeTask.suspended {
			// Only set started time if this is a new task, not a resumed one
			a.activeTask.started = time.Now()
		}
		a.activeTask.suspended = false
		fmt.Printf("activated task: %s (priority: %d)\n", a.activeTask.task.Name(), a.activeTask.priority)
	}

	if a.activeTask != nil {
		err := a.handleTask(a.activeTask)
		if err != nil {
			fmt.Println("task error:", a.activeTask.task.Name(), err)
			a.activeTask.status = TaskStatusError
			a.rememberTask(a.activeTask)
			a.activeTask.promise.ch <- NewTaskResultError(a.activeTask.task, err)
			a.activeTask = nil
			return
		}
		reportedStatus := strings.ToLower(a.activeTask.reportedStatus)
		isComplete := !strings.Contains(reportedStatus, "incomplete")
		if isComplete {
			fmt.Println("task completed:", a.activeTask.task.Name())
			a.activeTask.status = TaskStatusCompleted
			a.rememberTask(a.activeTask)
			// Handle chat task resolution
			if a.activeTask.task.Kind() == "chat" {
				a.activeTask.chatResult <- llm.NewResponse(llm.ResponseOptions{
					Role:    llm.Assistant,
					Message: llm.NewAssistantMessage(a.activeTask.output),
				})
			}
			if a.activeTask.promise != nil {
				a.activeTask.promise.ch <- &TaskResult{
					Task:   a.activeTask.task,
					Output: TaskOutput{Content: a.activeTask.output},
				}
			}
			a.activeTask = nil
		} else {
			fmt.Println("task not yet complete:", a.activeTask.task.Name())
		}
	}
}

func (a *StandardAgent) rememberTask(task *TaskState) {
	a.completedTasks = append(a.completedTasks, task)
	if len(a.completedTasks) > 10 {
		a.completedTasks = a.completedTasks[1:]
	}
}

func (a *StandardAgent) getTaskHistory() string {
	var history []string
	for _, status := range a.completedTasks {
		title := status.task.Name()
		if title == "" {
			title = status.task.Description()
		}
		title = TruncateText(title, 8)
		output := replaceNewlines(status.output)
		history = append(history, fmt.Sprintf("- task: %q status: %q output: %q\n",
			title, status.status, TruncateText(output, 8),
		))
	}
	result := strings.Join(history, "\n")
	if len(result) > 200 {
		result = result[:200]
	}
	// fmt.Println("==== task history ====")
	// fmt.Println(result)
	// fmt.Println("==== /task history ====")
	return result
}

func (a *StandardAgent) getWorkspaceState() string {
	var blobs []string
	for _, doc := range a.workspace {
		blobs = append(blobs, fmt.Sprintf("<document name=%q>\n%s\n</document>", doc.Name, doc.Content))
	}
	return strings.Join(blobs, "\n\n")
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

// func (a *StandardAgent) checkTaskCompletion(ctx context.Context, taskState *TaskState) (bool, error) {
// 	response, err := prompt.Execute(ctx, a.llm,
// 		prompt.WithSystemMessage(`You are evaluating if a task has been completed successfully. Review the original task and its output. Respond with exactly "complete" if the task was completed successfully, or "incomplete" if it needs more work.`),
// 		prompt.WithUserMessage(taskState.String()))
// 	if err != nil {
// 		return false, err
// 	}
// 	text := strings.TrimSpace(strings.ToLower(response.Message().Text()))
// 	fmt.Println("==== checkTaskCompletion ====")
// 	fmt.Println(text)
// 	fmt.Println(text == "complete")
// 	fmt.Println("==== /checkTaskCompletion ====")
// 	return text == "complete", nil
// }

func TruncateText(text string, maxWords int) string {
	// Split into lines while preserving newlines
	lines := strings.Split(text, "\n")
	wordCount := 0
	var result []string
	// Process each line
	for _, line := range lines {
		words := strings.Fields(line)
		// If we haven't reached maxWords, add words from this line
		if wordCount < maxWords {
			remaining := maxWords - wordCount
			if len(words) <= remaining {
				// Add entire line if it fits
				if len(words) > 0 {
					result = append(result, line)
				} else {
					// Preserve empty lines
					result = append(result, "")
				}
				wordCount += len(words)
			} else {
				// Add partial line up to remaining words
				result = append(result, strings.Join(words[:remaining], " "))
				wordCount = maxWords
			}
		}
	}
	truncated := strings.Join(result, "\n")
	if wordCount >= maxWords {
		truncated += "..."
	}
	return truncated
}

var newlinesRegex = regexp.MustCompile(`\n+`)

func replaceNewlines(text string) string {
	return newlinesRegex.ReplaceAllString(text, "<br>")
}
