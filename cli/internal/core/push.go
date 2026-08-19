package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/pushendpoint"
)

// ============================================================================
// Push Subscription Key Helpers
// ============================================================================

const (
	pushEndpointOwnerKeyPrefix            = "push_endpoint_owner."
	pushTestNotificationThrottleKeyPrefix = "push_test_notification_throttle."
	pushEndpointOwnerMaxRetries           = 8
	pushTestNotificationThrottleTTL       = 10 * time.Second

	// MaxPushSubscriptionsPerUser is the maximum number of active browser
	// subscriptions that one account can fan push delivery out to.
	MaxPushSubscriptionsPerUser = pushendpoint.MaxSubscriptionsPerUser
)

var (
	// ErrPushSubscriptionLimitReached is returned when a new endpoint would
	// exceed the account's active Web Push subscription limit.
	ErrPushSubscriptionLimitReached = errors.New("push subscription limit reached")

	// ErrPushTestNotificationRateLimited is returned when an account requests
	// another test notification inside the shared throttle window.
	ErrPushTestNotificationRateLimited = errors.New("push test notification rate limited")
)

type pushEndpointOwner struct {
	UserID               string `json:"user_id"`
	SubscriptionRevision uint64 `json:"subscription_revision"`
}

// pushSubscriptionKey returns the KV key for a push subscription.
// Format: push_subscription.{userId}.{hash}
// The hash is derived from the endpoint URL to allow multiple devices per user.
func pushSubscriptionKey(userID, endpoint string) string {
	hash := hashEndpoint(endpoint)
	return fmt.Sprintf("push_subscription.%s.%s", userID, hash)
}

// pushSubscriptionKeyFilter returns the NATS subject filter for all push subscriptions for a user.
func pushSubscriptionKeyFilter(userID string) string {
	return "push_subscription." + userID + ".*"
}

// pushEndpointOwnerKey returns the KV key that exclusively assigns a browser
// push endpoint to the account that most recently registered it.
func pushEndpointOwnerKey(endpoint string) string {
	h := sha256.Sum256([]byte(endpoint))
	return pushEndpointOwnerKeyPrefix + hex.EncodeToString(h[:])
}

// hashEndpoint returns a short hash of the endpoint URL for use in the key.
func hashEndpoint(endpoint string) string {
	h := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(h[:8]) // First 8 bytes = 16 hex chars
}

// ============================================================================
// Push Subscription CRUD Operations
// ============================================================================

// SavePushSubscription stores or updates a push subscription for a user.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) SavePushSubscription(
	ctx context.Context,
	userID string,
	endpoint, p256dh, auth, userAgent string,
) (*corev1.PushSubscription, error) {
	if err := validatePushSubscription(endpoint, p256dh, auth, userAgent); err != nil {
		return nil, err
	}
	if err := c.requirePushSubscriptionAccountActive(ctx, userID); err != nil {
		return nil, err
	}
	if err := c.checkPushSubscriptionCapacity(ctx, userID, endpoint); err != nil {
		return nil, err
	}

	subscription := &corev1.PushSubscription{
		Endpoint:  endpoint,
		P256Dh:    p256dh,
		Auth:      auth,
		CreatedAt: timestamppb.New(time.Now()),
		UserAgent: userAgent,
	}

	data, err := proto.Marshal(subscription)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal push subscription: %w", err)
	}

	key := pushSubscriptionKey(userID, endpoint)
	_, err = c.storage.runtimeStateKV.Put(ctx, key, data)
	if err != nil {
		return nil, fmt.Errorf("failed to store push subscription: %w", err)
	}
	if err := c.claimPushEndpointOwnership(ctx, userID, endpoint); err != nil {
		return nil, err
	}
	if err := c.requirePushSubscriptionAccountActive(ctx, userID); err != nil {
		_, cleanupErr := c.DeleteAllUserPushSubscriptions(ctx, userID)
		if cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("remove push subscription written across account deletion: %w", cleanupErr))
		}
		return nil, err
	}

	c.logger.Debug("Push subscription saved",
		"user_id", userID,
		"endpoint_hash", hashEndpoint(endpoint))

	return subscription, nil
}

