package environment

import (
	"context"
	"testing"

	"github.com/getstingrai/dive"
	"github.com/getstingrai/dive/agent"
	"github.com/getstingrai/dive/slogger"
	"github.com/getstingrai/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestNewEnvironment(t *testing.T) {
	logger := slogger.New(slogger.LevelDebug)

	var tasks []dive.Task

	a := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "Poet Laureate",
		Work: func(ctx context.Context, task dive.Task) (dive.Stream, error) {
			tasks = append(tasks, task)
			stream := dive.NewStream()
			publisher := stream.Publisher()
			defer publisher.Close()
			var content string
			if task.Name() == "Write a Poem" {
				content = "A haiku about the fall"
			} else if task.Name() == "Summary" {
				content = "A summary of that great poem"
			} else {
				t.Fatalf("unexpected task: %s", task.Name())
			}
			publisher.Send(ctx, &dive.Event{
				Type:    "task.result",
				Payload: &dive.TaskResult{Content: content},
			})
			return stream, nil
		},
	})

	w, err := workflow.NewWorkflow(workflow.WorkflowOptions{
		Name: "Poetry Writing",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:  "Write a Poem",
				Agent: a,
				Prompt: &dive.Prompt{
					Name: "Write a Poem",
					Text: "Write a limerick about cabbage",
				},
				Next: []*workflow.Edge{{Step: "Summary"}},
			}),
			workflow.NewStep(workflow.StepOptions{
				Name:  "Summary",
				Agent: a,
				Prompt: &dive.Prompt{
					Name: "Summary",
					Text: "Summarize the poem",
				},
				End: true,
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(EnvironmentOptions{
		Name:      "test",
		Agents:    []dive.Agent{a},
		Workflows: []*workflow.Workflow{w},
		Logger:    logger,
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	require.Equal(t, "test", env.Name())

	execution, err := env.ExecuteWorkflow(context.Background(), w.Name(), map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, execution)

	err = execution.Wait()
	require.NoError(t, err)

	require.Equal(t, 2, len(tasks))
	t1 := tasks[0]
	t2 := tasks[1]
	require.Equal(t, "Write a Poem", t1.Name())
	require.Equal(t, "Summary", t2.Name())

	pathStates := execution.PathStates()
	require.Equal(t, 1, len(pathStates))
	s0 := pathStates[0]
	require.Equal(t, PathStatusCompleted, s0.Status)
	require.Equal(t, "Summary", s0.CurrentStep.Name())
	require.Equal(t, "A summary of that great poem", s0.StepOutputs["Summary"])
	require.Equal(t, "A haiku about the fall", s0.StepOutputs["Write a Poem"])
}

func TestEnvironmentWithMultipleAgents(t *testing.T) {
	logger := slogger.New(slogger.LevelDebug)

	agent1 := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "Writer",
		Work: func(ctx context.Context, task dive.Task) (dive.Stream, error) {
			stream := dive.NewStream()
			go func() {
				publisher := stream.Publisher()
				defer publisher.Close()
				publisher.Send(ctx, &dive.Event{
					Type:    "task.result",
					Payload: &dive.TaskResult{Content: "Written content"},
				})
			}()
			return stream, nil
		},
	})

	agent2 := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "Editor",
		Work: func(ctx context.Context, task dive.Task) (dive.Stream, error) {
			stream := dive.NewStream()
			go func() {
				publisher := stream.Publisher()
				defer publisher.Close()
				publisher.Send(ctx, &dive.Event{
					Type:    "task.result",
					Payload: &dive.TaskResult{Content: "Edited content"},
				})
			}()
			return stream, nil
		},
	})

	env, err := New(EnvironmentOptions{
		Name:   "test-multi-agent",
		Agents: []dive.Agent{agent1, agent2},
		Logger: logger,
	})
	require.NoError(t, err)
	require.NotNil(t, env)

	defer env.Stop(context.Background())

	require.Equal(t, 2, len(env.Agents()))

	foundWriter := false
	foundEditor := false
	for _, a := range env.Agents() {
		if a.Name() == "Writer" {
			foundWriter = true
		}
		if a.Name() == "Editor" {
			foundEditor = true
		}
	}
	require.True(t, foundWriter, "Writer agent should be present")
	require.True(t, foundEditor, "Editor agent should be present")
}

