package core

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// The tests in this file check the compact, handle-based projections against
// each other instead of against hand-written expectations. A randomized EVT
// history drives several copies of the same projections. Every copy must give
// the same snapshot bytes and the same read results:
//
//   - with a private event ID table and with a shared table that already holds
//     unrelated IDs, so the handles of the two copies differ;
//   - after a snapshot restore into a new table;
//   - when a restore interrupts the replay at an arbitrary sequence.
//
// A defect that mixes up handle spaces, depends on handle order, or loses
// state in the snapshot codec changes the snapshot bytes or the read digest.

// compactProjections holds the projections with compact, handle-based state.
// All except contentKeys index message history by event ID handles.
type compactProjections struct {
	timeline    *RoomTimelineProjection
	threads     *ThreadProjection
	reactions   *ReactionProjection
	decisions   *NotificationDecisionProjection
	contentKeys *ContentKeyProjection
}

// newPrivateCompactProjections gives each projection a private table.
func newPrivateCompactProjections() *compactProjections {
	return &compactProjections{
		timeline:    NewRoomTimelineProjection(),
		threads:     NewThreadProjection(),
		reactions:   NewReactionProjection(),
		decisions:   NewNotificationDecisionProjection(),
		contentKeys: NewContentKeyProjection(),
	}
}

// newSharedCompactProjections gives all projections one table, as the
// production wiring does. The table first interns foreignIDs unrelated IDs, so
// the handles differ from those of a private table. The projections' own ID
// tables first intern the user, room, and emoji IDs in reverse order, so their
// handle order differs from the order of the history. A restore replaces these
// tables with tables in snapshot order, which differs again.
func newSharedCompactProjections(foreignIDs int) *compactProjections {
	table := newEventIDTable()
	for i := range foreignIDs {
		table.intern(fmt.Sprintf("FOREIGN-%04d", i))
	}
	c := &compactProjections{
		timeline:    newRoomTimelineProjection(table),
		threads:     newThreadProjection(table),
		reactions:   newReactionProjection(table),
		decisions:   newNotificationDecisionProjection(table),
		contentKeys: NewContentKeyProjection(),
	}
	for _, id := range []string{"tada", "heart", "wave", "D1", "R2", "R1", "U5", "U4", "U3", "U2", "U1"} {
		c.timeline.users.intern(id)
		c.timeline.rooms.intern(id)
		c.threads.principalIDs.intern(id)
		c.reactions.ids.intern(id)
		c.decisions.badges.ids.intern(id)
		c.contentKeys.users.intern(id)
	}
	return c
}

func (c *compactProjections) all() []interface {
	testProjection
	snapshotProjection
} {
	return []interface {
		testProjection
		snapshotProjection
	}{c.timeline, c.threads, c.reactions, c.decisions, c.contentKeys}
}

// apply feeds events to every projection. The first event gets sequence
// firstSeq.
func (c *compactProjections) apply(t *testing.T, events []*evtv1.Event, firstSeq uint64) {
	t.Helper()
	for i, event := range events {
		for _, projection := range c.all() {
			if err := projection.Apply(event, firstSeq+uint64(i)); err != nil {
				t.Fatalf("apply event %s to %T: %v", event.GetId(), projection, err)
			}
		}
	}
}

func (c *compactProjections) snapshots(t *testing.T) [][]byte {
	t.Helper()
	var out [][]byte
	for _, projection := range c.all() {
		data, err := projection.Snapshot()
		if err != nil {
			t.Fatalf("snapshot %T: %v", projection, err)
		}
		out = append(out, data)
	}
	return out
}

func (c *compactProjections) restore(t *testing.T, snapshots [][]byte) {
	t.Helper()
	for i, projection := range c.all() {
		if err := projection.Restore(snapshots[i]); err != nil {
			t.Fatalf("restore %T: %v", projection, err)
		}
	}
}

func assertSameSnapshots(t *testing.T, label string, want, got [][]byte) {
	t.Helper()
	names := []string{"room timeline", "threads", "reactions", "notification decisions", "content keys"}
	for i := range want {
		if !bytes.Equal(want[i], got[i]) {
			t.Fatalf("%s: %s snapshot differs (%d bytes, want %d)", label, names[i], len(got[i]), len(want[i]))
		}
	}
}

