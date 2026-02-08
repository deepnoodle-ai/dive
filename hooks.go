package dive

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/deepnoodle-ai/dive/llm"
)

// Generation Hooks
//
// This file defines hooks for customizing the agent's generation loop.
// All hooks receive a *HookContext, which provides mutable access to
// generation state, tool call details, and inter-hook communication.
//
// Hook types:
//   - PreGenerationHook: runs before the LLM generation loop
//   - PostGenerationHook: runs after the generation loop completes
//   - PreToolUseHook: runs before each tool execution
//   - PostToolUseHook: runs after a tool call succeeds
//   - PostToolUseFailureHook: runs after a tool call fails
//   - StopHook: runs when the agent is about to stop, can continue
//   - PreIterationHook: runs before each LLM call within the loop
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    Hooks: dive.Hooks{
//	        PreGeneration: []dive.PreGenerationHook{
//	            func(ctx context.Context, hctx *dive.HookContext) error {
//	                hctx.SystemPrompt += "\nToday is Monday."
//	                return nil
//	            },
//	        },
//	        PostGeneration: []dive.PostGenerationHook{
//	            func(ctx context.Context, hctx *dive.HookContext) error {
//	                fmt.Printf("Tokens used: %d\n", hctx.Usage.InputTokens+hctx.Usage.OutputTokens)
//	                return nil
//	            },
//	        },
//	    },
//	})

// HookContext provides mutable access to the generation context.
// All hook types receive a *HookContext. Fields are populated based on
// the hook phase:
//
//   - PreGeneration: Agent, Values, SystemPrompt, Messages
//   - PostGeneration: Agent, Values, SystemPrompt, Messages, Response, OutputMessages, Usage
//   - PreToolUse: Agent, Values, Tool, Call
//   - PostToolUse: Agent, Values, Tool, Call, Result
//   - PostToolUseFailure: Agent, Values, Tool, Call, Result
//   - Stop: Agent, Values, Response, OutputMessages, Usage, StopHookActive
//   - PreIteration: Agent, Values, SystemPrompt, Messages, Iteration
//
// The Values map allows hooks to communicate with each other by storing
// arbitrary data that persists across the hook chain within a single
// CreateResponse call.
type HookContext struct {
	// Available to all hooks

	// Agent is the agent running the generation.
	Agent *Agent

	// Values provides arbitrary storage for hooks to communicate.
	// Persists across all phases within one CreateResponse call.
	Values map[string]any

	// Generation-level (mutable in PreGeneration/PreIteration)

	// SystemPrompt is the system prompt that will be sent to the LLM.
	SystemPrompt string

	// Messages contains the conversation history plus new input messages.
	Messages []*llm.Message

	// Response (available in PostGeneration, Stop)

	// Response is the complete Response object returned by CreateResponse.
	Response *Response

	// OutputMessages contains the messages generated during this response.
	OutputMessages []*llm.Message

	// Usage contains token usage statistics for this generation.
	Usage *llm.Usage

	// Tool-level (available in PreToolUse, PostToolUse, PostToolUseFailure)

	// Tool is the tool being executed.
	Tool Tool

	// Call contains the tool invocation details including input.
	Call *llm.ToolUseContent

	// Result contains the tool execution result (PostToolUse/PostToolUseFailure only).
	Result *ToolCallResult

	// PreToolUse capabilities

	// UpdatedInput, when set by a PreToolUse hook, replaces Call.Input before
	// the tool is executed. Only the last hook's UpdatedInput takes effect.
	UpdatedInput []byte

	// AdditionalContext, when set by a hook, is appended as a text content
	// block to the tool result message sent to the LLM. This lets hooks
	// provide guidance without modifying the tool result itself.
	AdditionalContext string

	// Stop hook

	// StopHookActive is true when this stop check was triggered by a
	// previous stop hook continuation. Check this to prevent infinite loops.
	StopHookActive bool

	// PreIteration

	// Iteration is the zero-based iteration number within the generation loop.
	Iteration int
}

// PreGenerationHook is called before the LLM generation loop begins.
//
// PreGeneration hooks run in order and can:
//   - Modify hctx.SystemPrompt to customize the system prompt
//   - Modify hctx.Messages to inject context or load session history
//   - Store data in hctx.Values for use by later hooks
//   - Return an error to abort generation entirely
//
// If any PreGeneration hook returns an error, generation is aborted and
// CreateResponse returns that error. No subsequent hooks are called.
type PreGenerationHook func(ctx context.Context, hctx *HookContext) error

// PostGenerationHook is called after the LLM generation loop completes.
//
// PostGeneration hooks run in order and can:
//   - Read hctx.Response to access the complete response
//   - Read hctx.OutputMessages to access generated messages
//   - Read hctx.Usage to access token usage statistics
//   - Read data from hctx.Values stored by earlier hooks
//   - Perform side effects like logging, saving, or notifications
//
// PostGeneration hook errors are logged but do NOT affect the returned
// Response. This design ensures that generation results are not lost due
// to post-processing failures (e.g., if saving to a database fails).
type PostGenerationHook func(ctx context.Context, hctx *HookContext) error

// PreToolUseHook is called before a tool is executed.
//
// All hooks run in order. If any hook returns an error, the tool is denied
// and the error message is sent to the LLM. If all hooks return nil, the
// tool is executed.
//
// Hooks can set hctx.UpdatedInput to rewrite tool arguments before execution,
// and hctx.AdditionalContext to inject context into the tool result message.
//
// Error handling:
//   - nil: no objection (tool runs if all hooks return nil)
//   - error: deny the tool (error message sent to LLM)
//   - *HookAbortError: abort generation entirely
type PreToolUseHook func(ctx context.Context, hctx *HookContext) error

// PostToolUseHook is called after a tool call succeeds.
//
// The hook receives context about the completed tool call including the result.
// Hooks can modify hctx.Result to transform the tool output before it's
// sent to the LLM in the next generation iteration.
//
// Hooks can set hctx.AdditionalContext to inject context into the tool
// result message.
//
// Hook errors are logged but do not affect the tool result.
type PostToolUseHook func(ctx context.Context, hctx *HookContext) error

// PostToolUseFailureHook is called after a tool call fails.
//
// The hook receives the same context as PostToolUseHook, but fires only
// when the tool execution returned an error or the result has IsError set.
// This mirrors Claude Code's separate PostToolUseFailure event.
//
// Hooks can set hctx.AdditionalContext to inject context into the tool
// result message.
//
// Hook errors are logged but do not affect the tool result.
type PostToolUseFailureHook func(ctx context.Context, hctx *HookContext) error

// StopHook is called when the agent is about to stop responding.
// A hook can prevent stopping by returning a StopDecision with Continue: true,
// which injects the Reason as a user message and re-enters the generation loop.
//
// hctx.StopHookActive is true when this stop check was triggered by a
// previous stop hook continuation. Check this to prevent infinite loops.
type StopHook func(ctx context.Context, hctx *HookContext) (*StopDecision, error)

// PreIterationHook is called before each LLM call within the generation loop.
// Use these to modify the system prompt or messages between iterations.
//
// hctx.Iteration provides the zero-based iteration number.
// Errors abort generation (same as PreGeneration).
type PreIterationHook func(ctx context.Context, hctx *HookContext) error

// StopDecision tells the agent what to do after a stop hook runs.
type StopDecision struct {
	// Continue, when true, prevents the agent from stopping.
	// The Reason is injected as a user message so the LLM knows
	// why it should keep going.
	Continue bool

	// Reason is required when Continue is true. It's added to the
	// conversation as context for the next LLM iteration.
	Reason string
}

// NewHookContext creates a new HookContext with initialized Values map.
func NewHookContext() *HookContext {
	return &HookContext{
		Values: make(map[string]any),
	}
}

// Deprecated: NewGenerationState creates a new HookContext. Use NewHookContext instead.
func NewGenerationState() *HookContext {
	return NewHookContext()
}

// InjectContext returns a PreGenerationHook that prepends the given content
// to the conversation as a user message.
//
// This is useful for injecting context that should appear before the user's
// actual input, such as:
//   - Relevant documentation or code snippets
//   - Previous conversation summaries
//   - Environment information or system state
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a coding assistant.",
//	    Model:        model,
//	    Hooks: dive.Hooks{
//	        PreGeneration: []dive.PreGenerationHook{
//	            dive.InjectContext(
//	                llm.NewTextContent("Current working directory: /home/user/project"),
//	                llm.NewTextContent("Git branch: main"),
//	            ),
//	        },
//	    },
//	})
func InjectContext(content ...llm.Content) PreGenerationHook {
	return func(ctx context.Context, hctx *HookContext) error {
		if len(content) == 0 {
			return nil
		}
		contextMsg := llm.NewUserMessage(content...)
		hctx.Messages = append([]*llm.Message{contextMsg}, hctx.Messages...)
		return nil
	}
}

// CompactionHook returns a PreGenerationHook that triggers context compaction
// when the message count exceeds the given threshold.
//
// The summarizer function is called when compaction is triggered. It receives
// the current messages and should return compacted messages. If the summarizer
// returns an error, the hook returns that error (aborting generation).
func CompactionHook(messageThreshold int, summarizer func(context.Context, []*llm.Message) ([]*llm.Message, error)) PreGenerationHook {
	return func(ctx context.Context, hctx *HookContext) error {
		if len(hctx.Messages) < messageThreshold {
			return nil
		}
		compacted, err := summarizer(ctx, hctx.Messages)
		if err != nil {
			return err
		}
		hctx.Messages = compacted
		return nil
	}
}

// UsageLogger returns a PostGenerationHook that logs token usage after each
// generation using the provided callback function.
func UsageLogger(logFunc func(usage *llm.Usage)) PostGenerationHook {
	return func(ctx context.Context, hctx *HookContext) error {
		if hctx.Usage != nil && logFunc != nil {
			logFunc(hctx.Usage)
		}
		return nil
	}
}

// UsageLoggerWithSlog returns a PostGenerationHook that logs token usage
// using an slog.Logger.
func UsageLoggerWithSlog(logger llm.Logger) PostGenerationHook {
	return func(ctx context.Context, hctx *HookContext) error {
		if hctx.Usage == nil || logger == nil {
			return nil
		}
		logger.Info("generation complete",
			"input_tokens", hctx.Usage.InputTokens,
			"output_tokens", hctx.Usage.OutputTokens,
			"cache_creation_input_tokens", hctx.Usage.CacheCreationInputTokens,
			"cache_read_input_tokens", hctx.Usage.CacheReadInputTokens,
		)
		return nil
	}
}

// MatchTool returns a PreToolUseHook that only runs when the tool name
// matches the given pattern. The pattern is a Go regexp.
func MatchTool(pattern string, hook PreToolUseHook) PreToolUseHook {
	re := regexp.MustCompile(pattern)
	return func(ctx context.Context, hctx *HookContext) error {
		if hctx.Tool == nil || !re.MatchString(hctx.Tool.Name()) {
			return nil
		}
		return hook(ctx, hctx)
	}
}

// MatchToolPost returns a PostToolUseHook that only runs when the tool name
// matches the given pattern. The pattern is a Go regexp.
func MatchToolPost(pattern string, hook PostToolUseHook) PostToolUseHook {
	re := regexp.MustCompile(pattern)
	return func(ctx context.Context, hctx *HookContext) error {
		if hctx.Tool == nil || !re.MatchString(hctx.Tool.Name()) {
			return nil
		}
		return hook(ctx, hctx)
	}
}

// MatchToolPostFailure returns a PostToolUseFailureHook that only runs when
// the tool name matches the given pattern. The pattern is a Go regexp.
func MatchToolPostFailure(pattern string, hook PostToolUseFailureHook) PostToolUseFailureHook {
	re := regexp.MustCompile(pattern)
	return func(ctx context.Context, hctx *HookContext) error {
		if hctx.Tool == nil || !re.MatchString(hctx.Tool.Name()) {
			return nil
		}
		return hook(ctx, hctx)
	}
}

// HookAbortError signals that a hook wants to abort generation entirely.
// When returned from any hook, CreateResponse will abort and return this error.
// Use this for safety violations, compliance issues, or critical failures.
//
// Regular errors (non-HookAbortError) are handled gracefully:
//   - PreGeneration: aborts (setup is required)
//   - PostGeneration: logged only
//   - PreToolUse: converted to Deny message
//   - PostToolUse: logged only
//   - PostToolUseFailure: logged only
type HookAbortError struct {
	Reason   string
	HookType string // "PreGeneration", "PostGeneration", "PreToolUse", "PostToolUse", "PostToolUseFailure"
	HookName string // Optional: name/description of the hook that aborted
	Cause    error  // Optional: underlying error
}

func (e *HookAbortError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("generation aborted by %s hook: %s: %v", e.HookType, e.Reason, e.Cause)
	}
	return fmt.Sprintf("generation aborted by %s hook: %s", e.HookType, e.Reason)
}

