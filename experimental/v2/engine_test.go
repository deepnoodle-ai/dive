package v2

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/deepnoodle-ai/wonton/assert"
)

var ctx = context.Background()

func startCommand(limit int) Command {
	return Command{Start: &Start{TurnID: "turn", Input: "keep this input", ModelCallLimit: limit}}
}

func hostWithInput() *TransactionalHost {
	h := NewTransactionalHost()
	h.Queue("turn", "keep this input")
	return h
}

func scripted(external bool, models, tools *int) Engine {
	return Engine{
		Model: func(_ context.Context, request Request) (ModelResult, error) {
			*models++
			if request.CallNumber == 1 {
				return ModelResult{Model: "served-model", Usage: &Usage{Input: 10, Output: 2}, Call: &Call{Name: "echo", Arguments: `{"value":"original"}`, External: external}}, nil
			}
			return ModelResult{Text: "done", Model: "served-model", Stop: "end", Usage: &Usage{Input: 20, Output: 3}}, nil
		},
		Tools: map[string]Tool{"echo": func(_ context.Context, call ToolInvocation) (ToolResult, error) {
			*tools++
			return ToolResult{State: ToolSucceeded, Text: call.Arguments}, nil
		}},
		Authorize: func(context.Context, Call) error { return nil },
	}
}

type faultRecorder struct {
	store Store
	at    int
	after bool
}

func (f faultRecorder) Commit(ctx context.Context, step Step) error {
	if step.Sequence != f.at {
		return f.store.Commit(ctx, step)
	}
	if f.after {
		if err := f.store.Commit(ctx, step); err != nil {
			return err
		}
		// An untyped connection error must conservatively mean unknown.
		return errors.New("connection lost after commit")
	}
	return reject(errors.New("transaction rejected before commit"))
}

func TestEveryWriteBoundaryStopsEffectsAndPreservesEvidence(t *testing.T) {
	for _, after := range []bool{false, true} {
		for at := 1; at <= 8; at++ {
			t.Run(fmt.Sprintf("step_%d/after_%t", at, after), func(t *testing.T) {
				h := hostWithInput()
				models, tools := 0, 0
				e := scripted(false, &models, &tools)
				out, err := e.Execute(ctx, nil, startCommand(2), faultRecorder{store: h, at: at, after: after})
				assert.NoError(t, err)
				assert.Equal(t, out.Reason, "recording_failed")
				assert.Equal(t, len(out.Records), at-1)
				assert.Equal(t, out.Unacknowledged.Sequence, at)
				wantDisposition := Rejected
				persisted := at - 1
				if after {
					wantDisposition = Unknown
					persisted++
				}
				assert.Equal(t, out.WriteError.Disposition, wantDisposition)
				assert.Equal(t, models, []int{0, 0, 1, 1, 1, 1, 2, 2}[at-1])
				assert.Equal(t, tools, []int{0, 0, 0, 0, 1, 1, 1, 1}[at-1])
				loaded, err := h.Load(ctx, "turn")
				assert.NoError(t, err)
				assert.Equal(t, len(loaded), persisted)
				usage, unknown, inbox := h.Snapshot()
				wantUsage := Usage{}
				if persisted >= 3 {
					wantUsage = Usage{Input: 10, Output: 2}
				}
				if persisted >= 7 {
					wantUsage = Usage{Input: 30, Output: 5}
				}
				assert.Equal(t, usage, wantUsage)
				assert.Equal(t, unknown, 0)
				_, queued := inbox["turn"]
				assert.Equal(t, queued, persisted == 0)
				beforeModels, beforeTools := models, tools
				command := Command{}
				if len(loaded) == 0 {
					command = startCommand(2)
				}
				resumed, err := e.Execute(ctx, loaded, command, h)
				assert.NoError(t, err)
				if len(loaded) > 0 && (loaded[len(loaded)-1].Kind == ModelStarted || loaded[len(loaded)-1].Kind == ToolStarted) {
					assert.Equal(t, resumed.Reason, "reconcile")
					assert.Equal(t, models, beforeModels)
					assert.Equal(t, tools, beforeTools)
					// An observed result from a rejected write can be explicitly
					// accepted without repeating its effect.
					lost := out.Unacknowledged
					if !after && (lost.Kind == ModelFinished || lost.Kind == ToolFinished) {
						resolution := &Resolution{CommandID: "resolution-1", EffectID: lost.EffectID, Model: lost.Model, Tool: lost.Tool}
						resumed, err = e.Execute(ctx, loaded, Command{Resolution: resolution}, h)
						assert.NoError(t, err)
						assert.Equal(t, resumed.Reason, "completed")
					}
				} else {
					assert.Equal(t, resumed.Reason, "completed")
				}
				assert.True(t, models <= 2)
				assert.True(t, tools <= 1)
			})
		}
	}
}