func assertSameDigest(t *testing.T, label, want, got string) {
	t.Helper()
	if want == got {
		return
	}
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range min(len(wantLines), len(gotLines)) {
		if wantLines[i] != gotLines[i] {
			t.Fatalf("%s: read results differ at line %d\n got: %s\nwant: %s", label, i+1, gotLines[i], wantLines[i])
		}
	}
	t.Fatalf("%s: read results have %d lines, want %d", label, len(gotLines), len(wantLines))
}

// compactHistory is a generated EVT history and the IDs that its reads cover.
type compactHistory struct {
	events   []*evtv1.Event
	rooms    []string
	users    []string
	emojis   []string
	messages []string
	// messageRoom holds the room of each posted message.
	messageRoom map[string]string
	// roots holds the thread roots of each room.
	roots map[string][]string
	// now is a query time after the last event and inside notificationTTL.
	now int
}

// generateCompactHistory builds a deterministic history with root messages,
// thread replies, channel echoes, bodies that arrive before their posts,
// edits, retractions, reactions, pins, follows, membership changes, content
// keys, key shredding, an account deletion, a room deletion, and a gap that
// expires earlier Badge sources.
func generateCompactHistory(seed uint64, steps int) *compactHistory {
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	h := &compactHistory{
		rooms:       []string{"R1", "R2", "D1"},
		users:       []string{"U1", "U2", "U3", "U4", "U5"},
		emojis:      []string{"wave", "heart", "tada"},
		messageRoom: make(map[string]string),
		roots:       make(map[string][]string),
	}
	clock := 0
	nextID := 0
	id := func(prefix string) string {
		nextID++
		return fmt.Sprintf("%s%05d", prefix, nextID)
	}
	emit := func(event *evtv1.Event) {
		clock++
		if event.Id == "" {
			event.Id = id("E")
		}
		event.CreatedAt = timestamppb.New(fixedTime(clock))
		h.events = append(h.events, event)
	}
	pick := func(values []string) string { return values[rng.IntN(len(values))] }
	members := map[string]map[string]bool{"R1": {}, "R2": {}, "D1": {"U1": true, "U2": true}}
	memberOf := func(roomID string) []string {
		var ids []string
		for _, userID := range h.users {
			// R2 is universal, so every account can post there.
			if roomID == "R2" || members[roomID][userID] {
				ids = append(ids, userID)
			}
		}
		return ids
	}

	emit(&evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", RoleEveryone, PermMessageRead)}})
	emit(&evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R1", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}})
	emit(&evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "R2", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL, Universal: true}}})
	emit(&evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "D1", Kind: evtv1.RoomKind_ROOM_KIND_DM}}})
	for _, userID := range h.users {
		emit(&evtv1.Event{Event: &evtv1.Event_UserAccountCreated{UserAccountCreated: &evtv1.UserAccountCreatedEvent{UserId: userID}}})
		emit(&evtv1.Event{ActorId: userID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}})
		members["R1"][userID] = true
	}
	for _, userID := range []string{"U1", "U2"} {
		emit(&evtv1.Event{ActorId: userID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "D1"}}})
	}
	// Every user gives every cause Badge attention in every room, so reads
	// exercise every source kind.
	badge := evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE.Enum()
	for _, userID := range h.users {
		emit(&evtv1.Event{Event: &evtv1.Event_UserNotificationPolicyChanged{UserNotificationPolicyChanged: &evtv1.UserNotificationPolicyChangedEvent{
			UserId: userID, Overrides: &evtv1.NotificationDeliveryModes{
				DirectMessages: badge, DirectMentions: badge, Replies: badge, RoleMentions: badge, HereMentions: badge,
				AllMentions: badge, FollowedThreads: badge, Reactions: badge, RoomMessages: badge,
			},
		}}})
	}

	// orphans holds message IDs whose body arrived before their post.
	var orphans []string
	type reactionKey struct{ message, user, emoji string }
	var reactions []reactionKey
	var replies []string
	deletedR2 := false
	deletedU4 := false
	shredded := false
	epochs := make(map[string]int32)
	liveRooms := func() []string {
		if deletedR2 {
			return []string{"R1", "D1"}
		}
		return h.rooms
	}
	post := func(roomID, actorID string, fill func(*evtv1.MessagePostedEvent)) string {
		messageID := id("M")
		if len(orphans) > 0 && rng.IntN(2) == 0 {
			messageID, orphans = orphans[0], orphans[1:]
		}
		posted := &evtv1.MessagePostedEvent{RoomId: roomID, AuthorId: actorID}
		if rng.IntN(4) == 0 {
			posted.Mentions = append(posted.Mentions, randomMention(rng, h.users))
		}
		if fill != nil {
			fill(posted)
		}
		emit(&evtv1.Event{Id: messageID, ActorId: actorID, Event: &evtv1.Event_MessagePosted{MessagePosted: posted}})
		h.messages = append(h.messages, messageID)
		h.messageRoom[messageID] = roomID
		return messageID
	}
	body := func(messageID, roomID, authorID string) {
		var assets []string
		for range rng.IntN(3) {
			assets = append(assets, id("A"))
		}
		bodyID := id("B")
		emit(&evtv1.Event{Id: bodyID, ActorId: authorID, Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{
			RoomId: roomID, EventId: messageID,
			Body: &evtv1.MessageBody{AuthorId: authorID, BodyEventId: bodyID, EncryptedBody: []byte("ciphertext"), AssetIds: assets},
		}}})
	}
	// author returns the posting actor of a message.
	author := func(messageID string) string {
		for _, event := range h.events {
			if event.GetId() == messageID && event.GetMessagePosted() != nil {
				return event.GetActorId()
			}
		}
		return ""
	}

	generateKey := func(userID string, purpose evtv1.UserDEKPurpose) {
		key := fmt.Sprintf("%s/%d", userID, purpose)
		epochs[key]++
		emit(&evtv1.Event{Event: &evtv1.Event_UserDekGenerated{UserDekGenerated: &evtv1.UserDEKGeneratedEvent{
			UserId: userID, Purpose: purpose, Epoch: epochs[key], ContentKeyRef: "dek." + key + "." + fmt.Sprint(epochs[key]),
			WrappingKeyRef: "kek." + userID, WrappingAlgorithm: pick([]string{"aes-kw", "xchacha"}), WrappingMetadata: []byte{byte(epochs[key])},
		}}})
	}
	for _, userID := range h.users {
		for purpose := range evtv1.UserDEKPurpose(3) {
			for range rng.IntN(3) {
				generateKey(userID, purpose)
			}
		}
	}

	for step := range steps {
		if step == steps*3/5 {
			// A gap longer than notificationTTL expires the earlier Badge
			// sources. Live state and a restored snapshot must still agree.
			clock += int(notificationTTL / time.Second)
		}
		if !deletedR2 && step == steps*4/5 {
			emit(&evtv1.Event{Event: &evtv1.Event_RoomDeleted{RoomDeleted: &evtv1.RoomDeletedEvent{RoomId: "R2"}}})
			deletedR2 = true
			continue
		}
		if !deletedU4 && step == steps*9/10 {
			emit(&evtv1.Event{Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: "U4"}}})
			deletedU4 = true
			continue
		}
		roomID := pick(liveRooms())
		actorID := pick(memberOf(roomID))
		roomMessages := func() []string {
			var ids []string
			for _, messageID := range h.messages {
				if h.messageRoom[messageID] == roomID {
					ids = append(ids, messageID)
				}
			}
			return ids
		}
		switch roll := rng.IntN(100); {
		case roll < 22:
			rootID := post(roomID, actorID, nil)
			h.roots[roomID] = append(h.roots[roomID], rootID)
			if rng.IntN(3) > 0 {
				body(rootID, roomID, actorID)
			}
		case roll < 42:
			if len(h.roots[roomID]) == 0 {
				continue
			}
			rootID := pick(h.roots[roomID])
			replyID := post(roomID, actorID, func(posted *evtv1.MessagePostedEvent) {
				posted.InThread = rootID
				if rng.IntN(2) == 0 {
					posted.InReplyTo = pick(roomMessages())
				}
			})
			replies = append(replies, replyID)
			body(replyID, roomID, actorID)
		case roll < 46:
			var candidates []string
			for _, replyID := range replies {
				if h.messageRoom[replyID] == roomID {
					candidates = append(candidates, replyID)
				}
			}
			if len(candidates) == 0 {
				continue
			}
			replyID := pick(candidates)
			var rootID string
			for _, event := range h.events {
				if event.GetId() == replyID {
					rootID = event.GetMessagePosted().GetInThread()
				}
			}
			post(roomID, author(replyID), func(posted *evtv1.MessagePostedEvent) {
				posted.EchoOfEventId = replyID
				posted.EchoFromThreadRootEventId = rootID
				posted.Mentions = nil
			})
		case roll < 50:
			orphanID := id("M")
			orphans = append(orphans, orphanID)
			body(orphanID, roomID, actorID)
		case roll < 58:
			if messages := roomMessages(); len(messages) > 0 {
				messageID := pick(messages)
				body(messageID, roomID, author(messageID))
			}
		case roll < 60:
			if messages := roomMessages(); len(messages) > 0 {
				emit(editedEvent("", pick(messages), roomID, actorID, "", 0))
			}
		case roll < 64:
			if messages := roomMessages(); len(messages) > 0 {
				emit(retractedEvent("", pick(messages), roomID, actorID, "", 0))
			}
		case roll < 78:
			if messages := roomMessages(); len(messages) > 0 {
				key := reactionKey{message: pick(messages), user: actorID, emoji: pick(h.emojis)}
				reactions = append(reactions, key)
				emit(&evtv1.Event{ActorId: key.user, Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{
					RoomId: roomID, MessageEventId: key.message, Emoji: key.emoji,
				}}})
			}
		case roll < 84:
			if len(reactions) == 0 {
				continue
			}
			key := reactions[rng.IntN(len(reactions))]
			emit(&evtv1.Event{ActorId: key.user, Event: &evtv1.Event_ReactionRemoved{ReactionRemoved: &evtv1.ReactionRemovedEvent{
				RoomId: h.messageRoom[key.message], MessageEventId: key.message, Emoji: key.emoji,
			}}})
		case roll < 91:
			if len(h.roots[roomID]) == 0 {
				continue
			}
			emit(threadFollowSnapshotTestEvent("", roomID, pick(h.roots[roomID]), pick(h.users), rng.IntN(3) > 0))
		case roll < 95:
			if messages := roomMessages(); len(messages) > 0 {
				messageID := pick(messages)
				if rng.IntN(3) > 0 {
					emit(&evtv1.Event{ActorId: actorID, Event: &evtv1.Event_MessagePinned{MessagePinned: &evtv1.MessagePinnedEvent{RoomId: roomID, MessageEventId: messageID}}})
				} else {
					emit(&evtv1.Event{ActorId: actorID, Event: &evtv1.Event_MessageUnpinned{MessageUnpinned: &evtv1.MessageUnpinnedEvent{RoomId: roomID, MessageEventId: messageID}}})
				}
			}
		case roll < 96:
			// Asset processing facts name only the message, so the reaction
			// projection finds their room through its message index.
			if messages := roomMessages(); len(messages) > 0 {
				emit(&evtv1.Event{Event: &evtv1.Event_AssetProcessingStarted{AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{
					AssetId: id("A"), MessageEventId: pick(messages),
				}}})
			}
		case roll < 97:
			generateKey(pick(h.users), evtv1.UserDEKPurpose(rng.IntN(3)))
		case roll < 98:
			userID := pick([]string{"U3", "U4"})
			if members["R1"][userID] {
				emit(&evtv1.Event{ActorId: userID, Event: &evtv1.Event_UserLeftRoom{UserLeftRoom: &evtv1.UserLeftRoomEvent{RoomId: "R1"}}})
			} else {
				emit(&evtv1.Event{ActorId: userID, Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"}}})
			}
			members["R1"][userID] = !members["R1"][userID]
		default:
			if !shredded && step > steps/2 {
				emit(userKeyShreddedSnapshotTestEvent("", "U5"))
				shredded = true
			}
		}
	}
	// Reads also cover IDs without projected state.
	h.messages = append(h.messages, orphans...)
	h.messages = append(h.messages, "UNKNOWN")
	h.now = clock + 1
	return h
}

