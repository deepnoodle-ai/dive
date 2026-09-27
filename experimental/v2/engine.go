// Package v2 is a disposable execution-protocol prototype, not a supported API.
// It deliberately supports only text and one tool call per model result.
package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

type Kind string

const (
	Started       Kind = "started"
	ModelStarted  Kind = "model_started"
	ModelFinished Kind = "model_finished"
	ToolStarted   Kind = "tool_started"
	ToolFinished  Kind = "tool_finished"
	ToolUncertain Kind = "tool_uncertain"
	Finished      Kind = "finished"
)

// Step identity is (TurnID, Sequence); a repeated identity must have identical
// content. EffectID survives process/invocation boundaries and identifies an
// attempt, not evidence that the effect actually began.
type Step struct {
	Version   int          `json:"version"`
	TurnID    string       `json:"turn_id"`
	Sequence  int          `json:"sequence"`
	Kind      Kind         `json:"kind"`
	EffectID  string       `json:"effect_id,omitempty"`
	CommandID string       `json:"command_id,omitempty"`
	Start     *Start       `json:"start,omitempty"`
	Call      *Call        `json:"call,omitempty"`
	Model     *ModelResult `json:"model,omitempty"`
	Tool      *ToolResult  `json:"tool,omitempty"`
	Status    string       `json:"status,omitempty"`
}

type Start struct {
	TurnID         string `json:"turn_id"`
	Input          string `json:"input"`
	ModelCallLimit int    `json:"model_call_limit"`
}

type Call struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	External  bool   `json:"external,omitempty"`
}

// Nil Usage means unknown. An allocated zero Usage means known zero.
type Usage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

type ModelResult struct {
	Text  string `json:"text"`
	Model string `json:"model"`
	Stop  string `json:"stop"`
	Usage *Usage `json:"usage,omitempty"`
	Call  *Call  `json:"call,omitempty"`
	Error string `json:"error,omitempty"`
}

type ToolState string

const (
	ToolSucceeded   ToolState = "succeeded"
	ToolFailed      ToolState = "failed"
	ToolNotExecuted ToolState = "not_executed"
	ToolUnknown     ToolState = "unknown"
)

type ToolResult struct {
	Text  string    `json:"text"`
	State ToolState `json:"state"`
	// Error is diagnostic evidence, not proof of nonexecution. A Go error from
	// Tool always means unknown execution. Definite failures/nonexecution must
	// be explicitly returned as a ToolResult with nil Go error.
	Error string `json:"error,omitempty"`
}

type Resolution struct {
	CommandID string
	EffectID  string
	Model     *ModelResult
	Tool      *ToolResult
}

// An empty command continues a loaded turn. Resolution explicitly accepts a
// reconciled result; it never authorizes retry of an uncertain effect.
type Command struct {
	Start      *Start
	Resolution *Resolution
}

type Disposition string

const (
	Rejected Disposition = "rejected" // The recorder guarantees no write occurred.
	Unknown  Disposition = "unknown"  // The write may have committed; reload first.
)

type CommitError struct {
	Disposition Disposition
	Cause       error
}

func (e *CommitError) Error() string { return string(e.Disposition) + ": " + e.Cause.Error() }
func (e *CommitError) Unwrap() error { return e.Cause }

// Recorder acknowledges a complete step before the engine can advance.
// Implementations own serialization/fencing. Unknown errors are ambiguous by
// default. Recording alone cannot guarantee exactly-once external effects.
type Recorder interface {
	Commit(context.Context, Step) error
}

type Request struct {
	Input      string
	CallID     string
	CallNumber int
	History    []Step
}

type Model func(context.Context, Request) (ModelResult, error)
type ToolInvocation struct {
	EffectID string
	Call
}

type Tool func(context.Context, ToolInvocation) (ToolResult, error)

type Engine struct {
	Model     Model
	Tools     map[string]Tool
	Transform func(context.Context, Call) (Call, error)
	Authorize func(context.Context, Call) error
	// Observe cannot return execution control. Its values are isolated copies;
	// this synchronous spike does not isolate a slow or panicking observer.
	Observe func(Step)
}

type Outcome struct {
	Reason    string
	Duplicate bool
	// Records is the last acknowledged state, even on a failed terminal write.
	Records []Step
	// Unacknowledged retains an observed result or intended transition whose
	// persistence failed. It is never silently presented as durable history.
	Unacknowledged *Step
	WriteError     *CommitError
}

type state struct {
	start        *Start
	calls        int
	pendingModel string
	pendingTool  string
	effective    *Call
	next         *Call
	nextID       string
	last         *ModelResult
	toolFailure  string
	uncertain    bool
	terminal     string
}

func copyValue[T any](value T) T {
	// All wire types in this text-only experiment are JSON-safe concrete types.
	b, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var result T
	if err := json.Unmarshal(b, &result); err != nil {
		panic(err)
	}
	return result
}

