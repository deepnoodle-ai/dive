package dive

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

var (
	defaultResponseTimeout    = time.Minute * 30
	defaultToolIterationLimit = 100
	ErrSessionsAreNotEnabled  = errors.New("sessions are not enabled")
	ErrLLMNoResponse          = errors.New("llm did not return a response")
	ErrNoInstructions         = errors.New("no instructions provided")
	ErrNoLLM                  = errors.New("no llm provided")
)

// SetDefaultResponseTimeout sets the default response timeout for agents.
func SetDefaultResponseTimeout(timeout time.Duration) {
	defaultResponseTimeout = timeout
}

// SetDefaultToolIterationLimit sets the default tool iteration limit for agents.
func SetDefaultToolIterationLimit(limit int) {
	defaultToolIterationLimit = limit
}

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

// AgentOptions are used to configure an Agent.
type AgentOptions struct{
	// SystemPrompt is the system prompt sent to the LLM.
	SystemPrompt string

	// Model is the LLM to use for generation.
	Model llm.LLM

	// Tools available to the agent.
	Tools []Tool

	// PreGeneration hooks are called before the LLM generation loop.
	// Use these to load session history, inject context, or modify the system prompt.
	// If any hook returns an error, generation is aborted.
	PreGeneration []PreGenerationHook

	// PostGeneration hooks are called after the LLM generation loop completes.
	// Use these to save session history, log results, or trigger side effects.
	// Hook errors are logged but don't affect the returned Response.
	PostGeneration []PostGenerationHook

	// PreToolUse hooks are called before each tool execution.
	// Use these to implement permission checks, logging, or input modification.
	// Hooks run in order until one returns a non-Continue result.
	PreToolUse []PreToolUseHook

	// PostToolUse hooks are called after each tool execution.
	// Use these to modify tool results, log results, update metrics, or trigger side effects.
	// Hooks can modify the result before it's sent to the LLM.
	// Hook errors are logged but don't affect the tool result.
	PostToolUse []PostToolUseHook

	// Confirmer is called when a PreToolUse hook returns AskResult.
	// If nil, AskResult is treated as AllowResult.
	Confirmer ConfirmToolFunc

	// Infrastructure
	Logger        llm.Logger
	ModelSettings *ModelSettings
	Hooks         llm.Hooks // LLM-level hooks

	// Optional name for logging
	Name string

	// Timeouts and limits
	ResponseTimeout    time.Duration
	ToolIterationLimit int
}

// Agent represents an intelligent AI entity that can autonomously use tools to
// process information while responding to chat messages.
type Agent struct {
	name               string
	model              llm.LLM
	tools              []Tool
	toolsByName        map[string]Tool
	responseTimeout    time.Duration
	hooks              llm.Hooks
	logger             llm.Logger
	toolIterationLimit int
	modelSettings      *ModelSettings
	systemPrompt       string

	// Generation hooks
	preGeneration  []PreGenerationHook
	postGeneration []PostGenerationHook

	// Tool hooks
	preToolUse  []PreToolUseHook
	postToolUse []PostToolUseHook
	confirmer   ConfirmToolFunc
}

// NewAgent returns a new Agent configured with the given options.
func NewAgent(opts AgentOptions) (*Agent, error) {
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
		opts.Logger = &llm.NullLogger{}
	}
	agent := &Agent{
		name:               opts.Name,
		model:              opts.Model,
		responseTimeout:    opts.ResponseTimeout,
		toolIterationLimit: opts.ToolIterationLimit,
		hooks:              opts.Hooks,
		logger:             opts.Logger,
		systemPrompt:       opts.SystemPrompt,
		modelSettings:      opts.ModelSettings,
		preGeneration:      opts.PreGeneration,
		postGeneration:     opts.PostGeneration,
		preToolUse:         opts.PreToolUse,
		postToolUse:        opts.PostToolUse,
		confirmer:          opts.Confirmer,
	}
	tools := make([]Tool, len(opts.Tools))
	if len(opts.Tools) > 0 {
		copy(tools, opts.Tools)
	}
	agent.tools = tools
	if len(tools) > 0 {
		agent.toolsByName = make(map[string]Tool, len(tools))
		for _, tool := range tools {
			agent.toolsByName[tool.Name()] = tool
		}
	}
	return agent, nil
}

func (a *Agent) Name() string {
	return a.name
}

func (a *Agent) HasTools() bool {
	return len(a.tools) > 0
}

