package dive

import (
	"context"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// budgetLLM calls the "read" tool until it has made toolCalls calls, then
// answers. It records the config of every call.
type budgetLLM struct {
	toolCalls int
	configs   []*llm.Config
}

func (m *budgetLLM) Name() string { return "budget-llm" }

func (m *budgetLLM) Generate(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
	config := &llm.Config{}
	config.Apply(opts...)
	m.configs = append(m.configs, config)
	n := len(m.configs)
	if n <= m.toolCalls {
		return &llm.Response{
			ID: fmt.Sprintf("resp_%d", n), Role: llm.Assistant,
			Content: []llm.Content{&llm.ToolUseContent{
				ID: fmt.Sprintf("call_%d", n), Name: "read", Input: []byte(`{}`),
			}},
			StopReason: "tool_use",
			Usage:      llm.Usage{InputTokens: 100, OutputTokens: 500 * n},
		}, nil
	}
	return &llm.Response{
		ID: fmt.Sprintf("resp_%d", n), Role: llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: "report"}},
		StopReason: "end_turn",
		Usage:      llm.Usage{InputTokens: 100, OutputTokens: 50},
	}, nil
}

func budgetAgent(t *testing.T, model llm.LLM, budget *llm.TaskBudget, hooks Hooks) *Agent {
	t.Helper()
	agent, err := NewAgent(AgentOptions{
		Model:         model,
		ModelSettings: &ModelSettings{TaskBudget: budget},
		Hooks:         hooks,
		Tools: []Tool{&mockTool{name: "read", callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewToolResultText(fmt.Sprintf("%0400d", 7)), nil
		}}},
	})
	assert.NoError(t, err)
	return agent
}

// In a loop that sends its full history, every call carries the same total
// and no remaining: the provider derives the spend from the history.
func TestTaskBudgetSameTotalOnEveryCall(t *testing.T) {
	model := &budgetLLM{toolCalls: 3}
	agent := budgetAgent(t, model, &llm.TaskBudget{Total: 40000}, Hooks{})

	_, err := agent.CreateResponse(context.Background(), WithInput("read a lot"))
	assert.NoError(t, err)

	assert.Len(t, model.configs, 4)
	for _, config := range model.configs {
		assert.Equal(t, &llm.TaskBudget{Total: 40000}, config.TaskBudget)
	}
}

// After a PreIteration hook replaces messages of the turn with a summary, each
// later call carries the remaining budget: the total less what the removed
// messages spent, which the provider can no longer derive.
func TestTaskBudgetRemainingAfterMidTurnCompaction(t *testing.T) {
	model := &budgetLLM{toolCalls: 3}
	var removed []*llm.Message
	compact := func(ctx context.Context, hctx *HookContext) error {
		if hctx.Iteration != 2 {
			return nil
		}
		// user, assistant 1, result 1, assistant 2, result 2: summarize the
		// first exchange and keep the second.
		msgs := hctx.Messages
		removed = msgs[1:3]
		summary := llm.NewUserTextMessage("<summary>read one file</summary>")
		hctx.Messages = append([]*llm.Message{msgs[0], summary}, msgs[3:]...)
		return nil
	}
	agent := budgetAgent(t, model, &llm.TaskBudget{Total: 40000}, Hooks{
		PreIteration: []PreIterationHook{compact},
	})

	_, err := agent.CreateResponse(context.Background(), WithInput("read a lot"))
	assert.NoError(t, err)

	assert.Len(t, model.configs, 4)
	assert.Len(t, removed, 2)
	// The first response's output tokens are exact; its tool result is
	// estimated.
	want := 40000 - 500 - llm.EstimateTokens(removed[1])
	for i, config := range model.configs {
		if i < 2 {
			assert.Equal(t, &llm.TaskBudget{Total: 40000}, config.TaskBudget)
			continue
		}
		assert.Equal(t, &llm.TaskBudget{Total: 40000, Remaining: &want}, config.TaskBudget)
	}
}

func TestTaskBudgetNotSetWithoutModelSettings(t *testing.T) {
	model := &budgetLLM{toolCalls: 1}
	agent, err := NewAgent(AgentOptions{
		Model: model,
		Tools: []Tool{&mockTool{name: "read", callFunc: func(ctx context.Context, input any) (*ToolResult, error) {
			return NewToolResultText("ok"), nil
		}}},
	})
	assert.NoError(t, err)
	_, err = agent.CreateResponse(context.Background(), WithInput("hi"))
	assert.NoError(t, err)
	for _, config := range model.configs {
		assert.Nil(t, config.TaskBudget)
	}
}

func TestTaskBudgetSpendOption(t *testing.T) {
	kept := llm.NewUserTextMessage("kept")
	answer := llm.NewAssistantTextMessage("answer")
	result := llm.NewToolResultMessage(&llm.ToolResultContent{ToolUseID: "call_1", Content: "result"})

	var spend taskBudgetSpend
	spend.track(answer)
	spend.setGenerated(answer, 30000)
	spend.track(result)

	t.Run("nothing removed", func(t *testing.T) {
		assert.Nil(t, spend.option(&llm.TaskBudget{Total: 40000}, []*llm.Message{kept, answer, result}))
	})
	t.Run("no budget", func(t *testing.T) {
		assert.Nil(t, spend.option(nil, []*llm.Message{kept}))
	})

	budgetOf := func(budget llm.TaskBudget, working ...*llm.Message) *llm.TaskBudget {
		config := &llm.Config{}
		config.Apply(spend.option(&budget, working))
		return config.TaskBudget
	}
	t.Run("caller's remaining is the starting point", func(t *testing.T) {
		start := 35000
		got := budgetOf(llm.TaskBudget{Total: 40000, Remaining: &start}, kept, result)
		assert.Equal(t, 5000, *got.Remaining)
	})
	t.Run("never below zero", func(t *testing.T) {
		got := budgetOf(llm.TaskBudget{Total: 20000}, kept)
		assert.Equal(t, 20000, got.Total)
		assert.Equal(t, 0, *got.Remaining)
	})
}
