package core

import (
	"bytes"
	"context"
	"testing"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// TestAttachmentBinaryStatus_TriState pins the three-way classification that
// protects against permanently tombstoning a video on a transient storage
// blip. The crucial distinction is Missing (storage definitively said "not
// there") vs Unknown (we couldn't reach storage): only the former is safe to
// turn into a durable SOURCE_MISSING outcome. If this ever collapses to a
// bool, a restore against unreachable/unconfigured storage would burn
// SOURCE_MISSING into EVT for every in-flight asset — irreversibly.
func TestAttachmentBinaryStatus_TriState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "r", "r")

	present, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}

	// Explicit NATS storage pointing at a key that was never written →
	// the object store returns ErrObjectNotFound (a definitive "gone").
	missing := &corev1.Attachment{
		Id:          "Amissing",
		RoomId:      room.Id,
		ContentType: "video/mp4",
		Storage: &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_Nats{Nats: &corev1.NATSAsset{Key: "Amissing-never-written"}},
		},
	}

	// S3 storage in a core with no S3 client → "S3 client not configured",
	// which is NOT a not-found. We can't tell whether the binary exists.
	unknown := &corev1.Attachment{
		Id:          "Aunknown",
		RoomId:      room.Id,
		ContentType: "video/mp4",
		Storage: &corev1.DeprecatedAsset{
			Asset: &corev1.DeprecatedAsset_S3{S3: &corev1.S3Asset{Key: "Aunknown"}},
		},
	}

	cases := []struct {
		name string
		att  *corev1.Attachment
		want AttachmentBinaryStatus
	}{
		{"present binary", present, AttachmentBinaryPresent},
		{"definitively missing binary", missing, AttachmentBinaryMissing},
		{"storage unreachable", unknown, AttachmentBinaryUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := core.attachmentBinaryStatus(ctx, tc.att); got != tc.want {
				t.Fatalf("attachmentBinaryStatus = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestScheduleVideoProcessing_BinaryStateDecision verifies the scheduler only
// tombstones (SOURCE_MISSING) when the source binary is definitively gone,
// and otherwise emits a Started marker and dispatches local work.
func TestScheduleVideoProcessing_BinaryStateDecision(t *testing.T) {
	t.Run("present binary → started + local work", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)
		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "r", "r")
		att, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
		if err != nil {
			t.Fatalf("UploadAttachment: %v", err)
		}

		requests := captureVideoProcessingRequests(t, core)

		if err := core.ScheduleVideoProcessingForMessageAttachment(ctx, SystemActorID, KindChannel, room.Id, "M-present", att); err != nil {
			t.Fatalf("schedule: %v", err)
		}

		manifest, ok := core.Assets.VideoAttachmentManifest(att.Id)
		if !ok || manifest.Started == nil {
			t.Fatalf("manifest = %+v, want Started", manifest)
		}
		if manifest.Failed != nil {
			t.Fatalf("present binary must not produce a failed manifest, got %+v", manifest.Failed)
		}
		select {
		case req := <-requests:
			if req.assetID != att.Id || req.messageEventID != "M-present" {
				t.Fatalf("request = %+v, want asset %q msg M-present", req, att.Id)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("expected local work for a present binary")
		}
	})

	t.Run("definitively missing binary → SOURCE_MISSING, no local work", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)
		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "r", "r")
		att, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
		if err != nil {
			t.Fatalf("UploadAttachment: %v", err)
		}
		// Delete the binary but leave the asset declared: a definitive "gone".
		if err := core.DeleteAttachmentFromStorage(ctx, att); err != nil {
			t.Fatalf("delete binary: %v", err)
		}

		requests := captureVideoProcessingRequests(t, core)

		if err := core.ScheduleVideoProcessingForMessageAttachment(ctx, SystemActorID, KindChannel, room.Id, "M-missing", att); err != nil {
			t.Fatalf("schedule: %v", err)
		}

		manifest, ok := core.Assets.VideoAttachmentManifest(att.Id)
		if !ok || manifest.Failed == nil {
			t.Fatalf("manifest = %+v, want Failed", manifest)
		}
		if manifest.Failed.GetFailureCode() != corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING {
			t.Fatalf("failure code = %v, want SOURCE_MISSING", manifest.Failed.GetFailureCode())
		}
		if manifest.Started != nil {
			t.Fatalf("missing binary must not emit a Started marker, got %+v", manifest.Started)
		}
		// The SOURCE_MISSING path returns before publishing; nothing was sent.
		select {
		case req := <-requests:
			t.Fatalf("missing binary must not dispatch local work, got %+v", req)
		default:
		}
	})
}

// TestRecoverUnmanifestedVideoAttachments_ReschedulesUnmanifested exercises the
// full boot-recovery path: a message-owned video asset with no terminal
// manifest (including a STARTED-only attempt interrupted by a crash) must be
// re-discovered and re-dispatched. This path was dead before message ownership
// was derived correctly, so it carries no other coverage.
func TestRecoverUnmanifestedVideoAttachments_ReschedulesUnmanifested(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General")
	user, _ := core.CreateUser(ctx, "system", "recuser", "recuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	att, err := core.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}

	// Post a message referencing the video WITHOUT scheduling processing, so
	// the asset is message-owned, video, and unmanifested — the exact shape
	// recovery is meant to pick up.
	posted, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Video", []string{att.Id}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	pending := core.assetLifecycle().UnmanifestedVideoAttachments()
	if len(pending) != 1 || pending[0].Attachment.GetId() != att.Id {
		t.Fatalf("UnmanifestedVideoAttachments = %+v, want %q", pending, att.Id)
	}

	requests := captureVideoProcessingRequests(t, core)

	core.RecoverUnmanifestedVideoAttachments(ctx)

	// Recovery must dispatch local work carrying the owning message id.
	select {
	case req := <-requests:
		if req.assetID != att.Id || req.messageEventID != posted.Id {
			t.Fatalf("recovered request = %+v, want asset %q msg %q", req, att.Id, posted.Id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected recovery to dispatch local work")
	}

	// A STARTED marker is not terminal. It remains recoverable after a process
	// restart; the service invokes this scan once per boot.
	manifest, ok := core.Assets.VideoAttachmentManifest(att.Id)
	if !ok || manifest.Started == nil {
		t.Fatalf("manifest after recovery = %+v, want Started", manifest)
	}
	if got := core.assetLifecycle().UnmanifestedVideoAttachments(); len(got) != 1 || got[0].Attachment.GetId() != att.Id {
		t.Fatalf("UnmanifestedVideoAttachments after Started = %+v, want %q", got, att.Id)
	}
}

type capturedVideoProcessingRequest struct {
	assetID        string
	messageEventID string
}

func captureVideoProcessingRequests(t *testing.T, core *ChattoCore) <-chan capturedVideoProcessingRequest {
	t.Helper()
	requests := make(chan capturedVideoProcessingRequest, 4)
	previous := core.OnVideoProcessingRequested
	core.OnVideoProcessingRequested = func(_ context.Context, assetID, messageEventID string) error {
		requests <- capturedVideoProcessingRequest{assetID: assetID, messageEventID: messageEventID}
		return nil
	}
	t.Cleanup(func() { core.OnVideoProcessingRequested = previous })
	return requests
}
