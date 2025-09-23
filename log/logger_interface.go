package log

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

// SetDefaultLevel sets the default log level for Dive.
func SetDefaultLevel(level Level) {
	defaultLevel = level
}

// GetDefaultLevel returns the default log level for Dive.
func GetDefaultLevel() Level {
	return defaultLevel
}

// Logger defines the interface for logging within Dive. It is intended to
// align with slog package but allow for use with other libraries like zerolog
// by using logging adapters.
type Logger interface {
	// Debug logs a message at debug level with optional key-value pairs
	Debug(msg string, args ...any)

	// Info logs a message at info level with optional key-value pairs
	Info(msg string, args ...any)

	// Warn logs a message at warn level with optional key-value pairs
	Warn(msg string, args ...any)

	// Error logs a message at error level with optional key-value pairs
	Error(msg string, args ...any)

	// With returns a Logger that includes the given attributes in each
	// output operation.
	With(args ...any) Logger
}

// WithLogger returns a new context with the given logger.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, loggerKey, logger)
}

// Ctx returns the logger from the given context.
func Ctx(ctx context.Context) Logger {
	if ctx == nil {
		return New(defaultLevel)
	}
	logger, ok := ctx.Value(loggerKey).(Logger)
	if !ok {
		return New(defaultLevel)
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
