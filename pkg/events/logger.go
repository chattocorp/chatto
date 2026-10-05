package events

// Logger is the small logging surface used by event-sourcing mechanics.
// *log.Logger from github.com/charmbracelet/log satisfies it. Constructors
// accept a nil Logger and replace it with a no-op logger. Implementations may
// receive caller-provided subjects, projection keys, event IDs, stream
// identities, and handler errors as diagnostic fields; callers must keep those
// values opaque and free of personal data, credentials, tokens, and secrets.
type Logger interface {
	Debug(msg any, keyvals ...any)
	Info(msg any, keyvals ...any)
	Warn(msg any, keyvals ...any)
	Error(msg any, keyvals ...any)
}

type noopLogger struct{}

func (noopLogger) Debug(any, ...any) {}
func (noopLogger) Info(any, ...any)  {}
func (noopLogger) Warn(any, ...any)  {}
func (noopLogger) Error(any, ...any) {}

func normalizeLogger(logger Logger) Logger {
	if logger == nil {
		return noopLogger{}
	}
	return logger
}
