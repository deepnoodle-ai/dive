package slogger

import (
	"context"
	"strings"
)

// DefaultLogger is the default logger for the application.
var DefaultLogger = NewDevNullLogger()

// Logger defines the interface for logging within agents.
// It supports structured logging and is designed to be compatible
// with common logging libraries like zerolog and slog.
type Logger interface {
	// Debug logs a message at debug level with optional key-value pairs
	Debug(msg string, keysAndValues ...any)

	// Info logs a message at info level with optional key-value pairs
	Info(msg string, keysAndValues ...any)

	// Warn logs a message at warn level with optional key-value pairs
	Warn(msg string, keysAndValues ...any)

	// Error logs a message at error level with optional key-value pairs
	Error(msg string, keysAndValues ...any)

	// With returns a new Logger instance with the given key-value pairs added to the context
	With(keysAndValues ...any) Logger
}

type contextKey string

const (
	loggerKey contextKey = "dive.logger"
)

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
		return New(DefaultLogLevel)
	}
	logger, ok := ctx.Value(loggerKey).(Logger)
	if !ok {
		return New(DefaultLogLevel)
	}
	return logger
}

// LevelFromString converts a string to a LogLevel.
func LevelFromString(level string) LogLevel {
	value := strings.ToLower(level)
	switch value {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return DefaultLogLevel
	}
}
