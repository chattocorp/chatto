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

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateUsersToES seeds EVT from the legacy INSTANCE user/account keys
// for issue #643:
//
//   - user.{id}
//   - auth.{id}.password
//   - user.{id}.avatar
//   - verified_emails.{id}.{emailHash}
//   - user_preferences.{id}
//   - user_login_changed_at.{id}
//   - user_by_oidc.{issuerSubjectHash}
//
// Login and email indexes are not imported as their own events; they are
// reconstructed by the projection from user-created / verified-email-added
// events. OIDC index keys are one-way hashes, so legacy imports preserve the
// hash directly.
func MigrateUsersToES(
	ctx context.Context,
	serverKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	userKeys, err := listLegacyUserRecordKeys(ctx, serverKV)
	if err != nil {
		return err
	}
	if len(userKeys) == 0 {
		return nil
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

		entries, err := buildUserMigrationEntries(ctx, serverKV, &user, entry.Created(), oidcByUser[user.GetId()], logger)
		if err != nil {
			return fmt.Errorf("build user migration events for %s: %w", user.GetId(), err)
		}

		userImported, userSkipped, err := publishUserMigration(ctx, publisher, user.GetId(), entries, logger)
		if err != nil {
			return fmt.Errorf("publish user migration for %s: %w", user.GetId(), err)
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

func buildUserMigrationEntries(
	ctx context.Context,
	kv jetstream.KeyValue,
	user *corev1.User,
	legacyCreatedAt time.Time,
	oidcSubjectHashes []string,
	logger *log.Logger,
) ([]events.BatchEntry, error) {
	agg := events.UserAggregate(user.GetId())
	createdAt := user.GetCreatedAt()
	if createdAt == nil {
		createdAt = timestamppb.New(legacyCreatedAt)
	}

	created := stamp(&corev1.Event{Event: &corev1.Event_UserAccountCreated{
		UserAccountCreated: &corev1.UserAccountCreatedEvent{
			UserId:      user.GetId(),
			Login:       user.GetLogin(),
			DisplayName: user.GetDisplayName(),
		},
	}}, "system:migration", createdAt)

	entries := []events.BatchEntry{{
		Subject: agg.SubjectFor(created),
		Event:   created,
	}}

	if passwordHash, ok, err := getLegacyBytes(ctx, kv, "auth."+user.GetId()+".password"); err != nil {
		return nil, err
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
		return nil, err
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserAvatarSet{
			UserAvatarSet: &corev1.UserAvatarSetEvent{
				UserId: user.GetId(),
				Avatar: avatar,
			},
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	emailEntries, err := getLegacyVerifiedEmailEvents(ctx, kv, user.GetId(), createdAt, logger)
	if err != nil {
		return nil, err
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

	if prefs, ok, err := getLegacyPreferences(ctx, kv, "user_preferences."+user.GetId()); err != nil {
		return nil, err
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserServerPreferencesChanged{
			UserServerPreferencesChanged: &corev1.UserServerPreferencesChangedEvent{
				UserId:      user.GetId(),
				Preferences: prefs,
			},
		}}, "system:migration", createdAt)
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	if changedAt, ok, err := getLegacyLoginChangedAt(ctx, kv, "user_login_changed_at."+user.GetId()); err != nil {
		return nil, err
	} else if ok {
		event := stamp(&corev1.Event{Event: &corev1.Event_UserLoginCooldownStarted{
			UserLoginCooldownStarted: &corev1.UserLoginCooldownStartedEvent{
				UserId: user.GetId(),
			},
		}}, "system:migration", timestamppb.New(changedAt))
		entries = append(entries, events.BatchEntry{Subject: agg.SubjectFor(event), Event: event})
	}

	return entries, nil
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

func getLegacyPreferences(ctx context.Context, kv jetstream.KeyValue, key string) (*corev1.ServerUserPreferences, bool, error) {
	value, ok, err := getLegacyBytes(ctx, kv, key)
	if err != nil || !ok {
		return nil, ok, err
	}
	prefs := &corev1.ServerUserPreferences{}
	if err := proto.Unmarshal(value, prefs); err != nil {
		return nil, false, fmt.Errorf("unmarshal %s: %w", key, err)
	}
	return prefs, true, nil
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
				Email:  ve.GetEmail(),
			},
		}}, "system:migration", verifiedAt)
		out = append(out, event)
	}
	sort.Slice(out, func(i, j int) bool {
		a := out[i].GetUserVerifiedEmailAdded()
		b := out[j].GetUserVerifiedEmailAdded()
		if out[i].GetCreatedAt() != nil && out[j].GetCreatedAt() != nil && !out[i].GetCreatedAt().AsTime().Equal(out[j].GetCreatedAt().AsTime()) {
			return out[i].GetCreatedAt().AsTime().Before(out[j].GetCreatedAt().AsTime())
		}
		return strings.ToLower(a.GetEmail()) < strings.ToLower(b.GetEmail())
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
	logger *log.Logger,
) (imported int, skipped int, err error) {
	if len(entries) == 0 {
		return 0, 0, nil
	}

	agg := events.UserAggregate(userID)
	existingEvents, expectedSeq, err := publisher.SubjectEvents(ctx, agg.AllEventsFilter())
	if err != nil {
		return 0, 0, fmt.Errorf("read existing user events: %w", err)
	}
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

	pending := entries[len(existingEvents):]
	for start := 0; start < len(pending); start += messageMigrationBatchSize {
		end := start + messageMigrationBatchSize
		if end > len(pending) {
			end = len(pending)
		}

		chunk := append([]events.BatchEntry(nil), pending[start:end]...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = expectedSeq
		chunk[0].FilterSubject = agg.AllEventsFilter()

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
	return imported, len(existingEvents), nil
}

func userMigrationIdentity(event *corev1.Event) string {
	switch e := event.GetEvent().(type) {
	case *corev1.Event_UserAccountCreated:
		return strings.Join([]string{events.EventUserAccountCreated, e.UserAccountCreated.GetUserId(), strings.ToLower(e.UserAccountCreated.GetLogin()), e.UserAccountCreated.GetDisplayName()}, "\x00")
	case *corev1.Event_UserDisplayNameChanged:
		return events.EventUserDisplayNameChanged + "\x00" + e.UserDisplayNameChanged.GetUserId() + "\x00" + e.UserDisplayNameChanged.GetDisplayName()
	case *corev1.Event_UserPasswordHashChanged:
		return events.EventUserPasswordHashChanged + "\x00" + e.UserPasswordHashChanged.GetUserId() + "\x00" + string(e.UserPasswordHashChanged.GetPasswordHash())
	case *corev1.Event_UserAvatarSet:
		data, _ := proto.Marshal(e.UserAvatarSet.GetAvatar())
		return events.EventUserAvatarSet + "\x00" + e.UserAvatarSet.GetUserId() + "\x00" + hex.EncodeToString(data)
	case *corev1.Event_UserAvatarCleared:
		return events.EventUserAvatarCleared + "\x00" + e.UserAvatarCleared.GetUserId()
	case *corev1.Event_UserVerifiedEmailAdded:
		return events.EventUserVerifiedEmailAdded + "\x00" + e.UserVerifiedEmailAdded.GetUserId() + "\x00" + strings.ToLower(e.UserVerifiedEmailAdded.GetEmail())
	case *corev1.Event_UserOidcSubjectLinked:
		return events.EventUserOIDCSubjectLinked + "\x00" + e.UserOidcSubjectLinked.GetUserId() + "\x00" + e.UserOidcSubjectLinked.GetSubjectHash()
	case *corev1.Event_UserServerPreferencesChanged:
		data, _ := proto.Marshal(e.UserServerPreferencesChanged.GetPreferences())
		return events.EventUserServerPreferencesChanged + "\x00" + e.UserServerPreferencesChanged.GetUserId() + "\x00" + hex.EncodeToString(data)
	case *corev1.Event_UserLoginChanged:
		return events.EventUserLoginChanged + "\x00" + e.UserLoginChanged.GetUserId() + "\x00" + strings.ToLower(e.UserLoginChanged.GetLogin())
	case *corev1.Event_UserLoginCooldownStarted:
		return events.EventUserLoginCooldownStarted + "\x00" + e.UserLoginCooldownStarted.GetUserId()
	case *corev1.Event_UserLoginCooldownCleared:
		return events.EventUserLoginCooldownCleared + "\x00" + e.UserLoginCooldownCleared.GetUserId()
	}
	return events.EventTypeOf(event)
}
