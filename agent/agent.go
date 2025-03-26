package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/slogger"
)

var (
	DefaultTaskTimeout        = time.Minute * 5
	DefaultChatTimeout        = time.Minute * 1
	DefaultTickFrequency      = time.Second * 1
	DefaultToolIterationLimit = 8
)

// Confirm our standard implementation satisfies the different Agent interfaces
var (
	_ dive.Agent         = &Agent{}
	_ dive.RunnableAgent = &Agent{}
)

// Options are used to configure an Agent.
type Options struct {
	Name                 string
	Goal                 string
	Backstory            string
	AcceptedEvents       []string
	IsSupervisor         bool
	Subordinates         []string
	LLM                  llm.LLM
	Tools                []llm.Tool
	TickFrequency        time.Duration
	TaskTimeout          time.Duration
	ChatTimeout          time.Duration
	CacheControl         string
	Hooks                llm.Hooks
	Logger               slogger.Logger
	ToolIterationLimit   int
	Temperature          *float64
	PresencePenalty      *float64
	FrequencyPenalty     *float64
	ReasoningFormat      string
	ReasoningEffort      string
	DateAwareness        *bool
	Environment          dive.Environment
	DocumentRepository   dive.DocumentRepository
	ThreadRepository     dive.ThreadRepository
	SystemPromptTemplate string
}

// Agent is the standard implementation of the Agent interface.
type Agent struct {
	name                 string
	goal                 string
	backstory            string
	llm                  llm.LLM
	running              bool
	tools                []llm.Tool
	toolsByName          map[string]llm.Tool
	acceptedEvents       []string
	isSupervisor         bool
	subordinates         []string
	tickFrequency        time.Duration
	taskTimeout          time.Duration
	chatTimeout          time.Duration
	cacheControl         string
	taskQueue            []*taskState
	recentTasks          []*taskState
	activeTask           *taskState
	ticker               *time.Ticker
	hooks                llm.Hooks
	logger               slogger.Logger
	toolIterationLimit   int
	temperature          *float64
	presencePenalty      *float64
	frequencyPenalty     *float64
	reasoningFormat      string
	reasoningEffort      string
	dateAwareness        *bool
	environment          dive.Environment
	documentRepository   dive.DocumentRepository
	threadRepository     dive.ThreadRepository
	systemPromptTemplate *template.Template

	// Holds incoming messages to be processed by the agent's run loop
	mailbox chan interface{}

	mutex sync.Mutex
	wg    sync.WaitGroup
}

// New returns a new Agent configured with the given options.
func New(opts Options) (*Agent, error) {
	if opts.TickFrequency <= 0 {
		opts.TickFrequency = DefaultTickFrequency
	}
	if opts.TaskTimeout <= 0 {
		opts.TaskTimeout = DefaultTaskTimeout
	}
	if opts.ChatTimeout <= 0 {
		opts.ChatTimeout = DefaultChatTimeout
	}
	if opts.ToolIterationLimit <= 0 {
		opts.ToolIterationLimit = DefaultToolIterationLimit
	}
	if opts.Logger == nil {
		opts.Logger = slogger.DefaultLogger
	}
	if opts.LLM == nil {
		if llm, ok := detectProvider(); ok {
			opts.LLM = llm
		} else {
			return nil, fmt.Errorf("no llm provided")
		}
	}
	if opts.Name == "" {
		opts.Name = dive.RandomName()
	}
	if opts.SystemPromptTemplate == "" {
		opts.SystemPromptTemplate = SystemPromptTemplate
	}
	systemPromptTemplate, err := parseTemplate("agent", opts.SystemPromptTemplate)
	if err != nil {
		return nil, fmt.Errorf("invalid system prompt template: %w", err)
	}

	agent := &Agent{
		name:                 strings.TrimSpace(opts.Name),
		goal:                 strings.TrimSpace(opts.Goal),
		backstory:            strings.TrimSpace(opts.Backstory),
		llm:                  opts.LLM,
		environment:          opts.Environment,
		acceptedEvents:       opts.AcceptedEvents,
		isSupervisor:         opts.IsSupervisor,
		subordinates:         opts.Subordinates,
		tickFrequency:        opts.TickFrequency,
		taskTimeout:          opts.TaskTimeout,
		chatTimeout:          opts.ChatTimeout,
		toolIterationLimit:   opts.ToolIterationLimit,
		cacheControl:         opts.CacheControl,
		hooks:                opts.Hooks,
		mailbox:              make(chan interface{}, 16),
		logger:               opts.Logger,
		dateAwareness:        opts.DateAwareness,
		documentRepository:   opts.DocumentRepository,
		threadRepository:     opts.ThreadRepository,
		systemPromptTemplate: systemPromptTemplate,
	}

	tools := make([]llm.Tool, len(opts.Tools))
	if len(opts.Tools) > 0 {
		copy(tools, opts.Tools)
	}

	// Supervisors need a tool to give work assignments to others
	if opts.IsSupervisor {
		// Only create the assign_work tool if it wasn't provided. This allows
		// a custom assign_work implementation to be used.
		var foundAssignWorkTool bool
		for _, tool := range tools {
			if tool.Definition().Name == "assign_work" {
				foundAssignWorkTool = true
			}
		}
		if !foundAssignWorkTool {
			tools = append(tools, NewAssignWorkTool(AssignWorkToolOptions{
				Self:               agent,
				DefaultTaskTimeout: opts.TaskTimeout,
			}))
		}
	}

	agent.tools = tools
	if len(tools) > 0 {
		agent.toolsByName = make(map[string]llm.Tool, len(tools))
		for _, tool := range tools {
			agent.toolsByName[tool.Definition().Name] = tool
		}
	}

	// Register with environment if provided
	if opts.Environment != nil {
		if err := opts.Environment.RegisterAgent(agent); err != nil {
			return nil, fmt.Errorf("failed to register agent with environment: %w", err)
		}
	}

	return agent, nil
}