func TestFileSessionReopenExternalResumeAndTurnLimit(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			dir := t.TempDir()
			store, err := NewFileJournal(dir)
			assert.NoError(t, err)
			models, tools := 0, 0
			e := scripted(true, &models, &tools)
			model := e.Model
			e.Model = func(c context.Context, request Request) (ModelResult, error) {
				assert.Equal(t, request.Input, "keep this input")
				return model(c, request)
			}
			runner := &SessionRunner{Store: store, Engine: e}
			out, err := runner.Run(ctx, "turn", startCommand(limit))
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, "waiting")
			assert.Equal(t, models, 1)
			assert.Equal(t, tools, 0)
			store, err = NewFileJournal(dir)
			assert.NoError(t, err)
			runner = &SessionRunner{Store: store, Engine: e}
			out, err = runner.Run(ctx, "turn", Command{})
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, "waiting")
			assert.Equal(t, models, 1)
			_, err = runner.Run(ctx, "turn", Command{Resolution: &Resolution{CommandID: "resolution-1", EffectID: "wrong", Tool: &ToolResult{State: ToolSucceeded, Text: "result"}}})
			assert.Error(t, err)
			out, err = runner.Run(ctx, "turn", Command{Resolution: &Resolution{CommandID: "resolution-1", EffectID: "turn/model/1/tool", Tool: &ToolResult{State: ToolSucceeded, Text: "external result"}}})
			assert.NoError(t, err)
			want := "limited"
			if limit == 2 {
				want = "completed"
			}
			assert.Equal(t, out.Reason, want)
			assert.Equal(t, models, limit)
			assert.Equal(t, tools, 0)
			out, err = runner.Run(ctx, "turn", Command{})
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, want)
			assert.Equal(t, models, limit)
		})
	}
}

func TestFinalArgumentsAndObserverOwnership(t *testing.T) {
	models, tools := 0, 0
	e := scripted(false, &models, &tools)
	var approved, executed, observed string
	e.Transform = func(_ context.Context, call Call) (Call, error) {
		call.Arguments = `{"value":"effective"}`
		return call, nil
	}
	e.Authorize = func(_ context.Context, call Call) error { approved = call.Arguments; return nil }
	e.Tools["echo"] = func(_ context.Context, call ToolInvocation) (ToolResult, error) {
		executed = call.Arguments
		return ToolResult{State: ToolSucceeded, Text: "result"}, nil
	}
	e.Observe = func(step Step) {
		if step.Start != nil {
			step.Start.Input = "observer modified input"
		}
		if step.Model != nil {
			step.Model.Text = "observer modified answer"
			step.Model.Usage.Input = 999
		}
		if step.Call != nil {
			observed = step.Call.Arguments
			step.Call.Arguments = "observer modified arguments"
		}
	}
	model := e.Model
	e.Model = func(c context.Context, request Request) (ModelResult, error) {
		assert.Equal(t, request.Input, "keep this input")
		request.History[0].Start.Input = "provider modified history"
		return model(c, request)
	}
	h := hostWithInput()
	out, err := e.Execute(ctx, nil, startCommand(2), h)
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "completed")
	assert.Equal(t, approved, `{"value":"effective"}`)
	assert.Equal(t, executed, approved)
	assert.Equal(t, observed, approved)
	assert.Equal(t, out.Records[3].Call.Arguments, approved)
	assert.Equal(t, out.Records[6].Model.Text, "done")
	assert.Equal(t, out.Records[0].Start.Input, "keep this input")
	usage, unknown, _ := h.Snapshot()
	assert.Equal(t, usage, Usage{Input: 30, Output: 5})
	assert.Equal(t, unknown, 0)
	out.Records[0].Start.Input = "caller modified result"
	loaded, err := h.Load(ctx, "turn")
	assert.NoError(t, err)
	assert.Equal(t, loaded[0].Start.Input, "keep this input")
}