func (c *ChattoCore) requirePushSubscriptionAccountActive(ctx context.Context, userID string) error {
	if _, err := c.storage.runtimeStateKV.Get(ctx, pushSubscriptionDeletionFenceKey(userID)); err == nil {
		return fmt.Errorf("push-subscription account is deleted: %w", ErrNotFound)
	} else if !isPushRuntimeStateKeyAbsent(err) {
		return fmt.Errorf("check push-subscription account-deletion fence: %w", err)
	}
	deletedSubject := evtstream.UserAggregate(userID).Subject(evtstream.EventUserAccountDeleted)
	sequence, err := c.EventPublisher.LastSubjectSeq(ctx, deletedSubject)
	if err != nil {
		return fmt.Errorf("check push-subscription account state: %w", err)
	}
	if sequence > 0 {
		return fmt.Errorf("push-subscription account is deleted: %w", ErrNotFound)
	}
	return nil
}

func (c *ChattoCore) checkPushSubscriptionCapacity(ctx context.Context, userID, endpoint string) error {
	subscriptions, err := c.GetUserPushSubscriptions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check push subscription capacity: %w", err)
	}
	for _, subscription := range subscriptions {
		if subscription.Endpoint == endpoint {
			return nil
		}
	}
	if len(subscriptions) >= MaxPushSubscriptionsPerUser {
		return ErrPushSubscriptionLimitReached
	}
	return nil
}

// AdmitPushTestNotification reserves the per-account test-notification window
// in shared runtime state so concurrent replicas enforce one limit.
func (c *ChattoCore) AdmitPushTestNotification(ctx context.Context, userID string) error {
	key := pushTestNotificationThrottleKeyPrefix + userID
	_, err := c.storage.runtimeStateKV.Create(ctx, key, []byte{1}, jetstream.KeyTTL(pushTestNotificationThrottleTTL))
	if jetstreamutil.IsSequenceConflict(err) {
		return ErrPushTestNotificationRateLimited
	}
	if err != nil {
		return fmt.Errorf("failed to reserve push test notification window: %w", err)
	}
	return nil
}

func isPushRuntimeStateKeyAbsent(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

func (c *ChattoCore) claimPushEndpointOwnership(ctx context.Context, userID, endpoint string) error {
	ownerKey := pushEndpointOwnerKey(endpoint)
	subscriptionKey := pushSubscriptionKey(userID, endpoint)
	for range pushEndpointOwnerMaxRetries {
		subscriptionEntry, err := c.storage.runtimeStateKV.Get(ctx, subscriptionKey)
		if isPushRuntimeStateKeyAbsent(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get current push subscription: %w", err)
		}

		owner := pushEndpointOwner{UserID: userID, SubscriptionRevision: subscriptionEntry.Revision()}
		value, err := json.Marshal(owner)
		if err != nil {
			return fmt.Errorf("failed to marshal push endpoint owner: %w", err)
		}
		entry, err := c.storage.runtimeStateKV.Get(ctx, ownerKey)
		if isPushRuntimeStateKeyAbsent(err) {
			if ownerRevision, err := c.storage.runtimeStateKV.Create(ctx, ownerKey, value); err == nil {
				if current, err := c.storage.runtimeStateKV.Get(ctx, subscriptionKey); err == nil && current.Revision() == owner.SubscriptionRevision {
					return nil
				}
				if err := c.deleteStalePushEndpointOwner(ctx, ownerKey, ownerRevision); err != nil {
					return err
				}
				continue
			} else if jetstreamutil.IsSequenceConflict(err) {
				continue
			} else {
				return fmt.Errorf("failed to create push endpoint owner: %w", err)
			}
		}
		if err != nil {
			return fmt.Errorf("failed to get push endpoint owner: %w", err)
		}
		var current pushEndpointOwner
		if err := json.Unmarshal(entry.Value(), &current); err != nil {
			return fmt.Errorf("failed to unmarshal push endpoint owner: %w", err)
		}
		if current == owner {
			if latest, err := c.storage.runtimeStateKV.Get(ctx, subscriptionKey); err == nil && latest.Revision() == owner.SubscriptionRevision {
				return nil
			}
			if err := c.deleteStalePushEndpointOwner(ctx, ownerKey, entry.Revision()); err != nil {
				return err
			}
			continue
		}
		if ownerRevision, err := c.storage.runtimeStateKV.Update(ctx, ownerKey, value, entry.Revision()); err == nil {
			if latest, err := c.storage.runtimeStateKV.Get(ctx, subscriptionKey); err == nil && latest.Revision() == owner.SubscriptionRevision {
				return nil
			}
			if err := c.deleteStalePushEndpointOwner(ctx, ownerKey, ownerRevision); err != nil {
				return err
			}
			continue
		} else if jetstreamutil.IsSequenceConflict(err) {
			continue
		} else {
			return fmt.Errorf("failed to update push endpoint owner: %w", err)
		}
	}
	return fmt.Errorf("failed to claim push endpoint ownership after %d concurrent updates", pushEndpointOwnerMaxRetries)
}

func (c *ChattoCore) deleteStalePushEndpointOwner(ctx context.Context, key string, revision uint64) error {
	err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(revision))
	if err == nil || isPushRuntimeStateKeyAbsent(err) || jetstreamutil.IsSequenceConflict(err) {
		return nil
	}
	return fmt.Errorf("failed to remove stale push endpoint owner: %w", err)
}

