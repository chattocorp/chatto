package events

// Logger is the small logging surface used by event-sourcing mechanics.
// *log.Logger from github.com/charmbracelet/log satisfies it.
type Logger interface {
	Debug(msg interface{}, keyvals ...interface{})
	Info(msg interface{}, keyvals ...interface{})
	Warn(msg interface{}, keyvals ...interface{})
	Error(msg interface{}, keyvals ...interface{})
}
