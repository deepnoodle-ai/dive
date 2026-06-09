package dive

// Regression tests for the agent-loop findings in
// docs/reviews/2026-06-09-api-review.md §4 (batch 3):
//
//   - §4.5  SessionStart hook Values carry into the main hook chain
//   - §4.7  Mid-turn LLM failure exposes accumulated work via GenerationError
//   - §4.8  Hook Messages freshness no longer depends on PreIteration registration
//   - §4 notes: deterministic resume merge order, context-aware + reentrancy-safe
//     session lock, NewAgent not mutating caller-owned option slices, resume
//     merge surviving a message-replacing PreGeneration hook.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// TestSessionStartValuesVisibleToLaterHooks pins §4.5: HookContext.Values is
// documented to persist across the hook chain within one CreateResponse call,
// so a value stored by a SessionStart hook must be observable by PreGeneration
// and PreToolUse hooks in the same call.
func TestSessionStartValuesVisibleToLaterHooks(t *testing.T) {
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:         "resp_1",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.ToolUseContent{ID: "t1", Name: "noop", Input: []byte(`{}`)}},
					Type:       "message",
					StopReason: "tool_use",
				}, nil
			}
			return &llm.Response{
				ID:         "resp_2",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}
	tool := &mockTool{
		name:     "noop",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return NewToolResultText("ok"), nil },
	}

	var preGenSaw, preToolSaw any
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
		Hooks: Hooks{
			SessionStart: []SessionStartHook{
				func(ctx context.Context, hctx *HookContext) (*SessionStartResult, error) {
					hctx.Values["seeded"] = "from-session-start"
					return nil, nil
				},
			},
			PreGeneration: []PreGenerationHook{
				func(ctx context.Context, hctx *HookContext) error {
					preGenSaw = hctx.Values["seeded"]
					return nil
				},
			},
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hctx *HookContext) error {
					preToolSaw = hctx.Values["seeded"]
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("hello"))
	assert.NoError(t, err)
	assert.Equal(t, preGenSaw, "from-session-start")
	assert.Equal(t, preToolSaw, "from-session-start")
}

// TestSessionStartValuesReplacementHonored pins that a SessionStart hook
// replacing hctx.Values wholesale (rather than mutating the seeded map) is
// still carried into the main hook chain — the map is captured after the
// hooks run, not before.
func TestSessionStartValuesReplacementHonored(t *testing.T) {
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return &llm.Response{
				ID:         "resp_1",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}

	var preGenSaw any
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Hooks: Hooks{
			SessionStart: []SessionStartHook{
				func(ctx context.Context, hctx *HookContext) (*SessionStartResult, error) {
					hctx.Values = map[string]any{"replaced": "yes"}
					return nil, nil
				},
			},
			PreGeneration: []PreGenerationHook{
				func(ctx context.Context, hctx *HookContext) error {
					preGenSaw = hctx.Values["replaced"]
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("hello"))
	assert.NoError(t, err)
	assert.Equal(t, preGenSaw, "yes")
}

// TestGenerationErrorExposesPartialWork pins §4.7: when iteration N of a turn
// fails after iteration N-1 succeeded (and a tool with side effects already
// ran), CreateResponse still returns (nil, err) — but err wraps a
// *GenerationError exposing the accumulated usage, output messages, and items.
// The partial turn must NOT be saved to the session.
func TestGenerationErrorExposesPartialWork(t *testing.T) {
	llmFailure := errors.New("provider exploded mid-turn")
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:         "resp_1",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.ToolUseContent{ID: "t1", Name: "side_effect", Input: []byte(`{}`)}},
					Type:       "message",
					StopReason: "tool_use",
					Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			}
			return nil, llmFailure
		},
	}
	toolRan := false
	tool := &mockTool{
		name: "side_effect",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			toolRan = true
			return NewToolResultText("side effect happened"), nil
		},
	}
	sess := newMemSession("partial-work")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("do work"))
	assert.Nil(t, resp)
	assert.Error(t, err)
	assert.True(t, toolRan, "the side-effecting tool ran before the failure")

	// The original error remains reachable through the wrap chain.
	assert.True(t, errors.Is(err, llmFailure))

	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr), "error must wrap *GenerationError")

	// Cost accounting from the successful first iteration is recoverable.
	assert.NotNil(t, genErr.Usage)
	assert.Equal(t, genErr.Usage.InputTokens, 10)
	assert.Equal(t, genErr.Usage.OutputTokens, 5)

	// The turn's partial messages: assistant tool_use + tool_result.
	assert.Len(t, genErr.OutputMessages, 2)
	assert.Equal(t, genErr.OutputMessages[0].Role, llm.Assistant)
	assert.True(t, hasToolUseContent(genErr.OutputMessages[0]))
	assert.Equal(t, genErr.OutputMessages[1].Role, llm.User)
	assert.True(t, hasToolResultContent(genErr.OutputMessages[1]))

	// Items captured before the failure: assistant message, tool call, result.
	var types []ResponseItemType
	for _, item := range genErr.Items {
		types = append(types, item.Type)
	}
	assert.Equal(t, types, []ResponseItemType{
		ResponseItemTypeMessage,
		ResponseItemTypeToolCall,
		ResponseItemTypeToolCallResult,
	})

	// The half-turn must not be persisted (it could violate role alternation).
	msgs, msgsErr := sess.Messages(context.Background())
	assert.NoError(t, msgsErr)
	assert.Len(t, msgs, 0, "partial turn must not be saved to the session")
}