func TestTransactionRollbackAndIdempotentAccounting(t *testing.T) {
	h := hostWithInput()
	h.FailNextCommit()
	models, tools := 0, 0
	e := scripted(false, &models, &tools)
	out, err := e.Execute(ctx, nil, startCommand(2), h)
	assert.NoError(t, err)
	assert.Equal(t, out.WriteError.Disposition, Rejected)
	usage, unknown, inbox := h.Snapshot()
	assert.Equal(t, inbox["turn"], "keep this input")
	assert.Equal(t, usage, Usage{})
	assert.Equal(t, unknown, 0)
	assert.Equal(t, models, 0)
	out, err = e.Execute(ctx, nil, startCommand(2), h)
	assert.NoError(t, err)
	for _, step := range out.Records {
		assert.NoError(t, h.Commit(ctx, step))
	}
	usage, unknown, inbox = h.Snapshot()
	assert.Equal(t, len(inbox), 0)
	assert.Equal(t, usage, Usage{Input: 30, Output: 5})
	assert.Equal(t, unknown, 0)
	altered := copyValue(out.Records[2])
	altered.Model.Usage.Input = 999
	assert.Error(t, h.Commit(ctx, altered))
	usage, _, _ = h.Snapshot()
	assert.Equal(t, usage, Usage{Input: 30, Output: 5})
}

func TestEmptyRefusalErrorsAndUnknownUsageAreRecorded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result ModelResult
		err    error
		reason string
	}{
		{"empty", ModelResult{}, nil, "empty"},
		{"refusal", ModelResult{Stop: "refusal", Usage: &Usage{}}, nil, "refused"},
		{"truncated", ModelResult{Text: "partial", Stop: "length"}, nil, "truncated"},
		{"partial_error", ModelResult{Text: "partial"}, errors.New("stream disconnected"), "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := hostWithInput()
			e := Engine{Model: func(context.Context, Request) (ModelResult, error) { return tc.result, tc.err }}
			out, err := e.Execute(ctx, nil, startCommand(1), h)
			assert.NoError(t, err)
			assert.Equal(t, out.Reason, tc.reason)
			assert.Equal(t, len(out.Records), 4)
			assert.Equal(t, out.Records[2].Model.Text, tc.result.Text)
			assert.Equal(t, out.Records[2].Model.Usage, tc.result.Usage)
			_, unknown, _ := h.Snapshot()
			want := 0
			if tc.result.Usage == nil {
				want = 1
			}
			assert.Equal(t, unknown, want)
		})
	}
}

func TestToolInfrastructureFailureStopsTurn(t *testing.T) {
	models, tools := 0, 0
	e := scripted(false, &models, &tools)
	e.Tools["echo"] = func(context.Context, ToolInvocation) (ToolResult, error) {
		return ToolResult{State: ToolNotExecuted, Error: "worker unavailable"}, nil
	}
	out, err := e.Execute(ctx, nil, startCommand(2), hostWithInput())
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "not_executed")
	assert.Equal(t, models, 1)
	assert.Equal(t, out.Records[4].Tool.Error, "worker unavailable")
}

func TestCancellationStillRecordsObservedResult(t *testing.T) {
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	e := Engine{Model: func(context.Context, Request) (ModelResult, error) {
		cancel()
		return ModelResult{Text: "partial"}, context.Canceled
	}}
	out, err := e.Execute(cancelCtx, nil, startCommand(1), hostWithInput())
	assert.NoError(t, err)
	assert.Equal(t, out.Reason, "failed")
	assert.Equal(t, out.Records[2].Model.Text, "partial")
	assert.Equal(t, out.Records[2].Model.Error, "context canceled")
}

func TestNoImplicitAuthorizationOrInvalidReplay(t *testing.T) {
	models, tools := 0, 0
	e := scripted(false, &models, &tools)
	e.Authorize = nil
	out, err := e.Execute(ctx, nil, startCommand(2), hostWithInput())
	assert.ErrorContains(t, err, "authorization must be explicit")
	assert.Equal(t, tools, 0)
	assert.Equal(t, len(out.Records), 3)
	bad := copyValue(out.Records)
	bad[1].Sequence = 40
	_, err = e.Execute(ctx, bad, Command{}, hostWithInput())
	assert.Error(t, err)
	assert.Equal(t, models, 1)
}

func TestFileJournalDeduplicatesAndRejectsConflicts(t *testing.T) {
	store, err := NewFileJournal(t.TempDir())
	assert.NoError(t, err)
	models, tools := 0, 0
	e := scripted(false, &models, &tools)
	out, err := e.Execute(ctx, nil, startCommand(2), store)
	assert.NoError(t, err)
	for _, step := range out.Records {
		assert.NoError(t, store.Commit(ctx, step))
	}
	conflict := copyValue(out.Records[4])
	conflict.Tool.Text = "conflicting result"
	assert.Error(t, store.Commit(ctx, conflict))
	loaded, err := store.Load(ctx, "turn")
	assert.NoError(t, err)
	assert.Equal(t, loaded, out.Records)
}
