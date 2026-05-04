package dive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
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

// sessionLocks serializes CreateResponse calls that share a session ID.
// Concurrent calls on the same session would otherwise interleave their
// Messages() reads and SaveTurn writes, producing tangled event state and
// — on suspended sessions — mixed pending-call sets. The lock is keyed by
// Session.ID() so it also covers cross-agent usage of a single session.
//
// Entries accumulate in the map for the lifetime of the process; if you
// create unbounded fresh session IDs, the memory cost is a small *sync.Mutex
// per distinct ID. For typical workloads this is negligible.
var sessionLocks sync.Map

// acquireSessionLock blocks until the caller holds the exclusive lock for
// the given session ID. The returned function releases the lock and must
// be deferred by the caller.
func acquireSessionLock(id string) func() {
	v, _ := sessionLocks.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

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

	// OnSuspend hooks run when the agent transitions into a suspended state,
	// before PostGeneration. Use to notify external systems that human input
	// is needed.
	OnSuspend []OnSuspendHook
}

// Extension bundles tools, hooks, and system prompt rules that extend an
// agent's capabilities. Implementations provide any combination of these,
// returning nil/empty for aspects they don't use.
type Extension interface {
	// Tools returns additional tools to make available to the agent.
	Tools() []Tool

	// Hooks returns hooks to register on the agent.
	Hooks() Hooks

	// Rules returns text to append to the agent's system prompt.
	// Returns empty string if no rules are needed.
	Rules() string
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

	// Extensions provide additional tools, hooks, and system prompt rules.
	// Extensions are merged in order: tools are appended, hooks are appended
	// to their respective slices, and rules are appended to the system prompt.
	// Extension tools and hooks come after those set directly on AgentOptions.
	Extensions []Extension

	// Hooks groups all agent-level hooks.
	Hooks Hooks

	// Infrastructure
	Logger        llm.Logger
	ModelSettings *ModelSettings

	// LLMHooks are provider-level hooks passed to the LLM on each generation.
	// These are distinct from agent-level hooks which control the agent's
	// generation loop.
	LLMHooks llm.Hooks

	// Optional name for logging.
	Name string

	// Description is a free-form purpose/role description. Surfaced via
	// Tracer (e.g. as gen_ai.agent.description) for observability.
	Description string

	// Version identifies a revision of the agent's prompt/tooling/config
	// (e.g. "1.0.0", "2025-05-01"). Surfaced via Tracer.
	Version string

	// ID is a stable identifier for the agent, useful in multi-tenant
	// systems for correlating runs to a specific agent record. Surfaced
	// via Tracer. Empty values are not auto-generated.
	ID string

	// Tracer observes the agent's lifecycle (run start/end, each chat
	// iteration, each tool call) for tracing, metrics, or audit logging.
	// Defaults to NopTracer. The OpenTelemetry adapter lives in
	// experimental/otel.
	Tracer Tracer

	// Session enables persistent conversation state. When set, the agent
	// automatically loads history before generation and saves new messages
	// after generation. Can be overridden per-call with WithSession.
	Session Session

	// Timeouts and limits
	ResponseTimeout    time.Duration
	ToolIterationLimit int

	// ParallelToolExecution enables concurrent execution of tool calls when
	// the LLM returns multiple tool calls in a single message. When false
	// (the default), tool calls are executed sequentially in order.
	//
	// When enabled, ToolCallResult events and PostToolUse hooks fire in
	// completion order (fastest tool first), not in the order the LLM
	// declared the tool calls.
	ParallelToolExecution bool
}