// PushSubscriptionOwnedByUser reports whether the endpoint is currently claimed
// by userID.
func (c *ChattoCore) PushSubscriptionOwnedByUser(ctx context.Context, userID, endpoint string) (bool, error) {
	owner, err := c.getPushEndpointOwner(ctx, endpoint)
	if err != nil {
		return false, err
	}
	return owner != nil && owner.UserID == userID, nil
}

// PushSubscriptionCurrentForUser reports whether subscription is still the
// exact active record for userID. Callers should recheck this immediately before
// delivery because browsers can transfer or rotate a subscription while a push
// is being prepared.
func (c *ChattoCore) PushSubscriptionCurrentForUser(ctx context.Context, userID string, subscription *corev1.PushSubscription) (bool, error) {
	key := pushSubscriptionKey(userID, subscription.Endpoint)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if isPushRuntimeStateKeyAbsent(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get push subscription: %w", err)
	}

	var current corev1.PushSubscription
	if err := proto.Unmarshal(entry.Value(), &current); err != nil {
		return false, fmt.Errorf("failed to unmarshal push subscription: %w", err)
	}
	if !proto.Equal(&current, subscription) {
		return false, nil
	}
	return c.pushSubscriptionRevisionOwnedByUser(ctx, userID, subscription.Endpoint, entry.Revision())
}

func (c *ChattoCore) getPushEndpointOwner(ctx context.Context, endpoint string) (*pushEndpointOwner, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, pushEndpointOwnerKey(endpoint))
	if isPushRuntimeStateKeyAbsent(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get push endpoint owner: %w", err)
	}
	var owner pushEndpointOwner
	if err := json.Unmarshal(entry.Value(), &owner); err != nil {
		return nil, fmt.Errorf("failed to unmarshal push endpoint owner: %w", err)
	}
	return &owner, nil
}

func (c *ChattoCore) pushSubscriptionRevisionOwnedByUser(ctx context.Context, userID, endpoint string, subscriptionRevision uint64) (bool, error) {
	owner, err := c.getPushEndpointOwner(ctx, endpoint)
	if err != nil {
		return false, err
	}
	return owner != nil && owner.UserID == userID && owner.SubscriptionRevision == subscriptionRevision, nil
}

func (c *ChattoCore) releasePushEndpointOwnership(ctx context.Context, userID, endpoint string, subscriptionRevision uint64) error {
	key := pushEndpointOwnerKey(endpoint)
	for range pushEndpointOwnerMaxRetries {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if isPushRuntimeStateKeyAbsent(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get push endpoint owner: %w", err)
		}
		var owner pushEndpointOwner
		if err := json.Unmarshal(entry.Value(), &owner); err != nil {
			return fmt.Errorf("failed to unmarshal push endpoint owner: %w", err)
		}
		if owner.UserID != userID || owner.SubscriptionRevision != subscriptionRevision {
			return nil
		}
		err = c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		if err == nil || isPushRuntimeStateKeyAbsent(err) {
			return nil
		}
		if jetstreamutil.IsSequenceConflict(err) {
			continue
		}
		return fmt.Errorf("failed to delete push endpoint owner: %w", err)
	}
	return fmt.Errorf("failed to release push endpoint ownership after %d concurrent updates", pushEndpointOwnerMaxRetries)
}

