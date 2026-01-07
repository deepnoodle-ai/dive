package dive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
)

var (
	defaultResponseTimeout    = time.Minute * 10
	defaultToolIterationLimit = 100
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

// Confirm our standard implementation satisfies the StandardAgent interface.
var _ Agent = &StandardAgent{}

// ModelSettings are used to configure details of the LLM for an StandardAgent.
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

// AgentOptions are used to configure an StandardAgent.
type AgentOptions struct {
	ID                 string
	Name               string
	Goal               string
	Instructions       string
	IsSupervisor       bool
	Subordinates       []string
	Model              llm.LLM
	Tools              []Tool
	ResponseTimeout    time.Duration
	Hooks              llm.Hooks
	Logger             llm.Logger
	ToolIterationLimit int
	ModelSettings      *ModelSettings
	DateAwareness      *bool
	ThreadRepository   ThreadRepository
	SystemPrompt       string
	NoSystemPrompt     bool
	Context            []llm.Content

	// Permission configures tool permission behavior.
	// This replaces the legacy Interactor for tool-related confirmations.
	Permission *PermissionConfig

	// Interactor handles non-tool user interactions (Select, MultiSelect, Input).
	// For tool confirmations, use Permission instead.
	// Deprecated: For tool confirmations, use Permission with a CanUseTool callback.
	Interactor UserInteractor

	// Compaction configures client-side context compaction.
	// When enabled and token thresholds are exceeded, the agent automatically
	// summarizes conversation history to fit within context limits.
	Compaction *CompactionConfig

	// Subagents defines specialized subagents this agent can spawn via the Task tool.
	// Keys are subagent names (e.g., "code-reviewer"), values are their definitions.
	// Claude automatically decides when to invoke subagents based on descriptions.
	Subagents map[string]*SubagentDefinition

	// SubagentLoader loads subagent definitions from external sources.
	// Use FileSubagentLoader for filesystem-based loading, or implement
	// the SubagentLoader interface for custom sources (database, API, etc.).
	// Programmatically defined subagents (in Subagents map) take precedence.
	SubagentLoader SubagentLoader
}

// StandardAgent is the standard implementation of the Agent interface.
type StandardAgent struct {
	id                   string
	name                 string
	goal                 string
	instructions         string
	model                llm.LLM
	tools                []Tool
	toolsByName          map[string]Tool
	isSupervisor         bool
	subordinates         []string
	responseTimeout      time.Duration
	hooks                llm.Hooks
	logger               llm.Logger
	toolIterationLimit   int
	modelSettings        *ModelSettings
	dateAwareness        *bool
	threadRepository     ThreadRepository
	interactor           UserInteractor
	permissionManager    *PermissionManager
	systemPromptTemplate *template.Template
	context              []llm.Content
	compaction           *CompactionConfig
	subagentRegistry     *SubagentRegistry
}

// NewAgent returns a new StandardAgent configured with the given options.
func NewAgent(opts AgentOptions) (*StandardAgent, error) {
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
	if opts.ID == "" {
		opts.ID = newID()
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
	agent := &StandardAgent{
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
		interactor:           opts.Interactor,
		context:              opts.Context,
		compaction:           opts.Compaction,
	}
	// Initialize permission manager with backward compatibility
	agent.permissionManager = agent.initPermissionManager(opts.Permission, opts.Interactor)
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

	// Initialize subagent registry
	agent.subagentRegistry = NewSubagentRegistry(true) // Include general-purpose by default

	// Load subagents from external source if configured
	if opts.SubagentLoader != nil {
		loaded, err := opts.SubagentLoader.Load(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to load subagents: %w", err)
		}
		agent.subagentRegistry.RegisterAll(loaded)
	}

	// Register programmatically defined subagents (take precedence over loaded)
	if len(opts.Subagents) > 0 {
		agent.subagentRegistry.RegisterAll(opts.Subagents)
	}

	return agent, nil
}

func (a *StandardAgent) Name() string {
	return a.name
}

func (a *StandardAgent) Goal() string {
	return a.goal
}

func (a *StandardAgent) Instructions() string {
	return a.instructions
}

func (a *StandardAgent) IsSupervisor() bool {
	return a.isSupervisor
}

func (a *StandardAgent) Subordinates() []string {
	if !a.isSupervisor {
		return nil
	}
	return a.subordinates
}

func (a *StandardAgent) HasTools() bool {
	return len(a.tools) > 0
}

// Tools returns the agent's tools.
func (a *StandardAgent) Tools() []Tool {
	return a.tools
}

// SubagentRegistry returns the agent's subagent registry.
func (a *StandardAgent) SubagentRegistry() *SubagentRegistry {
	return a.subagentRegistry
}

// PermissionManager returns the agent's permission manager.
// This allows access to permission state, such as session allowlists.
func (a *StandardAgent) PermissionManager() *PermissionManager {
	return a.permissionManager
}

// Model returns the agent's LLM.
func (a *StandardAgent) Model() llm.LLM {
	return a.model
}

func (a *StandardAgent) prepareThread(ctx context.Context, messages []*llm.Message, options CreateResponseOptions) (*Thread, error) {
	thread, err := a.getOrCreateThread(ctx, options.ThreadID, options)
	if err != nil {
		return nil, err
	}
	thread.Messages = append(thread.Messages, messages...)
	return thread, nil
}

func (a *StandardAgent) CreateResponse(ctx context.Context, opts ...CreateResponseOption) (*Response, error) {
	var options CreateResponseOptions
	options.Apply(opts)

	// Auto-generate thread ID if not provided
	if options.ThreadID == "" {
		options.ThreadID = newThreadID()
	}

	logger := a.logger.With(
		"agent_name", a.name,
		"thread_id", options.ThreadID,
		"user_id", options.UserID,
	)
	logger.Info("creating response")

	messages := a.prepareMessages(options)
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Handle forking BEFORE prepareThread to avoid modifying the original thread
	if options.Fork && a.threadRepository != nil && options.ThreadID != "" {
		forkedThread, err := a.threadRepository.ForkThread(ctx, options.ThreadID)
		if err != nil && err != ErrThreadNotFound {
			return nil, fmt.Errorf("failed to fork thread: %w", err)
		}
		if forkedThread != nil {
			// Update options to use the forked thread ID
			options.ThreadID = forkedThread.ID
			logger = logger.With("forked_from", options.ThreadID, "forked_to", forkedThread.ID)
			logger.Info("forked thread")
		}
	}

	thread, err := a.prepareThread(ctx, messages, options)
	if err != nil {
		return nil, err
	}

	// Apply per-request compaction override
	originalCompaction := a.compaction
	if options.Compaction != nil {
		a.compaction = options.Compaction
	}
	defer func() { a.compaction = originalCompaction }()

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

	response := &Response{
		ID:        randomInt(),
		ThreadID:  thread.ID,
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
					Init: &InitEvent{ThreadID: thread.ID},
				}
				if err := options.EventCallback(ctx, initItem); err != nil {
					return err
				}
			}
			return options.EventCallback(ctx, item)
		}
		return nil
	}

	genResult, err := a.generate(ctx, thread.Messages, systemPrompt, eventCallback)
	if err != nil {
		logger.Error("failed to generate response", "error", err)
		return nil, err
	}

	// Handle compaction: replace old messages with summary + new output
	if genResult.CompactedMessages != nil {
		// Thread gets: compacted summary + post-compaction assistant response
		thread.Messages = append(genResult.CompactedMessages, genResult.OutputMessages...)
		// Record compaction in history
		thread.CompactionHistory = append(thread.CompactionHistory, CompactionRecord{
			Timestamp:         time.Now(),
			TokensBefore:      genResult.CompactionEvent.TokensBefore,
			TokensAfter:       genResult.CompactionEvent.TokensAfter,
			MessagesCompacted: genResult.CompactionEvent.MessagesCompacted,
		})
		// Set compaction on response for non-callback clients
		response.Compaction = genResult.CompactionEvent
	} else {
		thread.Messages = append(thread.Messages, genResult.OutputMessages...)
	}

	if a.threadRepository != nil {
		if err := a.threadRepository.PutThread(ctx, thread); err != nil {
			logger.Error("failed to save thread", "error", err)
			return nil, err
		}
	}

	response.FinishedAt = ptr(time.Now())
	response.Usage = genResult.Usage

	for _, msg := range genResult.OutputMessages {
		response.Items = append(response.Items, &ResponseItem{
			Type:    ResponseItemTypeMessage,
			Message: msg,
		})
	}
	return response, nil
}

