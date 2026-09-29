package core

import (
	"bytes"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// badgeTestFixture drives a NotificationDecisionProjection with stream
// sequences and answers Badge queries against it.
type badgeTestFixture struct {
	t    *testing.T
	p    *NotificationDecisionProjection
	seq  uint64
	read map[string]notificationReadBoundary
}

func newBadgeTestFixture(t *testing.T) *badgeTestFixture {
	t.Helper()
	return newBadgeTestFixtureFor(t, NewNotificationDecisionProjection())
}

// newBadgeTestFixtureFor prepares a room with two members in p.
func newBadgeTestFixtureFor(t *testing.T, p *NotificationDecisionProjection) *badgeTestFixture {
	t.Helper()
	f := &badgeTestFixture{t: t, p: p, read: make(map[string]notificationReadBoundary)}
	f.apply(&evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermMessageRead)}})
	f.apply(&evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}})
	for _, userID := range []string{"U1", "U2"} {
		f.apply(&evtv1.Event{Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: userID}}})
		f.apply(&evtv1.Event{ActorId: userID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}})
	}
	return f
}

// apply appends one fact and returns its stream sequence.
func (f *badgeTestFixture) apply(event *evtv1.Event) uint64 {
	f.t.Helper()
	f.seq++
	if event.CreatedAt == nil {
		event.CreatedAt = timestamppb.Now()
	}
	if err := f.p.Apply(event, f.seq); err != nil {
		f.t.Fatalf("apply %T: %v", event.GetEvent(), err)
	}
	return f.seq
}

func (f *badgeTestFixture) post(id, actorID, inThread string, mentions ...*evtv1.MessageMention) uint64 {
	f.t.Helper()
	return f.apply(&evtv1.Event{Id: id, ActorId: actorID, Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
		RoomId: "R1", InThread: inThread, AuthorId: actorID, Mentions: mentions,
	}}})
}

func (f *badgeTestFixture) setMode(userID string, modes *evtv1.NotificationDeliveryModes) {
	f.t.Helper()
	room := "R1"
	f.apply(&evtv1.Event{Event: &evtv1.Event_UserNotificationPolicyChanged{UserNotificationPolicyChanged: &evtv1.UserNotificationPolicyChangedEvent{
		UserId: userID, RoomId: &room, Overrides: modes,
	}}})
}

func (f *badgeTestFixture) unread(userID, threadRootEventID string) bool {
	f.t.Helper()
	return badgeUnread(f.t, f.p, badgeQuery{
		userID: userID, roomID: "R1", threadRootEventID: threadRootEventID, now: time.Now(),
		readBoundary: func(thread string) (notificationReadBoundary, bool) {
			boundary, ok := f.read[userID+"/"+thread]
			return boundary, ok
		},
	})
}

func badgeUnread(t *testing.T, p *NotificationDecisionProjection, query badgeQuery) bool {
	t.Helper()
	var unread bool
	if err := p.withCurrent(query.now, func(snapshot *notificationDecisionSnapshot) error {
		unread = snapshot.hasBadgeAttention(query)
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	return unread
}

var badgeMode = evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE.Enum()

func TestBadgeRootMessageAttentionFollowsCurrentState(t *testing.T) {
	f := newBadgeTestFixture(t)
	if f.unread("U1", "") {
		t.Fatal("empty room has Badge attention")
	}
	own := f.post("OWN", "U1", "")
	if f.unread("U1", "") {
		t.Fatal("the user's own message gave Badge attention")
	}
	f.post("OTHER", "U2", "")
	if !f.unread("U1", "") {
		t.Fatal("another member's root message gave no Badge attention")
	}
	f.setMode("U1", &evtv1.NotificationDeliveryModes{RoomMessages: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum()})
	if f.unread("U1", "") {
		t.Fatal("Badge attention remained after switching room messages Off")
	}
	f.setMode("U1", &evtv1.NotificationDeliveryModes{})
	if !f.unread("U1", "") {
		t.Fatal("Badge attention did not return with the default policy")
	}
	f.read["U1/"] = notificationReadBoundary{targetSequence: f.seq, observedSequence: f.seq}
	if f.unread("U1", "") {
		t.Fatal("read boundary did not cover the root message")
	}
	delete(f.read, "U1/")
	f.apply(&evtv1.Event{Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: "R1", EventId: "OTHER"}}})
	if f.unread("U1", "") {
		t.Fatal("retracted root message still gave Badge attention")
	}
	_ = own
}

