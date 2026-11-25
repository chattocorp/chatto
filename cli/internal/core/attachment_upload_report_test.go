package core

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAttachmentUploadProjection(t *testing.T) {
	projection := NewAssetProjection()
	midnight := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	creation := func(id, room, actor, owner string, ts time.Time, size int64) *evtv1.Event {
		event := testCoreAssetCreatedEvent(room, id, "text/plain")
		event.ActorId = actor
		event.CreatedAt = timestamppb.New(ts)
		event.GetAssetCreated().UserId = owner
		event.GetAssetCreated().Asset.Size = size
		return event
	}
	first := creation("a", "r", "u", "u", midnight.Add(-time.Second), 12)
	derivative := creation("b", "r", SystemActorID, "", midnight, 8)
	derivative.GetAssetCreated().ParentAssetId = "a"
	for i, event := range []*evtv1.Event{
		first, first, derivative,
		creation("avatar", "", "u", "u", midnight, 123),
		creation("import", "r", SystemActorID, "u", midnight, 5),
		creation("empty", "s", "u", "u", midnight, 0),
		{Id: "deleted", Event: &evtv1.Event_AssetDeleted{AssetDeleted: &evtv1.AssetDeletedEvent{AssetId: "a"}}},
	} {
		if err := projection.Apply(event, uint64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := projection.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored := NewAssetProjection()
	if err := restored.Restore(data); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*AssetProjection{projection, restored} {
		october := p.attachmentUploadMonth("2026-10")
		if october.GetRooms()["r"].GetBytes() != 12 || october.GetUsers()["u"].GetFiles() != 1 {
			t.Fatalf("October = %v", october)
		}
		november := p.attachmentUploadMonth("2026-11")
		if len(november.GetRooms()) != 2 || november.Rooms["r"].GetFiles() != 1 || november.Rooms["r"].GetBytes() != 5 || november.Rooms["s"].GetFiles() != 1 || november.Users[""].GetBytes() != 5 || november.Users["u"].GetFiles() != 1 {
			t.Fatalf("November = %v", november)
		}
	}
}

func TestGetAttachmentUploadsReport(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := core.CreateUser(ctx, SystemActorID, "report-owner", "Report Owner", "password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.GetAttachmentUploads(ctx, owner.Id, "2026-11"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("non-owner error = %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := core.GetAttachmentUploads(ctx, owner.Id, "2026-13"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid month error = %v", err)
	}
	empty, err := core.GetAttachmentUploads(ctx, owner.Id, "2001-03")
	if err != nil || empty.Total.Files != 0 || len(empty.Users)+len(empty.Rooms) != 0 {
		t.Fatalf("empty month = %#v, %v", empty, err)
	}
	channel, err := core.CreateRoom(ctx, owner.Id, KindChannel, "", "Reports", "")
	if err != nil {
		t.Fatal(err)
	}
	dm, _, err := core.FindOrCreateDM(ctx, owner.Id, []string{"another-user"})
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := core.UploadAttachment(ctx, owner.Id, channel.Id, "room.txt", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.UploadAttachment(ctx, SystemActorID, dm.Id, "dm.txt", "text/plain", strings.NewReader("hi")); err != nil {
		t.Fatal(err)
	}
	if err := core.assetModel.RecordAssetDeleted(ctx, owner.Id, channel.Id, attachment.Id); err != nil {
		t.Fatal(err)
	}
	report, err := core.GetAttachmentUploads(ctx, owner.Id, time.Now().UTC().Format("2006-01"))
	if err != nil {
		t.Fatal(err)
	}
	expected := &AttachmentUploadReport{
		Total: AttachmentUploadTotals{Bytes: 7, Files: 2},
		Rooms: []AttachmentUploadRow{
			{ID: channel.Id, Name: channel.Name, AttachmentUploadTotals: AttachmentUploadTotals{Bytes: 5, Files: 1}},
			{AttachmentUploadTotals: AttachmentUploadTotals{Bytes: 2, Files: 1}},
		},
		Users: []AttachmentUploadRow{
			{ID: owner.Id, Name: owner.DisplayName, AttachmentUploadTotals: AttachmentUploadTotals{Bytes: 5, Files: 1}},
			{AttachmentUploadTotals: AttachmentUploadTotals{Bytes: 2, Files: 1}},
		},
	}
	if !reflect.DeepEqual(report, expected) {
		t.Fatalf("got %+v, want %+v", report, expected)
	}
}
