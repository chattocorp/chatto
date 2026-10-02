package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModeratorEchoRemovalReauthorizes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(context.Context, *ChattoCore, string, string) error
		wantErr error
	}{
		{name: "manage revoked", wantErr: ErrPermissionDenied, change: func(ctx context.Context, c *ChattoCore, roomID, actorID string) error {
			return c.DenyUserRoomPermission(ctx, SystemActorID, roomID, actorID, PermMessageManage)
		}},
		{name: "membership removed", wantErr: ErrNotRoomMember, change: func(ctx context.Context, c *ChattoCore, roomID, actorID string) error {
			_, err := c.RemoveMember(ctx, SystemActorID, KindChannel, roomID, actorID)
			return err
		}},
		{name: "room conflict", change: func(ctx context.Context, c *ChattoCore, roomID, actorID string) error {
			_, err := c.PostMessage(ctx, KindChannel, roomID, actorID, "concurrent message", nil, "", "", nil, false)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			author, err := c.CreateUser(ctx, SystemActorID, "echo-author", "Author", "password123")
			require.NoError(t, err)
			moderator, err := c.CreateUser(ctx, SystemActorID, "echo-moderator", "Moderator", "password123")
			require.NoError(t, err)
			room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "echo-moderation", "")
			require.NoError(t, err)
			for _, userID := range []string{author.Id, moderator.Id} {
				_, err = c.JoinRoom(ctx, userID, KindChannel, userID, room.Id)
				require.NoError(t, err)
			}
			require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, moderator.Id, PermMessageManage))
			root, err := c.PostMessage(ctx, KindChannel, room.Id, author.Id, "root", nil, "", "", nil, false)
			require.NoError(t, err)
			reply, err := c.PostMessage(ctx, KindChannel, room.Id, author.Id, "reply", nil, root.Id, "", nil, true)
			require.NoError(t, err)
			echoID, ok := c.ChannelEchoEventID(reply.Id)
			require.True(t, ok)
			checks := 0
			err = c.EditMessage(ctx, moderator.Id, KindChannel, room.Id, reply.Id, "moderated reply",
				WithMessageChannelEcho(false), withEditMessageAuthorization(),
				withEditMessageCommitAuthorization(func(attemptCtx context.Context) error {
					checks++
					if checks == 1 {
						return tc.change(attemptCtx, c, room.Id, moderator.Id)
					}
					return nil
				}),
			)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				assertNoMessageMutationEvents(t, c, ctx, room.Id)
				require.False(t, c.IsHiddenChannelEcho(echoID))
				body, err := c.GetMessageBody(ctx, reply.Id)
				require.NoError(t, err)
				require.Equal(t, "reply", body)
			} else {
				require.NoError(t, err)
				require.GreaterOrEqual(t, checks, 2, "room conflict must repeat authorization")
				require.True(t, c.IsHiddenChannelEcho(echoID))
				body, err := c.GetMessageBody(ctx, reply.Id)
				require.NoError(t, err)
				require.Equal(t, "moderated reply", body)
			}
		})
	}
}