// Agent represents an intelligent AI entity that can autonomously use tools to
// process information while responding to chat messages.
type Agent struct {
	name                  string
	id                    string
	description           string
	version               string
	model                 llm.LLM
	tools                 []Tool
	toolsets              []Toolset
	toolsByName           map[string]Tool
	responseTimeout       time.Duration
	llmHooks              llm.Hooks
	logger                llm.Logger
	toolIterationLimit    int
	parallelToolExecution bool
	modelSettings         *ModelSettings
	systemPrompt          string
	session               Session
	tracer                Tracer

	// mu protects model and systemPrompt for concurrent access via
	// SetModel/SetSystemPrompt while CreateResponse is running.
	mu sync.Mutex

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
	// Merge extensions into opts before building the agent.
	for _, ext := range opts.Extensions {
		if ext == nil {
			continue
		}
		opts.Tools = append(opts.Tools, ext.Tools()...)
		extHooks := ext.Hooks()
		opts.Hooks.PreGeneration = append(opts.Hooks.PreGeneration, extHooks.PreGeneration...)
		opts.Hooks.PostGeneration = append(opts.Hooks.PostGeneration, extHooks.PostGeneration...)
		opts.Hooks.PreToolUse = append(opts.Hooks.PreToolUse, extHooks.PreToolUse...)
		opts.Hooks.PostToolUse = append(opts.Hooks.PostToolUse, extHooks.PostToolUse...)
		opts.Hooks.PostToolUseFailure = append(opts.Hooks.PostToolUseFailure, extHooks.PostToolUseFailure...)
		opts.Hooks.Stop = append(opts.Hooks.Stop, extHooks.Stop...)
		opts.Hooks.PreIteration = append(opts.Hooks.PreIteration, extHooks.PreIteration...)
		opts.Hooks.OnSuspend = append(opts.Hooks.OnSuspend, extHooks.OnSuspend...)
		if rules := ext.Rules(); rules != "" {
			opts.SystemPrompt = strings.TrimRight(opts.SystemPrompt, "\n") + "\n\n" + rules
		}
	}

	if opts.Tracer == nil {
		opts.Tracer = NopTracer{}
	}
	agent := &Agent{
		name:                  opts.Name,
		id:                    opts.ID,
		description:           opts.Description,
		version:               opts.Version,
		model:                 opts.Model,
		responseTimeout:       opts.ResponseTimeout,
		toolIterationLimit:    opts.ToolIterationLimit,
		parallelToolExecution: opts.ParallelToolExecution,
		llmHooks:              opts.LLMHooks,
		logger:                opts.Logger,
		systemPrompt:          opts.SystemPrompt,
		modelSettings:         opts.ModelSettings,
		hooks:                 opts.Hooks,
		session:               opts.Session,
		toolsets:              opts.Toolsets,
		tracer:                opts.Tracer,
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

// ID returns the agent's stable identifier set via AgentOptions.ID, or "" if
// none was provided.
func (a *Agent) ID() string {
	return a.id
}

// Description returns the free-form purpose/role description set via
// AgentOptions.Description, or "" if none was provided.
func (a *Agent) Description() string {
	return a.description
}

// Version returns the agent's version string set via AgentOptions.Version,
// or "" if none was provided.
func (a *Agent) Version() string {
	return a.version
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
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// SetModel replaces the agent's LLM. This allows switching models mid-session.
// It panics if model is nil; use NewAgent to validate the initial model.
func (a *Agent) SetModel(model llm.LLM) {
	if model == nil {
		panic("dive: SetModel called with nil model")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.model = model
}

// SystemPrompt returns the agent's current system prompt.
func (a *Agent) SystemPrompt() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.systemPrompt
}

// SetSystemPrompt replaces the agent's system prompt.
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.systemPrompt = prompt
}

func (a *Agent) CreateResponse(ctx context.Context, opts ...CreateResponseOption) (response *Response, err error) {
	var options CreateResponseOptions
	options.Apply(opts)

	// Snapshot mutable fields under the mutex so concurrent SetModel/SetSystemPrompt
	// calls don't race with the generation loop.
	a.mu.Lock()
	model := a.model
	systemPrompt := strings.TrimSpace(a.systemPrompt)
	a.mu.Unlock()

	logger := a.logger.With("agent_name", a.name)
	logger.Info("creating response")

	// Save the caller's input messages before session history is prepended.
	// These are used later to compute the turn delta for session saving.
	inputMessages := options.Messages

	// Determine active session (per-call override takes priority)
	sess := options.Session
	if sess == nil {
		sess = a.session
	}

	// Serialize concurrent CreateResponse calls that share a session ID.
	// Without this, two callers racing on the same session would interleave
	// Messages() reads and SaveTurn writes and corrupt the event stream.
	// Stateless callers (sess == nil) have no shared state to protect and
	// skip the lock entirely.
	if sess != nil {
		release := acquireSessionLock(sess.ID())
		defer release()
	}

	// Load session history
	var sessionMsgs []*llm.Message
	if sess != nil {
		var err error
		sessionMsgs, err = sess.Messages(ctx)
		if err != nil {
			return nil, fmt.Errorf("session load error: %w", err)
		}
	}

	// Determine the authoritative suspension state.
	//
	// Options supplied via WithResume always win — this lets stateless
	// users drive the feature without a session at all, and lets a
	// cross-process resumer override a stale session snapshot with a
	// fresher one. When the option is absent and the session is
	// suspendable, the session's stored state is used.
	suspendable, _ := sess.(SuspendableSession)
	suspState := options.Suspension
	if suspState == nil && suspendable != nil {
		suspState = suspendable.LoadSuspension()
	}

	hasToolResults := len(options.ToolResults) > 0
	hasExplicitSuspension := options.Suspension != nil
	hasResumeIntent := hasToolResults || hasExplicitSuspension

	if hasResumeIntent && suspState == nil {
		return nil, ErrNoSuspendedTurn
	}
	// A session-backed resume cannot accept new user input — the new
	// input belongs in a fresh turn after the suspended one resolves.
	// Stateless resumes (WithResume supplied) are the opposite:
	// options.Messages IS the pre-turn history, so non-empty input is
	// expected there. Checked before ErrResumeRequired so a clearer error
	// surfaces when the caller's intent is "start a new turn" on a
	// suspended session.
	if suspState != nil && !hasExplicitSuspension && len(inputMessages) > 0 {
		return nil, ErrInputOnSuspendedSession
	}
	// Resume is explicit: a suspended session without any opt-in errors
	// out rather than silently no-op re-saving the suspended turn.
	if suspState != nil && !hasResumeIntent {
		return nil, ErrResumeRequired
	}

	// Build the full history the agent will operate on.
	//
	//  - Stateless / cross-process resume (options.Suspension supplied):
	//    inputMessages is the pre-turn history, suspState.TurnMessages is
	//    the turn itself. Splice them.
	//  - Session-backed resume (no options.Suspension): session.Messages()
	//    already has the suspended turn at its tail — use it directly.
	//  - Normal (non-resume): session history followed by new input.
	var fullHistory []*llm.Message
	switch {
	case suspState != nil && hasExplicitSuspension:
		fullHistory = append(fullHistory, inputMessages...)
		fullHistory = append(fullHistory, suspState.TurnMessages...)
	case suspState != nil:
		fullHistory = append(fullHistory, sessionMsgs...)
	default:
		fullHistory = append(fullHistory, sessionMsgs...)
		fullHistory = append(fullHistory, inputMessages...)
	}

	var rs *resumeState
	if suspState != nil {
		var err error
		rs, err = a.prepareResume(fullHistory, suspState, options.ToolResults)
		if err != nil {
			return nil, err
		}
		// Replace the loaded history's tool_result message with the merged
		// version (or append one if there was no prior tool_result).
		fullHistory = rs.SessionMessagesWithMerged
	}

	// Build the message list for generation
	messages := fullHistory
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}

	// Open the agent-run span. The returned ctx carries the span so chat
	// and tool spans nest under it. NopTracer makes this a zero-cost no-op
	// for agents that don't configure a tracer.
	ctx, runSpan := a.tracer.StartAgentRun(ctx, AgentRunInfo{Agent: a, Session: sess})
	defer func() {
		if response != nil {
			runSpan.SetResponse(response)
			if response.Usage != nil {
				runSpan.SetUsage(response.Usage)
			}
		}
		runSpan.End(err)
	}()

	// Initialize hook context shared across all phases
	hctx := NewHookContext()
	hctx.Agent = a
	hctx.Session = sess
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

	response = &Response{
		Model:     model.Name(),
		CreatedAt: time.Now(),
	}

	eventCallback := func(ctx context.Context, item *ResponseItem) error {
		if options.EventCallback != nil {
			return options.EventCallback(ctx, item)
		}
		return nil
	}

	// Resume-specific handling before entering the generate loop:
	//  1. Fire PostToolUse/PostToolUseFailure hooks for caller-supplied results.
	//  2. If partial resume (some pending not supplied), short-circuit and
	//     re-save the suspended turn.
	//  3. Otherwise execute any "not-started" tool calls; if any re-suspend,
	//     capture and unwind.
	var resumeExtraItems []*ResponseItem
	if rs != nil {
		// Resolve tools once for the entire resume phase: used both to
		// populate HookContext.Tool on post hooks and to execute any
		// not-started tool calls.
		_, resumeToolsByName, err := a.resolveTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("tool resolution error: %w", err)
		}

		// Fire post hooks for caller-supplied results.
		if err := a.fireResumePostHooks(ctx, hctx, rs, resumeToolsByName); err != nil {
			return nil, err
		}
		if len(rs.RemainingPending) > 0 {
			// Partial resume: update session and return a new suspended response.
			// This is not a fresh transition — the session was already suspended —
			// so we skip OnSuspend notifications and the terminal stream item.
			snap := &suspendedSnapshot{
				PendingToolCalls:   rs.RemainingPendingCalls,
				CompletedToolCalls: rs.CompletedToolCalls(),
			}
			return a.finishSuspended(ctx, logger, hctx, response, inputMessages, snap, nil, eventCallback, sess, rs, true)
		}
		// Execute not-started tool calls, if any.
		if len(rs.NotStartedToolCalls) > 0 {
			var resumeItems []*ResponseItem
			resumeCallback := func(ctx context.Context, item *ResponseItem) error {
				resumeItems = append(resumeItems, item)
				return eventCallback(ctx, item)
			}
			batch, err := a.executeToolCalls(ctx, hctx, rs.NotStartedToolCalls, resumeToolsByName, resumeCallback)
			if err != nil {
				return nil, err
			}
			resumeExtraItems = resumeItems
			// Merge completed outcomes into the tool_result message.
			completed := batch.Completed()
			if len(completed) > 0 {
				rs.AppendToolResults(getToolResultContent(completed))
				for _, tc := range getAdditionalContextContent(completed) {
					rs.AppendToolResultTextContent(tc)
				}
			}
			if batch.Suspended {
				snap := buildSuspendedSnapshot(rs.NotStartedToolCalls, batch)
				// Prepend previously-completed calls from the original suspend.
				snap.CompletedToolCalls = append(rs.CompletedToolCalls(), snap.CompletedToolCalls...)
				return a.finishSuspended(ctx, logger, hctx, response, inputMessages, snap, resumeExtraItems, eventCallback, sess, rs, false)
			}
		}
	}

	stopHookActive := false

	// Accumulators persist across Stop-hook continuations so every
	// iteration's output (plus any synthetic user reason injected by a Stop
	// hook) is preserved on the Response and in the saved session turn.
	// generate() builds a fresh outputMessages slice on each call, so
	// without this accumulation a Stop-hook continuation that later
	// suspends would drop the first iteration's assistant turn.
	var accumulatedOutput []*llm.Message
	var accumulatedItems []*ResponseItem
	accumulatedUsage := &llm.Usage{}

generateLoop:
	genResult, err := a.generate(ctx, hctx, messages, systemPrompt, eventCallback, model)
	if err != nil {
		logger.Error("failed to generate response", "error", err)
		return nil, err
	}

	accumulatedOutput = append(accumulatedOutput, genResult.OutputMessages...)
	accumulatedItems = append(accumulatedItems, genResult.Items...)
	if genResult.Usage != nil {
		accumulatedUsage.Add(genResult.Usage)
	}

	response.FinishedAt = Ptr(time.Now())
	response.Usage = accumulatedUsage
	response.Items = accumulatedItems
	response.OutputMessages = accumulatedOutput

	// Merge any resume-phase items into the response, keeping chronological order.
	if len(resumeExtraItems) > 0 {
		response.Items = append(resumeExtraItems, response.Items...)
	}

	// Handle suspension from generate.
	if genResult.Suspended != nil {
		return a.finishSuspended(ctx, logger, hctx, response, inputMessages, genResult.Suspended, nil, eventCallback, sess, rs, false)
	}

	// Run Stop hooks before PostGeneration
	if len(a.hooks.Stop) > 0 {
		hctx.Response = response
		hctx.OutputMessages = accumulatedOutput
		hctx.Usage = accumulatedUsage
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
				// Inject reason as user message and re-enter generate loop.
				// The reason message becomes part of the conversation the LLM
				// sees, so it must also be accumulated onto the response /
				// saved turn so a subsequent suspend doesn't drop it.
				reasonMsg := llm.NewUserTextMessage(decision.Reason)
				messages = append(messages, genResult.OutputMessages...)
				messages = append(messages, reasonMsg)
				accumulatedOutput = append(accumulatedOutput, reasonMsg)
				response.OutputMessages = accumulatedOutput
				hctx.Messages = messages
				stopHookActive = true
				goto generateLoop
			}
		}
	}

	// Run PostGeneration hooks
	hctx.Response = response
	hctx.OutputMessages = accumulatedOutput
	hctx.Usage = accumulatedUsage
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

	// Save session turn. On resume, replace the suspended event with the
	// combined turn (pre-suspend turn messages plus new output). Otherwise
	// append a new turn with input + output. Persistence failures are fatal:
	// returning a successful Response while the session is out of sync would
	// strand the caller with state that doesn't match disk.
	//
	// On a resume completion we also populate Response.Suspension with the
	// final merged turn snapshot (PendingToolCalls = nil) so stateless
	// callers can flush the turn into their local history in one append
	// without reconciling a stale partial tool_result from their saved
	// state.
	if rs != nil {
		turnMsgs := make([]*llm.Message, 0, len(rs.TurnMessages)+len(response.OutputMessages))
		turnMsgs = append(turnMsgs, rs.TurnMessages...)
		turnMsgs = append(turnMsgs, response.OutputMessages...)
		switch {
		case suspendable != nil:
			if err := suspendable.SaveResumedTurn(ctx, turnMsgs, response.Usage); err != nil {
				logger.Error("session save error", "error", err)
				return nil, fmt.Errorf("save resumed turn: %w", err)
			}
		case sess != nil:
			// Plain session: the suspend never hit SaveTurn (only
			// SuspendableSessions auto-persist suspended turns), so this
			// resume completion is the first write for this turn. Append.
			if err := sess.SaveTurn(ctx, turnMsgs, response.Usage); err != nil {
				logger.Error("session save error", "error", err)
				return nil, fmt.Errorf("save turn: %w", err)
			}
		}
		response.Suspension = &SuspensionState{
			CompletedToolCalls: rs.CompletedToolCalls(),
			TurnMessages:       turnMsgs,
		}
	} else if sess != nil {
		turnMessages := make([]*llm.Message, 0, len(inputMessages)+len(response.OutputMessages))
		turnMessages = append(turnMessages, inputMessages...)
		turnMessages = append(turnMessages, response.OutputMessages...)
		if err := sess.SaveTurn(ctx, turnMessages, response.Usage); err != nil {
			logger.Error("session save error", "error", err)
			return nil, fmt.Errorf("save turn: %w", err)
		}
	}

	response.Status = ResponseStatusCompleted
	return response, nil
}

