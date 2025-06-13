package environment

// func TestContinueAsNewOptions(t *testing.T) {
// 	t.Run("DefaultOptions", func(t *testing.T) {
// 		options := DefaultContinueAsNewOptions()

// 		require.Equal(t, 10000, options.MaxEvents)
// 		require.Equal(t, 24*time.Hour, options.MaxDuration)
// 		require.Equal(t, int64(100*1024*1024), options.MaxEventSize)
// 		require.Equal(t, 1, options.WorkflowVersion)
// 		require.True(t, options.PreservePaths)
// 		require.True(t, options.PreserveGlobals)
// 		require.Equal(t, 5*time.Second, options.ContinuationDelay)
// 	})

// 	t.Run("Validate", func(t *testing.T) {
// 		// Valid options
// 		validOptions := DefaultContinueAsNewOptions()
// 		require.NoError(t, validOptions.Validate())

// 		// Invalid options
// 		invalidOptions := []ContinueAsNewOptions{
// 			{MaxEvents: -1},
// 			{MaxDuration: -time.Hour},
// 			{MaxEventSize: -1000},
// 			{ContinuationDelay: -time.Second},
// 		}

// 		for i, options := range invalidOptions {
// 			t.Run(fmt.Sprintf("Invalid_%d", i), func(t *testing.T) {
// 				require.Error(t, options.Validate())
// 			})
// 		}
// 	})
// }

// func TestContinueAsNewMetrics(t *testing.T) {
// 	t.Run("BasicMetrics", func(t *testing.T) {
// 		metrics := ContinueAsNewMetrics{
// 			EventCount:        5000,
// 			ExecutionDuration: 12 * time.Hour,
// 			TotalEventSize:    50 * 1024 * 1024, // 50MB
// 			ActivePaths:       3,
// 			LastEventTime:     time.Now(),
// 		}

// 		require.Equal(t, int64(5000), metrics.EventCount)
// 		require.Equal(t, 12*time.Hour, metrics.ExecutionDuration)
// 		require.Equal(t, int64(50*1024*1024), metrics.TotalEventSize)
// 		require.Equal(t, 3, metrics.ActivePaths)
// 		require.False(t, metrics.LastEventTime.IsZero())
// 	})
// }

// func TestDefaultContinueAsNewEvaluator(t *testing.T) {
// 	logger := &mockLogger{}
// 	evaluator := NewDefaultContinueAsNewEvaluator(logger)

// 	t.Run("ShouldNotContinue", func(t *testing.T) {
// 		options := ContinueAsNewOptions{
// 			MaxEvents:   10000,
// 			MaxDuration: 24 * time.Hour,
// 		}

// 		metrics := ContinueAsNewMetrics{
// 			EventCount:        5000,
// 			ExecutionDuration: 12 * time.Hour,
// 		}

// 		decision, err := evaluator.ShouldContinueAsNew(context.Background(), metrics, options)
// 		require.NoError(t, err)
// 		require.False(t, decision.ShouldContinue)
// 		require.Equal(t, metrics, decision.CurrentMetrics)
// 	})

// 	t.Run("ShouldContinueByMaxEvents", func(t *testing.T) {
// 		options := ContinueAsNewOptions{
// 			MaxEvents:   5000,
// 			MaxDuration: 24 * time.Hour,
// 		}

// 		metrics := ContinueAsNewMetrics{
// 			EventCount:        5000,
// 			ExecutionDuration: 12 * time.Hour,
// 		}

// 		decision, err := evaluator.ShouldContinueAsNew(context.Background(), metrics, options)
// 		require.NoError(t, err)
// 		require.True(t, decision.ShouldContinue)
// 		require.Equal(t, ContinueAsNewReasonMaxEvents, decision.Reason)
// 	})

// 	t.Run("ShouldContinueByMaxDuration", func(t *testing.T) {
// 		options := ContinueAsNewOptions{
// 			MaxEvents:   10000,
// 			MaxDuration: 10 * time.Hour,
// 		}

// 		metrics := ContinueAsNewMetrics{
// 			EventCount:        5000,
// 			ExecutionDuration: 12 * time.Hour,
// 		}

// 		decision, err := evaluator.ShouldContinueAsNew(context.Background(), metrics, options)
// 		require.NoError(t, err)
// 		require.True(t, decision.ShouldContinue)
// 		require.Equal(t, ContinueAsNewReasonMaxDuration, decision.Reason)
// 	})

// 	t.Run("ShouldContinueByMaxEventSize", func(t *testing.T) {
// 		options := ContinueAsNewOptions{
// 			MaxEvents:    10000,
// 			MaxDuration:  24 * time.Hour,
// 			MaxEventSize: 50 * 1024 * 1024, // 50MB
// 		}

