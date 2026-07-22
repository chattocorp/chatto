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
	hidden, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, "", "search-hidden", "")
	require.NoError(t, err)
	for _, roomID := range []string{visible.Id, archived.Id} {
		_, err = chattoCore.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, roomID)
		require.NoError(t, err)
	}
	dm, _, err := chattoCore.FindOrCreateDM(ctx, viewer.Id, []string{author.Id})
	require.NoError(t, err)
	_, err = chattoCore.PostMessage(ctx, KindDM, dm.Id, author.Id, "searchable direct message", nil, "", "", nil, false)
	require.NoError(t, err)
	_, err = chattoCore.ArchiveRoom(ctx, SystemActorID, KindChannel, archived.Id)
	require.NoError(t, err)

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: viewer.Id})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{visible.Id, archived.Id, dm.Id}, scope.RoomIDs)
	require.NotContains(t, scope.RoomIDs, hidden.Id)

	scope, err = chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{
		ActorID: viewer.Id, RoomSelectors: []string{"SEARCH-ARCHIVED"}, AuthorSelectors: []string{author.Login},
	})
	require.NoError(t, err)
	require.Equal(t, []string{archived.Id}, scope.RoomIDs)
	require.Equal(t, []string{author.Id}, scope.AuthorIDs)
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
	body, retracted, ok := chattoCore.RoomTimeline.LatestBody(reply.Id)
	require.True(t, ok)
	require.False(t, retracted)

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: viewer.Id})
	require.NoError(t, err)
	results, err := chattoCore.MessageSearchReads().HydrateHits(ctx, viewer.Id, scope, []MessageSearchHit{{
		MessageID: reply.Id, RoomID: room.Id, BodyEventID: body.GetBodyEventId(),
	}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, KindChannel, results[0].Kind)
	require.Equal(t, root.Id, results[0].Event.GetMessagePosted().GetInThread())
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
	visibleBody, retracted, ok := chattoCore.RoomTimeline.LatestBody(visibleMessage.Id)
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
	currentBody, retracted, ok := chattoCore.RoomTimeline.LatestBody(visibleMessage.Id)
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