func replay(records []Step) (state, error) {
	var s state
	commands := make(map[string]bool)
	for i, record := range records {
		if record.Version != 1 || record.Sequence != i+1 || record.TurnID == "" {
			return s, fmt.Errorf("invalid identity/version at step %d", i+1)
		}
		if s.terminal != "" || (s.start != nil && record.TurnID != s.start.TurnID) {
			return s, fmt.Errorf("step %d follows terminal state or changes turn", i+1)
		}
		if i == 0 && record.Kind != Started {
			return s, errors.New("first step must start a turn")
		}
		if record.CommandID != "" {
			if commands[record.CommandID] || (record.Kind != ModelFinished && record.Kind != ToolFinished && record.Kind != ToolUncertain) {
				return s, errors.New("invalid or repeated command identity")
			}
			commands[record.CommandID] = true
		}
		valid := false
		switch record.Kind {
		case Started:
			valid = i == 0 && record.Start != nil && record.Start.TurnID == record.TurnID && record.Start.ModelCallLimit > 0
			if valid {
				s.start = record.Start
			}
		case ModelStarted:
			valid = s.pendingModel == "" && s.pendingTool == "" && s.next == nil && s.last == nil && s.toolFailure == "" && s.calls < s.start.ModelCallLimit && record.EffectID == fmt.Sprintf("%s/model/%d", s.start.TurnID, s.calls+1)
			if valid {
				s.calls++
				s.pendingModel = record.EffectID
			}
		case ModelFinished:
			valid = s.pendingModel != "" && record.EffectID == s.pendingModel && record.Model != nil
			if valid {
				s.pendingModel = ""
				s.last = record.Model
				if record.Model.Error == "" {
					s.next = record.Model.Call
					s.nextID = record.EffectID + "/tool"
				}
			}
		case ToolStarted:
			valid = s.next != nil && s.pendingTool == "" && record.EffectID == s.nextID && record.Call != nil && record.Call.Name == s.next.Name && record.Call.External == s.next.External && json.Valid([]byte(record.Call.Arguments))
			if valid {
				s.pendingTool = record.EffectID
				s.effective = record.Call
			}
		case ToolFinished:
			valid = s.pendingTool != "" && record.EffectID == s.pendingTool && record.Tool != nil && (record.Tool.State == ToolSucceeded || record.Tool.State == ToolFailed || record.Tool.State == ToolNotExecuted)
			if valid {
				s.pendingTool = ""
				s.next = nil
				s.last = nil
				s.uncertain = false
				if record.Tool.State != ToolSucceeded {
					s.toolFailure = string(record.Tool.State)
				}
			}
		case ToolUncertain:
			valid = s.pendingTool != "" && record.EffectID == s.pendingTool && record.Tool != nil && record.Tool.State == ToolUnknown
			if valid {
				s.uncertain = true
			}
		case Finished:
			valid = s.pendingModel == "" && s.pendingTool == "" && s.next == nil && record.Status != "" && record.Status == terminalReason(s)
			if valid {
				s.terminal = record.Status
			}
		}
		if !valid {
			return s, fmt.Errorf("invalid %s transition at step %d", record.Kind, i+1)
		}
	}
	return s, nil
}

func terminalReason(s state) string {
	if s.toolFailure != "" {
		return s.toolFailure
	}
	if s.last != nil && s.last.Error != "" {
		return "failed"
	}
	if s.last != nil && s.next == nil {
		switch s.last.Stop {
		case "refusal":
			return "refused"
		case "length":
			return "truncated"
		}
		if s.last.Text == "" {
			return "empty"
		}
		return "completed"
	}
	if s.start != nil && s.calls >= s.start.ModelCallLimit && s.next == nil {
		return "limited"
	}
	return ""
}

