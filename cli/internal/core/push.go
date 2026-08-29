package core

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/jetstreamutil"
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
) (*runtimestatev1.PushSubscription, error) {
	return c.savePushSubscriptionForClient(ctx, userID, endpoint, p256dh, auth, userAgent, "", "")
}

// SavePushSubscriptionWithCleanupToken stores a subscription with a capability
// that identifies only this save generation.
func (c *ChattoCore) SavePushSubscriptionWithCleanupToken(
	ctx context.Context,
	userID string,
	endpoint, p256dh, auth, userAgent, cleanupToken string,
) (*runtimestatev1.PushSubscription, error) {
	if err := validatePushCleanupToken(cleanupToken); err != nil {
		return nil, err
	}
	return c.savePushSubscriptionForClient(ctx, userID, endpoint, p256dh, auth, userAgent, "", cleanupToken)
}

// SavePushSubscriptionForClient stores or updates a browser subscription with
// the URL host of its serving Chatto client.
func (c *ChattoCore) SavePushSubscriptionForClient(
	ctx context.Context,
	userID string,
	endpoint, p256dh, auth, userAgent, clientHost string,
) (*runtimestatev1.PushSubscription, error) {
	return c.savePushSubscriptionForClient(ctx, userID, endpoint, p256dh, auth, userAgent, clientHost, "")
}

// SavePushSubscriptionForClientWithCleanupToken stores a host-aware
// subscription with a capability that identifies only this save generation.
func (c *ChattoCore) SavePushSubscriptionForClientWithCleanupToken(
	ctx context.Context,
	userID string,
	endpoint, p256dh, auth, userAgent, clientHost, cleanupToken string,
) (*runtimestatev1.PushSubscription, error) {
	if err := validatePushCleanupToken(cleanupToken); err != nil {
		return nil, err
	}
	return c.savePushSubscriptionForClient(ctx, userID, endpoint, p256dh, auth, userAgent, clientHost, cleanupToken)
}

