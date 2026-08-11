package events

// Logger is the small logging surface used by event-sourcing mechanics.
// *log.Logger from github.com/charmbracelet/log satisfies it. Constructors
// accept a nil Logger and replace it with a no-op logger. Implementations may
// receive caller-provided subjects, projection keys, event IDs, stream
// identities, and handler errors as diagnostic fields; callers must keep those
// values opaque and free of personal data, credentials, tokens, and secrets.
type Logger interface {
	Debug(msg interface{}, keyvals ...interface{})
	Info(msg interface{}, keyvals ...interface{})
	Warn(msg interface{}, keyvals ...interface{})
	Error(msg interface{}, keyvals ...interface{})
}

type noopLogger struct{}

func (noopLogger) Debug(interface{}, ...interface{}) {}
func (noopLogger) Info(interface{}, ...interface{})  {}
func (noopLogger) Warn(interface{}, ...interface{})  {}
func (noopLogger) Error(interface{}, ...interface{}) {}

func normalizeLogger(logger Logger) Logger {
	if logger == nil {
		return noopLogger{}
	}
	return logger
}