// TestHookMessagesRefreshedWithoutPreIterationHooks pins §4.8: hctx.Messages
// must be refreshed every iteration even when no PreIteration hooks are
// registered, so a PreToolUse hook on iteration 2 observes iteration 1's
// assistant and tool_result messages instead of a stale start-of-turn snapshot.
func TestHookMessagesRefreshedWithoutPreIterationHooks(t *testing.T) {
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount <= 2 {
				return &llm.Response{
					ID:   fmt.Sprintf("resp_%d", callCount),
					Role: llm.Assistant,
					Content: []llm.Content{
						&llm.ToolUseContent{ID: fmt.Sprintf("t%d", callCount), Name: "noop", Input: []byte(`{}`)},
					},
					Type:       "message",
					StopReason: "tool_use",
				}, nil
			}
			return &llm.Response{
				ID:         "resp_final",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}
	tool := &mockTool{
		name:     "noop",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) { return NewToolResultText("ok"), nil },
	}

	var observedCounts []int
	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{tool},
		Hooks: Hooks{
			// Only a PreToolUse hook — deliberately NO PreIteration hooks.
			PreToolUse: []PreToolUseHook{
				func(ctx context.Context, hctx *HookContext) error {
					observedCounts = append(observedCounts, len(hctx.Messages))
					return nil
				},
			},
		},
	})
	assert.NoError(t, err)

	_, err = agent.CreateResponse(context.Background(), WithInput("go"))
	assert.NoError(t, err)

	// Iteration 1: hook sees just the user input. Iteration 2: hook sees the
	// user input plus iteration 1's assistant tool_use and tool_result.
	assert.Equal(t, observedCounts, []int{1, 3})
}

// TestSessionLockContextAwareAndReentrancyDetection pins the per-session lock
// behavior: a context already holding the lock fails fast with
// ErrReentrantSession, and a waiter honors context cancellation instead of
// blocking forever.
func TestSessionLockContextAwareAndReentrancyDetection(t *testing.T) {
	lockCtx, release, err := acquireSessionLock(context.Background(), "lock-test-1")
	assert.NoError(t, err)

	// Reentrant acquisition from the same context chain fails immediately.
	_, _, err = acquireSessionLock(lockCtx, "lock-test-1")
	assert.True(t, errors.Is(err, ErrReentrantSession))

	// A different caller waiting on the held lock honors context deadline.
	waitCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, _, err = acquireSessionLock(waitCtx, "lock-test-1")
	assert.True(t, errors.Is(err, context.DeadlineExceeded))

	// A different session ID is unaffected by the held lock.
	ctx2, release2, err := acquireSessionLock(lockCtx, "lock-test-2")
	assert.NoError(t, err)
	assert.NotNil(t, ctx2)
	release2()

	// After release, the lock can be acquired again.
	release()
	_, release3, err := acquireSessionLock(context.Background(), "lock-test-1")
	assert.NoError(t, err)
	release3()
}

