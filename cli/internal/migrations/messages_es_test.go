package migrations

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// =============================================================================
// Setup helpers
// =============================================================================

// messagesTestRig stands up a NATS server with both SERVER_EVENTS +
// SERVER_BODIES (legacy) and EVT (target), plus a Publisher pointed
// at EVT. Mirrors setupTestES but with the additional legacy stream
// and bucket the messages migration needs.
type messagesTestRig struct {
	ctx       context.Context
	js        jetstream.JetStream
	srcStream jetstream.Stream
	bodiesKV  jetstream.KeyValue
	evtStream jetstream.Stream
	publisher *events.Publisher
}

func legacyPosted(roomID, bodyKey string) *corev1.MessagePostedEvent {
	posted := &corev1.MessagePostedEvent{RoomId: roomID}
	if bodyKey != "" {
		var unknown []byte
		unknown = protowire.AppendTag(unknown, 3, protowire.BytesType)
		unknown = protowire.AppendString(unknown, bodyKey)
		posted.ProtoReflect().SetUnknown(unknown)
	}
	return posted
}

func setupMessagesRig(t *testing.T) *messagesTestRig {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	srcStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SERVER_EVENTS_TEST",
		Subjects: []string{"server.>"},
		Storage:  jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create SERVER_EVENTS stream: %v", err)
	}

	bodiesKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "SERVER_BODIES_TEST",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create SERVER_BODIES bucket: %v", err)
	}

	evtStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:               "EVT_TEST",
		Subjects:           []string{events.SubjectRoot + ">"},
		Storage:            jetstream.MemoryStorage,
		AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatalf("create EVT stream: %v", err)
	}

	publisher := events.NewPublisher(js, evtStream, log.New(io.Discard))

	return &messagesTestRig{
		ctx:       ctx,
		js:        js,
		srcStream: srcStream,
		bodiesKV:  bodiesKV,
		evtStream: evtStream,
		publisher: publisher,
	}
}

// publishLegacy publishes a *corev1.Event onto the legacy
// SERVER_EVENTS-shaped stream at the given subject. This simulates pre-ES
// legacy events for the import migration.
func (r *messagesTestRig) publishLegacy(t *testing.T, subject string, ev *corev1.Event) {
	t.Helper()
	data, err := proto.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := r.js.Publish(r.ctx, subject, data); err != nil {
		t.Fatalf("publish %s: %v", subject, err)
	}
}

// putBody puts a MessageBody into the legacy SERVER_BODIES bucket
// under the given compound key.
func (r *messagesTestRig) putBody(t *testing.T, key string, body *corev1.MessageBody) {
	t.Helper()
	data, err := proto.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if _, err := r.bodiesKV.Put(r.ctx, key, data); err != nil {
		t.Fatalf("put body %s: %v", key, err)
	}
}

// readEvent reads a single message from the EVT stream at the given
// sequence and returns the decoded Event proto.
func (r *messagesTestRig) readEvent(t *testing.T, seq uint64) *corev1.Event {
	t.Helper()
	msg, err := r.evtStream.GetMsg(r.ctx, seq)
	if err != nil {
		t.Fatalf("get evt seq %d: %v", seq, err)
	}
	var ev corev1.Event
	if err := proto.Unmarshal(msg.Data, &ev); err != nil {
		t.Fatalf("unmarshal evt seq %d: %v", seq, err)
	}
	return &ev
}

// =============================================================================
// Subject helper assertion
// =============================================================================

func TestIsMessageSubject(t *testing.T) {
	cases := []struct {
		subject string
		want    bool
	}{
		{"server.room.channel.R1.msg.E1", true},
		{"server.room.dm.R2.msg.E1", true},
		{"server.room.channel.R1.msg.ROOT.replies.E2", true},
		{"server.room.channel.R1.meta", false},
		{"evt.room.R1.message_posted", false},
		{"live.server.room.channel.R1.msg.E1", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsMessageSubject(c.subject); got != c.want {
			t.Errorf("IsMessageSubject(%q) = %v, want %v", c.subject, got, c.want)
		}
	}
}

// =============================================================================
// Migration behaviour
// =============================================================================

func TestMigrateMessagesToES_EmptySource(t *testing.T) {
	rig := setupMessagesRig(t)
	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Errorf("EVT should be empty after migration of empty source, got %d msgs", info.State.Msgs)
	}
}

