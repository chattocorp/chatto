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

func TestAssetProjectionOwnsMessageAssetReferences(t *testing.T) {
	projection := NewAssetProjection()
	bodyEvent := bodyEventWithAssets("E-body", "M1", "R1", "U1", "", []string{"A-video"}, 1)
	previewAssetID := "A-preview"
	bodyEvent.GetMessageBody().GetBody().LinkPreview = &corev1.LinkPreview{ImageAssetId: &previewAssetID}
	if err := projection.Apply(bodyEvent, 1); err != nil {
		t.Fatalf("Apply message body: %v", err)
	}

	roomID, messageID, ok := projection.AssetMessageOwner("A-video")
	if !ok || roomID != "R1" || messageID != "M1" {
		t.Fatalf("AssetMessageOwner = %q, %q, %v; want R1, M1, true", roomID, messageID, ok)
	}
	owned := projection.MessageAssetsByAuthor("U1")
	if len(owned) != 1 || owned[0].AssetID != "A-video" {
		t.Fatalf("MessageAssetsByAuthor = %+v, want A-video", owned)
	}
	if !projection.IsPublicLinkPreviewAsset("A-preview") {
		t.Fatal("IsPublicLinkPreviewAsset returned false")
	}

	if err := projection.Apply(&corev1.Event{
		Id: "E-deleted",
		Event: &corev1.Event_AssetDeleted{
			AssetDeleted: &corev1.AssetDeletedEvent{AssetId: "A-video"},
		},
	}, 2); err != nil {
		t.Fatalf("Apply asset deletion: %v", err)
	}
	roomID, messageID, ok = projection.AssetMessageOwner("A-video")
	if !ok || roomID != "R1" || messageID != "M1" {
		t.Fatalf("AssetMessageOwner after deletion = %q, %q, %v; want R1, M1, true", roomID, messageID, ok)
	}
}

func TestAssetProjectionVideoManifestTerminalStateDoesNotRegress(t *testing.T) {
	projection := NewAssetProjection()
	processed := &corev1.Event{
		Id: "E-video-ok",
		Event: &corev1.Event_AssetProcessingSucceeded{
			AssetProcessingSucceeded: &corev1.AssetProcessingSucceededEvent{
				AssetId: "A-video",
				Video: &corev1.AssetProcessedVideo{
					DurationMs: 1200,
					Variants: []*corev1.AssetVideoVariant{{
						Quality: "480p",
						AssetId: "A-video-480",
					}},
				},
			},
		},
	}
	failed := &corev1.Event{
		Id: "E-video-fail",
		Event: &corev1.Event_AssetProcessingFailed{
			AssetProcessingFailed: &corev1.AssetProcessingFailedEvent{
				AssetId:     "A-video",
				FailureCode: corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING,
			},
		},
	}

	applyAll(t, projection, []*corev1.Event{attachmentDeclaredEvent("R1", "A-video", "video/mp4"), processed, failed})
	manifest, ok := projection.VideoAttachmentManifest("A-video")
	if !ok || manifest.Succeeded == nil {
		t.Fatalf("VideoAttachmentManifest = %#v, want original processed manifest", manifest)
	}
	manifest.Succeeded.GetVideo().Variants[0].Quality = "mutated"
	again, _ := projection.VideoAttachmentManifest("A-video")
	if again.Succeeded.GetVideo().Variants[0].Quality != "480p" {
		t.Error("VideoAttachmentManifest should return clones")
	}
}

func TestAssetModelUnmanifestedVideoAttachmentsUsesAssetOwnershipAndTimelineTombstones(t *testing.T) {
	assets := NewAssetProjection()
	timeline := NewRoomTimelineProjection()
	model := NewAssetModel(&ChattoCore{Assets: assets, RoomTimeline: timeline})
	post := postedEvent(postedOpts{envelopeID: "M1", roomID: "R1", actorID: "U1", at: 1})
	body := bodyEventWithAssets("E-body", "M1", "R1", "U1", "", []string{"A-video"}, 2)
	applyAll(t, assets, []*corev1.Event{body, attachmentDeclaredEvent("R1", "A-video", "video/mp4")})
	applyAll(t, timeline, []*corev1.Event{post, body})

	got := model.UnmanifestedVideoAttachments()
	if len(got) != 1 || got[0].Attachment.GetId() != "A-video" {
		t.Fatalf("UnmanifestedVideoAttachments = %+v, want A-video", got)
	}

	retract := &corev1.Event{
		Id: "E-retract",
		Event: &corev1.Event_MessageRetracted{
			MessageRetracted: &corev1.MessageRetractedEvent{RoomId: "R1", EventId: "M1"},
		},
	}
	if err := timeline.Apply(retract, 3); err != nil {
		t.Fatalf("Apply retract: %v", err)
	}
	if got := model.UnmanifestedVideoAttachments(); len(got) != 0 {
		t.Fatalf("UnmanifestedVideoAttachments after retract = %+v, want none", got)
	}
}

func TestAssetProjectionRoomIDCycleGuardDoesNotHang(t *testing.T) {
	projection := NewAssetProjection()
	cyclicAsset := func(id, parentID string) *corev1.Event {
		return &corev1.Event{
			Id: "E-" + id,
			Event: &corev1.Event_AssetCreated{
				AssetCreated: &corev1.AssetCreatedEvent{
					Asset:         &corev1.AssetRecord{Id: id},
					ParentAssetId: parentID,
				},
			},
		}
	}
	applyAll(t, projection, []*corev1.Event{cyclicAsset("A", "B"), cyclicAsset("B", "A")})
	if roomID, ok := projection.AssetRoomID("A"); ok || roomID != "" {
		t.Fatalf("AssetRoomID for cyclic parents = %q, %v; want empty, false", roomID, ok)
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

func TestAssetProjectionApplyDoesNotMutateInputEvents(t *testing.T) {
	projection := NewAssetProjection()
	created := testCoreAssetCreatedEvent("R-assets", "A-source", "video/mp4")
	started := testCoreAssetProcessingStartedEvent("E-start-source", "A-source")
	assertApplyDoesNotMutateEvent(t, projection, created, 1)
	assertApplyDoesNotMutateEvent(t, projection, started, 2)
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
