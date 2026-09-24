package dive_test

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/dive/session"
	"github.com/deepnoodle-ai/wonton/assert"
)

// haltingTool is a scriptedTool annotated HaltsBatch.
type haltingTool struct{ *scriptedTool }

func (t haltingTool) Annotations() *ToolAnnotations { return &ToolAnnotations{HaltsBatch: true} }

func computerCall(id, name string) *llm.ToolUseContent {
	c := newScriptedToolUse(id, name, `{}`)
	c.ToolsetName = "computer"
	return c
}

// lastToolResultText returns the text the model was sent for a tool_use id
// on its most recent request.
func lastToolResultText(mock *scriptedLLM, id string) (string, bool) {
	mock.mu.Lock()
	defer mock.mu.Unlock()
	msgs := mock.received[len(mock.received)-1]
	for _, msg := range msgs {
		for _, c := range msg.Content {
			trc, ok := c.(*llm.ToolResultContent)
			if !ok || trc.ToolUseID != id {
				continue
			}
			// The recorded copy went through JSON, so decode the blocks back.
			var blocks []*ToolResultContent
			data, _ := json.Marshal(trc.Content)
			if json.Unmarshal(data, &blocks) == nil && len(blocks) > 0 {
				return blocks[0].Text, trc.IsError
			}
		}
	}
	return "", false
}

func TestBatchHaltAcrossResume(t *testing.T) {
	const haltText = "Not executed: an earlier computer action in this turn failed."

	t.Run("a failed result supplied on resume halts the not-started calls", func(t *testing.T) {
		mock := &scriptedLLM{script: []scriptedTurn{
			toolUseAssistantTurn(computerCall("c1", "left_click"), computerCall("c2", "type")),
			finalTextTurn("done"),
		}}
		click := &scriptedTool{name: "left_click", outcomes: []toolOutcome{{result: NewSuspendResult("allow the click?", nil)}}}
		typ := &scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}
		sess := session.New("halt-resume-denied")
		agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{click, typ}, Session: sess})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusSuspended)

		resp, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{
			"c1": NewToolResultError("the person declined"),
		}))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusCompleted)
		assert.Equal(t, typ.CallCount(), 0)
		text, isError := lastToolResultText(mock, "c2")
		assert.Equal(t, text, haltText)
		assert.True(t, isError)
	})

	t.Run("a successful result supplied on resume lets them run", func(t *testing.T) {
		mock := &scriptedLLM{script: []scriptedTurn{
			toolUseAssistantTurn(computerCall("c1", "left_click"), computerCall("c2", "type")),
			finalTextTurn("done"),
		}}
		click := &scriptedTool{name: "left_click", outcomes: []toolOutcome{{result: NewSuspendResult("allow the click?", nil)}}}
		typ := &scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}
		sess := session.New("halt-resume-allowed")
		agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{click, typ}, Session: sess})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		_, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{
			"c1": NewToolResultText("clicked"),
		}))
		assert.NoError(t, err)
		assert.Equal(t, typ.CallCount(), 1)
	})

	t.Run("a failure before the suspension still halts after it", func(t *testing.T) {
		mock := &scriptedLLM{script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("c1", "click", `{}`), // fails
				newScriptedToolUse("c2", "ask", `{}`),   // plain; suspends
				newScriptedToolUse("c3", "type", `{}`),  // not started, then halted
			),
			finalTextTurn("done"),
		}}
		click := haltingTool{&scriptedTool{name: "click", outcomes: []toolOutcome{{result: NewToolResultError("missed")}}}}
		ask := &scriptedTool{name: "ask", outcomes: []toolOutcome{{result: NewSuspendResult("which one?", nil)}}}
		typ := haltingTool{&scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}}
		sess := session.New("halt-before-suspend")
		agent, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{click, ask, typ}, Session: sess})
		assert.NoError(t, err)

		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusSuspended)

		resp, err = agent.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{
			"c2": NewToolResultText("the left one"),
		}))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusCompleted)
		assert.Equal(t, typ.CallCount(), 0)
		text, isError := lastToolResultText(mock, "c3")
		assert.Equal(t, text, "Not executed: an earlier tool call in this response failed.")
		assert.True(t, isError)
	})
}

