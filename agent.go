package dive

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
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

// Hooks groups all agent hook slices.
type Hooks struct {
	// PreGeneration hooks are called before the LLM generation loop.
	PreGeneration []PreGenerationHook

	// PostGeneration hooks are called after the LLM generation loop completes.
	PostGeneration []PostGenerationHook

	// PreToolUse hooks are called before each tool execution.
	PreToolUse []PreToolUseHook

	// PostToolUse hooks are called after each successful tool execution.
	PostToolUse []PostToolUseHook

	// PostToolUseFailure hooks are called after each failed tool execution.
	PostToolUseFailure []PostToolUseFailureHook

	// Stop hooks run when the agent is about to finish responding.
	// A hook can prevent stopping by returning a StopDecision with Continue: true.
	Stop []StopHook

	// PreIteration hooks run before each LLM call within the generation loop.
	PreIteration []PreIterationHook
}

// AgentOptions are used to configure an Agent.
type AgentOptions struct {
	// SystemPrompt is the system prompt sent to the LLM.
	SystemPrompt string

	// Model is the LLM to use for generation.
	Model llm.LLM

	// Tools available to the agent (static).
	Tools []Tool

	// Toolsets provide dynamic tool resolution. Each toolset's Tools() method
	// is called before each LLM request, enabling context-dependent tool
	// availability. Tools from toolsets are merged with static Tools.
	Toolsets []Toolset

	// Hooks groups all agent-level hooks.
	Hooks Hooks

	// Infrastructure
	Logger        llm.Logger
	ModelSettings *ModelSettings

	// LLMHooks are provider-level hooks passed to the LLM on each generation.
	// These are distinct from agent-level hooks which control the agent's
	// generation loop.
	LLMHooks llm.Hooks

	// Optional name for logging
	Name string

	// Session enables persistent conversation state. When set, the agent
	// automatically loads history before generation and saves new messages
	// after generation. Can be overridden per-call with WithSession.
	Session Session

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
	toolsets           []Toolset
	toolsByName        map[string]Tool
	responseTimeout    time.Duration
	llmHooks           llm.Hooks
	logger             llm.Logger
	toolIterationLimit int
	modelSettings      *ModelSettings
	systemPrompt       string
	session            Session

	// Agent hooks
	hooks Hooks
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
		hooks:              opts.Hooks,
		session:            opts.Session,
		toolsets:           opts.Toolsets,
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
	return len(a.tools) > 0 || len(a.toolsets) > 0
}

// Tools returns a copy of the agent's static tools.
func (a *Agent) Tools() []Tool {
	return slices.Clone(a.tools)
}