// finishSuspended populates the suspended response, runs OnSuspend and
// PostGeneration hooks, persists the suspended turn (if a
// SuspendableSession is present), and emits the terminal suspended stream
// item. Hooks run before persistence so a hook abort leaves the session
// untouched — no compensation needed.
//
// Suspension works without a session: when sess is nil or does not
// implement SuspendableSession, the Response.Suspension payload is still
// populated and returned to the caller, who is responsible for persisting
// history and state themselves.
//
// If skipSuspendNotifications is true, OnSuspend hooks and the terminal
// stream item are skipped. This is used for pure partial resumes, which
// continue an existing suspension rather than announcing a new one.
func (a *Agent) finishSuspended(
	ctx context.Context,
	logger llm.Logger,
	hctx *HookContext,
	response *Response,
	inputMessages []*llm.Message,
	snap *suspendedSnapshot,
	extraItems []*ResponseItem,
	callback EventCallback,
	sess Session,
	rs *resumeState,
	skipSuspendNotifications bool,
) (*Response, error) {
	if response.FinishedAt == nil {
		response.FinishedAt = Ptr(time.Now())
	}

	// Build the turn the caller will need on resume. For a generate-driven
	// suspend this is inputMessages + the assistant tool_use and any partial
	// tool_result. For a partial resume it is the existing turn plus any
	// tool_result updates captured in rs.
	var turnMsgs []*llm.Message
	if rs != nil {
		turnMsgs = append(turnMsgs, rs.TurnMessages...)
		turnMsgs = append(turnMsgs, response.OutputMessages...)
	} else {
		turnMsgs = append(turnMsgs, inputMessages...)
		turnMsgs = append(turnMsgs, response.OutputMessages...)
	}

	response.Status = ResponseStatusSuspended
	response.Suspension = &SuspensionState{
		PendingToolCalls:   snap.PendingToolCalls,
		CompletedToolCalls: snap.CompletedToolCalls,
		TurnMessages:       turnMsgs,
	}
	if len(extraItems) > 0 {
		response.Items = append(extraItems, response.Items...)
	}

	suspendable, _ := sess.(SuspendableSession)

	hctx.Response = response
	hctx.OutputMessages = response.OutputMessages
	hctx.Usage = response.Usage

	// Run OnSuspend hooks before PostGeneration and before persistence.
	// Aborting here leaves the session in its previous state.
	if !skipSuspendNotifications {
		for _, hook := range a.hooks.OnSuspend {
			if err := hook(ctx, hctx); err != nil {
				var abortErr *HookAbortError
				if errors.As(err, &abortErr) {
					abortErr.HookType = "OnSuspend"
					logger.Error("on-suspend hook aborted", "error", abortErr)
					return nil, abortErr
				}
				logger.Error("on-suspend hook error", "error", err)
			}
		}
	}

	// Run PostGeneration hooks (they see Status=Suspended). Still before
	// persistence so an abort cannot strand a saved suspended turn.
	for _, hook := range a.hooks.PostGeneration {
		if err := hook(ctx, hctx); err != nil {
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PostGeneration"
				logger.Error("post-generation hook aborted", "error", abortErr)
				return nil, abortErr
			}
			logger.Error("post-generation hook error", "error", err)
		}
	}

	// Persist the suspended turn only after hooks succeed, and only if the
	// caller opted into auto-persistence via a SuspendableSession. Plain
	// sessions and session-less callers rely on the Response.Suspension
	// payload to drive their own persistence.
	if suspendable != nil {
		if err := suspendable.SaveSuspendedTurn(ctx, turnMsgs, response.Usage, response.Suspension); err != nil {
			// If we can't persist the suspend, we must not return a
			// suspended Response with pending IDs that don't exist in the
			// session. Fail the call loudly.
			logger.Error("session save error", "error", err)
			return nil, fmt.Errorf("save suspended turn: %w", err)
		}
	}

	// Emit the terminal suspended stream item only after any persistence
	// succeeds and hooks have succeeded, so a stream consumer never sees a
	// suspended terminal for a call that ultimately returned an error.
	if !skipSuspendNotifications && callback != nil {
		if err := callback(ctx, &ResponseItem{
			Type:       ResponseItemTypeSuspended,
			Suspension: response.Suspension,
		}); err != nil {
			return nil, err
		}
	}

	return response, nil
}

