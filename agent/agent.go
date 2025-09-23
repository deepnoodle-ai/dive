package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/log"
)

var (
	defaultResponseTimeout    = time.Minute * 10
	defaultToolIterationLimit = 16
	ErrThreadsAreNotEnabled   = errors.New("threads are not enabled")
	ErrLLMNoResponse          = errors.New("llm did not return a response")
	ErrNoInstructions         = errors.New("no instructions provided")
	ErrNoLLM                  = errors.New("no llm provided")
	FinishNow                 = "Do not use any more tools. You must respond with your final answer now."
)

// SetDefaultResponseTimeout sets the default response timeout for agents.
func SetDefaultResponseTimeout(timeout time.Duration) {
	defaultResponseTimeout = timeout
}

// SetDefaultToolIterationLimit sets the default tool iteration limit for agents.
func SetDefaultToolIterationLimit(limit int) {
	defaultToolIterationLimit = limit
}

// Confirm our standard implementation satisfies the dive.Agent interface.
var _ dive.Agent = &Agent{}

// ModelSettings are used to configure details of the LLM for an Agent.
type ModelSettings struct {
	Temperature       *float64
	PresencePenalty   *float64
	FrequencyPenalty  *float64
	ParallelToolCalls *bool
	Caching           *bool
	MaxTokens         *int
	ReasoningBudget   *int
	ReasoningEffort   llm.ReasoningEffort
	ToolChoice        *llm.ToolChoice
	Features          []string
	RequestHeaders    http.Header
	MCPServers        []llm.MCPServerConfig
}

// Options are used to configure an Agent.
type Options struct {
	ID                 string
	Name               string
	Goal               string
	Instructions       string
	IsSupervisor       bool
	Subordinates       []string
	Model              llm.LLM
	Tools              []dive.Tool
	ResponseTimeout    time.Duration
	Hooks              llm.Hooks
	Logger             log.Logger
	ToolIterationLimit int
	ModelSettings      *ModelSettings
	DateAwareness      *bool
	ThreadRepository   dive.ThreadRepository
	Confirmer          dive.Confirmer
	SystemPrompt       string
	NoSystemPrompt     bool
	Context            []llm.Content
}

// Agent is the standard implementation of the Agent interface.
type Agent struct {
	id                   string
	name                 string
	goal                 string
	instructions         string
	model                llm.LLM
	tools                []dive.Tool
	toolsByName          map[string]dive.Tool
	isSupervisor         bool
	subordinates         []string
	responseTimeout      time.Duration
	hooks                llm.Hooks
	logger               log.Logger
	toolIterationLimit   int
	modelSettings        *ModelSettings
	dateAwareness        *bool
	threadRepository     dive.ThreadRepository
	confirmer            dive.Confirmer
	systemPromptTemplate *template.Template
	context              []llm.Content
}

