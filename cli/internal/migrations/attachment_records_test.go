package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// setupBodiesAndRuntime stands up an embedded NATS server and returns
// the two KV buckets the BackfillAttachmentRecords migration touches:
// bodies (source AND destination — attachment records co-locate with
// message bodies in this bucket) and runtime (sentinel store).
func setupBodiesAndRuntime(t *testing.T) (context.Context, jetstream.KeyValue, jetstream.KeyValue) {
	t.Helper()

	ns, err := server.NewServer(&server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	bodies, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "BODIES", Storage: jetstream.MemoryStorage})
	if err != nil {
		t.Fatalf("create bodies KV: %v", err)
	}
	runtime, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RUNTIME", Storage: jetstream.MemoryStorage})
	if err != nil {
		t.Fatalf("create runtime KV: %v", err)
	}

	return ctx, bodies, runtime
}

func TestBackfillAttachmentRecords_CopiesAttachmentsFromBodies(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	body := &corev1.MessageBody{
		AuthorId: "user-1",
		Attachments: []*corev1.Attachment{
			{Id: "att-a", RoomId: "room-x", Filename: "a.png"},
			{Id: "att-b", RoomId: "room-y", Filename: "b.png"},
		},
	}
	raw, err := proto.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if _, err := bodies.Put(ctx, "user-1.body-1", raw); err != nil {
		t.Fatalf("seed bodies: %v", err)
	}

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("BackfillAttachmentRecords: %v", err)
	}

	cases := []struct {
		key      string
		wantRoom string
		wantName string
	}{
		{"attachment.room-x.att-a", "room-x", "a.png"},
		{"attachment.room-y.att-b", "room-y", "b.png"},
	}
	for _, tc := range cases {
		entry, err := bodies.Get(ctx, tc.key)
		if err != nil {
			t.Fatalf("read %s: %v", tc.key, err)
		}
		var att corev1.Attachment
		if err := proto.Unmarshal(entry.Value(), &att); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.key, err)
		}
		if att.RoomId != tc.wantRoom {
			t.Errorf("%s: roomId=%q, want %q", tc.key, att.RoomId, tc.wantRoom)
		}
		if att.Filename != tc.wantName {
			t.Errorf("%s: filename=%q, want %q", tc.key, att.Filename, tc.wantName)
		}
	}

	if _, err := runtime.Get(ctx, "attachment_records.backfilled.v2"); err != nil {
		t.Errorf("expected backfill sentinel set: %v", err)
	}
}

func TestBackfillAttachmentRecords_Idempotent(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	body := &corev1.MessageBody{
		AuthorId:    "user-1",
		Attachments: []*corev1.Attachment{{Id: "att-a", RoomId: "room-x", Filename: "a.png"}},
	}
	raw, _ := proto.Marshal(body)
	if _, err := bodies.Put(ctx, "user-1.body-1", raw); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("second run: %v", err)
	}

	entry, err := bodies.Get(ctx, "attachment.room-x.att-a")
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var att corev1.Attachment
	if err := proto.Unmarshal(entry.Value(), &att); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if att.Filename != "a.png" {
		t.Errorf("filename: got %q, want %q", att.Filename, "a.png")
	}
}

func TestBackfillAttachmentRecords_EmptyBodiesSetsSentinel(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("BackfillAttachmentRecords: %v", err)
	}

	if _, err := runtime.Get(ctx, "attachment_records.backfilled.v2"); err != nil {
		t.Errorf("expected backfill sentinel set on empty bucket: %v", err)
	}
}

func TestBackfillAttachmentRecords_SkipsAttachmentWithoutRoomID(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	body := &corev1.MessageBody{
		AuthorId: "user-1",
		Attachments: []*corev1.Attachment{
			{Id: "att-good", RoomId: "room-x"},
			{Id: "att-stray"}, // no RoomId — should be skipped
		},
	}
	raw, _ := proto.Marshal(body)
	if _, err := bodies.Put(ctx, "user-1.body-1", raw); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("BackfillAttachmentRecords: %v", err)
	}

	if _, err := bodies.Get(ctx, "attachment.room-x.att-good"); err != nil {
		t.Errorf("expected record for att-good: %v", err)
	}
	// att-stray has no roomId so we wouldn't know what key to write.
	// Verify no orphan key got written under a wildcard.
	lister, err := bodies.ListKeysFiltered(ctx, "attachment.*.att-stray")
	if err == nil {
		for k := range lister.Keys() {
			t.Errorf("unexpected record key %q for att-stray", k)
		}
	}
}

