package dive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/slogger"
)

var (
	DefaultTaskTimeout      = time.Minute * 5
	DefaultChatTimeout      = time.Minute * 1
	DefaultTickFrequency    = time.Second * 1
	DefaultTaskMessageLimit = 12
	DefaultGenerationLimit  = 5
)

var _ Agent = &DiveAgent{}

type Document struct {
	Name    string
	Content string
	Format  OutputFormat
}

// AgentOptions are used to configure an Agent.
type AgentOptions struct {
	Name             string
	Role             Role
	LLM              llm.LLM
	Tools            []llm.Tool
	MaxActiveTasks   int
	TickFrequency    time.Duration
	TaskTimeout      time.Duration
	ChatTimeout      time.Duration
	CacheControl     string
	LogLevel         string
	Hooks            llm.Hooks
	Logger           slogger.Logger
	GenerationLimit  int
	TaskMessageLimit int
}

// DiveAgent implements the Agent interface.
type DiveAgent struct {
	name             string
	role             Role
	llm              llm.LLM
	team             Team
	running          bool
	tools            []llm.Tool
	toolsByName      map[string]llm.Tool
	isSupervisor     bool
	isWorker         bool
	maxActiveTasks   int
	tickFrequency    time.Duration
	taskTimeout      time.Duration
	chatTimeout      time.Duration
	cacheControl     string
	taskQueue        []*taskState
	recentTasks      []*taskState
	activeTask       *taskState
	workspace        []*Document
	ticker           *time.Ticker
	logLevel         string
	hooks            llm.Hooks
	logger           slogger.Logger
	generationLimit  int
	taskMessageLimit int

	// Holds incoming messages to be processed by the agent's run loop
	mailbox chan interface{}

	mutex sync.Mutex
	wg    sync.WaitGroup
}

// NewAgent returns a new Agent configured with the given options.
func NewAgent(opts AgentOptions) *DiveAgent {
	if opts.MaxActiveTasks <= 0 {
		opts.MaxActiveTasks = 1
	}
	if opts.TickFrequency <= 0 {
		opts.TickFrequency = DefaultTickFrequency
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = DefaultTaskTimeout
	}
	if opts.ChatTimeout <= 0 {
		opts.ChatTimeout = DefaultChatTimeout
	}
	if opts.GenerationLimit <= 0 {
		opts.GenerationLimit = DefaultGenerationLimit
	}
	if opts.TaskMessageLimit <= 0 {
		opts.TaskMessageLimit = DefaultTaskMessageLimit
	}
	if opts.LogLevel == "" {
		opts.LogLevel = "info"
	}
	if opts.Logger == nil {
		opts.Logger = slogger.New(slogger.LevelFromString(opts.LogLevel))
	}
	if opts.LLM == nil {
		if llm, ok := detectProvider(); ok {
			opts.LLM = llm
		} else {
			panic("no llm provided")
		}
	}
	if opts.Name == "" {
		if opts.Role.Description != "" {
			opts.Name = opts.Role.Description
		} else {
			opts.Name = randomName()
		}
	}
	a := &DiveAgent{
		name:             opts.Name,
		llm:              opts.LLM,
		role:             opts.Role,
		maxActiveTasks:   opts.MaxActiveTasks,
		tickFrequency:    opts.TickFrequency,
		taskTimeout:      opts.TaskTimeout,
		chatTimeout:      opts.ChatTimeout,
		generationLimit:  opts.GenerationLimit,
		taskMessageLimit: opts.TaskMessageLimit,
		cacheControl:     opts.CacheControl,
		hooks:            opts.Hooks,
		mailbox:          make(chan interface{}, 64),
		logger:           opts.Logger,
		logLevel:         strings.ToLower(opts.LogLevel),
	}
	var tools []llm.Tool
	if len(opts.Tools) > 0 {
		tools = make([]llm.Tool, len(opts.Tools))
		copy(tools, opts.Tools)
	}
	if opts.Role.IsSupervisor {
		tools = append(tools, NewAssignWorkTool(a))
	}
	a.tools = tools
	if len(tools) > 0 {
		a.toolsByName = make(map[string]llm.Tool, len(tools))
		for _, tool := range tools {
			a.toolsByName[tool.Definition().Name] = tool
		}
	}
	return a
}

func (a *DiveAgent) Name() string {
	return a.name
}

func (a *DiveAgent) Role() Role {
	return a.role
}

func (a *DiveAgent) Join(team Team) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return fmt.Errorf("agent is already running")
	}
	if a.team != nil {
		return fmt.Errorf("agent is already a member of a team")
	}
	a.team = team
	return nil
}

func (a *DiveAgent) Team() Team {
	return a.team
}

