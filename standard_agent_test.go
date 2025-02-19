package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/getstingrai/agents/llm"
	"github.com/getstingrai/agents/providers/anthropic"
	"github.com/stretchr/testify/require"
)

func TestStandardAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agent := NewStandardAgent(StandardAgentSpec{
		Name: "test",
		Role: &Role{
			Name: "test",
		},
	})

	err := agent.Start(ctx)
	require.NoError(t, err)

	err = agent.Event(ctx, &Event{Name: "test"})
	require.NoError(t, err)

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestStandardAgentChat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agent := NewStandardAgent(StandardAgentSpec{
		Name: "test",
		Role: &Role{Name: "test"},
		LLM:  anthropic.New(),
	})

	err := agent.Start(ctx)
	require.NoError(t, err)

	response, err := agent.Chat(ctx, llm.NewUserMessage("Hello, world!"))
	require.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	require.True(t, strings.Contains(text, "hello") || strings.Contains(text, "hi"))

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestStandardAgentTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agent := NewStandardAgent(StandardAgentSpec{
		Name: "test",
		Role: &Role{Name: "test"},
		LLM:  anthropic.New(),
	})

	err := agent.Start(ctx)
	require.NoError(t, err)
	defer agent.Stop(ctx)

	task := NewTask(TaskSpec{
		Name:           "Poem",
		Description:    "Write a cat poem",
		ExpectedOutput: "A short poem about a cat",
	})

	promise, err := agent.Work(ctx, task)
	require.NoError(t, err)

	result, err := promise.Get(ctx)
	require.NoError(t, err)

	content := result.Output.Content
	require.Contains(t, content, "cat")
	require.Contains(t, content, "poem")
}