// resumeState captures all information needed to resume a suspended session
// within a single CreateResponse call.
type resumeState struct {
	// TurnMessages is the set of messages that belong to the suspended turn
	// (the last session event's messages). During resume this is kept in
	// sync with any mutations to the tool_result message so that the final
	// SaveResumedTurn / SaveSuspendedTurn writes a consistent turn.
	TurnMessages []*llm.Message

	// SessionMessagesWithMerged is the full session history with the
	// suspended turn's tool_result message replaced (or appended) to hold
	// the merged tool_result content. This is what gets passed to generate.
	SessionMessagesWithMerged []*llm.Message

	// ToolResultMessageIdx is the index (within TurnMessages and within
	// SessionMessagesWithMerged's suspended-turn slice) of the merged
	// tool_result message. -1 if there was no tool_result message at
	// suspend time (rare — means all tools suspended before any completed).
	ToolResultMessageIdx int

	// AssistantToolUse is the last assistant message in the suspended turn
	// (the one with tool_use blocks).
	AssistantToolUse *llm.Message

	// NotStartedToolCalls are tool_use blocks from the assistant message
	// that neither completed nor suspended — they must be re-executed on
	// resume before the next LLM call.
	NotStartedToolCalls []*llm.ToolUseContent

	// CallerSupplied holds the results the caller provided via
	// WithToolResults, indexed by tool_use ID. Used to fire post hooks.
	CallerSupplied map[string]*ToolCallResult

	// PreviouslyCompleted lists tool calls that ran to completion in the
	// original (now-resumed) turn. Used to enrich partial-resume snapshots.
	PreviouslyCompleted []*CompletedToolCall

	// RemainingPending lists pending IDs the caller did NOT supply this
	// time. Non-empty means the resume is partial.
	RemainingPending []string

	// RemainingPendingCalls is the PendingToolCall list matching
	// RemainingPending, preserved from the original suspend's pending set.
	RemainingPendingCalls []*PendingToolCall
}

func (rs *resumeState) CompletedToolCalls() []*CompletedToolCall {
	return rs.PreviouslyCompleted
}

// AppendToolResults appends additional tool_result content blocks to the
// merged tool_result message. Used when not-started tools execute during
// resume and their results need to join the existing tool_result.
func (rs *resumeState) AppendToolResults(contents []*llm.ToolResultContent) {
	if rs.ToolResultMessageIdx < 0 {
		// No tool_result message existed; create one.
		toolResult := llm.NewToolResultMessage(contents...)
		rs.TurnMessages = append(rs.TurnMessages, toolResult)
		rs.SessionMessagesWithMerged = append(rs.SessionMessagesWithMerged, toolResult)
		rs.ToolResultMessageIdx = len(rs.TurnMessages) - 1
		return
	}
	msg := rs.TurnMessages[rs.ToolResultMessageIdx]
	for _, c := range contents {
		msg.Content = append(msg.Content, c)
	}
}

// AppendToolResultTextContent appends an auxiliary text content block (from
// hook AdditionalContext) to the merged tool_result message.
func (rs *resumeState) AppendToolResultTextContent(tc *llm.TextContent) {
	if rs.ToolResultMessageIdx < 0 || tc == nil {
		return
	}
	msg := rs.TurnMessages[rs.ToolResultMessageIdx]
	msg.Content = append(msg.Content, tc)
}

// UpdateToolResultContent updates the tool_result content block for the given
// tool_use ID with the (possibly hook-modified) ToolCallResult. This brings
// the merged message in sync with any mutations made by PostToolUse or
// PostToolUseFailure hooks during resume, and appends AdditionalContext as a
// trailing text block — the same contract executeOneToolCall honors.
func (rs *resumeState) UpdateToolResultContent(toolUseID string, result *ToolCallResult) {
	if rs.ToolResultMessageIdx < 0 || result == nil {
		return
	}
	msg := rs.TurnMessages[rs.ToolResultMessageIdx]

	// Find and replace the tool_result content block for this ID.
	var content any
	var isError bool
	if result.Result != nil {
		content = result.Result.Content
		isError = result.Result.IsError
	}
	isError = result.Error != nil || isError

	for i, c := range msg.Content {
		if trc, ok := c.(*llm.ToolResultContent); ok && trc.ToolUseID == toolUseID {
			msg.Content[i] = &llm.ToolResultContent{
				ToolUseID: toolUseID,
				Content:   content,
				IsError:   isError,
			}
			break
		}
	}

	// Append AdditionalContext as a trailing text block, matching the
	// normal path in executeOneToolCall / getAdditionalContextContent.
	if result.AdditionalContext != "" {
		msg.Content = append(msg.Content, &llm.TextContent{
			Text: result.AdditionalContext,
		})
	}
}

