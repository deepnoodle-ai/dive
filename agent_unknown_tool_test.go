package dive

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

func TestSuggestToolNames(t *testing.T) {
	tools := func(names ...string) map[string]Tool {
		result := make(map[string]Tool, len(names))
		for _, name := range names {
			result[name] = &mockTool{name: name}
		}
		return result
	}

	t.Run("segment suffix recovers wrong namespace", func(t *testing.T) {
		query := "mobius_market_clock"
		assert.True(t,
			levenshteinDistance(query, "core_market_clock") > min(3, len(query)/4),
			"the production incident must be recovered by the suffix tier, not edit distance",
		)
		assert.Equal(t,
			suggestToolNames(query, tools("core_market_clock", "mobius_memory_recall")),
			[]string{"core_market_clock"},
		)
	})

	t.Run("short single suffix is below the floor", func(t *testing.T) {
		assert.Nil(t, suggestToolNames("mobius_clock", tools("core_clock")))
	})

	t.Run("equal suffix scores use complete wire name and cap at three", func(t *testing.T) {
		expected := []string{"a_market_clock", "m_market_clock", "q_market_clock"}
		orders := [][]string{
			{"z_market_clock", "a_market_clock", "q_market_clock", "m_market_clock"},
			{"m_market_clock", "q_market_clock", "a_market_clock", "z_market_clock"},
		}
		for _, order := range orders {
			assert.Equal(t, suggestToolNames("mobius_market_clock", tools(order...)), expected)
		}
	})

	t.Run("bounded edit distance handles ordinary typo", func(t *testing.T) {
		assert.Equal(t,
			suggestToolNames("core_market_clok", tools("core_market_clock", "core_portfolio_value")),
			[]string{"core_market_clock"},
		)
	})

	t.Run("namespace tier normalizes all supported delimiters", func(t *testing.T) {
		assert.Equal(t,
			suggestToolNames("core.market.missing", tools(
				"core_market_beta",
				"core-market-alpha",
				"core.market.gamma",
				"flat",
			)),
			[]string{"core-market-alpha", "core.market.gamma", "core_market_beta"},
		)
	})

	t.Run("flat names do not qualify as namespaces", func(t *testing.T) {
		assert.Nil(t, suggestToolNames("unsupported", tools("unrelated", "another")))
	})

	t.Run("suggestions contain declared tools only", func(t *testing.T) {
		declared := tools("core_market_clock")
		suggestions := suggestToolNames("mobius_market_clock", declared)
		assert.Equal(t, suggestions, []string{"core_market_clock"})
		for _, suggestion := range suggestions {
			_, ok := declared[suggestion]
			assert.True(t, ok)
		}
	})
}

func TestUnknownToolMessage(t *testing.T) {
	assert.Equal(t,
		unknownToolMessage("mobius_market_clock", []string{"core_market_clock"}),
		`Tool "mobius_market_clock" does not exist and was not called. Did you mean: core_market_clock. Call one of the tools declared for this turn.`,
	)
	assert.Equal(t,
		unknownToolMessage("invented", nil),
		`Tool "invented" does not exist and was not called. Call one of the tools declared for this turn.`,
	)
}

