package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageSearchReadModelResolvesAuthorizedScope(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chattoCore.CreateUser(ctx, SystemActorID, "search-viewer", "Search Viewer", "password")
	require.NoError(t, err)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "search-author", "Search Author", "password")
	require.NoError(t, err)
	visible, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-visible", "")
	require.NoError(t, err)
	archived, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-archived", "")
	require.NoError(t, err)
	unicodeRoom, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "Team chat 💬", "")
	require.NoError(t, err)
	hidden, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-hidden", "")
	require.NoError(t, err)
	for _, roomID := range []string{visible.Id, archived.Id, unicodeRoom.Id} {
		_, err = chattoCore.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, roomID)
		require.NoError(t, err)
	}
	dm, _, err := chattoCore.FindOrCreateDM(ctx, viewer.Id, []string{author.Id})
	require.NoError(t, err)
	_, err = chattoCore.PostMessage(ctx, KindDM, dm.Id, author.Id, "searchable direct message", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NoError(t, chattoCore.DenyUserRoomPermission(ctx, SystemActorID, dm.Id, viewer.Id, PermMessageRead))
	_, err = chattoCore.ArchiveRoom(ctx, SystemActorID, KindChannel, archived.Id)
	require.NoError(t, err)
	require.NoError(t, chattoCore.DenyRoomPermission(ctx, SystemActorID, archived.Id, RoleEveryone, PermMessageRead))
	require.NoError(t, chattoCore.DenyRoomPermission(ctx, SystemActorID, archived.Id, RoleEveryone, PermMessageReadInteractions))

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: viewer.Id})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{visible.Id, unicodeRoom.Id, dm.Id}, scope.RoomIDs)
	require.NotContains(t, scope.RoomIDs, hidden.Id)
	require.NotContains(t, scope.RoomIDs, archived.Id)

	scope, err = chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{
		ActorID: viewer.Id, RoomSelectors: []string{"SEARCH-ARCHIVED"}, AuthorSelectors: []string{author.Login},
	})
	require.NoError(t, err)
	require.Empty(t, scope.RoomIDs)
	require.Equal(t, []string{author.Id}, scope.AuthorIDs)
	require.False(t, scope.NoMatches)

	scope, err = chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{
		ActorID: viewer.Id, RoomSelectors: []string{"Ｔｅａｍ chat 💬"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{unicodeRoom.Id}, scope.RoomIDs)
	require.False(t, scope.NoMatches)

	scope, err = chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{
		ActorID: viewer.Id, RoomID: hidden.Id, AuthorSelectors: []string{"missing-user"},
	})
	require.NoError(t, err)
	require.Empty(t, scope.RoomIDs)
	require.True(t, scope.NoMatches)
}

func TestMessageSearchReadModelHydratesThreadMessages(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chattoCore.CreateUser(ctx, SystemActorID, "search-thread-reader", "Search Thread Reader", "password")
	require.NoError(t, err)
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-thread-room", "")
	require.NoError(t, err)
	_, err = chattoCore.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
	require.NoError(t, err)
	root, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, viewer.Id, "thread root", nil, "", "", nil, false)
	require.NoError(t, err)
	reply, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, viewer.Id, "searchable thread reply", nil, root.Id, "", nil, false)
	require.NoError(t, err)
	body, retracted, ok := chattoCore.roomModel.latestBody(reply.Id)
	require.True(t, ok)
	require.False(t, retracted)

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: viewer.Id})
	require.NoError(t, err)
	results, err := chattoCore.MessageSearchReads().HydrateHits(ctx, viewer.Id, scope, []MessageSearchHit{{
		MessageID: reply.Id, RoomID: room.Id, BodyEventID: body.GetBodyEventId(), Score: 3.25,
	}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, KindChannel, results[0].Kind)
	require.Equal(t, root.Id, results[0].Event.GetMessagePosted().GetInThread())
	require.Equal(t, 3.25, results[0].Score)
}

