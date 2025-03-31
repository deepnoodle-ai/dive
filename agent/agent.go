package agent

import (
	"context"
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
	DefaultTaskTimeout        = time.Minute * 20
	DefaultChatTimeout        = time.Minute * 4
	DefaultTickFrequency      = time.Second * 1
	DefaultToolIterationLimit = 8
	ErrThreadsAreNotEnabled   = errors.New("threads are not enabled")
	ErrLLMNoResponse          = errors.New("llm did not return a response")
	ErrNoInstructions         = errors.New("no instructions provided")
	ErrNoLLM                  = errors.New("no llm provided")
	FinishNow                 = "Do not use any more tools. You must respond with your final answer now."
)

// Confirm our standard implementation satisfies the different Agent interfaces
var (
	_ dive.Agent         = &Agent{}
	_ dive.RunnableAgent = &Agent{}
)

// ModelSettings are used to configure details of the LLM for an Agent.
type ModelSettings struct {
	Temperature       *float64
	PresencePenalty   *float64
	FrequencyPenalty  *float64
	ReasoningBudget   *int
	ReasoningEffort   string
	MaxTokens         int
	ToolChoice        llm.ToolChoice
	ParallelToolCalls *bool
}

// Options are used to configure an Agent.
type Options struct {
	Name                 string
	Goal                 string
	Backstory            string
	IsSupervisor         bool
	Subordinates         []string
	Model                llm.LLM
	Tools                []llm.Tool
	ToolChoice           llm.ToolChoice
	TickFrequency        time.Duration
	TaskTimeout          time.Duration
	ChatTimeout          time.Duration
	Caching              *bool
	Hooks                llm.Hooks
	Logger               slogger.Logger
	ToolIterationLimit   int
	ModelSettings        *ModelSettings
	DateAwareness        *bool
	Environment          dive.Environment
	DocumentRepository   dive.DocumentRepository
	ThreadRepository     dive.ThreadRepository
	SystemPromptTemplate string
	AutoStart            bool
}

