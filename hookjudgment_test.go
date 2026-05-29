package dive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// decisionModel returns a mockLLM that answers a judgment call with the given
// verdict via the forced submit_decision tool.
func decisionModel(ok bool, reason string) *mockLLM {
	return &mockLLM{
		nameFunc: func() string { return "judge" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			input, _ := json.Marshal(JudgmentDecision{OK: ok, Reason: reason})
			return &llm.Response{
				ID:         "j",
				Model:      "judge",
				Role:       llm.Assistant,
				Type:       "message",
				StopReason: "tool_use",
				Content:    []llm.Content{&llm.ToolUseContent{ID: "t1", Name: judgmentToolName, Input: input}},
			}, nil
		},
	}
}

func erroringModel() *mockLLM {
	return &mockLLM{
		nameFunc: func() string { return "judge" },
		generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
			return nil, errors.New("transport boom")
		},
	}
}

func TestPromptToolGate(t *testing.T) {
	call := &llm.ToolUseContent{ID: "c1", Name: "Bash", Input: json.RawMessage(`{"command":"rm -rf /"}`)}

	t.Run("allows when model approves", func(t *testing.T) {
		hctx := NewHookContext()
		hctx.Call = call
		err := PromptToolGate(decisionModel(true, ""), "Is this safe?")(context.Background(), hctx)
		assert.NoError(t, err)
	})

	t.Run("denies and surfaces the reason", func(t *testing.T) {
		hctx := NewHookContext()
		hctx.Call = call
		err := PromptToolGate(decisionModel(false, "rm -rf is destructive"), "Is this safe?")(context.Background(), hctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rm -rf is destructive")
	})

	t.Run("fails closed on model error", func(t *testing.T) {
		hctx := NewHookContext()
		hctx.Call = call
		err := PromptToolGate(erroringModel(), "Is this safe?")(context.Background(), hctx)
		assert.Error(t, err) // a returned error denies the tool
	})

	t.Run("forces the submit_decision tool choice", func(t *testing.T) {
		var gotChoice *llm.ToolChoice
		var gotTools []llm.Tool
		model := &mockLLM{
			nameFunc: func() string { return "judge" },
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				var cfg llm.Config
				cfg.Apply(opts...)
				gotChoice = cfg.ToolChoice
				gotTools = cfg.Tools
				input, _ := json.Marshal(JudgmentDecision{OK: true})
				return &llm.Response{
					ID: "j", Role: llm.Assistant, Type: "message",
					Content: []llm.Content{&llm.ToolUseContent{Name: judgmentToolName, Input: input}},
				}, nil
			},
		}
		hctx := NewHookContext()
		hctx.Call = call
		err := PromptToolGate(model, "ok?")(context.Background(), hctx)
		assert.NoError(t, err)
		assert.NotNil(t, gotChoice)
		assert.Equal(t, llm.ToolChoiceTypeTool, gotChoice.Type)
		assert.Equal(t, judgmentToolName, gotChoice.Name)
		assert.Equal(t, 1, len(gotTools))
		assert.Equal(t, judgmentToolName, gotTools[0].Name())
	})
}

func TestPromptStopHook(t *testing.T) {
	prompt := "Is the user's request fully satisfied?"
	output := []*llm.Message{llm.NewAssistantTextMessage("I did half the task.")}

	t.Run("allows stop when model says done", func(t *testing.T) {
		hctx := NewHookContext()
		hctx.OutputMessages = output
		dec, err := PromptStopHook(decisionModel(true, ""), prompt)(context.Background(), hctx)
		assert.NoError(t, err)
		assert.Nil(t, dec)
	})

	t.Run("continues with reason when model says not done", func(t *testing.T) {
		hctx := NewHookContext()
		hctx.OutputMessages = output
		dec, err := PromptStopHook(decisionModel(false, "finish the second half"), prompt)(context.Background(), hctx)
		assert.NoError(t, err)
		assert.NotNil(t, dec)
		assert.True(t, dec.Continue)
		assert.Equal(t, "finish the second half", dec.Reason)
	})

	t.Run("steps aside once StopHookActive (loop guard)", func(t *testing.T) {
		consulted := false
		model := &mockLLM{
			nameFunc: func() string { return "judge" },
			generateFunc: func(ctx context.Context, opts ...llm.Option) (*llm.Response, error) {
				consulted = true
				return nil, errors.New("should not be called")
			},
		}
		hctx := NewHookContext()
		hctx.OutputMessages = output
		hctx.StopHookActive = true
		dec, err := PromptStopHook(model, prompt)(context.Background(), hctx)
		assert.NoError(t, err)
		assert.Nil(t, dec)
		assert.False(t, consulted, "judge must not be consulted once a continuation already happened")
	})

	t.Run("fails open on model error", func(t *testing.T) {
		hctx := NewHookContext()
		hctx.OutputMessages = output
		dec, err := PromptStopHook(erroringModel(), prompt)(context.Background(), hctx)
		// The error is returned so the agent logs it; Dive treats a Stop-hook
		// error as no-decision, i.e. the agent is allowed to stop (fail open).
		assert.Error(t, err)
		assert.Nil(t, dec)
	})
}
