package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecutionSignal(t *testing.T) {
	t.Run("NewExecutionSignal", func(t *testing.T) {
		executionID := "test-execution-1"
		signalType := "test-signal"
		data := map[string]interface{}{
			"key": "value",
		}

		signal := NewExecutionSignal(executionID, signalType, data)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, signalType, signal.SignalType)
		require.Equal(t, data, signal.Data)
		require.NotEmpty(t, signal.ID)
		require.False(t, signal.Timestamp.IsZero())
	})

	t.Run("Validate", func(t *testing.T) {
		// Valid signal
		validSignal := &ExecutionSignal{
			ID:          "signal-1",
			ExecutionID: "exec-1",
			SignalType:  "test",
			Timestamp:   time.Now(),
		}
		require.NoError(t, validSignal.Validate())

		// Invalid signals
		invalidSignals := []*ExecutionSignal{
			{ID: "signal-1", SignalType: "test", Timestamp: time.Now()},        // Missing ExecutionID
			{ID: "signal-1", ExecutionID: "exec-1", Timestamp: time.Now()},     // Missing SignalType
			{ID: "signal-1", ExecutionID: "exec-1", SignalType: "test"},        // Missing Timestamp
			{ExecutionID: "exec-1", SignalType: "test", Timestamp: time.Now()}, // Missing ID
		}

		for i, signal := range invalidSignals {
			t.Run(fmt.Sprintf("Invalid_%d", i), func(t *testing.T) {
				require.Error(t, signal.Validate())
			})
		}
	})
}

func TestSignalRegistry(t *testing.T) {
	registry := NewSignalRegistry()

	t.Run("RegisterAndGetHandler", func(t *testing.T) {
		handlerCalled := false
		handler := SignalHandlerFunc(func(ctx context.Context, signal *ExecutionSignal) error {
			handlerCalled = true
			return nil
		})

		// Register handler
		registry.RegisterHandler("test-signal", handler)

		// Get handler
		retrievedHandler, exists := registry.GetHandler("test-signal")
		require.True(t, exists)
		require.NotNil(t, retrievedHandler)

		// Test handler
		signal := NewExecutionSignal("exec-1", "test-signal", nil)
		err := retrievedHandler.HandleSignal(context.Background(), signal)
		require.NoError(t, err)
		require.True(t, handlerCalled)
	})

	t.Run("RegisterHandlerFunc", func(t *testing.T) {
		handlerCalled := false
		registry.RegisterHandlerFunc("func-signal", func(ctx context.Context, signal *ExecutionSignal) error {
			handlerCalled = true
			return nil
		})

		handler, exists := registry.GetHandler("func-signal")
		require.True(t, exists)

		signal := NewExecutionSignal("exec-1", "func-signal", nil)
		err := handler.HandleSignal(context.Background(), signal)
		require.NoError(t, err)
		require.True(t, handlerCalled)
	})

	t.Run("ProcessSignal", func(t *testing.T) {
		processedSignals := make([]string, 0)
		registry.RegisterHandlerFunc("process-test", func(ctx context.Context, signal *ExecutionSignal) error {
			processedSignals = append(processedSignals, signal.SignalType)
			return nil
		})

		signal := NewExecutionSignal("exec-1", "process-test", nil)
		err := registry.ProcessSignal(context.Background(), signal)
		require.NoError(t, err)
		require.Contains(t, processedSignals, "process-test")
	})

	t.Run("ProcessSignalNoHandler", func(t *testing.T) {
		signal := NewExecutionSignal("exec-1", "unknown-signal", nil)
		err := registry.ProcessSignal(context.Background(), signal)
		require.NoError(t, err) // Should not error for unknown signals
	})

	t.Run("HandlerError", func(t *testing.T) {
		registry.RegisterHandlerFunc("error-signal", func(ctx context.Context, signal *ExecutionSignal) error {
			return fmt.Errorf("handler error")
		})

		signal := NewExecutionSignal("exec-1", "error-signal", nil)
		err := registry.ProcessSignal(context.Background(), signal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "handler error")
	})
}