func TestMigrateMessagesToES_ImportsBodiesEmbedded(t *testing.T) {
	rig := setupMessagesRig(t)

	// Pre-populate SERVER_BODIES with two message bodies.
	rig.putBody(t, "U1.M1-BODY", &corev1.MessageBody{
		AuthorId:        "U1",
		EncryptedBody:   []byte("ciphertext-1"),
		EncryptionNonce: []byte("nonce-1"),
		CreatedAt:       timestamppb.Now(),
	})
	rig.putBody(t, "U2.M2-BODY", &corev1.MessageBody{
		AuthorId:        "U2",
		EncryptedBody:   []byte("ciphertext-2"),
		EncryptionNonce: []byte("nonce-2"),
		CreatedAt:       timestamppb.Now(),
	})

	// Pre-populate SERVER_EVENTS with two MessagePostedEvent envelopes.
	rig.publishLegacy(t, "server.room.channel.R1.msg.M1", &corev1.Event{
		Id:        "M1",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.M1-BODY"),
		},
	})
	rig.publishLegacy(t, "server.room.channel.R1.msg.M2", &corev1.Event{
		Id:        "M2",
		ActorId:   "U2",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U2.M2-BODY"),
		},
	})

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 4 {
		t.Fatalf("EVT msg count = %d, want 4", info.State.Msgs)
	}

	checkImportedBodyEvent(t, rig.readEvent(t, 1), "message_body.M1", "M1", "U1", "ciphertext-1")
	checkImportedMessage(t, rig.readEvent(t, 2), "M1", "U1")
	checkImportedBodyEvent(t, rig.readEvent(t, 3), "message_body.M2", "M2", "U2", "ciphertext-2")
	checkImportedMessage(t, rig.readEvent(t, 4), "M2", "U2")
}

func TestMigrateMessagesToES_ImportsThreadReplies(t *testing.T) {
	rig := setupMessagesRig(t)

	rig.putBody(t, "U1.ROOT-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("root")})
	rig.putBody(t, "U2.R1-BODY", &corev1.MessageBody{AuthorId: "U2", EncryptedBody: []byte("reply-1")})

	rig.publishLegacy(t, "server.room.channel.R1.msg.ROOT", &corev1.Event{
		Id:        "ROOT",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.ROOT-BODY"),
		},
	})
	rig.publishLegacy(t, "server.room.channel.R1.msg.ROOT.replies.REPLY1", &corev1.Event{
		Id:        "REPLY1",
		ActorId:   "U2",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: func() *corev1.MessagePostedEvent {
				posted := legacyPosted("R1", "U2.R1-BODY")
				posted.InThread = "ROOT"
				posted.InReplyTo = "ROOT"
				return posted
			}(),
		},
	})

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 5 {
		t.Fatalf("EVT msg count = %d, want 5", info.State.Msgs)
	}

	checkImportedBodyEvent(t, rig.readEvent(t, 1), "message_body.ROOT", "ROOT", "U1", "root")
	root := rig.readEvent(t, 2)
	checkImportedMessage(t, root, "ROOT", "U1")
	if root.GetMessagePosted().GetInThread() != "" {
		t.Errorf("root in_thread = %q, want empty", root.GetMessagePosted().GetInThread())
	}

	created := rig.readEvent(t, 3)
	checkImportedThreadCreated(t, created, "R1", "ROOT", "U2")

	checkImportedBodyEvent(t, rig.readEvent(t, 4), "message_body.REPLY1", "REPLY1", "U2", "reply-1")
	reply := rig.readEvent(t, 5)
	checkImportedMessage(t, reply, "REPLY1", "U2")
	if got := reply.GetMessagePosted().GetInThread(); got != "ROOT" {
		t.Errorf("reply in_thread = %q, want ROOT", got)
	}
}

