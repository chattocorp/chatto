package migrations

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateUsersToES seeds EVT from the legacy INSTANCE user/account keys
// for issue #643:
//
//   - user.{id}
//   - auth.{id}.password
//   - user.{id}.avatar
//   - verified_emails.{id}.{emailHash}
//   - user_login_changed_at.{id}
//   - user_by_oidc.{issuerSubjectHash}
//
// New durable user events encrypt login, display name, and verified email
// payloads. The migration therefore emits an initial content-key event for
// each imported user and writes encrypted PII facts using that epoch.
//
// Login and email indexes are not imported as their own events; they are
// reconstructed by the projection from user-created / verified-email-added
// events. OIDC index keys are one-way hashes, so legacy imports preserve the
// hash directly.
func MigrateUsersToES(
	ctx context.Context,
	serverKV jetstream.KeyValue,
	publisher *events.Publisher,
	keyWrapper kms.KeyWrapper,
	logger *log.Logger,
) error {
	userKeys, err := listLegacyUserRecordKeys(ctx, serverKV)
	if err != nil {
		return err
	}
	if len(userKeys) == 0 {
		return nil
	}
	if keyWrapper == nil {
		return fmt.Errorf("users ES migration requires a KMS key wrapper")
	}

	oidcByUser, err := loadOIDCSubjectHashesByUser(ctx, serverKV)
	if err != nil {
		return err
	}

	var imported, skipped int
	startedAt := time.Now()
	for _, key := range userKeys {
		entry, err := serverKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("get user record %s: %w", key, err)
		}

		var user corev1.User
		if err := proto.Unmarshal(entry.Value(), &user); err != nil {
			logger.Warn("users ES migration: skipping unmarshalable user", "key", key, "error", err)
			continue
		}
		if user.GetId() == "" {
			logger.Warn("users ES migration: skipping user without id", "key", key)
			continue
		}

		agg := events.UserAggregate(user.GetId())
		existingEvents, expectedSeq, err := publisher.SubjectEvents(ctx, agg.AllEventsFilter())
		if err != nil {
			return fmt.Errorf("read existing user events for %s: %w", user.GetId(), err)
		}

		entries, cleanupKeyRefs, err := buildUserMigrationEntries(ctx, serverKV, keyWrapper, &user, entry.Created(), oidcByUser[user.GetId()], existingEvents, logger)
		if err != nil {
			cleanupMigrationKeyRefs(ctx, keyWrapper, cleanupKeyRefs, logger)
			return fmt.Errorf("build user migration events for %s: %w", user.GetId(), err)
		}

		userImported, userSkipped, err := publishUserMigration(ctx, publisher, user.GetId(), entries, existingEvents, expectedSeq, logger)
		if err != nil {
			if userImported == 0 {
				cleanupMigrationKeyRefs(ctx, keyWrapper, cleanupKeyRefs, logger)
			}
			return fmt.Errorf("publish user migration for %s: %w", user.GetId(), err)
		}
		if userImported == 0 {
			cleanupMigrationKeyRefs(ctx, keyWrapper, cleanupKeyRefs, logger)
		}
		imported += userImported
		skipped += userSkipped
	}

	if imported > 0 || skipped > 0 {
		logger.Info(
			"users ES migration: seeded events from legacy INSTANCE KV",
			"user_events_imported", imported,
			"user_events_skipped", skipped,
			"users_processed", len(userKeys),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	return nil
}

func listLegacyUserRecordKeys(ctx context.Context, kv jetstream.KeyValue) ([]string, error) {
	keys, err := listSortedKeys(ctx, kv, "user.*")
	if err != nil {
		return nil, fmt.Errorf("list user keys: %w", err)
	}
	out := keys[:0]
	for _, key := range keys {
		parts := strings.Split(key, ".")
		if len(parts) == 2 {
			out = append(out, key)
		}
	}
	return out, nil
}

type migrationContentKey struct {
	epoch         int32
	key           []byte
	event         *corev1.UserContentKeyGeneratedEvent
	cleanupKeyRef string
}

func buildUserMigrationEntries(
	ctx context.Context,
	kv jetstream.KeyValue,
	keyWrapper kms.KeyWrapper,
	user *corev1.User,
	legacyCreatedAt time.Time,
	oidcSubjectHashes []string,
	existingEvents []*corev1.Event,
	logger *log.Logger,
) ([]events.BatchEntry, []string, error) {
	agg := events.UserAggregate(user.GetId())
	createdAt := user.GetCreatedAt()
	if createdAt == nil {
		createdAt = timestamppb.New(legacyCreatedAt)
	}

	contentKey, err := migrationContentKeyForUser(ctx, keyWrapper, user.GetId(), existingEvents)
	if err != nil {
		return nil, nil, err
	}
	var cleanupKeyRefs []string
	if contentKey.cleanupKeyRef != "" {
		cleanupKeyRefs = append(cleanupKeyRefs, contentKey.cleanupKeyRef)
	}

	var entries []events.BatchEntry
	if contentKey.event != nil {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserContentKeyGenerated{
			UserContentKeyGenerated: contentKey.event,
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	created := stamp(&corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{
			UserId: user.GetId(),
		},
	}}, "system:migration", createdAt)
	accountCreated := created.GetUserAccountCreated()
	accountCreated.EncryptedLogin, err = encryptMigrationUserPIIString(contentKey, created.GetId(), user.GetId(), events.EventUserAccountCreated, "login", user.GetLogin())
	if err != nil {
		return nil, cleanupKeyRefs, fmt.Errorf("encrypt legacy login: %w", err)
	}
	accountCreated.EncryptedDisplayName, err = encryptMigrationUserPIIString(contentKey, created.GetId(), user.GetId(), events.EventUserAccountCreated, "display_name", user.GetDisplayName())
	if err != nil {
		return nil, cleanupKeyRefs, fmt.Errorf("encrypt legacy display name: %w", err)
	}
	entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(created), Event: created})

	if passwordHash, ok, err := getLegacyBytes(ctx, kv, "auth."+user.GetId()+".password"); err != nil {
		return nil, cleanupKeyRefs, err
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
				UserId:       user.GetId(),
				PasswordHash: passwordHash,
			},
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	if avatar, ok, err := getLegacyAvatar(ctx, kv, "user."+user.GetId()+".avatar"); err != nil {
		return nil, cleanupKeyRefs, err
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserAvatarSet{
			UserAvatarSet: &corev1.UserAvatarSetEvent{
				UserId: user.GetId(),
				Avatar: avatar,
			},
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	emailEntries, err := getLegacyVerifiedEmailEvents(ctx, kv, user.GetId(), contentKey, createdAt, logger)
	if err != nil {
		return nil, cleanupKeyRefs, err
	}
	for _, event := range emailEntries {
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	sort.Strings(oidcSubjectHashes)
	for _, hash := range oidcSubjectHashes {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserOidcSubjectLinked{
			UserOidcSubjectLinked: &corev1.UserOIDCSubjectLinkedEvent{
				UserId:      user.GetId(),
				SubjectHash: hash,
			},
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	if changedAt, ok, err := getLegacyLoginChangedAt(ctx, kv, "user_login_changed_at."+user.GetId()); err != nil {
		return nil, cleanupKeyRefs, err
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserLoginCooldownStarted{
			UserLoginCooldownStarted: &corev1.UserLoginCooldownStartedEvent{
				UserId: user.GetId(),
			},
		}}, "system:migration", timestamppb.New(changedAt))
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	if needsEncryptedUserRepair(existingEvents) {
		return repairUserMigrationEntries(entries, existingEvents), cleanupKeyRefs, nil
	}
	return entries, cleanupKeyRefs, nil
}

func migrationContentKeyForUser(ctx context.Context, keyWrapper kms.KeyWrapper, userID string, existingEvents []*corev1.Event) (*migrationContentKey, error) {
	if existing := firstMigrationContentKey(existingEvents); existing != nil {
		key, err := unwrapMigrationContentKey(ctx, keyWrapper, existing)
		if err != nil {
			return nil, fmt.Errorf("unwrap existing content key: %w", err)
		}
		return &migrationContentKey{
			epoch: existing.GetEpoch(),
			key:   key,
			event: proto.Clone(existing).(*corev1.UserContentKeyGeneratedEvent),
		}, nil
	}

	keyRef := kms.LegacyUserKeyRef(userID)
	cleanupKeyRef := ""
	exists, err := keyWrapper.KeyExists(ctx, keyRef)
	if err != nil {
		return nil, err
	}
	if !exists {
		keyRef, err = keyWrapper.CreateKey(ctx, userID)
		if err != nil {
			return nil, err
		}
		cleanupKeyRef = keyRef
	}

	contentKey, err := encryption.GenerateKey()
	if err != nil {
		return nil, err
	}
	wrapped, err := keyWrapper.WrapContentKey(ctx, keyRef, contentKey, migrationContentKeyAAD(userID, 1))
	if err != nil {
		if cleanupKeyRef != "" {
			_ = keyWrapper.ShredKey(context.WithoutCancel(ctx), cleanupKeyRef)
			cleanupKeyRef = ""
		}
		return nil, fmt.Errorf("wrap migration content key: %w", err)
	}
	return &migrationContentKey{
		epoch: 1,
		key:   contentKey,
		event: &corev1.UserContentKeyGeneratedEvent{
			UserId:              userID,
			Epoch:               1,
			EncryptedContentKey: wrapped.EncryptedContentKey,
			ContentKeyNonce:     wrapped.Nonce,
			WrappingAlgorithm:   wrapped.Algorithm,
			WrappingMetadata:    wrapped.Metadata,
			WrappingKeyRef:      keyRef,
		},
		cleanupKeyRef: cleanupKeyRef,
	}, nil
}

func unwrapMigrationContentKey(ctx context.Context, keyWrapper kms.KeyWrapper, e *corev1.UserContentKeyGeneratedEvent) ([]byte, error) {
	keyRef := e.GetWrappingKeyRef()
	if keyRef == "" {
		keyRef = kms.LegacyUserKeyRef(e.GetUserId())
	}
	return keyWrapper.UnwrapContentKey(ctx, keyRef, kms.WrappedContentKey{
		EncryptedContentKey: e.GetEncryptedContentKey(),
		Nonce:               e.GetContentKeyNonce(),
		Algorithm:           e.GetWrappingAlgorithm(),
		Metadata:            e.GetWrappingMetadata(),
	}, migrationContentKeyAAD(e.GetUserId(), e.GetEpoch()))
}

func firstMigrationContentKey(existingEvents []*corev1.Event) *corev1.UserContentKeyGeneratedEvent {
	for _, event := range existingEvents {
		if e := event.GetUserContentKeyGenerated(); e != nil {
			return e
		}
	}
	return nil
}

func needsEncryptedUserRepair(existingEvents []*corev1.Event) bool {
	if len(existingEvents) == 0 {
		return false
	}
	for _, event := range existingEvents {
		if e := event.GetUserAccountCreated(); e != nil && e.GetEncryptedLogin() != nil && e.GetEncryptedDisplayName() != nil {
			return false
		}
	}
	return true
}

func repairUserMigrationEntries(entries []events.BatchEntry, existingEvents []*corev1.Event) []events.BatchEntry {
	seen := make(map[string]struct{})
	for _, event := range existingEvents {
		seen[events.EventTypeOf(event)] = struct{}{}
	}

	var out []events.BatchEntry
	for _, entry := range entries {
		eventType := events.EventTypeOf(entry.Event)
		switch eventType {
		case events.EventUserContentKeyGenerated:
			if _, ok := seen[eventType]; !ok {
				out = append(out, entry)
			}
		case events.EventUserAccountCreated, events.EventUserVerifiedEmailAdded:
			out = append(out, entry)
		default:
			if _, ok := seen[eventType]; !ok {
				out = append(out, entry)
			}
		}
	}
	return out
}

func encryptMigrationUserPIIString(contentKey *migrationContentKey, eventID, userID, eventType, purpose, plaintext string) (*corev1.EncryptedUserString, error) {
	if contentKey == nil || contentKey.epoch <= 0 || len(contentKey.key) == 0 {
		return nil, fmt.Errorf("content key is missing")
	}
	encrypted, err := encryption.EncryptXChaCha20Poly1305(contentKey.key, []byte(plaintext), migrationUserPIIAAD(eventID, userID, eventType, purpose, contentKey.epoch))
	if err != nil {
		return nil, err
	}
	return &corev1.EncryptedUserString{
		EncryptedValue:  encrypted.Ciphertext,
		Nonce:           encrypted.Nonce,
		ContentKeyEpoch: contentKey.epoch,
	}, nil
}

func migrationUserPIIAAD(eventID, userID, eventType, purpose string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:user-pii-context:v1\x00event_id=%s\x00user_id=%s\x00event_type=%s\x00field=%s\x00content_key_epoch=%d", eventID, userID, eventType, purpose, epoch))
}

func migrationContentKeyAAD(userID string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:content-key-context:v2\x00user_id=%s\x00epoch=%d", userID, epoch))
}

func cleanupMigrationKeyRefs(ctx context.Context, keyWrapper kms.KeyWrapper, keyRefs []string, logger *log.Logger) {
	for _, keyRef := range keyRefs {
		if keyRef == "" {
			continue
		}
		if err := keyWrapper.ShredKey(context.WithoutCancel(ctx), keyRef); err != nil {
			logger.Warn("users ES migration: failed to clean up unused key ref", "key_ref", keyRef, "error", err)
		}
	}
}

func getLegacyBytes(ctx context.Context, kv jetstream.KeyValue, key string) ([]byte, bool, error) {
	entry, err := kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get %s: %w", key, err)
	}
	return append([]byte(nil), entry.Value()...), true, nil
}

func getLegacyAvatar(ctx context.Context, kv jetstream.KeyValue, key string) (*corev1.DeprecatedAsset, bool, error) {
	value, ok, err := getLegacyBytes(ctx, kv, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	asset := &corev1.DeprecatedAsset{}
	if err := proto.Unmarshal(value, asset); err != nil {
		return nil, false, fmt.Errorf("unmarshal %s: %w", key, err)
	}
	return asset, true, nil
}

func getLegacyLoginChangedAt(ctx context.Context, kv jetstream.KeyValue, key string) (time.Time, bool, error) {
	value, ok, err := getLegacyBytes(ctx, kv, key)
	if err != nil || !ok {
		return time.Time{}, ok, err
	}
	t, err := time.Parse(time.RFC3339, string(value))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse %s: %w", key, err)
	}
	return t, true, nil
}

func getLegacyVerifiedEmailEvents(
	ctx context.Context,
	kv jetstream.KeyValue,
	userID string,
	contentKey *migrationContentKey,
	fallbackCreatedAt *timestamppb.Timestamp,
	logger *log.Logger,
) ([]*corev1.Event, error) {
	keys, err := listSortedKeys(ctx, kv, "verified_emails."+userID+".*")
	if err != nil {
		return nil, fmt.Errorf("list verified emails for %s: %w", userID, err)
	}
	out := make([]*corev1.Event, 0, len(keys))
	for _, key := range keys {
		entry, err := kv.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get %s: %w", key, err)
		}
		var ve corev1.VerifiedEmail
		if err := proto.Unmarshal(entry.Value(), &ve); err != nil {
			logger.Warn("users ES migration: skipping unmarshalable verified email", "key", key, "error", err)
			continue
		}
		verifiedAt := ve.GetVerifiedAt()
		if verifiedAt == nil {
			verifiedAt = timestamppb.New(entry.Created())
		}
		if verifiedAt == nil {
			verifiedAt = fallbackCreatedAt
		}
		event := stamp(&corev1.Event{Event: &corev1.Event_UserVerifiedEmailAdded{
			UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
				UserId: userID,
			},
		}}, "system:migration", verifiedAt)
		emailEvent := event.GetUserVerifiedEmailAdded()
		emailEvent.EncryptedEmail, err = encryptMigrationUserPIIString(contentKey, event.GetId(), userID, events.EventUserVerifiedEmailAdded, "email", ve.GetEmail())
		if err != nil {
			return nil, fmt.Errorf("encrypt legacy verified email %s: %w", key, err)
		}
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GetCreatedAt() != nil && out[j].GetCreatedAt() != nil && !out[i].GetCreatedAt().AsTime().Equal(out[j].GetCreatedAt().AsTime()) {
			return out[i].GetCreatedAt().AsTime().Before(out[j].GetCreatedAt().AsTime())
		}
		return out[i].GetId() < out[j].GetId()
	})
	return out, nil
}