// 		metrics := ContinueAsNewMetrics{
// 			EventCount:        5000,
// 			ExecutionDuration: 12 * time.Hour,
// 			TotalEventSize:    60 * 1024 * 1024, // 60MB
// 		}

// 		decision, err := evaluator.ShouldContinueAsNew(context.Background(), metrics, options)
// 		require.NoError(t, err)
// 		require.True(t, decision.ShouldContinue)
// 		require.Equal(t, ContinueAsNewReasonMaxEventSize, decision.Reason)
// 	})

// 	t.Run("PrepareNewExecution", func(t *testing.T) {
// 		decision := &ContinueAsNewDecision{
// 			ShouldContinue: true,
// 			Reason:         ContinueAsNewReasonMaxEvents,
// 			CurrentMetrics: ContinueAsNewMetrics{
// 				EventCount: 10000,
// 			},
// 		}

// 		preserved, err := evaluator.PrepareNewExecution(context.Background(), decision, nil)
// 		require.NoError(t, err)
// 		require.NotNil(t, preserved)
// 		require.NotNil(t, preserved.ScriptGlobals)
// 		require.NotNil(t, preserved.ActivePaths)
// 		require.NotNil(t, preserved.CompletedSteps)
// 		require.NotNil(t, preserved.ExecutionInputs)
// 		require.NotNil(t, preserved.ContinuationInfo)
// 		require.Equal(t, 1, preserved.ContinuationInfo.ContinuationCount)
// 		require.Equal(t, int64(10000), preserved.ContinuationInfo.TotalEvents)
// 	})
// }

// func TestContinueAsNewManager(t *testing.T) {
// 	// Setup
// 	tempDir := t.TempDir()
// 	eventStore, err := NewSQLiteExecutionEventStore(path.Join(tempDir, "executions.db"), DefaultSQLiteStoreOptions())
// 	require.NoError(t, err)
// 	logger := &mockLogger{}
// 	evaluator := NewDefaultContinueAsNewEvaluator(logger)
// 	options := DefaultContinueAsNewOptions()
// 	options.MaxEvents = 5 // Low threshold for testing

// 	manager := NewContinueAsNewManager(evaluator, eventStore, options, logger)

// 	// Create test execution with events
// 	executionID := "test-execution"
// 	snapshot := &ExecutionSnapshot{
// 		ID:           executionID,
// 		WorkflowName: "test-workflow",
// 		WorkflowHash: "hash-123",
// 		InputsHash:   "input-hash-123",
// 		Status:       "running",
// 		StartTime:    time.Now().Add(-time.Hour),
// 		CreatedAt:    time.Now().Add(-time.Hour),
// 		UpdatedAt:    time.Now(),
// 		LastEventSeq: 0,
// 		Inputs:       map[string]interface{}{"param": "value"},
// 		Outputs:      map[string]interface{}{},
// 	}

// 	ctx := context.Background()
// 	err = eventStore.SaveSnapshot(ctx, snapshot)
// 	require.NoError(t, err)

// 	// Create test events
// 	events := make([]*ExecutionEvent, 6) // Exceeds threshold of 5
// 	for i := 0; i < 6; i++ {
// 		events[i] = &ExecutionEvent{
// 			ID:          fmt.Sprintf("event-%d", i),
// 			ExecutionID: executionID,
// 			Sequence:    int64(i + 1),
// 			Timestamp:   time.Now().Add(-time.Duration(60-i) * time.Minute),
// 			EventType:   EventStepCompleted,
// 			StepName:    fmt.Sprintf("step-%d", i),
// 			Data:        map[string]interface{}{"output": fmt.Sprintf("result-%d", i)},
// 		}
// 	}

// 	err = eventStore.AppendEvents(ctx, events)
// 	require.NoError(t, err)

// 	t.Run("EvaluateContinuation", func(t *testing.T) {
// 		decision, err := manager.EvaluateContinuation(ctx, executionID)
// 		require.NoError(t, err)
// 		require.True(t, decision.ShouldContinue)
// 		require.Equal(t, ContinueAsNewReasonMaxEvents, decision.Reason)
// 		require.Equal(t, int64(6), decision.CurrentMetrics.EventCount)
// 	})

// 	t.Run("ExecuteContinuation", func(t *testing.T) {
// 		decision, err := manager.EvaluateContinuation(ctx, executionID)
// 		require.NoError(t, err)
// 		require.True(t, decision.ShouldContinue)

// 		newExecutionID, err := manager.ExecuteContinuation(ctx, executionID, decision)
// 		require.NoError(t, err)
// 		require.NotEmpty(t, newExecutionID)
// 		require.NotEqual(t, executionID, newExecutionID)

