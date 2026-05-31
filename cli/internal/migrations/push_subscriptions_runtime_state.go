package migrations

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

const legacyPushSubscriptionPrefix = "push_subscription."

// MigratePushSubscriptionsToRuntimeState copies Web Push subscription records
// from the legacy INSTANCE bucket into RUNTIME_STATE.
//
// Legacy and new keys use the same shape:
//
//	push_subscription.{userId}.{endpointHash}
//
// The old keys are intentionally retained for rollback and phase-7 cleanup.
// Destination writes use Create so this migration never overwrites newer
// RUNTIME_STATE subscriptions written by a previous boot or a live process.
func MigratePushSubscriptionsToRuntimeState(ctx context.Context, serverKV, runtimeState jetstream.KeyValue, logger *log.Logger) error {
	copied, skipped, err := copyRuntimeStatePrefix(ctx, serverKV, runtimeState, legacyPushSubscriptionPrefix, legacyPushSubscriptionPrefix)
	if err != nil {
		return fmt.Errorf("push subscriptions: %w", err)
	}
	if copied+skipped > 0 {
		logger.Info("push subscription migration: copied legacy subscriptions into RUNTIME_STATE",
			"copied", copied,
			"skipped", skipped)
	}
	return nil
}