// Execute never invokes an effect reconstructed in the pending state. A caller
// must reload after ambiguous writes and explicitly reconcile pending effects.
func (e Engine) Execute(ctx context.Context, records []Step, command Command, recorder Recorder) (Outcome, error) {
	out := Outcome{Records: copyValue(records)}
	s, err := replay(out.Records)
	if err != nil {
		return out, err
	}
	if recorder == nil || (command.Start != nil && (len(records) != 0 || command.Resolution != nil)) || (len(records) == 0 && command.Start == nil) {
		return out, errors.New("invalid command or missing recorder")
	}
	commit := func(step Step) bool {
		step.Version, step.Sequence = 1, len(out.Records)+1
		if s.start != nil {
			step.TurnID = s.start.TurnID
		} else if step.Start != nil {
			step.TurnID = step.Start.TurnID
		}
		candidate := append(copyValue(out.Records), copyValue(step))
		next, validationErr := replay(candidate)
		if validationErr != nil {
			err = validationErr
			return false
		}
		// Preserve an observed result after caller cancellation, with a bounded
		// write attempt. Fencing/ownership remains the recorder's responsibility.
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if writeErr := recorder.Commit(writeCtx, copyValue(step)); writeErr != nil {
			fault := &CommitError{Disposition: Unknown, Cause: writeErr}
			var declared *CommitError
			if errors.As(writeErr, &declared) && declared.Disposition == Rejected {
				fault.Disposition = Rejected
			}
			out.Reason, out.Unacknowledged, out.WriteError = "recording_failed", &step, fault
			return false
		}
		out.Records, s = candidate, next
		if e.Observe != nil {
			e.Observe(copyValue(step))
		}
		return true
	}
	if command.Start != nil && !commit(Step{Kind: Started, Start: copyValue(command.Start)}) {
		return out, err
	}
	if resolution := command.Resolution; resolution != nil {
		if resolution.CommandID == "" || (resolution.Model == nil) == (resolution.Tool == nil) {
			return out, errors.New("resolution requires a command identity and exactly one result")
		}
		for _, prior := range out.Records {
			if prior.CommandID != resolution.CommandID {
				continue
			}
			if prior.EffectID != resolution.EffectID || !reflect.DeepEqual(prior.Model, resolution.Model) || !reflect.DeepEqual(prior.Tool, resolution.Tool) {
				return out, errors.New("command identity reused with different content")
			}
			// A redelivery acknowledges the command only. It never implicitly
			// advances execution, even if a later effect is pending.
			out.Reason, out.Duplicate = "duplicate", true
			return out, nil
		}
		step := Step{EffectID: resolution.EffectID, CommandID: resolution.CommandID}
		switch {
		case s.pendingModel != "" && resolution.EffectID == s.pendingModel && resolution.Model != nil && resolution.Tool == nil:
			step.Kind, step.Model = ModelFinished, copyValue(resolution.Model)
		case s.pendingTool != "" && resolution.EffectID == s.pendingTool && resolution.Tool != nil && resolution.Model == nil:
			step.Kind, step.Tool = ToolFinished, copyValue(resolution.Tool)
			if resolution.Tool.State == ToolUnknown {
				step.Kind = ToolUncertain
			}
		default:
			return out, errors.New("resolution does not match an outstanding effect")
		}
		if !commit(step) {
			return out, err
		}
	}
	for {
		if s.terminal != "" {
			out.Reason = s.terminal
			return out, nil
		}
		if s.pendingModel != "" || s.pendingTool != "" {
			out.Reason = "reconcile"
			if s.pendingTool != "" && s.effective.External && !s.uncertain {
				out.Reason = "waiting"
			}
			return out, nil
		}
		if reason := terminalReason(s); reason != "" {
			if !commit(Step{Kind: Finished, Status: reason}) {
				return out, err
			}
			continue
		}
		if ctx.Err() != nil {
			out.Reason = "cancelled"
			return out, nil
		}
		if s.next != nil {
			call := *s.next
			if e.Transform != nil {
				call, err = e.Transform(ctx, call)
				if err != nil {
					return out, err
				}
			}
			if call.Name != s.next.Name || call.External != s.next.External || !json.Valid([]byte(call.Arguments)) {
				return out, errors.New("transform may only change valid JSON arguments")
			}
			tool := e.Tools[call.Name]
			if !call.External && tool == nil {
				return out, fmt.Errorf("tool %q unavailable", call.Name)
			}
			if e.Authorize == nil {
				return out, errors.New("tool authorization must be explicit")
			}
			if err := e.Authorize(ctx, call); err != nil {
				return out, err
			}
			if !commit(Step{Kind: ToolStarted, EffectID: s.nextID, Call: &call}) {
				return out, err
			}
			if call.External {
				out.Reason = "waiting"
				return out, nil
			}
			if ctx.Err() != nil {
				out.Reason = "reconcile"
				return out, nil
			}
			result, callErr := tool(ctx, ToolInvocation{EffectID: s.pendingTool, Call: call})
			if callErr != nil {
				result.State = ToolUnknown
				result.Error = callErr.Error()
			}
			// Malformed tool results cannot establish a definite outcome either.
			if result.State != ToolSucceeded && result.State != ToolFailed && result.State != ToolNotExecuted && result.State != ToolUnknown {
				result.State = ToolUnknown
				result.Error = "tool returned an invalid execution state: " + result.Error
			}
			kind := ToolFinished
			if result.State == ToolUnknown {
				kind = ToolUncertain
			}
			if !commit(Step{Kind: kind, EffectID: s.pendingTool, Tool: &result}) {
				return out, err
			}
			continue
		}
		if e.Model == nil {
			return out, errors.New("model unavailable")
		}
		id := fmt.Sprintf("%s/model/%d", s.start.TurnID, s.calls+1)
		if !commit(Step{Kind: ModelStarted, EffectID: id}) {
			return out, err
		}
		if ctx.Err() != nil {
			out.Reason = "reconcile"
			return out, nil
		}
		result, callErr := e.Model(ctx, Request{Input: s.start.Input, CallID: id, CallNumber: s.calls, History: copyValue(out.Records)})
		if callErr != nil {
			result.Error = callErr.Error()
		}
		if !commit(Step{Kind: ModelFinished, EffectID: id, Model: &result}) {
			return out, err
		}
	}
}
