package log

// NullLogger implements the Logger interface but does nothing.
type NullLogger struct{}

// NewNullLogger returns a new NullLogger instance
func NewNullLogger() *NullLogger {
	return &NullLogger{}
}

func (l *NullLogger) Debug(msg string, args ...any) {}
func (l *NullLogger) Info(msg string, args ...any)  {}
func (l *NullLogger) Warn(msg string, args ...any)  {}
func (l *NullLogger) Error(msg string, args ...any) {}
func (l *NullLogger) With(args ...any) Logger       { return l }
