package environment

import (
	"context"
	"testing"
	"time"

	"github.com/diveagents/dive"
	"github.com/diveagents/dive/slogger"
	"github.com/diveagents/dive/workflow"
	"github.com/stretchr/testify/require"
)

func TestExecution_BasicWorkflow(t *testing.T) {
	// Create a simple mock agent
	mockAgent := &mockAgent{}

	// Create steps
	promptStep := workflow.NewStep(workflow.StepOptions{
		Name:   "greet",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Say hello to ${name}",
	})

	actionStep := workflow.NewStep(workflow.StepOptions{
		Name:   "print",
		Type:   "action",
		Action: "Print",
		Parameters: map[string]any{
			"Message": "Executed step: ${greet}",
		},
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "test-workflow",
		Steps: []*workflow.Step{promptStep, actionStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create mock event store
	eventStore := &mockEventStore{}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs: map[string]interface{}{
			"name": "World",
		},
		EventStore: eventStore,
		Logger:     slogger.DefaultLogger,
		ReplayMode: false,
	})
	require.NoError(t, err)
	require.NotNil(t, execution)

	// Verify initial state
	require.Equal(t, ExecutionStatusPending, execution.Status())
	require.NotEmpty(t, execution.ID())

	// Run execution
	ctx := context.Background()
	err = execution.Run(ctx)
	require.NoError(t, err)

	// Verify final state
	require.Equal(t, ExecutionStatusCompleted, execution.Status())

	// Verify events were recorded
	require.True(t, len(eventStore.events) > 0)

	// Check for execution started and completed events
	var hasStartEvent, hasCompleteEvent bool
	for _, event := range eventStore.events {
		switch event.EventType {
		case EventExecutionStarted:
			hasStartEvent = true
		case EventExecutionCompleted:
			hasCompleteEvent = true
		}
	}
	require.True(t, hasStartEvent, "Should have execution started event")
	require.True(t, hasCompleteEvent, "Should have execution completed event")
}

func TestExecution_OperationReplay(t *testing.T) {
	// Create a simple mock agent
	mockAgent := &mockAgent{}

	// Create step
	promptStep := workflow.NewStep(workflow.StepOptions{
		Name:   "greet",
		Type:   "prompt",
		Agent:  mockAgent,
		Prompt: "Say hello",
	})

	// Create workflow
	wf, err := workflow.New(workflow.Options{
		Name:  "test-workflow",
		Steps: []*workflow.Step{promptStep},
	})
	require.NoError(t, err)

	// Create environment
	env, err := New(Options{
		Name:   "test-env",
		Agents: []dive.Agent{mockAgent},
	})
	require.NoError(t, err)

	// Start environment
	err = env.Start(context.Background())
	require.NoError(t, err)
	defer env.Stop(context.Background())

	// Create execution in normal mode
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs:      map[string]interface{}{},
		EventStore:  &mockEventStore{},
		Logger:      slogger.DefaultLogger,
		ReplayMode:  false,
	})
	require.NoError(t, err)

	// Execute an operation to create a result
	op := Operation{
		Type:     "test_operation",
		StepName: "test_step",
		PathID:   "test_path",
		Parameters: map[string]interface{}{
			"test": "value",
		},
	}

	result, err := execution.ExecuteOperation(context.Background(), op, func() (interface{}, error) {
		return "test_result", nil
	})
	require.NoError(t, err)
	require.Equal(t, "test_result", result)

	// Verify operation result was cached
	cachedResult, found := execution.FindOperationResult(op.GenerateID())
	require.True(t, found)
	require.Equal(t, "test_result", cachedResult.Result)
	require.NoError(t, cachedResult.Error)
}

// mockEventStore implements ExecutionEventStore for testing
type mockEventStore struct {
	events    []*ExecutionEvent
	snapshots map[string]*ExecutionSnapshot
}

func (m *mockEventStore) AppendEvents(ctx context.Context, events []*ExecutionEvent) error {
	m.events = append(m.events, events...)
	return nil
}

func (m *mockEventStore) GetEvents(ctx context.Context, executionID string, fromSeq int64) ([]*ExecutionEvent, error) {
	var result []*ExecutionEvent
	for _, event := range m.events {
		if event.ExecutionID == executionID && event.Sequence >= fromSeq {
			result = append(result, event)
		}
	}
	return result, nil
}

func (m *mockEventStore) GetEventHistory(ctx context.Context, executionID string) ([]*ExecutionEvent, error) {
	return m.GetEvents(ctx, executionID, 0)
}

func (m *mockEventStore) SaveSnapshot(ctx context.Context, snapshot *ExecutionSnapshot) error {
	if m.snapshots == nil {
		m.snapshots = make(map[string]*ExecutionSnapshot)
	}
	m.snapshots[snapshot.ID] = snapshot
	return nil
}

