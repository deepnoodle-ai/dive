package log

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLogLevel tests the log level conversion functionality
func TestLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Level
	}{
		{"debug level", "debug", LevelDebug},
		{"info level", "info", LevelInfo},
		{"warn level", "warn", LevelWarn},
		{"error level", "error", LevelError},
		{"uppercase", "DEBUG", LevelDebug},
		{"mixed case", "WaRn", LevelWarn},
		{"invalid level", "invalid", defaultLevel},
		{"empty string", "", defaultLevel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			level := LevelFromString(tc.input)
			require.Equal(t, tc.expected, level)
		})
	}
}

// TestDevNullLogger tests the DevNullLogger implementation
func TestDevNullLogger(t *testing.T) {
	logger := NewNullLogger()

	// These calls should not panic
	logger.Debug("debug message", "key", "value")
	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")

	withLogger := logger.With("context", "value")
	require.NotNil(t, withLogger)
	require.IsType(t, &NullLogger{}, withLogger)
}

func TestStructuredLogger(t *testing.T) {
	logger := New(LevelDebug)
	require.NotNil(t, logger)
	require.IsType(t, &StructuredLogger{}, logger)

	// These calls should not panic
	logger.Debug("debug message", "key", "value")
	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "key", "value")
	logger.Error("error message", "key", "value")

	withLogger := logger.With("context", "value")
	require.NotNil(t, withLogger)
	require.IsType(t, &StructuredLogger{}, withLogger)
}

//nolint:staticcheck // SA1012: Intentionally passing nil context for testing
func TestContextFunctions(t *testing.T) {
	// Test with nil context
	logger := NewNullLogger()

	ctx := WithLogger(context.Background(), logger)
	require.NotNil(t, ctx)

	// Test retrieving from context
	retrievedLogger := Ctx(ctx)
	require.NotNil(t, retrievedLogger)
	require.Equal(t, logger, retrievedLogger)

	// Test with existing context
	existingCtx := context.Background()
	newCtx := WithLogger(existingCtx, logger)
	require.NotNil(t, newCtx)
	retrievedLogger = Ctx(newCtx)
	require.Equal(t, logger, retrievedLogger)

	// Test with context but no logger
	emptyCtx := context.Background()
	emptyLogger := Ctx(emptyCtx)
	require.NotNil(t, emptyLogger)
	require.IsType(t, &StructuredLogger{}, emptyLogger)
}

// mockLogger is a simple implementation for testing
type mockLogger struct {
	messages []string
	context  map[string]interface{}
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		messages: []string{},
		context:  make(map[string]interface{}),
	}
}

func (l *mockLogger) Debug(msg string, args ...any) {
	l.messages = append(l.messages, "DEBUG: "+msg)
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.messages = append(l.messages, "INFO: "+msg)
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.messages = append(l.messages, "WARN: "+msg)
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.messages = append(l.messages, "ERROR: "+msg)
}

func (l *mockLogger) With(args ...any) Logger {
	newLogger := newMockLogger()
	newLogger.messages = l.messages
	newLogger.context = l.context

	// Add the key-value pairs to the context
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key, ok := args[i].(string)
			if ok {
				newLogger.context[key] = args[i+1]
			}
		}
	}
	return newLogger
}

func TestMockLogger(t *testing.T) {
	logger := newMockLogger()
	require.NotNil(t, logger)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	require.Equal(t, 4, len(logger.messages))
	require.Equal(t, "DEBUG: debug message", logger.messages[0])
	require.Equal(t, "INFO: info message", logger.messages[1])
	require.Equal(t, "WARN: warn message", logger.messages[2])
	require.Equal(t, "ERROR: error message", logger.messages[3])

	withLogger := logger.With("key", "value", "another", 123)
	require.NotNil(t, withLogger)
	require.IsType(t, &mockLogger{}, withLogger)

	mockLogger, ok := withLogger.(*mockLogger)
	require.True(t, ok)
	require.Equal(t, "value", mockLogger.context["key"])
	require.Equal(t, 123, mockLogger.context["another"])
}
