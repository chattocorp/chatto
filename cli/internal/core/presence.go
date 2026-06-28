package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Presence status constants used by public API and storage mappings.
const (
	PresenceStatusOffline      = "OFFLINE"
	PresenceStatusOnline       = "ONLINE"
	PresenceStatusAway         = "AWAY"
	PresenceStatusDoNotDisturb = "DO_NOT_DISTURB"
)

// Presence configuration constants
const (
	// PresenceTTL is the TTL for presence entries in the KV bucket.
	// If a client disconnects without explicit cleanup, entries expire after this duration.
	PresenceTTL = 60 * time.Second

	// PresenceRefreshInterval is how often clients refresh their presence.
	// Should be less than PresenceTTL to ensure entries don't expire while connected.
	PresenceRefreshInterval = 30 * time.Second
)

// presenceStatusFromString converts a stored presence status string to protobuf enum.
// Note: OFFLINE should never be stored - callers should delete the key instead.
func presenceStatusFromString(s string) corev1.UserPresenceStatus {
	switch s {
	case PresenceStatusAway:
		return corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY
	case PresenceStatusDoNotDisturb:
		return corev1.UserPresenceStatus_USER_PRESENCE_STATUS_DO_NOT_DISTURB
	default:
		return corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE
	}
}

// presenceStatusToString converts a protobuf UserPresenceStatus enum to storage string.
func presenceStatusToString(status corev1.UserPresenceStatus) string {
	switch status {
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY:
		return PresenceStatusAway
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_DO_NOT_DISTURB:
		return PresenceStatusDoNotDisturb
	default:
		return PresenceStatusOnline
	}
}

// ============================================================================
// Key Helpers
// ============================================================================

const maxPresenceWriteRetries = 5

// presenceKey returns the MEMORY_CACHE key for a user's live presence state.
func presenceKey(userID string) string {
	return fmt.Sprintf("presence.%s", userID)
}

// parsePresenceKey extracts userID from a presence key.
// Key format: presence.{userId}
func parsePresenceKey(key string) (userID string, ok bool) {
	const prefix = "presence."
	if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
		return "", false
	}
	userID = key[len(prefix):]
	if userID == "" {
		return "", false
	}
	return userID, true
}

// ============================================================================
// Presence Operations
// ============================================================================

// GetUserPresence retrieves a user's current presence status.
// Returns "OFFLINE" if the user has no presence entry (never connected or TTL expired).
func (s *PresenceModel) GetUserPresence(ctx context.Context, userID string) (string, error) {
	entry, err := s.memoryCacheKV.Get(ctx, presenceKey(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return PresenceStatusOffline, nil
		}
		return PresenceStatusOffline, fmt.Errorf("failed to get presence: %w", err)
	}
	if entry.Operation() == jetstream.KeyValueDelete ||
		entry.Operation() == jetstream.KeyValuePurge {
		return PresenceStatusOffline, nil
	}
	presence := &corev1.UserPresence{}
	if err := proto.Unmarshal(entry.Value(), presence); err != nil {
		s.logger.Warn("Failed to unmarshal presence, treating user as offline",
			"error", err, "user_id", userID)
		return PresenceStatusOffline, nil
	}

	return presenceStatusToString(presence.Status), nil
}

// SetPresence writes/refreshes a user's live presence in MEMORY_CACHE.
// Authorization: Caller must verify the user is authenticated before calling.
func (s *PresenceModel) SetPresence(ctx context.Context, userID string, status string) error {
	return s.SetPresenceWithOptions(ctx, userID, status, false)
}

// SetPresenceWithOptions writes/refreshes a user's live presence in MEMORY_CACHE.
// manuallySet marks explicit user-selected Away/DND so automatic reports from
// other clients do not overwrite the user's chosen availability.
func (s *PresenceModel) SetPresenceWithOptions(ctx context.Context, userID string, status string, manuallySet bool) error {
	presence := &corev1.UserPresence{
		Status:      presenceStatusFromString(status),
		ManuallySet: manuallySet && status != PresenceStatusOnline,
	}

	data, err := proto.Marshal(presence)
	if err != nil {
		return fmt.Errorf("failed to marshal presence: %w", err)
	}

	return s.writePresence(ctx, presenceKey(userID), data, manuallySet)
}

