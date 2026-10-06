// Package logbridge connects Chatto's charmbracelet logger to libraries that
// log through log/slog.
package logbridge

import (
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
	return slog.New(logger)
}