func (c *ChattoCore) savePushSubscriptionForClient(
	ctx context.Context,
	userID string,
	endpoint, p256dh, auth, userAgent, clientHost, cleanupToken string,
) (*runtimestatev1.PushSubscription, error) {
	if err := validatePushSubscription(endpoint, p256dh, auth, userAgent, clientHost, cleanupToken); err != nil {
		return nil, err
	}
	if err := c.requirePushSubscriptionAccountActive(ctx, userID); err != nil {
		return nil, err
	}
	if err := c.checkPushSubscriptionCapacity(ctx, userID, endpoint); err != nil {
		return nil, err
	}

	subscription := &runtimestatev1.PushSubscription{
		Endpoint:     endpoint,
		P256Dh:       p256dh,
		Auth:         auth,
		CreatedAt:    timestamppb.New(time.Now()),
		UserAgent:    userAgent,
		ClientHost:   clientHost,
		CleanupToken: cleanupToken,
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
		if subscription.GetEndpoint() == endpoint {
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

// listPushRuntimeStateKeys waits for JetStream's explicit initial-snapshot
// sentinel. The newer jetstream.KeyLister API closes its only channel both on
// completion and on interruption, so it cannot distinguish a complete list
// from a partial one. Cleanup callers must not acknowledge physical erasure
// after seeing only a prefix of the matching keys.
func listPushRuntimeStateKeys(ctx context.Context, kv jetstream.KeyValue, filters ...string) ([]string, error) {
	watcher, err := kv.WatchFiltered(ctx, filters, jetstream.IgnoreDeletes(), jetstream.MetaOnly())
	if err != nil {
		return nil, err
	}
	defer func() { _ = watcher.Stop() }()

	var keys []string
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case entry, ok := <-watcher.Updates():
			if !ok {
				return nil, errors.New("push runtime-state key snapshot ended before completion")
			}
			if entry == nil {
				return keys, nil
			}
			keys = append(keys, entry.Key())
		}
	}
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

		var currentSubscription runtimestatev1.PushSubscription
		if err := proto.Unmarshal(subscriptionEntry.Value(), &currentSubscription); err != nil {
			return fmt.Errorf("failed to unmarshal current push subscription: %w", err)
		}
		if currentSubscription.GetEndpoint() != endpoint {
			return nil
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
func (c *ChattoCore) PushSubscriptionCurrentForUser(ctx context.Context, userID string, subscription *runtimestatev1.PushSubscription) (bool, error) {
	endpoint := subscription.GetEndpoint()
	key := pushSubscriptionKey(userID, endpoint)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if isPushRuntimeStateKeyAbsent(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to get push subscription: %w", err)
	}

	var current runtimestatev1.PushSubscription
	if err := proto.Unmarshal(entry.Value(), &current); err != nil {
		return false, fmt.Errorf("failed to unmarshal push subscription: %w", err)
	}
	if !proto.Equal(&current, subscription) {
		return false, nil
	}
	return c.pushSubscriptionRevisionOwnedByUser(ctx, userID, endpoint, entry.Revision())
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

func validatePushSubscription(endpoint, p256dh, auth, userAgent, clientHost, cleanupToken string) error {
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
	if err := validatePushClientHost(clientHost); err != nil {
		return err
	}
	if cleanupToken != "" {
		if err := validatePushCleanupToken(cleanupToken); err != nil {
			return err
		}
	}
	return nil
}

func validatePushCleanupToken(value string) error {
	if len(value) < MinPushCleanupTokenLength {
		return fmt.Errorf("%w: push cleanup token must be at least %d bytes", ErrInvalidArgument, MinPushCleanupTokenLength)
	}
	return validateStringMaxLength("push cleanup token", value, MaxPushCleanupTokenLength)
}

func validatePushClientHost(value string) error {
	if err := validateStringMaxLength("push client host", value, MaxPushClientHostLength); err != nil {
		return err
	}
	if value == "" {
		return nil
	}

	parsed, err := url.Parse("https://" + value)
	if err != nil || parsed.Host != value || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.HasSuffix(parsed.Host, ":") {
		return fmt.Errorf("%w: push client host must contain only a hostname and optional port", ErrInvalidArgument)
	}
	if port := parsed.Port(); port != "" {
		numericPort, err := strconv.Atoi(port)
		if err != nil || numericPort < 1 || numericPort > 65535 {
			return fmt.Errorf("%w: push client host port must be between 1 and 65535", ErrInvalidArgument)
		}
	}
	return nil
}

func isLoopbackHostname(hostname string) bool {
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
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
		var subscription runtimestatev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &subscription); err != nil {
			return fmt.Errorf("failed to unmarshal push subscription before deleting: %w", err)
		}
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

// DeletePushSubscriptionByCapability removes only the exact current endpoint
// owner whose browser Push API auth secret and per-save token match. It is safe
// to call without an account session after a cancelled registration settles:
// later saves fail the token/revision checks even when they reuse the same
// browser subscription credentials.
func (c *ChattoCore) DeletePushSubscriptionByCapability(ctx context.Context, endpoint, auth, cleanupToken string) error {
	if err := validateStringMaxLength("push endpoint", endpoint, MaxPushEndpointLength); err != nil {
		return err
	}
	if err := pushendpoint.Validate(endpoint); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if auth == "" {
		return fmt.Errorf("%w: push auth secret is required", ErrInvalidArgument)
	}
	if err := validateStringMaxLength("push auth secret", auth, MaxPushAuthLength); err != nil {
		return err
	}
	if err := validatePushCleanupToken(cleanupToken); err != nil {
		return err
	}

	owner, err := c.getPushEndpointOwner(ctx, endpoint)
	if err != nil || owner == nil {
		return err
	}
	key := pushSubscriptionKey(owner.UserID, endpoint)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if isPushRuntimeStateKeyAbsent(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get push subscription for capability cleanup: %w", err)
	}
	if entry.Revision() != owner.SubscriptionRevision {
		return nil
	}

	var subscription runtimestatev1.PushSubscription
	if err := proto.Unmarshal(entry.Value(), &subscription); err != nil {
		return fmt.Errorf("failed to unmarshal push subscription for capability cleanup: %w", err)
	}
	if subscription.GetEndpoint() != endpoint ||
		subtle.ConstantTimeCompare([]byte(subscription.Auth), []byte(auth)) != 1 ||
		subtle.ConstantTimeCompare([]byte(subscription.CleanupToken), []byte(cleanupToken)) != 1 {
		return nil
	}

	if err := c.releasePushEndpointOwnership(ctx, owner.UserID, endpoint, entry.Revision()); err != nil {
		return err
	}
	err = c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
	if err != nil && !isPushRuntimeStateKeyAbsent(err) && !jetstreamutil.IsSequenceConflict(err) {
		return fmt.Errorf("failed to delete push subscription by capability: %w", err)
	}
	return nil
}

// GetUserPushSubscriptions returns all push subscriptions for a user.
// Authorization: Caller must verify userID matches authenticated user.
func (c *ChattoCore) GetUserPushSubscriptions(ctx context.Context, userID string) ([]*runtimestatev1.PushSubscription, error) {
	keys, err := listPushRuntimeStateKeys(ctx, c.storage.runtimeStateKV, pushSubscriptionKeyFilter(userID))
	if err != nil {
		return nil, fmt.Errorf("failed to list push subscription keys: %w", err)
	}

	var subscriptions []*runtimestatev1.PushSubscription
	for _, key := range keys {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get push subscription %s: %w", key, err)
		}

		var sub runtimestatev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &sub); err != nil {
			return nil, fmt.Errorf("failed to unmarshal push subscription %s: %w", key, err)
		}
		owned, err := c.pushSubscriptionRevisionOwnedByUser(ctx, userID, sub.GetEndpoint(), entry.Revision())
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
	keys, err := listPushRuntimeStateKeys(ctx, c.storage.runtimeStateKV, pushSubscriptionKeyFilter(userID))
	if err != nil {
		return 0, fmt.Errorf("failed to list push subscription keys: %w", err)
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

		var sub runtimestatev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &sub); err != nil {
			// The raw record may still contain credentials even when an older or
			// damaged payload cannot be decoded. Erase it first; the global
			// reconciler separately removes unusable or orphaned owner records.
			deleteErr := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
			if deleteErr == nil || isPushRuntimeStateKeyAbsent(deleteErr) {
				deleted++
				cleanupErrors = append(cleanupErrors, fmt.Errorf("decode push subscription %s during deletion: %w", key, err))
				continue
			}
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete undecodable push subscription %s: %w", key, deleteErr))
			continue
		}
		if err := c.releasePushEndpointOwnership(ctx, userID, sub.GetEndpoint(), entry.Revision()); err != nil {
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

	c.logger.Debug("Deleted all push subscriptions for user",
		"user_id", userID,
		"count", deleted)

	return deleted, errors.Join(cleanupErrors...)
}

// GetAllPushSubscriptions returns all push subscriptions in the system.
// Authorization: Internal use only.
//
// NOTE: Currently unused. Reserved for future admin dashboard feature to list
// all push subscriptions for monitoring/debugging purposes.
func (c *ChattoCore) GetAllPushSubscriptions(ctx context.Context) ([]*PushSubscriptionWithUser, error) {
	keys, err := listPushRuntimeStateKeys(ctx, c.storage.runtimeStateKV, "push_subscription.>")
	if err != nil {
		return nil, fmt.Errorf("failed to list push subscription keys: %w", err)
	}

	var subscriptions []*PushSubscriptionWithUser
	for _, key := range keys {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Failed to get push subscription", "key", key, "error", err)
			continue
		}

		var sub runtimestatev1.PushSubscription
		if err := proto.Unmarshal(entry.Value(), &sub); err != nil {
			c.logger.Warn("Failed to unmarshal push subscription", "key", key, "error", err)
			continue
		}

		// Extract userID from key: push_subscription.{userId}.{hash}
		userID := extractUserIDFromPushKey(key)
		if userID == "" {
			continue
		}
		owned, err := c.pushSubscriptionRevisionOwnedByUser(ctx, userID, sub.GetEndpoint(), entry.Revision())
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
	Subscription *runtimestatev1.PushSubscription
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
