package compaction

import (
	"context"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
)

// HookWithModel returns a PreGenerationHook that triggers context
// compaction using the provided LLM model when the estimated token count
// exceeds the given threshold.
//
// This hook uses the CompactMessages function to generate summaries. The
// compaction event is stored in state.Values[dive.StateKeyCompactionEvent] for access
// by PostGeneration hooks.
//
// Parameters:
//   - model: LLM to use for generating summaries
//   - tokenThreshold: Token count threshold to trigger compaction (default: 100000)
//   - systemPrompt: System prompt to include in summary requests (can be empty)
//
// Example:
//
//	agent, _ := dive.NewAgent(dive.AgentOptions{
//	    SystemPrompt: "You are a helpful assistant.",
//	    Model:        mainModel,
//	    PreGeneration: []dive.PreGenerationHook{
//	        compaction.HookWithModel(summaryModel, 80000, ""),
//	    },
//	})
func HookWithModel(model llm.LLM, tokenThreshold int, systemPrompt string) dive.PreGenerationHook {
	if tokenThreshold <= 0 {
		tokenThreshold = DefaultContextTokenThreshold
	}
	return func(ctx context.Context, state *dive.GenerationState) error {
		// Rough token estimation: ~4 chars per token
		estimatedTokens := 0
		for _, msg := range state.Messages {
			estimatedTokens += len(msg.Text()) / 4
		}

		if estimatedTokens < tokenThreshold {
			return nil
		}

		compacted, event, err := CompactMessages(ctx, model, state.Messages, systemPrompt, "", estimatedTokens)
		if err != nil {
			return err
		}

		state.Messages = compacted
		state.Values[dive.StateKeyCompactionEvent] = event
		return nil
	}
}