func loadOIDCSubjectHashesByUser(ctx context.Context, kv jetstream.KeyValue) (map[string][]string, error) {
	keys, err := listSortedKeys(ctx, kv, "user_by_oidc.*")
	if err != nil {
		return nil, fmt.Errorf("list OIDC indexes: %w", err)
	}
	out := make(map[string][]string)
	for _, key := range keys {
		entry, err := kv.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("get %s: %w", key, err)
		}
		hash := strings.TrimPrefix(key, "user_by_oidc.")
		out[string(entry.Value())] = append(out[string(entry.Value())], hash)
	}
	return out, nil
}

func publishUserMigration(
	ctx context.Context,
	publisher *events.Publisher,
	userID string,
	entries []events.BatchEntry,
	existingEvents []*corev1.Event,
	expectedSeq uint64,
	logger *log.Logger,
) (imported int, skipped int, err error) {
	if len(entries) == 0 {
		return 0, len(existingEvents), nil
	}

	if !needsEncryptedUserRepair(existingEvents) {
		if len(existingEvents) > len(entries) {
			logger.Warn(
				"users ES migration: skipping user with more existing events than legacy events",
				"user_id", userID,
				"existing_events", len(existingEvents),
				"legacy_events", len(entries),
			)
			return 0, len(entries), nil
		}
		for i, existing := range existingEvents {
			if userMigrationIdentity(existing) != userMigrationIdentity(entries[i].Event) {
				logger.Warn(
					"users ES migration: skipping user with non-matching existing event prefix",
					"user_id", userID,
					"index", i,
					"existing_event", userMigrationIdentity(existing),
					"legacy_event", userMigrationIdentity(entries[i].Event),
				)
				return 0, len(entries), nil
			}
		}
		if len(existingEvents) == len(entries) {
			return 0, len(entries), nil
		}
		entries = entries[len(existingEvents):]
	} else {
		skipped = len(existingEvents)
	}

	for start := 0; start < len(entries); start += messageMigrationBatchSize {
		end := start + messageMigrationBatchSize
		if end > len(entries) {
			end = len(entries)
		}

		chunk := append([]events.BatchEntry(nil), entries[start:end]...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = expectedSeq
		chunk[0].FilterSubject = events.UserAggregate(userID).AllEventsFilter()

		seqs, err := publisher.AppendBatch(ctx, chunk)
		if err != nil {
			if errors.Is(err, events.ErrConflict) {
				return imported, skipped, fmt.Errorf("user chunk OCC conflict after resume point %d: %w", len(existingEvents)+imported, err)
			}
			return imported, skipped, err
		}
		expectedSeq = seqs[len(seqs)-1]
		imported += len(chunk)
	}
	return imported, skipped, nil
}

func userMigrationIdentity(event *corev1.Event) string {
	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserContentKeyGenerated:
		return strings.Join([]string{events.EventUserContentKeyGenerated, e.UserContentKeyGenerated.GetUserId(), fmt.Sprint(e.UserContentKeyGenerated.GetEpoch())}, "\x00")
	case *corev1.Event_UserAccountCreated:
		return events.EventUserAccountCreated + "\x00" + e.UserAccountCreated.GetUserId()
	case *corev1.Event_UserDisplayNameChanged:
		return events.EventUserDisplayNameChanged + "\x00" + e.UserDisplayNameChanged.GetUserId()
	case *corev1.Event_UserPasswordHashChanged:
		return events.EventUserPasswordHashChanged + "\x00" + e.UserPasswordHashChanged.GetUserId() + "\x00" + string(e.UserPasswordHashChanged.GetPasswordHash())
	case *corev1.Event_UserAvatarSet:
		data, _ := proto.Marshal(e.UserAvatarSet.GetAvatar())
		return events.EventUserAvatarSet + "\x00" + e.UserAvatarSet.GetUserId() + "\x00" + hex.EncodeToString(data)
	case *corev1.Event_UserAvatarCleared:
		return events.EventUserAvatarCleared + "\x00" + e.UserAvatarCleared.GetUserId()
	case *corev1.Event_UserVerifiedEmailAdded:
		return events.EventUserVerifiedEmailAdded + "\x00" + e.UserVerifiedEmailAdded.GetUserId()
	case *corev1.Event_UserOidcSubjectLinked:
		return events.EventUserOIDCSubjectLinked + "\x00" + e.UserOidcSubjectLinked.GetUserId() + "\x00" + e.UserOidcSubjectLinked.GetSubjectHash()
	case *corev1.Event_UserServerPreferencesChanged:
		data, _ := proto.Marshal(e.UserServerPreferencesChanged.GetPreferences())
		return events.EventUserServerPreferencesChanged + "\x00" + e.UserServerPreferencesChanged.GetUserId() + "\x00" + hex.EncodeToString(data)
	case *corev1.Event_UserLoginChanged:
		return events.EventUserLoginChanged + "\x00" + e.UserLoginChanged.GetUserId()
	case *corev1.Event_UserLoginCooldownStarted:
		return events.EventUserLoginCooldownStarted + "\x00" + e.UserLoginCooldownStarted.GetUserId()
	case *corev1.Event_UserLoginCooldownCleared:
		return events.EventUserLoginCooldownCleared + "\x00" + e.UserLoginCooldownCleared.GetUserId()
	}
	return events.EventTypeOf(event)
}
