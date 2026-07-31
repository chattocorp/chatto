// Package logging adapts Authling's standard structured logger to narrow
// dependency logging interfaces.
package logging

import (
	"fmt"
	"log/slog"
)

// Events adapts slog.Logger to the shared event framework's logging surface.
type Events struct {
	Logger *slog.Logger
}

func (l Events) Debug(message interface{}, keyvals ...interface{}) {
	l.Logger.Debug(fmt.Sprint(message), keyvals...)
}

func (l Events) Info(message interface{}, keyvals ...interface{}) {
	l.Logger.Info(fmt.Sprint(message), keyvals...)
}

func (l Events) Warn(message interface{}, keyvals ...interface{}) {
	l.Logger.Warn(fmt.Sprint(message), keyvals...)
}

func (l Events) Error(message interface{}, keyvals ...interface{}) {
	l.Logger.Error(fmt.Sprint(message), keyvals...)
}