func validatePushSubscription(endpoint, p256dh, auth, userAgent string) error {
	if err := validateStringMaxLength("push endpoint", endpoint, MaxPushEndpointLength); err != nil {
		return err
	}
	if err := pushendpoint.Validate(endpoint); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if err := validateStringMaxLength("push p256dh key", p256dh, MaxPushKeyLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("push auth secret", auth, MaxPushAuthLength); err != nil {
		return err
	}
	if err := validateStringMaxLength("push user agent", userAgent, MaxPushUserAgentLength); err != nil {
		return err
	}
	return nil
}

// DeletePushSubscription removes a push subscription by endpoint.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) DeletePushSubscription(ctx context.Context, userID, endpoint string) error {
	key := pushSubscriptionKey(userID, endpoint)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil && !isPushRuntimeStateKeyAbsent(err) {
		return fmt.Errorf("failed to get push subscription before deleting: %w", err)
	}

	if entry != nil {
		if err := c.releasePushEndpointOwnership(ctx, userID, endpoint, entry.Revision()); err != nil {
			return err
		}
	}

	if entry != nil {
		err = c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		if err != nil && !isPushRuntimeStateKeyAbsent(err) && !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("failed to delete push subscription: %w", err)
		}
	}

	c.logger.Debug("Push subscription deleted",
		"user_id", userID,
		"endpoint_hash", hashEndpoint(endpoint))

	return nil
}

// GetUserPushSubscriptions returns all push subscriptions for a user.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) GetUserPushSubscriptions(ctx context.Context, userID string) ([]*corev1.PushSubscription, error) {
	prefix := pushSubscriptionKeyFilter(userID)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return []*corev1.PushSubscription{}, nil
		}
		return nil, fmt.Errorf("failed to list push subscription keys: %w", err)
	}

	var subscriptions []*corev1.PushSubscription
	for key := range lister.Keys() {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get push subscription %s: %w", key, err)
		}

		var sub corev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &sub); err != nil {
			return nil, fmt.Errorf("failed to unmarshal push subscription %s: %w", key, err)
		}
		owned, err := c.pushSubscriptionRevisionOwnedByUser(ctx, userID, sub.Endpoint, entry.Revision())
		if err != nil {
			return nil, err
		}
		if !owned {
			continue
		}
		subscriptions = append(subscriptions, &sub)
	}

	return subscriptions, nil
}

// DeleteAllUserPushSubscriptions removes all push subscriptions for a user.
// Used when a user account is deleted.
// Authorization: Internal use only - called from user deletion flow.
func (c *ChattoCore) DeleteAllUserPushSubscriptions(ctx context.Context, userID string) (int, error) {
	prefix := pushSubscriptionKeyFilter(userID)
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
		return 0, fmt.Errorf("failed to list push subscription keys: %w", err)
	}

	// Collect keys first to avoid modifying while iterating
	var keys []string
	if lister != nil {
		for key := range lister.Keys() {
			keys = append(keys, key)
		}
	}

	deleted := 0
	var cleanupErrors []error
	for _, key := range keys {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if isPushRuntimeStateKeyAbsent(err) {
			continue
		}
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("get push subscription %s before deleting: %w", key, err))
			continue
		}

		var sub corev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &sub); err != nil {
			// The raw record may still contain credentials even when an older or
			// damaged payload cannot be decoded. Erase it first; the owner scan
			// below removes any content-free ownership record that still names the
			// deleted account.
			deleteErr := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
			if deleteErr == nil || isPushRuntimeStateKeyAbsent(deleteErr) {
				deleted++
				cleanupErrors = append(cleanupErrors, fmt.Errorf("decode push subscription %s during deletion: %w", key, err))
				continue
			}
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete undecodable push subscription %s: %w", key, deleteErr))
			continue
		}
		if err := c.releasePushEndpointOwnership(ctx, userID, sub.Endpoint, entry.Revision()); err != nil {
			// Retain the subscription so redelivery can still recover its endpoint
			// and retry the owner-first deletion ordering.
			cleanupErrors = append(cleanupErrors, fmt.Errorf("release push endpoint owner for %s: %w", key, err))
			continue
		}

		err = c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		if err != nil && !isPushRuntimeStateKeyAbsent(err) {
			// A revision conflict means a concurrent registration replaced the
			// credentials. Report it so the durable delivery retries and removes
			// the newer exact revision rather than acknowledging partial cleanup.
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete push subscription %s: %w", key, err))
			continue
		}
		if err == nil {
			deleted++
		}
	}

	if err := c.deletePushEndpointOwnersForUser(ctx, userID); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}

	c.logger.Debug("Deleted all push subscriptions for user",
		"user_id", userID,
		"count", deleted)

	return deleted, errors.Join(cleanupErrors...)
}