func randomMention(rng *rand.Rand, users []string) *evtv1.MessageMention {
	userID := users[rng.IntN(len(users))]
	switch rng.IntN(4) {
	case 0:
		return &evtv1.MessageMention{UserId: userID, Cause: &evtv1.MessageMention_Role{Role: &evtv1.RoleMessageMention{}}}
	case 1:
		return &evtv1.MessageMention{UserId: userID, Cause: &evtv1.MessageMention_Here{Here: &evtv1.HereMessageMention{}}}
	case 2:
		return &evtv1.MessageMention{UserId: userID, Cause: &evtv1.MessageMention_All{All: &evtv1.AllMessageMention{}}}
	default:
		return &evtv1.MessageMention{UserId: userID, Cause: &evtv1.MessageMention_Direct{Direct: &evtv1.DirectUserMention{}}}
	}
}

// digest renders the results of the public reads of every projection over
// the history's IDs. It formats values only, never handles or pointers.
func (c *compactProjections) digest(t *testing.T, h *compactHistory) string {
	t.Helper()
	var out strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&out, format, args...)
		out.WriteByte('\n')
	}
	entry := func(e *TimelineEntry) string {
		if e == nil {
			return "<nil>"
		}
		return fmt.Sprintf("%+v", *e)
	}
	entries := func(values []*TimelineEntry) string {
		parts := make([]string, len(values))
		for i, value := range values {
			parts[i] = entry(value)
		}
		return strings.Join(parts, " | ")
	}

	tl := c.timeline
	for _, id := range h.messages {
		got, ok := tl.Get(id)
		line("timeline get %s: %v %s", id, ok, entry(got))
		reference, retracted, ok := tl.LatestBodyReference(id)
		line("timeline body %s: %+v %v %v current=%v", id, reference, retracted, ok, tl.BodyReferenceCurrent(reference))
		seqs, current, ok := tl.BodyEventSeqs(id)
		line("timeline body seqs %s: %v %d %v obsolete=%v", id, seqs, current, ok, tl.ObsoleteBodyEventSeqs(id))
		line("timeline hydration %s: %+v", id, tl.MessageHydrationState(id))
		contentID, ok := tl.ContentEventID(id)
		line("timeline content %s: %s %v", id, contentID, ok)
		echoID, echoOK := tl.ChannelEchoEventID(id)
		linkedEchoID, linkedEchoOK := tl.LinkedChannelEchoEventID(id)
		deletedAt, deleted := tl.MessageDeletedAt(id)
		line("timeline links %s: linked=%v echo=%v hidden=%v channel=%s,%v linkedChannel=%s,%v tombstoned=%v deleted=%v,%v",
			id, tl.LinkedEventIDs(id), tl.IsEcho(id), tl.IsHiddenEcho(id), echoID, echoOK, linkedEchoID, linkedEchoOK,
			tl.MessageTombstoned(id), deletedAt, deleted)
	}
	for _, roomID := range h.rooms {
		line("timeline room %s newest: %s", roomID, entries(tl.VisibleRoomTimeline(roomID, 1_000, 0, nil)))
		line("timeline room %s oldest: %s", roomID, entries(tl.VisibleRoomTimelineAfter(roomID, 1_000, 0, nil)))
		line("timeline room %s events: %s count=%d", roomID, entries(tl.RoomEvents(roomID, 1_000, 0)), tl.VisibleRoomEventCount(roomID))
		last, ok := tl.LastRoomMessageEntry(roomID)
		line("timeline room %s last: %v %s", roomID, ok, entry(last))
		for _, attachment := range tl.CurrentRoomAttachmentMessages(roomID) {
			line("timeline room %s attachment: %s %s %d %s %s %d", roomID, entry(attachment.Entry), attachment.BodyMessageEventID,
				attachment.BodySequence, attachment.BodyEventID, attachment.BodyAuthorID, attachment.AttachmentCount)
		}
		pins, latest := tl.PinnedMessagesWithLatest(roomID)
		line("timeline room %s pins: %+v %s", roomID, pins, latest)
		for _, userID := range h.users {
			at, ok := tl.LatestOriginalPostAt(roomID, userID)
			line("timeline room %s latest post %s: %v %v", roomID, userID, at, ok)
		}
		if messages := h.roots[roomID]; len(messages) > 0 {
			around, target, older, newer, ok := tl.VisibleRoomTimelineAround(roomID, messages[len(messages)/2], 7)
			line("timeline room %s around: %s %d %v %v %v", roomID, entries(around), target, older, newer, ok)
		}
	}
	obsolete := tl.AllObsoleteBodyEventSeqs()
	slices.Sort(obsolete)
	line("timeline obsolete: %v", obsolete)
	timelineRooms, timelineEntries, timelinePosts := tl.Stats()
	line("timeline stats: %d %d %d", timelineRooms, timelineEntries, timelinePosts)

	th := c.threads
	for _, id := range h.messages {
		metadata := th.ThreadMetadata(id)
		lastReply := "<nil>"
		if metadata.LastReplyAt != nil {
			lastReply = metadata.LastReplyAt.String()
		}
		line("thread %s: events=%+v exists=%v replies=%d metadata=%v,%d,%s,%d,%v,%s participants=%v",
			id, th.ThreadEvents(id), th.ThreadExists(id), th.ReplyCount(id), metadata.Exists, metadata.ReplyCount,
			metadata.LatestReplyEventID, metadata.ParticipantCount, metadata.ParticipantIDs, lastReply, th.ParticipantIDs(id))
		for _, roomID := range h.rooms {
			root, ok := th.ThreadRootForMessage(roomID, id)
			line("thread root %s in %s: %s %v followers=%v", id, roomID, root, ok, th.ThreadFollowers(roomID, id))
			for _, userID := range h.users {
				line("thread user %s %s %s: follow=%q interaction=%v", userID, roomID, id, th.FollowState(userID, roomID, id), th.HasInteraction(userID, roomID, id))
			}
		}
	}
	for _, userID := range h.users {
		followed := th.FollowedThreadsForUser(userID)
		refs := make([]string, len(followed))
		for i, ref := range followed {
			refs[i] = ref.roomID + "/" + ref.threadRootEventID
		}
		line("thread followed %s: %v", userID, refs)
	}
	threads, threadEntries, replies := th.Stats()
	line("thread stats: %d %d %d %d", threads, threadEntries, replies, th.ThreadCount())

	rp := c.reactions
	line("reactions batch: %+v", rp.ReactionsBatch(h.messages))
	for _, id := range h.messages {
		line("reactions %s: %+v", id, rp.Reactions(id))
		for _, userID := range h.users {
			for _, emoji := range h.emojis {
				line("reaction mutation %s %s %s: %+v has=%v", id, userID, emoji, rp.ReactionMutationSnapshot(h.messageRoom[id], id, emoji, userID), rp.HasReaction(id, emoji, userID))
			}
		}
	}
	for _, roomID := range h.rooms {
		line("reaction room %s: %d", roomID, rp.RoomSequence(roomID))
	}
	reactedMessages, activeReactions := rp.Stats()
	line("reaction stats: %d %d", reactedMessages, activeReactions)

	dek := func(event *evtv1.UserDEKGeneratedEvent, ok bool) string {
		if !ok {
			return "none"
		}
		return fmt.Sprintf("%s,%d,%d,%s,%s,%s,%x", event.GetUserId(), event.GetPurpose(), event.GetEpoch(), event.GetContentKeyRef(),
			event.GetWrappingKeyRef(), event.GetWrappingAlgorithm(), event.GetWrappingMetadata())
	}
	for _, userID := range h.users {
		for purpose := range evtv1.UserDEKPurpose(3) {
			line("content key active %s %d: %s", userID, purpose, dek(c.contentKeys.Active(userID, purpose)))
			for epoch := range int32(4) {
				line("content key %s %d %d: %s", userID, purpose, epoch, dek(c.contentKeys.Get(userID, purpose, epoch)))
			}
		}
	}

	now := fixedTime(h.now)
	if err := c.decisions.withCurrent(now, func(s *notificationDecisionSnapshot) error {
		for _, roomID := range h.rooms {
			scopes := append([]string{""}, h.roots[roomID]...)
			for _, scope := range scopes {
				line("decision thread %s %s: followers=%v replies=%d", roomID, scope, s.threadFollowerIDs(roomID, scope), s.threadReplyCount(scope))
				for _, userID := range h.users {
					line("decision badge %s %s %s: %v follow=%q", userID, roomID, scope, s.hasBadgeAttention(badgeQuery{
						userID: userID, roomID: roomID, threadRootEventID: scope, now: now,
					}), s.threadFollowState(userID, roomID, scope))
				}
			}
		}
		for _, id := range h.messages {
			// The message handle depends on the table, so the digest omits it.
			roomID, rootID, _, audience := s.badgeAudience(id)
			line("decision audience %s: %s %s %v", id, roomID, rootID, audience)
		}
		return nil
	}); err != nil {
		t.Fatalf("withCurrent: %v", err)
	}
	return out.String()
}

