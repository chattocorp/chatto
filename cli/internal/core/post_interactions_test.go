package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessagePostInteractions(t *testing.T) {
	for _, botAccount := range []bool{false, true} {
		for _, dm := range []bool{false, true} {
			name := "human/channel"
			if botAccount {
				name = "bot/channel"
			}
			if dm {
				name += "/dm"
			}
			t.Run(name, func(t *testing.T) {
				c, _ := setupTestCore(t)
				ctx := testContext(t)
				operator, err := c.CreateUser(ctx, SystemActorID, "post-operator", "Operator", "password123")
				require.NoError(t, err)
				require.NoError(t, c.AssignOwnerRole(ctx, operator.Id))
				author, err := c.CreateUser(ctx, SystemActorID, "post-author", "Author", "password123")
				require.NoError(t, err)
				actor, err := c.CreateUser(ctx, SystemActorID, "post-reader", "Reader", "password123")
				require.NoError(t, err)
				if botAccount {
					bot, err := c.CreateBot(ctx, author.Id, "post_bot", "Bot")
					require.NoError(t, err)
					actor = bot.User
				}
				room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "post-interactions", "")
				require.NoError(t, err)
				kind := KindChannel
				scope := PermissionTargetScope{Kind: MatrixScopeRoom, ID: room.Id}
				if dm {
					room, _, err = c.RoomCommands().StartDM(ctx, RoomStartDMInput{ActorID: author.Id, ParticipantIDs: []string{actor.Id}})
					require.NoError(t, err)
					kind = KindDM
					scope = PermissionTargetScope{Kind: MatrixScopeDM}
				} else {
					for _, id := range []string{author.Id, actor.Id} {
						_, err := c.AddMember(ctx, SystemActorID, kind, room.Id, id)
						require.NoError(t, err)
					}
				}
				set := func(userID string, permission Permission, state PermissionState) {
					t.Helper()
					if botAccount && userID == actor.Id && state == PermissionStateDeny {
						state = PermissionStateNone
					}
					require.NoError(t, c.SetUserPermissionState(ctx, operator.Id, userID, scope, permission, state))
				}
				set(actor.Id, PermMessagePost, PermissionStateDeny)
				set(actor.Id, PermMessagePostInThread, PermissionStateDeny)
				set(actor.Id, PermMessagePostInteractions, PermissionStateAllow)
				set(actor.Id, PermMessageRead, PermissionStateAllow)
				root, err := c.PostMessage(ctx, kind, room.Id, author.Id, "context", nil, "", "", nil, false)
				require.NoError(t, err)
				post := func(rootID string) error {
					_, err := c.Messages().PostMessage(ctx, MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "reply", ThreadRootEventID: rootID})
					return err
				}
				require.ErrorIs(t, post(""), ErrPermissionDenied, "interaction posting cannot start a root")
				if !dm {
					require.ErrorIs(t, post(root.Id), ErrPermissionDenied, "broad read does not authorize unrelated replies")
					_, err = c.PostMessage(ctx, kind, room.Id, actor.Id, "@"+actor.Login, nil, root.Id, "", nil, false)
					require.NoError(t, err)
					require.ErrorIs(t, post(root.Id), ErrPermissionDenied, "authored replies and self mentions create no relationship")
					_, err = c.PostMessage(ctx, kind, room.Id, author.Id, "@"+actor.Login, nil, root.Id, "", nil, false)
					require.NoError(t, err)
				}
				require.NoError(t, post(root.Id), "direct mentions and received DMs authorize replies")
				reply, err := c.PostMessage(ctx, kind, room.Id, author.Id, "more context", nil, root.Id, "", nil, false)
				require.NoError(t, err)
				_, err = c.Messages().PostMessage(ctx, MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "inferred reply", InReplyTo: reply.Id})
				require.NoError(t, err, "attribution to a thread reply uses the same interaction gate")
				_, err = c.Messages().PostMessage(ctx, MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "echo", ThreadRootEventID: root.Id, AlsoSendToChannel: true})
				require.ErrorIs(t, err, ErrPermissionDenied, "interaction posting cannot bypass room posting through an echo")
				ownRoot, err := c.PostMessage(ctx, kind, room.Id, actor.Id, "authored root", nil, "", "", nil, false)
				require.NoError(t, err)
				require.NoError(t, post(ownRoot.Id), "authored roots reuse the existing relationship")
				set(actor.Id, PermMessageRead, PermissionStateDeny)
				set(actor.Id, PermMessageReadInteractions, PermissionStateDeny)
				require.ErrorIs(t, post(root.Id), ErrPermissionDenied, "posting does not grant read access")
				set(actor.Id, PermMessageReadInteractions, PermissionStateAllow)
				require.NoError(t, post(root.Id))
				if botAccount {
					set(author.Id, PermMessagePost, PermissionStateDeny)
					set(author.Id, PermMessagePostInteractions, PermissionStateDeny)
					require.ErrorIs(t, post(root.Id), ErrPermissionDenied, "owner ceiling is required")
					set(author.Id, PermMessagePostInteractions, PermissionStateAllow)
					require.NoError(t, post(root.Id), "owner can delegate the narrow permission")
				}
				set(actor.Id, PermMessagePostInteractions, PermissionStateDeny)
				require.ErrorIs(t, post(root.Id), ErrPermissionDenied, "revocation closes posting")
				if botAccount {
					set(author.Id, PermMessagePost, PermissionStateAllow)
				}
				set(actor.Id, PermMessagePost, PermissionStateAllow)
				require.NoError(t, post(root.Id), "broad posting includes both reply permissions despite narrow denies")
			})
		}
	}
}
