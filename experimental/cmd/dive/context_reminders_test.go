package main

import (
	"context"
	"testing"

	"github.com/deepnoodle-ai/dive"
	"github.com/deepnoodle-ai/dive/llm"
	"github.com/deepnoodle-ai/wonton/assert"
)

type contextCaptureModel struct{ messages []*llm.Message }

func (m *contextCaptureModel) Name() string { return "test" }
func (m *contextCaptureModel) Generate(_ context.Context, opts ...llm.Option) (*llm.Response, error) {
	cfg := &llm.Config{}
	cfg.Apply(opts...)
	m.messages = cfg.Messages
	return &llm.Response{Role: llm.Assistant, Content: []llm.Content{&llm.TextContent{Text: "ok"}}, StopReason: "stop"}, nil
}

func TestParseReminderSpecs(t *testing.T) {
	pinned, operator, err := parseReminderSpecs(
		[]string{"environment=cwd=/srv/app"},
		[]string{"mode=read only"},
	)
	assert.NoError(t, err)
	assert.Len(t, pinned, 1)
	assert.Equal(t, "environment", pinned[0].Name)
	assert.Equal(t, "cwd=/srv/app", pinned[0].Content)
	assert.Len(t, operator, 1)
	assert.Equal(t, dive.ReminderTierOperator, operator[0].Tier)

	_, _, err = parseReminderSpecs([]string{"missing-separator"}, nil)
	assert.Error(t, err)
}

func TestCLIContextDemoWiring(t *testing.T) {
	pinned, operator, err := parseReminderSpecs(
		[]string{"environment=cwd=/srv/app"},
		[]string{"mode=read only"},
	)
	assert.NoError(t, err)
	model := &contextCaptureModel{}
	agent, err := dive.NewAgent(dive.AgentOptions{
		Model: model,
		Hooks: dive.Hooks{PreGeneration: []dive.PreGenerationHook{pinRemindersHook(pinned)}},
	})
	assert.NoError(t, err)
	input := reminderInputMessages("continue", nil, operator)
	_, err = agent.CreateResponse(context.Background(), dive.WithMessages(input...))
	assert.NoError(t, err)

	contextReminder, ok := dive.FindLatestReminder(model.messages, "environment")
	assert.True(t, ok)
	assert.Equal(t, "cwd=/srv/app", contextReminder.Content)
	modeReminder, ok := dive.FindLatestReminder(model.messages, "mode")
	assert.True(t, ok)
	assert.Equal(t, dive.ReminderTierOperator, modeReminder.Tier)
	assert.Equal(t, llm.System, input[1].Role)
}