func TestBadgeIgnoresSourcesBeforeMembershipStarts(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.post("BEFORE", "U2", "")
	f.apply(&evtv1.Event{Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "U3"}}})
	f.apply(&evtv1.Event{ActorId: "U3", Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}})
	if f.unread("U3", "") {
		t.Fatal("a message from before the join gave Badge attention")
	}
	f.post("AFTER", "U2", "")
	if !f.unread("U3", "") {
		t.Fatal("a message after the join gave no Badge attention")
	}
}

func TestBadgeUniversalMembershipStartsWithTheAccount(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.apply(&evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R2", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL, Universal: true}}})
	f.apply(&evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermRoomJoin)}})
	f.apply(&evtv1.Event{Id: "OLD", ActorId: "U2", Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R2", AuthorId: "U2"}}})
	f.apply(&evtv1.Event{Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: "U3"}}})
	query := badgeQuery{userID: "U3", roomID: "R2", now: time.Now()}
	if badgeUnread(t, f.p, query) {
		t.Fatal("a universal-room message from before the account gave Badge attention")
	}
	f.apply(&evtv1.Event{Id: "NEW", ActorId: "U2", Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R2", AuthorId: "U2"}}})
	if !badgeUnread(t, f.p, query) {
		t.Fatal("a universal-room message after the account gave no Badge attention")
	}
}

func TestBadgeFollowedThreadCountsRepliesAfterTheFollow(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{RoomMessages: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF.Enum(), FollowedThreads: badgeMode})
	f.post("ROOT", "U2", "")
	f.post("EARLY", "U2", "ROOT")
	f.apply(&evtv1.Event{Event: &evtv1.Event_ThreadFollowed{ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"}}})
	if f.unread("U1", "ROOT") || f.unread("U1", "") {
		t.Fatal("a reply from before the follow gave Badge attention")
	}
	f.post("LATE", "U2", "ROOT")
	if !f.unread("U1", "ROOT") || !f.unread("U1", "") {
		t.Fatal("a reply after the follow gave no thread and room Badge attention")
	}
	f.read["U1/ROOT"] = notificationReadBoundary{targetSequence: f.seq, observedSequence: f.seq}
	if f.unread("U1", "") {
		t.Fatal("the thread read boundary did not cover the reply")
	}
	delete(f.read, "U1/ROOT")
	f.apply(&evtv1.Event{Event: &evtv1.Event_ThreadUnfollowed{ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"}}})
	if f.unread("U1", "ROOT") {
		t.Fatal("followed-thread attention remained after the unfollow")
	}
}

func TestBadgeFirstReplyAddressesTheRootAuthor(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{FollowedThreads: badgeMode})
	f.post("ROOT", "U1", "")
	f.post("FIRST", "U2", "ROOT")
	// Posting the first reply auto-follows the root author after the reply.
	f.apply(&evtv1.Event{Event: &evtv1.Event_ThreadFollowed{ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"}}})
	if !f.unread("U1", "ROOT") {
		t.Fatal("the first reply gave the root author no Badge attention")
	}
	f.apply(&evtv1.Event{Event: &evtv1.Event_ThreadUnfollowed{ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"}}})
	if f.unread("U1", "ROOT") {
		t.Fatal("the first reply still gave attention after an explicit unfollow")
	}
}

func TestBadgeReactionAttentionEndsWithTheReaction(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{Reactions: badgeMode})
	f.post("MINE", "U1", "")
	f.read["U1/"] = notificationReadBoundary{targetSequence: f.seq, observedSequence: f.seq}
	reaction := &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "MINE", Emoji: "tada"}
	added := f.apply(&evtv1.Event{Id: "REACT", ActorId: "U2", Event: &evtv1.Event_ReactionAdded{ReactionAdded: reaction}})
	if !f.unread("U1", "") {
		t.Fatal("a reaction after the read gave no Badge attention")
	}
	f.read["U1/"] = notificationReadBoundary{targetSequence: f.read["U1/"].targetSequence, observedSequence: added}
	if f.unread("U1", "") {
		t.Fatal("a read that observed the reaction did not cover it")
	}
	f.read["U1/"] = notificationReadBoundary{targetSequence: added - 1, observedSequence: added - 1}
	f.apply(&evtv1.Event{ActorId: "U2", Event: &evtv1.Event_ReactionRemoved{ReactionRemoved: &evtv1.ReactionRemovedEvent{RoomId: "R1", MessageEventId: "MINE", Emoji: "tada"}}})
	if f.unread("U1", "") {
		t.Fatal("a removed reaction still gave Badge attention")
	}
}

func TestBadgeBeforeBoundExcludesLaterSources(t *testing.T) {
	f := newBadgeTestFixture(t)
	source := f.post("SOURCE", "U2", "")
	query := badgeQuery{userID: "U1", roomID: "R1", now: time.Now(), before: source}
	if badgeUnread(t, f.p, query) {
		t.Fatal("the before bound did not exclude the source")
	}
	query.before = 0
	if !badgeUnread(t, f.p, query) {
		t.Fatal("the source gave no Badge attention without a before bound")
	}
}

func TestBadgeSourcesSurviveSnapshotRestore(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{FollowedThreads: badgeMode, Reactions: badgeMode, DirectMentions: badgeMode})
	f.post("ROOT", "U1", "")
	f.post("REPLY", "U2", "ROOT", &evtv1.MessageMention{UserId: "U1", Cause: &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}}})
	f.apply(&evtv1.Event{Event: &evtv1.Event_ThreadFollowed{ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"}}})
	f.apply(&evtv1.Event{Id: "REACT", ActorId: "U2", Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "ROOT", Emoji: "tada"}}})
	f.post("ROOT2", "U2", "")
	f.apply(&evtv1.Event{Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: "R1", EventId: "ROOT2"}}})

	data, err := f.p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored := NewNotificationDecisionProjection()
	if err := restored.Restore(data); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	again, err := restored.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot after restore: %v", err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("restored snapshot differs from the original")
	}
	for _, query := range []badgeQuery{
		{userID: "U1", roomID: "R1"},
		{userID: "U1", roomID: "R1", threadRootEventID: "ROOT"},
		{userID: "U2", roomID: "R1"},
	} {
		query.now = time.Now()
		if got, want := badgeUnread(t, restored, query), badgeUnread(t, f.p, query); got != want {
			t.Fatalf("restored Badge for %+v = %v, want %v", query, got, want)
		}
	}
}

// BenchmarkBadgeAttentionFromStore replays a real EVT stream into the
// notification decision projection and measures room-level Badge evaluation
// for every (member, room) pair, as a room list does. Set
// CHATTO_BENCH_EVT_STORE_DIR to a copied NATS data directory.
func BenchmarkBadgeAttentionFromStore(b *testing.B) {
	fixture := loadProjectionBenchmarkStoreFixture(b, projectionBenchmarkStoreDir(b))
	targets, err := replayProjectionBenchmarkFixture(fixture, "notification_decisions")
	if err != nil {
		b.Fatal(err)
	}
	p := targets[0].projection.(*NotificationDecisionProjection)
	var queries []badgeQuery
	now := time.Now()
	if err := p.withCurrent(now, func(snapshot *notificationDecisionSnapshot) error {
		for room := range snapshot.badges.rooms {
			roomID := snapshot.badges.ids.id(room)
			for _, userID := range snapshot.roomMemberIDs(roomID) {
				queries = append(queries, badgeQuery{userID: userID, roomID: roomID, now: now})
			}
		}
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	if len(queries) == 0 {
		b.Skip("store has no rooms with Badge sources")
	}
	b.Logf("%d member-room pairs", len(queries))
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		query := queries[i%len(queries)]
		_ = p.withCurrent(now, func(snapshot *notificationDecisionSnapshot) error {
			snapshot.hasBadgeAttention(query)
			return nil
		})
	}
}

func TestBadgeRepeatedJoinKeepsTheMembershipStart(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.post("AFTER-JOIN", "U2", "")
	f.apply(&evtv1.Event{ActorId: "U1", Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}})
	if !f.unread("U1", "") {
		t.Fatal("a repeated join hid a message posted during the membership")
	}
}

func TestBadgeIgnoresSourcesWithoutCreationTime(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.seq++
	if err := f.p.Apply(&evtv1.Event{Id: "UNDATED", ActorId: "U2", Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", AuthorId: "U2"}}}, f.seq); err != nil {
		t.Fatal(err)
	}
	if f.unread("U1", "") {
		t.Fatal("a message without a creation time gave Badge attention")
	}
}

func TestBadgeListsDropExpiredSources(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{Reactions: badgeMode})
	old := timestamppb.New(time.Now().Add(-notificationTTL - time.Hour))
	f.apply(&evtv1.Event{Id: "OLD", ActorId: "U1", CreatedAt: old, Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", AuthorId: "U1"}}})
	f.apply(&evtv1.Event{Id: "OLD-REACT", ActorId: "U2", CreatedAt: old, Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "OLD", Emoji: "tada"}}})
	f.post("NEW", "U2", "")
	f.apply(&evtv1.Event{Id: "NEW-REACT", ActorId: "U2", Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "OLD", Emoji: "star"}}})
	var roots, targeted, reactions int
	_ = f.p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		b := snapshot.badges
		room, _ := b.ids.lookup("R1")
		user, _ := b.ids.lookup("U1")
		roots = len(b.rooms[room].roots)
		targeted = len(b.rooms[room].targeted[user][0])
		reactions = len(b.reactions)
		return nil
	})
	if roots != 1 || targeted != 1 || reactions != 1 {
		t.Fatalf("roots=%d targeted=%d reactions=%d, want expired sources dropped (1, 1, 1)", roots, targeted, reactions)
	}
}

