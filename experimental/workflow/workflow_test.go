package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/experimental/workflow"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

// mockLLM is a minimal LLM that returns a fixed response text.
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Name() string { return "mock" }
func (m *mockLLM) Generate(_ context.Context, _ ...llm.Option) (*llm.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &llm.Response{
		ID:         "r1",
		Model:      "mock",
		Role:       llm.Assistant,
		Content:    []llm.Content{&llm.TextContent{Text: m.response}},
		Type:       "message",
		StopReason: "stop",
	}, nil
}

func makeAgent(t *testing.T, response string) *dive.Agent {
	t.Helper()
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model: &mockLLM{response: response},
	})
	assert.NoError(t, err)
	return agent
}

func TestSequential(t *testing.T) {
	ctx := context.Background()

	t.Run("single agent returns its response", func(t *testing.T) {
		p := workflow.Sequential(makeAgent(t, "step one output"))
		resp, err := p.Run(ctx, dive.WithInput("start"))
		assert.NoError(t, err)
		assert.Equal(t, "step one output", resp.OutputText())
		assert.Equal(t, 1, len(p.Steps()))
	})

	t.Run("final step output is returned", func(t *testing.T) {
		p := workflow.Sequential(
			makeAgent(t, "alpha"),
			makeAgent(t, "beta"),
			makeAgent(t, "gamma"),
		)
		resp, err := p.Run(ctx, dive.WithInput("initial"))
		assert.NoError(t, err)
		assert.Equal(t, "gamma", resp.OutputText())
	})

	t.Run("all steps are recorded", func(t *testing.T) {
		p := workflow.Sequential(
			makeAgent(t, "a"),
			makeAgent(t, "b"),
			makeAgent(t, "c"),
		)
		_, err := p.Run(ctx, dive.WithInput("go"))
		assert.NoError(t, err)
		steps := p.Steps()
		assert.Equal(t, 3, len(steps))
		assert.Equal(t, "a", steps[0].OutputText())
		assert.Equal(t, "b", steps[1].OutputText())
		assert.Equal(t, "c", steps[2].OutputText())
	})

	t.Run("error in middle step stops execution", func(t *testing.T) {
		errAgent, err := dive.NewAgent(dive.AgentOptions{
			Model: &mockLLM{err: errors.New("agent failed")},
		})
		assert.NoError(t, err)

		p := workflow.Sequential(
			makeAgent(t, "first"),
			errAgent,
			makeAgent(t, "third"),
		)
		resp, err := p.Run(ctx, dive.WithInput("start"))
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 1, len(p.Steps()))
	})

	t.Run("steps resets between runs", func(t *testing.T) {
		p := workflow.Sequential(makeAgent(t, "out"))
		_, err := p.Run(ctx, dive.WithInput("first run"))
		assert.NoError(t, err)
		assert.Equal(t, 1, len(p.Steps()))

		_, err = p.Run(ctx, dive.WithInput("second run"))
		assert.NoError(t, err)
		assert.Equal(t, 1, len(p.Steps()))
	})

	t.Run("steps returns a copy", func(t *testing.T) {
		p := workflow.Sequential(makeAgent(t, "out"))
		_, err := p.Run(ctx, dive.WithInput("start"))
		assert.NoError(t, err)
		s1 := p.Steps()
		s2 := p.Steps()
		assert.Equal(t, len(s1), len(s2))
		// Mutating the copy does not affect Pipeline internals
		s1[0] = nil
		assert.NotNil(t, p.Steps()[0])
	})
}

func TestSequentialWithMapper(t *testing.T) {
	ctx := context.Background()

	t.Run("custom mapper receives each step response", func(t *testing.T) {
		var mapperOutputs []string
		mapper := func(_ context.Context, resp *dive.Response) (dive.CreateResponseOption, error) {
			mapperOutputs = append(mapperOutputs, resp.OutputText())
			return dive.WithInput("mapped: " + resp.OutputText()), nil
		}

		p := workflow.SequentialWithMapper(
			[]*dive.Agent{makeAgent(t, "step1"), makeAgent(t, "step2")},
			mapper,
		)
		resp, err := p.Run(ctx, dive.WithInput("start"))
		assert.NoError(t, err)
		assert.Equal(t, "step2", resp.OutputText())
		// Mapper is called between steps only (not after the last step)
		assert.Equal(t, 1, len(mapperOutputs))
		assert.Equal(t, "step1", mapperOutputs[0])
	})

	t.Run("mapper error stops execution", func(t *testing.T) {
		mapper := func(_ context.Context, _ *dive.Response) (dive.CreateResponseOption, error) {
			return nil, errors.New("mapper failed")
		}
		p := workflow.SequentialWithMapper(
			[]*dive.Agent{makeAgent(t, "out"), makeAgent(t, "out2")},
			mapper,
		)
		resp, err := p.Run(ctx, dive.WithInput("start"))
		assert.Error(t, err)
		assert.Nil(t, resp)
	})
}