func (e *HookAbortError) Unwrap() error {
	return e.Cause
}

// AbortGeneration creates a HookAbortError to abort generation.
// Use this in hooks when a critical failure occurs that should stop generation entirely.
func AbortGeneration(reason string) error {
	return &HookAbortError{Reason: reason}
}

// AbortGenerationWithCause creates a HookAbortError with an underlying cause.
func AbortGenerationWithCause(reason string, cause error) error {
	return &HookAbortError{Reason: reason, Cause: cause}
}

// UserFeedbackError wraps user-provided feedback when they deny a tool call.
// This allows distinguishing user feedback from actual errors.
type UserFeedbackError struct {
	Feedback string
}

func (e *UserFeedbackError) Error() string {
	return e.Feedback
}

// NewUserFeedback creates a UserFeedbackError with the given feedback.
func NewUserFeedback(feedback string) error {
	return &UserFeedbackError{Feedback: feedback}
}

// IsUserFeedback checks if an error is user feedback and returns the feedback text.
// Returns the feedback string and true if it's user feedback, empty string and false otherwise.
func IsUserFeedback(err error) (string, bool) {
	var uf *UserFeedbackError
	if errors.As(err, &uf) {
		return uf.Feedback, true
	}
	return "", false
}

// Type aliases for backwards compatibility.
type GenerationState = HookContext
type PreToolUseContext = HookContext
type PostToolUseContext = HookContext