// prepareMessages processes the ChatAgentOptions to create messages for the LLM.
// It handles both WithMessages and WithInput options.
func (a *StandardAgent) prepareMessages(options CreateResponseOptions) []*llm.Message {
	var messages []*llm.Message
	if len(a.context) > 0 {
		messages = append(messages, llm.NewUserMessage(a.context...))
	}
	if len(options.Messages) > 0 {
		messages = append(messages, options.Messages...)
	}
	return messages
}

func (a *StandardAgent) getOrCreateThread(ctx context.Context, threadID string, options CreateResponseOptions) (*Thread, error) {
	if a.threadRepository != nil {
		thread, err := a.threadRepository.GetThread(ctx, threadID)
		if err != nil {
			if err != ErrThreadNotFound {
				return nil, err
			}
		} else {
			return thread, nil
		}
	}
	return &Thread{
		ID:        threadID,
		UserID:    options.UserID,
		AgentID:   a.id,
		AgentName: a.name,
		Messages:  []*llm.Message{},
	}, nil
}

func (a *StandardAgent) buildSystemPrompt() (string, error) {
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
		prompt = fmt.Sprintf("%s\n\n%s", prompt, dateTimeString(time.Now()))
	}
	return strings.TrimSpace(prompt), nil
}

// generate runs the LLM generation and tool execution loop. It handles the
// interaction between the agent and the LLM, including tool calls. Returns the
// final LLM response, updated messages, and any error that occurred.
func (a *StandardAgent) generate(
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

	// Track if compaction occurred during this generation
	var compactedMessages []*llm.Message
	var compactionEvent *CompactionEvent
	var compactionOccurred bool // Forces one more generation after compaction

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

		// Check for pending tool calls BEFORE compaction
		// If there are tool calls, we must execute them before compacting
		toolCalls := response.ToolCalls()
		hasPendingToolCalls := len(toolCalls) > 0

		// If we just generated a post-compaction response, we're done
		// (check this before compaction trigger to handle the follow-up generation)
		if compactionOccurred {
			compactionOccurred = false
			if !hasPendingToolCalls {
				break
			}
			// If there are tool calls after compaction, continue processing them
		}

		// Check for compaction trigger (but defer if we have pending tool calls)
		// Use per-call usage, not cumulative, since each call includes the full context
		if !hasPendingToolCalls && a.shouldCompact(&response.Usage, len(updatedMessages)) {
			// Calculate pre-compaction token count for accurate event reporting
			tokensBefore := a.calculateContextTokens(&response.Usage)
			compacted, event, err := a.performCompaction(ctx, updatedMessages, systemPrompt, tokensBefore)
			if err != nil {
				// Log warning but don't fail - continue with uncompacted context
				a.logger.Warn("compaction failed", "error", err)
			} else {
				// Replace context with compacted summary for next generation
				updatedMessages = compacted
				// Clear output messages - we only want post-compaction responses
				// The summary is a User message and shouldn't be in output items
				outputMessages = nil
				// Track compaction for final result
				compactedMessages = compacted
				compactionEvent = event
				// Emit compaction event
				if err := callback(ctx, &ResponseItem{
					Type:       ResponseItemTypeCompaction,
					Compaction: event,
				}); err != nil {
					return nil, err
				}
				// Set flag to generate exactly one more response with compacted context
				compactionOccurred = true
				a.logger.Debug("compaction complete, will generate response with compacted context",
					"tokens_before", event.TokensBefore,
					"tokens_after", event.TokensAfter,
					"messages_compacted", event.MessagesCompacted,
				)
				// Continue to next iteration to generate a response
				continue
			}
		}

		// We're done if there are no tool calls
		if !hasPendingToolCalls {
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

	// Return result with compaction info if it occurred
	result := &generateResult{
		OutputMessages: outputMessages,
		Usage:          totalUsage,
	}
	if compactedMessages != nil {
		result.CompactedMessages = compactedMessages
		result.CompactionEvent = compactionEvent
	}
	return result, nil
}

func (a *StandardAgent) isCachingEnabled() bool {
	if a.modelSettings == nil || a.modelSettings.Caching == nil {
		return true // default to caching enabled
	}
	return *a.modelSettings.Caching
}

func (a *StandardAgent) configureCacheControl(messages []*llm.Message) {
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

// initPermissionManager creates a permission manager with backward compatibility.
// If Permission config is provided, it uses that. Otherwise, it converts the
// legacy Interactor to a permission-based flow.
func (a *StandardAgent) initPermissionManager(
	permission *PermissionConfig,
	interactor UserInteractor,
) *PermissionManager {
	// Create confirmer function from interactor or default
	confirmer := func(ctx context.Context, tool Tool, call *llm.ToolUseContent, message string) (bool, error) {
		if interactor == nil {
			return true, nil // No interactor = auto-approve
		}
		return interactor.Confirm(ctx, &ConfirmRequest{
			Tool:    tool,
			Call:    call,
			Title:   fmt.Sprintf("Execute %s?", tool.Name()),
			Message: message,
		})
	}

	// If permission config is provided, use it
	if permission != nil {
		return NewPermissionManager(permission, confirmer)
	}

	// Convert legacy interactor to permission config
	config := a.permissionConfigFromInteractor(interactor)
	return NewPermissionManager(config, confirmer)
}

// permissionConfigFromInteractor converts a legacy UserInteractor to PermissionConfig.
func (a *StandardAgent) permissionConfigFromInteractor(interactor UserInteractor) *PermissionConfig {
	if interactor == nil {
		// No interactor = bypass permissions (auto-approve)
		return &PermissionConfig{Mode: PermissionModeBypassPermissions}
	}

	// Check if it's a TerminalInteractor with a mode
	if ti, ok := interactor.(*TerminalInteractor); ok {
		switch ti.Mode {
		case InteractNever:
			return &PermissionConfig{Mode: PermissionModeBypassPermissions}
		case InteractAlways:
			return &PermissionConfig{
				Mode:  PermissionModeDefault,
				Rules: PermissionRules{{Type: PermissionRuleAsk, Tool: "*"}},
			}
		case InteractIfDestructive:
			return &PermissionConfig{
				Mode: PermissionModeDefault,
				CanUseTool: func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error) {
					if tool != nil {
						annotations := tool.Annotations()
						if annotations != nil && annotations.DestructiveHint {
							return AskResult(""), nil
						}
					}
					return AllowResult(), nil
				},
			}
		case InteractIfNotReadOnly:
			return &PermissionConfig{
				Mode: PermissionModeDefault,
				CanUseTool: func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error) {
					if tool != nil {
						annotations := tool.Annotations()
						if annotations != nil && annotations.ReadOnlyHint {
							return AllowResult(), nil
						}
					}
					return AskResult(""), nil
				},
			}
		}
	}

	// Default: ask for all tools
	return &PermissionConfig{
		Mode:  PermissionModeDefault,
		Rules: PermissionRules{{Type: PermissionRuleAsk, Tool: "*"}},
	}
}

