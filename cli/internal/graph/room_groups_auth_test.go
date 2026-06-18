package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

func TestMoveRoomToGroupRequiresRoomManageInSourceAndTarget(t *testing.T) {
	tests := []struct {
		name            string
		grantSource     bool
		grantTarget     bool
		grantRoleManage bool
		wantErr         bool
	}{
		{
			name:        "source allowed target denied rejects",
			grantSource: true,
			wantErr:     true,
		},
		{
			name:        "source denied target allowed rejects",
			grantTarget: true,
			wantErr:     true,
		},
		{
			name:        "both allowed succeeds",
			grantSource: true,
			grantTarget: true,
		},
		{
			name:            "role manage alone rejects",
			grantRoleManage: true,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupTestResolver(t)
			mutation := env.resolver.Mutation()
			manager := env.createVerifiedUser(t, "room-move-manager", "Room Move Manager", "password123")
			sourceGroupID := env.testRoom.GroupId
			if sourceGroupID == "" {
				t.Fatal("expected test room to have a source group")
			}

			targetGroup, err := env.core.CreateRoomGroup(env.ctx, core.SystemActorID, "Move Target", "")
			if err != nil {
				t.Fatalf("CreateRoomGroup: %v", err)
			}
			if tt.grantSource {
				if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, sourceGroupID, manager.Id, core.PermRoomManage); err != nil {
					t.Fatalf("GrantUserGroupPermission(source): %v", err)
				}
			}
			if tt.grantTarget {
				if err := env.core.GrantUserGroupPermission(env.ctx, core.SystemActorID, targetGroup.Id, manager.Id, core.PermRoomManage); err != nil {
					t.Fatalf("GrantUserGroupPermission(target): %v", err)
				}
			}
			if tt.grantRoleManage {
				if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, manager.Id, core.PermRoleManage); err != nil {
					t.Fatalf("GrantUserPermission(role.manage): %v", err)
				}
			}

			room, err := mutation.MoveRoomToGroup(env.authContextForUser(manager), model.MoveRoomToGroupInput{
				RoomID:  env.testRoom.Id,
				GroupID: targetGroup.Id,
			})
			if tt.wantErr {
				if !errors.Is(err, core.ErrPermissionDenied) {
					t.Fatalf("MoveRoomToGroup err = %v, want ErrPermissionDenied", err)
				}
				current, getErr := env.core.GetRoom(env.ctx, core.KindChannel, env.testRoom.Id)
				if getErr != nil {
					t.Fatalf("GetRoom after rejected move: %v", getErr)
				}
				if current.GroupId != sourceGroupID {
					t.Fatalf("rejected move changed group to %q, want %q", current.GroupId, sourceGroupID)
				}
				return
			}

			if err != nil {
				t.Fatalf("MoveRoomToGroup: %v", err)
			}
			if room.GroupId != targetGroup.Id {
				t.Fatalf("moved room group = %q, want %q", room.GroupId, targetGroup.Id)
			}
		})
	}
}
