package dive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
		Name:         "test",
		Description:  "test",
		Instructions: "test",
		IsSupervisor: false,
		LLM:          anthropic.New(),
		LogLevel:     "info",
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
		Name:         "test",
		Description:  "test",
		IsSupervisor: false,
		LLM:          anthropic.New(),
		LogLevel:     "info",
	})

	err := agent.Start(ctx)
	require.NoError(t, err)

	response, err := agent.Generate(ctx, llm.NewUserMessage("Hello, world!"))
	require.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	require.True(t, strings.Contains(text, "hello") || strings.Contains(text, "hi"))

	err = agent.Stop(ctx)
	require.NoError(t, err)
}

// TestAgentChatWithTools tests that the agent can use tools in chat mode
func TestAgentChatWithTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Create a simple tool for testing
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

	agent := NewAgent(AgentOptions{
		Name:         "test",
		Description:  "test",
		IsSupervisor: false,
		LLM:          anthropic.New(),
		LogLevel:     "info",
		Tools:        []llm.Tool{llm.NewTool(&echoToolDef, echoFunc)},
	})

	err := agent.Start(ctx)
	require.NoError(t, err)

	response, err := agent.Generate(ctx, llm.NewUserMessage("Please use the echo tool to echo 'hello world'"))
	require.NoError(t, err)

	text := strings.ToLower(response.Message().Text())
	fmt.Println(text)
	require.Contains(t, text, "echo")
	require.Equal(t, "hello world", echoInput)
	err = agent.Stop(ctx)
	require.NoError(t, err)
}

func TestAgentTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	agent := NewAgent(AgentOptions{
		Name:         "test",
		Description:  "test",
		Instructions: "test",
		IsSupervisor: false,
		LLM:          anthropic.New(),
		LogLevel:     "info",
	})

	err := agent.Start(ctx)
	require.NoError(t, err)
	defer agent.Stop(ctx)

	task := NewTask(TaskOptions{
		Name:           "Poem",
		Description:    "Write a cat poem",
		ExpectedOutput: "A short poem about a cat",
	})

	stream, err := agent.Work(ctx, task)
	require.NoError(t, err)

	var result *TaskResult
	running := true
	for running {
		select {
		case event := <-stream.Channel():
			if event.TaskResult != nil {
				result = event.TaskResult
				running = false
			}
		case <-ctx.Done():
			t.Fatal("context canceled while waiting for task result")
		}
	}

	content := strings.ToLower(result.Content)

	matches := 0
	for _, word := range []string{"cat", "whiskers", "paws"} {
		if strings.Contains(content, word) {
			matches += 1
		}
	}
	require.Greater(t, matches, 0)
}

func TestAgentSystemPromptWithoutTeam(t *testing.T) {
	tests := []struct {
		name     string
		options  AgentOptions
		expected string
	}{
		{
			name: "basic agent",
			options: AgentOptions{
				Name:         "TestAgent",
				Description:  "You are a research assistant.",
				Instructions: "You are extremely thorough and detail-oriented.",
				IsSupervisor: false,
				LLM:          anthropic.New(),
				LogLevel:     "info",
			},
			expected: "fixtures/agent-system-prompt-1.txt",
		},
		{
			name: "supervisor agent",
			options: AgentOptions{
				Name:         "Lead Researcher",
				Description:  "You supervise a research team.",
				Instructions: "You are kind and helpful.",
				IsSupervisor: true,
				LLM:          anthropic.New(),
				LogLevel:     "info",
			},
			expected: "fixtures/agent-system-prompt-2.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create agent with test options
			agent := NewAgent(tt.options)

			// Get the system prompt
			systemPrompt, err := agent.getSystemPromptForMode("task")
			require.NoError(t, err)

			fmt.Println(systemPrompt)

			// Read expected prompt from file
			expected, err := os.ReadFile(tt.expected)
			require.NoError(t, err)

			require.Equal(t, string(expected), systemPrompt)
		})
	}
}

func TestAgentSystemPromptWithTeam(t *testing.T) {
	team, err := NewTeam(TeamOptions{
		Name:        "Research Team",
		Description: "A team of researchers",
		Agents: []Agent{
			NewAgent(AgentOptions{
				Name:         "Supervisor",
				Description:  "Research lead.",
				IsSupervisor: true,
				LLM:          anthropic.New(),
				LogLevel:     "info",
			}),
			NewAgent(AgentOptions{
				Name:         "Researcher",
				Description:  "Research assistant.",
				IsSupervisor: false,
				LLM:          anthropic.New(),
				LogLevel:     "info",
			}),
		},
	})
	require.NoError(t, err)

	supervisorAgent, found := team.GetAgent("Supervisor")
	require.True(t, found)
	require.True(t, supervisorAgent.(TeamAgent).IsSupervisor())

	researcherAgent, found := team.GetAgent("Researcher")
	require.True(t, found)
	require.False(t, researcherAgent.(TeamAgent).IsSupervisor())

	supervisor, ok := supervisorAgent.(*DiveAgent)
	require.True(t, ok)

	systemPrompt, err := supervisor.getSystemPromptForMode("task")
	require.NoError(t, err)

	expected, err := os.ReadFile("fixtures/agent-system-prompt-3.txt")
	require.NoError(t, err)

	require.Equal(t, string(expected), systemPrompt)
}

func TestAgentChatSystemPrompt(t *testing.T) {
	agent := NewAgent(AgentOptions{
		Name:         "TestAgent",
		Description:  "You are a research assistant.",
		Instructions: "You are extremely thorough and detail-oriented.",
		IsSupervisor: false,
		LLM:          anthropic.New(),
		LogLevel:     "info",
	})

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
