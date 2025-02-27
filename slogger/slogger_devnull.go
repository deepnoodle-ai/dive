package slogger

// DevNullLogger implements the Logger interface but does nothing.
type DevNullLogger struct{}

// NewDevNullLogger returns a new DevNullLogger instance
func NewDevNullLogger() *DevNullLogger {
	return &DevNullLogger{}
}

func (l *DevNullLogger) Debug(msg string, keysAndValues ...any) {}
func (l *DevNullLogger) Info(msg string, keysAndValues ...any)  {}
func (l *DevNullLogger) Warn(msg string, keysAndValues ...any)  {}
func (l *DevNullLogger) Error(msg string, keysAndValues ...any) {}
func (l *DevNullLogger) With(keysAndValues ...any) Logger       { return l }
