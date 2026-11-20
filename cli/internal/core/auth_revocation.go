package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
)

const runtimeCredentialCleanupConsumerName = "chatto-runtime-credential-cleanup-v1"

// Replay retained deletions on first start to cover accounts whose earlier
// best-effort cleanup did not finish. Authentication uses the auth generation
// independently of the physical deletion of these records.
func (c *ChattoCore) configureRuntimeCredentialCleanup(ctx context.Context) error {
	stream := c.storage.serverEvtStream
	consumer, err := evtstream.CreateEffectConsumer(ctx, stream, evtstream.EffectConsumerConfig{
		Name:           runtimeCredentialCleanupConsumerName,
		Description:    "Hand off runtime credential cleanup for deleted accounts",
		FilterSubjects: []string{evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted)},
		AckWait:        2 * time.Minute,
		MaxAckPending:  16,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return err
	}
	_, err = c.newEffectJob(jobTypeRuntimeCredentialCleanup, stream, consumer, c.cleanDeletedAccountCredentials, jobqueue.EffectOptions{
		MaxAttempts: jobqueue.UnlimitedAttempts,
		RetryDelay:  30 * time.Second,
	})
	return err
}

func (c *ChattoCore) cleanDeletedAccountCredentials(ctx context.Context, delivery events.DurableDelivery) error {
	event, err := decodeDurableCoreDelivery(delivery)
	if err != nil {
		return err
	}
	userID := event.GetUserAccountDeleted().GetUserId()
	if userID == "" || delivery.Subject != evtstream.UserAggregate(userID).Subject(evtstream.EventUserAccountDeleted) {
		return events.TerminateDelivery("invalid runtime credential cleanup fact", errors.New("account-deletion subject and payload do not match"))
	}
	// Remove renewal authorities before their access records. Both stored
	// credential formats carry the same user_id, including older records.
	for _, prefix := range []string{renewableSessionKeyPrefix, authTokenKeyPrefix} {
		err := visitRuntimeStateSnapshot(ctx, c.storage.runtimeStateKV, []string{prefix + "*"}, func(entry jetstream.KeyValueEntry) error {
			var owner struct {
				UserID string `json:"user_id"`
			}
			if json.Unmarshal(entry.Value(), &owner) != nil || owner.UserID != userID {
				return nil
			}
			// Delete the revision whose owner was checked. Retry from a fresh
			// snapshot if another process has updated or replaced that record.
			err := c.storage.runtimeStateKV.Delete(ctx, entry.Key(), jetstream.LastRevision(entry.Revision()))
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if prefix == renewableSessionKeyPrefix {
				if err := c.recordBearerTokenRevoked(ctx, userID, "account_deleted"); err != nil {
					c.logger.Warn("Failed to append bearer-token revocation audit event", "error", err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
