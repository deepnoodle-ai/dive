package agents

import (
	"log/slog"
	"os"
)

// SlogLogger implements the Logger interface using slog
type SlogLogger struct {
	logger *slog.Logger
}

// NewSlogLogger creates a new SlogLogger instance
func NewSlogLogger(logger *slog.Logger) *SlogLogger {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	return &SlogLogger{logger: logger}
}

func (l *SlogLogger) Debug(msg string, keysAndValues ...any) {
	l.logger.Debug(msg, keysAndValues...)
}

func (l *SlogLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, keysAndValues...)
}

func (l *SlogLogger) Warn(msg string, keysAndValues ...any) {
	l.logger.Warn(msg, keysAndValues...)
}

func (l *SlogLogger) Error(msg string, keysAndValues ...any) {
	l.logger.Error(msg, keysAndValues...)
}

func (l *SlogLogger) With(keysAndValues ...any) Logger {
	return &SlogLogger{logger: l.logger.With(keysAndValues...)}
}
