package log

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

// Level represents the minimum log level
type Level slog.Level

// Available log levels
const (
	LevelDebug Level = Level(slog.LevelDebug)
	LevelInfo  Level = Level(slog.LevelInfo)
	LevelWarn  Level = Level(slog.LevelWarn)
	LevelError Level = Level(slog.LevelError)
)

// StructuredLogger implements the Logger interface using slog
type StructuredLogger struct {
	logger *slog.Logger
}

// New returns a new StructuredLogger instance
func New(level Level) *StructuredLogger {
	tintHandler := tint.NewHandler(os.Stdout, &tint.Options{
		NoColor:    !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: time.Kitchen,
		Level:      slog.Level(level),
	})
	return &StructuredLogger{
		logger: slog.New(tintHandler),
	}
}

func (l *StructuredLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, withCaller(args...)...)
}

func (l *StructuredLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, withCaller(args...)...)
}

func (l *StructuredLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, withCaller(args...)...)
}

func (l *StructuredLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, withCaller(args...)...)
}

func (l *StructuredLogger) With(args ...any) Logger {
	return &StructuredLogger{logger: l.logger.With(args...)}
}

func withCaller(args ...any) []any {
	const callerSkip = 2 // Skip withCaller and the logging function
	if _, file, line, ok := runtime.Caller(callerSkip); ok {
		caller := formatCaller(file, line)
		return append([]any{"caller", caller}, args...)
	}
	return args
}

func formatCaller(file string, line int) string {
	// Take the last two path components for readability
	parts := strings.Split(file, "/")
	switch len(parts) {
	case 0:
		return "unknown"
	case 1:
		return fmt.Sprintf("%s:%d", parts[0], line)
	default:
		return fmt.Sprintf("%s/%s:%d",
			parts[len(parts)-2],
			parts[len(parts)-1],
			line)
	}
}
