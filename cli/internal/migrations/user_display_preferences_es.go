package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const legacyUserDisplayPreferencesPrefix = "user_preferences."

// MigrateUserDisplayPreferencesToES imports legacy per-user display settings
// from INSTANCE KV into each user's generic config subject:
//
//   - user_preferences.{userId}.timezone → preferences.timezone
//   - user_preferences.{userId}.time_format → preferences.time_format
func MigrateUserDisplayPreferencesToES(
	ctx context.Context,
	serverKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	keys, err := listSortedKeys(ctx, serverKV, legacyUserDisplayPreferencesPrefix+"*")
	if err != nil {
		return fmt.Errorf("list legacy user display preferences: %w", err)
	}
	var imported int
	startedAt := time.Now()
	for _, key := range keys {
		userID := strings.TrimPrefix(key, legacyUserDisplayPreferencesPrefix)
		if userID == "" {
			continue
		}
		entry, err := serverKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("read legacy user display preferences %q: %w", key, err)
		}
		prefs := &corev1.ServerUserPreferences{}
		if err := proto.Unmarshal(entry.Value(), prefs); err != nil {
			return fmt.Errorf("unmarshal legacy user display preferences %q: %w", key, err)
		}
		count, err := migrateOneUserDisplayPreferences(ctx, publisher, userID, prefs, entry.Created())
		if err != nil {
			return err
		}
		imported += count
	}
	if imported > 0 {
		logger.Info("user_display_preferences ES migration: seeded generic config events from legacy KV", "values", imported, "duration_ms", time.Since(startedAt).Milliseconds())
	}
	return nil
}

func migrateOneUserDisplayPreferences(
	ctx context.Context,
	publisher *events.Publisher,
	userID string,
	prefs *corev1.ServerUserPreferences,
	createdAt time.Time,
) (int, error) {
	seen, lastSeq, err := seenConfigPaths(ctx, publisher, userID)
	if err != nil {
		return 0, fmt.Errorf("read existing config paths for %s: %w", userID, err)
	}
	agg := events.ConfigSubjectAggregate(userID)
	batch := make([]events.BatchEntry, 0, 2)
	add := func(path string, value *configv1.ConfigValue) {
		if _, ok := seen[path]; ok {
			return
		}
		event := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: timestamppb.New(createdAt),
			Event: &corev1.Event_ConfigValueSet{
				ConfigValueSet: &corev1.ConfigValueSetEvent{
					Subject: userID,
					Path:    path,
					Value:   value,
				},
			},
		}
		batch = append(batch, events.BatchEntry{
			Subject: agg.Subject(events.EventConfigValueSet),
			Event:   event,
		})
	}

	if prefs.GetTimezone() != "" {
		add("preferences.timezone", &configv1.ConfigValue{
			Value: &configv1.ConfigValue_StringValue{StringValue: prefs.GetTimezone()},
		})
	}
	if prefs.GetTimeFormat() != corev1.TimeFormat_TIME_FORMAT_UNSPECIFIED {
		add("preferences.time_format", &configv1.ConfigValue{
			Value: &configv1.ConfigValue_IntValue{IntValue: int64(prefs.GetTimeFormat())},
		})
	}
	if len(batch) == 0 {
		return 0, nil
	}
	batch[0].ExpectedSeq = lastSeq
	batch[0].FilterSubject = agg.AllEventsFilter()
	batch[0].HasOCC = true
	if _, err := publisher.AppendBatch(ctx, batch); err != nil {
		if errors.Is(err, events.ErrConflict) {
			return 0, nil
		}
		return 0, fmt.Errorf("publish user display preferences for %s: %w", userID, err)
	}
	return len(batch), nil
}
