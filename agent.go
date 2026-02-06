package dive

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

const (
	defaultResponseTimeout    = 30 * time.Minute
	defaultToolIterationLimit = 100
)

var (
	ErrLLMNoResponse = errors.New("llm did not return a response")
	ErrNoLLM         = errors.New("no llm provided")
)

// AgentOptions are used to configure an Agent.
type AgentOptions struct {
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
	// All hooks run in order. If any returns an error, the tool is denied.
	// If all return nil, the tool is executed.
	PreToolUse []PreToolUseHook

	// PostToolUse hooks are called after each tool execution.
	// Use these to modify tool results, log results, update metrics, or trigger side effects.
	// Hooks can modify the result before it's sent to the LLM.
	// Hook errors are logged but don't affect the tool result.
	PostToolUse []PostToolUseHook

	// Infrastructure
	Logger        llm.Logger
	ModelSettings *ModelSettings

	// LLMHooks are provider-level hooks passed to the LLM on each generation.
	// These are distinct from agent-level hooks (PreGeneration, PostGeneration,
	// PreToolUse, PostToolUse) which control the agent's generation loop.
	LLMHooks llm.Hooks

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
	llmHooks           llm.Hooks
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
		llmHooks:           opts.LLMHooks,
		logger:             opts.Logger,
		systemPrompt:       opts.SystemPrompt,
		modelSettings:      opts.ModelSettings,
		preGeneration:      opts.PreGeneration,
		postGeneration:     opts.PostGeneration,
		preToolUse:         opts.PreToolUse,
		postToolUse:        opts.PostToolUse,
	}
	tools := make([]Tool, len(opts.Tools))
	if len(opts.Tools) > 0 {
		copy(tools, opts.Tools)
	}
	agent.tools = tools
	if len(tools) > 0 {
		agent.toolsByName = make(map[string]Tool, len(tools))
		for _, tool := range tools {
			name := tool.Name()
			if _, exists := agent.toolsByName[name]; exists {
				return nil, fmt.Errorf("duplicate tool name: %q", name)
			}
			agent.toolsByName[name] = tool
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

// Tools returns a copy of the agent's tools.
func (a *Agent) Tools() []Tool {
	return slices.Clone(a.tools)
}

// Model returns the agent's LLM.
func (a *Agent) Model() llm.LLM {
	return a.model
}

func (a *Agent) CreateResponse(ctx context.Context, opts ...CreateResponseOption) (*Response, error) {
	var options CreateResponseOptions
	options.Apply(opts)

	logger := a.logger.With("agent_name", a.name)
	logger.Info("creating response")

	messages := a.prepareMessages(options)
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	systemPrompt := strings.TrimSpace(a.systemPrompt)

	// Initialize generation state for hooks
	genState := NewGenerationState()
	genState.SystemPrompt = systemPrompt
	genState.Messages = messages

	// Run PreGeneration hooks
	for _, hook := range a.preGeneration {
		if err := hook(ctx, genState); err != nil {
			logger.Error("pre-generation hook error", "error", err)
			return nil, fmt.Errorf("pre-generation hook error: %w", err)
		}
	}

	// Use potentially modified values from hooks
	systemPrompt = genState.SystemPrompt
	messages = genState.Messages

	logger.Debug("system prompt", "system_prompt", systemPrompt)

	var cancel context.CancelFunc
	if a.responseTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, a.responseTimeout)
		defer cancel()
	}

	response := &Response{
		Model:     a.model.Name(),
		CreatedAt: time.Now(),
	}

	eventCallback := func(ctx context.Context, item *ResponseItem) error {
		if options.EventCallback != nil {
			return options.EventCallback(ctx, item)
		}
		return nil
	}

	genResult, err := a.generate(ctx, messages, systemPrompt, eventCallback)
	if err != nil {
		logger.Error("failed to generate response", "error", err)
		return nil, err
	}

	response.FinishedAt = Ptr(time.Now())
	response.Usage = genResult.Usage
	response.Items = genResult.Items

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

// prepareMessages returns the messages from the provided options.
func (a *Agent) prepareMessages(options CreateResponseOptions) []*llm.Message {
	return options.Messages
}

// generate runs the LLM generation and tool execution loop. It handles the
// interaction between the agent and the LLM, including tool calls. Returns the
// final LLM response, updated messages, and any error that occurred.
func (a *Agent) generate(ctx context.Context, messages []*llm.Message, systemPrompt string, callback EventCallback) (*generateResult, error) {

	// Contains the message history we pass to the LLM
	updatedMessages := make([]*llm.Message, len(messages))
	copy(updatedMessages, messages)

	// New messages that are the output
	var outputMessages []*llm.Message

	// All response items in chronological order
	var items []*ResponseItem

	// Wrap callback to collect all items
	collectingCallback := func(ctx context.Context, item *ResponseItem) error {
		items = append(items, item)
		return callback(ctx, item)
	}

	// Accumulates usage across multiple LLM calls
	totalUsage := &llm.Usage{}

	newMessage := func(msg *llm.Message) {
		updatedMessages = append(updatedMessages, msg)
		outputMessages = append(outputMessages, msg)
	}

	// Base options passed to the LLM (built once, never mutated)
	baseOpts := a.getGenerationAgentOptions(systemPrompt)

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	generationLimit := a.toolIterationLimit + 1
	lastIteration := false
	for i := range generationLimit {
		// Build per-iteration options from base options + current messages
		iterOpts := append(slices.Clone(baseOpts), llm.WithMessages(updatedMessages...))
		if lastIteration {
			iterOpts = append(iterOpts, llm.WithToolChoice(llm.ToolChoiceNone))
		}
		var err error
		var response *llm.Response
		if streamingLLM, ok := a.model.(llm.StreamingLLM); ok {
			response, err = a.generateStreaming(ctx, streamingLLM, iterOpts, collectingCallback)
		} else {
			response, err = a.model.Generate(ctx, iterOpts...)
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
		if err := collectingCallback(ctx, &ResponseItem{
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
		toolResults, err := a.executeToolCalls(ctx, toolCalls, collectingCallback)
		if err != nil {
			return nil, err
		}

		// Capture results in a new message to send to LLM on the next iteration
		toolResultMessage := llm.NewToolResultMessage(getToolResultContent(toolResults)...)
		newMessage(toolResultMessage)

		// Add instructions to the message to not use any more tools if we have
		// only one generation left
		if i == generationLimit-2 {
			lastIteration = true
			toolResultMessage.Content = append(toolResultMessage.Content, &llm.TextContent{
				Text: "Your tool calls are complete. You must respond with a final answer now.",
			})
			a.logger.Debug("set tool choice to none", "agent", a.name, "generation_number", i+1)
		}
	}

	return &generateResult{
		OutputMessages: outputMessages,
		Items:          items,
		Usage:          totalUsage,
	}, nil
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
// PreToolUse hooks run in order for each call. If any hook returns an error,
// the tool is denied. If all hooks return nil, the tool is executed.
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

		// Run PreToolUse hooks — any error denies the tool
		var result *ToolCallResult
		hookCtx := &PreToolUseContext{
			Tool:  tool,
			Call:  toolCall,
			Agent: a,
		}
		denied := false
		for _, hook := range a.preToolUse {
			if err := hook(ctx, hookCtx); err != nil {
				var abortErr *HookAbortError
				if errors.As(err, &abortErr) {
					abortErr.HookType = "PreToolUse"
					a.logger.Error("pre-tool-use hook aborted", "error", abortErr)
					return nil, abortErr
				}
				a.logger.Debug("pre-tool-use hook denied tool", "error", err)
				result = a.createDeniedResult(toolCall, err.Error(), preview)
				denied = true
				break
			}
		}
		if !denied {
			result = a.executeTool(ctx, tool, toolCall, toolCall.Input, preview)
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
	if output == nil {
		output = &ToolResult{Content: []*ToolResultContent{}}
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
func (a *Agent) createDeniedResult(call *llm.ToolUseContent, message string, preview *ToolCallPreview) *ToolCallResult {
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
	if a.llmHooks != nil {
		generateOpts = append(generateOpts, llm.WithHooks(a.llmHooks))
	}
	if a.logger != nil {
		generateOpts = append(generateOpts, llm.WithLogger(a.logger))
	}
	generateOpts = append(generateOpts, a.modelSettings.Options()...)
	return generateOpts
}

type generateResult struct {
	OutputMessages []*llm.Message
	Items          []*ResponseItem
	Usage          *llm.Usage
}
