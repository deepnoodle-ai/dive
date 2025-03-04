package dive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/memory"
	"github.com/getstingrai/dive/slogger"
)

var (
	DefaultTaskTimeout        = time.Minute * 5
	DefaultChatTimeout        = time.Minute * 1
	DefaultTickFrequency      = time.Second * 1
	DefaultToolIterationLimit = 8
	DefaultLogger             = slogger.NewDevNullLogger()
)

// Confirm our standard implementation satisfies the different Agent interfaces
var (
	_ Agent             = &DiveAgent{}
	_ TeamAgent         = &DiveAgent{}
	_ RunnableAgent     = &DiveAgent{}
	_ EventHandlerAgent = &DiveAgent{}
)

// AgentOptions are used to configure an Agent.
type AgentOptions struct {
	Name               string
	Description        string
	Instructions       string
	AcceptedEvents     []string
	IsSupervisor       bool
	Subordinates       []string
	LLM                llm.LLM
	Tools              []llm.Tool
	TickFrequency      time.Duration
	TaskTimeout        time.Duration
	ChatTimeout        time.Duration
	CacheControl       string
	LogLevel           string
	Hooks              llm.Hooks
	Logger             slogger.Logger
	ToolIterationLimit int
	Memory             memory.Memory
}

// chatThread contains a message history for a thread ID
type chatThread struct {
	ID       string
	Messages []*llm.Message
}

// DiveAgent is the standard implementation of the Agent interface.
type DiveAgent struct {
	name               string
	description        string
	instructions       string
	llm                llm.LLM
	team               Team
	running            bool
	tools              []llm.Tool
	toolsByName        map[string]llm.Tool
	acceptedEvents     []string
	isSupervisor       bool
	subordinates       []string
	tickFrequency      time.Duration
	taskTimeout        time.Duration
	chatTimeout        time.Duration
	cacheControl       string
	taskQueue          []*taskState
	recentTasks        []*taskState
	activeTask         *taskState
	ticker             *time.Ticker
	logLevel           string
	hooks              llm.Hooks
	logger             slogger.Logger
	toolIterationLimit int
	memory             memory.Memory
	threads            map[string]*chatThread

	// Holds incoming messages to be processed by the agent's run loop
	mailbox chan interface{}

	mutex sync.Mutex
	wg    sync.WaitGroup
}

// NewAgent returns a new Agent configured with the given options.
func NewAgent(opts AgentOptions) *DiveAgent {
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
		opts.Logger = DefaultLogger
	}
	if opts.LLM == nil {
		if llm, ok := detectProvider(); ok {
			opts.LLM = llm
		} else {
			panic("no llm provided")
		}
	}
	if opts.Name == "" {
		if opts.Description != "" {
			opts.Name = opts.Description
		} else {
			opts.Name = randomName()
		}
	}
	agent := &DiveAgent{
		name:               opts.Name,
		llm:                opts.LLM,
		description:        opts.Description,
		instructions:       opts.Instructions,
		acceptedEvents:     opts.AcceptedEvents,
		isSupervisor:       opts.IsSupervisor,
		subordinates:       opts.Subordinates,
		tickFrequency:      opts.TickFrequency,
		taskTimeout:        opts.TaskTimeout,
		chatTimeout:        opts.ChatTimeout,
		toolIterationLimit: opts.ToolIterationLimit,
		cacheControl:       opts.CacheControl,
		hooks:              opts.Hooks,
		mailbox:            make(chan interface{}, 16),
		logger:             opts.Logger,
		logLevel:           strings.ToLower(opts.LogLevel),
		threads:            make(map[string]*chatThread),
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
	return agent
}

func (a *DiveAgent) Name() string {
	return a.name
}

func (a *DiveAgent) Description() string {
	return a.description
}

func (a *DiveAgent) Instructions() string {
	return a.instructions
}

func (a *DiveAgent) AcceptedEvents() []string {
	return a.acceptedEvents
}