func TestCompactProjectionsAgreeAcrossTablesSnapshotsAndRestores(t *testing.T) {
	for seed := range uint64(6) {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			h := generateCompactHistory(seed, 300)

			private := newPrivateCompactProjections()
			private.apply(t, h.events, 1)
			wantSnapshots := private.snapshots(t)
			wantDigest := private.digest(t, h)
			if again := private.digest(t, h); again != wantDigest {
				t.Fatal("digest is not deterministic")
			}

			shared := newSharedCompactProjections(97)
			shared.apply(t, h.events, 1)
			assertSameSnapshots(t, "shared table", wantSnapshots, shared.snapshots(t))
			assertSameDigest(t, "shared table", wantDigest, shared.digest(t, h))

			restored := newSharedCompactProjections(13)
			restored.restore(t, wantSnapshots)
			assertSameSnapshots(t, "restored", wantSnapshots, restored.snapshots(t))
			assertSameDigest(t, "restored", wantDigest, restored.digest(t, h))

			// A restore that interrupts replay must continue to the same state.
			for _, cut := range []int{len(h.events) / 3, len(h.events) * 2 / 3} {
				first := newSharedCompactProjections(5)
				first.apply(t, h.events[:cut], 1)
				resumed := newSharedCompactProjections(211)
				resumed.restore(t, first.snapshots(t))
				resumed.apply(t, h.events[cut:], uint64(cut+1))
				label := fmt.Sprintf("restore at event %d", cut)
				assertSameSnapshots(t, label, wantSnapshots, resumed.snapshots(t))
				assertSameDigest(t, label, wantDigest, resumed.digest(t, h))
			}
		})
	}
}

