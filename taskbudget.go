package dive

import "github.com/deepnoodle-ai/dive/llm"

// taskBudgetSpend follows the messages the generation loop adds to the
// working history, so that the task budget can be carried across a history
// rewrite.
//
// A task budget (ModelSettings.TaskBudget) covers every model call of one
// turn. While the agent sends the full history, the provider derives what the
// turn has spent from that history, and the agent sends only the total. When a
// PreIteration hook removes messages the turn produced, as mid-turn compaction
// does, that spend is lost, so the agent sends the remaining budget itself: the
// total less the tokens of the removed messages. Messages still in the history
// are left out, since the provider counts those.
//
// It is used only by the generation loop, across the Stop-hook continuations
// of one turn.
type taskBudgetSpend struct {
	// messages are the messages the loop added, in order.
	messages []*llm.Message

	// generated holds the output tokens of each model response, which
	// include thinking that the message may no longer carry. Other messages
	// are estimated.
	generated map[*llm.Message]int
}

// track records a message the loop added to the working history.
func (s *taskBudgetSpend) track(msg *llm.Message) {
	s.messages = append(s.messages, msg)
}

// setGenerated records the output tokens of the model response msg holds.
func (s *taskBudgetSpend) setGenerated(msg *llm.Message, outputTokens int) {
	if s.generated == nil {
		s.generated = map[*llm.Message]int{}
	}
	s.generated[msg] = outputTokens
}

// removed returns the tokens of the tracked messages that are not in working.
// It is computed from scratch on each call, so a history that is sent whole
// again, as a Stop-hook continuation does, counts nothing.
func (s *taskBudgetSpend) removed(working []*llm.Message) int {
	if len(s.messages) == 0 {
		return 0
	}
	present := make(map[*llm.Message]bool, len(working))
	for _, msg := range working {
		present[msg] = true
	}
	tokens := 0
	for _, msg := range s.messages {
		if present[msg] {
			continue
		}
		if generated, ok := s.generated[msg]; ok {
			tokens += generated
		} else {
			tokens += llm.EstimateTokens(msg)
		}
	}
	return tokens
}

// option returns the task budget for the next model call, with Remaining set
// when messages the turn produced are no longer in working. It returns nil
// when nothing was removed, so that the model settings' budget applies as is.
func (s *taskBudgetSpend) option(budget *llm.TaskBudget, working []*llm.Message) llm.Option {
	if budget == nil {
		return nil
	}
	removed := s.removed(working)
	if removed == 0 {
		return nil
	}
	remaining := budget.Total
	if budget.Remaining != nil {
		remaining = *budget.Remaining
	}
	remaining = max(remaining-removed, 0)
	return llm.WithTaskBudget(llm.TaskBudget{Total: budget.Total, Remaining: &remaining})
}