// resolveTools returns all tools for the current request, including static tools
// and dynamically resolved tools from toolsets.
func (a *Agent) resolveTools(ctx context.Context) (tools []Tool, toolsByName map[string]Tool, err error) {
	tools = slices.Clone(a.tools)

	// Resolve dynamic tools from toolsets
	for _, ts := range a.toolsets {
		dynamic, tsErr := ts.Tools(ctx)
		if tsErr != nil {
			return nil, nil, fmt.Errorf("toolset %s: %w", ts.Name(), tsErr)
		}
		tools = append(tools, dynamic...)
	}

	// Build name index
	toolsByName = make(map[string]Tool, len(tools))
	for _, tool := range tools {
		name := tool.Name()
		if _, exists := toolsByName[name]; exists {
			return nil, nil, fmt.Errorf("duplicate tool name: %q", name)
		}
		toolsByName[name] = tool
	}
	return tools, toolsByName, nil
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

	// Save the caller's input messages before session history is prepended.
	// These are used later to compute the turn delta for session saving.
	inputMessages := options.Messages

	messages := a.prepareMessages(options)
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Determine active session (per-call override takes priority)
	sess := options.Session
	if sess == nil {
		sess = a.session
	}

	// Load session history and prepend to messages
	if sess != nil {
		sessionMsgs, err := sess.Messages(ctx)
		if err != nil {
			return nil, fmt.Errorf("session load error: %w", err)
		}
		if len(sessionMsgs) > 0 {
			messages = append(sessionMsgs, messages...)
		}
	}

	systemPrompt := strings.TrimSpace(a.systemPrompt)

	// Initialize hook context shared across all phases
	hctx := NewHookContext()
	hctx.Agent = a
	hctx.SystemPrompt = systemPrompt
	hctx.Messages = messages

	// Copy caller-provided values into hook context
	maps.Copy(hctx.Values, options.Values)

	// Run PreGeneration hooks
	for _, hook := range a.hooks.PreGeneration {
		if err := hook(ctx, hctx); err != nil {
			logger.Error("pre-generation hook error", "error", err)
			return nil, fmt.Errorf("pre-generation hook error: %w", err)
		}
	}

	// Use potentially modified values from hooks
	systemPrompt = hctx.SystemPrompt
	messages = hctx.Messages

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

	stopHookActive := false

generateLoop:
	genResult, err := a.generate(ctx, hctx, messages, systemPrompt, eventCallback)
	if err != nil {
		logger.Error("failed to generate response", "error", err)
		return nil, err
	}

	response.FinishedAt = Ptr(time.Now())
	response.Usage = genResult.Usage
	response.Items = genResult.Items
	response.OutputMessages = genResult.OutputMessages

	// Run Stop hooks before PostGeneration
	if len(a.hooks.Stop) > 0 {
		hctx.Response = response
		hctx.OutputMessages = genResult.OutputMessages
		hctx.Usage = genResult.Usage
		hctx.StopHookActive = stopHookActive

		for _, hook := range a.hooks.Stop {
			decision, err := hook(ctx, hctx)
			if err != nil {
				var abortErr *HookAbortError
				if errors.As(err, &abortErr) {
					abortErr.HookType = "Stop"
					logger.Error("stop hook aborted", "error", abortErr)
					return nil, abortErr
				}
				logger.Error("stop hook error", "error", err)
				continue
			}
			if decision != nil && decision.Continue {
				// Inject reason as user message and re-enter generate loop
				reasonMsg := llm.NewUserTextMessage(decision.Reason)
				messages = append(messages, genResult.OutputMessages...)
				messages = append(messages, reasonMsg)
				hctx.Messages = messages
				stopHookActive = true
				goto generateLoop
			}
		}
	}

	// Run PostGeneration hooks
	hctx.Response = response
	hctx.OutputMessages = genResult.OutputMessages
	hctx.Usage = genResult.Usage
	for _, hook := range a.hooks.PostGeneration {
		if err := hook(ctx, hctx); err != nil {
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

	// Save session turn (input + output messages)
	if sess != nil {
		turnMessages := make([]*llm.Message, 0, len(inputMessages)+len(response.OutputMessages))
		turnMessages = append(turnMessages, inputMessages...)
		turnMessages = append(turnMessages, response.OutputMessages...)
		if err := sess.SaveTurn(ctx, turnMessages, response.Usage); err != nil {
			logger.Error("session save error", "error", err)
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
func (a *Agent) generate(ctx context.Context, hctx *HookContext, messages []*llm.Message, systemPrompt string, callback EventCallback) (*generateResult, error) {

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

	// The loop is used to run and respond to the primary generation request
	// and then automatically run any tool-use invocations. The first time
	// through, we submit the primary generation. On subsequent loops, we are
	// running tool-uses and responding with the results.
	generationLimit := a.toolIterationLimit + 1
	lastIteration := false
	for i := range generationLimit {
		// Run PreIteration hooks
		if len(a.hooks.PreIteration) > 0 {
			hctx.Iteration = i
			hctx.SystemPrompt = systemPrompt
			hctx.Messages = updatedMessages
			for _, hook := range a.hooks.PreIteration {
				if err := hook(ctx, hctx); err != nil {
					return nil, fmt.Errorf("pre-iteration hook error: %w", err)
				}
			}
			// Apply any modifications from hooks
			if hctx.SystemPrompt != systemPrompt {
				systemPrompt = hctx.SystemPrompt
			}
		}

		// Resolve tools (static + dynamic toolsets)
		resolvedTools, toolsByName, resolveErr := a.resolveTools(ctx)
		if resolveErr != nil {
			return nil, fmt.Errorf("tool resolution error: %w", resolveErr)
		}

		// Build per-iteration LLM options
		baseOpts := a.getGenerationOptions(systemPrompt, resolvedTools)
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
		toolResults, err := a.executeToolCalls(ctx, hctx, toolCalls, toolsByName, collectingCallback)
		if err != nil {
			return nil, err
		}

		// Capture results in a new message to send to LLM on the next iteration
		toolResultMessage := llm.NewToolResultMessage(getToolResultContent(toolResults)...)

		// Append any additional context injected by hooks
		for _, tc := range getAdditionalContextContent(toolResults) {
			toolResultMessage.Content = append(toolResultMessage.Content, tc)
		}

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
	hctx *HookContext,
	toolCalls []*llm.ToolUseContent,
	toolsByName map[string]Tool,
	callback EventCallback,
) ([]*ToolCallResult, error) {
	results := make([]*ToolCallResult, len(toolCalls))
	for i, toolCall := range toolCalls {
		tool, ok := toolsByName[toolCall.Name]
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
		preHctx := &HookContext{
			Agent:  a,
			Values: hctx.Values,
			Tool:   tool,
			Call:   toolCall,
		}
		denied := false
		for _, hook := range a.hooks.PreToolUse {
			if err := hook(ctx, preHctx); err != nil {
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
			input := toolCall.Input
			if preHctx.UpdatedInput != nil {
				input = preHctx.UpdatedInput
			}
			result = a.executeTool(ctx, tool, toolCall, input, preview)
		}

		// Determine if the tool call failed
		failed := result.Error != nil || (result.Result != nil && result.Result.IsError)

		// Run PostToolUse or PostToolUseFailure hooks based on outcome
		postHctx := &HookContext{
			Agent:  a,
			Values: hctx.Values,
			Tool:   tool,
			Call:   toolCall,
			Result: result,
		}
		if failed {
			for _, hook := range a.hooks.PostToolUseFailure {
				if err := hook(ctx, postHctx); err != nil {
					var abortErr *HookAbortError
					if errors.As(err, &abortErr) {
						abortErr.HookType = "PostToolUseFailure"
						a.logger.Error("post-tool-use-failure hook aborted", "error", abortErr)
						return nil, abortErr
					}
					a.logger.Debug("post-tool-use-failure hook error", "error", err)
				}
			}
		} else {
			for _, hook := range a.hooks.PostToolUse {
				if err := hook(ctx, postHctx); err != nil {
					var abortErr *HookAbortError
					if errors.As(err, &abortErr) {
						abortErr.HookType = "PostToolUse"
						a.logger.Error("post-tool-use hook aborted", "error", abortErr)
						return nil, abortErr
					}
					a.logger.Debug("post-tool-use hook error", "error", err)
				}
			}
		}

		// Use potentially modified result from hooks
		result = postHctx.Result
		results[i] = result

		// Apply AdditionalContext from pre or post hooks
		additionalContext := preHctx.AdditionalContext
		if postHctx.AdditionalContext != "" {
			if additionalContext != "" {
				additionalContext += "\n"
			}
			additionalContext += postHctx.AdditionalContext
		}
		if additionalContext != "" {
			result.AdditionalContext = additionalContext
		}

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

// executeTool runs the tool and returns the result. Panics in tool.Call are
// recovered and converted to error results so the LLM can see the failure
// and adapt, rather than crashing the process.
func (a *Agent) executeTool(
	ctx context.Context,
	tool Tool,
	call *llm.ToolUseContent,
	input []byte,
	preview *ToolCallPreview,
) (result *ToolCallResult) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("tool panic recovered",
				"tool", tool.Name(),
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)
			result = &ToolCallResult{
				ID:      call.ID,
				Name:    call.Name,
				Input:   call.Input,
				Preview: preview,
				Result: &ToolResult{
					Content: []*ToolResultContent{
						{
							Type: ToolResultContentTypeText,
							Text: fmt.Sprintf("Tool %s panicked: %v", tool.Name(), r),
						},
					},
					IsError: true,
				},
				Error: fmt.Errorf("tool %s panicked: %v", tool.Name(), r),
			}
		}
	}()

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

// getGenerationOptions builds LLM options for a generation iteration using
// the resolved tool set and effective system prompt.
func (a *Agent) getGenerationOptions(systemPrompt string, tools []Tool) []llm.Option {
	var generateOpts []llm.Option
	if systemPrompt != "" {
		generateOpts = append(generateOpts, llm.WithSystemPrompt(systemPrompt))
	}
	if len(tools) > 0 {
		defs := make([]llm.Tool, len(tools))
		for i, tool := range tools {
			defs[i] = tool
		}
		generateOpts = append(generateOpts, llm.WithTools(defs...))
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
