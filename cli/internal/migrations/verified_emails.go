package migrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateVerifiedEmailsToProto converts verified-emails storage from a
// single JSON blob per user to a per-email proto entry.
//
// # Legacy layout
//
//	key:   user.{userID}.verified_emails
//	value: JSON []{ "email": ..., "verified_at": ... }
//
// # New layout
//
//	key:   verified_emails.{userID}.{sha256(lowercase(email))}
//	value: proto-encoded corev1.VerifiedEmail
//
// The companion email-to-user index (`user_by_email.{hash}`) is left
// untouched — it was already shaped for O(1) reverse lookups and is
// orthogonal to this change.
//
// # Why
//
// The legacy shape required a read-modify-write to append an email, which
// is racy between concurrent verifications for the same user and forces
// the full list to be unmarshalled on every read. The new shape is also
// consistent with the rest of the project (proto everywhere except
// short-TTL token records).
//
// # Idempotency
//
// Safe to re-run. Per-email keys are deterministic in (userID, email);
// re-writing them with the same content is a no-op. The legacy key is
// only deleted after every per-email entry has been written, so a crash
// mid-migration just leaves the user's record to be re-converted on the
// next boot.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version that
// includes this migration. Operators can verify by inspecting the
// INSTANCE bucket for any remaining `user.*.verified_emails` keys.
func MigrateVerifiedEmailsToProto(ctx context.Context, kv jetstream.KeyValue, logger *log.Logger) error {
	const (
		legacyKeyPrefix = "user."
		legacyKeySuffix = ".verified_emails"
	)

	keyLister, err := kv.ListKeysFiltered(ctx, "user.*.verified_emails")
	if err != nil {
		// NATS returns an error when no keys match the filter. That's
		// the steady-state "nothing to migrate" case, not a failure.
		return nil
	}

	type legacyEntry struct {
		Email      string    `json:"email"`
		VerifiedAt time.Time `json:"verified_at"`
	}

	convertedUsers := 0
	convertedEmails := 0

	for legacyKey := range keyLister.Keys() {
		if !strings.HasPrefix(legacyKey, legacyKeyPrefix) || !strings.HasSuffix(legacyKey, legacyKeySuffix) {
			// Defensive: ListKeysFiltered should only yield matching
			// keys, but the wildcard `*` is unanchored so we double-check.
			continue
		}
		userID := strings.TrimSuffix(strings.TrimPrefix(legacyKey, legacyKeyPrefix), legacyKeySuffix)
		if userID == "" {
			continue
		}

		entry, err := kv.Get(ctx, legacyKey)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				// Concurrent deletion; nothing to do.
				continue
			}
			return fmt.Errorf("get legacy verified_emails for %s: %w", userID, err)
		}

		var emails []legacyEntry
		if err := json.Unmarshal(entry.Value(), &emails); err != nil {
			return fmt.Errorf("unmarshal legacy verified_emails for %s: %w", userID, err)
		}

		for _, le := range emails {
			if le.Email == "" {
				continue
			}
			hash := sha256.Sum256([]byte(strings.ToLower(le.Email)))
			newKey := fmt.Sprintf("verified_emails.%s.%s", userID, hex.EncodeToString(hash[:]))
			data, err := proto.Marshal(&corev1.VerifiedEmail{
				Email:      le.Email,
				VerifiedAt: timestamppb.New(le.VerifiedAt),
			})
			if err != nil {
				return fmt.Errorf("marshal verified_email for %s/%s: %w", userID, le.Email, err)
			}
			if _, err := kv.Put(ctx, newKey, data); err != nil {
				return fmt.Errorf("put verified_email for %s/%s: %w", userID, le.Email, err)
			}
			convertedEmails++
		}

		if err := kv.Delete(ctx, legacyKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("delete legacy verified_emails for %s: %w", userID, err)
		}
		convertedUsers++
	}

	if convertedUsers > 0 {
		logger.Info("verified_emails migration: converted JSON blobs to per-email proto entries",
			"users", convertedUsers, "emails", convertedEmails)
	}
	return nil
}
