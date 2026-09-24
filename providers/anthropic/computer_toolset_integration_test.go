//go:build integration

package anthropic

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// toolRecorder records the tools declared on each streamed request.
type toolRecorder struct {
	*Provider
	tools [][]string
}

func (r *toolRecorder) Stream(ctx context.Context, opts ...llm.Option) (llm.StreamIterator, error) {
	var cfg llm.Config
	cfg.Apply(opts...)
	var names []string
	for _, t := range cfg.Tools {
		names = append(names, t.Name())
	}
	r.tools = append(r.tools, names)
	return r.Provider.Stream(ctx, opts...)
}

// The API accepts a request declaring the computer toolset with the member
// tools left out, and accepts a failed action followed by the halt text for
// the rest of the batch.
func TestIntegration_ComputerToolsetInAgent(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var clicks, typed atomic.Int32
	member := func(name string, run func() *dive.ToolResult) dive.Tool {
		return dive.FuncTool(name, name, func(ctx context.Context, in map[string]any) (*dive.ToolResult, error) {
			return run(), nil
		})
	}
	model := &toolRecorder{Provider: New(WithModel(ModelClaudeSonnet5), WithMaxTokens(1024))}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model:              model,
		ToolIterationLimit: 1,
		Tools: []dive.Tool{
			NewComputerToolset(ComputerToolsetOptions{}),
			member("left_click", func() *dive.ToolResult {
				clicks.Add(1)
				return dive.NewToolResultError("Click failed: the display is locked.")
			}),
			member("type", func() *dive.ToolResult {
				typed.Add(1)
				return dive.NewToolResultText("typed")
			}),
			member("screenshot", func() *dive.ToolResult {
				return dive.NewToolResultError("Screenshots are unavailable.")
			}),
		},
	})
	assert.NoError(t, err)

	resp, err := agent.CreateResponse(ctx, dive.WithInput(
		"In a single response, left click at coordinate (100, 200) and then type hello. "+
			"Do not take a screenshot first."))
	assert.NoError(t, err)
	assert.Equal(t, []string{"computer"}, model.tools[0])
	t.Logf("clicks=%d typed=%d results=%d", clicks.Load(), typed.Load(), len(resp.ToolCallResults()))
	for _, r := range resp.ToolCallResults() {
		t.Logf("%s -> error=%v %q", r.Name, r.Result.IsError, r.Result.Content[0].Text)
	}
	t.Logf("final: %s", resp.OutputText())
	assert.Equal(t, int32(0), typed.Load(), "type must not run after the click failed")
}