func TestEnvironmentGetAgent(t *testing.T) {
	logger := slogger.New(slogger.LevelDebug)

	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "TestAgent",
	})

	env, err := New(EnvironmentOptions{
		Name:   "test-get-agent",
		Agents: []dive.Agent{mockAgent},
		Logger: logger,
	})
	require.NoError(t, err)

	// Test getting existing agent
	agent, err := env.GetAgent("TestAgent")
	require.NoError(t, err)
	require.NotNil(t, agent)
	require.Equal(t, "TestAgent", agent.Name())

	// Test getting non-existent agent
	agent, err = env.GetAgent("NonExistentAgent")
	require.Error(t, err)
	require.Nil(t, agent)
	require.Contains(t, err.Error(), "agent not found")
}

func TestExecutionStats(t *testing.T) {
	logger := slogger.New(slogger.LevelDebug)

	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "StatsAgent",
		Work: func(ctx context.Context, task dive.Task) (dive.Stream, error) {
			stream := dive.NewStream()
			go func() {
				publisher := stream.Publisher()
				defer publisher.Close()
				publisher.Send(ctx, &dive.Event{
					Type:    "task.result",
					Payload: &dive.TaskResult{Content: "Task completed"},
				})
			}()
			return stream, nil
		},
	})

	w, err := workflow.NewWorkflow(workflow.WorkflowOptions{
		Name: "Stats Test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:  "Task1",
				Agent: mockAgent,
				Prompt: &dive.Prompt{
					Name: "Test Task",
					Text: "Test Task",
				},
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(EnvironmentOptions{
		Name:      "test-stats",
		Agents:    []dive.Agent{mockAgent},
		Workflows: []*workflow.Workflow{w},
		Logger:    logger,
	})
	require.NoError(t, err)

	execution, err := env.ExecuteWorkflow(context.Background(), w.Name(), map[string]interface{}{})
	require.NoError(t, err)

	err = execution.Wait()
	require.NoError(t, err)

	stats := execution.GetStats()
	require.Equal(t, 1, stats.TotalPaths)
	require.Equal(t, 0, stats.ActivePaths)
	require.Equal(t, 1, stats.CompletedPaths)
	require.Equal(t, 0, stats.FailedPaths)
	require.False(t, stats.StartTime.IsZero())
	require.False(t, stats.EndTime.IsZero())
	require.True(t, stats.Duration > 0)
}

func TestExecutionCancellation(t *testing.T) {
	logger := slogger.New(slogger.LevelDebug)

	// Create a channel to control when the task completes
	taskControl := make(chan struct{})

	mockAgent := agent.NewMockAgent(agent.MockAgentOptions{
		Name: "SlowAgent",
		Work: func(ctx context.Context, task dive.Task) (dive.Stream, error) {
			stream := dive.NewStream()
			go func() {
				publisher := stream.Publisher()
				defer publisher.Close()

				select {
				case <-ctx.Done():
					// Context was cancelled
					return
				case <-taskControl:
					// Task was allowed to complete
					publisher.Send(ctx, &dive.Event{
						Type:    "task.result",
						Payload: &dive.TaskResult{Content: "Completed"},
					})
				}
			}()
			return stream, nil
		},
	})

	w, err := workflow.NewWorkflow(workflow.WorkflowOptions{
		Name: "Cancellation Test",
		Steps: []*workflow.Step{
			workflow.NewStep(workflow.StepOptions{
				Name:  "SlowTask",
				Agent: mockAgent,
				Prompt: &dive.Prompt{
					Name: "Slow Task",
					Text: "Slow Task",
				},
			}),
		},
	})
	require.NoError(t, err)

	env, err := New(EnvironmentOptions{
		Name:      "test-cancellation",
		Agents:    []dive.Agent{mockAgent},
		Workflows: []*workflow.Workflow{w},
		Logger:    logger,
	})
	require.NoError(t, err)

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())

	execution, err := env.ExecuteWorkflow(ctx, w.Name(), map[string]interface{}{})
	require.NoError(t, err)

	// Cancel the context before allowing the task to complete
	cancel()

	err = execution.Wait()
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")

	// Clean up
	close(taskControl)
}