// TestReentrantCreateResponseFailsFast pins that a tool calling back into
// CreateResponse on the same session errors promptly with ErrReentrantSession
// instead of deadlocking on the per-session lock.
func TestReentrantCreateResponseFailsFast(t *testing.T) {
	callCount := 0
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			callCount++
			if callCount == 1 {
				return &llm.Response{
					ID:         "resp_1",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.ToolUseContent{ID: "t1", Name: "recurse", Input: []byte(`{}`)}},
					Type:       "message",
					StopReason: "tool_use",
				}, nil
			}
			return &llm.Response{
				ID:         "resp_2",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}

	sess := newMemSession("reentrant")
	var agent *Agent
	var nestedErr error
	tool := &mockTool{
		name: "recurse",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			// Reentrant call on the same session from within a tool. Without
			// reentrancy detection this would deadlock forever.
			_, nestedErr = agent.CreateResponse(ctx, WithInput("nested"))
			return NewToolResultText("recursed"), nil
		},
	}

	var err error
	agent, err = NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	done := make(chan struct{})
	var resp *Response
	var outerErr error
	go func() {
		defer close(done)
		resp, outerErr = agent.CreateResponse(context.Background(), WithInput("start"))
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("CreateResponse deadlocked on reentrant session lock")
	}

	assert.NoError(t, outerErr)
	assert.Equal(t, resp.OutputText(), "Done")
	assert.Error(t, nestedErr)
	assert.True(t, errors.Is(nestedErr, ErrReentrantSession))
}

// TestNewAgentDoesNotContaminateReusedOptions pins that NewAgent clones the
// caller's Tools and Hooks slices before merging extensions, so building two
// agents from one AgentOptions value (with spare slice capacity) does not
// leak one agent's extension hooks into the other.
func TestNewAgentDoesNotContaminateReusedOptions(t *testing.T) {
	newTextLLM := func() *mockLLM {
		return &mockLLM{
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				return &llm.Response{
					ID:         "resp",
					Role:       llm.Assistant,
					Content:    []llm.Content{&llm.TextContent{Text: "ok"}},
					Type:       "message",
					StopReason: "stop",
				}, nil
			},
		}
	}

	var callerCalls, ext1Calls, ext2Calls int
	ext1 := &mockExtension{hooks: Hooks{PreGeneration: []PreGenerationHook{
		func(ctx context.Context, hctx *HookContext) error { ext1Calls++; return nil },
	}}}
	ext2 := &mockExtension{hooks: Hooks{PreGeneration: []PreGenerationHook{
		func(ctx context.Context, hctx *HookContext) error { ext2Calls++; return nil },
	}}}

	// Caller-owned hook slice with spare capacity, so an in-place append in
	// NewAgent would write extension hooks into the shared backing array.
	callerHooks := make([]PreGenerationHook, 0, 4)
	callerHooks = append(callerHooks, func(ctx context.Context, hctx *HookContext) error {
		callerCalls++
		return nil
	})

	opts := AgentOptions{
		Model:      newTextLLM(),
		Hooks:      Hooks{PreGeneration: callerHooks},
		Extensions: []Extension{ext1},
	}
	agent1, err := NewAgent(opts)
	assert.NoError(t, err)

	opts.Model = newTextLLM()
	opts.Extensions = []Extension{ext2}
	agent2, err := NewAgent(opts)
	assert.NoError(t, err)

	// agent1 must run the caller hook + ext1's hook — and NOT ext2's, which
	// (pre-fix) would have overwritten ext1's slot in the shared backing array.
	_, err = agent1.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, callerCalls, 1)
	assert.Equal(t, ext1Calls, 1)
	assert.Equal(t, ext2Calls, 0)

	_, err = agent2.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	assert.Equal(t, callerCalls, 2)
	assert.Equal(t, ext1Calls, 1)
	assert.Equal(t, ext2Calls, 1)
}

// TestResumeCallerResultsMergeDeterministic pins that caller-supplied resume
// results merge into the tool_result message in sorted tool_use-ID order
// rather than nondeterministic map-iteration order.
func TestResumeCallerResultsMergeDeterministic(t *testing.T) {
	var received []*llm.Message
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			cfg := &llm.Config{}
			cfg.Apply(opts...)
			received = cfg.Messages
			return &llm.Response{
				ID:         "resp",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)

	assistant := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
		&llm.ToolUseContent{ID: "toolu_c", Name: "ask", Input: []byte(`{}`)},
		&llm.ToolUseContent{ID: "toolu_a", Name: "ask", Input: []byte(`{}`)},
		&llm.ToolUseContent{ID: "toolu_b", Name: "ask", Input: []byte(`{}`)},
	}}
	state := &SuspensionState{
		PendingToolCalls: []*PendingToolCall{
			{ID: "toolu_c", Name: "ask"},
			{ID: "toolu_a", Name: "ask"},
			{ID: "toolu_b", Name: "ask"},
		},
		TurnMessages: []*llm.Message{llm.NewUserTextMessage("go"), assistant},
	}

	resp, err := agent.CreateResponse(context.Background(), WithResume(state, map[string]*ToolResult{
		"toolu_c": NewToolResultText("c"),
		"toolu_a": NewToolResultText("a"),
		"toolu_b": NewToolResultText("b"),
	}))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "Done")

	// The merged tool_result message sent to the LLM lists results in sorted
	// tool_use-ID order.
	var order []string
	for _, msg := range received {
		if msg.Role != llm.User {
			continue
		}
		for _, c := range msg.Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				order = append(order, trc.ToolUseID)
			}
		}
	}
	assert.Equal(t, order, []string{"toolu_a", "toolu_b", "toolu_c"})
}

