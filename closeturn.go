package dive

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/deepnoodle-ai/dive/llm"
)

// ReminderNameTurnIncomplete is the name of the reminder that closes an
// incomplete turn. Its Details hold the TurnOutcome (see FindTurnOutcome).
const ReminderNameTurnIncomplete = "turn-incomplete"

// ReminderNameTurnContinue is the name of the model-only reminder
// WithContinue adds, saying the user asked to continue.
const ReminderNameTurnContinue = "turn-continue"

// toolCallsNote ends the outcome reminder of a turn that answered calls it
// did not complete.
const toolCallsNote = `A tool result that begins "Not run:" is a call that never started and had no effect. A tool result that begins "Unknown result:" is a call that was running when the turn ended; whether it took effect is unknown, so check before repeating it.`

// turnContinueText is the content of the turn-continue reminder.
const turnContinueText = "The user asked to continue from where the previous turn stopped. Continue the work without repeating what was already done."

// NewTurnOutcomeReminder builds the reminder recorded at the end of an
// incomplete turn: a contextual reminder named ReminderNameTurnIncomplete
// whose Details hold the outcome. Applications match the name and read the
// details with FindTurnOutcome; the Content is Dive's wording for the
// reason, written for the model, and may change between releases.
func NewTurnOutcomeReminder(outcome *TurnOutcome) Reminder {
	if outcome == nil {
		outcome = &TurnOutcome{Reason: TurnReasonError, Next: TurnNextInput}
	}
	return Reminder{
		Name:    ReminderNameTurnIncomplete,
		Tier:    ReminderTierContextual,
		Content: turnOutcomeText(outcome),
		Details: turnOutcomeDetails(outcome),
	}
}

// FindTurnOutcome returns the outcome recorded in a message by the
// turn-incomplete reminder, if it has one.
func FindTurnOutcome(message *llm.Message) (*TurnOutcome, bool) {
	r, ok := FindReminder(message, ReminderNameTurnIncomplete)
	if !ok || len(r.Details) == 0 {
		return nil, false
	}
	data, err := json.Marshal(r.Details)
	if err != nil {
		return nil, false
	}
	var outcome TurnOutcome
	if err := json.Unmarshal(data, &outcome); err != nil || outcome.Reason == "" {
		return nil, false
	}
	return &outcome, true
}

// FindLatestTurnOutcome returns the most recent outcome recorded in
// messages, searching from newest to oldest.
func FindLatestTurnOutcome(messages []*llm.Message) (*TurnOutcome, bool) {
	for i := len(messages) - 1; i >= 0; i-- {
		if outcome, ok := FindTurnOutcome(messages[i]); ok {
			return outcome, true
		}
	}
	return nil, false
}

// CloseTurn returns turn with every client tool call that has no result
// answered, and the outcome recorded at the end as the turn-incomplete
// reminder, so the turn can be sent to a provider again. A call the outcome
// lists as unknown is answered with ToolCallUnknownText; any other
// unanswered call with ToolCallNotRunText, and the recorded outcome's
// ToolCalls gains a not_started record for it. The recorded outcome is read
// back with FindTurnOutcome. CloseTurn modifies neither turn nor outcome.
//
// The agent closes every incomplete turn this way before saving it; an
// application that keeps turns itself can apply it to messages it holds.
func CloseTurn(turn []*llm.Message, outcome *TurnOutcome) []*llm.Message {
	if outcome == nil {
		outcome = &TurnOutcome{Reason: TurnReasonError, Next: TurnNextInput}
	}
	recorded := *outcome
	closed, answered := answerOpenToolCalls(turn, recorded.ToolCalls)
	recorded.ToolCalls = withNotStartedRecords(recorded.ToolCalls, answered)
	return append(closed, NewReminderMessage(NewTurnOutcomeReminder(&recorded)))
}

