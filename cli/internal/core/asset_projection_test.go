package core

import (
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAssetProjectionReadsCanonicalAndLegacyLifecycleEvents(t *testing.T) {
	projection := NewAssetProjection()

	created := testCoreAssetCreatedEvent("R-assets", "A-source", "video/mp4")
	if err := projection.Apply(created, 10); err != nil {
		t.Fatalf("Apply canonical asset created: %v", err)
	}
	if got, ok := projection.AssetCreation("A-source"); !ok || got.GetRoomId() != "R-assets" {
		t.Fatalf("AssetCreation = %+v, %v; want room R-assets", got, ok)
	}

	started := &corev1.Event{
		Id: "E-started",
		Event: &corev1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{AssetId: "A-source"},
		},
	}
	// The projector now subscribes to evt.asset.>, but Apply intentionally does
	// not care which subscribed lane produced the event. That keeps legacy
	// evt.room.*.asset_* histories and new evt.asset.* histories equivalent.
	if err := projection.Apply(started, 11); err != nil {
		t.Fatalf("Apply lifecycle event: %v", err)
	}
	if manifest, ok := projection.VideoAttachmentManifest("A-source"); !ok || manifest.Started == nil {
		t.Fatalf("VideoAttachmentManifest = %+v, %v; want started", manifest, ok)
	}
}

func TestAssetProjectionTerminalProcessingStateDoesNotRegress(t *testing.T) {
	projection := NewAssetProjection()
	if err := projection.Apply(testCoreAssetCreatedEvent("R-assets", "A-video", "video/mp4"), 1); err != nil {
		t.Fatalf("Apply asset created: %v", err)
	}
	if err := projection.Apply(&corev1.Event{
		Id: "E-succeeded",
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{AssetId: "A-video"},
		},
	}, 2); err != nil {
		t.Fatalf("Apply succeeded: %v", err)
	}
	if err := projection.Apply(&corev1.Event{
		Id: "E-failed",
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{AssetId: "A-video"},
		},
	}, 3); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	manifest, ok := projection.VideoAttachmentManifest("A-video")
	if !ok || manifest.Succeeded == nil || manifest.Failed != nil {
		t.Fatalf("manifest = %#v, %v; want succeeded only", manifest, ok)
	}
}

func TestAssetProjectionDeletedAssetIgnoresLaterProcessing(t *testing.T) {
	projection := NewAssetProjection()
	if err := projection.Apply(testCoreAssetCreatedEvent("R-assets", "A-video", "video/mp4"), 1); err != nil {
		t.Fatalf("Apply asset created: %v", err)
	}
	if err := projection.Apply(&corev1.Event{
		Id: "E-deleted",
		Event: &corev1.Event_AssetDeleted{
			AssetDeleted: &corev1.AssetDeletedEvent{AssetId: "A-video"},
		},
	}, 2); err != nil {
		t.Fatalf("Apply deleted: %v", err)
	}
	if !projection.AssetDeleted("A-video") {
		t.Fatal("AssetDeleted returned false after deletion event")
	}
	if err := projection.Apply(&corev1.Event{
		Id: "E-stale-succeeded",
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{AssetId: "A-video"},
		},
	}, 3); err != nil {
		t.Fatalf("Apply stale succeeded: %v", err)
	}
	if manifest, ok := projection.VideoAttachmentManifest("A-video"); ok || manifest != nil {
		t.Fatalf("VideoAttachmentManifest after stale processing = %#v, %v; want none", manifest, ok)
	}
	if _, ok := projection.AssetCreation("A-video"); ok {
		t.Fatal("AssetCreation still present after deletion")
	}
}

func TestAssetAggregateSubjectHelpers(t *testing.T) {
	subject := events.AssetAggregate("A-123").Subject(events.EventAssetCreated)
	assetID, ok := events.ParseAssetSubject(subject)
	if !ok {
		t.Fatalf("ParseAssetSubject(%q) failed", subject)
	}
	if assetID != "A-123" {
		t.Fatalf("ParseAssetSubject = %q; want A-123", assetID)
	}
	if got := events.AssetSubjectFilter(); got != "evt.asset.>" {
		t.Fatalf("AssetSubjectFilter = %q, want evt.asset.>", got)
	}
}

func TestAssetProjectionReplayLimitCountsOnlyMemberRoomEvents(t *testing.T) {
	projection := NewAssetProjection()
	eventsToApply := []*corev1.Event{
		testCoreAssetCreatedEvent("R-other", "A-other", "video/mp4"),
		testCoreAssetProcessingStartedEvent("E-start-other", "A-other"),
		testCoreAssetCreatedEvent("R-member", "A-member", "video/mp4"),
		testCoreAssetProcessingStartedEvent("E-start-member", "A-member"),
	}
	for i, event := range eventsToApply {
		if err := projection.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("Apply event %d: %v", i, err)
		}
	}

	entries := projection.AssetEventsBetweenForRooms(
		0,
		4,
		map[string]struct{}{"R-member": {}},
		isDeliverableLiveEVTAssetEvent,
		1,
	)
	if len(entries) != 1 {
		t.Fatalf("AssetEventsBetweenForRooms entries = %d, want 1", len(entries))
	}
	if got := assetIDOfLifecycleEvent(entries[0].Event); got != "A-member" {
		t.Fatalf("replayed asset id = %q, want A-member", got)
	}
}

func testCoreAssetCreatedEvent(roomID, attachmentID, contentType string) *corev1.Event {
	return &corev1.Event{
		Id: "E-created-" + attachmentID,
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset: &corev1.AssetRecord{
					Id:          attachmentID,
					ContentType: contentType,
				},
				RoomId: roomID,
			},
		},
	}
}

func testCoreAssetProcessingStartedEvent(eventID, assetID string) *corev1.Event {
	return &corev1.Event{
		Id: eventID,
		Event: &corev1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{AssetId: assetID},
		},
	}
}