func (a *DiveAgent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *DiveAgent) Subordinates() []string {
	if !a.isSupervisor || a.team == nil || len(a.team.Agents()) == 1 {
		return nil
	}
	if a.subordinates != nil {
		return a.subordinates
	}
	// If there are no other supervisors, assume we are the supervisor of all
	// agents in the team.
	var isAnotherSupervisor bool
	for _, agent := range a.team.Agents() {
		teamAgent, ok := agent.(TeamAgent)
		if ok && teamAgent.IsSupervisor() && teamAgent.Name() != a.name {
			isAnotherSupervisor = true
		}
	}
	if isAnotherSupervisor {
		return nil
	}
	var others []string
	for _, agent := range a.team.Agents() {
		if agent.Name() != a.name {
			others = append(others, agent.Name())
		}
	}
	a.subordinates = others
	return others
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

	a.logger.Debug("agent started",
		"name", a.name,
		"description", a.description,
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

// May need to be able to express whether this is the beginning of a conversation
// or a continuation. Pass a conversation ID? Multiple messages? What if two people
// are talking to the same agent?
func (a *DiveAgent) Generate(ctx context.Context, message *llm.Message, opts ...GenerateOption) (*llm.Response, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	var generateOptions generateOptions
	for _, opt := range opts {
		opt(&generateOptions)
	}

	resultChan := make(chan *llm.Response, 1)
	errChan := make(chan error, 1)

	chatMessage := messageChat{
		message:    message,
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

func (a *DiveAgent) Stream(ctx context.Context, message *llm.Message, opts ...GenerateOption) (Stream, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	var generateOptions generateOptions
	for _, opt := range opts {
		opt(&generateOptions)
	}

	stream := NewDiveStream()

	chatMessage := messageChat{
		message: message,
		options: generateOptions,
		stream:  stream,
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

func (a *DiveAgent) HandleEvent(ctx context.Context, event *Event) error {
	if !a.IsRunning() {
		return fmt.Errorf("agent is not running")
	}

	select {
	case a.mailbox <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *DiveAgent) Work(ctx context.Context, task *Task) (Stream, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	// Stream to be returned to the caller so it can wait for results async
	stream := NewDiveStream()

	message := messageWork{
		task:      task,
		publisher: NewStreamPublisher(stream),
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
func (a *DiveAgent) run() error {
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

			case *Event:
				a.handleEvent(msg)

			case messageStop:
				msg.done <- nil
				return nil
			}
		}
		// Make progress on any active tasks
		a.doSomeWork()
	}
}

func (a *DiveAgent) handleWork(m messageWork) {
	a.taskQueue = append(a.taskQueue, &taskState{
		Task:      m.task,
		Publisher: m.publisher,
		Status:    TaskStatusQueued,
	})
}

func (a *DiveAgent) handleEvent(event *Event) {
	a.logger.Info("event received",
		"agent", a.name,
		"event", event.Name)

	// TODO: implement event triggered behaviors
}

func (a *DiveAgent) handleChat(m messageChat) {
	ctx, cancel := context.WithTimeout(context.Background(), a.chatTimeout)
	defer cancel()

	systemPrompt, err := a.getSystemPromptForMode("chat")
	if err != nil {
		m.errChan <- err
		return
	}

	var isStreaming bool
	var publisher *StreamPublisher
	if m.stream != nil {
		isStreaming = true
		publisher = NewStreamPublisher(m.stream)
		defer publisher.Close()
	}

	logger := a.logger.With(
		"agent_name", a.name,
		"streaming", isStreaming,
		"thread_id", m.options.ThreadID,
		"user_id", m.options.UserID,
	)

	logger.Info("handling chat",
		"truncated_message", TruncateText(m.message.Text(), 10))

	// Append this new message to the thread history if a thread ID is provided
	var thread *chatThread
	var messages []*llm.Message
	if m.options.ThreadID != "" {
		var exists bool
		thread, exists = a.threads[m.options.ThreadID]
		if !exists {
			thread = &chatThread{
				ID:       m.options.ThreadID,
				Messages: []*llm.Message{},
			}
			a.threads[m.options.ThreadID] = thread
		}
		messages = append(messages, thread.Messages...)
	}
	messages = append(messages, m.message)

	response, updatedMessages, err := a.executeToolLoop(
		ctx,
		messages,
		systemPrompt,
		"chat",
		publisher,
	)
	if err != nil {
		logger.Error("error generating chat response", "error", err)
		// Intentional fall-through
	}
	if thread != nil {
		thread.Messages = updatedMessages
	}

	if isStreaming {
		return
	}
	if err != nil {
		m.errChan <- err
		return
	}
	m.resultChan <- response
	return
}

func (a *DiveAgent) getSystemPromptForMode(mode string) (string, error) {
	data := newAgentTemplateData(a)
	if mode == "chat" {
		return executeTemplate(chatSystemPromptTemplate, data)
	}
	return executeTemplate(agentSystemPromptTemplate, data)
}

// executeToolLoop runs the LLM generation and tool execution loop.
// It handles the interaction between the agent and the LLM, including tool calls.
// Returns the final LLM response and any error that occurred.
func (a *DiveAgent) executeToolLoop(
	ctx context.Context,
	messages []*llm.Message,
	systemPrompt string,
	taskName string,
	publisher *StreamPublisher,
) (*llm.Response, []*llm.Message, error) {

	// Holds the most recent response from the LLM
	var response *llm.Response
	updatedMessages := make([]*llm.Message, len(messages))
	copy(updatedMessages, messages)

	// Helper function to safely send events to the publisher
	safePublish := func(event *StreamEvent) error {
		if publisher == nil {
			return nil
		}
		return publisher.Send(ctx, event)
	}

	generationLimit := a.toolIterationLimit + 1

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	for i := 0; i < generationLimit; i++ {
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

		var currentResponse *llm.Response

		if streamingLLM, ok := a.llm.(llm.StreamingLLM); ok && false {
			stream, err := streamingLLM.Stream(ctx, updatedMessages, generateOpts...)
			if err != nil {
				return nil, updatedMessages, err
			}
			// Guarantee that Close is called. It's ok if this is redundant with
			// additional calls to Close below.
			defer stream.Close()

			for {
				event, ok := stream.Next(ctx)
				if !ok {
					if err := stream.Err(); err != nil {
						return nil, updatedMessages, err
					}
					break
				}
				if event.Response != nil {
					currentResponse = event.Response
				}
				err = safePublish(&StreamEvent{
					Type:      "llm.event",
					TaskName:  taskName,
					AgentName: a.name,
					LLMEvent:  event,
					Response:  currentResponse,
				})
				if err != nil {
					return nil, updatedMessages, err
				}
			}
			stream.Close()
		} else {
			var err error
			currentResponse, err = a.llm.Generate(ctx, updatedMessages, generateOpts...)
			if err != nil {
				return nil, updatedMessages, err
			}
		}

		if currentResponse == nil {
			// This indicates a bug in the LLM provider implementation
			return nil, updatedMessages, errors.New("no final response from llm provider")
		}
		response = currentResponse
		responseMessage := response.Message()

		a.logger.Info("llm response",
			"usage_input_tokens", response.Usage().InputTokens,
			"usage_output_tokens", response.Usage().OutputTokens,
			"cache_creation_input_tokens", response.Usage().CacheCreationInputTokens,
			"cache_read_input_tokens", response.Usage().CacheReadInputTokens,
			"truncated_response", TruncateText(responseMessage.Text(), 10),
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
				return nil, updatedMessages, fmt.Errorf("tool call for unknown tool: %q", toolCall.Name)
			}
			a.logger.Debug("tool call",
				"agent_name", a.name,
				"tool_name", toolCall.Name,
				"tool_input", toolCall.Input,
			)
			result, err := tool.Call(ctx, toolCall.Input)
			if err != nil {
				return nil, updatedMessages, fmt.Errorf("tool call error: %w", err)
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

		// Add instructions to the message to not use any more tools if we have
		// only one generation left.
		if i == generationLimit-2 {
			resultMessage.Content = append(resultMessage.Content, &llm.Content{
				Type: llm.ContentTypeText,
				Text: "Do not use any more tools. You must respond with your final answer now.",
			})
			a.logger.Debug("adding tool use limit instruction",
				"agent", a.name,
				"task", taskName,
				"generation_number", i+1,
			)
		}
		updatedMessages = append(updatedMessages, resultMessage)
	}

	return response, updatedMessages, nil
}

func (a *DiveAgent) handleTask(state *taskState) error {
	task := state.Task
	timeout := task.Timeout()
	if timeout == 0 {
		timeout = a.taskTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logger := a.logger.With(
		"agent_name", a.name,
		"task_name", task.Name(),
	)

	systemPrompt, err := a.getSystemPromptForMode("task")
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
		messages = append(messages, llm.NewUserMessage(task.Prompt()))
	} else if len(state.Messages) < 32 {
		// We're resuming a task and can still work some more
		messages = append(messages, state.Messages...)
		messages = append(messages, llm.NewUserMessage("Continue working on the task."))
	} else {
		// We're resuming a task but need to wrap it up
		messages = append(messages, state.Messages...)
		messages = append(messages, llm.NewUserMessage("Finish the task to the best of your ability now. Do not use any more tools. Respond with the complete response to the task's prompt."))
	}

	logger.Info("handling task",
		"status", state.Status,
		"truncated_description", TruncateText(task.Description(), 10),
		"truncated_prompt", TruncateText(task.Prompt(), 10),
	)

	// Execute the LLM generation and tool execution loop
	response, updatedMessages, err := a.executeToolLoop(
		ctx,
		messages,
		systemPrompt,
		task.Name(),
		state.Publisher,
	)
	if err != nil {
		return err
	}

	// Update task state based on the last response from the LLM. It should
	// contain thinking, primary output, then status. We could concatenate
	// the new output with prior output, but for now it seems like it's better
	// not to, and to request a full final response instead.
	taskResponse := ParseStructuredResponse(response.Message().Text())
	state.Output = taskResponse.Text
	state.Reasoning = taskResponse.Thinking
	state.StatusDescription = taskResponse.StatusDescription
	state.Messages = updatedMessages

	usage := response.Usage()
	state.Usage.InputTokens += usage.InputTokens
	state.Usage.OutputTokens += usage.OutputTokens
	state.Usage.CacheCreationInputTokens += usage.CacheCreationInputTokens
	state.Usage.CacheReadInputTokens += usage.CacheReadInputTokens

	// For now, if the status description is empty, let's assume it is complete.
	// We may need to make this configurable in the future.
	if taskResponse.StatusDescription == "" {
		state.Status = TaskStatusCompleted
		logger.Warn("defaulting to completed status")
	} else {
		state.Status = taskResponse.Status()
	}

	logger.Info("task updated",
		"status", state.Status,
		"status_description", state.StatusDescription,
		"truncated_output", TruncateText(state.Output, 10),
	)
	return nil
}

func (a *DiveAgent) doSomeWork() {

	publish := func(event *StreamEvent) {
		if a.activeTask.Publisher != nil {
			a.activeTask.Publisher.Send(context.Background(), event)
		}
	}

	// Activate the next task if there is one and we're idle
	if a.activeTask == nil && len(a.taskQueue) > 0 {
		// Pop and activate the first task in queue
		a.activeTask = a.taskQueue[0]
		a.taskQueue = a.taskQueue[1:]
		a.activeTask.Status = TaskStatusActive
		if !a.activeTask.Paused {
			a.activeTask.Started = time.Now()
		} else {
			a.activeTask.Paused = false
		}
		publish(&StreamEvent{
			Type:      "task.activated",
			TaskName:  a.activeTask.Task.Name(),
			AgentName: a.name,
		})
		a.logger.Debug("task activated",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"description", a.activeTask.Task.Description(),
		)
	}

	if a.activeTask == nil {
		return // Nothing to do!
	}
	taskName := a.activeTask.Task.Name()

	// Make progress on the active task
	err := a.handleTask(a.activeTask)

	// An error deactivates the task and pushes an error event on the stream
	if err != nil {
		a.activeTask.Status = TaskStatusError
		a.rememberTask(a.activeTask)
		publish(&StreamEvent{
			Type:      "task.error",
			TaskName:  taskName,
			AgentName: a.name,
			Error:     err.Error(),
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

	case TaskStatusCompleted:
		a.rememberTask(a.activeTask)
		a.logger.Debug("task completed",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
			"duration", time.Since(a.activeTask.Started).Seconds(),
		)
		publish(&StreamEvent{
			Type:      "task.result",
			TaskName:  taskName,
			AgentName: a.name,
			TaskResult: &TaskResult{
				Task:    a.activeTask.Task,
				Content: a.activeTask.Output,
				Usage:   a.activeTask.Usage,
			},
		})
		if a.activeTask.Publisher != nil {
			a.activeTask.Publisher.Close()
			a.activeTask.Publisher = nil
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
		publish(&StreamEvent{
			Type:      "task.progress",
			TaskName:  taskName,
			AgentName: a.name,
		})

	case TaskStatusPaused:
		// Set paused flag and return the task to the queue
		a.logger.Debug("task paused",
			"agent", a.name,
			"task", a.activeTask.Task.Name(),
		)
		publish(&StreamEvent{
			Type:      "task.paused",
			TaskName:  taskName,
			AgentName: a.name,
		})
		a.activeTask.Paused = true
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
		publish(&StreamEvent{
			Type:      "task.error",
			TaskName:  taskName,
			AgentName: a.name,
			Error:     fmt.Sprintf("task status: %s", a.activeTask.Status),
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

func (a *DiveAgent) Fingerprint() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("agent: %s\n", a.name))
	sb.WriteString(fmt.Sprintf("description: %s\n", a.description))
	sb.WriteString(fmt.Sprintf("instructions: %s\n", a.instructions))
	sb.WriteString(fmt.Sprintf("accepted_events: %v\n", a.acceptedEvents))
	sb.WriteString(fmt.Sprintf("is_supervisor: %t\n", a.isSupervisor))
	sb.WriteString(fmt.Sprintf("subordinates: %v\n", a.subordinates))
	sb.WriteString(fmt.Sprintf("llm: %s\n", a.llm.Name()))
	hash := sha256.New()
	hash.Write([]byte(sb.String()))
	return hex.EncodeToString(hash.Sum(nil))
}

// if !a.disablePrefill {
// 	prefill := "<think>"
// 	if i >= a.generationLimit-1 {
// 		prefill += "I must respond with my final answer now."
// 	}
// 	generateOpts = append(generateOpts, llm.WithPrefill(prefill, "</think>"))
// }