// Tools returns the agent's tools.
func (a *Agent) Tools() []Tool {
	return a.tools
}

// Model returns the agent's LLM.
func (a *Agent) Model() llm.LLM {
	return a.model
}

func (a *Agent) prepareSession(ctx context.Context, messages []*llm.Message, options CreateResponseOptions) *Session {
	return &Session{
		ID:       options.SessionID,
		Messages: messages,
	}
}

func (a *Agent) CreateResponse(ctx context.Context, opts ...CreateResponseOption) (*Response, error) {
	var options CreateResponseOptions
	options.Apply(opts)

	// Auto-generate session ID if not provided
	if options.SessionID == "" {
		options.SessionID = newSessionID()
	}

	logger := a.logger.With(
		"agent_name", a.name,
		"session_id", options.SessionID,
		"user_id", options.UserID,
	)
	logger.Info("creating response")

	messages := a.prepareMessages(options)
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	session := a.prepareSession(ctx, messages, options)

	systemPrompt := a.buildSystemPrompt()

	// Initialize generation state for hooks
	genState := NewGenerationState()
	genState.SessionID = session.ID
	genState.UserID = options.UserID
	genState.SystemPrompt = systemPrompt
	genState.Messages = session.Messages

	// Run PreGeneration hooks
	for _, hook := range a.preGeneration {
		if err := hook(ctx, genState); err != nil {
			logger.Error("pre-generation hook error", "error", err)
			return nil, fmt.Errorf("pre-generation hook error: %w", err)
		}
	}

	// Use potentially modified values from hooks
	systemPrompt = genState.SystemPrompt
	session.Messages = genState.Messages

	logger.Debug("system prompt", "system_prompt", systemPrompt)

	var cancel context.CancelFunc
	if a.responseTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, a.responseTimeout)
		defer cancel()
	}

	response := &Response{
		ID:        randomInt(),
		Model:     a.model.Name(),
		CreatedAt: time.Now(),
	}

	// Track whether we've emitted the init event
	initEventEmitted := false

	eventCallback := func(ctx context.Context, item *ResponseItem) error {
		if options.EventCallback != nil {
			// Emit init event before the first real event
			if !initEventEmitted {
				initEventEmitted = true
				initItem := &ResponseItem{
					Type: ResponseItemTypeInit,
					Init: &InitEvent{SessionID: session.ID},
				}
				if err := options.EventCallback(ctx, initItem); err != nil {
					return err
				}
			}
			return options.EventCallback(ctx, item)
		}
		return nil
	}

	genResult, err := a.generate(ctx, session.Messages, systemPrompt, eventCallback)
	if err != nil {
		logger.Error("failed to generate response", "error", err)
		return nil, err
	}

	session.Messages = append(session.Messages, genResult.OutputMessages...)

	response.FinishedAt = ptr(time.Now())
	response.Usage = genResult.Usage

	for _, msg := range genResult.OutputMessages {
		response.Items = append(response.Items, &ResponseItem{
			Type:    ResponseItemTypeMessage,
			Message: msg,
		})
	}

	// Run PostGeneration hooks
	genState.Response = response
	genState.OutputMessages = genResult.OutputMessages
	genState.Usage = genResult.Usage
	for _, hook := range a.postGeneration {
		if err := hook(ctx, genState); err != nil {
			// Check if this is a fatal abort error
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PostGeneration"
				logger.Error("post-generation hook aborted", "error", abortErr)
				return nil, abortErr
			}
			// Regular errors are logged but don't affect the response
			logger.Error("post-generation hook error", "error", err)
		}
	}

	return response, nil
}

// prepareMessages processes the ChatAgentOptions to create messages for the LLM.
// It handles both WithMessages and WithInput options.
func (a *Agent) prepareMessages(options CreateResponseOptions) []*llm.Message {
	return options.Messages
}

func (a *Agent) buildSystemPrompt() string {
	return strings.TrimSpace(a.systemPrompt)
}