func TestBadgeSweepDropsExpiredSourcesOfQuietThreads(t *testing.T) {
	f := newBadgeTestFixture(t)
	old := timestamppb.New(time.Now().Add(-notificationTTL - time.Hour))
	f.apply(&evtv1.Event{Id: "ROOT", ActorId: "U1", CreatedAt: old, Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", AuthorId: "U1"}}})
	f.apply(&evtv1.Event{Id: "REPLY", ActorId: "U2", CreatedAt: old, Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{
		RoomId: "R1", AuthorId: "U2", InThread: "ROOT",
		Mentions: []*evtv1.MessageMention{{UserId: "U1", Cause: &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}}}},
	}}})
	for i := range badgeSweepInterval {
		f.post(fmt.Sprintf("NEW-%d", i), "U2", "")
	}
	var replies, scopes int
	_ = f.p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		b := snapshot.badges
		room, _ := b.ids.lookup("R1")
		user, _ := b.ids.lookup("U1")
		replies = len(b.rooms[room].replies)
		scopes = len(b.rooms[room].targeted[user])
		return nil
	})
	if replies != 0 || scopes != 0 {
		t.Fatalf("after a sweep: %d reply lists and %d targeted scopes, want the expired thread dropped", replies, scopes)
	}
}

func TestBadgeUnretractedQueryCountsTheRetractedMessage(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.setMode("U1", &evtv1.NotificationDeliveryModes{Reactions: badgeMode})
	f.post("MINE", "U1", "")
	f.read["U1/"] = notificationReadBoundary{targetSequence: f.seq, observedSequence: f.seq}
	f.apply(&evtv1.Event{Id: "REACT", ActorId: "U2", Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "MINE", Emoji: "tada"}}})
	f.apply(&evtv1.Event{Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: "R1", EventId: "MINE"}}})
	if f.unread("U1", "") {
		t.Fatal("a reaction on a retracted message gave Badge attention")
	}
	var live bool
	_ = f.p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		_, _, message, users := snapshot.badgeAudience("MINE")
		if !slices.Contains(users, "U1") {
			t.Fatalf("audience %v omits the reacted message's author", users)
		}
		live = snapshot.hasBadgeAttention(badgeQuery{
			userID: "U1", roomID: "R1", now: time.Now(), unretracted: message,
			readBoundary: func(thread string) (notificationReadBoundary, bool) {
				boundary, ok := f.read["U1/"+thread]
				return boundary, ok
			},
		})
		return nil
	})
	if !live {
		t.Fatal("attention with the message counted as live is off, so the retraction would send no hint")
	}
}