func (a *Agent) Name() string {
	return a.name
}

func (a *Agent) Goal() string {
	return a.goal
}

func (a *Agent) Backstory() string {
	return a.backstory
}

func (a *Agent) AcceptedEvents() []string {
	return a.acceptedEvents
}

func (a *Agent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *Agent) Subordinates() []string {
	if !a.isSupervisor || a.environment == nil {
		return nil
	}
	if a.subordinates != nil {
		return a.subordinates
	}
	// If there are no other supervisors, assume we are the supervisor of all
	// agents in the environment.
	var isAnotherSupervisor bool
	for _, agent := range a.environment.Agents() {
		if agent.IsSupervisor() && agent.Name() != a.name {
			isAnotherSupervisor = true
		}
	}
	if isAnotherSupervisor {
		return nil
	}
	var others []string
	for _, agent := range a.environment.Agents() {
		if agent.Name() != a.name {
			others = append(others, agent.Name())
		}
	}
	a.subordinates = others
	return others
}

func (a *Agent) SetEnvironment(env dive.Environment) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.environment = env
}

func (a *Agent) Start(ctx context.Context) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return fmt.Errorf("agent is already running")
	}

	a.running = true
	a.wg = sync.WaitGroup{}
	a.wg.Add(1)
	go a.run()

	a.logger.Debug("agent started",
		"name", a.name,
		"goal", a.goal,
		"cache_control", a.cacheControl,
		"is_supervisor", a.isSupervisor,
		"subordinates", a.subordinates,
		"task_timeout", a.taskTimeout,
		"chat_timeout", a.chatTimeout,
		"tick_frequency", a.tickFrequency,
		"tool_iteration_limit", a.toolIterationLimit,
		"model", a.llm.Name(),
	)
	return nil
}

