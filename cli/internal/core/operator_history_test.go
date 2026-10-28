package core

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestHistoricalThreadImport(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := c.CreateUser(ctx, SystemActorID, "thread-import-author", "Author", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "thread-import", "")
	require.NoError(t, err)
	created := time.Date(2025, 12, 28, 18, 21, 38, 872519000, time.UTC)
	for _, explicit := range []bool{false, true} {
		root, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "root", CreatedAt: created, CreateThread: explicit})
		require.NoError(t, err)
		if explicit {
			require.True(t, c.GetThreadMetadata(root.Id).Exists)
			require.Zero(t, c.GetThreadMetadata(root.Id).ReplyCount)
		}
		var replyIDs []string
		for i := range 4 {
			replyTime := created.Add(time.Duration(i+1) * 24 * time.Hour)
			reply, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "reply", CreatedAt: replyTime, ThreadRootEventID: root.Id, InReplyTo: root.Id})
			require.NoError(t, err)
			require.Equal(t, root.Id, reply.GetMessagePosted().GetInThread())
			require.Equal(t, author.Id, messageAuthorID(reply))
			require.Equal(t, replyTime, reply.CreatedAt.AsTime())
			replyIDs = append(replyIDs, reply.Id)
		}
		metadata := c.GetThreadMetadata(root.Id)
		require.Equal(t, 4, metadata.ReplyCount)
		require.Equal(t, []string{author.Id}, metadata.ParticipantIDs)
		require.Equal(t, replyIDs[3], metadata.LatestReplyEventID)
		page, err := c.GetThreadReplyEvents(ctx, KindChannel, room.Id, root.Id, 10, nil, nil)
		require.NoError(t, err)
		require.Len(t, page.Events, 4)
		channel, err := c.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
		require.NoError(t, err)
		for _, event := range channel.Events {
			require.NotContains(t, replyIDs, event.Id)
		}
		_, err = c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "nested", CreatedAt: created, ThreadRootEventID: replyIDs[0]})
		require.Error(t, err, "a reply cannot be a thread root")
	}
	createdEvents, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventThreadCreated))
	require.NoError(t, err)
	require.Len(t, createdEvents, 2, "first replies must not duplicate thread creation")
	for _, event := range createdEvents {
		require.Equal(t, SystemActorID, event.ActorId)
		require.True(t, event.GetThreadCreated().GetHistoricalImport())
		require.False(t, isDeliverableLiveEVTRoomEvent(event))
	}
	follows, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventThreadFollowed))
	require.NoError(t, err)
	require.Empty(t, follows)
	_, err = c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "bad", CreatedAt: created, ThreadRootEventID: "missing"})
	require.Error(t, err)
	_, err = c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "bad", CreatedAt: created, ThreadRootEventID: "missing", CreateThread: true})
	require.Error(t, err)
}