// TestCompactProjectionsSurviveRestoreAfterStartupReplay checks the same
// equivalence after CompleteStartupReplay switches the replay guards from
// event IDs to stream sequences.
func TestCompactProjectionsSurviveRestoreAfterStartupReplay(t *testing.T) {
	h := generateCompactHistory(42, 300)
	cut := len(h.events) / 2
	complete := func(c *compactProjections) {
		c.timeline.CompleteStartupReplay()
		c.threads.CompleteStartupReplay()
		c.reactions.CompleteStartupReplay()
		c.decisions.CompleteStartupReplay()
		c.contentKeys.CompleteStartupReplay()
	}

	straight := newPrivateCompactProjections()
	straight.apply(t, h.events[:cut], 1)
	complete(straight)
	straight.apply(t, h.events[cut:], uint64(cut+1))

	first := newSharedCompactProjections(31)
	first.apply(t, h.events[:cut], 1)
	complete(first)
	resumed := newSharedCompactProjections(0)
	resumed.restore(t, first.snapshots(t))
	resumed.apply(t, h.events[cut:], uint64(cut+1))

	assertSameSnapshots(t, "restore after startup replay", straight.snapshots(t), resumed.snapshots(t))
	assertSameDigest(t, "restore after startup replay", straight.digest(t, h), resumed.digest(t, h))
}