func (a *DiveAgent) Log(msg string, keysAndValues ...any) {
	switch a.logLevel {
	case "debug":
		a.logger.Debug(msg, keysAndValues...)
	case "info":
		a.logger.Info(msg, keysAndValues...)
	case "warn":
		a.logger.Warn(msg, keysAndValues...)
	case "error":
		a.logger.Error(msg, keysAndValues...)
	}
}

func (a *DiveAgent) Chat(ctx context.Context, message *llm.Message) (*llm.Response, error) {
	result := make(chan *llm.Response, 1)
	errChan := make(chan error, 1)

	chatMessage := messageChat{
		ctx:     ctx,
		message: message,
		result:  result,
		err:     errChan,
	}

	select {
	case a.mailbox <- chatMessage:
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

func (a *DiveAgent) ChatStream(ctx context.Context, message *llm.Message) (llm.Stream, error) {
	return nil, errors.New("not yet implemented")
}

func (a *DiveAgent) Event(ctx context.Context, event *Event) error {
	if !a.IsRunning() {
		return fmt.Errorf("agent is not running")
	}

	message := messageEvent{event: event}

	select {
	case a.mailbox <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *DiveAgent) Work(ctx context.Context, task *Task) (*Promise, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	promise := &Promise{
		task: task,
		ch:   make(chan *TaskResult, 1),
	}
	message := messageWork{task: task, promise: promise}

	select {
	case a.mailbox <- message:
		return promise, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *DiveAgent) Start(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return fmt.Errorf("agent is already running")
	}

	a.running = true
	a.wg = sync.WaitGroup{}
	a.wg.Add(1)
	go a.run()
	a.logger.Debug("agent started", "name", a.name)
	return nil
}

func (a *DiveAgent) Stop(ctx context.Context) error {
	a.mutex.Lock()
	defer func() {
		a.running = false
		a.mutex.Unlock()
		a.logger.Debug("agent stopped", "name", a.name)
	}()

	if !a.running {
		return fmt.Errorf("agent is not running")
	}
	done := make(chan error)

	a.mailbox <- messageStop{ctx: ctx, done: done}
	close(a.mailbox)

	select {
	case err := <-done:
		a.wg.Wait()
		return err
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for agent to stop: %w", ctx.Err())
	}
}

func (a *DiveAgent) IsRunning() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return a.running
}

func (a *DiveAgent) run() error {
	defer a.wg.Done()

	a.ticker = time.NewTicker(a.tickFrequency)
	defer a.ticker.Stop()

	for {
		select {
		case <-a.ticker.C:
			a.doSomeWork()
		case msg := <-a.mailbox:
			switch m := msg.(type) {
			case messageWork:
				a.handleWorkMessage(m)
			case messageChat:
				a.handleChatMessage(m)
			case messageEvent:
				a.handleEventMessage(m.event)
			case messageStop:
				return a.handleStopMessage(m)
			}
			a.doSomeWork()
		}
	}
}

func (a *DiveAgent) handleWorkMessage(m messageWork) {
	a.taskQueue = append(a.taskQueue, &taskState{
		Task:    m.task,
		Promise: m.promise,
		Status:  TaskStatusQueued,
	})
}

func (a *DiveAgent) handleEventMessage(event *Event) {
	fmt.Printf("event: %+v\n", event)
}

func (a *DiveAgent) handleChatMessage(m messageChat) {
	// Create a task corresponding to the chat request
	task := &Task{
		kind:        "chat",
		name:        fmt.Sprintf("chat-%s", petname.Generate(2, "-")),
		description: fmt.Sprintf("Generate a response to user message: %q", m.message.Text()),
		timeout:     a.chatTimeout,
	}
	// Enqueue a corresponding task state
	a.taskQueue = append(a.taskQueue, &taskState{
		Task:         task,
		Promise:      &Promise{ch: make(chan *TaskResult, 1)},
		Status:       TaskStatusQueued,
		Messages:     []*llm.Message{m.message},
		ChanResponse: m.result,
		ChanError:    m.err,
	})
}

func (a *DiveAgent) handleStopMessage(msg messageStop) error {
	msg.done <- nil
	return nil
}

func (a *DiveAgent) getSystemPrompt() (string, error) {
	return executeTemplate(agentSystemPromptTemplate, NewAgentTemplateData(a))
}

func (a *DiveAgent) handleTask(state *taskState) error {
	task := state.Task
	timeout := task.Timeout()
	if timeout == 0 {
		timeout = a.taskTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	systemPrompt, err := a.getSystemPrompt()
	if err != nil {
		return err
	}
	messages := []*llm.Message{}

	if len(state.Messages) == 0 {
		// We're starting a new task
		recentTasksMessage, ok := a.getTasksHistoryMessage()
		if ok {
			messages = append(messages, recentTasksMessage)
		}
		messages = append(messages, llm.NewUserMessage(task.PromptText()))
	} else if len(state.Messages) < a.taskMessageLimit {
		// We're resuming a task and can still work some more
		messages = append(messages, state.Messages...)
		messages = append(messages, llm.NewUserMessage("Continue working on the task."))
	} else {
		// We're resuming a task but need to wrap it up
		messages = append(messages, state.Messages...)
		messages = append(messages, llm.NewUserMessage("Finish the task to the best of your ability now. Do not use any more tools. Respond with the complete response to the task's prompt."))
	}

	a.logger.Info("handling task",
		"agent", a.name,
		"task", task.Name(),
		"status", state.Status,
		"truncated_description", TruncateText(task.Description(), 10),
		"truncated_prompt", TruncateText(task.PromptText(), 10),
	)

	// Holds the most recent response from the LLM
	var response *llm.Response

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	for i := 0; i < a.generationLimit; i++ {
		var prefill string
		if i == a.generationLimit-1 {
			prefill = "<think>I must respond with my final answer now."
		} else {
			prefill = "<think>"
		}
		promptMessages := append(messages, llm.NewAssistantMessage(prefill))

		generateOpts := []llm.Option{
			llm.WithSystemPrompt(systemPrompt),
			llm.WithCacheControl(a.cacheControl),
			llm.WithLogLevel(a.logLevel),
			llm.WithTools(a.tools...),
		}
		if a.hooks != nil {
			generateOpts = append(generateOpts, llm.WithHooks(a.hooks))
		}
		if a.logger != nil {
			generateOpts = append(generateOpts, llm.WithLogger(a.logger))
		}

		response, err = a.llm.Generate(ctx, promptMessages, generateOpts...)
		if err != nil {
			return err
		}

		// Mutate first text message response to include the prefill text if
		// we see the closing </think> tag. The prefill is a behind-the-scenes
		// behavior for the caller.
		responseMessage := response.Message()
		addPrefill(responseMessage, prefill)

		// Remember the assistant response message
		messages = append(messages, responseMessage)

		// We're done if there are no tool-uses
		if len(response.ToolCalls()) == 0 {
			break
		}

		// Execute tool-uses and accumulate results
		shouldReturnResult := false
		toolResults := make([]*llm.ToolResult, len(response.ToolCalls()))
		for i, toolCall := range response.ToolCalls() {
			tool, ok := a.toolsByName[toolCall.Name]
			if !ok {
				return fmt.Errorf("tool call for unknown tool: %q", toolCall.Name)
			}
			result, err := tool.Call(ctx, toolCall.Input)
			if err != nil {
				return fmt.Errorf("tool call error: %w", err)
			}
			toolResults[i] = &llm.ToolResult{
				ID:     toolCall.ID,
				Name:   toolCall.Name,
				Result: result,
			}
			if tool.ShouldReturnResult() {
				shouldReturnResult = true
			}
		}

		// If no tool calls need to return results to the LLM, we're done
		if !shouldReturnResult {
			break
		}

		// Capture results in a new message to send on next loop iteration
		resultMessage := llm.NewToolResultMessage(toolResults)

		// Add instructions to the message to not use any more tools if we
		// have only one generation left.
		if i == a.generationLimit-2 {
			resultMessage.Content = append(resultMessage.Content, &llm.Content{
				Type: llm.ContentTypeText,
				Text: "Do not use any more tools. You must respond with your final answer now.",
			})
			a.logger.Debug("adding tool use limit instruction", "agent", a.name, "task", task.Name())
		}

		messages = append(messages, resultMessage)
	}

	// Update task state based on the last response from the LLM. It should
	// contain thinking, status, and the primary output. We could concatenate
	// the new output with prior output, but for now it seems like it's better
	// not to, and to request a full final response instead.
	taskResponse := ParseStructuredResponse(response.Message().Text())
	state.Output = taskResponse.Text
	state.Reasoning = taskResponse.Thinking
	state.StatusDescription = taskResponse.StatusDescription
	state.Messages = messages

	// For now, if the status description is empty, let's assume it is complete.
	// We may need to make this configurable in the future.
	if taskResponse.StatusDescription == "" {
		state.Status = TaskStatusCompleted
		a.logger.Warn("defaulting to completed status",
			"agent", a.name,
			"task", task.Name(),
		)
	} else {
		state.Status = taskResponse.Status()
	}

	a.logger.Info("task updated",
		"agent", a.name,
		"task", task.Name(),
		"status", state.Status,
		"status_description", state.StatusDescription,
		"truncated_output", TruncateText(state.Output, 10),
	)
	return nil
}

func (a *DiveAgent) doSomeWork() {

	// Activate the next task if there is one and we're idle
	if a.activeTask == nil && len(a.taskQueue) > 0 {
		// Pop and activate the first task in queue
		a.activeTask = a.taskQueue[0]
		a.taskQueue = a.taskQueue[1:]
		a.activeTask.Status = TaskStatusActive
		if !a.activeTask.Suspended {
			a.activeTask.Started = time.Now()
		}
		a.activeTask.Suspended = false
		a.logger.Debug("task activated",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"description", a.activeTask.Task.Description(),
		)
	}

	if a.activeTask == nil {
		return
	}

	// Make progress on the active task
	err := a.handleTask(a.activeTask)

	// An error deactivates the task and we send the error via the promise
	if err != nil {
		duration := time.Since(a.activeTask.Started)
		a.logger.Error("task error",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"duration", duration.Seconds(),
			"error", err,
		)

		a.activeTask.Status = TaskStatusError
		a.rememberTask(a.activeTask)
		a.activeTask.Promise.ch <- NewTaskResultError(a.activeTask.Task, err)
		a.activeTask = nil
		return
	}

	switch a.activeTask.Status {
	case TaskStatusCompleted:
		duration := time.Since(a.activeTask.Started)
		a.logger.Debug("task completed",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"duration", duration.Seconds(),
		)
		a.activeTask.Status = TaskStatusCompleted
		a.rememberTask(a.activeTask)
		if a.activeTask.Task.Kind() == "chat" {
			a.activeTask.ChanResponse <- llm.NewResponse(llm.ResponseOptions{
				Role:    llm.Assistant,
				Message: llm.NewAssistantMessage(a.activeTask.Output),
			})
		}
		if a.activeTask.Promise != nil {
			a.activeTask.Promise.ch <- &TaskResult{
				Task:    a.activeTask.Task,
				Content: a.activeTask.Output,
			}
		}
		a.activeTask = nil

	case TaskStatusPaused:
		a.logger.Debug("task paused",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
		)
		a.activeTask.Suspended = true
		a.taskQueue = append(a.taskQueue, a.activeTask)
		a.activeTask = nil

	case TaskStatusBlocked, TaskStatusError, TaskStatusInvalid:
		a.logger.Warn("task error",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"status", a.activeTask.Status,
			"status_description", a.activeTask.StatusDescription,
			"duration", time.Since(a.activeTask.Started).Seconds(),
		)
		if a.activeTask.Task.Kind() == "chat" && a.activeTask.ChanError != nil {
			a.activeTask.ChanError <- fmt.Errorf("task error: %s", a.activeTask.Status)
		}
		if a.activeTask.Promise != nil {
			a.activeTask.Promise.ch <- NewTaskResultError(a.activeTask.Task, fmt.Errorf("task error"))
		}
		a.activeTask = nil

	case TaskStatusActive:
		a.logger.Debug("task remains active",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"status", a.activeTask.Status,
			"status_description", a.activeTask.StatusDescription,
			"duration", time.Since(a.activeTask.Started).Seconds(),
		)
	}
}

// Remember the last 10 tasks that were worked on, so that the agent can
// use them as context for future tasks.
func (a *DiveAgent) rememberTask(task *taskState) {
	a.recentTasks = append(a.recentTasks, task)
	if len(a.recentTasks) > 10 {
		a.recentTasks = a.recentTasks[1:]
	}
}

// Returns a block of text that summarizes the most recent tasks worked on by
// the agent. The text is truncated if needed to avoid using a lot of tokens.
func (a *DiveAgent) getTasksHistory() string {
	if len(a.recentTasks) == 0 {
		return ""
	}
	history := make([]string, len(a.recentTasks))
	for i, status := range a.recentTasks {
		title := status.Task.Name()
		if title == "" {
			title = status.Task.Description()
		}
		history[i] = fmt.Sprintf("- task: %q status: %q output: %q\n",
			TruncateText(title, 8),
			status.Status,
			TruncateText(replaceNewlines(status.Output), 8),
		)
	}
	result := strings.Join(history, "\n")
	if len(result) > 200 {
		result = result[:200]
	}
	return result
}

// Returns a user message that contains a summary of the most recent tasks
// worked on by the agent.
func (a *DiveAgent) getTasksHistoryMessage() (*llm.Message, bool) {
	history := a.getTasksHistory()
	if history == "" {
		return nil, false
	}
	text := fmt.Sprintf("Recently completed tasks:\n\n%s", history)
	return llm.NewUserMessage(text), true
}

func (a *DiveAgent) getWorkspaceState() string {
	var blobs []string
	for _, doc := range a.workspace {
		blobs = append(blobs, fmt.Sprintf("<document name=%q>\n%s\n</document>", doc.Name, doc.Content))
	}
	return strings.Join(blobs, "\n\n")
}
