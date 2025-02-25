package slogger

import (
	"log/slog"
	"os"
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
		AddSource:  true,
		NoColor:    !isatty.IsTerminal(os.Stdout.Fd()),
		TimeFormat: time.Kitchen,
		Level:      slog.Level(level),
	})
	return &Slogger{
		logger: slog.New(tintHandler),
	}
}

func (l *Slogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug(msg, keysAndValues...)
}

func (l *Slogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, keysAndValues...)
}

func (l *Slogger) Warn(msg string, keysAndValues ...any) {
	l.logger.Warn(msg, keysAndValues...)
}

func (l *Slogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error(msg, keysAndValues...)
}

func (l *Slogger) With(keysAndValues ...any) Logger {
	return &Slogger{logger: l.logger.With(keysAndValues...)}
}
