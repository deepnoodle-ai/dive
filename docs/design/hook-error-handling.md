# Hook Error Handling Design

## Overview

This document describes the error handling semantics for agent hooks in Dive. Hooks need the ability to either fail gracefully (log and continue) or fail fatally (abort generation entirely). This design provides a consistent, flexible approach across all hook types.

## Problem Statement

Hooks have different failure modes depending on their purpose:

1. **Safety/Compliance Failures** - Must abort generation immediately (e.g., sensitive data detected, malicious input)
2. **Operational Failures** - Should log but allow generation to continue (e.g., metrics logging failed, audit log write failed)
3. **Setup Failures** - Can't proceed without successful setup (e.g., session load failed)

The original implementation had inconsistent error handling:
- PreGeneration: errors abort
- PostGeneration: errors logged
- PreToolUse: errors converted to Deny
- PostToolUse: errors logged

This made it impossible for Post hooks to abort on critical failures.

## Solution: HookAbortError Type

Introduce a typed error that signals fatal failure across all hook types.

### Error Type Definition

```go
// HookAbortError signals that a hook wants to abort generation entirely.
// When returned from any hook, CreateResponse will abort and return this error.
// Use this for safety violations, compliance issues, or critical failures.
//
// Regular errors (non-HookAbortError) are handled gracefully:
//   - PreGeneration: aborts (setup is required)
//   - PostGeneration: logged only
//   - PreToolUse: converted to Deny message
//   - PostToolUse: logged only
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

// Helper constructor
func AbortGeneration(reason string) error {
	return &HookAbortError{Reason: reason}
}

// Helper with cause
func AbortGenerationWithCause(reason string, cause error) error {
	return &HookAbortError{Reason: reason, Cause: cause}
}
```

## Error Handling Semantics by Hook Type

### PreGeneration Hooks

**ANY error aborts generation** (setup is required to proceed).

```go
// Regular error - aborts
return fmt.Errorf("failed to load session: %w", err)

// HookAbortError - aborts with better context
return dive.AbortGeneration("session not found")
```

**Implementation**: Check for any error, abort immediately.

### PostGeneration Hooks

**Regular errors logged, HookAbortError aborts**.

```go
// Regular error - logged, generation completes
return fmt.Errorf("failed to save session: %w", err)

// HookAbortError - aborts and discards response
return dive.AbortGeneration("safety violation in output")
```

**Implementation**:
- Check if `errors.As(err, &HookAbortError{})` → abort
- Otherwise log and continue

**Note**: Aborting in PostGeneration discards the response even though generation completed. Use sparingly for safety/compliance only.

### PreToolUse Hooks

**Regular errors converted to Deny, HookAbortError aborts**.

```go
// Regular error - converted to Deny message to LLM
return nil, fmt.Errorf("permission denied: %s", reason)

// HookAbortError - aborts entire generation
return nil, dive.AbortGeneration("malicious tool call detected")

// Explicit deny (recommended for clarity)
return dive.DenyResult("permission denied"), nil
```

**Implementation**:
- Check if `errors.As(err, &HookAbortError{})` → abort
- Otherwise convert to `DenyResult(err.Error())`

**Recommendation**: Use explicit `DenyResult` instead of returning errors for normal denials. Reserve errors for unexpected failures.

### PostToolUse Hooks

**Regular errors logged, HookAbortError aborts**.

```go
// Regular error - logged, result used unchanged
return fmt.Errorf("failed to log tool result: %w", err)

// HookAbortError - aborts generation
return dive.AbortGeneration("sensitive data in tool output")
```

**Implementation**:
- Check if `errors.As(err, &HookAbortError{})` → abort
- Otherwise log and use modified result

## Usage Examples

### Safety Filter (Post Hook)

```go
func sensitiveDataFilter() dive.PostToolUseHook {
    return func(ctx context.Context, hookCtx *dive.PostToolUseContext) error {
        if containsPII(hookCtx.Result) {
            // Fatal - abort generation to prevent data leak
            return dive.AbortGeneration("PII detected in tool output")
        }
        return nil
    }
}
```

### Audit Logger (Post Hook)

```go
func auditLogger(db *DB) dive.PostToolUseHook {
    return func(ctx context.Context, hookCtx *dive.PostToolUseContext) error {
        if err := db.LogToolUse(hookCtx); err != nil {
            // Non-fatal - log failure but don't break generation
            return fmt.Errorf("audit log write failed: %w", err)
        }
        return nil
    }
}
```

### Permission Checker (Pre Hook)

```go
func permissionChecker() dive.PreToolUseHook {
    return func(ctx context.Context, hookCtx *dive.PreToolUseContext) (*dive.ToolHookResult, error) {
        if isMalicious(hookCtx.Call) {
            // Fatal - security threat
            return nil, dive.AbortGeneration("malicious input detected")
        }
        if !hasPermission(hookCtx.Tool) {
            // Graceful - let LLM try something else
            return dive.DenyResult("permission denied"), nil
        }
        return dive.AllowResult(), nil
    }
}
```

### Session Loader (Pre Hook)

```go
func sessionLoader(repo SessionRepository) dive.PreGenerationHook {
    return func(ctx context.Context, state *dive.GenerationState) error {
        session, err := repo.GetSession(ctx, state.SessionID)
        if err == ErrSessionNotFound {
            return nil // New session, ok
        }
        if err != nil {
            // Any error aborts (can't proceed without session)
            return fmt.Errorf("failed to load session: %w", err)
        }
        state.Messages = append(session.Messages, state.Messages...)
        return nil
    }
}
```

## Implementation Checklist

- [ ] Add `HookAbortError` type to `hooks.go` or new `errors.go`
- [ ] Add `AbortGeneration()` and `AbortGenerationWithCause()` helpers
- [ ] Update PreGeneration hook execution (agent.go ~lines 238-243)
- [ ] Update PostGeneration hook execution (agent.go ~lines 307-312)
- [ ] Update PreToolUse hook execution (agent.go ~lines 419-427)
- [ ] Update PostToolUse hook execution (agent.go ~lines 622-626)
- [ ] Update hook documentation in `hooks.go`
- [ ] Update AgentOptions documentation in `agent.go`
- [ ] Add examples to hook documentation
- [ ] Add test cases for abort behavior

## Backward Compatibility

✅ **Fully backward compatible**

Existing hooks that return regular errors will behave exactly as before:
- PreGeneration: errors still abort (unchanged)
- PostGeneration: errors still logged (unchanged)
- PreToolUse: errors still converted to Deny (unchanged)
- PostToolUse: errors still logged (unchanged)

Only new behavior is the ability to use `AbortGeneration()` to explicitly abort from any hook.

## Future Enhancements

Potential additions without breaking changes:

1. **Context Fields**: Add fields to `HookAbortError` for better debugging
   ```go
   type HookAbortError struct {
       Reason   string
       HookType string
       ToolName string
       Details  map[string]any
   }
   ```

2. **Retry Signals**: Distinguish between permanent failures and transient errors
   ```go
   type HookRetryableError struct {
       HookAbortError
       Retryable bool
   }
   ```

3. **Hook Chaining Control**: Allow hooks to signal "skip remaining hooks"
   ```go
   type HookSkipError struct {
       Reason string
   }
   ```