// 		// Verify new execution snapshot was created
// 		newSnapshot, err := eventStore.GetSnapshot(ctx, newExecutionID)
// 		require.NoError(t, err)
// 		require.Equal(t, newExecutionID, newSnapshot.ID)
// 		require.Equal(t, snapshot.WorkflowName, newSnapshot.WorkflowName)
// 		require.Equal(t, "pending", newSnapshot.Status)

// 		// Verify continue-as-new event was recorded
// 		originalEvents, err := eventStore.GetEventHistory(ctx, executionID)
// 		require.NoError(t, err)
// 		require.Greater(t, len(originalEvents), 6) // Should have the continue-as-new event

// 		lastEvent := originalEvents[len(originalEvents)-1]
// 		require.Equal(t, EventExecutionContinueAsNew, lastEvent.EventType)

// 		// Verify new execution has start event
// 		newEvents, err := eventStore.GetEventHistory(ctx, newExecutionID)
// 		require.NoError(t, err)
// 		require.Len(t, newEvents, 1)
// 		require.Equal(t, EventExecutionStarted, newEvents[0].EventType)
// 		require.Equal(t, executionID, newEvents[0].Data["continued_from"])
// 	})

// 	t.Run("ExecuteContinuationFailsWhenShouldNotContinue", func(t *testing.T) {
// 		decision := &ContinueAsNewDecision{
// 			ShouldContinue: false,
// 		}

// 		_, err := manager.ExecuteContinuation(ctx, executionID, decision)
// 		require.Error(t, err)
// 		require.Contains(t, err.Error(), "should not continue as new")
// 	})

// 	t.Run("MonitorForContinuation", func(t *testing.T) {
// 		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
// 		defer cancel()

// 		decisions := manager.MonitorForContinuation(ctx, executionID, 10*time.Millisecond)

// 		// Should get a decision quickly since we exceed threshold
// 		select {
// 		case decision := <-decisions:
// 			require.NotNil(t, decision)
// 			require.True(t, decision.ShouldContinue)
// 		case <-ctx.Done():
// 			t.Fatal("Expected to receive a continue-as-new decision")
// 		}
// 	})
// }

// func TestContinueAsNewConstants(t *testing.T) {
// 	// Test that reason constants are defined
// 	reasons := []ContinueAsNewReason{
// 		ContinueAsNewReasonMaxEvents,
// 		ContinueAsNewReasonMaxDuration,
// 		ContinueAsNewReasonMaxEventSize,
// 		ContinueAsNewReasonCustomTrigger,
// 		ContinueAsNewReasonManual,
// 	}

// 	for _, reason := range reasons {
// 		require.NotEmpty(t, string(reason))
// 	}

// 	// Test that they're unique
// 	seen := make(map[ContinueAsNewReason]bool)
// 	for _, reason := range reasons {
// 		require.False(t, seen[reason], "Duplicate reason: %s", reason)
// 		seen[reason] = true
// 	}
// }

// func TestCalculateEventSize(t *testing.T) {
// 	t.Run("EmptyEvents", func(t *testing.T) {
// 		size := calculateEventSize([]*ExecutionEvent{})
// 		require.Equal(t, int64(0), size)
// 	})

// 	t.Run("SingleEvent", func(t *testing.T) {
// 		event := &ExecutionEvent{
// 			ID:          "test-id",
// 			ExecutionID: "exec-id",
// 			StepName:    "step-name",
// 			Data:        map[string]interface{}{"key": "value"},
// 		}

// 		size := calculateEventSize([]*ExecutionEvent{event})
// 		require.Greater(t, size, int64(0))
// 	})

// 	t.Run("MultipleEvents", func(t *testing.T) {
// 		events := []*ExecutionEvent{
// 			{
// 				ID:          "event-1",
// 				ExecutionID: "exec-1",
// 				StepName:    "step-1",
// 			},
// 			{
// 				ID:          "event-2",
// 				ExecutionID: "exec-1",
// 				StepName:    "step-2",
// 				Data:        map[string]interface{}{"result": "success"},
// 			},
// 		}

// 		size := calculateEventSize(events)
// 		require.Greater(t, size, int64(0))

// 		// Size should increase with more events
// 		singleEventSize := calculateEventSize(events[:1])
// 		require.Greater(t, size, singleEventSize)
// 	})
// }

// func TestGetOriginalExecutionID(t *testing.T) {
// 	t.Run("NoPreservedState", func(t *testing.T) {
// 		originalID := getOriginalExecutionID(nil, "current-exec")
// 		require.Equal(t, "current-exec", originalID)
// 	})

// 	t.Run("NoContinuationInfo", func(t *testing.T) {
// 		preserved := &PreservedState{}
// 		originalID := getOriginalExecutionID(preserved, "current-exec")
// 		require.Equal(t, "current-exec", originalID)
// 	})