// generateStreaming handles streaming generation with an LLM, including
// receiving and republishing events, and accumulating a complete response.
func (a *StandardAgent) generateStreaming(
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

func (a *StandardAgent) getInteractor() UserInteractor {
	return a.interactor
}

// executeToolCalls executes all tool calls and returns the tool call results.
// This implements Anthropic's permission flow:
// PreToolUse Hook → Deny Rules → Allow Rules → Ask Rules → Mode Check → CanUseTool → Execute → PostToolUse Hook
func (a *StandardAgent) executeToolCalls(
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

		// Evaluate permissions using the permission manager
		hookResult, err := a.permissionManager.EvaluateToolUse(ctx, tool, toolCall, a)
		if err != nil {
			return nil, fmt.Errorf("permission evaluation error: %w", err)
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
			// Prompt user for confirmation
			message := hookResult.Message
			if message == "" && preview != nil {
				message = preview.Summary
			}
			confirmed, err := a.permissionManager.Confirm(ctx, tool, toolCall, message)
			if err != nil {
				// Check if this is user feedback (not a real error)
				if feedback, ok := IsUserFeedback(err); ok {
					result = a.createDeniedResult(toolCall, feedback, preview)
				} else {
					return nil, fmt.Errorf("tool call confirmation error: %w", err)
				}
			} else if confirmed {
				result = a.executeTool(ctx, tool, toolCall, toolCall.Input, preview)
			} else {
				result = a.createDeniedResult(toolCall, "User denied tool call", preview)
			}

		default:
			// ToolHookContinue should not happen after full evaluation
			// Treat as ask to be safe
			confirmed, err := a.permissionManager.Confirm(ctx, tool, toolCall, "")
			if err != nil {
				// Check if this is user feedback (not a real error)
				if feedback, ok := IsUserFeedback(err); ok {
					result = a.createDeniedResult(toolCall, feedback, preview)
				} else {
					return nil, fmt.Errorf("tool call confirmation error: %w", err)
				}
			} else if confirmed {
				result = a.executeTool(ctx, tool, toolCall, toolCall.Input, preview)
			} else {
				result = a.createDeniedResult(toolCall, "User denied tool call", preview)
			}
		}

		results[i] = result

		// Run PostToolUse hooks
		postCtx := &PostToolUseContext{
			Tool:   tool,
			Call:   toolCall,
			Result: result,
			Agent:  a,
		}
		if err := a.permissionManager.RunPostToolUseHooks(ctx, postCtx); err != nil {
			a.logger.Debug("post-tool-use hook error", "error", err)
		}

		// Emit result event
		if err := callback(ctx, &ResponseItem{
			Type:           ResponseItemTypeToolCallResult,
			ToolCallResult: result,
		}); err != nil {
			return nil, err
		}

		// Emit TodoEvent for TodoWrite tool calls
		if toolCall.Name == "TodoWrite" && result.Error == nil {
			var todoInput struct {
				Todos []TodoItem `json:"todos"`
			}
			if err := json.Unmarshal(toolCall.Input, &todoInput); err == nil {
				if err := callback(ctx, &ResponseItem{
					Type: ResponseItemTypeTodo,
					Todo: &TodoEvent{Todos: todoInput.Todos},
				}); err != nil {
					return nil, err
				}
			}
		}
	}
	return results, nil
}

