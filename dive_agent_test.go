package dive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/getstingrai/dive/llm"
	"github.com/getstingrai/dive/providers/anthropic"
	"github.com/stretchr/testify/require"
)

func TestAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agent := NewAgent(AgentOptions{
		Name: "test",
		Role: Role{Description: "test"},
	})

	err := agent.Start(ctx)
	require.NoError(t, err)

	err = agent.Event(ctx, &Event{Name: "test"})
	require.NoError(t, err)

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestAgentChat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agent := NewAgent(AgentOptions{
		Name: "test",
		Role: Role{Description: "test"},
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

func TestAgentTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agent := NewAgent(AgentOptions{
		Name: "test",
		Role: Role{Description: "test"},
		LLM:  anthropic.New(),
	})

	err := agent.Start(ctx)
	require.NoError(t, err)
	defer agent.Stop(ctx)

	task := NewTask(TaskOptions{
		Name:           "Poem",
		Description:    "Write a cat poem",
		ExpectedOutput: "A short poem about a cat",
	})

	promise, err := agent.Work(ctx, task)
	require.NoError(t, err)

	result, err := promise.Get(ctx)
	require.NoError(t, err)

	content := strings.ToLower(result.Content)

	matches := 0
	for _, word := range []string{"cat", "whiskers", "paws"} {
		if strings.Contains(content, word) {
			matches += 1
		}
	}
	require.Greater(t, matches, 0)
}
