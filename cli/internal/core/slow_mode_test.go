package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
)

func TestRoomSlowModeConfiguration(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	manager, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-manager", "Slow Mode Manager", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, manager.Id, KindChannel, "", "slow-mode-config", "")
	require.NoError(t, err)
	require.NoError(t, chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage))

	seconds := uint32(30)
	updated, err := chatto.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
		ActorID: manager.Id, RoomID: room.Id, SlowModeSeconds: &seconds,
	})
	require.NoError(t, err)
	require.Equal(t, seconds, updated.GetSlowModeSeconds())

	events, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomSlowModeChanged))
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, seconds, events[0].GetRoomSlowModeChanged().GetSlowModeSeconds())

	_, err = chatto.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
		ActorID: manager.Id, RoomID: room.Id, SlowModeSeconds: &seconds,
	})
	require.NoError(t, err)
	events, _, err = chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomSlowModeChanged))
	require.NoError(t, err)
	require.Len(t, events, 1, "an unchanged interval must not write another event")

	tooLong := uint32(MaxRoomSlowModeSeconds + 1)
	_, err = chatto.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
		ActorID: manager.Id, RoomID: room.Id, SlowModeSeconds: &tooLong,
	})
	require.ErrorIs(t, err, ErrInvalidArgument)

	participant, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-dm-peer", "Slow Mode DM Peer", "password123")
	require.NoError(t, err)
	dm, _, err := chatto.FindOrCreateDM(ctx, manager.Id, []string{participant.Id})
	require.NoError(t, err)
	_, err = chatto.SetRoomSlowMode(ctx, manager.Id, KindDM, dm.Id, 5)
	require.ErrorIs(t, err, ErrInvalidArgument)
}

func TestMessageSlowModeEnforcementAndImmediateChanges(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-poster", "Slow Mode Poster", "password123")
	require.NoError(t, err)
	other, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-other", "Slow Mode Other", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "slow-mode-posting", "")
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, other.Id, KindChannel, other.Id, room.Id)
	require.NoError(t, err)

	first, err := chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "before slow mode"})
	require.NoError(t, err)
	room, err = chatto.SetRoomSlowMode(ctx, SystemActorID, KindChannel, room.Id, 60)
	require.NoError(t, err)

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "thread reply", ThreadRootEventID: first.Event.Id,
	})
	require.ErrorIs(t, err, ErrSlowModeActive, "root and thread posts must share one room timer")

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: other.Id, RoomID: room.Id, Body: "other user"})
	require.NoError(t, err, "slow mode is per user")

	next := first.Event.GetCreatedAt().AsTime().Add(time.Minute)
	require.Equal(t, next, chatto.Messages().slowModeNextPostAt(room, user.Id, false, next.Add(-time.Nanosecond)))
	require.True(t, chatto.Messages().slowModeNextPostAt(room, user.Id, false, next).IsZero(), "the exact expiry boundary must allow posting")

	room, err = chatto.SetRoomSlowMode(ctx, SystemActorID, KindChannel, room.Id, 1)
	require.NoError(t, err)
	require.True(t, chatto.Messages().slowModeNextPostAt(room, user.Id, false, first.Event.GetCreatedAt().AsTime().Add(time.Second)).IsZero(), "decreasing the interval applies immediately")
	room, err = chatto.SetRoomSlowMode(ctx, SystemActorID, KindChannel, room.Id, 0)
	require.NoError(t, err)
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "disabled"})
	require.NoError(t, err)
}

func TestMessageSlowModeBypassAndPermissionLoss(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		permission Permission
	}{{"room-manage", PermRoomManage}, {"message-manage", PermMessageManage}} {
		t.Run(testCase.name, func(t *testing.T) {
			chatto, _ := setupTestCore(t)
			ctx := testContext(t)
			user, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-bypass-"+testCase.name, "Slow Mode Bypass", "password123")
			require.NoError(t, err)
			room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "slow-bypass-"+testCase.name, "")
			require.NoError(t, err)
			_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
			require.NoError(t, err)
			_, err = chatto.SetRoomSlowMode(ctx, SystemActorID, KindChannel, room.Id, 60)
			require.NoError(t, err)
			require.NoError(t, chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, testCase.permission))

			_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "bypassed one"})
			require.NoError(t, err)
			_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "bypassed two"})
			require.NoError(t, err)

			require.NoError(t, chatto.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, user.Id, testCase.permission))
			_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "no longer bypassed"})
			require.ErrorIs(t, err, ErrSlowModeActive, "the latest bypassed post becomes effective when bypass is removed")
		})
	}
}