// The suspension records that the batch halted, so a resume keeps it halted
// even when the agent resuming it no longer has the failed tool, or has it
// without HaltsBatch, as after a deploy while the turn waited.
func TestBatchHaltSurvivesToolChangesAcrossResume(t *testing.T) {
	script := func() *scriptedLLM {
		return &scriptedLLM{script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("c1", "click", `{}`), // fails
				newScriptedToolUse("c2", "ask", `{}`),   // plain; suspends
				newScriptedToolUse("c3", "type", `{}`),  // not started, then halted
			),
			finalTextTurn("done"),
		}}
	}
	suspend := func(t *testing.T, mock *scriptedLLM, sess *session.Session) *SuspensionState {
		click := haltingTool{&scriptedTool{name: "click", outcomes: []toolOutcome{{result: NewToolResultError("missed")}}}}
		ask := &scriptedTool{name: "ask", outcomes: []toolOutcome{{result: NewSuspendResult("which one?", nil)}}}
		typ := haltingTool{&scriptedTool{name: "type"}}
		opts := AgentOptions{Model: mock, Tools: []Tool{click, ask, typ}}
		if sess != nil {
			opts.Session = sess
		}
		agent, err := NewAgent(opts)
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusSuspended)
		assert.True(t, resp.Suspension.BatchHalted)
		return resp.Suspension
	}
	const haltText = "Not executed: an earlier tool call in this response failed."

	for name, clickTool := range map[string]Tool{
		"failed tool unregistered": nil,
		"failed tool unannotated":  &scriptedTool{name: "click"},
	} {
		t.Run("session, "+name, func(t *testing.T) {
			mock := script()
			sess := session.New("halt-registry-" + name)
			suspend(t, mock, sess)

			ask := &scriptedTool{name: "ask"}
			typ := haltingTool{&scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}}
			tools := []Tool{ask, typ}
			if clickTool != nil {
				tools = append(tools, clickTool)
			}
			resumer, err := NewAgent(AgentOptions{Model: mock, Tools: tools, Session: sess})
			assert.NoError(t, err)
			resp, err := resumer.CreateResponse(context.Background(), WithToolResults(map[string]*ToolResult{
				"c2": NewToolResultText("the left one"),
			}))
			assert.NoError(t, err)
			assert.Equal(t, resp.Status, ResponseStatusCompleted)
			assert.Equal(t, typ.CallCount(), 0)
			text, isError := lastToolResultText(mock, "c3")
			assert.Equal(t, text, haltText)
			assert.True(t, isError)
		})
	}

	t.Run("stateless, state round-tripped through JSON", func(t *testing.T) {
		mock := script()
		state := suspend(t, mock, nil)
		data, err := json.Marshal(state)
		assert.NoError(t, err)
		var restored SuspensionState
		assert.NoError(t, json.Unmarshal(data, &restored))

		ask := &scriptedTool{name: "ask"}
		typ := haltingTool{&scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}}
		resumer, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{ask, typ}})
		assert.NoError(t, err)
		resp, err := resumer.CreateResponse(context.Background(), WithResume(&restored, map[string]*ToolResult{
			"c2": NewToolResultText("the left one"),
		}))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusCompleted)
		assert.Equal(t, typ.CallCount(), 0)
		text, _ := lastToolResultText(mock, "c3")
		assert.Equal(t, text, haltText)
	})
}

// A pending call records that it takes part in batch halting, so an error
// the caller supplies for it on resume halts the batch even when the agent
// resuming it no longer has the tool, or has it without HaltsBatch.
func TestSuspendedHaltingCallFailureSurvivesToolChanges(t *testing.T) {
	script := func() *scriptedLLM {
		return &scriptedLLM{script: []scriptedTurn{
			toolUseAssistantTurn(
				newScriptedToolUse("c1", "click", `{}`), // halting; suspends for approval
				newScriptedToolUse("c2", "type", `{}`),  // not started, then halted
			),
			finalTextTurn("done"),
		}}
	}
	suspend := func(t *testing.T, mock *scriptedLLM, sess *session.Session) *SuspensionState {
		click := haltingTool{&scriptedTool{name: "click", outcomes: []toolOutcome{{result: NewSuspendResult("allow the click?", nil)}}}}
		typ := haltingTool{&scriptedTool{name: "type"}}
		opts := AgentOptions{Model: mock, Tools: []Tool{click, typ}}
		if sess != nil {
			opts.Session = sess
		}
		agent, err := NewAgent(opts)
		assert.NoError(t, err)
		resp, err := agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusSuspended)
		assert.False(t, resp.Suspension.BatchHalted)
		assert.Len(t, resp.Suspension.PendingToolCalls, 1)
		assert.True(t, resp.Suspension.PendingToolCalls[0].HaltsBatch)
		return resp.Suspension
	}
	declined := map[string]*ToolResult{"c1": NewToolResultError("the person declined")}
	const haltText = "Not executed: an earlier tool call in this response failed."

	for name, clickTool := range map[string]Tool{
		"suspended tool unregistered": nil,
		"suspended tool unannotated":  &scriptedTool{name: "click"},
	} {
		t.Run("session, "+name, func(t *testing.T) {
			mock := script()
			sess := session.New("halt-pending-" + name)
			suspend(t, mock, sess)

			typ := haltingTool{&scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}}
			tools := []Tool{typ}
			if clickTool != nil {
				tools = append(tools, clickTool)
			}
			resumer, err := NewAgent(AgentOptions{Model: mock, Tools: tools, Session: sess})
			assert.NoError(t, err)
			resp, err := resumer.CreateResponse(context.Background(), WithToolResults(declined))
			assert.NoError(t, err)
			assert.Equal(t, resp.Status, ResponseStatusCompleted)
			assert.Equal(t, typ.CallCount(), 0)
			text, isError := lastToolResultText(mock, "c2")
			assert.Equal(t, text, haltText)
			assert.True(t, isError)
		})
	}

	t.Run("stateless, state round-tripped through JSON", func(t *testing.T) {
		mock := script()
		data, err := json.Marshal(suspend(t, mock, nil))
		assert.NoError(t, err)
		var restored SuspensionState
		assert.NoError(t, json.Unmarshal(data, &restored))

		typ := haltingTool{&scriptedTool{name: "type", outcomes: []toolOutcome{{result: NewToolResultText("typed")}}}}
		resumer, err := NewAgent(AgentOptions{Model: mock, Tools: []Tool{typ}})
		assert.NoError(t, err)
		resp, err := resumer.CreateResponse(context.Background(), WithResume(&restored, declined))
		assert.NoError(t, err)
		assert.Equal(t, resp.Status, ResponseStatusCompleted)
		assert.Equal(t, typ.CallCount(), 0)
		text, _ := lastToolResultText(mock, "c2")
		assert.Equal(t, text, haltText)
	})
}