// TestBackfillAttachmentRecords_IgnoresExistingAttachmentRecords is the
// "don't loop on yourself" check: since the migration writes attachment
// records into the same bucket it scans, a second pass must not try to
// unmarshal those keys as MessageBody and fail.
func TestBackfillAttachmentRecords_IgnoresExistingAttachmentRecords(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	// Pre-populate an attachment record as if a previous boot wrote one.
	preexisting := &corev1.Attachment{Id: "preexisting", RoomId: "room-x", Filename: "p.png"}
	raw, _ := proto.Marshal(preexisting)
	if _, err := bodies.Put(ctx, "attachment.room-x.preexisting", raw); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("BackfillAttachmentRecords: %v", err)
	}

	// Preexisting record untouched.
	entry, err := bodies.Get(ctx, "attachment.room-x.preexisting")
	if err != nil {
		t.Fatalf("read preexisting record: %v", err)
	}
	var got corev1.Attachment
	if err := proto.Unmarshal(entry.Value(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Filename != "p.png" {
		t.Errorf("preexisting record clobbered: filename=%q", got.Filename)
	}
}

// TestBackfillAttachmentRecords_IndexesVideoVariantsAndThumbnail covers
// the regression that triggered v2: the video service uploads variants
// and thumbnails as separate attachments referenced only via the
// VideoProcessingState in SERVER_RUNTIME, never embedded in any
// MessageBody. v1 missed them and they 404'd after upgrade.
func TestBackfillAttachmentRecords_IndexesVideoVariantsAndThumbnail(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	// Seed a body containing the original video attachment so the
	// migration's pass 1 captures its room ID.
	body := &corev1.MessageBody{
		AuthorId: "user-1",
		Attachments: []*corev1.Attachment{
			{Id: "video-orig", RoomId: "room-x", Filename: "clip.mp4", ContentType: "video/mp4"},
		},
	}
	bodyRaw, _ := proto.Marshal(body)
	if _, err := bodies.Put(ctx, "user-1.body-1", bodyRaw); err != nil {
		t.Fatalf("seed body: %v", err)
	}

	// Seed a VideoProcessingState pointing at thumbnail + variants.
	state := &corev1.VideoProcessingState{
		Status:                corev1.VideoStatus_VIDEO_STATUS_COMPLETED,
		ThumbnailAttachmentId: "video-thumb",
		Variants: []*corev1.VideoVariant{
			{AttachmentId: "video-360p", Quality: "360p"},
			{AttachmentId: "video-720p", Quality: "720p"},
		},
	}
	stateRaw, _ := proto.Marshal(state)
	if _, err := runtime.Put(ctx, "video.video-orig", stateRaw); err != nil {
		t.Fatalf("seed video state: %v", err)
	}

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("BackfillAttachmentRecords: %v", err)
	}

	// All three output records exist under the original's room.
	for _, id := range []string{"video-thumb", "video-360p", "video-720p"} {
		entry, err := bodies.Get(ctx, "attachment.room-x."+id)
		if err != nil {
			t.Errorf("expected record for %s: %v", id, err)
			continue
		}
		var att corev1.Attachment
		if err := proto.Unmarshal(entry.Value(), &att); err != nil {
			t.Errorf("unmarshal %s: %v", id, err)
			continue
		}
		if att.Id != id {
			t.Errorf("%s: record id=%q, want %q", id, att.Id, id)
		}
		if att.RoomId != "room-x" {
			t.Errorf("%s: record roomId=%q, want room-x", id, att.RoomId)
		}
	}
}

// TestBackfillAttachmentRecords_SkipsVideoOutputsForOrphanedOriginal
// guards against panics when the original video attachment is no
// longer in any body (e.g. GDPR-deleted) while its processing state
// still exists. The variants stay unindexed — acceptable given the
// content driving access is also gone.
func TestBackfillAttachmentRecords_SkipsVideoOutputsForOrphanedOriginal(t *testing.T) {
	ctx, bodies, runtime := setupBodiesAndRuntime(t)

	state := &corev1.VideoProcessingState{
		Status:                corev1.VideoStatus_VIDEO_STATUS_COMPLETED,
		ThumbnailAttachmentId: "orphan-thumb",
		Variants:              []*corev1.VideoVariant{{AttachmentId: "orphan-720p"}},
	}
	raw, _ := proto.Marshal(state)
	if _, err := runtime.Put(ctx, "video.orphan-original", raw); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := BackfillAttachmentRecords(ctx, bodies, runtime, log.New(nil)); err != nil {
		t.Fatalf("BackfillAttachmentRecords: %v", err)
	}

	// No records written under any room for the orphan outputs.
	lister, err := bodies.ListKeysFiltered(ctx, "attachment.*.orphan-thumb")
	if err == nil {
		for k := range lister.Keys() {
			t.Errorf("unexpected record key %q for orphan-thumb", k)
		}
	}
}
