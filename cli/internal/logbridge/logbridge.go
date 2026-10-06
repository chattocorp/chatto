// Package logbridge connects Chatto's charmbracelet logger to libraries that
// log through log/slog.
package logbridge

import (
	"context"
	"log/slog"

	"github.com/charmbracelet/log"
)

// Slog returns a slog.Logger that writes through logger, with its level,
// prefix, and formatting. It returns nil for a nil logger, which the shared
// event framework treats as "discard".
func Slog(logger *log.Logger) *slog.Logger {
	if logger == nil {
		return nil
	}
	return slog.New(durationStrings{Handler: logger})
}

// durationStrings writes time.Duration attributes as strings such as "1.5s".
// The charmbracelet JSON formatter writes a slog duration as an integer, but
// writes a time.Duration that Chatto logs directly as a string. Converting
// keeps one format for each field in the same log stream.
type durationStrings struct {
	slog.Handler
}

func (h durationStrings) Handle(ctx context.Context, record slog.Record) error {
	converted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		converted.AddAttrs(durationString(attr))
		return true
	})
	return h.Handler.Handle(ctx, converted)
}

func (h durationStrings) WithAttrs(attrs []slog.Attr) slog.Handler {
	converted := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		converted[i] = durationString(attr)
	}
	return durationStrings{Handler: h.Handler.WithAttrs(converted)}
}

func (h durationStrings) WithGroup(name string) slog.Handler {
	return durationStrings{Handler: h.Handler.WithGroup(name)}
}

func durationString(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindDuration {
		return slog.String(attr.Key, attr.Value.Duration().String())
	}
	return attr
}