func TestMigrateMessagesToES_BackfillsMessageIdentityAndThreadRootFromEnvelopeOrSubject(t *testing.T) {
	rig := setupMessagesRig(t)

	rig.putBody(t, "U1.ROOT-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("root")})
	rig.putBody(t, "U2.REPLY-BODY", &corev1.MessageBody{AuthorId: "U2", EncryptedBody: []byte("reply")})

	// Legacy PostMessage did not persist a payload message ID; it lived on
	// the envelope and subject. This is the real pre-ES shape
	// imported from production-like data.
	rig.publishLegacy(t, "server.room.channel.R1.msg.ROOT", &corev1.Event{
		Id:        "ROOT",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.ROOT-BODY"),
		},
	})

	// Be defensive for even older/broken imports: if InThread is absent,
	// the legacy thread subject still carries the root event id.
	rig.publishLegacy(t, "server.room.channel.R1.msg.ROOT.replies.REPLY1", &corev1.Event{
		Id:        "REPLY1",
		ActorId:   "U2",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U2.REPLY-BODY"),
		},
	})

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	checkImportedBodyEvent(t, rig.readEvent(t, 1), "message_body.ROOT", "ROOT", "U1", "root")
	root := rig.readEvent(t, 2)
	checkImportedMessage(t, root, "ROOT", "U1")
	if got := root.GetMessagePosted().GetInThread(); got != "" {
		t.Errorf("root in_thread = %q, want empty", got)
	}

	created := rig.readEvent(t, 3)
	checkImportedThreadCreated(t, created, "R1", "ROOT", "U2")

	checkImportedBodyEvent(t, rig.readEvent(t, 4), "message_body.REPLY1", "REPLY1", "U2", "reply")
	reply := rig.readEvent(t, 5)
	checkImportedMessage(t, reply, "REPLY1", "U2")
	if got := reply.GetMessagePosted().GetInThread(); got != "ROOT" {
		t.Errorf("reply in_thread = %q, want ROOT", got)
	}
}

func TestMigrateMessagesToES_MissingBodyImportsAsNil(t *testing.T) {
	// Legacy hard-delete: body wiped from SERVER_BODIES, post envelope
	// still in SERVER_EVENTS. Migration imports the envelope with
	// body=nil; projection retains the audit-trail event.
	rig := setupMessagesRig(t)
	rig.publishLegacy(t, "server.room.channel.R1.msg.DELETED", &corev1.Event{
		Id:        "DELETED",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.GONE"),
		},
	})

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("EVT msg count = %d, want 1", info.State.Msgs)
	}
	got := rig.readEvent(t, 1)
	if got.GetMessagePosted().GetBody() != nil {
		t.Errorf("expected body=nil for missing-body import, got %+v", got.GetMessagePosted().GetBody())
	}
}

func TestMigrateMessagesToES_ReplayIsIdempotent(t *testing.T) {
	rig := setupMessagesRig(t)
	rig.putBody(t, "U1.M1-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("hello")})
	rig.publishLegacy(t, "server.room.channel.R1.msg.M1", &corev1.Event{
		Id:        "M1",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.M1-BODY"),
		},
	})

	for i := 0; i < 2; i++ {
		if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 2 {
		t.Errorf("EVT msg count after replay = %d, want 2 (idempotent at room level)", info.State.Msgs)
	}
}

func TestMigrateMessagesToES_ResumesPartiallyImportedRoom(t *testing.T) {
	rig := setupMessagesRig(t)
	rig.putBody(t, "U1.M1-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("one")})
	rig.putBody(t, "U1.M2-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("two")})

	rig.publishLegacy(t, "server.room.channel.R1.msg.M1", &corev1.Event{
		Id:        "M1",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.M1-BODY"),
		},
	})
	rig.publishLegacy(t, "server.room.channel.R1.msg.M2", &corev1.Event{
		Id:        "M2",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: legacyPosted("R1", "U1.M2-BODY"),
		},
	})

	// Simulate a previously imported first chunk. The migration should
	// verify the existing event ID prefix and resume at M2.
	if _, err := rig.publisher.Append(rig.ctx, "evt.room.R1.message_posted", &corev1.Event{
		Id:        "M1",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
		},
	}); err != nil {
		t.Fatalf("seed target event: %v", err)
	}

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 3 {
		t.Errorf("EVT msg count = %d, want 3 (resumed after M1)", info.State.Msgs)
	}
}

func TestMigrateMessagesToES_MismatchedExistingPrefixSkipsRoom(t *testing.T) {
	rig := setupMessagesRig(t)
	rig.publishLegacy(t, "server.room.channel.R1.msg.M1", &corev1.Event{
		Id:        "M1",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
		},
	})
	rig.publishLegacy(t, "server.room.channel.R1.msg.M2", &corev1.Event{
		Id:        "M2",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
		},
	})

	if _, err := rig.publisher.Append(rig.ctx, "evt.room.R1.message_posted", &corev1.Event{
		Id:        "DIFFERENT",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
		},
	}); err != nil {
		t.Fatalf("seed target event: %v", err)
	}

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 1 {
		t.Errorf("EVT msg count = %d, want 1 (mismatched room skipped)", info.State.Msgs)
	}
}

func TestMigrateMessagesToES_ChunksLargeRoom(t *testing.T) {
	rig := setupMessagesRig(t)
	const total = messageMigrationBatchSize + 3

	for i := 0; i < total; i++ {
		eventID := "M" + time.Unix(int64(i), 0).Format("150405")
		rig.publishLegacy(t, "server.room.channel.R1.msg."+eventID, &corev1.Event{
			Id:        eventID,
			ActorId:   "U1",
			CreatedAt: timestamppb.New(time.Unix(int64(i), 0)),
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
			},
		})
	}

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != total {
		t.Errorf("EVT msg count = %d, want %d", info.State.Msgs, total)
	}
}

