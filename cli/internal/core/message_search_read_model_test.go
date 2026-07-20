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
	_, err = chattoCore.ArchiveRoom(ctx, SystemActorID, KindChannel, archived.Id)
	require.NoError(t, err)

	scope, err := chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{ActorID: viewer.Id})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{visible.Id, archived.Id}, scope.RoomIDs)
	require.NotContains(t, scope.RoomIDs, hidden.Id)

	scope, err = chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{
		ActorID: viewer.Id, RoomSelectors: []string{"SEARCH-ARCHIVED"}, AuthorSelectors: []string{author.Login},
	})
	require.NoError(t, err)
	require.Equal(t, []string{archived.Id}, scope.RoomIDs)
	require.Equal(t, []string{author.Id}, scope.AuthorIDs)
	require.False(t, scope.NoMatches)

	scope, err = chattoCore.MessageSearchReads().ResolveScope(ctx, MessageSearchScopeInput{
		ActorID: viewer.Id, RoomIDs: []string{hidden.Id}, AuthorSelectors: []string{"missing-user"},
	})
	require.NoError(t, err)
	require.Empty(t, scope.RoomIDs)
	require.True(t, scope.NoMatches)
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
