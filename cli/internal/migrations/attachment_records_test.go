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

	if _, err := runtime.Get(ctx, "attachment_records.backfilled"); err != nil {
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

	if _, err := runtime.Get(ctx, "attachment_records.backfilled"); err != nil {
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