func threadFollowEvent(userID, rootID string, follow bool) *evtv1.Event {
	if follow {
		return &evtv1.Event{Event: &evtv1.Event_ThreadFollowed{ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: rootID, UserId: userID}}}
	}
	return &evtv1.Event{Event: &evtv1.Event_ThreadUnfollowed{ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: rootID, UserId: userID}}}
}

// threadState reads the follow state, followers, and reply count of ROOT.
func threadState(t *testing.T, p *NotificationDecisionProjection, userID string) (ThreadFollowState, []string, uint64) {
	t.Helper()
	var (
		state     ThreadFollowState
		followers []string
		replies   uint64
	)
	if err := p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		state = snapshot.threadFollowState(userID, "R1", "ROOT")
		followers = snapshot.threadFollowerIDs("R1", "ROOT")
		replies = snapshot.threadReplyCount("ROOT")
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	return state, followers, replies
}

func TestNotificationThreadFollowStateTransitions(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.post("ROOT", "U1", "")
	f.post("REPLY-1", "U2", "ROOT")
	f.post("REPLY-2", "U1", "ROOT")
	for _, event := range []*evtv1.Event{
		threadFollowEvent("U2", "ROOT", true),
		threadFollowEvent("U1", "ROOT", true),
		threadFollowEvent("U1", "ROOT", false),
		threadFollowEvent("U1", "ROOT", true),
		threadFollowEvent("U3", "ROOT", false),
	} {
		f.apply(event)
	}

	state, followers, replies := threadState(t, f.p, "U1")
	if state != ThreadFollowStateFollowing || !slices.Equal(followers, []string{"U1", "U2"}) || replies != 2 {
		t.Fatalf("U1 state=%q followers=%v replies=%d; want following, [U1 U2], 2", state, followers, replies)
	}
	if state, _, _ := threadState(t, f.p, "U3"); state != ThreadFollowStateUnfollowed {
		t.Fatalf("U3 state = %q, want unfollowed", state)
	}
	if state, _, _ := threadState(t, f.p, "U-UNKNOWN"); state != ThreadFollowStateNone {
		t.Fatalf("unknown user state = %q, want none", state)
	}
	f.apply(threadFollowEvent("U2", "ROOT", false))
	if _, followers, _ := threadState(t, f.p, "U2"); !slices.Equal(followers, []string{"U1"}) {
		t.Fatalf("followers after U2 unfollow = %v, want [U1]", followers)
	}
}

