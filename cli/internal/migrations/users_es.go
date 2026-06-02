package migrations

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
)

// MigrateUsersToES intentionally does not import legacy INSTANCE user/account
// keys. User EVT is fresh-only now that durable login, display name, and
// verified-email payloads are encrypted with per-user content keys.
func MigrateUsersToES(
	_ context.Context,
	_ jetstream.KeyValue,
	_ *events.Publisher,
	_ *log.Logger,
) error {
	return nil
}