// answerOpenToolCalls answers every client tool call in messages that has no
// result: with ToolCallUnknownText when records list it as unknown, else
// with ToolCallNotRunText. It returns the messages, copied where changed,
// and the calls it answered, in order.
func answerOpenToolCalls(messages []*llm.Message, records []ToolCallRecord) ([]*llm.Message, []*llm.ToolUseContent) {
	unknown := map[string]bool{}
	for _, record := range records {
		if record.State == ToolCallStateUnknown {
			unknown[record.ID] = true
		}
	}
	var answered []*llm.ToolUseContent
	closed := llm.AnswerUnansweredToolCallsWith(messages, func(call *llm.ToolUseContent) *llm.ToolResultContent {
		answered = append(answered, call)
		text := ToolCallNotRunText
		if unknown[call.ID] {
			text = ToolCallUnknownText
		}
		return &llm.ToolResultContent{
			ToolUseID:   call.ID,
			ToolsetName: call.ToolsetName,
			Content:     text,
			IsError:     true,
		}
	})
	return slices.Clone(closed), answered
}

// withNotStartedRecords returns records with a not_started record added for
// each answered call it does not list.
func withNotStartedRecords(records []ToolCallRecord, answered []*llm.ToolUseContent) []ToolCallRecord {
	listed := map[string]bool{}
	for _, record := range records {
		listed[record.ID] = true
	}
	out := slices.Clone(records)
	for _, call := range answered {
		if !listed[call.ID] {
			out = append(out, ToolCallRecord{ID: call.ID, Name: call.Name, State: ToolCallStateNotStarted})
		}
	}
	return out
}

// turnOutcomeDetails encodes an outcome as reminder details.
func turnOutcomeDetails(outcome *TurnOutcome) map[string]any {
	data, err := json.Marshal(outcome)
	if err != nil {
		return nil
	}
	var details map[string]any
	if err := json.Unmarshal(data, &details); err != nil {
		return nil
	}
	return details
}

// turnOutcomeText is the wording of the outcome reminder for the model.
func turnOutcomeText(o *TurnOutcome) string {
	const asShown = "Everything above this note, including every tool result, happened as shown."
	var text string
	switch o.Reason {
	case TurnReasonCanceled:
		text = "The previous turn was stopped by the user before it finished. " + asShown +
			" Do not repeat completed steps or assume unfinished ones happened. Wait for the user's next instruction rather than resuming the stopped work on your own."
	case TurnReasonHookAbort:
		text = fmt.Sprintf("The previous turn was stopped by the application before it finished: %s. Everything above this note happened as shown. Do not retry the stopped step unless the user asks.", o.Error)
	case TurnReasonOutputLimit:
		text = "The previous response was cut off at the output limit before it finished. Continue from exactly where it stopped, without repeating what was already written."
	case TurnReasonContextLimit:
		text = "The previous response stopped because the conversation reached the model's context window. Everything above this note happened as shown. Continue from where it stopped, without repeating what was already written."
	case TurnReasonIterationLimit:
		text = "The previous turn reached its limit of tool calls before finishing. Finish with the information already gathered, or ask the user before continuing."
	case TurnReasonProviderStopped:
		text = fmt.Sprintf("The previous turn ended because the provider stopped the response before it finished (reason: %s). Any tool call it made was not run. Everything above this note happened as shown. Continue from the completed work; if the same stop repeats, tell the user rather than retrying.", o.Error)
	case TurnReasonPause:
		text = "The previous turn's server tool loop was paused before it finished. Its trailing call was not completed; start it again if the user still wants it."
	default:
		errText := o.Error
		if o.Reason == TurnReasonDeadline {
			errText = "the turn's time limit was reached"
		}
		text = fmt.Sprintf("The previous turn failed before it finished. Error: %s. %s Continue from the completed work: do not repeat steps that completed, and do not assume steps that did not complete have happened.", errText, asShown)
	}
	for _, record := range o.ToolCalls {
		if record.State != ToolCallStateCompleted {
			return text + " " + toolCallsNote
		}
	}
	return text
}