func TestSignalQueue(t *testing.T) {
	queue := NewSignalQueue()

	t.Run("EnqueueAndDequeue", func(t *testing.T) {
		executionID := "test-execution"

		// Create test signals
		signal1 := NewExecutionSignal(executionID, "signal-1", nil)
		signal2 := NewExecutionSignal(executionID, "signal-2", nil)

		// Test empty queue
		require.False(t, queue.HasSignals(executionID))
		signals := queue.DequeueSignals(executionID)
		require.Nil(t, signals)

		// Enqueue signals
		queue.EnqueueSignal(signal1)
		queue.EnqueueSignal(signal2)

		// Check HasSignals
		require.True(t, queue.HasSignals(executionID))

		// Peek signals
		peekedSignals := queue.PeekSignals(executionID)
		require.Len(t, peekedSignals, 2)
		require.Equal(t, signal1.ID, peekedSignals[0].ID)
		require.Equal(t, signal2.ID, peekedSignals[1].ID)

		// Signals should still be in queue after peek
		require.True(t, queue.HasSignals(executionID))

		// Dequeue signals
		dequeuedSignals := queue.DequeueSignals(executionID)
		require.Len(t, dequeuedSignals, 2)
		require.Equal(t, signal1.ID, dequeuedSignals[0].ID)
		require.Equal(t, signal2.ID, dequeuedSignals[1].ID)

		// Queue should be empty after dequeue
		require.False(t, queue.HasSignals(executionID))
	})

	t.Run("MultipleExecutions", func(t *testing.T) {
		exec1 := "execution-1"
		exec2 := "execution-2"

		signal1 := NewExecutionSignal(exec1, "signal-1", nil)
		signal2 := NewExecutionSignal(exec2, "signal-2", nil)

		queue.EnqueueSignal(signal1)
		queue.EnqueueSignal(signal2)

		// Each execution should have its own signals
		require.True(t, queue.HasSignals(exec1))
		require.True(t, queue.HasSignals(exec2))

		// Dequeue from one execution
		signals1 := queue.DequeueSignals(exec1)
		require.Len(t, signals1, 1)
		require.Equal(t, signal1.ID, signals1[0].ID)

		// Other execution should still have signals
		require.False(t, queue.HasSignals(exec1))
		require.True(t, queue.HasSignals(exec2))

		signals2 := queue.DequeueSignals(exec2)
		require.Len(t, signals2, 1)
		require.Equal(t, signal2.ID, signals2[0].ID)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		const numGoroutines = 10
		const signalsPerGoroutine = 5
		executionID := "concurrent-test"

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Start goroutines that enqueue signals
		for i := 0; i < numGoroutines; i++ {
			go func(goroutineID int) {
				defer wg.Done()
				for j := 0; j < signalsPerGoroutine; j++ {
					signal := NewExecutionSignal(executionID, fmt.Sprintf("signal-%d-%d", goroutineID, j), nil)
					queue.EnqueueSignal(signal)
				}
			}(i)
		}

		wg.Wait()

		// Verify all signals were enqueued
		signals := queue.DequeueSignals(executionID)
		require.Len(t, signals, numGoroutines*signalsPerGoroutine)
	})
}

func TestSignalHelpers(t *testing.T) {
	executionID := "test-execution"

	t.Run("NewPauseSignal", func(t *testing.T) {
		reason := "system maintenance"
		signal := NewPauseSignal(executionID, reason)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypePause, signal.SignalType)
		require.Equal(t, reason, signal.Data["reason"])
	})

	t.Run("NewResumeSignal", func(t *testing.T) {
		reason := "maintenance completed"
		signal := NewResumeSignal(executionID, reason)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeResume, signal.SignalType)
		require.Equal(t, reason, signal.Data["reason"])
	})

	t.Run("NewCancelSignal", func(t *testing.T) {
		reason := "user requested"
		force := true
		signal := NewCancelSignal(executionID, reason, force)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeCancel, signal.SignalType)
		require.Equal(t, reason, signal.Data["reason"])
		require.Equal(t, force, signal.Data["force"])
	})

	t.Run("NewUpdateInputsSignal", func(t *testing.T) {
		newInputs := map[string]interface{}{
			"param1": "new_value",
			"param2": 42,
		}
		merge := true
		signal := NewUpdateInputsSignal(executionID, newInputs, merge)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeUpdateInputs, signal.SignalType)
		require.Equal(t, newInputs, signal.Data["new_inputs"])
		require.Equal(t, merge, signal.Data["merge"])
	})

	t.Run("NewUpdateParamsSignal", func(t *testing.T) {
		stepName := "step1"
		newParams := map[string]interface{}{
			"temperature": 0.8,
		}
		merge := false
		signal := NewUpdateParamsSignal(executionID, stepName, newParams, merge)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeUpdateParams, signal.SignalType)
		require.Equal(t, stepName, signal.Data["step_name"])
		require.Equal(t, newParams, signal.Data["new_params"])
		require.Equal(t, merge, signal.Data["merge"])
	})

	t.Run("NewStepCompleteSignal", func(t *testing.T) {
		stepName := "step1"
		output := "Step completed successfully"
		force := true
		signal := NewStepCompleteSignal(executionID, stepName, output, force)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeStepComplete, signal.SignalType)
		require.Equal(t, stepName, signal.Data["step_name"])
		require.Equal(t, output, signal.Data["output"])
		require.Equal(t, force, signal.Data["force"])
	})

	t.Run("NewStepSkipSignal", func(t *testing.T) {
		stepName := "step1"
		reason := "step not needed"
		signal := NewStepSkipSignal(executionID, stepName, reason)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeStepSkip, signal.SignalType)
		require.Equal(t, stepName, signal.Data["step_name"])
		require.Equal(t, reason, signal.Data["reason"])
	})

	t.Run("NewCustomSignal", func(t *testing.T) {
		customType := "custom_action"
		data := map[string]interface{}{
			"action": "restart",
			"target": "service1",
		}
		signal := NewCustomSignal(executionID, customType, data)

		require.Equal(t, executionID, signal.ExecutionID)
		require.Equal(t, SignalTypeCustom, signal.SignalType)
		require.Equal(t, customType, signal.Data["custom_type"])
		require.Equal(t, "restart", signal.Data["action"])
		require.Equal(t, "service1", signal.Data["target"])
	})
}