func TestMigrateMessagesToES_MultipleRoomsIsolated(t *testing.T) {
	rig := setupMessagesRig(t)
	rig.putBody(t, "U1.A-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("in-R1")})
	rig.putBody(t, "U1.B-BODY", &corev1.MessageBody{AuthorId: "U1", EncryptedBody: []byte("in-R2")})

	rig.publishLegacy(t, "server.room.channel.R1.msg.A", &corev1.Event{
		Id: "A", ActorId: "U1", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{MessagePosted: legacyPosted("R1", "U1.A-BODY")},
	})
	rig.publishLegacy(t, "server.room.channel.R2.msg.B", &corev1.Event{
		Id: "B", ActorId: "U1", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{MessagePosted: legacyPosted("R2", "U1.B-BODY")},
	})

	if err := MigrateMessagesToES(rig.ctx, rig.srcStream, rig.bodiesKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}

	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 4 {
		t.Fatalf("EVT count = %d, want 4", info.State.Msgs)
	}

	r1, err := rig.evtStream.GetLastMsgForSubject(rig.ctx, "evt.room.R1.message_posted")
	if err != nil {
		t.Fatalf("get R1 last msg: %v", err)
	}
	r2, err := rig.evtStream.GetLastMsgForSubject(rig.ctx, "evt.room.R2.message_posted")
	if err != nil {
		t.Fatalf("get R2 last msg: %v", err)
	}
	if r1.Subject != "evt.room.R1.message_posted" || r2.Subject != "evt.room.R2.message_posted" {
		t.Errorf("subjects: r1=%s r2=%s", r1.Subject, r2.Subject)
	}
}

// =============================================================================
// Assertion helpers
// =============================================================================

func checkImportedBodyEvent(t *testing.T, ev *corev1.Event, wantEventID, wantMessageID, wantActor, wantCiphertext string) {
	t.Helper()
	if ev.GetId() != wantEventID {
		t.Errorf("envelope id = %q, want %q", ev.GetId(), wantEventID)
	}
	if ev.GetActorId() != wantActor {
		t.Errorf("actor id = %q, want %q", ev.GetActorId(), wantActor)
	}
	bodyEvent := ev.GetMessageBody()
	if bodyEvent == nil {
		t.Fatal("expected MessageBodyEvent payload")
	}
	if got := bodyEvent.GetEventId(); got != wantMessageID {
		t.Errorf("body target event_id = %q, want %q", got, wantMessageID)
	}
	body := bodyEvent.GetBody()
	if body == nil {
		t.Fatal("expected body on imported body event event")
	}
	if got := string(body.GetEncryptedBody()); got != wantCiphertext {
		t.Errorf("body ciphertext = %q, want %q", got, wantCiphertext)
	}
}

// checkImportedMessage asserts that an event is a bodyless MessagePostedEvent
// with the given envelope ID.
func checkImportedMessage(t *testing.T, ev *corev1.Event, wantEventID, wantActor string) {
	t.Helper()
	if ev.GetId() != wantEventID {
		t.Errorf("envelope id = %q, want %q", ev.GetId(), wantEventID)
	}
	if ev.GetActorId() != wantActor {
		t.Errorf("actor id = %q, want %q", ev.GetActorId(), wantActor)
	}
	posted := ev.GetMessagePosted()
	if posted == nil {
		t.Fatal("expected MessagePostedEvent payload")
	}
	if posted.GetBody() != nil {
		t.Fatal("expected imported MessagePostedEvent to be bodyless")
	}
}

func checkImportedThreadCreated(t *testing.T, ev *corev1.Event, wantRoomID, wantThreadRootID, wantActor string) {
	t.Helper()
	if ev.GetActorId() != wantActor {
		t.Errorf("actor id = %q, want %q", ev.GetActorId(), wantActor)
	}
	created := ev.GetThreadCreated()
	if created == nil {
		t.Fatal("expected ThreadCreatedEvent payload")
	}
	if got := created.GetRoomId(); got != wantRoomID {
		t.Errorf("room_id = %q, want %q", got, wantRoomID)
	}
	if got := created.GetThreadRootEventId(); got != wantThreadRootID {
		t.Errorf("thread_root_event_id = %q, want %q", got, wantThreadRootID)
	}
}