// prepareResume inspects the full history and caller-supplied tool results
// to build a resumeState. It validates invariants per FR-19 and returns
// descriptive errors.
func (a *Agent) prepareResume(fullHistory []*llm.Message, state *SuspensionState, toolResults map[string]*ToolResult) (*resumeState, error) {
	pendingCalls := state.PendingToolCalls
	pendingByID := make(map[string]*PendingToolCall, len(pendingCalls))
	pendingIDs := make([]string, len(pendingCalls))
	for i, pc := range pendingCalls {
		pendingByID[pc.ID] = pc
		pendingIDs[i] = pc.ID
	}
	// Validate caller-supplied IDs. Any ID not in the pending set — including
	// IDs that already have a completed tool_result in the persisted turn —
	// is rejected. The caller must reconcile their view of outstanding work
	// against the authoritative pending set.
	for id := range toolResults {
		if _, ok := pendingByID[id]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownPendingToolCall, id)
		}
	}
	pendingSet := make(map[string]bool, len(pendingIDs))
	for _, id := range pendingIDs {
		pendingSet[id] = true
	}

	turnLen := len(state.TurnMessages)
	if turnLen <= 0 || turnLen > len(fullHistory) {
		return nil, fmt.Errorf("dive: suspended state has invalid turn message count (%d)", turnLen)
	}
	turnStart := len(fullHistory) - turnLen
	// Copy turn messages so mutations don't leak into the caller's snapshot.
	turnMessages := make([]*llm.Message, turnLen)
	for i, msg := range fullHistory[turnStart:] {
		turnMessages[i] = msg
	}

	// Find the last assistant message with tool_use blocks and any
	// trailing tool_result message within the turn.
	assistantIdx := -1
	toolResultIdx := -1
	for i := len(turnMessages) - 1; i >= 0; i-- {
		msg := turnMessages[i]
		if msg.Role == llm.User && hasToolResultContent(msg) {
			if toolResultIdx < 0 {
				toolResultIdx = i
			}
			continue
		}
		if msg.Role == llm.Assistant && hasToolUseContent(msg) {
			assistantIdx = i
			break
		}
	}
	if assistantIdx < 0 {
		return nil, fmt.Errorf("dive: suspended session has no assistant tool_use message in the last turn")
	}

	assistant := turnMessages[assistantIdx]
	toolUseIDs := collectToolUseIDs(assistant)

	// Existing tool_result IDs (if any).
	existingResults := make(map[string]*llm.ToolResultContent)
	if toolResultIdx >= 0 {
		for _, c := range turnMessages[toolResultIdx].Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				existingResults[trc.ToolUseID] = trc
			}
		}
	}

	// Compute not-started tool calls (assistant IDs minus completed minus pending).
	var notStarted []*llm.ToolUseContent
	for _, toolUse := range toolUseContents(assistant) {
		if _, done := existingResults[toolUse.ID]; done {
			continue
		}
		if pendingSet[toolUse.ID] {
			continue
		}
		notStarted = append(notStarted, toolUse)
	}

	// Build merged tool_result message: existing content + caller-supplied for pending.
	var mergedContent []*llm.ToolResultContent
	var mergedAux []llm.Content // trailing text content from hooks, preserved
	if toolResultIdx >= 0 {
		for _, c := range turnMessages[toolResultIdx].Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				mergedContent = append(mergedContent, trc)
			} else {
				mergedAux = append(mergedAux, c)
			}
		}
	}
	callerSupplied := make(map[string]*ToolCallResult)
	for id, result := range toolResults {
		toolUse := findToolUseByID(assistant, id)
		name := ""
		var input json.RawMessage
		if toolUse != nil {
			name = toolUse.Name
			input = toolUse.Input
		}
		callerSupplied[id] = &ToolCallResult{
			ID:     id,
			Name:   name,
			Input:  input,
			Result: result,
		}
		isError := result != nil && result.IsError
		var content any
		if result != nil {
			content = result.Content
		}
		mergedContent = append(mergedContent, &llm.ToolResultContent{
			ToolUseID: id,
			Content:   content,
			IsError:   isError,
		})
	}

	// Build the merged tool_result message. When there are not-started tool
	// calls we will append their results during resume via AppendToolResults;
	// to keep the `messages` slice passed to generate in sync with those
	// mutations (which must mutate an existing shared pointer rather than
	// append a new one), ensure a tool_result message exists up front.
	var mergedMessage *llm.Message
	newToolResultIdx := toolResultIdx
	needPlaceholder := toolResultIdx < 0 && len(notStarted) > 0
	if len(mergedContent) > 0 || needPlaceholder {
		mergedMessage = llm.NewToolResultMessage(mergedContent...)
		for _, aux := range mergedAux {
			mergedMessage.Content = append(mergedMessage.Content, aux)
		}
		if toolResultIdx >= 0 {
			turnMessages[toolResultIdx] = mergedMessage
		} else {
			turnMessages = append(turnMessages, mergedMessage)
			newToolResultIdx = len(turnMessages) - 1
		}
	}

	// Build the full history list with the merged turn.
	sessionWithMerged := make([]*llm.Message, 0, turnStart+len(turnMessages))
	sessionWithMerged = append(sessionWithMerged, fullHistory[:turnStart]...)
	sessionWithMerged = append(sessionWithMerged, turnMessages...)

	// Previously-completed calls from the incoming SuspensionState.
	// Prefer the rich CompletedToolCall entries (which carry Result and
	// Error) over reconstructing lossy versions from message content
	// blocks. Fall back to message-based reconstruction only for IDs not
	// found in the state — shouldn't happen in practice, but keeps the
	// code robust against incomplete snapshots.
	priorCompletedByID := make(map[string]*CompletedToolCall, len(state.CompletedToolCalls))
	for _, cc := range state.CompletedToolCalls {
		priorCompletedByID[cc.ID] = cc
	}
	var previouslyCompleted []*CompletedToolCall
	for _, id := range toolUseIDs {
		if _, ok := existingResults[id]; !ok {
			continue
		}
		if cc, found := priorCompletedByID[id]; found {
			previouslyCompleted = append(previouslyCompleted, cc)
			continue
		}
		// Fallback: reconstruct from the assistant message (lossy — no
		// Result/Error). This path is only hit if the SuspensionState's
		// CompletedToolCalls is incomplete.
		toolUse := findToolUseByID(assistant, id)
		if toolUse == nil {
			continue
		}
		previouslyCompleted = append(previouslyCompleted, &CompletedToolCall{
			ID:    id,
			Name:  toolUse.Name,
			Input: toolUse.Input,
		})
	}

	// Compute remaining pending (pending minus caller-supplied). Pull
	// Prompt/Metadata from the persisted PendingCall so partial-resume-again
	// and cross-process flows preserve the original SuspendResult payload.
	var remaining []string
	var remainingCalls []*PendingToolCall
	for _, id := range pendingIDs {
		if _, supplied := toolResults[id]; supplied {
			continue
		}
		remaining = append(remaining, id)
		pc := pendingByID[id]
		input := pc.Input
		name := pc.Name
		if toolUse := findToolUseByID(assistant, id); toolUse != nil {
			// Prefer the live tool_use input if it's present — it is
			// authoritative for the call shape; Name should match either way.
			if len(toolUse.Input) > 0 {
				input = toolUse.Input
			}
			if name == "" {
				name = toolUse.Name
			}
		}
		remainingCalls = append(remainingCalls, &PendingToolCall{
			ID:       id,
			Name:     name,
			Input:    input,
			Prompt:   pc.Prompt,
			Reason:   pc.Reason,
			Metadata: pc.Metadata,
		})
	}

	return &resumeState{
		TurnMessages:              turnMessages,
		SessionMessagesWithMerged: sessionWithMerged,
		ToolResultMessageIdx:      newToolResultIdx,
		AssistantToolUse:          assistant,
		NotStartedToolCalls:       notStarted,
		CallerSupplied:            callerSupplied,
		PreviouslyCompleted:       previouslyCompleted,
		RemainingPending:          remaining,
		RemainingPendingCalls:     remainingCalls,
	}, nil
}

// fireResumePostHooks fires PostToolUse or PostToolUseFailure for each
// caller-supplied result on resume, mirroring the contract used for tools
// that run in-process. Hooks fire in the order the tool_use blocks appeared
// in the suspended assistant message so ordering is deterministic.
//
// toolsByName is used to populate HookContext.Tool, matching the normal
// execution path contract. In cross-process scenarios the tool may not exist
// in the current agent's registry; Tool is nil in that case (best-effort).
//
// After hooks fire, any mutations to postHctx.Result and
// postHctx.AdditionalContext are propagated back into the merged tool_result
// message on rs, keeping resume behaviorally equivalent to the normal tool
// execution path.
func (a *Agent) fireResumePostHooks(ctx context.Context, hctx *HookContext, rs *resumeState, toolsByName map[string]Tool) error {
	if len(rs.CallerSupplied) == 0 {
		return nil
	}
	// Walk assistant tool_use blocks in original order so PostToolUse hooks
	// fire deterministically instead of in random map-iteration order.
	for _, toolUseID := range collectToolUseIDs(rs.AssistantToolUse) {
		result, ok := rs.CallerSupplied[toolUseID]
		if !ok {
			continue
		}
		failed := result.Result != nil && result.Result.IsError
		postHctx := &HookContext{
			Agent:        a,
			Session:      hctx.Session,
			Values:       hctx.Values,
			SystemPrompt: hctx.SystemPrompt,
			Messages:     hctx.Messages,
			Tool:         toolsByName[result.Name], // nil if tool not in registry (cross-process)
			Call: &llm.ToolUseContent{
				ID:    result.ID,
				Name:  result.Name,
				Input: rawOrEmpty(result.Input),
			},
			Result: result,
		}
		if failed {
			for _, hook := range a.hooks.PostToolUseFailure {
				if err := hook(ctx, postHctx); err != nil {
					var abortErr *HookAbortError
					if errors.As(err, &abortErr) {
						abortErr.HookType = "PostToolUseFailure"
						a.logger.Error("post-tool-use-failure hook aborted", "error", abortErr)
						return abortErr
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
						return abortErr
					}
					a.logger.Debug("post-tool-use hook error", "error", err)
				}
			}
		}

		// Propagate hook mutations back into the caller-supplied result and
		// the merged tool_result message, mirroring the normal execution path
		// in executeOneToolCall which reads postHctx.Result and
		// postHctx.AdditionalContext after hooks fire.
		result = postHctx.Result
		rs.CallerSupplied[toolUseID] = result

		if postHctx.AdditionalContext != "" {
			result.AdditionalContext = postHctx.AdditionalContext
		}

		// Update the corresponding tool_result content block in the merged
		// message so the LLM sees the hook-modified result.
		rs.UpdateToolResultContent(toolUseID, result)
	}
	return nil
}

