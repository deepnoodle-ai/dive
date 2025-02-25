package dive

import (
	"context"
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
		Name:     "test",
		Role:     Role{Description: "test"},
		LLM:      anthropic.New(),
		LogLevel: "info",
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

func TestAgentSystemPromptWithoutTeam(t *testing.T) {
	tests := []struct {
		name     string
		options  AgentOptions
		expected string
	}{
		{
			name: "basic agent",
			options: AgentOptions{
				Name: "TestAgent",
				Role: Role{Description: "You are a research assistant"},
			},
			expected: "fixtures/agent-system-prompt-1.txt",
		},
		{
			name: "supervisor agent",
			options: AgentOptions{
				Name: "Lead Researcher",
				Role: Role{
					Description:  "You supervise a research team",
					IsSupervisor: true,
				},
			},
			expected: "fixtures/agent-system-prompt-2.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create agent with test options
			agent := NewAgent(tt.options)

			// Get the system prompt
			systemPrompt, err := agent.getSystemPrompt()
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
				Name: "Supervisor",
				Role: Role{
					Description:  "You are a research lead",
					IsSupervisor: true,
				},
			}),
			NewAgent(AgentOptions{
				Name: "Researcher",
				Role: Role{
					Description:  "You are a research assistant",
					IsSupervisor: false,
				},
			}),
		},
	})
	require.NoError(t, err)

	supervisorAgent, found := team.GetAgent("Supervisor")
	require.True(t, found)
	require.True(t, supervisorAgent.Role().IsSupervisor)

	researcherAgent, found := team.GetAgent("Researcher")
	require.True(t, found)
	require.False(t, researcherAgent.Role().IsSupervisor)

	supervisor, ok := supervisorAgent.(*DiveAgent)
	require.True(t, ok)

	systemPrompt, err := supervisor.getSystemPrompt()
	require.NoError(t, err)

	fmt.Println(systemPrompt)

	expected, err := os.ReadFile("fixtures/agent-system-prompt-3.txt")
	require.NoError(t, err)

	require.Equal(t, string(expected), systemPrompt)
}
