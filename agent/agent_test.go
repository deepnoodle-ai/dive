package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/llm"
	"github.com/diveagents/dive/llm/providers/anthropic"
	"github.com/diveagents/dive/slogger"
	"github.com/stretchr/testify/require"
)

func TestAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agent, err := New(Options{
		Name:         "Testing Agent",
		Goal:         "Test the agent",
		Backstory:    "You are a testing agent",
		IsSupervisor: false,
		LLM:          anthropic.New(),
	})
	require.NoError(t, err)

	err = agent.Start(ctx)
	require.NoError(t, err)

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestAgentChat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	agent, err := New(Options{
		Name: "Testing Agent",
		LLM:  anthropic.New(),
	})
	require.NoError(t, err)

	err = agent.Start(ctx)
	require.NoError(t, err)

	response, err := agent.Generate(ctx, llm.NewSingleUserMessage("Hello, world!"))
	require.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	matches := strings.Contains(text, "hello") || strings.Contains(text, "hi")
	require.True(t, matches)

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestAgentChatWithTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	echoToolDef := llm.ToolDefinition{
		Name:        "echo",
		Description: "Echoes back the input",
		Parameters: llm.Schema{
			Type:     "object",
			Required: []string{"text"},
			Properties: map[string]*llm.SchemaProperty{
				"text": {Type: "string", Description: "The text to echo back"},
			},
		},
	}

	var echoInput string

	echoFunc := func(ctx context.Context, input string) (string, error) {
		var m map[string]interface{}
		err := json.Unmarshal([]byte(input), &m)
		require.NoError(t, err)
		echoInput = m["text"].(string)
		return input, nil
	}

	agent, err := New(Options{
		LLM:   anthropic.New(),
		Tools: []llm.Tool{llm.NewTool(&echoToolDef, echoFunc)},
	})
	require.NoError(t, err)

	err = agent.Start(ctx)
	require.NoError(t, err)

	response, err := agent.Generate(ctx, llm.NewSingleUserMessage("Please use the echo tool to echo 'hello world'"))
	require.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	require.Contains(t, text, "echo")
	require.Equal(t, "hello world", echoInput)

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestAgentTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	logger := slogger.New(slogger.LevelDebug)

	agent, err := New(Options{
		Name:      "Poet",
		Backstory: "You're a poet that loves writing limericks",
		LLM:       anthropic.New(),
		Logger:    logger,
	})
	require.NoError(t, err)
	require.NoError(t, agent.Start(ctx))
	defer agent.Stop(ctx)

	stream, err := agent.Work(ctx, &Task{
		name: "Limerick",
		prompt: &dive.Prompt{
			Text:         "Write a limerick about a cat",
			Output:       "A limerick about a cat",
			OutputFormat: dive.OutputFormatMarkdown,
		},
	})
	require.NoError(t, err)

	event, err := dive.WaitForEvent[*dive.TaskResult](ctx, stream)
	require.NoError(t, err)

	content := strings.ToLower(event.Content)
	matches := 0
	for _, word := range []string{"cat", "whiskers", "paws"} {
		if strings.Contains(content, word) {
			matches += 1
		}
	}
	require.Greater(t, matches, 0, "poem should contain at least one cat-related word")
}

func TestAgentChatSystemPrompt(t *testing.T) {
	agent, err := New(Options{
		Name:         "TestAgent",
		Goal:         "Help research a topic.",
		Backstory:    "You are a research assistant.",
		IsSupervisor: false,
		LLM:          anthropic.New(),
	})
	require.NoError(t, err)

	// Get the chat system prompt
	chatSystemPrompt, err := agent.getSystemPromptForMode("chat")
	require.NoError(t, err)

	// Verify that the chat system prompt doesn't contain the status section
	require.NotContains(t, chatSystemPrompt, "<status>")
	require.NotContains(t, chatSystemPrompt, "active")
	require.NotContains(t, chatSystemPrompt, "completed")
	require.NotContains(t, chatSystemPrompt, "paused")
	require.NotContains(t, chatSystemPrompt, "blocked")
	require.NotContains(t, chatSystemPrompt, "error")

	// Get the task system prompt
	taskSystemPrompt, err := agent.getSystemPromptForMode("task")
	require.NoError(t, err)

	// Verify that the task system prompt contains the status section
	require.Contains(t, taskSystemPrompt, "<status>")
	require.Contains(t, taskSystemPrompt, "active")
	require.Contains(t, taskSystemPrompt, "completed")
	require.Contains(t, taskSystemPrompt, "paused")
	require.Contains(t, taskSystemPrompt, "blocked")
	require.Contains(t, taskSystemPrompt, "error")
}
