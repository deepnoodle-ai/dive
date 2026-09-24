package dive

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// declarerTool stands in for a provider toolset such as
// anthropic.ComputerToolset.
type declarerTool struct {
	countingTool
	declares []string
}

func (t *declarerTool) DeclaredTools() []string { return t.declares }

func newDeclarer() *declarerTool {
	return &declarerTool{countingTool: countingTool{name: "computer"}, declares: []string{"left_click", "type"}}
}

func TestToolDeclarer(t *testing.T) {
	click := &countingTool{name: "left_click"}
	typ := &countingTool{name: "type"}
	search := &countingTool{name: "search"}

	t.Run("declared tools are not sent while the declarer is", func(t *testing.T) {
		clickCall := call("c1", "left_click")
		clickCall.ToolsetName = "computer"
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{clickCall}}}
		agent, err := NewAgent(AgentOptions{
			Model: model,
			Tools: []Tool{click, search, newDeclarer(), typ},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		// Order of the remaining tools is preserved on every request.
		assert.Equal(t, model.tools, [][]string{{"search", "computer"}, {"search", "computer"}})
		// Calls still route to the declared tool by name.
		assert.Equal(t, click.runs.Load(), int32(1))
	})

	t.Run("declared tools are sent as ordinary tools without the declarer", func(t *testing.T) {
		model := &batchLLM{}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{click, search, typ}})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, model.tools, [][]string{{"left_click", "search", "type"}})
	})

	t.Run("a declarer that declares its own name is still sent", func(t *testing.T) {
		model := &batchLLM{}
		self := &declarerTool{countingTool: countingTool{name: "computer"}, declares: []string{"computer", "left_click"}}
		agent, err := NewAgent(AgentOptions{Model: model, Tools: []Tool{self, click, search}})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, model.tools, [][]string{{"computer", "search"}})
	})

	t.Run("a declarer from a dynamic toolset hides static tools", func(t *testing.T) {
		model := &batchLLM{}
		agent, err := NewAgent(AgentOptions{
			Model: model,
			Tools: []Tool{click, search, typ},
			Toolsets: []Toolset{&ToolsetFunc{ToolsetName: "computer-use", Resolve: func(context.Context) ([]Tool, error) {
				return []Tool{newDeclarer()}, nil
			}}},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, model.tools, [][]string{{"search", "computer"}})
	})

	t.Run("dynamic declared tools are hidden by a static declarer", func(t *testing.T) {
		model := &batchLLM{}
		agent, err := NewAgent(AgentOptions{
			Model: model,
			Tools: []Tool{newDeclarer(), search},
			Toolsets: []Toolset{&ToolsetFunc{ToolsetName: "members", Resolve: func(context.Context) ([]Tool, error) {
				return []Tool{click, typ}, nil
			}}},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, model.tools, [][]string{{"computer", "search"}})
	})

	t.Run("a toolset that drops the declarer brings the tools back", func(t *testing.T) {
		model := &batchLLM{batches: [][]*llm.ToolUseContent{{call("c1", "search")}}}
		iteration := 0
		agent, err := NewAgent(AgentOptions{
			Model: model,
			Tools: []Tool{click, search},
			Toolsets: []Toolset{&ToolsetFunc{ToolsetName: "computer-use", Resolve: func(context.Context) ([]Tool, error) {
				iteration++
				if iteration == 1 {
					return []Tool{newDeclarer()}, nil
				}
				return nil, nil
			}}},
		})
		assert.NoError(t, err)

		_, err = agent.CreateResponse(context.Background(), WithInput("go"))
		assert.NoError(t, err)
		assert.Equal(t, model.tools, [][]string{{"search", "computer"}, {"left_click", "search"}})
	})
}
