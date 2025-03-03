package slogger

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

var (
	DefaultLogLevel = LevelInfo
)

// LogLevel represents the minimum log level
type LogLevel slog.Level

// Available log levels
const (
	LevelDebug LogLevel = LogLevel(slog.LevelDebug)
	LevelInfo  LogLevel = LogLevel(slog.LevelInfo)
	LevelWarn  LogLevel = LogLevel(slog.LevelWarn)
	LevelError LogLevel = LogLevel(slog.LevelError)
)

// Slogger implements the Logger interface using slog
type Slogger struct {
	logger *slog.Logger
}

// New returns a new Slogger instance
func New(level LogLevel) *Slogger {
	tintHandler := tint.NewHandler(os.Stdout, &tint.Options{
		NoColor:    !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: time.Kitchen,
		Level:      slog.Level(level),
	})
	return &Slogger{
		logger: slog.New(tintHandler),
	}
}

func (l *Slogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug(msg, withCaller(keysAndValues...)...)
}

func (l *Slogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, withCaller(keysAndValues...)...)
}

func (l *Slogger) Warn(msg string, keysAndValues ...any) {
	l.logger.Warn(msg, withCaller(keysAndValues...)...)
}

func (l *Slogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error(msg, withCaller(keysAndValues...)...)
}

func (l *Slogger) With(keysAndValues ...any) Logger {
	return &Slogger{logger: l.logger.With(keysAndValues...)}
}

func withCaller(keysAndValues ...any) []any {
	const callerSkip = 2 // Skip withCaller and the logging function
	if _, file, line, ok := runtime.Caller(callerSkip); ok {
		caller := formatCaller(file, line)
		return append([]any{"caller", caller}, keysAndValues...)
	}
	return keysAndValues
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
