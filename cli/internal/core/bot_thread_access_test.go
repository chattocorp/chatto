package core

import "testing"

func TestBotDirectMessageReadBoundaryInvalidatesAfterCapabilityRevocation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "bot-dm-boundary", "Bot DM Boundary", "password123")
	if err != nil {
		t.Fatal(err)
	}
	bot, err := chatto.CreateBot(ctx, SystemActorID, owner.GetId(), "boundary_dm_bot", "Boundary DM Bot", "Tests retained DM authorization.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chatto.SetBotCapabilities(ctx, SystemActorID, bot.GetId(), []string{string(ApplicationCapabilityDMMessageRead)}); err != nil {
		t.Fatal(err)
	}
	dm, _, err := chatto.RoomCommands().StartDM(ctx, RoomStartDMInput{
		ActorID: owner.GetId(), ParticipantIDs: []string{bot.GetId()},
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := chatto.AuthorizeBotDirectMessageReadAtBoundary(ctx, bot.GetId(), dm.GetId())
	if err != nil {
		t.Fatal(err)
	}
	rooms, listBoundary, err := chatto.ListBotDirectMessageRoomsAtBoundary(ctx, bot.GetId())
	if err != nil || len(rooms) != 1 || rooms[0].GetId() != dm.GetId() {
		t.Fatalf("ListBotDirectMessageRoomsAtBoundary = %+v, %v", rooms, err)
	}
	if _, err := chatto.SetBotCapabilities(ctx, SystemActorID, bot.GetId(), nil); err != nil {
		t.Fatal(err)
	}
	if unchanged, err := chatto.BotContentReadBoundaryUnchanged(ctx, boundary); err != nil || unchanged {
		t.Fatalf("DM read boundary unchanged = %v, error = %v after capability revocation; want false, nil", unchanged, err)
	}
	if unchanged, err := chatto.BotContentListBoundaryUnchanged(ctx, listBoundary); err != nil || unchanged {
		t.Fatalf("DM list boundary unchanged = %v, error = %v after capability revocation; want false, nil", unchanged, err)
	}
}

func TestBotThreadReadBoundaryDetectsAuthorizationInputsAdvancing(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "bot-thread-boundary", "Bot Thread Boundary", "password123")
	if err != nil {
		t.Fatal(err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "bot-thread-boundary", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chatto.JoinRoom(ctx, owner.GetId(), KindChannel, owner.GetId(), room.GetId()); err != nil {
		t.Fatal(err)
	}

	roomBoundary, err := chatto.prepareBotThreadReadBoundary(ctx, room.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), owner.GetId(), "advance room", nil, "", "", nil, false); err != nil {
		t.Fatal(err)
	}
	if unchanged, err := chatto.BotContentReadBoundaryUnchanged(ctx, roomBoundary); err != nil || unchanged {
		t.Fatalf("room boundary unchanged = %v, error = %v; want false, nil", unchanged, err)
	}

	authorizationBoundary, err := chatto.prepareBotThreadReadBoundary(ctx, room.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), owner.GetId(), PermRoomManage); err != nil {
		t.Fatal(err)
	}
	if unchanged, err := chatto.BotContentReadBoundaryUnchanged(ctx, authorizationBoundary); err != nil || unchanged {
		t.Fatalf("authorization boundary unchanged = %v, error = %v; want false, nil", unchanged, err)
	}
}

func TestRemoveBotThreadAccessAdvancesAuthorizationFence(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "bot-thread-owner", "Bot Thread Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := chatto.CreateUser(ctx, SystemActorID, "bot-thread-manager", "Bot Thread Manager", "password123")
	if err != nil {
		t.Fatal(err)
	}
	bot, err := chatto.CreateBot(ctx, SystemActorID, owner.GetId(), "fenced_thread_bot", "Fenced Thread Bot", "Tests fenced thread revocation.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chatto.SetBotCapabilities(ctx, SystemActorID, bot.GetId(), []string{string(ApplicationCapabilityThreadRead)}); err != nil {
		t.Fatal(err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "bot-thread-fence", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{owner.GetId(), manager.GetId()} {
		if _, err := chatto.JoinRoom(ctx, userID, KindChannel, userID, room.GetId()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := chatto.AddMember(ctx, owner.GetId(), KindChannel, room.GetId(), bot.GetId()); err != nil {
		t.Fatal(err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), manager.GetId(), PermRoomManage); err != nil {
		t.Fatal(err)
	}
	root, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), owner.GetId(), "please help @fenced_thread_bot", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, readBoundary, err := chatto.AuthorizeBotThreadContextAtBoundary(
		ctx, bot.GetId(), room.GetId(), root.GetId(), ApplicationCapabilityThreadRead,
	)
	if err != nil {
		t.Fatal(err)
	}
	contexts, listBoundary, err := chatto.ListBotThreadContextsAtBoundary(ctx, bot.GetId())
	if err != nil || len(contexts) != 1 || contexts[0].ThreadRootEventID != root.GetId() {
		t.Fatalf("ListBotThreadContextsAtBoundary = %+v, %v", contexts, err)
	}

	before, err := chatto.authorizationFenceSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := chatto.RemoveBotThreadAccess(ctx, manager.GetId(), bot.GetId(), room.GetId(), root.GetId())
	if err != nil || !removed {
		t.Fatalf("RemoveBotThreadAccess = %v, %v; want true, nil", removed, err)
	}
	after, err := chatto.authorizationFenceSeq(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatalf("authorization fence = %d after removal, want greater than %d", after, before)
	}
	if unchanged, err := chatto.BotContentReadBoundaryUnchanged(ctx, readBoundary); err != nil || unchanged {
		t.Fatalf("released read boundary unchanged = %v, error = %v after revocation; want false, nil", unchanged, err)
	}
	if unchanged, err := chatto.BotContentListBoundaryUnchanged(ctx, listBoundary); err != nil || unchanged {
		t.Fatalf("thread list boundary unchanged = %v, error = %v after revocation; want false, nil", unchanged, err)
	}
}
