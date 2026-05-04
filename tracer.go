package dive

import (
	"context"

	"github.com/deepnoodle-ai/dive/llm"
)

// Tracer observes an agent's lifecycle. The agent calls the Start* methods at
// each lifecycle boundary and threads the returned context into the operation
// that follows, so spans created by the tracer naturally nest under their
// caller.
//
// Tracer is the right shape for observation (tracing, metrics, audit logging).
// It is intentionally separate from the hook system, which handles
// modification (rewriting prompts, denying tool calls, injecting context).
//
// Implementations live in adapter packages — see experimental/otel for the
// OpenTelemetry adapter. NopTracer is the default.
type Tracer interface {
	// StartAgentRun is called once per CreateResponse. Implementations should
	// open a root operation span (or equivalent) and return a derived ctx that
	// carries it. The agent threads that ctx into every downstream call so
	// chat and tool spans nest under it.
	StartAgentRun(ctx context.Context, info AgentRunInfo) (context.Context, AgentRunSpan)

	// StartChat is called once per LLM iteration, before the model is invoked.
	// The returned ctx is passed into LLM.Generate / LLM.Stream so any spans
	// the provider's HTTP client emits (e.g. via otelhttp middleware) nest
	// under the chat span.
	StartChat(ctx context.Context, info ChatInfo) (context.Context, ChatSpan)

	// StartToolCall is called once per tool invocation, before Tool.Call. The
	// returned ctx is passed into Tool.Call so any spans the tool's internals
	// emit nest under the execute_tool span.
	StartToolCall(ctx context.Context, info ToolCallInfo) (context.Context, ToolCallSpan)
}

// AgentRunInfo describes an agent run at the moment it begins.
type AgentRunInfo struct {
	// Agent is the agent handling the run.
	Agent *Agent

	// Session is the active session for this run (per-call WithSession overrides
	// AgentOptions.Session). Nil for stateless calls.
	Session Session
}

// ChatInfo describes a single LLM iteration at the moment it begins.
type ChatInfo struct {
	// Agent is the agent handling the run.
	Agent *Agent

	// Session is the active session, or nil for stateless calls.
	Session Session

	// Model is the model name configured for this iteration. May be empty if
	// the provider applies its own default.
	Model string

	// Streaming reports whether the agent dispatched this iteration through
	// LLM.Stream (true) or LLM.Generate (false).
	Streaming bool

	// MaxTokens, Temperature, FrequencyPenalty, PresencePenalty mirror the
	// matching llm.Config fields when set. Nil means "unset / provider default."
	MaxTokens        *int
	Temperature      *float64
	FrequencyPenalty *float64
	PresencePenalty  *float64

	// SystemPrompt is the system prompt sent for this iteration.
	SystemPrompt string

	// Messages is the conversation history sent for this iteration. Tracers
	// that emit message payloads (e.g. gen_ai.input.messages) read this; the
	// slice MUST NOT be mutated.
	Messages []*llm.Message

	// Iteration is the zero-based iteration number within the agent run.
	Iteration int
}

// ToolCallInfo describes a tool invocation at the moment it begins.
type ToolCallInfo struct {
	// Agent is the agent handling the run.
	Agent *Agent

	// Session is the active session, or nil for stateless calls.
	Session Session

	// Tool is the tool that will be invoked.
	Tool Tool

	// Call carries the tool name, call ID, and JSON arguments.
	Call *llm.ToolUseContent
}

// AgentRunSpan represents an in-flight agent-run observation. The agent calls
// End exactly once.
type AgentRunSpan interface {
	// SetResponse records the final response. Called before End on the
	// happy path; not called when the agent fails before producing one.
	SetResponse(*Response)

	// SetUsage records aggregate token usage across all iterations of the
	// run. Called before End on the happy path.
	SetUsage(*llm.Usage)

	// End marks the run as finished. err is non-nil only if the run failed
	// outright; a suspended response is not an error.
	End(err error)
}

// ChatSpan represents an in-flight chat-iteration observation. The agent
// calls End exactly once.
type ChatSpan interface {
	// SetResponse records the LLM response. Called before End on the happy
	// path; not called when Generate / Stream errors before producing one.
	SetResponse(*llm.Response)

	// SetTimeToFirstChunk records elapsed seconds from request issue to the
	// first content-bearing chunk. Streaming iterations only.
	SetTimeToFirstChunk(seconds float64)

	// End marks the iteration as finished. err is non-nil only when the
	// underlying Generate / Stream call returned an error.
	End(err error)
}