// executeTool runs the tool and returns the result.
func (a *StandardAgent) executeTool(
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
func (a *StandardAgent) createDeniedResult(
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

func (a *StandardAgent) getToolDefinitions() []llm.Tool {
	definitions := make([]llm.Tool, len(a.tools))
	for i, tool := range a.tools {
		definitions[i] = tool
	}
	return definitions
}

func (a *StandardAgent) getGenerationAgentOptions(systemPrompt string) []llm.Option {
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

func (a *StandardAgent) Context() []llm.Content {
	return a.context
}

// isToolAllowed checks if a tool is allowed by any ToolAllowanceChecker in the
// agent's tools. This is used to enforce skill-based tool restrictions.
func (a *StandardAgent) isToolAllowed(toolName string) bool {
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
	OutputMessages    []*llm.Message
	Usage             *llm.Usage
	CompactedMessages []*llm.Message   // If set, Thread.Messages should be replaced with this
	CompactionEvent   *CompactionEvent // If set, compaction occurred
}

// calculateTotalTokens returns the total token count for billing/stats purposes.
// Includes all token types: input, output, and cache tokens.
func (a *StandardAgent) calculateTotalTokens(usage *llm.Usage) int {
	return usage.InputTokens + usage.OutputTokens +
		usage.CacheCreationInputTokens + usage.CacheReadInputTokens
}

// calculateContextTokens returns the context window token count for threshold checks.
// Per Anthropic API: input_tokens are non-cached tokens, cache_read_input_tokens are
// tokens read from cache. Together they represent the actual context size.
// Note: cache_creation_input_tokens is a subset of input_tokens, not additive.
func (a *StandardAgent) calculateContextTokens(usage *llm.Usage) int {
	return usage.InputTokens + usage.CacheReadInputTokens
}

// shouldCompact returns true if compaction should be triggered based on token usage.
func (a *StandardAgent) shouldCompact(usage *llm.Usage, messageCount int) bool {
	if a.compaction == nil || !a.compaction.Enabled {
		return false
	}
	// Never compact if there are fewer than 2 messages
	if messageCount < 2 {
		return false
	}
	threshold := a.compaction.ContextTokenThreshold
	if threshold <= 0 {
		threshold = DefaultContextTokenThreshold
	}
	return a.calculateContextTokens(usage) >= threshold
}

// performCompaction generates a summary of the conversation and returns compacted messages.
// tokensBefore is the pre-compaction context token count for accurate event reporting.
func (a *StandardAgent) performCompaction(
	ctx context.Context,
	messages []*llm.Message,
	systemPrompt string,
	tokensBefore int,
) ([]*llm.Message, *CompactionEvent, error) {
	// Step 1: Filter out pending tool use blocks
	cleanedMessages := a.filterPendingToolUse(messages)
	if len(cleanedMessages) == 0 {
		return nil, nil, fmt.Errorf("no messages to compact after filtering")
	}

	// Track original message count before any trimming for accurate reporting
	originalMessageCount := len(cleanedMessages)

	// Step 1.5: Trim messages if too many to avoid exceeding context during summarization
	// Keep first message (often contains important context) + recent messages
	const maxMessagesForSummary = 50
	if len(cleanedMessages) > maxMessagesForSummary {
		cleanedMessages = append(
			cleanedMessages[:1], // Keep first message
			cleanedMessages[len(cleanedMessages)-maxMessagesForSummary+1:]..., // Keep recent messages
		)
	}

	// Step 2: Build summary request
	summaryPrompt := a.compaction.SummaryPrompt
	if summaryPrompt == "" {
		summaryPrompt = DefaultCompactionSummaryPrompt
	}

	// Add summary instruction as a user message
	summaryMessages := make([]*llm.Message, len(cleanedMessages)+1)
	copy(summaryMessages, cleanedMessages)
	summaryMessages[len(cleanedMessages)] = llm.NewUserTextMessage(summaryPrompt)

	// Step 3: Choose model for summary
	model := a.compaction.Model
	if model == nil {
		model = a.model
	}

	// Step 4: Generate summary (non-streaming for simplicity)
	summaryOpts := []llm.Option{
		llm.WithSystemPrompt(systemPrompt),
		llm.WithMessages(summaryMessages...),
	}

	a.logger.Debug("performing compaction",
		"message_count", len(cleanedMessages),
		"model", model.Name(),
	)

	summaryResp, err := model.Generate(ctx, summaryOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("compaction summary generation failed: %w", err)
	}

	// Step 5: Extract summary from response
	summaryText := a.extractSummary(summaryResp.Message().Text())
	if summaryText == "" {
		return nil, nil, fmt.Errorf("no summary found in compaction response (missing <summary> tags)")
	}

	// Step 6: Create new message list with the summary as a user message
	// This ensures the first message is from the User role, which is required by most LLM APIs
	summaryPrefix := "Here is a summary of our conversation so far:\n\n"
	compactedMessages := []*llm.Message{
		llm.NewUserTextMessage(summaryPrefix + summaryText),
	}

	// Step 7: Build compaction event
	// TokensAfter is estimated from full summary message length (rough heuristic: ~4 chars per token)
	// This is an approximation since actual tokenization varies by model
	fullSummaryLen := len(summaryPrefix) + len(summaryText)
	tokensAfter := fullSummaryLen / 4
	if tokensAfter < 100 {
		tokensAfter = 100 // Minimum reasonable estimate
	}
	event := &CompactionEvent{
		TokensBefore:      tokensBefore,
		TokensAfter:       tokensAfter,
		Summary:           summaryText,
		MessagesCompacted: originalMessageCount, // Use pre-trim count
	}

	a.logger.Debug("compaction complete",
		"tokens_before", event.TokensBefore,
		"tokens_after", event.TokensAfter,
		"messages_compacted", event.MessagesCompacted,
	)

	return compactedMessages, event, nil
}

// extractSummary extracts content from <summary></summary> tags.
// Matching is case-insensitive to handle variations like <Summary> or <SUMMARY>.
func (a *StandardAgent) extractSummary(text string) string {
	lower := strings.ToLower(text)
	startTag := "<summary>"
	endTag := "</summary>"

	startIdx := strings.Index(lower, startTag)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startTag)

	endIdx := strings.Index(lower[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	// Extract from original text (not lowercase) to preserve case of summary content
	return strings.TrimSpace(text[startIdx : startIdx+endIdx])
}

// filterPendingToolUse removes tool_use blocks that don't have corresponding tool_result.
// If the last assistant message contains only tool_use blocks, remove the entire message.
func (a *StandardAgent) filterPendingToolUse(messages []*llm.Message) []*llm.Message {
	if len(messages) == 0 {
		return messages
	}

	// Check if the last message is an assistant message with tool_use
	lastMsg := messages[len(messages)-1]
	if lastMsg.Role != llm.Assistant {
		return messages
	}

	// Count tool use blocks in the last message
	toolUseCount := 0
	nonToolUseCount := 0
	for _, content := range lastMsg.Content {
		if _, ok := content.(*llm.ToolUseContent); ok {
			toolUseCount++
		} else {
			nonToolUseCount++
		}
	}

	// If no tool use, return as-is
	if toolUseCount == 0 {
		return messages
	}

	// If all content was tool use, remove the entire message
	if nonToolUseCount == 0 {
		return messages[:len(messages)-1]
	}

	// Otherwise, filter out tool use blocks from the last message
	filteredContent := make([]llm.Content, 0, nonToolUseCount)
	for _, content := range lastMsg.Content {
		if _, isToolUse := content.(*llm.ToolUseContent); !isToolUse {
			filteredContent = append(filteredContent, content)
		}
	}

	// Create a copy with filtered content
	result := make([]*llm.Message, len(messages))
	copy(result, messages)
	result[len(result)-1] = &llm.Message{
		ID:      lastMsg.ID,
		Role:    lastMsg.Role,
		Content: filteredContent,
	}
	return result
}