// hasToolUseContent reports whether a message contains any tool_use blocks.
func hasToolUseContent(msg *llm.Message) bool {
	for _, c := range msg.Content {
		if _, ok := c.(*llm.ToolUseContent); ok {
			return true
		}
	}
	return false
}

// hasToolResultContent reports whether a message contains any tool_result blocks.
func hasToolResultContent(msg *llm.Message) bool {
	for _, c := range msg.Content {
		if _, ok := c.(*llm.ToolResultContent); ok {
			return true
		}
	}
	return false
}

// collectToolUseIDs returns the IDs of all tool_use blocks in a message,
// preserving their original order.
func collectToolUseIDs(msg *llm.Message) []string {
	var ids []string
	for _, c := range msg.Content {
		if tu, ok := c.(*llm.ToolUseContent); ok {
			ids = append(ids, tu.ID)
		}
	}
	return ids
}

// toolUseContents returns all tool_use blocks in a message, in order.
func toolUseContents(msg *llm.Message) []*llm.ToolUseContent {
	var out []*llm.ToolUseContent
	for _, c := range msg.Content {
		if tu, ok := c.(*llm.ToolUseContent); ok {
			out = append(out, tu)
		}
	}
	return out
}

// findToolUseByID looks up a tool_use block by ID within a message.
func findToolUseByID(msg *llm.Message, id string) *llm.ToolUseContent {
	for _, c := range msg.Content {
		if tu, ok := c.(*llm.ToolUseContent); ok && tu.ID == id {
			return tu
		}
	}
	return nil
}

// rawOrEmpty returns input unchanged, or an empty JSON object if nil.
func rawOrEmpty(input any) json.RawMessage {
	switch v := input.(type) {
	case json.RawMessage:
		return v
	case []byte:
		return json.RawMessage(v)
	case nil:
		return json.RawMessage("{}")
	default:
		return json.RawMessage("{}")
	}
}

// prepareMessages returns the messages from the provided options.
func (a *Agent) prepareMessages(options CreateResponseOptions) []*llm.Message {
	return options.Messages
}