// TestResumeMergeSurvivesMessageReplacingPreGenerationHook pins the
// shared-pointer note in §4: resume merging mutates the merged tool_result
// message through a pointer captured in resumeState. A PreGeneration hook
// that replaces hctx.Messages with deep copies (e.g. compaction) used to
// break that sharing, so results from re-executed not-started tools vanished
// from the model-facing history, orphaning their tool_use blocks.
func TestResumeMergeSurvivesMessageReplacingPreGenerationHook(t *testing.T) {
	var received []*llm.Message
	mock := &mockLLM{
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			cfg := &llm.Config{}
			cfg.Apply(opts...)
			received = cfg.Messages
			return &llm.Response{
				ID:         "resp",
				Role:       llm.Assistant,
				Content:    []llm.Content{&llm.TextContent{Text: "Done"}},
				Type:       "message",
				StopReason: "stop",
			}, nil
		},
	}
	noopRan := false
	noop := &mockTool{
		name: "noop",
		callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			noopRan = true
			return NewToolResultText("b-result"), nil
		},
	}

	// PreGeneration hook that replaces every message with a deep copy,
	// severing pointer sharing with the resume state.
	copyAll := func(ctx context.Context, hctx *HookContext) error {
		copied := make([]*llm.Message, len(hctx.Messages))
		for i, m := range hctx.Messages {
			copied[i] = m.Copy()
		}
		hctx.Messages = copied
		return nil
	}

	agent, err := NewAgent(AgentOptions{
		Model: mock,
		Tools: []Tool{noop},
		Hooks: Hooks{PreGeneration: []PreGenerationHook{copyAll}},
	})
	assert.NoError(t, err)

	// Suspended turn: tool A ("ask") suspended (pending); tool B ("noop")
	// never started and must be re-executed on resume.
	assistant := &llm.Message{Role: llm.Assistant, Content: []llm.Content{
		&llm.ToolUseContent{ID: "toolu_ask", Name: "ask", Input: []byte(`{}`)},
		&llm.ToolUseContent{ID: "toolu_noop", Name: "noop", Input: []byte(`{}`)},
	}}
	state := &SuspensionState{
		PendingToolCalls: []*PendingToolCall{{ID: "toolu_ask", Name: "ask"}},
		TurnMessages:     []*llm.Message{llm.NewUserTextMessage("run the tools"), assistant},
	}

	resp, err := agent.CreateResponse(context.Background(), WithResume(state, map[string]*ToolResult{
		"toolu_ask": NewToolResultText("a-result"),
	}))
	assert.NoError(t, err)
	assert.Equal(t, resp.OutputText(), "Done")
	assert.True(t, noopRan, "not-started tool must re-execute on resume")

	// The LLM must see tool_results for BOTH tool_use blocks — including the
	// not-started tool whose result was merged after the hook replaced the
	// message slice with copies.
	resultIDs := make(map[string]bool)
	toolResultMessages := 0
	for _, msg := range received {
		if msg.Role != llm.User {
			continue
		}
		found := false
		for _, c := range msg.Content {
			if trc, ok := c.(*llm.ToolResultContent); ok {
				resultIDs[trc.ToolUseID] = true
				found = true
			}
		}
		if found {
			toolResultMessages++
		}
	}
	assert.Equal(t, toolResultMessages, 1, "exactly one tool_result message expected")
	assert.True(t, resultIDs["toolu_ask"], "caller-supplied result must reach the LLM")
	assert.True(t, resultIDs["toolu_noop"], "re-executed not-started tool result must reach the LLM")
}