func (a *Agent) Stop(ctx context.Context) error {
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

func (a *Agent) IsRunning() bool {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	return a.running
}

func (a *Agent) Generate(ctx context.Context, messages []*llm.Message, opts ...dive.GenerateOption) (*llm.Response, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	var generateOptions dive.GenerateOptions
	generateOptions.Apply(opts)

	resultChan := make(chan *llm.Response, 1)
	errChan := make(chan error, 1)

	chatMessage := messageChat{
		messages:   messages,
		options:    generateOptions,
		resultChan: resultChan,
		errChan:    errChan,
	}

	// Send the chat message to the agent's mailbox, but make sure we timeout
	// if the agent doesn't pick it up in a reasonable amount of time
	select {
	case a.mailbox <- chatMessage:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for the agent to respond
	select {
	case resp := <-resultChan:
		return resp, nil
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Agent) Stream(ctx context.Context, messages []*llm.Message, opts ...dive.GenerateOption) (dive.Stream, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	var generateOptions dive.GenerateOptions
	generateOptions.Apply(opts)

	stream := dive.NewStream()

	chatMessage := messageChat{
		messages: messages,
		options:  generateOptions,
		stream:   stream,
	}

	// Send the chat message to the agent's mailbox, but make sure we timeout
	// if the agent doesn't pick it up in a reasonable amount of time
	select {
	case a.mailbox <- chatMessage:
	case <-ctx.Done():
		stream.Close()
		return nil, ctx.Err()
	}

	return stream, nil
}

func (a *Agent) Work(ctx context.Context, task dive.Task) (dive.Stream, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	// Stream to be returned to the caller so it can wait for results
	stream := dive.NewStream()

	message := messageWork{
		task:      task,
		publisher: stream.Publisher(),
	}

	select {
	case a.mailbox <- message:
		return stream, nil
	case <-ctx.Done():
		stream.Close()
		return nil, ctx.Err()
	}
}

// This is the agent's main run loop. It dispatches incoming messages and runs
// a ticker that wakes the agent up periodically even if there are no messages.
func (a *Agent) run() error {
	defer a.wg.Done()

	a.ticker = time.NewTicker(a.tickFrequency)
	defer a.ticker.Stop()

	for {
		select {
		case <-a.ticker.C:
		case msg := <-a.mailbox:
			switch msg := msg.(type) {
			case messageWork:
				a.handleWork(msg)

			case messageChat:
				a.handleChat(msg)

			case messageStop:
				msg.done <- nil
				return nil
			}
		}
		// Make progress on any active tasks
		a.doSomeWork()
	}
}

func (a *Agent) handleWork(m messageWork) {
	a.taskQueue = append(a.taskQueue, &taskState{
		Task:      m.task,
		Publisher: m.publisher,
		Status:    dive.TaskStatusQueued,
	})
}

func (a *Agent) handleChat(m messageChat) {
	var ctx context.Context
	if a.chatTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), a.chatTimeout)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	systemPrompt, err := a.getSystemPromptForMode("chat")
	if err != nil {
		m.errChan <- err
		return
	}

	var isStreaming bool
	var publisher dive.Publisher
	if m.stream != nil {
		isStreaming = true
		publisher = m.stream.Publisher()
		defer publisher.Close()
	}

	logger := a.logger.With(
		"agent_name", a.name,
		"streaming", isStreaming,
		"thread_id", m.options.ThreadID,
		"user_id", m.options.UserID,
	)
	logger.Info("handling chat")

	// Append this new message to the thread history if a thread id was provided
	var thread *dive.Thread
	var messages []*llm.Message
	if m.options.ThreadID != "" {
		if a.threadRepository == nil {
			m.errChan <- fmt.Errorf("thread history not enabled")
			return
		}
		var err error
		thread, err = a.getOrCreateThread(ctx, m.options.ThreadID)
		if err != nil {
			m.errChan <- err
			return
		}
		if len(thread.Messages) > 0 {
			messages = append(messages, thread.Messages...)
		}
	}
	messages = append(messages, m.messages...)

	response, updatedMessages, err := a.generate(ctx, messages, systemPrompt, publisher)
	if err != nil {
		logger.Error("error handling chat", "error", err)
		// Intentional fall-through
	} else if thread != nil {
		thread.Messages = updatedMessages
		if err := a.threadRepository.PutThread(ctx, thread); err != nil {
			logger.Error("error updating thread", "error", err)
			m.errChan <- err
			return
		}
	}
	if isStreaming {
		return
	}
	if err != nil {
		m.errChan <- err
		return
	}
	m.resultChan <- response
}

func (a *Agent) getOrCreateThread(ctx context.Context, threadID string) (*dive.Thread, error) {
	if a.threadRepository == nil {
		return nil, fmt.Errorf("thread history not enabled")
	}
	thread, err := a.threadRepository.GetThread(ctx, threadID)
	if err == nil {
		return thread, nil
	}
	if err != dive.ErrThreadNotFound {
		return nil, err
	}
	return &dive.Thread{
		ID:       threadID,
		Messages: []*llm.Message{},
	}, nil
}

func (a *Agent) getSystemPromptForMode(mode string) (string, error) {
	var responseGuidelines string
	if mode == "task" {
		responseGuidelines = PromptForTaskResponses
	}
	data := newAgentTemplateData(a, responseGuidelines)
	prompt, err := executeTemplate(a.systemPromptTemplate, data)
	if err != nil {
		return "", err
	}
	if a.dateAwareness == nil || *a.dateAwareness {
		prompt = fmt.Sprintf("%s\n\n# Date and Time\n\n%s", prompt, dive.DateString(time.Now()))
	}
	return strings.TrimSpace(prompt), nil
}

// generate runs the LLM generation and tool execution loop. It handles the
// interaction between the agent and the LLM, including tool calls. Returns the
// final LLM response and any error that occurred.
func (a *Agent) generate(
	ctx context.Context,
	messages []*llm.Message,
	systemPrompt string,
	publisher dive.Publisher,
) (*llm.Response, []*llm.Message, error) {
	// Holds the most recent response from the LLM
	var response *llm.Response

	// Contains the message history. We'll append to this and return it when done.
	updatedMessages := make([]*llm.Message, len(messages))
	copy(updatedMessages, messages)

	// Helper function to safely send events to the publisher
	safePublish := func(event *dive.Event) error {
		if publisher == nil {
			return nil
		}
		return publisher.Send(ctx, event)
	}

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	generationLimit := a.toolIterationLimit + 1
	for i := range generationLimit {
		generateOpts := []llm.Option{
			llm.WithSystemPrompt(systemPrompt),
			llm.WithCacheControl(a.cacheControl),
			llm.WithTools(a.tools...),
		}
		if a.hooks != nil {
			generateOpts = append(generateOpts, llm.WithHooks(a.hooks))
		}
		if a.logger != nil {
			generateOpts = append(generateOpts, llm.WithLogger(a.logger))
		}
		if a.temperature != nil {
			generateOpts = append(generateOpts, llm.WithTemperature(*a.temperature))
		}
		if a.presencePenalty != nil {
			generateOpts = append(generateOpts, llm.WithPresencePenalty(*a.presencePenalty))
		}
		if a.frequencyPenalty != nil {
			generateOpts = append(generateOpts, llm.WithFrequencyPenalty(*a.frequencyPenalty))
		}
		if a.reasoningFormat != "" {
			generateOpts = append(generateOpts, llm.WithReasoningFormat(a.reasoningFormat))
		}
		if a.reasoningEffort != "" {
			generateOpts = append(generateOpts, llm.WithReasoningEffort(a.reasoningEffort))
		}

		var currentResponse *llm.Response

		if streamingLLM, ok := a.llm.(llm.StreamingLLM); ok {
			iterator, err := streamingLLM.Stream(ctx, updatedMessages, generateOpts...)
			if err != nil {
				return nil, nil, err
			}
			for iterator.Next() {
				event := iterator.Event()
				if err := safePublish(&dive.Event{
					Type:    "llm.event",
					Origin:  a.eventOrigin(),
					Payload: event,
				}); err != nil {
					iterator.Close()
					return nil, nil, err
				}
				if event.Response != nil {
					currentResponse = event.Response
				}
			}
			iterator.Close()
			if err := iterator.Err(); err != nil {
				return nil, nil, err
			}
		} else {
			var err error
			currentResponse, err = a.llm.Generate(ctx, updatedMessages, generateOpts...)
			if err != nil {
				return nil, nil, err
			}
		}

		if currentResponse == nil {
			// This indicates a bug in the LLM provider implementation
			return nil, nil, errors.New("no final response from llm provider")
		}

		if err := safePublish(&dive.Event{
			Type:    "llm.response",
			Origin:  a.eventOrigin(),
			Payload: currentResponse,
		}); err != nil {
			return nil, nil, err
		}

		response = currentResponse
		responseMessage := response.Message()

		a.logger.Debug("llm response",
			"usage_input_tokens", response.Usage().InputTokens,
			"usage_output_tokens", response.Usage().OutputTokens,
			"cache_creation_input_tokens", response.Usage().CacheCreationInputTokens,
			"cache_read_input_tokens", response.Usage().CacheReadInputTokens,
			"response_text", responseMessage.Text(),
			"generation_number", i+1,
		)

		// Remember the assistant response message
		updatedMessages = append(updatedMessages, responseMessage)

		// We're done if there are no tool-uses
		if len(response.ToolCalls()) == 0 {
			break
		}

		// Execute all requested tool uses and accumulate results
		shouldReturnResult := false
		toolResults := make([]*llm.ToolResult, len(response.ToolCalls()))

		for i, toolCall := range response.ToolCalls() {
			tool, ok := a.toolsByName[toolCall.Name]
			if !ok {
				return nil, nil, fmt.Errorf("tool call for unknown tool: %q", toolCall.Name)
			}
			a.logger.Debug("executing tool call",
				"tool_id", toolCall.ID,
				"tool_name", toolCall.Name,
				"tool_input", toolCall.Input)

			if err := safePublish(&dive.Event{
				Type:    "llm.tool_call",
				Origin:  a.eventOrigin(),
				Payload: toolCall,
			}); err != nil {
				return nil, nil, err
			}

			result, err := tool.Call(ctx, toolCall.Input)
			if err != nil {
				if err := safePublish(&dive.Event{
					Type:   "llm.tool_error",
					Origin: a.eventOrigin(),
					Payload: &llm.ToolError{
						ID:    toolCall.ID,
						Name:  toolCall.Name,
						Error: err.Error(),
					},
				}); err != nil {
					return nil, nil, err
				}
				return nil, nil, fmt.Errorf("tool call error: %w", err)
			}

			toolResults[i] = &llm.ToolResult{
				ID:     toolCall.ID,
				Name:   toolCall.Name,
				Result: result,
			}

			if err := safePublish(&dive.Event{
				Type:    "llm.tool_result",
				Origin:  a.eventOrigin(),
				Payload: toolResults[i],
			}); err != nil {
				return nil, nil, err
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

		if err := safePublish(&dive.Event{
			Type:    "llm.tool_result_message",
			Origin:  a.eventOrigin(),
			Payload: resultMessage,
		}); err != nil {
			return nil, nil, err
		}

		// Add instructions to the message to not use any more tools if we have
		// only one generation left
		if i == generationLimit-2 {
			resultMessage.Content = append(resultMessage.Content, &llm.Content{
				Type: llm.ContentTypeText,
				Text: "Do not use any more tools. You must respond with your final answer now.",
			})
			a.logger.Debug("added tool use limit instruction",
				"agent", a.name,
				"generation_number", i+1)
		}
		updatedMessages = append(updatedMessages, resultMessage)
	}

	return response, updatedMessages, nil
}

func (a *Agent) TeamOverview() string {
	if a.environment == nil {
		return ""
	}
	agents := a.environment.Agents()
	if len(agents) == 0 {
		return ""
	}
	if len(agents) == 1 && agents[0].Name() == a.name {
		return "You are the only agent on the team."
	}
	lines := []string{
		"The team is comprised of the following agents:",
	}
	for _, agent := range agents {
		description := fmt.Sprintf("- Name: %s", agent.Name())
		if goal := agent.Goal(); goal != "" {
			description += fmt.Sprintf(" Goal: %s", goal)
		}
		if agent.Name() == a.name {
			description += " (You)"
		}
		lines = append(lines, description)
	}
	return strings.Join(lines, "\n")
}

func (a *Agent) handleTask(ctx context.Context, state *taskState) error {
	task := state.Task

	timeout := a.taskTimeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	logger := a.logger.With(
		"agent_name", a.name,
		"task_name", task.Name(),
		"timeout", timeout.String(),
	)
	logger.Info("handling task", "status", state.Status)

	systemPrompt, err := a.getSystemPromptForMode("task")
	if err != nil {
		return err
	}

	messages := []*llm.Message{}

	if len(state.Messages) == 0 {
		// Starting a task
		if recentTasksMessage, ok := a.getTasksHistoryMessage(); ok {
			messages = append(messages, recentTasksMessage)
		}
		prompt, err := task.Prompt()
		if err != nil {
			return err
		}
		promptMessages, err := taskPromptMessages(prompt)
		if err != nil {
			return err
		}
		messages = append(messages, promptMessages...)
	} else {
		// Resuming a task
		messages = append(messages, state.Messages...)
		if len(messages) < 32 {
			messages = append(messages, llm.NewUserMessage(PromptContinue))
		} else {
			messages = append(messages, llm.NewUserMessage(PromptFinishNow))
		}
	}

	response, updatedMessages, err := a.generate(ctx, messages, systemPrompt, state.Publisher)
	if err != nil {
		return err
	}
	state.TrackResponse(response, updatedMessages)

	logger.Info("step updated",
		"status", state.Status,
		"status_description", state.StatusDescription(),
	)
	return nil
}

func (a *Agent) eventOrigin() dive.EventOrigin {
	var taskName string
	if a.activeTask != nil {
		taskName = a.activeTask.Task.Name()
	}
	var environmentName string
	if a.environment != nil {
		environmentName = a.environment.Name()
	}
	return dive.EventOrigin{
		AgentName:       a.name,
		TaskName:        taskName,
		EnvironmentName: environmentName,
	}
}

func (a *Agent) doSomeWork() {

	// Helper function to safely send events to the active task's publisher
	safePublish := func(event *dive.Event) error {
		if a.activeTask.Publisher == nil {
			return nil
		}
		return a.activeTask.Publisher.Send(context.Background(), event)
	}

	// Activate the next task if there is one and we're idle
	if a.activeTask == nil && len(a.taskQueue) > 0 {
		// Pop and activate the first task in queue
		a.activeTask = a.taskQueue[0]
		a.taskQueue = a.taskQueue[1:]
		a.activeTask.Status = dive.TaskStatusActive
		if !a.activeTask.Paused {
			a.activeTask.Started = time.Now()
		} else {
			a.activeTask.Paused = false
		}
		safePublish(&dive.Event{
			Type:   "task.activated",
			Origin: a.eventOrigin(),
		})
		a.logger.Debug("task activated",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
		)
	}

	if a.activeTask == nil {
		return // Nothing to do!
	}
	taskName := a.activeTask.Task.Name()

	// Make progress on the active task
	err := a.handleTask(context.Background(), a.activeTask)

	// An error deactivates the task and pushes an error event on the stream
	if err != nil {
		a.activeTask.Status = dive.TaskStatusError
		a.rememberTask(a.activeTask)
		safePublish(&dive.Event{
			Type:   "task.error",
			Origin: a.eventOrigin(),
			Error:  err,
		})
		a.logger.Error("task error",
			"agent", a.name,
			"task", taskName,
			"duration", time.Since(a.activeTask.Started).Seconds(),
			"error", err,
		)
		if a.activeTask.Publisher != nil {
			a.activeTask.Publisher.Close()
			a.activeTask.Publisher = nil
		}
		a.activeTask = nil
		return
	}

	// Handle task state transitions
	switch a.activeTask.Status {

	case dive.TaskStatusCompleted:
		a.rememberTask(a.activeTask)
		safePublish(&dive.Event{
			Type:   "task.result",
			Origin: a.eventOrigin(),
			Payload: &dive.TaskResult{
				Task:    a.activeTask.Task,
				Usage:   a.activeTask.Usage,
				Content: a.activeTask.LastOutput(),
			},
		})
		if a.activeTask.Publisher != nil {
			a.activeTask.Publisher.Close()
			a.activeTask.Publisher = nil
		}
		a.activeTask = nil

	case dive.TaskStatusActive:
		a.logger.Debug("step remains active",
			"agent", a.name,
			"task", taskName,
			"status", a.activeTask.Status,
			"status_description", a.activeTask.StatusDescription,
			"duration", time.Since(a.activeTask.Started).Seconds(),
		)
		safePublish(&dive.Event{
			Type:   "task.progress",
			Origin: a.eventOrigin(),
		})

	case dive.TaskStatusPaused:
		// Set paused flag and return the task to the queue
		a.logger.Debug("step paused",
			"agent", a.name,
			"task", taskName,
		)
		safePublish(&dive.Event{
			Type:   "task.paused",
			Origin: a.eventOrigin(),
		})
		a.activeTask.Paused = true
		a.taskQueue = append(a.taskQueue, a.activeTask)
		a.activeTask = nil

	case dive.TaskStatusBlocked, dive.TaskStatusError, dive.TaskStatusInvalid:
		a.logger.Warn("task error",
			"agent", a.name,
			"task", taskName,
			"status", a.activeTask.Status,
			"status_description", a.activeTask.StatusDescription,
			"duration", time.Since(a.activeTask.Started).Seconds(),
		)
		safePublish(&dive.Event{
			Type:   "task.error",
			Origin: a.eventOrigin(),
			Error:  fmt.Errorf("task status: %s", a.activeTask.Status),
		})
		if a.activeTask.Publisher != nil {
			a.activeTask.Publisher.Close()
			a.activeTask.Publisher = nil
		}
		a.activeTask = nil
	}
}

// Remember the last 10 tasks that were worked on, so that the agent can use
// them as context for future tasks.
func (a *Agent) rememberTask(task *taskState) {
	a.recentTasks = append(a.recentTasks, task)
	if len(a.recentTasks) > 10 {
		a.recentTasks = a.recentTasks[1:]
	}
}

// Returns a block of text that summarizes the most recent tasks worked on by
// the agent. The text is truncated if needed to avoid using a lot of tokens.
func (a *Agent) getTasksHistory() string {
	if len(a.recentTasks) == 0 {
		return ""
	}
	history := make([]string, len(a.recentTasks))
	for i, status := range a.recentTasks {
		title := status.Task.Name()
		history[i] = fmt.Sprintf("- task: %q status: %q output: %q\n",
			dive.TruncateText(title, 8),
			status.Status,
			dive.TruncateText(replaceNewlines(status.LastOutput()), 10),
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
func (a *Agent) getTasksHistoryMessage() (*llm.Message, bool) {
	history := a.getTasksHistory()
	if history == "" {
		return nil, false
	}
	text := fmt.Sprintf("Recently completed tasks:\n\n%s", history)
	return llm.NewUserMessage(text), true
}

func (a *Agent) Fingerprint() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("agent: %s\n", a.name))
	sb.WriteString(fmt.Sprintf("goal: %s\n", a.goal))
	sb.WriteString(fmt.Sprintf("backstory: %s\n", a.backstory))
	sb.WriteString(fmt.Sprintf("is_supervisor: %t\n", a.isSupervisor))
	sb.WriteString(fmt.Sprintf("subordinates: %v\n", a.subordinates))
	sb.WriteString(fmt.Sprintf("llm: %s\n", a.llm.Name()))
	hash := sha256.New()
	hash.Write([]byte(sb.String()))
	return hex.EncodeToString(hash.Sum(nil))
}

func taskPromptMessages(prompt *dive.Prompt) ([]*llm.Message, error) {
	messages := []*llm.Message{}

	// Add context information if available
	if len(prompt.Context) > 0 {
		contextLines := []string{
			"Important: The following context may contain relevant information to help you complete the task. " +
				"Review and incorporate this information into your approach:",
		}
		for _, context := range prompt.Context {
			var contextBlock string
			if context.Name != "" {
				contextBlock = fmt.Sprintf("<context name=%q>\n%s\n</context>", context.Name, context.Text)
			} else {
				contextBlock = fmt.Sprintf("<context>\n%s\n</context>", context.Text)
			}
			contextLines = append(contextLines, contextBlock)
		}
		messages = append(messages, llm.NewUserMessage(strings.Join(contextLines, "\n\n")))
	}

	var lines []string

	// Add task instructions
	lines = append(lines, "You must complete the following task:")

	if prompt.Text != "" {
		if prompt.Name != "" {
			lines = append(lines, fmt.Sprintf("<task name=%q>\n%s\n</task>", prompt.Name, prompt.Text))
		} else {
			lines = append(lines, fmt.Sprintf("<task>\n%s\n</task>", prompt.Text))
		}
	}

	// Add output expectations if specified
	if prompt.Output != "" {
		output := "Response requirements: " + prompt.Output
		if prompt.OutputFormat != "" {
			output += fmt.Sprintf("\n\nFormat your response in %s format.", prompt.OutputFormat)
		}
		lines = append(lines, output)
	}

	if len(lines) == 0 {
		return nil, fmt.Errorf("no instructions provided")
	}
	messages = append(messages, llm.NewUserMessage(strings.Join(lines, "\n\n")))
	return messages, nil
}
