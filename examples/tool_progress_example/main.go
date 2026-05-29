// tool_progress_example demonstrates structured tool progress streaming.
//
// A "process_dataset" tool simulates a long-running job and emits typed
// progress snapshots with dive.ReportProgress(ctx, *dive.ToolProgress) while it
// runs. The caller observes them through WithEventCallback as
// ResponseItemTypeToolProgress items and renders a live status line.
//
// ReportProgress (structured, latest-wins snapshots) is independent of
// StreamOutput (text deltas) — a tool may use either, both, or neither. This
// example focuses on the structured channel.
//
// Run: cd examples && go run ./tool_progress_example
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/providers/anthropic"
	"github.com/deepnoodle-ai/dive/session"
)

type ProcessInput struct {
	Dataset string `json:"dataset" description:"Name of the dataset to process"`
}

func main() {
	ctx := context.Background()

	// A tool that simulates record-by-record processing. It reports a typed
	// progress snapshot after each record — percent complete, bytes processed,
	// and elapsed time — none of which a text-only stream could convey cleanly.
	processTool := dive.FuncTool("process_dataset",
		"Processes the named dataset record by record, reporting progress as it goes.",
		func(ctx context.Context, in *ProcessInput) (*dive.ToolResult, error) {
			const total = 40
			start := time.Now()
			var bytes int
			for i := 1; i <= total; i++ {
				select {
				case <-time.After(35 * time.Millisecond): // simulate work
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				bytes += 1024 + i*13 // pretend each record is ~1 KB

				// Structured, latest-wins snapshot of in-progress state.
				dive.ReportProgress(ctx, &dive.ToolProgress{
					Display: fmt.Sprintf("processed %d/%d records (%d%%)", i, total, i*100/total),
					Metadata: map[string]any{
						"records_done":    i,
						"records_total":   total,
						"percent":         i * 100 / total,
						"bytes_processed": bytes,
						"elapsed_ms":      time.Since(start).Milliseconds(),
					},
				})
			}
			return dive.NewToolResultText(fmt.Sprintf(
				"Processed %d records from %q (%d bytes) in %s.",
				total, in.Dataset, bytes, time.Since(start).Round(time.Millisecond),
			)), nil
		})

	agent, err := dive.NewAgent(dive.AgentOptions{
		SystemPrompt: "You are a data assistant. Use the process_dataset tool when asked to process a dataset.",
		Model:        anthropic.New(),
		Tools:        []dive.Tool{processTool},
		Session:      session.New("tool-progress-demo"),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Asking agent to process a dataset...")
	fmt.Println()

	resp, err := agent.CreateResponse(ctx,
		dive.WithInput(`Process the dataset named "events-2026" and tell me how many records it had.`),
		dive.WithEventCallback(func(ctx context.Context, item *dive.ResponseItem) error {
			if item.Type == dive.ResponseItemTypeToolProgress && item.ToolProgress != nil {
				p := item.ToolProgress.Progress
				// Each snapshot replaces the previous one — overwrite a single
				// status line rather than scrolling.
				fmt.Printf("\r  ⟳ %-32s bytes=%-7v elapsed_ms=%v   ",
					p.Display, p.Metadata["bytes_processed"], p.Metadata["elapsed_ms"])
			}
			return nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\r%-72s\r", "") // clear the status line
	fmt.Printf("Agent: %s\n", resp.OutputText())
}
