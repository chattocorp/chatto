// Package migrations holds one-shot data migrations that run at boot.
//
// # Why this package exists
//
// Chatto's storage layer evolves over time, and occasionally the shape of
// data already persisted in NATS KV needs to change (e.g. JSON → proto,
// one-blob-per-user → one-entry-per-thing). Each such transition has a
// migration function in this package that:
//
//  1. Runs idempotently at boot from ChattoCore initialisation.
//  2. Scans for legacy storage keys, converts them to the new shape, and
//     deletes the legacy entries.
//  3. Short-circuits when no legacy keys remain, so it costs essentially
//     nothing on subsequent boots.
//
// Migrations live here, not next to the feature code, so there is a
// single place to audit "what code do we keep around purely to convert
// legacy data?". When a migration's legacy data is known to have been
// drained from all live deployments, the migration can be deleted in
// one tightly scoped PR.
//
// # Adding a migration
//
//  1. Add a file `xxx.go` in this package with a function taking
//     whatever it needs (typically a KV bucket and a logger). Document
//     the legacy shape, the new shape, and the reason for the change in
//     a doc comment.
//  2. Call it from [RunAll].
//
// # Removing a migration
//
// Once the legacy data has been confirmed drained on every live
// deployment that will ever boot this code, delete the file and the
// corresponding call in [RunAll].
package migrations

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

// RunAll runs every active migration in order, stopping at the first
// error.
func RunAll(ctx context.Context, kv jetstream.KeyValue, logger *log.Logger) error {
	if err := MigrateVerifiedEmailsToProto(ctx, kv, logger); err != nil {
		return fmt.Errorf("verified_emails: %w", err)
	}
	return nil
}