func TestUnknownToolRecoversMixedBatch(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(fmt.Sprintf("parallel=%t", parallel), func(t *testing.T) {
			var toolCalls atomic.Int32
			validTool := &mockTool{
				name: "core_market_clock",
				callFunc: func(context.Context, any) (*ToolResult, error) {
					toolCalls.Add(1)
					return NewToolResultText("market is open"), nil
				},
			}

			modelCalls := 0
			var secondRequest []*llm.Message
			model := &mockLLM{generateFunc: func(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
				modelCalls++
				var cfg llm.Config
				cfg.Apply(opts...)
				if modelCalls == 1 {
					return unknownToolUseResponse("first",
						&llm.ToolUseContent{ID: "valid", Name: "core_market_clock", Input: []byte(`{}`)},
						&llm.ToolUseContent{ID: "unknown", Name: "mobius_market_clock", Input: []byte(`{"timezone":"UTC"}`)},
					), nil
				}
				secondRequest = slices.Clone(cfg.Messages)
				return unknownTextResponse("second", "Recovered and finished"), nil
			}}

			var preNames, postNames, failureNames []string
			logger := &recordingLogger{}
			agent, err := NewAgent(AgentOptions{
				Name:                  "market-agent",
				Model:                 model,
				Tools:                 []Tool{validTool},
				ParallelToolExecution: parallel,
				Logger:                logger,
				Hooks: Hooks{
					PreToolUse: []PreToolUseHook{func(_ context.Context, hctx *HookContext) error {
						preNames = append(preNames, hctx.Call.Name)
						return nil
					}},
					PostToolUse: []PostToolUseHook{func(_ context.Context, hctx *HookContext) error {
						postNames = append(postNames, hctx.Call.Name)
						return nil
					}},
					PostToolUseFailure: []PostToolUseFailureHook{func(_ context.Context, hctx *HookContext) error {
						failureNames = append(failureNames, hctx.Call.Name)
						return nil
					}},
				},
			})
			assert.NoError(t, err)

			resp, err := agent.CreateResponse(context.Background(), WithInput("check the market"))
			assert.NoError(t, err)
			assert.Equal(t, resp.OutputText(), "Recovered and finished")
			assert.Equal(t, toolCalls.Load(), int32(1))
			assert.Equal(t, preNames, []string{"core_market_clock"})
			assert.Equal(t, postNames, []string{"core_market_clock"})
			assert.Len(t, failureNames, 0)

			results := resp.ToolCallResults()
			assert.Len(t, results, 2)
			resultsByName := map[string]*ToolCallResult{}
			for _, result := range results {
				resultsByName[result.Name] = result
			}
			validResult := resultsByName["core_market_clock"]
			unknownResult := resultsByName["mobius_market_clock"]
			assert.NotNil(t, validResult)
			assert.NotNil(t, unknownResult)
			assert.False(t, validResult.Result.IsError)
			assert.True(t, unknownResult.Result.IsError)
			var unknownErr *UnknownToolError
			assert.True(t, errors.As(unknownResult.Error, &unknownErr))
			assert.Equal(t, unknownErr.Name, "mobius_market_clock")
			assert.Equal(t, unknownErr.Suggestions, []string{"core_market_clock"})
			assert.True(t, strings.Contains(toolResultText(unknownResult.Result), "was not called"))

			assert.Equal(t, countResponseItems(resp.Items, ResponseItemTypeToolCall), 2)
			assert.Equal(t, countResponseItems(resp.Items, ResponseItemTypeToolCallResult), 2)
			assert.Equal(t, countToolResultsInMessages(secondRequest), 2,
				"each tool_use in the mixed batch must have one tool_result")
			assert.Equal(t, logger.warnCount("unknown tool requested"), 1)
			warning := logger.firstWarn("unknown tool requested")
			assert.Equal(t, warning.value("tool_name"), "mobius_market_clock")
			assert.Equal(t, warning.value("agent_name"), "market-agent")
			assert.Equal(t, warning.value("suggestions"), []string{"core_market_clock"})
		})
	}
}

func TestUnknownOnlyCallContinuesGeneration(t *testing.T) {
	calls := 0
	model := &mockLLM{generateFunc: func(_ context.Context, _ ...llm.Option) (*llm.Response, error) {
		calls++
		if calls == 1 {
			return unknownToolUseResponse("unknown", &llm.ToolUseContent{
				ID: "unknown", Name: "invented", Input: []byte(`{}`),
			}), nil
		}
		return unknownTextResponse("final", "I could not call that tool."), nil
	}}
	agent, err := NewAgent(AgentOptions{Model: model})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("continue"))
	assert.NoError(t, err)
	assert.Equal(t, calls, 2)
	assert.Equal(t, resp.OutputText(), "I could not call that tool.")
	assert.Len(t, resp.ToolCallResults(), 1)
	assert.True(t, resp.ToolCallResults()[0].Result.IsError)
}

func TestRepeatedUnknownToolsUseStandardIterationLimit(t *testing.T) {
	calls := 0
	model := &mockLLM{generateFunc: func(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
		calls++
		var cfg llm.Config
		cfg.Apply(opts...)
		if calls <= 2 {
			if cfg.ToolChoice != nil && cfg.ToolChoice.Type == llm.ToolChoiceTypeNone {
				return nil, fmt.Errorf("tool choice disabled before the standard iteration limit")
			}
			return unknownToolUseResponse(fmt.Sprintf("unknown-%d", calls), &llm.ToolUseContent{
				ID: fmt.Sprintf("unknown-%d", calls), Name: "invented", Input: []byte(`{}`),
			}), nil
		}
		if cfg.ToolChoice == nil || cfg.ToolChoice.Type != llm.ToolChoiceTypeNone {
			return nil, fmt.Errorf("standard final iteration did not force tool choice none")
		}
		return unknownTextResponse("final", "Stopped at the configured limit."), nil
	}}
	agent, err := NewAgent(AgentOptions{Model: model, ToolIterationLimit: 2})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("continue"))
	assert.NoError(t, err)
	assert.Equal(t, calls, 3)
	assert.Equal(t, resp.OutputText(), "Stopped at the configured limit.")
}