func TestMessageSearchReadModelFiltersInteractionScopedHits(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	reader, err := chattoCore.CreateUser(ctx, SystemActorID, "search-interaction-reader", "Search Interaction Reader", "password")
	require.NoError(t, err)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "search-interaction-author", "Search Interaction Author", "password")
	require.NoError(t, err)
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-interaction-room", "")
	require.NoError(t, err)
	for _, userID := range []string{reader.GetId(), author.GetId()} {
		_, err = chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.GetId())
		require.NoError(t, err)
	}
	visibleRoot, err := chattoCore.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "visible interaction root", nil, "", "", nil, false)
	require.NoError(t, err)
	unrelatedRoot, err := chattoCore.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "unrelated search root", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NoError(t, chattoCore.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageRead))
	require.NoError(t, chattoCore.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions))
	mention, err := chattoCore.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "search this @search-interaction-reader", nil, visibleRoot.GetId(), "", nil, false)
	require.NoError(t, err)

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: reader.GetId()})
	require.NoError(t, err)
	require.Contains(t, scope.RoomIDs, room.GetId())
	hit := func(eventID string) MessageSearchHit {
		body, retracted, ok := chattoCore.roomModel.latestBody(eventID)
		require.True(t, ok)
		require.False(t, retracted)
		return MessageSearchHit{MessageID: eventID, RoomID: room.GetId(), BodyEventID: body.GetBodyEventId()}
	}
	results, err := chattoCore.MessageSearchReads().HydrateHits(ctx, reader.GetId(), scope, []MessageSearchHit{
		hit(unrelatedRoot.GetId()), hit(visibleRoot.GetId()), hit(mention.GetId()),
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Equal(t, []string{visibleRoot.GetId(), mention.GetId()}, []string{results[0].Event.GetId(), results[1].Event.GetId()})
}

func TestMessageSearchReadModelReauthorizesAndHydratesHits(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	viewer, err := chattoCore.CreateUser(ctx, SystemActorID, "search-reader", "Search Reader", "password")
	require.NoError(t, err)
	visible, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-readable", "")
	require.NoError(t, err)
	hidden, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-private", "")
	require.NoError(t, err)
	_, err = chattoCore.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, visible.Id)
	require.NoError(t, err)
	visibleMessage, err := chattoCore.PostMessage(ctx, KindChannel, visible.Id, viewer.Id, "visible search result", nil, "", "", nil, false)
	require.NoError(t, err)
	staleMessage, err := chattoCore.PostMessage(ctx, KindChannel, visible.Id, viewer.Id, "stale search result", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NoError(t, chattoCore.DeleteMessage(ctx, viewer.Id, KindChannel, visible.Id, staleMessage.Id))

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: viewer.Id})
	require.NoError(t, err)
	visibleBody, retracted, ok := chattoCore.roomModel.latestBody(visibleMessage.Id)
	require.True(t, ok)
	require.False(t, retracted)
	require.NotNil(t, visibleBody)
	results, err := chattoCore.MessageSearchReads().HydrateHits(ctx, viewer.Id, scope, []MessageSearchHit{
		{MessageID: visibleMessage.Id, RoomID: visible.Id, BodyEventID: visibleBody.GetBodyEventId()},
		{MessageID: visibleMessage.Id, RoomID: visible.Id, BodyEventID: visibleBody.GetBodyEventId()},
		{MessageID: staleMessage.Id, RoomID: visible.Id},
		{MessageID: "hidden-message", RoomID: hidden.Id},
		{MessageID: visibleMessage.Id, RoomID: hidden.Id},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, visibleMessage.Id, results[0].Event.GetId())
	require.NoError(t, chattoCore.EditMessage(ctx, viewer.Id, KindChannel, visible.Id, visibleMessage.Id, "edited body no longer matching"))
	results, err = chattoCore.MessageSearchReads().HydrateHits(ctx, viewer.Id, scope, []MessageSearchHit{{
		MessageID: visibleMessage.Id, RoomID: visible.Id, BodyEventID: visibleBody.GetBodyEventId(),
	}})
	require.NoError(t, err)
	require.Empty(t, results)
	currentBody, retracted, ok := chattoCore.roomModel.latestBody(visibleMessage.Id)
	require.True(t, ok)
	require.False(t, retracted)
	results, err = chattoCore.MessageSearchReads().HydrateHits(ctx, viewer.Id, scope, []MessageSearchHit{{
		MessageID: visibleMessage.Id, RoomID: visible.Id, BodyEventID: currentBody.GetBodyEventId(),
	}})
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.NoError(t, chattoCore.LeaveRoom(ctx, viewer.Id, KindChannel, viewer.Id, visible.Id))
	results, err = chattoCore.MessageSearchReads().HydrateHits(ctx, viewer.Id, scope, []MessageSearchHit{{MessageID: visibleMessage.Id, RoomID: visible.Id, BodyEventID: currentBody.GetBodyEventId()}})
	require.NoError(t, err)
	require.Empty(t, results)
}