// Agent is the standard implementation of the Agent interface.
type Agent struct {
	name                 string
	goal                 string
	backstory            string
	model                llm.LLM
	running              bool
	tools                []llm.Tool
	toolsByName          map[string]llm.Tool
	toolChoice           llm.ToolChoice
	isSupervisor         bool
	subordinates         []string
	tickFrequency        time.Duration
	taskTimeout          time.Duration
	chatTimeout          time.Duration
	caching              *bool
	taskQueue            []*taskState
	recentTasks          []*taskState
	activeTask           *taskState
	ticker               *time.Ticker
	hooks                llm.Hooks
	logger               slogger.Logger
	toolIterationLimit   int
	modelSettings        *ModelSettings
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
	if opts.Model == nil {
		if llm, ok := detectProvider(); ok {
			opts.Model = llm
		} else {
			return nil, ErrNoLLM
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
		model:                opts.Model,
		environment:          opts.Environment,
		isSupervisor:         opts.IsSupervisor,
		subordinates:         opts.Subordinates,
		tickFrequency:        opts.TickFrequency,
		taskTimeout:          opts.TaskTimeout,
		chatTimeout:          opts.ChatTimeout,
		toolIterationLimit:   opts.ToolIterationLimit,
		caching:              opts.Caching,
		hooks:                opts.Hooks,
		mailbox:              make(chan interface{}, 16),
		logger:               opts.Logger,
		dateAwareness:        opts.DateAwareness,
		documentRepository:   opts.DocumentRepository,
		threadRepository:     opts.ThreadRepository,
		systemPromptTemplate: systemPromptTemplate,
		modelSettings:        opts.ModelSettings,
		toolChoice:           opts.ToolChoice,
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

	if opts.AutoStart {
		if err := agent.Start(context.Background()); err != nil {
			return nil, fmt.Errorf("failed to start agent: %w", err)
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

func (a *Agent) SetEnvironment(env dive.Environment) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return fmt.Errorf("agent is already running")
	}
	if a.environment != nil {
		return fmt.Errorf("agent is already associated with an environment")
	}
	a.environment = env
	return nil
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
		"is_supervisor", a.isSupervisor,
		"subordinates", a.subordinates,
		"task_timeout", a.taskTimeout,
		"chat_timeout", a.chatTimeout,
		"tick_frequency", a.tickFrequency,
		"tool_iteration_limit", a.toolIterationLimit,
		"model", a.model.Name(),
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

func (a *Agent) Chat(ctx context.Context, messages []*llm.Message, opts ...dive.ChatOption) (dive.EventStream, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	var chatOptions dive.ChatOptions
	chatOptions.Apply(opts)

	stream, publisher := dive.NewEventStream()

	chatMessage := messageChat{
		messages:  messages,
		options:   chatOptions,
		publisher: publisher,
	}

	select {
	case a.mailbox <- chatMessage:
		return stream, nil
	case <-ctx.Done():
		stream.Close()
		return nil, ctx.Err()
	}
}

func (a *Agent) Work(ctx context.Context, task dive.Task) (dive.EventStream, error) {
	if !a.IsRunning() {
		return nil, fmt.Errorf("agent is not running")
	}

	stream, publisher := dive.NewEventStream()

	message := messageWork{
		task:      task,
		publisher: publisher,
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

	logger := a.logger.With(
		"agent", a.name,
		"thread_id", m.options.ThreadID,
		"user_id", m.options.UserID,
	)
	logger.Info("handling chat")

	publisher := m.publisher
	defer publisher.Close()

	// Build the system prompt for a chat
	systemPrompt, err := a.buildSystemPrompt("chat")
	if err != nil {
		publisher.Send(ctx, a.errorEvent(err))
		return
	}

	// Append the new messages to the thread history if there is a thread ID
	var thread *dive.Thread
	var threadMessages []*llm.Message
	if m.options.ThreadID != "" {
		if a.threadRepository == nil {
			logger.Error("threads are not enabled")
			publisher.Send(ctx, a.errorEvent(ErrThreadsAreNotEnabled))
			return
		}
		var err error
		thread, err = a.getOrCreateThread(ctx, m.options.ThreadID)
		if err != nil {
			logger.Error("error retrieving thread", "error", err)
			publisher.Send(ctx, a.errorEvent(err))
			return
		}
		if len(thread.Messages) > 0 {
			threadMessages = append(threadMessages, thread.Messages...)
		}
	}
	threadMessages = append(threadMessages, m.messages...)

	// Generate the response
	_, updatedMessages, err := a.generate(ctx, threadMessages, systemPrompt, publisher)
	if err != nil {
		logger.Error("error generating response", "error", err)
		publisher.Send(ctx, a.errorEvent(err))
		return
	}

	// Save the updated thread messages
	if thread != nil {
		thread.Messages = updatedMessages
		if err := a.threadRepository.PutThread(ctx, thread); err != nil {
			logger.Error("error saving thread", "error", err)
			publisher.Send(ctx, a.errorEvent(err))
			return
		}
	}
}

func (a *Agent) getOrCreateThread(ctx context.Context, threadID string) (*dive.Thread, error) {
	if a.threadRepository == nil {
		return nil, ErrThreadsAreNotEnabled
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

func (a *Agent) buildSystemPrompt(mode string) (string, error) {
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
	publisher dive.EventPublisher,
) (*llm.Response, []*llm.Message, error) {

	// Holds the most recent response from the LLM
	var response *llm.Response

	// Contains the message history. We'll append to this and return it when done.
	updatedMessages := make([]*llm.Message, len(messages))
	copy(updatedMessages, messages)

	// Options passed to the LLM
	generateOpts := a.getGenerationOptions(systemPrompt)

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	generationLimit := a.toolIterationLimit + 1
	for i := range generationLimit {

		// Publish an event indicating that a request is starting
		publisher.Send(ctx, &dive.Event{
			Type:    dive.EventTypeLLMRequest,
			Origin:  a.eventOrigin(),
			Payload: &llm.Request{Messages: updatedMessages},
		})

		// Generate a response in either streaming or non-streaming mode
		var err error
		if streamingLLM, ok := a.model.(llm.StreamingLLM); ok {
			response, err = a.generateStreaming(ctx, streamingLLM, updatedMessages, generateOpts, publisher)
		} else {
			response, err = a.model.Generate(ctx, updatedMessages, generateOpts...)
		}
		if err == nil && response == nil {
			// This indicates a bug in the LLM provider implementation
			err = ErrLLMNoResponse
		}
		if err != nil {
			publisher.Send(ctx, &dive.Event{
				Type:    dive.EventTypeError,
				Origin:  a.eventOrigin(),
				Payload: err,
			})
			return nil, nil, err
		}

		// Publish an event indicating that the response was fully generated
		publisher.Send(ctx, &dive.Event{
			Type:    dive.EventTypeLLMResponse,
			Origin:  a.eventOrigin(),
			Payload: response,
		})

		a.logger.Debug("llm response",
			"agent", a.name,
			"usage_input_tokens", response.Usage.InputTokens,
			"usage_output_tokens", response.Usage.OutputTokens,
			"cache_creation_input_tokens", response.Usage.CacheCreationInputTokens,
			"cache_read_input_tokens", response.Usage.CacheReadInputTokens,
			"response_text", response.Message().Text(),
			"generation_number", i+1,
		)

		// Remember the assistant response message
		updatedMessages = append(updatedMessages, response.Message())

		// We're done if there are no tool calls
		if len(response.ToolCalls()) == 0 {
			break
		}

		// Execute all requested tool calls
		toolResults, err := a.executeToolCalls(ctx, response.ToolCalls(), publisher)
		if err != nil {
			return nil, nil, err
		}

		// We're done if the results don't need to be provided to the LLM
		if len(toolResults) == 0 {
			break
		}

		// Capture results in a new message to send to LLM on the next iteration
		resultMessage := llm.NewToolOutputMessage(toolResults)

		// Add instructions to the message to not use any more tools if we have
		// only one generation left
		if i == generationLimit-2 {
			generateOpts = append(generateOpts, llm.WithToolChoice(llm.ToolChoiceNone))
			resultMessage.Content = append(resultMessage.Content, &llm.Content{
				Type: llm.ContentTypeText,
				Text: FinishNow,
			})
			a.logger.Debug("added finish now statement",
				"agent", a.name,
				"generation_number", i+1,
			)
		}

		// Messages to be sent to the LLM on the next iteration
		updatedMessages = append(updatedMessages, resultMessage)
	}

	return response, updatedMessages, nil
}

// generateStreaming handles streaming generation with an LLM, including
// receiving and republishing events, and accumulating a complete response.
func (a *Agent) generateStreaming(
	ctx context.Context,
	streamingLLM llm.StreamingLLM,
	messages []*llm.Message,
	generateOpts []llm.Option,
	publisher dive.EventPublisher,
) (*llm.Response, error) {
	accum := llm.NewResponseAccumulator()
	iter, err := streamingLLM.Stream(ctx, messages, generateOpts...)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	for iter.Next() {
		event := iter.Event()
		if err := accum.AddEvent(event); err != nil {
			return nil, err
		}
		publisher.Send(ctx, &dive.Event{
			Type:    dive.EventTypeLLMEvent,
			Origin:  a.eventOrigin(),
			Payload: event,
		})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return accum.Response(), nil
}

// executeToolCalls executes all tool calls and returns the results. If the
// tools are configured to not return results, (nil, nil) is returned.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	toolCalls []*llm.ToolCall,
	publisher dive.EventPublisher,
) ([]*llm.ToolOutput, error) {
	outputs := make([]*llm.ToolOutput, len(toolCalls))
	shouldReturnResult := false
	for i, toolCall := range toolCalls {
		tool, ok := a.toolsByName[toolCall.Name]
		if !ok {
			return nil, fmt.Errorf("tool call error: unknown tool %q", toolCall.Name)
		}
		a.logger.Debug("executing tool call",
			"tool_id", toolCall.ID,
			"tool_name", toolCall.Name,
			"tool_input", toolCall.Input)
		publisher.Send(ctx, &dive.Event{
			Type:    dive.EventTypeToolCalled,
			Origin:  a.eventOrigin(),
			Payload: toolCall,
		})
		toolOutput, err := tool.Call(ctx, toolCall.Input)
		if err != nil {
			publisher.Send(ctx, &dive.Event{
				Type:   dive.EventTypeToolError,
				Origin: a.eventOrigin(),
				Payload: &llm.ToolError{
					ID:    toolCall.ID,
					Name:  toolCall.Name,
					Error: err.Error(),
				},
			})
			return nil, fmt.Errorf("tool call error: %w", err)
		}
		outputs[i] = &llm.ToolOutput{
			ID:     toolCall.ID,
			Name:   toolCall.Name,
			Output: toolOutput,
		}
		publisher.Send(ctx, &dive.Event{
			Type:    dive.EventTypeToolOutput,
			Origin:  a.eventOrigin(),
			Payload: outputs[i],
		})
		if tool.ShouldReturnResult() {
			shouldReturnResult = true
		}
	}
	if shouldReturnResult {
		return outputs, nil
	}
	return nil, nil
}

func (a *Agent) getGenerationOptions(systemPrompt string) []llm.Option {
	var generateOpts []llm.Option
	if systemPrompt != "" {
		generateOpts = append(generateOpts, llm.WithSystemPrompt(systemPrompt))
	}
	if len(a.tools) > 0 {
		generateOpts = append(generateOpts, llm.WithTools(a.tools...))
	}
	if a.toolChoice != "" {
		generateOpts = append(generateOpts, llm.WithToolChoice(a.toolChoice))
	}
	if a.hooks != nil {
		generateOpts = append(generateOpts, llm.WithHooks(a.hooks))
	}
	if a.logger != nil {
		generateOpts = append(generateOpts, llm.WithLogger(a.logger))
	}
	if a.caching != nil {
		generateOpts = append(generateOpts, llm.WithCaching(*a.caching))
	} else {
		// Caching defaults to on
		generateOpts = append(generateOpts, llm.WithCaching(true))
	}
	if a.modelSettings != nil {
		settings := a.modelSettings
		if settings.Temperature != nil {
			generateOpts = append(generateOpts, llm.WithTemperature(*settings.Temperature))
		}
		if settings.PresencePenalty != nil {
			generateOpts = append(generateOpts, llm.WithPresencePenalty(*settings.PresencePenalty))
		}
		if settings.FrequencyPenalty != nil {
			generateOpts = append(generateOpts, llm.WithFrequencyPenalty(*settings.FrequencyPenalty))
		}
		if settings.ReasoningBudget != nil {
			generateOpts = append(generateOpts, llm.WithReasoningBudget(*settings.ReasoningBudget))
		}
		if settings.ReasoningEffort != "" {
			generateOpts = append(generateOpts, llm.WithReasoningEffort(settings.ReasoningEffort))
		}
		if settings.MaxTokens != 0 {
			generateOpts = append(generateOpts, llm.WithMaxTokens(settings.MaxTokens))
		}
		if settings.ToolChoice != "" {
			generateOpts = append(generateOpts, llm.WithToolChoice(settings.ToolChoice))
		}
		if settings.ParallelToolCalls != nil {
			generateOpts = append(generateOpts, llm.WithParallelToolCalls(*settings.ParallelToolCalls))
		}
	}
	return generateOpts
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
	// Chat timeout is used to limit the time spent on one generation
	if a.chatTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.chatTimeout)
		defer cancel()
	}
	// Check the task timeout, which is tracked since the task was started
	if a.taskTimeout > 0 {
		since := time.Since(state.Started)
		if since > a.taskTimeout {
			return fmt.Errorf("task timed out after %s", since)
		}
	}
	systemPrompt, err := a.buildSystemPrompt("task")
	if err != nil {
		return err
	}
	var messages, truncatedMessages llm.Messages

	if len(state.Messages) == 0 {
		// Starting a task
		if recentTasksMessage, ok := a.getTasksHistoryMessage(); ok {
			messages = append(messages, recentTasksMessage)
		}
		prompt, err := state.Task.Prompt()
		if err != nil {
			return err
		}
		promptMessages, err := taskPromptMessages(prompt)
		if err != nil {
			return err
		}
		messages = append(messages, promptMessages...)
	} else {
		// Resuming a task. Pass up to the last 32 messages to the LLM.
		messages = append(messages, state.Messages...)
		messages = append(messages, llm.NewUserMessage(PromptContinue))
		if len(messages) > 32 {
			truncatedMessages = messages[len(messages)-32:]
		}
	}

	messagesToSend := messages
	if len(truncatedMessages) > 0 {
		messagesToSend = truncatedMessages
	}
	response, updatedMessages, err := a.generate(ctx, messagesToSend, systemPrompt, state.Publisher)
	if err != nil {
		return err
	}

	// If we truncated messages, we need to reassemble the complete message list
	if len(truncatedMessages) > 0 {
		// Messages before truncation point
		completeMessages := messages[:len(messages)-len(truncatedMessages)]
		// Add all messages after truncation point
		completeMessages = append(completeMessages, updatedMessages...)
		updatedMessages = completeMessages
	}

	state.TrackResponse(response, updatedMessages)
	return nil
}

func (a *Agent) environmentName() string {
	if a.environment == nil {
		return ""
	}
	return a.environment.Name()
}

func (a *Agent) doSomeWork() {
	ctx := context.Background()
	logger := a.logger.With("agent", a.name)

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
		a.activeTask.Publisher.Send(ctx, &dive.Event{
			Type: dive.EventTypeTaskActivated,
			Origin: dive.EventOrigin{
				AgentName:       a.name,
				EnvironmentName: a.environmentName(),
				TaskName:        a.activeTask.Task.Name(),
			},
		})
		logger.Info("task started", "task", a.activeTask.Task.Name())
	}

	// Return if there's nothing to do
	if a.activeTask == nil {
		return
	}

	// Make progress on the active task
	taskState := a.activeTask
	taskName := taskState.Task.Name()
	err := a.handleTask(context.Background(), taskState)

	// An error deactivates the task and pushes an error event on the stream
	if err != nil {
		taskState.Status = dive.TaskStatusError
		a.rememberTask(taskState)
		taskState.Publisher.Send(ctx, &dive.Event{
			Type: dive.EventTypeTaskError,
			Origin: dive.EventOrigin{
				AgentName:       a.name,
				EnvironmentName: a.environmentName(),
				TaskName:        taskName,
			},
			Error: err,
		})
		logger.Error("task error", "task", taskName, "error", err)
		taskState.Publisher.Close()
		taskState.Publisher = nil
		a.activeTask = nil
		return
	}

	// Handle task state transitions
	switch taskState.Status {

	case dive.TaskStatusActive:
		// The task will remain active so that the agent can continue working
		logger.Info("task progress",
			"task", taskName,
			"status_description", taskState.StatusDescription,
		)
		taskState.Publisher.Send(ctx, &dive.Event{
			Type: dive.EventTypeTaskProgress,
			Origin: dive.EventOrigin{
				AgentName:       a.name,
				EnvironmentName: a.environmentName(),
				TaskName:        taskName,
			},
		})

	case dive.TaskStatusCompleted:
		// The task is now finished, so clear the active task
		a.activeTask = nil
		a.rememberTask(taskState)
		logger.Info("task completed", "task", taskName)
		taskState.Publisher.Send(ctx, &dive.Event{
			Type: dive.EventTypeTaskCompleted,
			Origin: dive.EventOrigin{
				AgentName:       a.name,
				EnvironmentName: a.environmentName(),
				TaskName:        taskName,
			},
			Payload: &dive.TaskResult{
				Task:    taskState.Task,
				Usage:   taskState.Usage,
				Content: taskState.LastOutput(),
			},
		})
		taskState.Publisher.Close()
		taskState.Publisher = nil

	case dive.TaskStatusPaused:
		// The task is now paused, so return it to the task queue
		a.activeTask = nil
		logger.Info("task paused", "task", taskName)
		taskState.Paused = true
		taskState.Publisher.Send(ctx, &dive.Event{
			Type: dive.EventTypeTaskPaused,
			Origin: dive.EventOrigin{
				AgentName:       a.name,
				EnvironmentName: a.environmentName(),
				TaskName:        taskName,
			},
		})
		a.taskQueue = append(a.taskQueue, taskState)

	case dive.TaskStatusBlocked, dive.TaskStatusError, dive.TaskStatusInvalid:
		// The task failed, so clear the active task
		a.activeTask = nil
		logger.Warn("task error",
			"task", taskName,
			"status", taskState.Status,
			"status_description", taskState.StatusDescription,
		)
		taskState.Publisher.Send(ctx, &dive.Event{
			Type: dive.EventTypeTaskError,
			Origin: dive.EventOrigin{
				AgentName:       a.name,
				EnvironmentName: a.environmentName(),
				TaskName:        taskName,
			},
			Error: fmt.Errorf("task status: %s", taskState.Status),
		})
		taskState.Publisher.Close()
		taskState.Publisher = nil
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

func (a *Agent) eventOrigin() dive.EventOrigin {
	var environmentName string
	if a.environment != nil {
		environmentName = a.environment.Name()
	}
	return dive.EventOrigin{
		AgentName:       a.name,
		EnvironmentName: environmentName,
	}
}

func (a *Agent) errorEvent(err error) *dive.Event {
	return &dive.Event{
		Type:   dive.EventTypeError,
		Error:  err,
		Origin: a.eventOrigin(),
	}
}

func taskPromptMessages(prompt *dive.Prompt) ([]*llm.Message, error) {
	messages := []*llm.Message{}

	if prompt.Text == "" {
		return nil, ErrNoInstructions
	}

	// Add context information if available
	if len(prompt.Context) > 0 {
		contextLines := []string{
			"Important: The following context may contain relevant information to help you complete the task.",
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
	if prompt.Name != "" {
		lines = append(lines, fmt.Sprintf("<task name=%q>\n%s\n</task>", prompt.Name, prompt.Text))
	} else {
		lines = append(lines, fmt.Sprintf("<task>\n%s\n</task>", prompt.Text))
	}

	// Add output expectations if specified
	if prompt.Output != "" {
		output := "Response requirements: " + prompt.Output
		if prompt.OutputFormat != "" {
			output += fmt.Sprintf("\n\nFormat your response in %s format.", prompt.OutputFormat)
		}
		lines = append(lines, output)
	}

	messages = append(messages, llm.NewUserMessage(strings.Join(lines, "\n\n")))
	return messages, nil
}
