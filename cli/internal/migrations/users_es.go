package migrations

import (
	"bytes"
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

	"hmans.de/chatto/internal/dekstore"
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
// payloads. The migration therefore emits initial purpose-scoped DEK events
// for each imported user and writes encrypted PII facts using that epoch.
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
	contentKeys *dekstore.Store,
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
	if contentKeys == nil {
		return fmt.Errorf("users ES migration requires a DEK store")
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

		entries, cleanupKeyRefs, err := buildUserMigrationEntries(ctx, serverKV, keyWrapper, contentKeys, &user, entry.Created(), oidcByUser[user.GetId()], existingEvents, logger)
		if err != nil {
			cleanupMigrationKeyRefs(ctx, keyWrapper, contentKeys, cleanupKeyRefs, logger)
			return fmt.Errorf("build user migration events for %s: %w", user.GetId(), err)
		}

		userImported, userSkipped, err := publishUserMigration(ctx, publisher, user.GetId(), entries, existingEvents, expectedSeq, logger)
		if err != nil {
			if userImported == 0 {
				cleanupMigrationKeyRefs(ctx, keyWrapper, contentKeys, cleanupKeyRefs, logger)
			}
			return fmt.Errorf("publish user migration for %s: %w", user.GetId(), err)
		}
		if userImported == 0 {
			cleanupMigrationKeyRefs(ctx, keyWrapper, contentKeys, cleanupKeyRefs, logger)
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

type migrationDEK struct {
	epoch                int32
	purpose              corev1.UserDEKPurpose
	key                  []byte
	event                *corev1.UserDEKGeneratedEvent
	cleanupKeyRef        string
	cleanupContentKeyRef string
}

func buildUserMigrationEntries(
	ctx context.Context,
	kv jetstream.KeyValue,
	keyWrapper kms.KeyWrapper,
	contentKeys *dekstore.Store,
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

	messageDEK, err := migrationDEKForUser(ctx, keyWrapper, contentKeys, user.GetId(), corev1.UserDEKPurpose_USER_DEK_PURPOSE_MESSAGE_BODY, existingEvents)
	if err != nil {
		return nil, nil, err
	}
	var cleanupKeyRefs []string
	if messageDEK.cleanupKeyRef != "" {
		cleanupKeyRefs = append(cleanupKeyRefs, messageDEK.cleanupKeyRef)
	}
	if messageDEK.cleanupContentKeyRef != "" {
		cleanupKeyRefs = append(cleanupKeyRefs, messageDEK.cleanupContentKeyRef)
	}
	piiDEK, err := migrationDEKForUser(ctx, keyWrapper, contentKeys, user.GetId(), corev1.UserDEKPurpose_USER_DEK_PURPOSE_USER_PII, existingEvents)
	if err != nil {
		return nil, cleanupKeyRefs, err
	}
	if piiDEK.cleanupKeyRef != "" {
		cleanupKeyRefs = append(cleanupKeyRefs, piiDEK.cleanupKeyRef)
	}
	if piiDEK.cleanupContentKeyRef != "" {
		cleanupKeyRefs = append(cleanupKeyRefs, piiDEK.cleanupContentKeyRef)
	}

	var entries []events.BatchEntry
	if messageDEK.event != nil {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: messageDEK.event,
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}
	if piiDEK.event != nil && !sameMigrationDEKEvent(messageDEK.event, piiDEK.event) {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserDekGenerated{
			UserDekGenerated: piiDEK.event,
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	created := stamp(&corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{
			UserId: user.GetId(),
		},
	}}, "system:migration", createdAt)
	accountCreated := created.GetUserAccountCreated()
	accountCreated.EncryptedLogin, err = encryptMigrationUserPIIString(piiDEK, created.GetId(), user.GetId(), events.EventUserAccountCreated, "login", user.GetLogin())
	if err != nil {
		return nil, cleanupKeyRefs, fmt.Errorf("encrypt legacy login: %w", err)
	}
	accountCreated.EncryptedDisplayName, err = encryptMigrationUserPIIString(piiDEK, created.GetId(), user.GetId(), events.EventUserAccountCreated, "display_name", user.GetDisplayName())
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
		if isCorruptLegacyValue(err) {
			logger.Warn("users ES migration: skipping corrupt legacy avatar", "user_id", user.GetId(), "error", err)
		} else {
			return nil, cleanupKeyRefs, err
		}
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserAvatarSet{
			UserAvatarSet: &corev1.UserAvatarSetEvent{
				UserId: user.GetId(),
				Avatar: avatar,
			},
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	emailEntries, err := getLegacyVerifiedEmailEvents(ctx, kv, user.GetId(), piiDEK, createdAt, logger)
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
		if isCorruptLegacyValue(err) {
			logger.Warn("users ES migration: skipping corrupt login cooldown timestamp", "user_id", user.GetId(), "error", err)
		} else {
			return nil, cleanupKeyRefs, err
		}
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

func migrationDEKForUser(ctx context.Context, keyWrapper kms.KeyWrapper, contentKeys *dekstore.Store, userID string, purpose corev1.UserDEKPurpose, existingEvents []*corev1.Event) (*migrationDEK, error) {
	if existing := firstMigrationDEK(existingEvents, purpose); existing != nil {
		key, err := unwrapMigrationDEK(ctx, keyWrapper, contentKeys, existing)
		if err != nil {
			return nil, fmt.Errorf("unwrap existing DEK: %w", err)
		}
		return &migrationDEK{
			epoch:   existing.GetEpoch(),
			purpose: existing.GetPurpose(),
			key:     key,
			event:   proto.Clone(existing).(*corev1.UserDEKGeneratedEvent),
		}, nil
	}

	keyRef, err := keyWrapper.CreateKey(ctx, userID)
	if err != nil {
		return nil, err
	}
	cleanupKeyRef := keyRef

	dek, err := encryption.GenerateKey()
	if err != nil {
		return nil, err
	}
	wrapped, err := keyWrapper.WrapContentKey(ctx, keyRef, dek, migrationUserDEKAAD(userID, purpose, 1))
	if err != nil {
		if cleanupKeyRef != "" {
			_ = keyWrapper.ShredKey(context.WithoutCancel(ctx), cleanupKeyRef)
			cleanupKeyRef = ""
		}
		return nil, fmt.Errorf("wrap migration DEK: %w", err)
	}
	stored := &corev1.UserDataEncryptionKey{
		EncryptedContentKey: wrapped.EncryptedContentKey,
		ContentKeyNonce:     wrapped.Nonce,
		WrappingAlgorithm:   wrapped.Algorithm,
		WrappingMetadata:    wrapped.Metadata,
		WrappingKeyRef:      keyRef,
	}
	contentKeyRef, err := contentKeys.Create(ctx, stored)
	if err != nil {
		if cleanupKeyRef != "" {
			_ = keyWrapper.ShredKey(context.WithoutCancel(ctx), cleanupKeyRef)
			cleanupKeyRef = ""
		}
		return nil, err
	}
	event := &corev1.UserDEKGeneratedEvent{
		UserId:            userID,
		Epoch:             1,
		Purpose:           purpose,
		ContentKeyRef:     contentKeyRef,
		WrappingAlgorithm: stored.WrappingAlgorithm,
		WrappingMetadata:  stored.WrappingMetadata,
		WrappingKeyRef:    stored.WrappingKeyRef,
	}
	verifiedDEK, err := unwrapMigrationDEK(ctx, keyWrapper, contentKeys, event)
	if err != nil || !bytes.Equal(verifiedDEK, dek) {
		_ = contentKeys.Shred(context.WithoutCancel(ctx), contentKeyRef)
		if cleanupKeyRef != "" {
			_ = keyWrapper.ShredKey(context.WithoutCancel(ctx), cleanupKeyRef)
			cleanupKeyRef = ""
		}
		if err != nil {
			return nil, fmt.Errorf("verify migration DEK: %w", err)
		}
		return nil, fmt.Errorf("verify migration DEK: unwrapped DEK mismatch")
	}
	return &migrationDEK{
		epoch:                1,
		purpose:              purpose,
		key:                  dek,
		event:                event,
		cleanupKeyRef:        cleanupKeyRef,
		cleanupContentKeyRef: contentKeyRef,
	}, nil
}

func sameMigrationDEKEvent(a, b *corev1.UserDEKGeneratedEvent) bool {
	if a == nil || b == nil {
		return false
	}
	return a.GetUserId() == b.GetUserId() &&
		a.GetPurpose() == b.GetPurpose() &&
		a.GetEpoch() == b.GetEpoch()
}

func unwrapMigrationDEK(ctx context.Context, keyWrapper kms.KeyWrapper, contentKeys dekstore.Reader, e *corev1.UserDEKGeneratedEvent) ([]byte, error) {
	stored, err := contentKeys.Get(ctx, e.GetContentKeyRef())
	if err != nil {
		return nil, err
	}
	keyRef := stored.WrappingKeyRef
	if keyRef == "" {
		keyRef = kms.LegacyUserKeyRef(e.GetUserId())
	}
	return keyWrapper.UnwrapContentKey(ctx, keyRef, kms.WrappedContentKey{
		EncryptedContentKey: stored.EncryptedContentKey,
		Nonce:               stored.ContentKeyNonce,
		Algorithm:           stored.WrappingAlgorithm,
		Metadata:            stored.WrappingMetadata,
	}, migrationUserDEKAAD(e.GetUserId(), e.GetPurpose(), e.GetEpoch()))
}

func firstMigrationDEK(existingEvents []*corev1.Event, purpose corev1.UserDEKPurpose) *corev1.UserDEKGeneratedEvent {
	var fallback *corev1.UserDEKGeneratedEvent
	for _, event := range existingEvents {
		if e := event.GetUserDekGenerated(); e != nil {
			if e.GetPurpose() == purpose {
				return e
			}
			if e.GetPurpose() == corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED && fallback == nil {
				fallback = e
			}
		}
	}
	return fallback
}

func needsEncryptedUserRepair(existingEvents []*corev1.Event) bool {
	if len(existingEvents) == 0 {
		return false
	}
	for _, event := range existingEvents {
		if e := event.GetUserAccountCreated(); e != nil && (e.GetEncryptedLogin() == nil || e.GetEncryptedDisplayName() == nil) {
			return true
		}
	}
	return false
}

func repairUserMigrationEntries(entries []events.BatchEntry, existingEvents []*corev1.Event) []events.BatchEntry {
	seen := make(map[string]struct{})
	hasEncryptedAccount := false
	encryptedEmailCount := 0
	for _, event := range existingEvents {
		seen[userMigrationIdentity(event)] = struct{}{}
		if account := event.GetUserAccountCreated(); account != nil && account.GetEncryptedLogin() != nil && account.GetEncryptedDisplayName() != nil {
			hasEncryptedAccount = true
		}
		if email := event.GetUserVerifiedEmailAdded(); email != nil && email.GetEncryptedEmail() != nil {
			encryptedEmailCount++
		}
	}

	var out []events.BatchEntry
	seenEmailEntries := 0
	for _, entry := range entries {
		eventType := events.EventTypeOf(entry.Event)
		switch eventType {
		case events.EventUserAccountCreated:
			if !hasEncryptedAccount {
				out = append(out, entry)
			}
		case events.EventUserVerifiedEmailAdded:
			if seenEmailEntries < encryptedEmailCount {
				seenEmailEntries++
				continue
			}
			out = append(out, entry)
		default:
			if _, ok := seen[userMigrationIdentity(entry.Event)]; !ok {
				out = append(out, entry)
			}
		}
	}
	return out
}

func encryptMigrationUserPIIString(dek *migrationDEK, eventID, userID, eventType, purpose, plaintext string) (*corev1.EncryptedUserString, error) {
	if dek == nil || dek.epoch <= 0 || len(dek.key) == 0 {
		return nil, fmt.Errorf("DEK is missing")
	}
	encrypted, err := encryption.EncryptXChaCha20Poly1305(dek.key, []byte(plaintext), migrationUserPIIAAD(eventID, userID, eventType, purpose, dek.epoch))
	if err != nil {
		return nil, err
	}
	return &corev1.EncryptedUserString{
		EncryptedValue:  encrypted.Ciphertext,
		Nonce:           encrypted.Nonce,
		ContentKeyEpoch: dek.epoch,
	}, nil
}

func migrationUserPIIAAD(eventID, userID, eventType, purpose string, epoch int32) []byte {
	return []byte(fmt.Sprintf("chatto:user-pii-context:v1\x00event_id=%s\x00user_id=%s\x00event_type=%s\x00field=%s\x00content_key_epoch=%d", eventID, userID, eventType, purpose, epoch))
}

func migrationUserDEKAAD(userID string, purpose corev1.UserDEKPurpose, epoch int32) []byte {
	if purpose == corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return []byte(fmt.Sprintf("chatto:content-key-context:v2\x00user_id=%s\x00epoch=%d", userID, epoch))
	}
	return []byte(fmt.Sprintf("chatto:user-dek-context:v1\x00user_id=%s\x00purpose=%d\x00epoch=%d", userID, purpose, epoch))
}

func cleanupMigrationKeyRefs(ctx context.Context, keyWrapper kms.KeyWrapper, contentKeys *dekstore.Store, keyRefs []string, logger *log.Logger) {
	for _, keyRef := range keyRefs {
		if keyRef == "" {
			continue
		}
		var err error
		if strings.HasPrefix(keyRef, "dek.") {
			err = contentKeys.Shred(context.WithoutCancel(ctx), keyRef)
		} else {
			err = keyWrapper.ShredKey(context.WithoutCancel(ctx), keyRef)
		}
		if err != nil {
			logger.Warn("users ES migration: failed to clean up unused key ref", "key_ref", keyRef, "error", err)
		}
	}
}

func isCorruptLegacyValue(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "unmarshal ") || strings.HasPrefix(msg, "parse ")
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
	piiDEK *migrationDEK,
	fallbackCreatedAt *timestamppb.Timestamp,
	logger *log.Logger,
) ([]*corev1.Event, error) {
	keys, err := listSortedKeys(ctx, kv, "verified_emails."+userID+".*")
	if err != nil {
		return nil, fmt.Errorf("list verified emails for %s: %w", userID, err)
	}
	out := make([]legacyEmailEvent, 0, len(keys))
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
		emailEvent.EncryptedEmail, err = encryptMigrationUserPIIString(piiDEK, event.GetId(), userID, events.EventUserVerifiedEmailAdded, "email", ve.GetEmail())
		if err != nil {
			return nil, fmt.Errorf("encrypt legacy verified email %s: %w", key, err)
		}
		out = append(out, legacyEmailEvent{legacyKey: key, event: event})
	}
	sortLegacyEmailEvents(out)
	events := make([]*corev1.Event, 0, len(out))
	for _, entry := range out {
		events = append(events, entry.event)
	}
	return events, nil
}

type legacyEmailEvent struct {
	legacyKey string
	event     *corev1.Event
}

func sortLegacyEmailEvents(out []legacyEmailEvent) {
	sort.Slice(out, func(i, j int) bool {
		leftCreatedAt := out[i].event.GetCreatedAt()
		rightCreatedAt := out[j].event.GetCreatedAt()
		if leftCreatedAt != nil && rightCreatedAt != nil && !leftCreatedAt.AsTime().Equal(rightCreatedAt.AsTime()) {
			return leftCreatedAt.AsTime().Before(rightCreatedAt.AsTime())
		}
		return out[i].legacyKey < out[j].legacyKey
	})
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
	case *corev1.Event_UserDekGenerated:
		return strings.Join([]string{events.EventUserDEKGenerated, e.UserDekGenerated.GetUserId(), fmt.Sprint(e.UserDekGenerated.GetPurpose()), fmt.Sprint(e.UserDekGenerated.GetEpoch())}, "\x00")
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
