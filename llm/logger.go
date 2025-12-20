package llm

import (
	"context"
	golog "log"
	"strings"
)

type contextKey string

const (
	loggerKey contextKey = "dive.logger"
)

var defaultLevel = LevelWarn

// SetDefaultLogLevel sets the default log level for Dive.
func SetDefaultLogLevel(level Level) {
	defaultLevel = level
}

// GetDefaultLogLevel returns the default log level for Dive.
func GetDefaultLogLevel() Level {
	return defaultLevel
}

// Logger defines the interface for logging within Dive.
// This interface is designed to be compatible with slog and other structured logging libraries.
type Logger interface {
	// Debug logs a message at debug level with optional key-value pairs
	Debug(msg string, args ...any)

	// Info logs a message at info level with optional key-value pairs
	Info(msg string, args ...any)

	// Warn logs a message at warn level with optional key-value pairs
	Warn(msg string, args ...any)

	// Error logs a message at error level with optional key-value pairs
	Error(msg string, args ...any)

	// With returns a Logger that includes the given attributes in each output operation
	With(args ...any) Logger
}

// Level represents the minimum log level
type Level int

// Available log levels
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ContextWithLogger returns a new context with the given logger.
func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFromContext returns the logger from the given context.
// If no logger is set, it returns a NullLogger.
func LoggerFromContext(ctx context.Context) Logger {
	if ctx == nil {
		return &NullLogger{}
	}
	logger, ok := ctx.Value(loggerKey).(Logger)
	if !ok {
		return &NullLogger{}
	}
	return logger
}

// LevelFromString converts a string to a LogLevel.
func LevelFromString(value string) Level {
	switch strings.ToLower(value) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return defaultLevel
	}
}

// Fatal wraps the standard library log.Fatal function.
func Fatal(args ...any) {
	golog.Fatal(args...)
}

// NullLogger implements the Logger interface but does nothing.
// This is useful for testing or when you want to disable logging.
type NullLogger struct{}

func (l *NullLogger) Debug(msg string, args ...any) {}
func (l *NullLogger) Info(msg string, args ...any)  {}
func (l *NullLogger) Warn(msg string, args ...any)  {}
func (l *NullLogger) Error(msg string, args ...any) {}
func (l *NullLogger) With(args ...any) Logger       { return l }