// generate runs the LLM generation and tool execution loop. It handles the
// interaction between the agent and the LLM, including tool calls. Returns the
// final LLM response, updated messages, and any error that occurred.
func (a *Agent) generate(ctx context.Context, hctx *HookContext, messages []*llm.Message, systemPrompt string, callback EventCallback, model llm.LLM) (*generateResult, error) {

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

		// Open chat span before invoking the model. The returned ctx carries
		// the span so any HTTP-client middleware (e.g. otelhttp) the provider
		// installs nests under it.
		_, streaming := model.(llm.StreamingLLM)
		infoCfg := &llm.Config{}
		infoCfg.Apply(iterOpts...)
		chatCtx, chatSpan := a.tracer.StartChat(ctx, ChatInfo{
			Agent:            a,
			Session:          hctx.Session,
			Model:            infoCfg.Model,
			Streaming:        streaming,
			MaxTokens:        infoCfg.MaxTokens,
			Temperature:      infoCfg.Temperature,
			FrequencyPenalty: infoCfg.FrequencyPenalty,
			PresencePenalty:  infoCfg.PresencePenalty,
			SystemPrompt:     systemPrompt,
			Messages:         updatedMessages,
			Iteration:        i,
		})

		var err error
		var response *llm.Response
		var ttfc float64
		if streamingLLM, ok := model.(llm.StreamingLLM); ok {
			response, ttfc, err = a.generateStreaming(chatCtx, streamingLLM, iterOpts, collectingCallback)
		} else {
			response, err = model.Generate(chatCtx, iterOpts...)
		}
		if err == nil && response == nil {
			// This indicates a bug in the LLM provider implementation
			err = ErrLLMNoResponse
		}
		if response != nil {
			chatSpan.SetResponse(response)
		}
		if ttfc > 0 {
			chatSpan.SetTimeToFirstChunk(ttfc)
		}
		chatSpan.End(err)
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
		batch, err := a.executeToolCalls(ctx, hctx, toolCalls, toolsByName, collectingCallback)
		if err != nil {
			return nil, err
		}

		// Build the tool_result message from completed outcomes only. On a
		// suspended batch, this is the PARTIAL tool_result that gets persisted
		// to the session for later merging with caller-supplied results.
		completedResults := batch.Completed()
		var toolResultMessage *llm.Message
		if len(completedResults) > 0 {
			toolResultMessage = llm.NewToolResultMessage(getToolResultContent(completedResults)...)
			for _, tc := range getAdditionalContextContent(completedResults) {
				toolResultMessage.Content = append(toolResultMessage.Content, tc)
			}
			newMessage(toolResultMessage)
		}

		if batch.Suspended {
			snapshot := buildSuspendedSnapshot(toolCalls, batch)
			return &generateResult{
				OutputMessages: outputMessages,
				Items:          items,
				Usage:          totalUsage,
				Suspended:      snapshot,
			}, nil
		}

		// If no tools actually ran (all denied/suspended and skipped), we
		// still need a tool_result message to feed back to the LLM. The
		// "all denied" case was already handled above; the "all suspended"
		// case has returned early. So if we get here with no completed
		// results, something is off — but guard defensively.
		if toolResultMessage == nil {
			toolResultMessage = llm.NewToolResultMessage()
			newMessage(toolResultMessage)
		}

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
// Returns the accumulated response, the time-to-first-chunk in seconds
// (zero for pre-first-chunk failures), and any error.
func (a *Agent) generateStreaming(
	ctx context.Context,
	streamingLLM llm.StreamingLLM,
	generateOpts []llm.Option,
	callback EventCallback,
) (*llm.Response, float64, error) {
	accum := llm.NewResponseAccumulator()
	streamStart := time.Now()
	iter, err := streamingLLM.Stream(ctx, generateOpts...)
	if err != nil {
		return nil, 0, err
	}
	defer iter.Close()

	var ttfc float64
	for iter.Next() {
		event := iter.Event()
		if ttfc == 0 && eventHasContent(event) {
			ttfc = time.Since(streamStart).Seconds()
		}
		if err := accum.AddEvent(event); err != nil {
			return nil, ttfc, err
		}
		if err := callback(ctx, &ResponseItem{
			Type:  ResponseItemTypeModelEvent,
			Event: event,
		}); err != nil {
			return nil, ttfc, err
		}
	}
	if err := iter.Err(); err != nil {
		return nil, ttfc, err
	}
	return accum.Response(), ttfc, nil
}

// eventHasContent reports whether the event carries assistant content
// (text delta, populated content block, or input_json delta). Used to
// detect the first chunk for time-to-first-chunk telemetry. Lifecycle
// events without payload (message_start, ping) don't count.
func eventHasContent(event *llm.Event) bool {
	if event == nil {
		return false
	}
	switch event.Type {
	case llm.EventTypeContentBlockDelta:
		if event.Delta == nil {
			return false
		}
		return event.Delta.Text != "" || event.Delta.PartialJSON != "" || event.Delta.Thinking != ""
	case llm.EventTypeContentBlockStart:
		if event.ContentBlock == nil {
			return false
		}
		return event.ContentBlock.Text != "" || event.ContentBlock.Name != "" || event.ContentBlock.Input != nil
	}
	return false
}

// executeToolCalls executes all tool calls and returns the tool call results.
// PreToolUse hooks run in order for each call. If any hook returns an error,
// the tool is denied. If all hooks return nil, the tool is executed.
//
// When parallelToolExecution is enabled on the agent, tool calls are executed
// concurrently using a three-phase approach: PreToolUse hooks run sequentially,
// then tool executions run in parallel, then PostToolUse hooks and result
// events run sequentially. This keeps hooks single-threaded while parallelizing
// the expensive tool execution.
func (a *Agent) executeToolCalls(
	ctx context.Context,
	hctx *HookContext,
	toolCalls []*llm.ToolUseContent,
	toolsByName map[string]Tool,
	callback EventCallback,
) (*toolBatchResult, error) {
	if a.parallelToolExecution && len(toolCalls) > 1 {
		return a.executeToolCallsParallel(ctx, hctx, toolCalls, toolsByName, callback)
	}
	return a.executeToolCallsSequential(ctx, hctx, toolCalls, toolsByName, callback)
}

// executeToolCallsSequential executes tool calls one at a time in order.
// If any tool returns a SuspendResult, the remaining trailing tool calls are
// NOT executed; their outcomes stay zero-valued ("not started") and are
// re-scheduled on resume.
func (a *Agent) executeToolCallsSequential(
	ctx context.Context,
	hctx *HookContext,
	toolCalls []*llm.ToolUseContent,
	toolsByName map[string]Tool,
	callback EventCallback,
) (*toolBatchResult, error) {
	batch := &toolBatchResult{Outcomes: make([]toolCallOutcome, len(toolCalls))}
	for i, toolCall := range toolCalls {
		result, err := a.executeOneToolCall(ctx, hctx, toolCall, toolsByName, callback)
		if err != nil {
			return nil, err
		}
		if result != nil && result.Result != nil && result.Result.Suspend != nil {
			batch.Outcomes[i] = toolCallOutcome{
				Pending: toPendingToolCall(toolCall, result.Result.Suspend),
			}
			batch.Suspended = true
			return batch, nil
		}
		batch.Outcomes[i] = toolCallOutcome{Result: result}
	}
	return batch, nil
}

// toolCallPrep holds the result of the PreToolUse phase for a single tool call.
type toolCallPrep struct {
	tool    Tool
	preview *ToolCallPreview
	preHctx *HookContext
	denied  bool
	input   []byte
}

// executeToolCallsParallel uses a two-phase approach:
//
//	Phase 1 (sequential): PreToolUse hooks, previews, and tool_call events
//	Phase 2 (parallel):   Tool execution with streamed results
//
// Tools execute concurrently but results are processed as they arrive via a
// channel. PostToolUse hooks and callbacks fire as soon as each tool completes,
// rather than waiting for all tools to finish. Hooks and callbacks remain
// single-threaded since a single goroutine drains the channel.
//
// Note: ToolCallResult events and PostToolUse hooks fire in completion order,
// not tool-call declaration order. The results slice is indexed correctly
// regardless of completion order.
func (a *Agent) executeToolCallsParallel(
	ctx context.Context,
	hctx *HookContext,
	toolCalls []*llm.ToolUseContent,
	toolsByName map[string]Tool,
	callback EventCallback,
) (*toolBatchResult, error) {

	batch := &toolBatchResult{Outcomes: make([]toolCallOutcome, len(toolCalls))}
	deniedResults := make([]*ToolCallResult, len(toolCalls))

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Phase 1: PreToolUse hooks (sequential)
	preps := make([]toolCallPrep, len(toolCalls))
	for i, toolCall := range toolCalls {
		tool, ok := toolsByName[toolCall.Name]
		if !ok {
			return nil, fmt.Errorf("tool call error: unknown tool %q", toolCall.Name)
		}

		a.logger.Debug("executing tool call",
			"tool_id", toolCall.ID,
			"tool_name", toolCall.Name,
			"tool_input", string(toolCall.Input))

		var preview *ToolCallPreview
		if previewer, ok := tool.(ToolPreviewer); ok {
			preview = previewer.PreviewCall(childCtx, toolCall.Input)
		}

		if err := callback(childCtx, &ResponseItem{
			Type:     ResponseItemTypeToolCall,
			ToolCall: toolCall,
		}); err != nil {
			return nil, err
		}

		preHctx := &HookContext{
			Agent:        a,
			Session:      hctx.Session,
			Values:       hctx.Values,
			SystemPrompt: hctx.SystemPrompt,
			Messages:     hctx.Messages,
			Tool:         tool,
			Call:         toolCall,
		}

		var denialErr error
		for _, hook := range a.hooks.PreToolUse {
			if err := hook(childCtx, preHctx); err != nil {
				var abortErr *HookAbortError
				if errors.As(err, &abortErr) {
					abortErr.HookType = "PreToolUse"
					a.logger.Error("pre-tool-use hook aborted", "error", abortErr)
					return nil, abortErr
				}
				if denialErr == nil {
					denialErr = err
				}
				a.logger.Debug("pre-tool-use hook denied tool", "error", err)
			}
		}

		prep := toolCallPrep{
			tool:    tool,
			preview: preview,
			preHctx: preHctx,
			input:   toolCall.Input,
		}
		if denialErr != nil {
			prep.denied = true
			deniedResults[i] = a.createDeniedResult(toolCall, denialErr.Error(), preview)
		} else if preHctx.UpdatedInput != nil {
			prep.input = preHctx.UpdatedInput
		}
		preps[i] = prep
	}

	// Phase 2: Tool execution (parallel) with streamed results
	type completedTool struct {
		index  int
		result *ToolCallResult
		err    error // fatal error (e.g. context cancellation)
	}

	ch := make(chan completedTool, len(toolCalls))

	// Send denied results immediately — no goroutine needed.
	for i, prep := range preps {
		if prep.denied {
			ch <- completedTool{index: i, result: deniedResults[i]}
		}
	}

	// Launch tool executions.
	for i, prep := range preps {
		if prep.denied {
			continue
		}
		go func() {
			if err := childCtx.Err(); err != nil {
				ch <- completedTool{index: i, err: err}
				return
			}
			toolCtx, toolSpan := a.tracer.StartToolCall(childCtx, ToolCallInfo{
				Agent:   a,
				Session: hctx.Session,
				Tool:    prep.tool,
				Call:    toolCalls[i],
			})
			result := a.executeTool(toolCtx, prep.tool, toolCalls[i], prep.input, prep.preview, callback)
			toolSpan.SetResult(result)
			if result != nil && result.Error != nil {
				toolSpan.End(result.Error)
			} else {
				toolSpan.End(nil)
			}
			if result.Error != nil && childCtx.Err() != nil {
				ch <- completedTool{index: i, err: childCtx.Err()}
				return
			}
			ch <- completedTool{index: i, result: result}
		}()
	}

	// Drain results as they arrive (single-threaded: hooks + callbacks are safe).
	// On a suspend, we do NOT cancel childCtx — still-running siblings must
	// complete so their results can be recorded in the partial tool_result.
	remaining := len(toolCalls)
	for remaining > 0 {
		ct := <-ch
		remaining--

		if ct.err != nil {
			cancel() // cancel remaining tools
			return nil, ct.err
		}

		i := ct.index
		result := ct.result
		prep := preps[i]

		// Suspend path: skip PostToolUse hooks but still emit a tool_call_result
		// event so stream consumers can see the suspend signal.
		if result != nil && result.Result != nil && result.Result.Suspend != nil {
			batch.Outcomes[i] = toolCallOutcome{
				Pending: toPendingToolCall(toolCalls[i], result.Result.Suspend),
			}
			batch.Suspended = true
			if err := callback(ctx, &ResponseItem{
				Type:           ResponseItemTypeToolCallResult,
				ToolCallResult: result,
			}); err != nil {
				return nil, err
			}
			continue
		}

		failed := result.Error != nil || (result.Result != nil && result.Result.IsError)

		postHctx := &HookContext{
			Agent:        prep.preHctx.Agent,
			Session:      prep.preHctx.Session,
			Values:       prep.preHctx.Values,
			SystemPrompt: prep.preHctx.SystemPrompt,
			Messages:     prep.preHctx.Messages,
			Tool:         prep.tool,
			Call:         toolCalls[i],
			Result:       result,
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

		result = postHctx.Result

		additionalContext := prep.preHctx.AdditionalContext
		if postHctx.AdditionalContext != "" {
			if additionalContext != "" {
				additionalContext += "\n"
			}
			additionalContext += postHctx.AdditionalContext
		}
		if additionalContext != "" {
			result.AdditionalContext = additionalContext
		}

		batch.Outcomes[i] = toolCallOutcome{Result: result}

		if err := callback(ctx, &ResponseItem{
			Type:           ResponseItemTypeToolCallResult,
			ToolCallResult: result,
		}); err != nil {
			return nil, err
		}
	}

	return batch, nil
}

// executeOneToolCall executes a single tool call including hooks and callbacks.
// Used by the sequential path only.
func (a *Agent) executeOneToolCall(
	ctx context.Context,
	hctx *HookContext,
	toolCall *llm.ToolUseContent,
	toolsByName map[string]Tool,
	callback EventCallback,
) (*ToolCallResult, error) {
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

	preHctx := &HookContext{
		Agent:        a,
		Session:      hctx.Session,
		Values:       hctx.Values,
		SystemPrompt: hctx.SystemPrompt,
		Messages:     hctx.Messages,
		Tool:         tool,
		Call:         toolCall,
	}

	// Run PreToolUse hooks — any error denies the tool. All hooks run even
	// if an earlier one denies; only HookAbortError short-circuits.
	var result *ToolCallResult
	var denialErr error
	for _, hook := range a.hooks.PreToolUse {
		if err := hook(ctx, preHctx); err != nil {
			var abortErr *HookAbortError
			if errors.As(err, &abortErr) {
				abortErr.HookType = "PreToolUse"
				a.logger.Error("pre-tool-use hook aborted", "error", abortErr)
				return nil, abortErr
			}
			if denialErr == nil {
				denialErr = err
			}
			a.logger.Debug("pre-tool-use hook denied tool", "error", err)
		}
	}

	if denialErr != nil {
		result = a.createDeniedResult(toolCall, denialErr.Error(), preview)
	} else {
		input := toolCall.Input
		if preHctx.UpdatedInput != nil {
			input = preHctx.UpdatedInput
		}
		toolCtx, toolSpan := a.tracer.StartToolCall(ctx, ToolCallInfo{
			Agent:   a,
			Session: hctx.Session,
			Tool:    tool,
			Call:    toolCall,
		})
		result = a.executeTool(toolCtx, tool, toolCall, input, preview, callback)
		toolSpan.SetResult(result)
		if result != nil && result.Error != nil {
			toolSpan.End(result.Error)
		} else {
			toolSpan.End(nil)
		}
	}

	// Suspend path: emit the tool_call_result event but skip PostToolUse
	// hooks. The caller inspects result.Result.Suspend to classify as pending.
	if result != nil && result.Result != nil && result.Result.Suspend != nil {
		if err := callback(ctx, &ResponseItem{
			Type:           ResponseItemTypeToolCallResult,
			ToolCallResult: result,
		}); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Determine if the tool call failed
	failed := result.Error != nil || (result.Result != nil && result.Result.IsError)

	// Build postHctx sharing Values with preHctx so that mutations from
	// PreToolUse hooks are visible in PostToolUse hooks.
	postHctx := &HookContext{
		Agent:        preHctx.Agent,
		Session:      preHctx.Session,
		Values:       preHctx.Values,
		SystemPrompt: preHctx.SystemPrompt,
		Messages:     preHctx.Messages,
		Tool:         tool,
		Call:         toolCall,
		Result:       result,
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
	return result, nil
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
	callback EventCallback,
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

	// Inject tool call ID and streaming function into context
	toolCtx := WithToolCallID(ctx, call.ID)
	if callback != nil {
		toolCtx = WithToolStreamFunc(toolCtx, func(toolCallID, text string) {
			_ = callback(ctx, &ResponseItem{
				Type: ResponseItemTypeToolStream,
				ToolStream: &ToolStreamEvent{
					ToolCallID: toolCallID,
					Text:       text,
				},
			})
		})
	}

	output, err := tool.Call(toolCtx, input)
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
	// Validate ToolResult.Suspend mutual exclusion (M3). Suspend and the
	// regular result fields are a tagged union; setting both is a bug on
	// the tool author's side. Surface it as a normal IsError result so the
	// agent converges instead of panicking, and so PostToolUseFailure hooks
	// fire just like any other tool error.
	if output.Suspend != nil && (len(output.Content) > 0 || output.Display != "" || output.IsError) {
		msg := fmt.Sprintf(
			"Tool %s returned a SuspendResult together with Content/Display/IsError; these fields are mutually exclusive.",
			tool.Name(),
		)
		a.logger.Error("tool returned malformed suspend result",
			"tool", tool.Name(),
			"has_content", len(output.Content) > 0,
			"has_display", output.Display != "",
			"is_error", output.IsError,
		)
		return &ToolCallResult{
			ID:      call.ID,
			Name:    call.Name,
			Input:   call.Input,
			Preview: preview,
			Result: &ToolResult{
				Content: []*ToolResultContent{
					{Type: ToolResultContentTypeText, Text: msg},
				},
				IsError: true,
			},
			Error: errors.New(msg),
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

	// Suspended is non-nil if the terminal iteration of the loop unwound
	// because at least one tool returned SuspendResult. CreateResponse uses
	// this to persist the partial turn and return a suspended Response.
	Suspended *suspendedSnapshot
}

// suspendedSnapshot describes the state captured when generate() returns
// early due to a tool suspension.
type suspendedSnapshot struct {
	PendingToolCalls   []*PendingToolCall
	CompletedToolCalls []*CompletedToolCall
}

// toolCallOutcome is the per-tool-call result of an executeToolCalls batch.
// On a successful return, at most one of Result or Pending is non-nil. If
// both are nil, the tool call was "not started" (sequential path unwound
// early due to an earlier sibling suspending) and must be re-scheduled on
// resume.
type toolCallOutcome struct {
	Result  *ToolCallResult
	Pending *PendingToolCall
}

// toolBatchResult aggregates per-call outcomes for one LLM iteration.
type toolBatchResult struct {
	Outcomes  []toolCallOutcome
	Suspended bool
}

// Completed returns a slice of ToolCallResult for outcomes that completed
// normally, in original (input) order. Used to build the tool_result message.
func (b *toolBatchResult) Completed() []*ToolCallResult {
	var out []*ToolCallResult
	for _, o := range b.Outcomes {
		if o.Result != nil {
			out = append(out, o.Result)
		}
	}
	return out
}

// buildSuspendedSnapshot constructs the snapshot returned from generate when
// a tool batch suspended. "Not started" outcomes (sequential skipped) are
// intentionally omitted — they are re-scheduled from the assistant tool_use
// blocks on resume, not carried in the suspended response.
func buildSuspendedSnapshot(toolCalls []*llm.ToolUseContent, batch *toolBatchResult) *suspendedSnapshot {
	snap := &suspendedSnapshot{}
	for i, o := range batch.Outcomes {
		switch {
		case o.Pending != nil:
			snap.PendingToolCalls = append(snap.PendingToolCalls, o.Pending)
		case o.Result != nil:
			completed := &CompletedToolCall{
				ID:     o.Result.ID,
				Name:   o.Result.Name,
				Input:  toolCalls[i].Input,
				Result: o.Result.Result,
			}
			if o.Result.Error != nil {
				completed.Error = o.Result.Error.Error()
			}
			snap.CompletedToolCalls = append(snap.CompletedToolCalls, completed)
		}
	}
	return snap
}

// toPendingToolCall builds a PendingToolCall from a tool_use block and a
// SuspendResult returned by the tool.
func toPendingToolCall(toolCall *llm.ToolUseContent, sr *SuspendResult) *PendingToolCall {
	p := &PendingToolCall{
		ID:    toolCall.ID,
		Name:  toolCall.Name,
		Input: toolCall.Input,
	}
	if sr != nil {
		p.Prompt = sr.Prompt
		p.Reason = sr.Reason
		p.Metadata = sr.Metadata
	}
	return p
}
