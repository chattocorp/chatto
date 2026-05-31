package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	legacyUserPreferencesPrefix     = "user_preferences."
	legacyRoomUserPreferencesPrefix = "room_user_preferences."
)

// MigrateNotificationPreferencesToES imports legacy notification preferences
// from SERVER_CONFIG into semantic user config events.
//
// Legacy keys:
//   - user_preferences.{userId} → corev1.UserPreferences
//   - room_user_preferences.{userId}.{roomId} → corev1.RoomUserPreferences
//
// Idempotency: each user subject is imported as one atomic batch guarded by
// wildcard OCC against evt.config.{userId}.>. Previously migrated server or
// room notification events are skipped individually.
func MigrateNotificationPreferencesToES(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	keys, err := listSortedKeys(ctx, serverConfigKV, legacyUserPreferencesPrefix+"*", legacyRoomUserPreferencesPrefix+"*.*")
	if err != nil {
		return fmt.Errorf("list legacy notification preference keys: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}

	byUser := map[string][]events.BatchEntry{}
	for _, key := range keys {
		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("read legacy notification preference %q: %w", key, err)
		}

		userID, roomID, level, ok, err := legacyNotificationPreferenceEntry(key, entry.Value())
		if err != nil {
			return err
		}
		if !ok || level == corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT {
			continue
		}

		event := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: timestamppb.New(entry.Created()),
		}
		if roomID == "" {
			event.Event = &corev1.Event_UserServerNotificationLevelSet{
				UserServerNotificationLevelSet: &corev1.UserServerNotificationLevelSetEvent{UserId: userID, Level: level},
			}
		} else {
			event.Event = &corev1.Event_UserRoomNotificationLevelSet{
				UserRoomNotificationLevelSet: &corev1.UserRoomNotificationLevelSetEvent{UserId: userID, RoomId: roomID, Level: level},
			}
		}
		agg := events.ConfigSubjectAggregate(userID)
		byUser[userID] = append(byUser[userID], events.BatchEntry{
			Subject: agg.SubjectFor(event),
			Event:   event,
		})
	}

	var imported, skipped int
	for userID, batch := range byUser {
		if len(batch) == 0 {
			continue
		}
		agg := events.ConfigSubjectAggregate(userID)
		existing, lastSeq, err := configSubjectEvents(ctx, publisher, userID)
		if err != nil {
			return fmt.Errorf("read existing notification config for user %s: %w", userID, err)
		}
		batch = skipSeenNotificationEvents(existing, batch)
		if len(batch) == 0 {
			continue
		}
		batch[0].ExpectedSeq = lastSeq
		batch[0].FilterSubject = agg.AllEventsFilter()
		batch[0].HasOCC = true
		if _, err := publisher.AppendBatch(ctx, batch); err != nil {
			if errors.Is(err, events.ErrConflict) {
				skipped += len(batch)
				continue
			}
			return fmt.Errorf("seed notification preferences for user %s: %w", userID, err)
		}
		imported += len(batch)
	}
	if imported > 0 || skipped > 0 {
		logger.Info("notification preferences ES migration: seeded semantic config events", "imported", imported, "skipped", skipped)
	}
	return nil
}

func legacyNotificationPreferenceEntry(key string, data []byte) (userID, roomID string, level corev1.NotificationLevel, ok bool, err error) {
	if strings.HasPrefix(key, legacyUserPreferencesPrefix) {
		userID = strings.TrimPrefix(key, legacyUserPreferencesPrefix)
		if userID == "" || strings.Contains(userID, ".") {
			return "", "", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, false, nil
		}
		prefs := &corev1.UserPreferences{}
		if err := proto.Unmarshal(data, prefs); err != nil {
			return "", "", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, false, fmt.Errorf("unmarshal %s: %w", key, err)
		}
		return userID, "", prefs.GetNotificationLevel(), true, nil
	}

	if strings.HasPrefix(key, legacyRoomUserPreferencesPrefix) {
		suffix := strings.TrimPrefix(key, legacyRoomUserPreferencesPrefix)
		parts := strings.Split(suffix, ".")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, false, nil
		}
		prefs := &corev1.RoomUserPreferences{}
		if err := proto.Unmarshal(data, prefs); err != nil {
			return "", "", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, false, fmt.Errorf("unmarshal %s: %w", key, err)
		}
		return parts[0], parts[1], prefs.GetNotificationLevel(), true, nil
	}

	return "", "", corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT, false, nil
}

func skipSeenNotificationEvents(existing []*corev1.Event, batch []events.BatchEntry) []events.BatchEntry {
	serverSeen := false
	roomsSeen := make(map[string]struct{})
	for _, event := range existing {
		switch e := event.GetEvent().(type) {
		case *corev1.Event_UserServerNotificationLevelSet:
			serverSeen = true
		case *corev1.Event_UserServerNotificationLevelCleared:
			serverSeen = true
		case *corev1.Event_UserRoomNotificationLevelSet:
			roomsSeen[e.UserRoomNotificationLevelSet.GetRoomId()] = struct{}{}
		case *corev1.Event_UserRoomNotificationLevelCleared:
			roomsSeen[e.UserRoomNotificationLevelCleared.GetRoomId()] = struct{}{}
		}
	}
	filtered := batch[:0]
	for _, entry := range batch {
		switch e := entry.Event.GetEvent().(type) {
		case *corev1.Event_UserServerNotificationLevelSet:
			if serverSeen {
				continue
			}
		case *corev1.Event_UserRoomNotificationLevelSet:
			if _, ok := roomsSeen[e.UserRoomNotificationLevelSet.GetRoomId()]; ok {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