func TestUnknownToolDuringResumeBecomesResult(t *testing.T) {
	var dynamicAvailable atomic.Bool
	dynamicAvailable.Store(true)
	laterTool := &mockTool{
		name: "later_tool",
		callFunc: func(context.Context, any) (*ToolResult, error) {
			return NewToolResultText("should not run"), nil
		},
	}
	toolset := &ToolsetFunc{
		ToolsetName: "dynamic",
		Resolve: func(context.Context) ([]Tool, error) {
			if dynamicAvailable.Load() {
				return []Tool{laterTool}, nil
			}
			return nil, nil
		},
	}
	approve := &mockTool{
		name: "approve",
		callFunc: func(context.Context, any) (*ToolResult, error) {
			return NewSuspendResult("approve the turn", nil), nil
		},
	}
	modelCalls := 0
	model := &mockLLM{generateFunc: func(_ context.Context, _ ...llm.Option) (*llm.Response, error) {
		modelCalls++
		if modelCalls == 1 {
			return unknownToolUseResponse("suspend",
				&llm.ToolUseContent{ID: "approval", Name: "approve", Input: []byte(`{}`)},
				&llm.ToolUseContent{ID: "later", Name: "later_tool", Input: []byte(`{}`)},
			), nil
		}
		return unknownTextResponse("final", "Resume recovered."), nil
	}}

	var preNames, postNames, failureNames []string
	agent, err := NewAgent(AgentOptions{
		Model:    model,
		Tools:    []Tool{approve},
		Toolsets: []Toolset{toolset},
		Hooks: Hooks{
			PreToolUse: []PreToolUseHook{func(_ context.Context, hctx *HookContext) error {
				preNames = append(preNames, hctx.Call.Name)
				return nil
			}},
			PostToolUse: []PostToolUseHook{func(_ context.Context, hctx *HookContext) error {
				postNames = append(postNames, hctx.Call.Name)
				return nil
			}},
			PostToolUseFailure: []PostToolUseFailureHook{func(_ context.Context, hctx *HookContext) error {
				failureNames = append(failureNames, hctx.Call.Name)
				return nil
			}},
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	dynamicAvailable.Store(false)

	resp, err = agent.CreateResponse(context.Background(),
		WithResume(resp.Suspension, map[string]*ToolResult{
			"approval": NewToolResultText("approved"),
		}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusCompleted)
	assert.Equal(t, resp.OutputText(), "Resume recovered.")
	assert.Equal(t, preNames, []string{"approve"})
	assert.Equal(t, postNames, []string{"approve"})
	assert.Len(t, failureNames, 0)

	var recovered *ToolCallResult
	for _, result := range resp.ToolCallResults() {
		if result.ID == "later" {
			recovered = result
		}
	}
	assert.NotNil(t, recovered)
	assert.True(t, recovered.Result.IsError)
	var unknownErr *UnknownToolError
	assert.True(t, errors.As(recovered.Error, &unknownErr))
	assert.Equal(t, unknownErr.Name, "later_tool")
}

func unknownToolUseResponse(id string, calls ...*llm.ToolUseContent) *llm.Response {
	content := make([]llm.Content, len(calls))
	for i, call := range calls {
		content[i] = call
	}
	return &llm.Response{
		ID: id, Role: llm.Assistant, Content: content, Type: "message", StopReason: "tool_use",
	}
}

func unknownTextResponse(id, text string) *llm.Response {
	return &llm.Response{
		ID: id, Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: text}}, Type: "message", StopReason: "stop",
	}
}

func countResponseItems(items []*ResponseItem, itemType ResponseItemType) int {
	count := 0
	for _, item := range items {
		if item.Type == itemType {
			count++
		}
	}
	return count
}

func countToolResultsInMessages(messages []*llm.Message) int {
	count := 0
	for _, message := range messages {
		for _, content := range message.Content {
			if _, ok := content.(*llm.ToolResultContent); ok {
				count++
			}
		}
	}
	return count
}

type recordedLog struct {
	message string
	args    []any
}

func (l recordedLog) value(key string) any {
	for i := 0; i+1 < len(l.args); i += 2 {
		if l.args[i] == key {
			return l.args[i+1]
		}
	}
	return nil
}

type recordingLogger struct {
	mu    sync.Mutex
	warns []recordedLog
}

func (l *recordingLogger) Debug(string, ...any) {}
func (l *recordingLogger) Info(string, ...any)  {}
func (l *recordingLogger) Error(string, ...any) {}
func (l *recordingLogger) Warn(message string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, recordedLog{message: message, args: slices.Clone(args)})
}
func (l *recordingLogger) With(...any) llm.Logger { return l }

func (l *recordingLogger) warnCount(message string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	count := 0
	for _, warning := range l.warns {
		if warning.message == message {
			count++
		}
	}
	return count
}

func (l *recordingLogger) firstWarn(message string) recordedLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, warning := range l.warns {
		if warning.message == message {
			return warning
		}
	}
	return recordedLog{}
}