func TestHistoricalReactionImportConcurrentReplayAndRemoval(t *testing.T) {
	t.Parallel()
	const emoji = "Party_Parrot"
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	author, err := c.CreateUser(ctx, SystemActorID, "reaction-import-author", "Author", "password")
	require.NoError(t, err)
	reactor, err := c.CreateUser(ctx, SystemActorID, "reaction-import-reactor", "Reactor", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "reaction-import", "")
	require.NoError(t, err)
	root, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "root", CreatedAt: time.Now(), CreateThread: true})
	require.NoError(t, err)
	reply, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.Id, AuthorID: author.Id, Body: "reply", CreatedAt: time.Now(), ThreadRootEventID: root.Id})
	require.NoError(t, err)
	replica := startReplica(t, ns, testCoreConfig())
	for _, messageID := range []string{root.Id, reply.Id} {
		input := HistoricalReactionInput{RoomID: room.Id, MessageEventID: messageID, UserID: reactor.Id, Emoji: emoji}
		var addedCount atomic.Int32
		var wg sync.WaitGroup
		for i := range 6 {
			writer := c
			if i%2 == 0 {
				writer = replica
			}
			wg.Go(func() {
				added, err := writer.ImportHistoricalReaction(ctx, input)
				if err != nil {
					t.Error(err)
				}
				if added {
					addedCount.Add(1)
				}
			})
		}
		wg.Wait()
		require.EqualValues(t, 1, addedCount.Load())
		reactions, err := c.GetReactions(ctx, messageID)
		require.NoError(t, err)
		require.Equal(t, []ReactionSummary{{Emoji: emoji, UserIDs: []string{reactor.Id}}}, reactions)
	}
	reactionEvents, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventReactionAdded))
	require.NoError(t, err)
	require.Len(t, reactionEvents, 2)
	projection := NewReactionProjection()
	for i, event := range reactionEvents {
		require.Equal(t, SystemActorID, event.ActorId)
		require.Equal(t, reactor.Id, event.GetReactionAdded().GetUserId())
		require.True(t, event.GetReactionAdded().GetHistoricalImport())
		require.False(t, isDeliverableLiveEVTRoomEvent(event))
		require.NoError(t, projection.Apply(event, uint64(i+1)))
	}
	snapshot, err := projection.Snapshot()
	require.NoError(t, err)
	restored := NewReactionProjection()
	require.NoError(t, restored.Restore(snapshot))
	// The mapped user can remove an imported reaction through the ordinary path.
	removed, err := c.reactionModel.removeReaction(ctx, KindChannel, room.Id, root.Id, emoji, reactor.Id)
	require.NoError(t, err)
	require.True(t, removed)
	removal := newReactionRemovedEvent(reactor.Id, room.Id, root.Id, emoji)
	for _, p := range []*ReactionProjection{projection, restored} {
		require.NoError(t, p.Apply(removal, 3))
		bytes, err := p.Snapshot()
		require.NoError(t, err)
		if p == projection {
			snapshot = bytes
		} else {
			require.Equal(t, snapshot, bytes)
		}
	}
	require.NoError(t, c.notificationMaterializer.WaitCurrent(ctx))
	require.Empty(t, testNotificationOccurrences(t, c, author.Id))
	// A fresh process must rebuild attribution and thread placement from EVT.
	replayed := startReplica(t, ns, testCoreConfig())
	require.Equal(t, c.GetThreadMetadata(root.Id), replayed.GetThreadMetadata(root.Id))
	require.Equal(t, []string{author.Id}, replayed.GetThreadMetadata(root.Id).ParticipantIDs)
	reactions, err := replayed.GetReactions(ctx, reply.Id)
	require.NoError(t, err)
	require.Equal(t, []ReactionSummary{{Emoji: emoji, UserIDs: []string{reactor.Id}}}, reactions)
	following, err := replayed.IsFollowingThread(ctx, KindChannel, author.Id, room.Id, root.Id)
	require.NoError(t, err)
	require.False(t, following)
}

func TestHistoricalReactionSuppressesMaterializedNotifications(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	author, room, messageID := setupReactionTest(t, c, ctx)
	reactor, err := c.CreateUser(ctx, SystemActorID, "silent-reactor", "Reactor", "password")
	require.NoError(t, err)
	_, err = c.AddMember(ctx, SystemActorID, KindChannel, room.Id, reactor.Id)
	require.NoError(t, err)
	_, err = c.NotificationPolicy().SetRoomNotificationMode(ctx, author.Id, room.Id, notificationTestSignalReaction, evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION)
	require.NoError(t, err)
	added, err := c.ImportHistoricalReaction(ctx, HistoricalReactionInput{RoomID: room.Id, MessageEventID: messageID, UserID: reactor.Id, Emoji: "heart"})
	require.NoError(t, err)
	require.True(t, added)
	require.NoError(t, c.notificationMaterializer.WaitCurrent(ctx))
	require.Empty(t, testNotificationOccurrences(t, c, author.Id))
	added, err = c.reactionModel.addReaction(ctx, KindChannel, room.Id, messageID, "tada", reactor.Id)
	require.NoError(t, err)
	require.True(t, added)
	require.NoError(t, c.notificationMaterializer.WaitCurrent(ctx))
	require.Len(t, testNotificationOccurrences(t, c, author.Id), 1, "ordinary reactions must still notify")
}