// ToolCallSpan represents an in-flight tool-call observation. The agent calls
// End exactly once.
type ToolCallSpan interface {
	// SetResult records the tool's result. Called before End for both
	// success and tool-reported-error paths; not called when the tool panics
	// or the agent itself fails to invoke it.
	SetResult(*ToolCallResult)

	// End marks the call as finished. err is non-nil only when the agent
	// failed to invoke the tool (e.g. panic, missing tool); a tool that
	// returned IsError=true reports success here and the failure on the
	// result.
	End(err error)
}

// NopTracer is a Tracer that does nothing. It is the default tracer used by
// agents that do not configure one.
type NopTracer struct{}

func (NopTracer) StartAgentRun(ctx context.Context, _ AgentRunInfo) (context.Context, AgentRunSpan) {
	return ctx, nopAgentRunSpan{}
}

func (NopTracer) StartChat(ctx context.Context, _ ChatInfo) (context.Context, ChatSpan) {
	return ctx, nopChatSpan{}
}

func (NopTracer) StartToolCall(ctx context.Context, _ ToolCallInfo) (context.Context, ToolCallSpan) {
	return ctx, nopToolCallSpan{}
}

type nopAgentRunSpan struct{}

func (nopAgentRunSpan) SetResponse(*Response) {}
func (nopAgentRunSpan) SetUsage(*llm.Usage)   {}
func (nopAgentRunSpan) End(error)             {}

type nopChatSpan struct{}

func (nopChatSpan) SetResponse(*llm.Response)      {}
func (nopChatSpan) SetTimeToFirstChunk(float64)    {}
func (nopChatSpan) End(error)                      {}

type nopToolCallSpan struct{}

func (nopToolCallSpan) SetResult(*ToolCallResult) {}
func (nopToolCallSpan) End(error)                 {}

// MultiTracer fans Start* calls out to N tracers and returns a span whose
// methods fan out in turn. The ctx returned by the first tracer is the input
// to the next, so each tracer's span context layers on top of the previous —
// independent tracing systems can coexist without trampling each other.
//
// MultiTracer of zero tracers is equivalent to NopTracer.
func MultiTracer(tracers ...Tracer) Tracer {
	switch len(tracers) {
	case 0:
		return NopTracer{}
	case 1:
		return tracers[0]
	default:
		return multiTracer(tracers)
	}
}

type multiTracer []Tracer

func (m multiTracer) StartAgentRun(ctx context.Context, info AgentRunInfo) (context.Context, AgentRunSpan) {
	spans := make([]AgentRunSpan, len(m))
	for i, t := range m {
		ctx, spans[i] = t.StartAgentRun(ctx, info)
	}
	return ctx, multiAgentRunSpan(spans)
}

func (m multiTracer) StartChat(ctx context.Context, info ChatInfo) (context.Context, ChatSpan) {
	spans := make([]ChatSpan, len(m))
	for i, t := range m {
		ctx, spans[i] = t.StartChat(ctx, info)
	}
	return ctx, multiChatSpan(spans)
}

func (m multiTracer) StartToolCall(ctx context.Context, info ToolCallInfo) (context.Context, ToolCallSpan) {
	spans := make([]ToolCallSpan, len(m))
	for i, t := range m {
		ctx, spans[i] = t.StartToolCall(ctx, info)
	}
	return ctx, multiToolCallSpan(spans)
}

type multiAgentRunSpan []AgentRunSpan

func (m multiAgentRunSpan) SetResponse(r *Response) {
	for _, s := range m {
		s.SetResponse(r)
	}
}
func (m multiAgentRunSpan) SetUsage(u *llm.Usage) {
	for _, s := range m {
		s.SetUsage(u)
	}
}
func (m multiAgentRunSpan) End(err error) {
	for _, s := range m {
		s.End(err)
	}
}

type multiChatSpan []ChatSpan

func (m multiChatSpan) SetResponse(r *llm.Response) {
	for _, s := range m {
		s.SetResponse(r)
	}
}
func (m multiChatSpan) SetTimeToFirstChunk(seconds float64) {
	for _, s := range m {
		s.SetTimeToFirstChunk(seconds)
	}
}
func (m multiChatSpan) End(err error) {
	for _, s := range m {
		s.End(err)
	}
}

type multiToolCallSpan []ToolCallSpan

func (m multiToolCallSpan) SetResult(r *ToolCallResult) {
	for _, s := range m {
		s.SetResult(r)
	}
}
func (m multiToolCallSpan) End(err error) {
	for _, s := range m {
		s.End(err)
	}
}