// 	t.Run("WithOriginalExecutionID", func(t *testing.T) {
// 		preserved := &PreservedState{
// 			ContinuationInfo: &ContinuationInfo{
// 				OriginalExecutionID: "original-exec",
// 			},
// 		}
// 		originalID := getOriginalExecutionID(preserved, "current-exec")
// 		require.Equal(t, "original-exec", originalID)
// 	})

// 	t.Run("EmptyOriginalExecutionID", func(t *testing.T) {
// 		preserved := &PreservedState{
// 			ContinuationInfo: &ContinuationInfo{
// 				OriginalExecutionID: "",
// 			},
// 		}
// 		originalID := getOriginalExecutionID(preserved, "current-exec")
// 		require.Equal(t, "current-exec", originalID)
// 	})
// }

// func TestContinuationInfo(t *testing.T) {
// 	t.Run("BasicInfo", func(t *testing.T) {
// 		info := &ContinuationInfo{
// 			OriginalExecutionID: "original-exec",
// 			PreviousExecutionID: "previous-exec",
// 			ContinuationCount:   3,
// 			TotalEvents:         30000,
// 			ChainStartTime:      time.Now().Add(-time.Hour),
// 		}

// 		require.Equal(t, "original-exec", info.OriginalExecutionID)
// 		require.Equal(t, "previous-exec", info.PreviousExecutionID)
// 		require.Equal(t, 3, info.ContinuationCount)
// 		require.Equal(t, int64(30000), info.TotalEvents)
// 		require.False(t, info.ChainStartTime.IsZero())
// 	})
// }

// func TestPreservedState(t *testing.T) {
// 	t.Run("CompleteState", func(t *testing.T) {
// 		state := &PreservedState{
// 			ScriptGlobals: map[string]interface{}{
// 				"var1": "value1",
// 				"var2": 42,
// 			},
// 			ActivePaths: []*ReplayPathState{
// 				{
// 					ID:              "path-1",
// 					CurrentStepName: "step-3",
// 					StepOutputs:     map[string]string{"step-1": "output-1"},
// 				},
// 			},
// 			CompletedSteps: map[string]string{
// 				"step-1": "output-1",
// 				"step-2": "output-2",
// 			},
// 			ExecutionInputs: map[string]interface{}{
// 				"input1": "value1",
// 			},
// 			ContinuationInfo: &ContinuationInfo{
// 				OriginalExecutionID: "original",
// 				ContinuationCount:   1,
// 			},
// 		}

// 		require.Len(t, state.ScriptGlobals, 2)
// 		require.Len(t, state.ActivePaths, 1)
// 		require.Len(t, state.CompletedSteps, 2)
// 		require.Len(t, state.ExecutionInputs, 1)
// 		require.NotNil(t, state.ContinuationInfo)
// 		require.Equal(t, "original", state.ContinuationInfo.OriginalExecutionID)
// 	})
// }

// // mockLogger implements the logger interface for testing
// type mockLogger struct {
// 	logs []string
// }

// func (m *mockLogger) Info(msg string, keysAndValues ...interface{}) {
// 	m.logs = append(m.logs, fmt.Sprintf("INFO: %s", msg))
// }

// func (m *mockLogger) Warn(msg string, keysAndValues ...interface{}) {
// 	m.logs = append(m.logs, fmt.Sprintf("WARN: %s", msg))
// }

// func (m *mockLogger) Error(msg string, keysAndValues ...interface{}) {
// 	m.logs = append(m.logs, fmt.Sprintf("ERROR: %s", msg))
// }

// func (m *mockLogger) GetLogs() []string {
// 	return m.logs
// }

// func BenchmarkContinueAsNewEvaluator(b *testing.B) {
// 	logger := &mockLogger{}
// 	evaluator := NewDefaultContinueAsNewEvaluator(logger)

// 	options := DefaultContinueAsNewOptions()
// 	metrics := ContinueAsNewMetrics{
// 		EventCount:        5000,
// 		ExecutionDuration: 12 * time.Hour,
// 		TotalEventSize:    50 * 1024 * 1024,
// 		ActivePaths:       1,
// 		LastEventTime:     time.Now(),
// 	}

// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		_, err := evaluator.ShouldContinueAsNew(context.Background(), metrics, options)
// 		if err != nil {
// 			b.Fatal(err)
// 		}
// 	}
// }

// func BenchmarkCalculateEventSize(b *testing.B) {
// 	// Create test events
// 	events := make([]*ExecutionEvent, 1000)
// 	for i := 0; i < 1000; i++ {
// 		events[i] = &ExecutionEvent{
// 			ID:          fmt.Sprintf("event-%d", i),
// 			ExecutionID: "test-execution",
// 			StepName:    fmt.Sprintf("step-%d", i),
// 			Data:        map[string]interface{}{"index": i, "output": "test output"},
// 		}
// 	}

// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		calculateEventSize(events)
// 	}
// }