// TestNotificationDecisionsShareEventIDsAcrossRestore verifies that the
// decision projection adds no event IDs that the content view already holds
// and keeps sharing the table after a snapshot restore.
func TestNotificationDecisionsShareEventIDsAcrossRestore(t *testing.T) {
	eventIDs := newEventIDTable()
	timeline := newRoomTimelineProjection(eventIDs)
	f := newBadgeTestFixtureFor(t, newNotificationDecisionProjection(eventIDs))
	f.setMode("U1", &evtv1.NotificationDeliveryModes{FollowedThreads: badgeMode, Reactions: badgeMode})
	posts := []*evtv1.Event{
		postedEvent(postedOpts{envelopeID: "ROOT", roomID: "R1", actorID: "U1"}),
		postedEvent(postedOpts{envelopeID: "REPLY", roomID: "R1", actorID: "U2", inThread: "ROOT"}),
	}
	for i, event := range posts {
		event.CreatedAt = timestamppb.Now()
		if err := timeline.Apply(event, uint64(i+1)); err != nil {
			t.Fatalf("apply timeline: %v", err)
		}
	}
	known := eventIDs.len()
	for _, event := range posts {
		f.apply(event)
	}
	f.apply(threadFollowEvent("U1", "ROOT", true))
	f.apply(&evtv1.Event{Id: "REACT", ActorId: "U2", Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "ROOT", Emoji: "tada"}}})
	if got := eventIDs.len(); got != known {
		t.Fatalf("shared event IDs after decision replay = %d, want %d", got, known)
	}

	data, err := f.p.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored := newNotificationDecisionProjection(eventIDs)
	if err := restored.Restore(data); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.badges.eventIDs != eventIDs || eventIDs.len() != known {
		t.Fatalf("restore used table %p with %d IDs, want shared table with %d IDs", restored.badges.eventIDs, eventIDs.len(), known)
	}
	again, err := restored.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot after restore: %v", err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("restored snapshot differs from the original")
	}
	if state, followers, replies := threadState(t, restored, "U1"); state != ThreadFollowStateFollowing || !slices.Equal(followers, []string{"U1"}) || replies != 1 {
		t.Fatalf("restored thread state = %q, %v, %d; want following, [U1], 1", state, followers, replies)
	}
	query := badgeQuery{userID: "U1", roomID: "R1", now: time.Now()}
	if !badgeUnread(t, restored, query) || !badgeUnread(t, f.p, query) {
		t.Fatal("restored projection lost Badge attention from the reply and reaction")
	}
}

func TestBadgeRoomDeletionClearsMessageRecords(t *testing.T) {
	f := newBadgeTestFixture(t)
	f.post("ROOT", "U1", "")
	f.apply(&evtv1.Event{Id: "REACT", ActorId: "U2", Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "ROOT", Emoji: "tada"}}})
	f.apply(&evtv1.Event{Event: &evtv1.Event_RoomDeleted{RoomDeleted: &evtv1.RoomDeletedEvent{RoomId: "R1"}}})

	if err := f.p.withCurrent(time.Now(), func(snapshot *notificationDecisionSnapshot) error {
		if roomID, _, _, users := snapshot.badgeAudience("ROOT"); roomID != "" || len(users) != 0 {
			t.Fatalf("badgeAudience after room deletion = %q, %v; want none", roomID, users)
		}
		if got := len(snapshot.badges.reactions); got != 0 {
			t.Fatalf("reactions after room deletion = %d, want 0", got)
		}
		if got := len(snapshot.badges.snapshot().GetMessages()); got != 0 {
			t.Fatalf("snapshot messages after room deletion = %d, want 0", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
}