// refreshPresence reads the current presence value from KV and re-puts it
// to refresh the TTL. If no entry exists (race with expiry), sets ONLINE as default.
// This preserves whatever status the client set via updateMyPresence.
//
// Uses optimistic locking (kv.Update with revision) to avoid overwriting a concurrent
// SetPresence call from updateMyPresence. If the revision has changed between Get and
// Update, the newer value is preserved and we silently skip the refresh.
func (s *PresenceModel) refreshPresence(ctx context.Context, userID string) error {
	key := presenceKey(userID)
	entry, err := s.memoryCacheKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// Entry expired between ticks — re-set to ONLINE as safe default
			return s.SetPresence(ctx, userID, PresenceStatusOnline)
		}
		return fmt.Errorf("failed to read presence for refresh: %w", err)
	}

	// Re-put the same value to refresh TTL using optimistic locking.
	// If a concurrent SetPresence modified the entry, Update fails and
	// the newer status is preserved — which is the correct behavior.
	_, err = s.putPresenceWithTTL(ctx, key, entry.Value(), entry.Revision())
	if err != nil {
		// ErrKeyExists means the revision changed (concurrent write) — that's fine,
		// the newer value already has a fresh TTL from the concurrent Put.
		if errors.Is(err, jetstream.ErrKeyExists) {
			return nil
		}
		return fmt.Errorf("failed to refresh presence: %w", err)
	}
	return nil
}

func (s *PresenceModel) writePresence(ctx context.Context, key string, data []byte, forceOverwrite bool) error {
	for attempt := 0; attempt < maxPresenceWriteRetries; attempt++ {
		entry, err := s.memoryCacheKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				_, err = s.memoryCacheKV.Create(ctx, key, data, jetstream.KeyTTL(PresenceTTL))
				if errors.Is(err, jetstream.ErrKeyExists) {
					continue
				}
				return err
			}
			return fmt.Errorf("failed to read presence: %w", err)
		}

		if !forceOverwrite && shouldIgnoreAutomaticPresenceWrite(entry.Value(), data) {
			return nil
		}

		_, err = s.putPresenceWithTTL(ctx, key, data, entry.Revision())
		if err == nil {
			return nil
		}
		if errors.Is(err, jetstream.ErrKeyExists) {
			continue
		}
		return err
	}

	return fmt.Errorf("presence update failed after %d retries", maxPresenceWriteRetries)
}

func shouldIgnoreAutomaticPresenceWrite(existingData, incomingData []byte) bool {
	var existing corev1.UserPresence
	if err := proto.Unmarshal(existingData, &existing); err != nil {
		return false
	}
	if !existing.ManuallySet {
		return false
	}
	var incoming corev1.UserPresence
	if err := proto.Unmarshal(incomingData, &incoming); err != nil {
		return false
	}
	return !incoming.ManuallySet
}

func (s *PresenceModel) putPresenceWithTTL(ctx context.Context, key string, data []byte, revision uint64) (uint64, error) {
	ack, err := s.js.Publish(
		ctx,
		"$KV.MEMORY_CACHE."+key,
		data,
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(PresenceTTL),
	)
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}

// GetUserPresence retrieves a user's current presence status.
// Returns "OFFLINE" if the user has no presence entry (never connected or TTL expired).
func (c *ChattoCore) GetUserPresence(ctx context.Context, userID string) (string, error) {
	return c.presenceModel.GetUserPresence(ctx, userID)
}

// SetPresence writes/refreshes a user's live presence in MEMORY_CACHE.
// Authorization: Caller must verify the user is authenticated before calling.
func (c *ChattoCore) SetPresence(ctx context.Context, userID string, status string) error {
	return c.presenceModel.SetPresence(ctx, userID, status)
}

func (c *ChattoCore) SetPresenceWithOptions(ctx context.Context, userID string, status string, manuallySet bool) error {
	return c.presenceModel.SetPresenceWithOptions(ctx, userID, status, manuallySet)
}

func (c *ChattoCore) refreshPresence(ctx context.Context, userID string) error {
	return c.presenceModel.refreshPresence(ctx, userID)
}
