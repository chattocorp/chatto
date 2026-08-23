package connectapi

import (
	"testing"

	"connectrpc.com/connect"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestRoomDirectoryServiceArchivesAndRestoresDMForViewer(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-archive-other", "Directory Archive Other", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "archive me", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	if _, err := env.directory.ArchiveDM(env.ctx, connect.NewRequest(&apiv1.ArchiveDMRequest{RoomId: dm.Id})); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("unauthenticated ArchiveDM code = %v, want unauthenticated", connect.CodeOf(err))
	}
	archiveResponse, err := env.directory.ArchiveDM(ctx, connect.NewRequest(&apiv1.ArchiveDMRequest{RoomId: dm.Id}))
	if err != nil {
		t.Fatalf("ArchiveDM: %v", err)
	}
	if !archiveResponse.Msg.GetRoom().GetViewerState().GetIsDmArchived() {
		t.Fatalf("ArchiveDM viewer state = %+v, want is_dm_archived=true", archiveResponse.Msg.GetRoom().GetViewerState())
	}

	listResponse, err := env.directory.ListRooms(ctx, connect.NewRequest(&apiv1.ListRoomsRequest{Scope: apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_DMS}))
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	listed := directoryRoomsByID(listResponse.Msg.GetRooms())[dm.Id]
	if listed == nil || !listed.GetViewerState().GetIsDmArchived() {
		t.Fatalf("archived DM missing from canonical directory list: %+v", listed)
	}

	unarchiveResponse, err := env.directory.UnarchiveDM(ctx, connect.NewRequest(&apiv1.UnarchiveDMRequest{RoomId: dm.Id}))
	if err != nil {
		t.Fatalf("UnarchiveDM: %v", err)
	}
	if unarchiveResponse.Msg.GetRoom().GetViewerState().GetIsDmArchived() {
		t.Fatalf("UnarchiveDM viewer state = %+v, want is_dm_archived=false", unarchiveResponse.Msg.GetRoom().GetViewerState())
	}
	if _, err := env.directory.UnarchiveDM(ctx, connect.NewRequest(&apiv1.UnarchiveDMRequest{RoomId: dm.Id})); err != nil {
		t.Fatalf("idempotent UnarchiveDM: %v", err)
	}
}

func TestRoomDirectoryServiceAutomaticallyRestoresArchivedDMAfterRootMessage(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-auto-restore", "Directory Auto Restore", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "first", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}
	if _, err := env.directory.ArchiveDM(ctx, connect.NewRequest(&apiv1.ArchiveDMRequest{RoomId: dm.Id})); err != nil {
		t.Fatalf("ArchiveDM: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, other.Id, "new", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage new: %v", err)
	}
	roomResponse, err := env.directory.GetRoom(ctx, connect.NewRequest(&apiv1.GetRoomRequest{RoomId: dm.Id}))
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if roomResponse.Msg.GetRoom().GetViewerState().GetIsDmArchived() {
		t.Fatalf("new root message left DM archived: %+v", roomResponse.Msg.GetRoom().GetViewerState())
	}
}

func TestRoomDirectoryServiceRejectsInvalidDMArchiveTargets(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	channel := env.createJoinedRoom("directory-archive-channel")
	if _, err := env.directory.ArchiveDM(ctx, connect.NewRequest(&apiv1.ArchiveDMRequest{RoomId: channel.Id})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ArchiveDM channel code = %v, want invalid argument", connect.CodeOf(err))
	}

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-empty-archive", "Directory Empty Archive", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	emptyDM, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := env.directory.ArchiveDM(ctx, connect.NewRequest(&apiv1.ArchiveDMRequest{RoomId: emptyDM.Id})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ArchiveDM empty code = %v, want invalid argument", connect.CodeOf(err))
	}

	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "directory-archive-outsider", "Directory Archive Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := env.directory.UnarchiveDM(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.UnarchiveDMRequest{RoomId: emptyDM.Id})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("UnarchiveDM outsider code = %v, want permission denied", connect.CodeOf(err))
	}
}
