package migrations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	legacyServerLogoKey   = "instance.logo"
	legacyServerBannerKey = "instance.banner"
)

// MigrateServerBrandingToES imports legacy server logo/banner asset pointers
// from INSTANCE KV into the generic config aggregate:
//
//   - instance.logo → server.logo
//   - instance.banner → server.banner
//
// The pointed-to asset bytes remain in object storage; only the pointer moves.
func MigrateServerBrandingToES(
	ctx context.Context,
	serverKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	seen, lastSeq, err := seenConfigPaths(ctx, publisher, events.ConfigSingletonID)
	if err != nil {
		return fmt.Errorf("read existing server config paths: %w", err)
	}

	agg := events.ConfigAggregate()
	batch := make([]events.BatchEntry, 0, 2)
	add := func(kvKey, path string) error {
		if _, ok := seen[path]; ok {
			return nil
		}
		entry, err := serverKV.Get(ctx, kvKey)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return nil
			}
			return fmt.Errorf("read legacy server branding %q: %w", kvKey, err)
		}
		asset := &corev1.DeprecatedAsset{}
		if err := proto.Unmarshal(entry.Value(), asset); err != nil {
			return fmt.Errorf("unmarshal legacy server branding %q: %w", kvKey, err)
		}
		event := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: timestamppb.New(entry.Created()),
			Event: &corev1.Event_ConfigValueSet{
				ConfigValueSet: &corev1.ConfigValueSetEvent{
					Subject: events.ConfigSingletonID,
					Path:    path,
					Value: &configv1.ConfigValue{
						Value: &configv1.ConfigValue_BytesValue{BytesValue: append([]byte(nil), entry.Value()...)},
					},
				},
			},
		}
		batch = append(batch, events.BatchEntry{
			Subject: agg.Subject(events.EventConfigValueSet),
			Event:   event,
		})
		return nil
	}

	if err := add(legacyServerLogoKey, "server.logo"); err != nil {
		return err
	}
	if err := add(legacyServerBannerKey, "server.banner"); err != nil {
		return err
	}
	if len(batch) == 0 {
		return nil
	}

	batch[0].ExpectedSeq = lastSeq
	batch[0].FilterSubject = agg.AllEventsFilter()
	batch[0].HasOCC = true
	startedAt := time.Now()
	if _, err := publisher.AppendBatch(ctx, batch); err != nil {
		if errors.Is(err, events.ErrConflict) {
			logger.Info("server_branding ES migration: config aggregate already changed, skipping")
			return nil
		}
		return err
	}
	logger.Info("server_branding ES migration: seeded generic config events from legacy KV", "values", len(batch), "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}
