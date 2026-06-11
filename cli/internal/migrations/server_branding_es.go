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
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	legacyServerLogoKey   = "instance.logo"
	legacyServerBannerKey = "instance.banner"
)

// MigrateServerBrandingToES imports legacy server logo/banner asset pointers
// from INSTANCE KV into semantic config events. The pointed-to asset bytes
// remain in object storage; only the pointer moves.
func MigrateServerBrandingToES(
	ctx context.Context,
	serverKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	seen, lastSeq, err := seenConfigEventTypes(ctx, publisher, events.ConfigSingletonID)
	if err != nil {
		return fmt.Errorf("read existing server config events: %w", err)
	}

	agg := events.ConfigAggregate()
	batch := make([]events.BatchEntry, 0, 2)
	add := func(kvKey, eventType, filename string, build func(*corev1.AssetRecord) *corev1.Event) error {
		if _, ok := seen[eventType]; ok {
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
		event := build(assetRecordFromLegacyBrandingAsset(asset, filename))
		event.Id = newMigrationEventID()
		event.ActorId = "system:migration"
		event.CreatedAt = timestamppb.New(entry.Created())
		batch = append(batch, events.BatchEntry{
			Subject: agg.SubjectFor(event),
			Event:   event,
		})
		return nil
	}

	if err := add(legacyServerLogoKey, events.EventServerLogoSet, "logo.webp", func(asset *corev1.AssetRecord) *corev1.Event {
		return &corev1.Event{Event: &corev1.Event_ServerLogoSet{
			ServerLogoSet: &corev1.ServerLogoSetEvent{Asset: asset},
		}}
	}); err != nil {
		return err
	}
	if err := add(legacyServerBannerKey, events.EventServerBannerSet, "banner.webp", func(asset *corev1.AssetRecord) *corev1.Event {
		return &corev1.Event{Event: &corev1.Event_ServerBannerSet{
			ServerBannerSet: &corev1.ServerBannerSetEvent{Asset: asset},
		}}
	}); err != nil {
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
	logger.Info("server_branding ES migration: seeded semantic config events from legacy KV", "values", len(batch), "duration_ms", time.Since(startedAt).Milliseconds())
	return nil
}

func assetRecordFromLegacyBrandingAsset(storage *corev1.DeprecatedAsset, filename string) *corev1.AssetRecord {
	if storage == nil {
		return nil
	}
	asset := &corev1.AssetRecord{
		Id:          legacyBrandingAssetID(storage),
		Filename:    filename,
		ContentType: "image/webp",
	}
	switch stored := storage.GetAsset().(type) {
	case *corev1.DeprecatedAsset_Nats:
		if stored.Nats != nil {
			asset.Storage = &corev1.AssetRecord_Nats{Nats: proto.Clone(stored.Nats).(*corev1.NATSAsset)}
		}
	case *corev1.DeprecatedAsset_S3:
		if stored.S3 != nil {
			asset.Storage = &corev1.AssetRecord_S3{S3: proto.Clone(stored.S3).(*corev1.S3Asset)}
		}
	}
	return asset
}

func legacyBrandingAssetID(storage *corev1.DeprecatedAsset) string {
	if storage == nil {
		return ""
	}
	switch stored := storage.GetAsset().(type) {
	case *corev1.DeprecatedAsset_Nats:
		return stored.Nats.GetKey()
	case *corev1.DeprecatedAsset_S3:
		return stored.S3.GetKey()
	default:
		return ""
	}
}
