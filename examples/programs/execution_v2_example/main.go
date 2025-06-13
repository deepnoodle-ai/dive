package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/agent"
	"github.com/diveagents/dive/environment"
	"github.com/diveagents/dive/llm/providers/openai"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
)

func main() {
	ctx := context.Background()

	// Create an OpenAI model (you'll need to set OPENAI_API_KEY)
	model := openai.New(openai.WithModel("gpt-3.5-turbo"))

	aiAgent, err := agent.New(agent.Options{
		Name:         "assistant",
		Goal:         "You are a helpful assistant",
		Instructions: "You provide clear and concise responses",
		Model:        model,
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Create workflow steps
	greetStep := workflow.NewStep(workflow.StepOptions{
		Name:   "greet",
		Type:   "prompt",
		Agent:  aiAgent,
		Prompt: "Say hello to ${name} in a friendly way.",
	})

	printStep := workflow.NewStep(workflow.StepOptions{
		Name:   "print_result",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "The agent said: ${greet}",
		},
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "greeting-workflow",
		Steps: []*workflow.Step{greetStep, printStep},
		Inputs: []*workflow.Input{
			{Name: "name", Type: "string", Required: true},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create workflow: %v", err)
	}

	// Create environment
	env, err := environment.New(environment.Options{
		Name:      "execution-v2-example",
		Agents:    []dive.Agent{aiAgent},
		Workflows: []*workflow.Workflow{wf},
		Logger:    slogger.DefaultLogger,
	})
	if err != nil {
		log.Fatalf("Failed to create environment: %v", err)
	}

	// Start environment
	err = env.Start(ctx)
	if err != nil {
		log.Fatalf("Failed to start environment: %v", err)
	}
	defer env.Stop(ctx)

	// Create mock event store for this example
	eventStore := &mockEventStore{}

	// Create new deterministic execution
	execution, err := environment.NewExecution(environment.ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs: map[string]interface{}{
			"name": "World",
		},
		EventStore: eventStore,
		Logger:     slogger.DefaultLogger,
		ReplayMode: false,
	})
	if err != nil {
		log.Fatalf("Failed to create execution: %v", err)
	}

	fmt.Printf("Created execution with ID: %s\n", execution.ID())
	fmt.Printf("Initial status: %s\n", execution.Status())

	// Run the execution
	fmt.Println("\nRunning workflow...")
	err = execution.Run(ctx)
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}

	fmt.Printf("Final status: %s\n", execution.Status())
	fmt.Printf("Events recorded: %d\n", len(eventStore.events))

	// Show recorded events
	fmt.Println("\nRecorded events:")
	for i, event := range eventStore.events {
		fmt.Printf("%d. %s - %s (Step: %s)\n",
			i+1, event.EventType, event.Timestamp.Format("15:04:05"), event.StepName)
	}
}

// mockEventStore for the example
type mockEventStore struct {
	events    []*environment.ExecutionEvent
	snapshots map[string]*environment.ExecutionSnapshot
}

func (m *mockEventStore) AppendEvents(ctx context.Context, events []*environment.ExecutionEvent) error {
	m.events = append(m.events, events...)
	return nil
}

func (m *mockEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*environment.ExecutionEvent, error) {
	var result []*environment.ExecutionEvent
	for _, event := range m.events {
		if event.ExecutionID == executionID && event.Sequence >= fromSeq {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*environment.ExecutionEvent, error) {
	return m.GetEvents(ctx, executionID, 0)
}

func (m *mockEventStore) SaveSnapshot(ctx context.Context, snapshot *environment.ExecutionSnapshot) error {
	if m.snapshots == nil {
		m.snapshots = make(map[string]*environment.ExecutionSnapshot)
	}
	m.snapshots[snapshot.ID] = snapshot
	return nil
}

func (m *mockEventStore) GetSnapshot(ctx context.Context, executionID string) (*environment.ExecutionSnapshot, error) {
	if m.snapshots == nil {
		return nil, nil
	}
	return m.snapshots[executionID], nil
}

func (m *mockEventStore) ListExecutions(ctx context.Context, filter environment.ExecutionFilter) ([]*environment.ExecutionSnapshot, error) {
	var result []*environment.ExecutionSnapshot
	for _, snapshot := range m.snapshots {
		result = append(result, snapshot)
	}
	return result, nil
}

func (m *mockEventStore) DeleteExecution(ctx context.Context, executionID string) error {
	if m.snapshots != nil {
		delete(m.snapshots, executionID)
	}
	return nil
}

func (m *mockEventStore) CleanupCompletedExecutions(ctx context.Context, olderThan time.Time) error {
	return nil
}