func TestHistoricalReactionImportRejectsInvalidTargets(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	u, err := c.CreateUser(ctx, SystemActorID, "reaction-invalid", "Author", "password")
	require.NoError(t, err)
	r, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "reaction-invalid", "")
	require.NoError(t, err)
	m, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: r.Id, AuthorID: u.Id, Body: "root", CreatedAt: time.Now()})
	require.NoError(t, err)
	other, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "reaction-other", "")
	require.NoError(t, err)
	base := HistoricalReactionInput{RoomID: r.Id, MessageEventID: m.Id, UserID: u.Id, Emoji: "heart"}
	for _, change := range []func(*HistoricalReactionInput){
		func(v *HistoricalReactionInput) { v.RoomID = "" },
		func(v *HistoricalReactionInput) { v.RoomID = other.Id },
		func(v *HistoricalReactionInput) { v.UserID = "missing" },
		func(v *HistoricalReactionInput) { v.MessageEventID = "missing" },
		func(v *HistoricalReactionInput) { v.Emoji = ":invalid:" },
	} {
		input := base
		change(&input)
		_, err := c.ImportHistoricalReaction(ctx, input)
		require.Error(t, err)
	}
	require.NoError(t, c.DeleteMessage(ctx, SystemActorID, KindChannel, r.Id, m.Id))
	added, err := c.ImportHistoricalReaction(ctx, base)
	require.NoError(t, err)
	require.True(t, added)
	added, err = c.ImportHistoricalReaction(ctx, base)
	require.NoError(t, err)
	require.False(t, added)
	// Historical replies retain their placement even when the root was deleted.
	_, err = c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: r.Id, AuthorID: u.Id, Body: "surviving reply", CreatedAt: time.Now(), ThreadRootEventID: m.Id})
	require.NoError(t, err)
	_, err = c.ArchiveRoom(ctx, SystemActorID, KindChannel, r.Id)
	require.NoError(t, err)
	_, err = c.ImportHistoricalReaction(ctx, base)
	require.ErrorIs(t, err, ErrRoomArchived)
}

func TestHistoricalThreadAndReactionBadgeReplay(t *testing.T) {
	t.Parallel()
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{Reactions: badgeMode})
	f.post("root", "U1", "")
	f.apply(&evtv1.Event{ActorId: "U1", Event: &evtv1.Event_ThreadFollowed{ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "root", UserId: "U1"}}})
	f.apply(newEvent(SystemActorID, &evtv1.Event{Id: "reply", Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", InThread: "root", AuthorId: "U2", HistoricalImport: true}}}))
	f.apply(newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "root", Emoji: "heart", UserId: "U2", HistoricalImport: true}}}))
	require.False(t, f.unread("U1", "root"))
	require.False(t, f.unread("U1", ""))
	snapshot, err := f.p.Snapshot()
	require.NoError(t, err)
	f.p = NewNotificationDecisionProjection()
	require.NoError(t, f.p.Restore(snapshot))
	require.False(t, f.unread("U1", "root"))
	require.False(t, f.unread("U1", ""))
	// The same ordinary reaction must generate attention: this checks that the
	// fixture is capable of detecting a missing historical guard.
	f.apply(newReactionAddedEvent("U2", "R1", "root", "tada"))
	require.True(t, f.unread("U1", ""))
}

func TestHistoricalReactionImportEchoAndInteractiveLimit(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, rootID := setupReactionTest(t, c, ctx)
	reply, err := c.PostMessage(ctx, KindChannel, room.Id, user.Id, "reply", nil, rootID, "", nil, true)
	require.NoError(t, err)
	echoID, ok := c.ChannelEchoEventID(reply.Id)
	require.True(t, ok)
	for _, emoji := range reactionLimitTestEmoji[:MaxReactionsPerUserPerMessage+1] {
		added, err := c.ImportHistoricalReaction(ctx, HistoricalReactionInput{RoomID: room.Id, MessageEventID: echoID, UserID: user.Id, Emoji: emoji})
		require.NoError(t, err)
		require.True(t, added)
	}
	input := HistoricalReactionInput{RoomID: room.Id, MessageEventID: reply.Id, UserID: user.Id, Emoji: reactionLimitTestEmoji[0]}
	added, err := c.ImportHistoricalReaction(ctx, input)
	require.NoError(t, err)
	require.False(t, added, "echo and original must share one reaction set")
	_, err = c.reactionModel.addReaction(ctx, KindChannel, room.Id, reply.Id, "joy", user.Id)
	require.ErrorIs(t, err, ErrReactionLimitExceeded)
}