func TestSignalConstants(t *testing.T) {
	// Test that signal type constants are defined
	signalTypes := []string{
		SignalTypePause,
		SignalTypeResume,
		SignalTypeCancel,
		SignalTypeUpdateInputs,
		SignalTypeUpdateParams,
		SignalTypeStepComplete,
		SignalTypeStepSkip,
		SignalTypeCustom,
	}

	for _, signalType := range signalTypes {
		require.NotEmpty(t, signalType)
	}

	// Test that they're unique
	seen := make(map[string]bool)
	for _, signalType := range signalTypes {
		require.False(t, seen[signalType], "Duplicate signal type: %s", signalType)
		seen[signalType] = true
	}
}

func TestSignalID(t *testing.T) {
	// Test that generateSignalID produces unique IDs
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateSignalID()
		require.NotEmpty(t, id)
		require.False(t, ids[id], "Duplicate ID generated: %s", id)
		ids[id] = true
	}
}

func BenchmarkSignalQueue(b *testing.B) {
	queue := NewSignalQueue()
	executionID := "bench-execution"

	b.Run("EnqueueSignal", func(b *testing.B) {
		signals := make([]*ExecutionSignal, b.N)
		for i := 0; i < b.N; i++ {
			signals[i] = NewExecutionSignal(executionID, "test-signal", nil)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			queue.EnqueueSignal(signals[i])
		}
	})

	b.Run("DequeueSignals", func(b *testing.B) {
		// Pre-populate queue
		for i := 0; i < 1000; i++ {
			signal := NewExecutionSignal(fmt.Sprintf("exec-%d", i), "test-signal", nil)
			queue.EnqueueSignal(signal)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			execID := fmt.Sprintf("exec-%d", i%1000)
			queue.DequeueSignals(execID)
		}
	})

	b.Run("HasSignals", func(b *testing.B) {
		// Pre-populate queue
		for i := 0; i < 1000; i++ {
			signal := NewExecutionSignal(fmt.Sprintf("exec-%d", i), "test-signal", nil)
			queue.EnqueueSignal(signal)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			execID := fmt.Sprintf("exec-%d", i%1000)
			queue.HasSignals(execID)
		}
	})
}

func BenchmarkSignalRegistry(b *testing.B) {
	registry := NewSignalRegistry()

	// Pre-register handlers
	for i := 0; i < 100; i++ {
		signalType := fmt.Sprintf("signal-%d", i)
		registry.RegisterHandlerFunc(signalType, func(ctx context.Context, signal *ExecutionSignal) error {
			return nil
		})
	}

	signal := NewExecutionSignal("bench-execution", "signal-0", nil)

	b.Run("ProcessSignal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			registry.ProcessSignal(context.Background(), signal)
		}
	})

	b.Run("GetHandler", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			signalType := fmt.Sprintf("signal-%d", i%100)
			registry.GetHandler(signalType)
		}
	})
}