func (c *ChattoCore) deletePushEndpointOwnersForUser(ctx context.Context, userID string) error {
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, pushEndpointOwnerKeyPrefix+">")
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list push endpoint owners during account cleanup: %w", err)
	}
	var cleanupErrors []error
	for key := range lister.Keys() {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if isPushRuntimeStateKeyAbsent(err) {
			continue
		}
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("get push endpoint owner during account cleanup: %w", err))
			continue
		}
		var owner pushEndpointOwner
		if err := json.Unmarshal(entry.Value(), &owner); err != nil {
			// A malformed owner record is unusable for delivery or future claims.
			// Remove its exact revision, but return the decode failure once so the
			// durable worker retries any subscription retained when owner release
			// encountered the same record earlier in this pass.
			deleteErr := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
			if deleteErr != nil && !isPushRuntimeStateKeyAbsent(deleteErr) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("delete undecodable push endpoint owner: %w", deleteErr))
				continue
			}
			cleanupErrors = append(cleanupErrors, fmt.Errorf("decode push endpoint owner during account cleanup: %w", err))
			continue
		}
		if owner.UserID != userID {
			continue
		}
		if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil && !isPushRuntimeStateKeyAbsent(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete push endpoint owner during account cleanup: %w", err))
		}
	}
	return errors.Join(cleanupErrors...)
}

// GetAllPushSubscriptions returns all push subscriptions in the system.
// Authorization: Internal use only.
//
// NOTE: Currently unused. Reserved for future admin dashboard feature to list
// all push subscriptions for monitoring/debugging purposes.
func (c *ChattoCore) GetAllPushSubscriptions(ctx context.Context) ([]*PushSubscriptionWithUser, error) {
	prefix := "push_subscription.>"
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return []*PushSubscriptionWithUser{}, nil
		}
		return nil, fmt.Errorf("failed to list push subscription keys: %w", err)
	}

	var subscriptions []*PushSubscriptionWithUser
	for key := range lister.Keys() {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Failed to get push subscription", "key", key, "error", err)
			continue
		}

		var sub corev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &sub); err != nil {
			c.logger.Warn("Failed to unmarshal push subscription", "key", key, "error", err)
			continue
		}

		// Extract userID from key: push_subscription.{userId}.{hash}
		userID := extractUserIDFromPushKey(key)
		if userID == "" {
			continue
		}
		owned, err := c.pushSubscriptionRevisionOwnedByUser(ctx, userID, sub.Endpoint, entry.Revision())
		if err != nil {
			return nil, err
		}
		if !owned {
			continue
		}

		subscriptions = append(subscriptions, &PushSubscriptionWithUser{
			UserID:       userID,
			Subscription: &sub,
		})
	}

	return subscriptions, nil
}

// PushSubscriptionWithUser pairs a subscription with its owner's user ID.
type PushSubscriptionWithUser struct {
	UserID       string
	Subscription *corev1.PushSubscription
}

// extractUserIDFromPushKey extracts the user ID from a push subscription key.
// Key format: push_subscription.{userId}.{hash}
func extractUserIDFromPushKey(key string) string {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "push_subscription" {
		return ""
	}
	return parts[1]
}
