package dive

import (
	"context"
	"errors"
	"fmt"

	"github.com/deepnoodle-ai/dive/llm"
)

// Generation Hooks
//
// This file defines hooks for customizing the agent's generation loop.
// Generation hooks allow you to modify inputs before generation and process
// outputs after generation without modifying the agent's core behavior.
//
// PreGeneration hooks can:
//   - Load session history from external storage
//   - Inject context or system prompts
//   - Modify messages before they're sent to the LLM
//   - Short-circuit generation by returning an error
//
// PostGeneration hooks can:
//   - Save session history to external storage
//   - Log or audit generation results
//   - Trigger side effects based on the response
//   - Update metrics or analytics
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    PreGeneration: []dive.PreGenerationHook{
//	        func(ctx context.Context, state *dive.GenerationState) error {
//	            // Inject additional context before generation
//	            state.SystemPrompt += "\nToday is Monday."
//	            return nil
//	        },
//	    },
//	    PostGeneration: []dive.PostGenerationHook{
//	        func(ctx context.Context, state *dive.GenerationState) error {
//	            // Log token usage after generation
//	            fmt.Printf("Tokens used: %d\n", state.Usage.InputTokens+state.Usage.OutputTokens)
//	            return nil
//	        },
//	    },
//	})

// GenerationState provides mutable access to the generation context.
// PreGeneration hooks can modify the input fields (SystemPrompt, Messages).
// PostGeneration hooks can read the output fields (Response, OutputMessages, Usage).
//
// The Values map allows hooks to communicate with each other by storing
// arbitrary data that persists across the hook chain.
type GenerationState struct {
	// Input (mutable in PreGeneration)

	// SystemPrompt is the system prompt that will be sent to the LLM.
	// PreGeneration hooks can modify this to customize agent behavior.
	SystemPrompt string

	// Messages contains the conversation history plus new input messages.
	// PreGeneration hooks can prepend history, inject context, or filter messages.
	Messages []*llm.Message

	// Output (available in PostGeneration)

	// Response is the complete Response object returned by CreateResponse.
	// Only available in PostGeneration hooks.
	Response *Response

	// OutputMessages contains the messages generated during this response.
	// This includes assistant messages and tool result messages.
	// Only available in PostGeneration hooks.
	OutputMessages []*llm.Message

	// Usage contains token usage statistics for this generation.
	// Only available in PostGeneration hooks.
	Usage *llm.Usage

	// Values provides arbitrary storage for hooks to communicate.
	// Use this to pass data between PreGeneration and PostGeneration hooks,
	// or between multiple hooks in the same phase.
	//
	// Example:
	//   // In PreGeneration hook:
	//   state.Values["start_time"] = time.Now()
	//
	//   // In PostGeneration hook:
	//   startTime := state.Values["start_time"].(time.Time)
	//   duration := time.Since(startTime)
	Values map[string]any
}

// PreGenerationHook is called before the LLM generation loop begins.
//
// PreGeneration hooks run in order and can:
//   - Modify state.SystemPrompt to customize the system prompt
//   - Modify state.Messages to inject context or load session history
//   - Store data in state.Values for use by PostGeneration hooks
//   - Return an error to abort generation entirely
//
// If any PreGeneration hook returns an error, generation is aborted and
// CreateResponse returns that error. No subsequent hooks are called.
//
// Example:
//
//	func contextInjector(info string) PreGenerationHook {
//	    return func(ctx context.Context, state *GenerationState) error {
//	        state.SystemPrompt += "\n" + info
//	        return nil
//	    }
//	}
type PreGenerationHook func(ctx context.Context, state *GenerationState) error

// PostGenerationHook is called after the LLM generation loop completes.
//
// PostGeneration hooks run in order and can:
//   - Read state.Response to access the complete response
//   - Read state.OutputMessages to access generated messages
//   - Read state.Usage to access token usage statistics
//   - Read data from state.Values stored by PreGeneration hooks
//   - Perform side effects like logging, saving, or notifications
//
// PostGeneration hook errors are logged but do NOT affect the returned
// Response. This design ensures that generation results are not lost due
// to post-processing failures (e.g., if saving to a database fails).
//
// Example:
//
//	func usageTracker(totals *UsageTotals) PostGenerationHook {
//	    return func(ctx context.Context, state *GenerationState) error {
//	        if state.Usage != nil {
//	            totals.InputTokens += state.Usage.InputTokens
//	            totals.OutputTokens += state.Usage.OutputTokens
//	        }
//	        return nil
//	    }
//	}
type PostGenerationHook func(ctx context.Context, state *GenerationState) error

// NewGenerationState creates a new GenerationState with initialized Values map.
func NewGenerationState() *GenerationState {
	return &GenerationState{
		Values: make(map[string]any),
	}
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
//	    PreGeneration: []dive.PreGenerationHook{
//	        dive.InjectContext(
//	            llm.NewTextContent("Current working directory: /home/user/project"),
//	            llm.NewTextContent("Git branch: main"),
//	        ),
//	    },
//	})
func InjectContext(content ...llm.Content) PreGenerationHook {
	return func(ctx context.Context, state *GenerationState) error {
		if len(content) == 0 {
			return nil
		}
		contextMsg := llm.NewUserMessage(content...)
		state.Messages = append([]*llm.Message{contextMsg}, state.Messages...)
		return nil
	}
}

// CompactionHook returns a PreGenerationHook that triggers context compaction
// when the message count exceeds the given threshold.
//
// The summarizer function is called when compaction is triggered. It receives
// the current messages and should return compacted messages. If the summarizer
// returns an error, the hook returns that error (aborting generation).
//
// For integration with the existing CompactMessages function, use
// CompactionHookWithModel instead, which handles the full compaction flow
// including token estimation.
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    PreGeneration: []dive.PreGenerationHook{
//	        dive.CompactionHook(50, func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error) {
//	            // Custom summarization logic
//	            return summarize(ctx, msgs)
//	        }),
//	    },
//	})
func CompactionHook(messageThreshold int, summarizer func(context.Context, []*llm.Message) ([]*llm.Message, error)) PreGenerationHook {
	return func(ctx context.Context, state *GenerationState) error {
		if len(state.Messages) < messageThreshold {
			return nil
		}
		compacted, err := summarizer(ctx, state.Messages)
		if err != nil {
			return err
		}
		state.Messages = compacted
		return nil
	}
}

// UsageLogger returns a PostGenerationHook that logs token usage after each
// generation using the provided callback function.
//
// Example with slog:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    PostGeneration: []dive.PostGenerationHook{
//	        dive.UsageLogger(func(usage *llm.Usage) {
//	            slog.Info("generation complete",
//	                "input_tokens", usage.InputTokens,
//	                "output_tokens", usage.OutputTokens,
//	            )
//	        }),
//	    },
//	})
func UsageLogger(logFunc func(usage *llm.Usage)) PostGenerationHook {
	return func(ctx context.Context, state *GenerationState) error {
		if state.Usage != nil && logFunc != nil {
			logFunc(state.Usage)
		}
		return nil
	}
}

// UsageLoggerWithSlog returns a PostGenerationHook that logs token usage
// using an slog.Logger.
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    PostGeneration: []dive.PostGenerationHook{
//	        dive.UsageLoggerWithSlog(slog.Default()),
//	    },
//	})
func UsageLoggerWithSlog(logger llm.Logger) PostGenerationHook {
	return func(ctx context.Context, state *GenerationState) error {
		if state.Usage == nil || logger == nil {
			return nil
		}
		logger.Info("generation complete",
			"input_tokens", state.Usage.InputTokens,
			"output_tokens", state.Usage.OutputTokens,
			"cache_creation_input_tokens", state.Usage.CacheCreationInputTokens,
			"cache_read_input_tokens", state.Usage.CacheReadInputTokens,
		)
		return nil
	}
}

// Tool Hooks
//
// Tool hooks run around individual tool executions within the generation loop.
// They allow inspection and control of tool calls without modifying the agent.
//
// PreToolUse hooks can:
//   - Allow tool execution unconditionally
//   - Deny tool execution with a message
//   - Request user confirmation before execution
//   - Modify the tool input before execution
//   - Audit or log tool calls
//
// PostToolUse hooks can:
//   - Modify tool results before they're sent to the LLM
//   - Log tool results
//   - Update metrics
//   - Trigger side effects based on results
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        model,
//	    Tools:        tools,
//	    PreToolUse: []dive.PreToolUseHook{
//	        func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
//	            // Allow read-only tools automatically
//	            if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().ReadOnlyHint {
//	                return dive.AllowResult(), nil
//	            }
//	            // Ask for confirmation on other tools
//	            return dive.AskResult("Execute this tool?"), nil
//	        },
//	    },
//	})

// ToolHookAction represents the action a PreToolUse hook wants to take.
type ToolHookAction string

const (
	// ToolHookAllow allows the tool execution to proceed.
	ToolHookAllow ToolHookAction = "allow"

	// ToolHookDeny prevents the tool execution.
	ToolHookDeny ToolHookAction = "deny"

	// ToolHookAsk requests user confirmation before proceeding.
	ToolHookAsk ToolHookAction = "ask"

	// ToolHookContinue defers to the next hook in the chain.
	ToolHookContinue ToolHookAction = "continue"
)

// ToolCategory represents a category of tools for permission grouping.
type ToolCategory struct {
	// Key is the machine-readable identifier (e.g., "bash", "edit", "read").
	Key string

	// Label is the human-readable description (e.g., "bash commands", "file edits").
	Label string
}

// ToolHookResult is returned by PreToolUse hooks to indicate the desired action.
type ToolHookResult struct {
	// Action indicates what should happen (allow, deny, ask, continue).
	Action ToolHookAction

	// Message provides context for deny/ask actions.
	Message string

	// UpdatedInput optionally provides modified input for the tool call.
	// Only used when Action is ToolHookAllow.
	UpdatedInput []byte

	// Category optionally identifies the tool category for session allowlists.
	Category *ToolCategory
}

// PreToolUseContext provides context about a pending tool execution.
type PreToolUseContext struct {
	// Tool is the tool about to be executed.
	Tool Tool

	// Call contains the tool invocation details including input.
	Call *llm.ToolUseContent

	// Agent is the agent executing the tool.
	Agent *Agent
}

// PostToolUseContext provides context about a completed tool execution.
type PostToolUseContext struct {
	// Tool is the tool that was executed.
	Tool Tool

	// Call contains the tool invocation details.
	Call *llm.ToolUseContent

	// Result contains the tool execution result.
	// PostToolUse hooks can modify this result before it's sent to the LLM.
	Result *ToolCallResult

	// Agent is the agent that executed the tool.
	Agent *Agent
}

// PreToolUseHook is called before a tool is executed.
//
// The hook receives context about the pending tool call and returns a result
// indicating the desired action:
//   - AllowResult(): Execute the tool
//   - DenyResult(msg): Reject the tool call with a message
//   - AskResult(msg): Request user confirmation
//   - ContinueResult(): Defer to the next hook
//
// If all hooks return ContinueResult, the default behavior is to ask for confirmation.
//
// Example:
//
//	func readOnlyAllower() PreToolUseHook {
//	    return func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error) {
//	        if hookCtx.Tool.Annotations() != nil && hookCtx.Tool.Annotations().ReadOnlyHint {
//	            return AllowResult(), nil
//	        }
//	        return ContinueResult(), nil
//	    }
//	}
type PreToolUseHook func(ctx context.Context, hookCtx *PreToolUseContext) (*ToolHookResult, error)

// PostToolUseHook is called after a tool has been executed.
//
// The hook receives context about the completed tool call including the result.
// Hooks can modify hookCtx.Result to transform the tool output before it's
// sent to the LLM in the next generation iteration.
//
// Hook errors are logged but do not affect the tool result.
//
// Example (logging):
//
//	func toolLogger(logger *slog.Logger) PostToolUseHook {
//	    return func(ctx context.Context, hookCtx *PostToolUseContext) error {
//	        logger.Info("tool executed",
//	            "tool", hookCtx.Tool.Name(),
//	            "error", hookCtx.Result.Error,
//	        )
//	        return nil
//	    }
//	}
//
// Example (modifying result):
//
//	func resultTruncator(maxLen int) PostToolUseHook {
//	    return func(ctx context.Context, hookCtx *PostToolUseContext) error {
//	        if hookCtx.Result.Result != nil && len(hookCtx.Result.Result.Content) > 0 {
//	            // Truncate long text results
//	            for _, content := range hookCtx.Result.Result.Content {
//	                if content.Type == dive.ToolResultContentTypeText && len(content.Text) > maxLen {
//	                    content.Text = content.Text[:maxLen] + "... (truncated)"
//	                }
//	            }
//	        }
//	        return nil
//	    }
//	}
type PostToolUseHook func(ctx context.Context, hookCtx *PostToolUseContext) error

// ConfirmToolFunc is called when user confirmation is needed for a tool call.
// Returns true if the user approved, false if denied.
type ConfirmToolFunc func(ctx context.Context, tool Tool, call *llm.ToolUseContent, message string) (bool, error)

// CanUseToolFunc is a callback for custom permission logic.
// Return nil to defer to other hooks, or a ToolHookResult to make a decision.
type CanUseToolFunc func(ctx context.Context, tool Tool, call *llm.ToolUseContent) (*ToolHookResult, error)

// AllowResult creates a ToolHookResult that allows tool execution.
func AllowResult() *ToolHookResult {
	return &ToolHookResult{Action: ToolHookAllow}
}

// DenyResult creates a ToolHookResult that denies tool execution.
func DenyResult(message string) *ToolHookResult {
	return &ToolHookResult{Action: ToolHookDeny, Message: message}
}

// AskResult creates a ToolHookResult that requests user confirmation.
func AskResult(message string) *ToolHookResult {
	return &ToolHookResult{Action: ToolHookAsk, Message: message}
}

// ContinueResult creates a ToolHookResult that defers to the next hook.
func ContinueResult() *ToolHookResult {
	return &ToolHookResult{Action: ToolHookContinue}
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
//
// Example:
//
//	func sensitiveDataFilter() PostToolUseHook {
//	    return func(ctx context.Context, hookCtx *PostToolUseContext) error {
//	        if containsPII(hookCtx.Result) {
//	            return dive.AbortGeneration("PII detected in tool output")
//	        }
//	        return nil
//	    }
//	}
type HookAbortError struct {
	Reason   string
	HookType string // "PreGeneration", "PostGeneration", "PreToolUse", "PostToolUse"
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
