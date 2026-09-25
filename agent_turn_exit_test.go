package dive_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// These tests pin what each exit after the turn boundary returns, so the
// exit paths can be restructured without changing it.

// A model error after a Stop-hook continuation reports the whole turn: both
// rounds' output, the continuation reminder, every item and the usage of
// every model call.
func TestGenerationErrorAfterStopContinuationCarriesWholeTurn(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{text: "first round", usage: llm.Usage{InputTokens: 10, OutputTokens: 4}},
			// The script ends here, so the continuation's model call fails.
		},
	}
	sess := session.New("stop-continue-error")
	agent, err := NewAgent(AgentOptions{
		Model:   mock,
		Session: sess,
		Hooks: Hooks{
			Stop: []StopHook{
				func(ctx context.Context, hctx *HookContext) (*StopDecision, error) {
					return &StopDecision{Continue: true, Reason: "keep going"}, nil
				},
			},
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.Nil(t, resp)
	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr))
	assert.Equal(t, mock.Calls(), 1)

	assert.Len(t, genErr.OutputMessages, 2)
	assert.Equal(t, genErr.OutputMessages[0].Text(), "first round")
	_, ok := FindReminder(genErr.OutputMessages[1], "stop-continuation")
	assert.True(t, ok)

	assert.Len(t, genErr.Items, 1)
	assert.Equal(t, genErr.Items[0].Type, ResponseItemTypeMessage)
	assert.Equal(t, genErr.Usage.InputTokens, 10)
	assert.Equal(t, genErr.Usage.OutputTokens, 4)

	// The partial turn is not saved.
	assert.Equal(t, sess.EventCount(), 0)
}

// A callback error on the assistant message item still reports the message
// and its usage: both are recorded before the callback runs.
func TestGenerationErrorWhenCallbackRejectsAssistantMessage(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			{text: "hello", usage: llm.Usage{InputTokens: 7, OutputTokens: 3}},
		},
	}
	agent, err := NewAgent(AgentOptions{Model: mock})
	assert.NoError(t, err)

	errRejected := errors.New("rejected")
	resp, err := agent.CreateResponse(context.Background(),
		WithInput("hi"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			if item.Type == ResponseItemTypeMessage {
				return errRejected
			}
			return nil
		}),
	)
	assert.Nil(t, resp)
	assert.True(t, errors.Is(err, errRejected))
	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr))
	assert.Len(t, genErr.OutputMessages, 1)
	assert.Equal(t, genErr.OutputMessages[0].Text(), "hello")
	assert.Len(t, genErr.Items, 1)
	assert.Equal(t, genErr.Usage.InputTokens, 7)
	assert.Equal(t, genErr.Usage.OutputTokens, 3)
}

// A model error after a full resume reports the caller-supplied results
// emitted in the resume phase, with zero usage.
func TestGenerationErrorAfterResumeCarriesResumeItems(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
			// The script ends here, so the model call after the resume fails.
		},
	}
	tool := &scriptedTool{name: "approve", outcomes: []toolOutcome{{result: NewSuspendResult("wait", nil)}}}
	sess := session.New("resume-error")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)

	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("approved")}),
	)
	assert.Nil(t, resp)
	var genErr *GenerationError
	assert.True(t, errors.As(err, &genErr))
	assert.Len(t, genErr.Items, 1)
	assert.Equal(t, genErr.Items[0].Type, ResponseItemTypeToolCallResult)
	assert.Equal(t, genErr.Items[0].ToolCallResult.ID, "toolu_a")
	assert.Len(t, genErr.OutputMessages, 0)
	assert.NotNil(t, genErr.Usage)
	assert.Equal(t, genErr.Usage.InputTokens, 0)

	// The session is still suspended on the original turn.
	assert.True(t, sessIsSuspended(sess))
}

// A partial resume never calls the model, so it reports no usage and no
// output; its items are the results the caller supplied.
func TestPartialResumeReportsNoUsage(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("toolu_a", "tool_a", `{}`),
				newScriptedToolUse("toolu_b", "tool_b", `{}`),
			),
		},
	}
	toolA := &scriptedTool{name: "tool_a", outcomes: []toolOutcome{{result: NewSuspendResult("wait a", nil)}}}
	toolB := &scriptedTool{name: "tool_b", outcomes: []toolOutcome{{result: NewSuspendResult("wait b", nil)}}}
	sess := session.New("partial-usage")
	agent, err := NewAgent(AgentOptions{
		Model:                 mock,
		Tools:                 []Tool{toolA, toolB},
		Session:               sess,
		ParallelToolExecution: true,
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(context.Background(), WithInput("start"))
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.NotNil(t, resp.Usage)

	resp, err = agent.CreateResponse(context.Background(),
		WithToolResults(map[string]*ToolResult{"toolu_a": NewToolResultText("A done")}),
	)
	assert.NoError(t, err)
	assert.Equal(t, resp.Status, ResponseStatusSuspended)
	assert.Nil(t, resp.Usage)
	assert.Len(t, resp.OutputMessages, 0)
	assert.Len(t, resp.Items, 1)
	assert.Equal(t, resp.Items[0].ToolCallResult.ID, "toolu_a")
	assert.NotNil(t, resp.FinishedAt)
	assert.Equal(t, mock.Calls(), 1)
}

// A callback error on the terminal suspended item is returned as is, after
// the suspended turn was saved.
func TestSuspendedItemCallbackError(t *testing.T) {
	mock := &scriptedLLM{
		script: []scriptedTurn{
			toolUseAssistantTurn(newScriptedToolUse("toolu_a", "approve", `{}`)),
		},
	}
	tool := &scriptedTool{name: "approve", outcomes: []toolOutcome{{result: NewSuspendResult("wait", nil)}}}
	sess := session.New("suspended-item-error")
	agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{tool}, Session: sess})
	assert.NoError(t, err)

	errRejected := errors.New("rejected")
	resp, err := agent.CreateResponse(context.Background(),
		WithInput("start"),
		WithEventCallback(func(ctx context.Context, item *ResponseItem) error {
			if item.Type == ResponseItemTypeSuspended {
				return errRejected
			}
			return nil
		}),
	)
	assert.Nil(t, resp)
	assert.Equal(t, err, errRejected)
	assert.True(t, sessIsSuspended(sess))
}