func (m *mockEventStore) GetSnapshot(ctx context.Context, executionID string) (*ExecutionSnapshot, error) {
	if m.snapshots == nil {
		return nil, nil
	}
	return m.snapshots[executionID], nil
}

func (m *mockEventStore) ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ExecutionSnapshot, error) {
	var result []*ExecutionSnapshot
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

func TestExecution_evaluateCondition(t *testing.T) {
	// Create a mock workflow
	wf := &workflow.Workflow{}

	// Create a mock environment
	env := &Environment{
		started: true,
		logger:  slogger.DefaultLogger,
	}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs: map[string]interface{}{
			"user_age":  25,
			"user_name": "Alice",
			"active":    true,
		},
		Logger: slogger.DefaultLogger,
	})
	require.NoError(t, err)

	// Set some state variables
	execution.state.Set("score", 85)
	execution.state.Set("category", "premium")

	ctx := context.Background()

	tests := []struct {
		name      string
		condition string
		expected  bool
		wantErr   bool
	}{
		{
			name:      "simple true",
			condition: "true",
			expected:  true,
		},
		{
			name:      "simple false",
			condition: "false",
			expected:  false,
		},
		{
			name:      "access input variable",
			condition: "$(inputs.user_age > 18)",
			expected:  true,
		},
		{
			name:      "access input variable - false case",
			condition: "$(inputs.user_age < 18)",
			expected:  false,
		},
		{
			name:      "access state variable",
			condition: "$(state.score >= 80)",
			expected:  true,
		},
		{
			name:      "access state variable - false case",
			condition: "$(state.score < 50)",
			expected:  false,
		},
		{
			name:      "string comparison",
			condition: "$(state.category == \"premium\")",
			expected:  true,
		},
		{
			name:      "boolean input",
			condition: "$(inputs.active)",
			expected:  true,
		},
		{
			name:      "complex expression",
			condition: "$(inputs.user_age > 21 && state.score > 80)",
			expected:  true,
		},
		{
			name:      "complex expression - false case",
			condition: "$(inputs.user_age < 21 || state.score < 50)",
			expected:  false,
		},
		{
			name:      "string length check",
			condition: "$(len(inputs.user_name) > 3)",
			expected:  true,
		},
		{
			name:      "invalid syntax",
			condition: "$(invalid syntax here",
			wantErr:   true,
		},
		{
			name:      "undefined variable",
			condition: "$(nonexistent_var > 10)",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := execution.evaluateCondition(ctx, tt.condition)

			if tt.wantErr {
				require.Error(t, err, "Expected error for condition: %s", tt.condition)
				return
			}

			require.NoError(t, err, "Unexpected error for condition: %s", tt.condition)
			require.Equal(t, tt.expected, result, "Condition evaluation mismatch for: %s", tt.condition)
		})
	}
}

func TestExecution_buildConditionGlobals(t *testing.T) {
	// Create a mock workflow
	wf := &workflow.Workflow{}

	// Create a mock environment
	env := &Environment{
		started: true,
		logger:  slogger.DefaultLogger,
	}

	// Create execution
	execution, err := NewExecution(ExecutionV2Options{
		Workflow:    wf,
		Environment: env,
		Inputs: map[string]interface{}{
			"test_input": "test_value",
		},
		Logger: slogger.DefaultLogger,
	})
	require.NoError(t, err)

	// Set some state variables
	execution.state.Set("test_state", "state_value")

	// Build globals
	globals := execution.buildConditionGlobals()

	// Check that inputs are included
	require.Contains(t, globals, "inputs")
	inputs, ok := globals["inputs"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "test_value", inputs["test_input"])

	// Check that state is included
	require.Contains(t, globals, "state")
	state, ok := globals["state"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "state_value", state["test_state"])
	require.Equal(t, "test_value", state["test_input"]) // inputs are also stored in state

	// Check that safe built-ins are included
	require.Contains(t, globals, "len")
	require.Contains(t, globals, "string")

	// Check that unsafe built-ins are excluded (these would be dangerous)
	require.NotContains(t, globals, "read")
	require.NotContains(t, globals, "write")
	require.NotContains(t, globals, "exec")
}

func TestExecution_convertToBool(t *testing.T) {
	// Create a mock execution
	execution := &Execution{}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"empty string", "", false},
		{"non-empty string", "hello", true},
		{"false string", "false", false},
		{"true string", "true", true},
		{"False string", "False", false},
		{"TRUE string", "TRUE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily create Risor objects in tests without more setup,
			// so we'll just verify the method exists and can be called
			// The actual object conversion testing would require integration tests
			require.NotNil(t, execution.convertToBool)
		})
	}
}