// generate runs the LLM generation and tool execution loop. It handles the
// interaction between the agent and the LLM, including tool calls. Returns the
// final LLM response, updated messages, and any error that occurred.
func (a *Agent) generate(
	ctx context.Context,
	messages []*llm.Message,
	systemPrompt string,
	callback EventCallback,
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

	// AgentOptions passed to the LLM
	generateOpts := a.getGenerationAgentOptions(systemPrompt)

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
			"agent_name", a.name,
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

		// Always call callback for every LLM-generated message
		if err := callback(ctx, &ResponseItem{
			Type:    ResponseItemTypeMessage,
			Message: assistantMsg,
			Usage:   response.Usage.Copy(),
		}); err != nil {
			return nil, err
		}

		// Check for tool calls
		toolCalls := response.ToolCalls()
		if len(toolCalls) == 0 {
			break
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
	callback EventCallback,
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
		if err := callback(ctx, &ResponseItem{
			Type:  ResponseItemTypeModelEvent,
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

// executeToolCalls executes all tool calls and returns the tool call results.
// Hooks are evaluated in order: PreToolUse hooks determine whether to allow/deny/ask.
// If a hook returns AskResult and a confirmer is set, confirmation is requested.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	toolCalls []*llm.ToolUseContent,
	callback EventCallback,
) ([]*ToolCallResult, error) {
	results := make([]*ToolCallResult, len(toolCalls))
	for i, toolCall := range toolCalls {
		tool, ok := a.toolsByName[toolCall.Name]
		if !ok {
			return nil, fmt.Errorf("tool call error: unknown tool %q", toolCall.Name)
		}

		// Check if any tool (e.g., SkillTool) restricts this tool
		if !a.isToolAllowed(toolCall.Name) {
			results[i] = &ToolCallResult{
				ID:    toolCall.ID,
				Name:  toolCall.Name,
				Input: toolCall.Input,
				Result: &ToolResult{
					Content: []*ToolResultContent{
						{
							Type: ToolResultContentTypeText,
							Text: fmt.Sprintf("Tool %q is not allowed by the active skill. Check the skill's allowed-tools list.", toolCall.Name),
						},
					},
					IsError: true,
				},
			}
			if err := callback(ctx, &ResponseItem{
				Type:           ResponseItemTypeToolCallResult,
				ToolCallResult: results[i],
			}); err != nil {
				return nil, err
			}
			continue
		}

		a.logger.Debug("executing tool call",
			"tool_id", toolCall.ID,
			"tool_name", toolCall.Name,
			"tool_input", string(toolCall.Input))

		// Generate preview if tool supports it
		var preview *ToolCallPreview
		if previewer, ok := tool.(ToolPreviewer); ok {
			preview = previewer.PreviewCall(ctx, toolCall.Input)
		}

		// Emit tool call event
		if err := callback(ctx, &ResponseItem{
			Type:     ResponseItemTypeToolCall,
			ToolCall: toolCall,
		}); err != nil {
			return nil, err
		}

		// Evaluate PreToolUse hooks
		hookResult, err := a.evaluatePreToolUseHooks(ctx, tool, toolCall)
		if err != nil {
			// Fatal error - abort generation
			return nil, err
		}

		// Handle the hook result
		var result *ToolCallResult
		switch hookResult.Action {
		case ToolHookAllow:
			// Execute tool with potentially updated input
			input := toolCall.Input
			if hookResult.UpdatedInput != nil {
				input = hookResult.UpdatedInput
			}
			result = a.executeTool(ctx, tool, toolCall, input, preview)

		case ToolHookDeny:
			// Tool execution denied
			message := hookResult.Message
			if message == "" {
				message = "Tool execution denied"
			}
			result = a.createDeniedResult(toolCall, message, preview)

		case ToolHookAsk:
			// Prompt user for confirmation if confirmer is set
			result = a.handleToolConfirmation(ctx, tool, toolCall, hookResult.Message, preview)

		default:
			// ToolHookContinue - default to allow if no confirmer, ask otherwise
			if a.confirmer == nil {
				result = a.executeTool(ctx, tool, toolCall, toolCall.Input, preview)
			} else {
				result = a.handleToolConfirmation(ctx, tool, toolCall, "", preview)
			}
		}

		// Run PostToolUse hooks (before storing result)
		// Hooks can modify result before it's sent to the LLM
		postCtx := &PostToolUseContext{
			Tool:   tool,
			Call:   toolCall,
			Result: result,
			Agent:  a,
		}
		for _, hook := range a.postToolUse {
			if err := hook(ctx, postCtx); err != nil {
				// Check if this is a fatal abort error
				var abortErr *HookAbortError
				if errors.As(err, &abortErr) {
					abortErr.HookType = "PostToolUse"
					a.logger.Error("post-tool-use hook aborted", "error", abortErr)
					return nil, abortErr
				}
				// Regular errors are logged but don't affect the result
				a.logger.Debug("post-tool-use hook error", "error", err)
			}
		}

		// Use potentially modified result from hooks
		result = postCtx.Result
		results[i] = result

		// Emit result event
		if err := callback(ctx, &ResponseItem{
			Type:           ResponseItemTypeToolCallResult,
			ToolCallResult: result,
		}); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// evaluatePreToolUseHooks runs all PreToolUse hooks and returns the result.
// Hooks are evaluated in order until one returns a non-Continue result.
// Returns a ToolHookResult, or nil with error if generation should abort.
func (a *Agent) evaluatePreToolUseHooks(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error) {
	hookCtx := &PreToolUseContext{
		Tool:  tool,
		Call:  call,
		Agent: a,
	}
	for _, hook := range a.preToolUse {
		result, err := hook(ctx, hookCtx)
		if err != nil {
			// Check if this is a fatal abort error
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PreToolUse"
				a.logger.Error("pre-tool-use hook aborted", "error", abortErr)
				return nil, abortErr
			}
			// Regular errors are converted to Deny
			a.logger.Debug("pre-tool-use hook error", "error", err)
			return DenyResult(err.Error()), nil
		}
		if result != nil && result.Action != ToolHookContinue {
			return result, nil
		}
	}
	// All hooks returned Continue - default behavior
	return ContinueResult(), nil
}

// handleToolConfirmation prompts for user confirmation if a confirmer is set.
func (a *Agent) handleToolConfirmation(
	ctx context.Context,
	tool Tool,
	call *llm.ToolUseContent,
	message string,
	preview *ToolCallPreview,
) *ToolCallResult {
	// If no confirmer, auto-allow
	if a.confirmer == nil {
		return a.executeTool(ctx, tool, call, call.Input, preview)
	}

	// Use preview summary if no message provided
	if message == "" && preview != nil {
		message = preview.Summary
	}

	confirmed, err := a.confirmer(ctx, tool, call, message)
	if err != nil {
		// Check if this is user feedback (not a real error)
		if feedback, ok := IsUserFeedback(err); ok {
			return a.createDeniedResult(call, feedback, preview)
		}
		return a.createDeniedResult(call, fmt.Sprintf("Confirmation error: %v", err), preview)
	}
	if confirmed {
		return a.executeTool(ctx, tool, call, call.Input, preview)
	}
	return a.createDeniedResult(call, "User denied tool call", preview)
}

// executeTool runs the tool and returns the result.
func (a *Agent) executeTool(
	ctx context.Context,
	tool Tool,
	call *llm.ToolUseContent,
	input []byte,
	preview *ToolCallPreview,
) *ToolCallResult {
	output, err := tool.Call(ctx, input)
	if err != nil {
		return &ToolCallResult{
			ID:      call.ID,
			Name:    call.Name,
			Input:   call.Input,
			Preview: preview,
			Result: &ToolResult{
				Content: []*ToolResultContent{
					{
						Type: ToolResultContentTypeText,
						Text: fmt.Sprintf("Tool execution error: %v", err),
					},
				},
				IsError: true,
			},
			Error: err,
		}
	}
	return &ToolCallResult{
		ID:      call.ID,
		Name:    call.Name,
		Input:   call.Input,
		Preview: preview,
		Result:  output,
	}
}

// createDeniedResult creates a tool result for a denied tool call.
func (a *Agent) createDeniedResult(
	call *llm.ToolUseContent,
	message string,
	preview *ToolCallPreview,
) *ToolCallResult {
	return &ToolCallResult{
		ID:      call.ID,
		Name:    call.Name,
		Input:   call.Input,
		Preview: preview,
		Result: &ToolResult{
			Content: []*ToolResultContent{
				{
					Type: ToolResultContentTypeText,
					Text: message,
				},
			},
			IsError: true,
		},
	}
}

func (a *Agent) getToolDefinitions() []llm.Tool {
	definitions := make([]llm.Tool, len(a.tools))
	for i, tool := range a.tools {
		definitions[i] = tool
	}
	return definitions
}

func (a *Agent) getGenerationAgentOptions(systemPrompt string) []llm.Option {
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

// isToolAllowed checks if a tool is allowed by any ToolAllowanceChecker in the
// agent's tools. This is used to enforce skill-based tool restrictions.
func (a *Agent) isToolAllowed(toolName string) bool {
	for _, tool := range a.tools {
		if checker, ok := tool.(ToolAllowanceChecker); ok {
			if !checker.IsToolAllowed(toolName) {
				return false
			}
		}
	}
	return true
}

type generateResult struct {
	OutputMessages []*llm.Message
	Usage          *llm.Usage
}
