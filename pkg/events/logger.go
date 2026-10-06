package events

import "log/slog"

// discardLogger receives diagnostics when a caller supplies no logger.
var discardLogger = slog.New(slog.DiscardHandler)

// normalizeLogger returns logger, or a logger that discards every record when
// logger is nil.
//
// The framework can log caller-provided subjects, projection keys, event IDs,
// stream identities, and handler errors as attributes. Callers must keep those
// values opaque and free of personal data, credentials, tokens, and secrets.
func normalizeLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return discardLogger
	}
	return logger
}