// New returns a new Agent configured with the given options.
func New(opts Options) (*Agent, error) {
	if opts.Model == nil {
		return nil, ErrNoLLM
	}
	if opts.ResponseTimeout <= 0 {
		opts.ResponseTimeout = defaultResponseTimeout
	}
	if opts.ToolIterationLimit <= 0 {
		opts.ToolIterationLimit = defaultToolIterationLimit
	}
	if opts.Logger == nil {
		opts.Logger = log.New(log.GetDefaultLevel())
	}
	if opts.ID == "" {
		opts.ID = dive.NewID()
	}
	var systemPromptTemplate *template.Template
	if !opts.NoSystemPrompt {
		if opts.SystemPrompt == "" {
			opts.SystemPrompt = defaultSystemPrompt
		}
		var err error
		systemPromptTemplate, err = parseTemplate("agent", opts.SystemPrompt)
		if err != nil {
			return nil, fmt.Errorf("invalid system prompt template: %w", err)
		}
	}
	agent := &Agent{
		id:                   opts.ID,
		name:                 opts.Name,
		goal:                 opts.Goal,
		instructions:         opts.Instructions,
		model:                opts.Model,
		isSupervisor:         opts.IsSupervisor,
		subordinates:         opts.Subordinates,
		responseTimeout:      opts.ResponseTimeout,
		toolIterationLimit:   opts.ToolIterationLimit,
		hooks:                opts.Hooks,
		logger:               opts.Logger,
		dateAwareness:        opts.DateAwareness,
		threadRepository:     opts.ThreadRepository,
		systemPromptTemplate: systemPromptTemplate,
		modelSettings:        opts.ModelSettings,
		confirmer:            opts.Confirmer,
		context:              opts.Context,
	}
	tools := make([]dive.Tool, len(opts.Tools))
	if len(opts.Tools) > 0 {
		copy(tools, opts.Tools)
	}
	agent.tools = tools
	if len(tools) > 0 {
		agent.toolsByName = make(map[string]dive.Tool, len(tools))
		for _, tool := range tools {
			agent.toolsByName[tool.Name()] = tool
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

func (a *Agent) Instructions() string {
	return a.instructions
}

func (a *Agent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *Agent) Subordinates() []string {
	if !a.isSupervisor {
		return nil
	}
	return a.subordinates
}

func (a *Agent) HasTools() bool {
	return len(a.tools) > 0
}

func (a *Agent) prepareThread(ctx context.Context, messages []*llm.Message, options dive.Options) (*dive.Thread, error) {
	thread, err := a.getOrCreateThread(ctx, options.ThreadID, options)
	if err != nil {
		return nil, err
	}
	thread.Messages = append(thread.Messages, messages...)
	return thread, nil
}

func (a *Agent) CreateResponse(ctx context.Context, opts ...dive.Option) (*dive.Response, error) {
	var chatOptions dive.Options
	chatOptions.Apply(opts)

	responseID := dive.NewID()

	logger := a.logger.With(
		"agent", a.name,
		"thread_id", chatOptions.ThreadID,
		"user_id", chatOptions.UserID,
		"response_id", responseID,
	)
	logger.Info("creating response")

	messages := a.prepareMessages(chatOptions)
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	thread, err := a.prepareThread(ctx, messages, chatOptions)
	if err != nil {
		return nil, err
	}

	systemPrompt, err := a.buildSystemPrompt()
	if err != nil {
		return nil, fmt.Errorf("failed to build system prompt: %w", err)
	}

	logger.Debug("system prompt", "system_prompt", systemPrompt)

	var cancel context.CancelFunc
	if a.responseTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, a.responseTimeout)
		defer cancel()
	}

	response := &dive.Response{
		ID:        responseID,
		Model:     a.model.Name(),
		CreatedAt: time.Now(),
	}

	eventCallback := func(ctx context.Context, item *dive.ResponseItem) error {
		if chatOptions.EventCallback != nil {
			return chatOptions.EventCallback(ctx, item)
		}
		return nil
	}

	genResult, err := a.generate(ctx, thread.Messages, systemPrompt, eventCallback)
	if err != nil {
		logger.Error("failed to generate response", "error", err)
		return nil, err
	}

	thread.Messages = append(thread.Messages, genResult.OutputMessages...)
	if a.threadRepository != nil {
		if err := a.threadRepository.PutThread(ctx, thread); err != nil {
			logger.Error("failed to save thread", "error", err)
			return nil, err
		}
	}

	response.FinishedAt = ptr(time.Now())
	response.Usage = genResult.Usage

	for _, msg := range genResult.OutputMessages {
		response.Items = append(response.Items, &dive.ResponseItem{
			Type:    dive.ResponseItemTypeMessage,
			Message: msg,
		})
	}
	return response, nil
}

// prepareMessages processes the ChatOptions to create messages for the LLM.
// It handles both WithMessages and WithInput options.
func (a *Agent) prepareMessages(options dive.Options) []*llm.Message {
	var messages []*llm.Message
	if len(a.context) > 0 {
		messages = append(messages, llm.NewUserMessage(a.context...))
	}
	if len(options.Messages) > 0 {
		messages = append(messages, options.Messages...)
	}
	return messages
}

func (a *Agent) getOrCreateThread(ctx context.Context, threadID string, options dive.Options) (*dive.Thread, error) {
	if a.threadRepository != nil {
		thread, err := a.threadRepository.GetThread(ctx, threadID)
		if err != nil {
			if err != dive.ErrThreadNotFound {
				return nil, err
			}
		} else {
			return thread, nil
		}
	}
	return &dive.Thread{
		ID:        threadID,
		UserID:    options.UserID,
		AgentID:   a.id,
		AgentName: a.name,
		Messages:  []*llm.Message{},
	}, nil
}

func (a *Agent) buildSystemPrompt() (string, error) {
	var prompt string
	if a.systemPromptTemplate != nil {
		var err error
		prompt, err = executeTemplate(a.systemPromptTemplate, a)
		if err != nil {
			return "", err
		}
		prompt = strings.TrimSpace(prompt)
	}
	if a.dateAwareness == nil || *a.dateAwareness {
		prompt = fmt.Sprintf("%s\n\n%s", prompt, dive.DateString(time.Now()))
	}
	return strings.TrimSpace(prompt), nil
}

// generate runs the LLM generation and tool execution loop. It handles the
// interaction between the agent and the LLM, including tool calls. Returns the
// final LLM response, updated messages, and any error that occurred.
func (a *Agent) generate(
	ctx context.Context,
	messages []*llm.Message,
	systemPrompt string,
	callback dive.EventCallback,
) (*generateResult, error) {

	// Contains the message history we pass to the LLM
	updatedMessages := make([]*llm.Message, len(messages))
	copy(updatedMessages, messages)

	// New messages that are the output
	var outputMessages []*llm.Message

	// Accumulates usage across multiple LLM calls
	totalUsage := &llm.Usage{}

	newMessage := func(msg *llm.Message) {
		updatedMessages = append(updatedMessages, msg)
		outputMessages = append(outputMessages, msg)
	}

	// Options passed to the LLM
	generateOpts := a.getGenerationOptions(systemPrompt)

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	generationLimit := a.toolIterationLimit + 1
	for i := range generationLimit {
		// Add cache control flag to messages if appropriate
		a.configureCacheControl(updatedMessages)

		// Generate a response in either streaming or non-streaming mode
		generateOpts = append(generateOpts, llm.WithMessages(updatedMessages...))
		var err error
		var response *llm.Response
		if streamingLLM, ok := a.model.(llm.StreamingLLM); ok {
			response, err = a.generateStreaming(ctx, streamingLLM, generateOpts, callback)
		} else {
			response, err = a.model.Generate(ctx, generateOpts...)
		}
		if err == nil && response == nil {
			// This indicates a bug in the LLM provider implementation
			err = ErrLLMNoResponse
		}
		if err != nil {
			return nil, err
		}

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
		assistantMsg := response.Message()
		newMessage(assistantMsg)

		// Track total token usage
		totalUsage.Add(&response.Usage)

		// We're done if there are no tool calls
		toolCalls := response.ToolCalls()
		if len(toolCalls) == 0 {
			break
		}

		if err := callback(ctx, &dive.ResponseItem{
			Type:    dive.ResponseItemTypeMessage,
			Message: assistantMsg,
			Usage:   response.Usage.Copy(),
		}); err != nil {
			return nil, err
		}

		// Execute all requested tool calls
		toolResults, err := a.executeToolCalls(ctx, toolCalls, callback)
		if err != nil {
			return nil, err
		}

		// Capture results in a new message to send to LLM on the next iteration
		toolResultMessage := llm.NewToolResultMessage(getToolResultContent(toolResults)...)
		newMessage(toolResultMessage)

		// Add instructions to the message to not use any more tools if we have
		// only one generation left
		if i == generationLimit-2 {
			generateOpts = append(generateOpts, llm.WithToolChoice(llm.ToolChoiceNone))
			toolResultMessage.Content = append(toolResultMessage.Content, &llm.TextContent{
				Text: "Your tool calls are complete. You must respond with a final answer now.",
			})
			a.logger.Debug("set tool choice to none",
				"agent", a.name,
				"generation_number", i+1,
			)
		}
	}

	return &generateResult{
		OutputMessages: outputMessages,
		Usage:          totalUsage,
	}, nil
}

func (a *Agent) isCachingEnabled() bool {
	if a.modelSettings == nil || a.modelSettings.Caching == nil {
		return true // default to caching enabled
	}
	return *a.modelSettings.Caching
}

func (a *Agent) configureCacheControl(messages []*llm.Message) {
	if !a.isCachingEnabled() || len(messages) == 0 {
		return
	}
	// Clear cache control from all messages
	for _, message := range messages {
		for _, content := range message.Content {
			if setter, ok := content.(llm.CacheControlSetter); ok {
				setter.SetCacheControl(nil)
			}
		}
	}
	// Add cache control to the last message
	lastMessage := messages[len(messages)-1]
	if contents := lastMessage.Content; len(contents) > 0 {
		if setter, ok := contents[len(contents)-1].(llm.CacheControlSetter); ok {
			setter.SetCacheControl(&llm.CacheControl{Type: llm.CacheControlTypeEphemeral})
		}
	}
}

// generateStreaming handles streaming generation with an LLM, including
// receiving and republishing events, and accumulating a complete response.
func (a *Agent) generateStreaming(
	ctx context.Context,
	streamingLLM llm.StreamingLLM,
	generateOpts []llm.Option,
	callback dive.EventCallback,
) (*llm.Response, error) {
	accum := llm.NewResponseAccumulator()
	iter, err := streamingLLM.Stream(ctx, generateOpts...)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.Next() {
		event := iter.Event()
		if err := accum.AddEvent(event); err != nil {
			return nil, err
		}
		if err := callback(ctx, &dive.ResponseItem{
			Type:  dive.ResponseItemTypeModelEvent,
			Event: event,
		}); err != nil {
			return nil, err
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return accum.Response(), nil
}

func (a *Agent) getConfirmer() (dive.Confirmer, bool) {
	if a.confirmer != nil {
		return a.confirmer, true
	}
	return nil, false
}

// executeToolCalls executes all tool calls and returns the tool call results.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	toolCalls []*llm.ToolUseContent,
	callback dive.EventCallback,
) ([]*dive.ToolCallResult, error) {
	results := make([]*dive.ToolCallResult, len(toolCalls))
	for i, toolCall := range toolCalls {
		tool, ok := a.toolsByName[toolCall.Name]
		if !ok {
			return nil, fmt.Errorf("tool call error: unknown tool %q", toolCall.Name)
		}
		a.logger.Debug("executing tool call",
			"tool_id", toolCall.ID,
			"tool_name", toolCall.Name,
			"tool_input", string(toolCall.Input))

		if err := callback(ctx, &dive.ResponseItem{
			Type:     dive.ResponseItemTypeToolCall,
			ToolCall: toolCall,
		}); err != nil {
			return nil, err
		}

		isConfirmed := true
		if confirmer, ok := a.getConfirmer(); ok {
			confirmed, err := confirmer.Confirm(ctx, a, tool, toolCall)
			if err != nil {
				return nil, fmt.Errorf("tool call confirmation error: %w", err)
			}
			if !confirmed {
				isConfirmed = false
			}
		}

		if isConfirmed {
			output, err := tool.Call(ctx, toolCall.Input)
			if err != nil {
				return nil, fmt.Errorf("tool call error: %w", err)
			}
			results[i] = &dive.ToolCallResult{
				ID:     toolCall.ID,
				Name:   toolCall.Name,
				Input:  toolCall.Input,
				Result: output,
				Error:  err,
			}
		} else {
			results[i] = &dive.ToolCallResult{
				ID:    toolCall.ID,
				Name:  toolCall.Name,
				Input: toolCall.Input,
				Result: &dive.ToolResult{
					Content: []*dive.ToolResultContent{
						{
							Type: dive.ToolResultContentTypeText,
							Text: "User denied tool call",
						},
					},
					IsError: true,
				},
			}
		}

		if err := callback(ctx, &dive.ResponseItem{
			Type:           dive.ResponseItemTypeToolCallResult,
			ToolCallResult: results[i],
		}); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (a *Agent) getToolDefinitions() []llm.Tool {
	definitions := make([]llm.Tool, len(a.tools))
	for i, tool := range a.tools {
		definitions[i] = tool
	}
	return definitions
}

func (a *Agent) getGenerationOptions(systemPrompt string) []llm.Option {
	var generateOpts []llm.Option
	if systemPrompt != "" {
		generateOpts = append(generateOpts, llm.WithSystemPrompt(systemPrompt))
	}
	if len(a.tools) > 0 {
		generateOpts = append(generateOpts, llm.WithTools(a.getToolDefinitions()...))
	}
	if a.hooks != nil {
		generateOpts = append(generateOpts, llm.WithHooks(a.hooks))
	}
	if a.logger != nil {
		generateOpts = append(generateOpts, llm.WithLogger(a.logger))
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
		if settings.MaxTokens != nil {
			generateOpts = append(generateOpts, llm.WithMaxTokens(*settings.MaxTokens))
		}
		if settings.ToolChoice != nil {
			generateOpts = append(generateOpts, llm.WithToolChoice(settings.ToolChoice))
		}
		if settings.ParallelToolCalls != nil {
			generateOpts = append(generateOpts, llm.WithParallelToolCalls(*settings.ParallelToolCalls))
		}
		if len(settings.Features) > 0 {
			generateOpts = append(generateOpts, llm.WithFeatures(settings.Features...))
		}
		if len(settings.RequestHeaders) > 0 {
			generateOpts = append(generateOpts, llm.WithRequestHeaders(settings.RequestHeaders))
		}
		if len(settings.MCPServers) > 0 {
			generateOpts = append(generateOpts, llm.WithMCPServers(settings.MCPServers...))
		}
	}
	return generateOpts
}

func (a *Agent) Context() []llm.Content {
	return a.context
}

type generateResult struct {
	OutputMessages []*llm.Message
	Usage          *llm.Usage
}
