package core

import (
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestImportHistoricalMessageRoundTripWithoutPostingEffects(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chatto.CreateUser(ctx, SystemActorID, "historical-author", "Historical Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := chatto.CreateUser(ctx, SystemActorID, "historical-reader", "Historical Reader", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "historical-room", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{author.Id, reader.Id} {
		if _, err := chatto.AddMember(ctx, SystemActorID, KindChannel, room.Id, userID); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := chatto.PostMessage(ctx, KindChannel, room.Id, author.Id, "current message", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chatto.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, baseline.Id); err != nil {
		t.Fatal(err)
	}
	if err := chatto.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	beforeNotifications := len(testNotificationOccurrences(t, chatto, reader.Id))
	created := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Millisecond)
	edited := created.Add(time.Minute)
	preview := &evtv1.LinkPreview{Url: "https://example.invalid/post", Title: "Exported title", Description: "Exported description", EmbedType: "link"}
	root, err := chatto.ImportHistoricalMessage(ctx, HistoricalMessageInput{
		RoomID: room.Id, AuthorID: author.Id, CreatedAt: created, EditedAt: &edited,
		Body: "first\nsecond", LinkPreview: preview,
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := chatto.ImportHistoricalMessage(ctx, HistoricalMessageInput{
		RoomID: room.Id, AuthorID: reader.Id, CreatedAt: edited, InReplyTo: root.Id, Body: "reply",
	})
	if err != nil {
		t.Fatal(err)
	}
	if root.ActorId != SystemActorID || root.GetMessagePosted().GetAuthorId() != author.Id || !root.GetMessagePosted().GetHistoricalImport() {
		t.Fatalf("import attribution = %+v", root)
	}
	if reply.GetMessagePosted().GetInThread() != "" || reply.GetMessagePosted().GetInReplyTo() != root.Id {
		t.Fatalf("import reply routing = %+v", reply.GetMessagePosted())
	}
	body, err := chatto.GetFullMessageBody(ctx, root.Id)
	if err != nil {
		t.Fatal(err)
	}
	if body == nil || body.Body != "first\nsecond" || body.AuthorId != author.Id || !body.CreatedAt.Equal(created) || body.UpdatedAt == nil || !body.UpdatedAt.Equal(edited) || body.LinkPreview.GetTitle() != preview.Title {
		t.Fatalf("hydrated body = %+v", body)
	}
	page, err := chatto.GetRoomEvents(ctx, KindChannel, room.Id, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, event := range page.Events {
		if event.Id == root.Id || event.Id == reply.Id {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("timeline contains %d imported messages, want 2", found)
	}
	lastID, _, exists, err := chatto.GetRoomLastReadableEvent(ctx, KindChannel, reader.Id, room.Id)
	if err != nil || !exists || lastID != baseline.Id {
		t.Fatalf("last readable activity = (%q, %t, %v)", lastID, exists, err)
	}
	if unread, err := chatto.HasUnread(ctx, KindChannel, reader.Id, room.Id); err != nil || unread {
		t.Fatalf("unread after import = (%t, %v)", unread, err)
	}
	if err := chatto.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(testNotificationOccurrences(t, chatto, reader.Id)); got != beforeNotifications {
		t.Fatalf("notification count = %d, want %d", got, beforeNotifications)
	}
	events, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventThreadFollowed))
	if err != nil || len(events) != 0 {
		t.Fatalf("thread follows = %d, err = %v", len(events), err)
	}
	duplicate, err := chatto.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, CreatedAt: created, Body: "first\nsecond"})
	if err != nil || duplicate.Id == root.Id {
		t.Fatalf("repeat import ID = %q, err = %v", duplicate.GetId(), err)
	}
}

func TestImportHistoricalMessageRejectsInvalidInputs(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chatto.CreateUser(ctx, SystemActorID, "historical-invalid-author", "Historical Author", "password")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "historical-invalid-room", "")
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	base := HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, CreatedAt: created, Body: "hello"}
	badTime := created.Add(-time.Second)
	cases := []struct {
		name   string
		change func(*HistoricalMessageInput)
	}{
		{"missing room", func(v *HistoricalMessageInput) { v.RoomID = "missing" }},
		{"missing author", func(v *HistoricalMessageInput) { v.AuthorID = "missing" }},
		{"missing reply", func(v *HistoricalMessageInput) { v.InReplyTo = "missing" }},
		{"bad edit time", func(v *HistoricalMessageInput) { v.EditedAt = &badTime }},
		{"no content", func(v *HistoricalMessageInput) { v.Body = "" }},
		{"oversize body", func(v *HistoricalMessageInput) { v.Body = strings.Repeat("x", MaxMessageBodyLength+1) }},
		{"missing asset", func(v *HistoricalMessageInput) { v.AttachmentAssetIDs = []string{"missing"} }},
		{"duplicate asset", func(v *HistoricalMessageInput) { v.AttachmentAssetIDs = []string{"same", "same"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.change(&input)
			if _, err := chatto.ImportHistoricalMessage(ctx, input); err == nil {
				t.Fatal("invalid import succeeded")
			}
		})
	}
}

func TestHistoricalMessageProjectionRestoreKeepsActivitySuppressed(t *testing.T) {
	projection := NewRoomTimelineProjection()
	ordinary := newEvent("user", &evtv1.Event{Id: "ordinary", Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "room"}}})
	historical := newEvent(SystemActorID, &evtv1.Event{Id: "historical", Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "room", AuthorId: "user", HistoricalImport: true}}})
	if err := projection.Apply(ordinary, 1); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(historical, 2); err != nil {
		t.Fatal(err)
	}
	snapshot, err := projection.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored := NewRoomTimelineProjection()
	if err := restored.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	for _, current := range []*RoomTimelineProjection{projection, restored} {
		entry, ok := current.LastRoomMessageEntry("room")
		if !ok || entry.EventID != "ordinary" {
			t.Fatalf("last activity = %+v", entry)
		}
		imported, ok := current.Get("historical")
		if !ok || !imported.HistoricalImport {
			t.Fatalf("historical entry = %+v", imported)
		}
		if _, ok := current.LatestOriginalPostAt("room", SystemActorID); ok {
			t.Fatal("historical import affected slow mode")
		}
	}
	if isDeliverableLiveEVTRoomEvent(historical) {
		t.Fatal("historical import is live-deliverable")
	}
	shredded := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserKeyShredded{UserKeyShredded: &evtv1.UserKeyShreddedEvent{UserId: "user"}}})
	if err := restored.Apply(shredded, 3); err != nil {
		t.Fatal(err)
	}
	if !restored.MessageTombstoned("historical") {
		t.Fatal("author key shred did not tombstone imported message")
	}
}