func TestMessageSlowModeCountsAttachmentOnlyPostsButNotAttachmentStaging(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-attachment", "Slow Mode Attachment", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "slow-mode-attachment", "")
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	_, err = chatto.SetRoomSlowMode(ctx, SystemActorID, KindChannel, room.Id, 60)
	require.NoError(t, err)

	first := uploadRoomAttachment(t, chatto, ctx, room.Id, "slow-mode-first.png")
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, AttachmentAssetIDs: []string{first.Id},
	})
	require.NoError(t, err, "staging an attachment must not start slow mode")

	second := uploadRoomAttachment(t, chatto, ctx, room.Id, "slow-mode-second.png")
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, AttachmentAssetIDs: []string{second.Id},
	})
	require.ErrorIs(t, err, ErrSlowModeActive, "an attachment-only message must start slow mode")
}

func TestMessageSlowModeConcurrentPostsUseRoomOCC(t *testing.T) {
	chatto, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "slow-mode-race", "Slow Mode Race", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "slow-mode-race", "")
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	_, err = chatto.SetRoomSlowMode(ctx, SystemActorID, KindChannel, room.Id, 60)
	require.NoError(t, err)
	replica, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	require.NoError(t, err)
	startCoreServices(t, replica)
	replicaRoom, err := replica.GetRoom(ctx, KindChannel, room.Id)
	require.NoError(t, err)
	require.Equal(t, uint32(60), replicaRoom.GetSlowModeSeconds())

	postingCores := []*ChattoCore{chatto, replica}
	input := MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "racing post"}
	for _, postingCore := range postingCores {
		_, err := postingCore.Messages().PreflightPost(ctx, input)
		require.NoError(t, err)
	}

	// Hold both replicas inside their first commit-time authorization callback.
	// prepareMessageAppendAttempt has captured the same room tail before calling
	// this hook, so releasing both attempts together deterministically forces one
	// append to conflict and retry against the winner's projected post.
	ready := make(chan struct{}, len(postingCores))
	release := make(chan struct{})
	attemptCounts := make([]int, len(postingCores))
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index, postingCore := range postingCores {
		wg.Add(1)
		go func(index int, core *ChattoCore) {
			defer wg.Done()
			authorize := func(attemptCtx context.Context, _ string) error {
				attemptCounts[index]++
				_, authErr := core.Messages().AuthorizePost(attemptCtx, MessagePostAuthorizationInput{
					ActorID: input.ActorID, RoomID: input.RoomID, Body: input.Body,
				})
				if authErr != nil {
					return authErr
				}
				if attemptCounts[index] == 1 {
					ready <- struct{}{}
					select {
					case <-release:
					case <-attemptCtx.Done():
						return attemptCtx.Err()
					}
				}
				return nil
			}
			_, postErr := core.PostMessage(
				ctx, KindChannel, room.Id, user.Id, input.Body, nil, "", "", nil, false,
				withPostMessageCommitAuthorization(authorize),
			)
			errs <- postErr
		}(index, postingCore)
	}
	for range postingCores {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(release)
	wg.Wait()
	close(errs)

	var succeeded, throttled int
	for postErr := range errs {
		switch {
		case postErr == nil:
			succeeded++
		case errors.Is(postErr, ErrSlowModeActive):
			throttled++
		default:
			t.Fatalf("unexpected concurrent post error: %v", postErr)
		}
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, 1, throttled)
	require.ElementsMatch(t, []int{1, 2}, attemptCounts, "the conflicting replica must rerun authorization")
}
