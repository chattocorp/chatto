package core

import (
	"cmp"
	"fmt"
	"slices"
	"time"
	"unsafe"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	notificationv1 "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

// badgeCause is one notification cause that can resolve to Badge.
type badgeCause uint8

const (
	badgeCauseRoomMessage badgeCause = iota
	badgeCauseDirectMessage
	badgeCauseFollowedThread
	badgeCauseDirectMention
	badgeCauseRoleMention
	badgeCauseHereMention
	badgeCauseAllMention
	badgeCauseReply
	badgeCauseReaction
	badgeCauseCount
)

// badgeCauseSignals holds one content-free signal per cause. The delivery-mode
// lookup only inspects the signal variant, so these values avoid allocating a
// signal for every evaluated source.
var badgeCauseSignals = [badgeCauseCount]*notificationv1.NotificationSignal{
	badgeCauseRoomMessage:    {Kind: &notificationv1.NotificationSignal_RoomMessageReceived{RoomMessageReceived: &notificationv1.RoomMessageReceived{}}},
	badgeCauseDirectMessage:  {Kind: &notificationv1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &notificationv1.DirectMessageReceived{}}},
	badgeCauseFollowedThread: {Kind: &notificationv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &notificationv1.FollowedThreadActivity{}}},
	badgeCauseDirectMention:  {Kind: &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{}}},
	badgeCauseRoleMention:    {Kind: &notificationv1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &notificationv1.RoleMentionReceived{}}},
	badgeCauseHereMention:    {Kind: &notificationv1.NotificationSignal_HereMentionReceived{HereMentionReceived: &notificationv1.HereMentionReceived{}}},
	badgeCauseAllMention:     {Kind: &notificationv1.NotificationSignal_AllMentionReceived{AllMentionReceived: &notificationv1.AllMentionReceived{}}},
	badgeCauseReply:          {Kind: &notificationv1.NotificationSignal_ReplyReceived{ReplyReceived: &notificationv1.ReplyReceived{}}},
	badgeCauseReaction:       {Kind: &notificationv1.NotificationSignal_ReactionReceived{ReactionReceived: &notificationv1.ReactionReceived{}}},
}

// badgeSourceKind identifies the cause of one targeted Badge source. Root
// messages and followed-thread replies are not targeted; they are found
// through the room's root and reply lists.
type badgeSourceKind uint8

const (
	badgeSourceDirectMention badgeSourceKind = iota + 1
	badgeSourceRoleMention
	badgeSourceHereMention
	badgeSourceAllMention
	badgeSourceReply
	badgeSourceReaction
	// badgeSourceFirstThreadReply is the first reply in a thread, addressed to
	// the thread root's author. Posting that reply auto-follows the author
	// only after the reply, so the follow range misses it; this source covers
	// it unless the author has explicitly unfollowed the thread.
	badgeSourceFirstThreadReply
)

func (kind badgeSourceKind) cause() (badgeCause, bool) {
	switch kind {
	case badgeSourceDirectMention:
		return badgeCauseDirectMention, true
	case badgeSourceRoleMention:
		return badgeCauseRoleMention, true
	case badgeSourceHereMention:
		return badgeCauseHereMention, true
	case badgeSourceAllMention:
		return badgeCauseAllMention, true
	case badgeSourceReply:
		return badgeCauseReply, true
	case badgeSourceReaction:
		return badgeCauseReaction, true
	case badgeSourceFirstThreadReply:
		return badgeCauseFollowedThread, true
	default:
		return 0, false
	}
}

func badgeSourceKindForMention(mention *evtv1.MessageMention) badgeSourceKind {
	switch mention.GetCause().(type) {
	case *evtv1.MessageMention_Direct:
		return badgeSourceDirectMention
	case *evtv1.MessageMention_Role:
		return badgeSourceRoleMention
	case *evtv1.MessageMention_Here:
		return badgeSourceHereMention
	case *evtv1.MessageMention_All:
		return badgeSourceAllMention
	default:
		return 0
	}
}

// badgeMessage is one indexed message post. thread is an eventIDs handle;
// room, actor, and author are ids handles. The record is never all zero, so a
// zero record in the messages slice means that no post is indexed.
type badgeMessage struct {
	seq uint64
	// createdAt is the post's compact creation time (see projectionTime).
	createdAt int64
	room      uint32
	// thread is the thread root handle, or zero for a root message.
	thread uint32
	// actor posted the message; a user's own activity never gives them Badge
	// attention.
	actor uint32
	// author receives replies to, and reactions on, this message.
	author    uint32
	retracted bool
	// source is false for echoes, historical imports, and posts without a
	// creation time, which are never Badge sources. They stay indexed as
	// reply parents and reaction targets.
	source bool
}

// badgeTargetedSource is one source addressed to one user. Its scope is the
// thread of its message.
type badgeTargetedSource struct {
	seq       uint64
	createdAt int64
	// message is the eventIDs handle of the source message, or of the reaction
	// target for a reaction.
	message uint32
	// reactor and emoji identify a reaction so its removal drops this source.
	// They are ids handles.
	reactor uint32
	emoji   uint32
	kind    badgeSourceKind
}

// badgeRoomSources holds one room's Badge sources. Every list is in stream
// order and holds only sources younger than notificationTTL at the time of the
// latest append. Message and thread handles are eventIDs handles; user handles
// are ids handles.
type badgeRoomSources struct {
	roots   []uint32
	replies map[uint32][]uint32
	// targeted maps a user to their sources per scope: zero for the room, or
	// a thread root handle.
	targeted map[uint32]map[uint32][]badgeTargetedSource
	// appends counts appends since the room's last sweep of quiet lists.
	appends int
}

// badgeSweepInterval is the number of appends to a room after which its
// reply and targeted lists are swept for expired sources. Appending only trims
// the list that grows, so quiet threads need the sweep.
const badgeSweepInterval = 512

type badgeMembershipKey struct {
	user uint32
	room uint32
}

type badgeReactionKey struct {
	message uint32
	reactor uint32
	emoji   uint32
}

// badgeThreadKey identifies a thread by its ids room handle and eventIDs root
// handle.
type badgeThreadKey struct {
	room   uint32
	thread uint32
}

// badgeFollowKey identifies one user's relationship with one thread.
type badgeFollowKey struct {
	user uint32
	badgeThreadKey
}

// notificationBadgeSources indexes every fact that can give a user Badge
// attention or decide thread notification recipients, so Badge attention is
// computed from current state when it is read instead of being stored per
// recipient. The NotificationDecisionProjection lock guards it, except for
// the event ID table, which synchronizes itself because it can be shared.
type notificationBadgeSources struct {
	// ids interns user, room, and emoji IDs.
	ids projectionIDTable
	// eventIDs interns message and thread-root event IDs. Production shares
	// the process's event ID table with the ServerContentView; a standalone
	// index owns a private table.
	eventIDs       *eventIDTable
	sharedEventIDs bool
	// messages holds the record of each indexed message post, indexed by
	// eventIDs handle. Records are pointer-free and stay indexed after their
	// sources expire, because later replies and reactions address them.
	messages handleSlice[badgeMessage]
	rooms    map[uint32]*badgeRoomSources
	// reactions holds the current indexed reactions. The reaction projection
	// keeps the first of repeated adds; so does the index.
	reactions map[badgeReactionKey]struct{}
	// memberSince holds the sequence of each current explicit membership's
	// first join. Sources at or before it predate the membership.
	memberSince map[badgeMembershipKey]uint64
	// accountSince and universalSince bound implicit universal-room
	// membership: it starts when both the account and universal access exist.
	accountSince   map[uint32]uint64
	universalSince map[uint32]uint64
	// follows maps a (user, room) pair to the threads that the user currently
	// follows there, with the sequence of the follow.
	follows map[badgeMembershipKey]map[uint32]uint64
	// followStates holds the latest explicit follow state of each user and
	// thread, including unfollows.
	followStates map[badgeFollowKey]compactThreadFollowState
	// followers holds the users who currently follow each thread.
	followers map[badgeThreadKey][]uint32
	// replyCounts counts the posted replies of each thread root handle.
	replyCounts map[uint32]uint64
	// latestCreatedAt is the creation time of the newest indexed source. The
	// sweep and the snapshot drop sources that are expired relative to it.
	latestCreatedAt int64
}

// newNotificationBadgeSources returns an empty index that interns event IDs in
// eventIDs. A nil table gives the index a private table.
func newNotificationBadgeSources(eventIDs *eventIDTable) *notificationBadgeSources {
	shared := eventIDs != nil
	if !shared {
		eventIDs = newEventIDTable()
	}
	return &notificationBadgeSources{
		ids:            newProjectionIDTable(),
		eventIDs:       eventIDs,
		sharedEventIDs: shared,
		rooms:          make(map[uint32]*badgeRoomSources),
		reactions:      make(map[badgeReactionKey]struct{}),
		memberSince:    make(map[badgeMembershipKey]uint64),
		accountSince:   make(map[uint32]uint64),
		universalSince: make(map[uint32]uint64),
		follows:        make(map[badgeMembershipKey]map[uint32]uint64),
		followStates:   make(map[badgeFollowKey]compactThreadFollowState),
		followers:      make(map[badgeThreadKey][]uint32),
		replyCounts:    make(map[uint32]uint64),
	}
}

// message returns the indexed record of a message handle.
func (b *notificationBadgeSources) message(message uint32) (badgeMessage, bool) {
	return b.messages.get(message)
}

// messageRecord returns the record of a message handle, or a zero record.
func (b *notificationBadgeSources) messageRecord(message uint32) badgeMessage {
	record, _ := b.messages.get(message)
	return record
}

func (b *notificationBadgeSources) room(room uint32) *badgeRoomSources {
	sources := b.rooms[room]
	if sources == nil {
		sources = &badgeRoomSources{replies: make(map[uint32][]uint32), targeted: make(map[uint32]map[uint32][]badgeTargetedSource)}
		b.rooms[room] = sources
	}
	return sources
}

// noteSource records a new source's creation time and sweeps the room's quiet
// lists every badgeSweepInterval appends.
func (b *notificationBadgeSources) noteSource(sources *badgeRoomSources, createdAt int64) {
	b.latestCreatedAt = max(b.latestCreatedAt, createdAt)
	sources.appends++
	if sources.appends < badgeSweepInterval {
		return
	}
	sources.appends = 0
	cutoff := expiredBefore(b.latestCreatedAt)
	for thread, replies := range sources.replies {
		drop := 0
		for drop < len(replies) && b.messageRecord(replies[drop]).createdAt <= cutoff {
			drop++
		}
		if drop == len(replies) {
			delete(sources.replies, thread)
		} else if drop > 0 {
			sources.replies[thread] = slices.Clone(replies[drop:])
		}
	}
	for user, scopes := range sources.targeted {
		for scope, targeted := range scopes {
			drop := 0
			for drop < len(targeted) && targeted[drop].createdAt <= cutoff {
				if expired := targeted[drop]; expired.kind == badgeSourceReaction {
					delete(b.reactions, badgeReactionKey{message: expired.message, reactor: expired.reactor, emoji: expired.emoji})
				}
				drop++
			}
			if drop == len(targeted) {
				delete(scopes, scope)
			} else if drop > 0 {
				scopes[scope] = slices.Clone(targeted[drop:])
			}
		}
		if len(scopes) == 0 {
			delete(sources.targeted, user)
		}
	}
}

// expiredBefore returns the creation time at or before which a source is
// older than notificationTTL relative to now.
func expiredBefore(now int64) int64 {
	return now - int64(notificationTTL)
}

// appendMessage appends a message handle and drops expired handles from the
// front of the list.
func (b *notificationBadgeSources) appendMessage(list []uint32, message uint32, createdAt int64) []uint32 {
	cutoff := expiredBefore(createdAt)
	drop := 0
	for drop < len(list) && b.messageRecord(list[drop]).createdAt <= cutoff {
		drop++
	}
	return append(list[drop:], message)
}

// addTargeted appends a targeted source and drops expired sources from the
// front of the list, forgetting the reactions among them.
func (b *notificationBadgeSources) addTargeted(sources *badgeRoomSources, user, scope uint32, source badgeTargetedSource) {
	scopes := sources.targeted[user]
	if scopes == nil {
		scopes = make(map[uint32][]badgeTargetedSource)
		sources.targeted[user] = scopes
	}
	list := scopes[scope]
	cutoff := expiredBefore(source.createdAt)
	drop := 0
	for drop < len(list) && list[drop].createdAt <= cutoff {
		if expired := list[drop]; expired.kind == badgeSourceReaction {
			delete(b.reactions, badgeReactionKey{message: expired.message, reactor: expired.reactor, emoji: expired.emoji})
		}
		drop++
	}
	scopes[scope] = append(list[drop:], source)
}

// apply indexes one decision-projection fact.
func (b *notificationBadgeSources) apply(event *evtv1.Event, seq uint64) {
	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		var replyCount uint64
		if rootID := payload.MessagePosted.GetInThread(); rootID != "" {
			thread := b.eventIDs.intern(rootID)
			b.replyCounts[thread]++
			replyCount = b.replyCounts[thread]
		}
		b.applyMessagePosted(event, payload.MessagePosted, seq, replyCount)
	case *evtv1.Event_MessageRetracted:
		if message, ok := b.eventIDs.lookup(payload.MessageRetracted.GetEventId()); ok {
			if record, exists := b.message(message); exists {
				record.retracted = true
				b.messages.set(message, record)
			}
		}
	case *evtv1.Event_ReactionAdded:
		b.applyReactionAdded(event, payload.ReactionAdded, seq)
	case *evtv1.Event_ReactionRemoved:
		b.applyReactionRemoved(event.GetActorId(), payload.ReactionRemoved)
	case *evtv1.Event_UserJoinedRoom:
		if event.GetActorId() != "" && payload.UserJoinedRoom.GetRoomId() != "" {
			key := badgeMembershipKey{user: b.ids.intern(event.GetActorId()), room: b.ids.intern(payload.UserJoinedRoom.GetRoomId())}
			if _, member := b.memberSince[key]; !member {
				b.memberSince[key] = seq
			}
		}
	case *evtv1.Event_UserLeftRoom:
		b.endMembership(event.GetActorId(), payload.UserLeftRoom.GetRoomId())
	case *evtv1.Event_RoomMemberBanned:
		b.endMembership(payload.RoomMemberBanned.GetUserId(), payload.RoomMemberBanned.GetRoomId())
	case *evtv1.Event_RoomUniversalChanged:
		if roomID := payload.RoomUniversalChanged.GetRoomId(); roomID != "" {
			room := b.ids.intern(roomID)
			if payload.RoomUniversalChanged.GetUniversal() {
				if _, exists := b.universalSince[room]; !exists {
					b.universalSince[room] = seq
				}
			} else {
				delete(b.universalSince, room)
			}
		}
	case *evtv1.Event_RoomCreated:
		if room := payload.RoomCreated; room.GetUniversal() && room.GetRoomId() != "" {
			b.universalSince[b.ids.intern(room.GetRoomId())] = seq
		}
	case *evtv1.Event_RoomDeleted:
		if room, ok := b.ids.lookup(payload.RoomDeleted.GetRoomId()); ok {
			b.deleteRoom(room)
		}
	case *evtv1.Event_UserAccountCreated:
		if userID := payload.UserAccountCreated.GetUserId(); userID != "" {
			b.accountSince[b.ids.intern(userID)] = seq
		}
	case *evtv1.Event_UserAccountDeleted:
		if user, ok := b.ids.lookup(payload.UserAccountDeleted.GetUserId()); ok {
			b.deleteUser(user)
		}
	case *evtv1.Event_ThreadFollowed:
		follow := payload.ThreadFollowed
		if follow.GetUserId() == "" || follow.GetRoomId() == "" || follow.GetThreadRootEventId() == "" {
			return
		}
		key := badgeFollowKey{user: b.ids.intern(follow.GetUserId()), badgeThreadKey: badgeThreadKey{room: b.ids.intern(follow.GetRoomId()), thread: b.eventIDs.intern(follow.GetThreadRootEventId())}}
		b.setFollowState(key, compactThreadFollowFollowing)
		membership := badgeMembershipKey{user: key.user, room: key.room}
		threads := b.follows[membership]
		if threads == nil {
			threads = make(map[uint32]uint64)
			b.follows[membership] = threads
		}
		if _, following := threads[key.thread]; !following {
			threads[key.thread] = seq
		}
	case *evtv1.Event_ThreadUnfollowed:
		unfollow := payload.ThreadUnfollowed
		if unfollow.GetUserId() == "" || unfollow.GetRoomId() == "" || unfollow.GetThreadRootEventId() == "" {
			return
		}
		key := badgeFollowKey{user: b.ids.intern(unfollow.GetUserId()), badgeThreadKey: badgeThreadKey{room: b.ids.intern(unfollow.GetRoomId()), thread: b.eventIDs.intern(unfollow.GetThreadRootEventId())}}
		b.setFollowState(key, compactThreadFollowUnfollowed)
		membership := badgeMembershipKey{user: key.user, room: key.room}
		delete(b.follows[membership], key.thread)
		if len(b.follows[membership]) == 0 {
			delete(b.follows, membership)
		}
	}
}

// setFollowState records a user's latest explicit follow state for a thread
// and maintains the thread's follower list.
func (b *notificationBadgeSources) setFollowState(key badgeFollowKey, state compactThreadFollowState) {
	if b.followStates[key] == compactThreadFollowFollowing {
		followers := slices.DeleteFunc(b.followers[key.badgeThreadKey], func(user uint32) bool { return user == key.user })
		if len(followers) == 0 {
			delete(b.followers, key.badgeThreadKey)
		} else {
			b.followers[key.badgeThreadKey] = followers
		}
	}
	b.followStates[key] = state
	if state == compactThreadFollowFollowing {
		b.followers[key.badgeThreadKey] = append(b.followers[key.badgeThreadKey], key.user)
	}
}

// threadKey returns the handles of a thread without interning its IDs.
func (b *notificationBadgeSources) threadKey(roomID, threadRootEventID string) (badgeThreadKey, bool) {
	room, roomKnown := b.ids.lookup(roomID)
	thread, threadKnown := b.eventIDs.lookup(threadRootEventID)
	return badgeThreadKey{room: room, thread: thread}, roomKnown && threadKnown
}

// followState returns a user's latest explicit follow state for a thread.
func (b *notificationBadgeSources) followState(userID, roomID, threadRootEventID string) ThreadFollowState {
	user, userKnown := b.ids.lookup(userID)
	thread, threadKnown := b.threadKey(roomID, threadRootEventID)
	if !userKnown || !threadKnown {
		return ThreadFollowStateNone
	}
	return b.followStates[badgeFollowKey{user: user, badgeThreadKey: thread}].public()
}

// followerIDs returns the users who currently follow a thread, sorted.
func (b *notificationBadgeSources) followerIDs(roomID, threadRootEventID string) []string {
	thread, known := b.threadKey(roomID, threadRootEventID)
	if !known {
		return []string{}
	}
	followers := b.followers[thread]
	userIDs := make([]string, len(followers))
	for i, user := range followers {
		userIDs[i] = b.ids.id(user)
	}
	slices.Sort(userIDs)
	return userIDs
}

// replyCount returns the number of posted replies of a thread root.
func (b *notificationBadgeSources) replyCount(threadRootEventID string) uint64 {
	thread, ok := b.eventIDs.lookup(threadRootEventID)
	if !ok {
		return 0
	}
	return b.replyCounts[thread]
}

func (b *notificationBadgeSources) endMembership(userID, roomID string) {
	user, userKnown := b.ids.lookup(userID)
	room, roomKnown := b.ids.lookup(roomID)
	if userKnown && roomKnown {
		delete(b.memberSince, badgeMembershipKey{user: user, room: room})
	}
}

// deleteRoom drops every indexed fact of a deleted room. Interned IDs stay in
// the append-only table.
func (b *notificationBadgeSources) deleteRoom(room uint32) {
	delete(b.rooms, room)
	delete(b.universalSince, room)
	for i, record := range b.messages {
		if record.room == room {
			b.messages[i] = badgeMessage{}
		}
	}
	for key := range b.reactions {
		if _, exists := b.message(key.message); !exists {
			delete(b.reactions, key)
		}
	}
	for key := range b.memberSince {
		if key.room == room {
			delete(b.memberSince, key)
		}
	}
	for key := range b.follows {
		if key.room == room {
			delete(b.follows, key)
		}
	}
}

// deleteUser drops the state of a deleted account. Its messages stay indexed
// as roots, replies, and reply parents for other users.
func (b *notificationBadgeSources) deleteUser(user uint32) {
	delete(b.accountSince, user)
	for _, sources := range b.rooms {
		for _, targeted := range sources.targeted[user] {
			for _, source := range targeted {
				if source.kind == badgeSourceReaction {
					delete(b.reactions, badgeReactionKey{message: source.message, reactor: source.reactor, emoji: source.emoji})
				}
			}
		}
		delete(sources.targeted, user)
	}
	for key := range b.memberSince {
		if key.user == user {
			delete(b.memberSince, key)
		}
	}
	for key := range b.follows {
		if key.user == user {
			delete(b.follows, key)
		}
	}
}

func (b *notificationBadgeSources) applyMessagePosted(event *evtv1.Event, posted *evtv1.MessagePostedEvent, seq uint64, replyCount uint64) {
	if event.GetId() == "" || posted.GetRoomId() == "" {
		return
	}
	message := b.eventIDs.intern(event.GetId())
	if _, exists := b.message(message); exists {
		return
	}
	record := badgeMessage{
		seq:       seq,
		createdAt: eventCreatedNanos(event),
		room:      b.ids.intern(posted.GetRoomId()),
		thread:    b.eventIDs.intern(posted.GetInThread()),
		actor:     b.ids.intern(event.GetActorId()),
		author:    b.ids.intern(messageAuthorID(event)),
	}
	record.source = record.createdAt != 0 && posted.GetEchoOfEventId() == "" && !posted.GetHistoricalImport()
	b.messages.set(message, record)
	if !record.source {
		return
	}
	sources := b.room(record.room)
	b.noteSource(sources, record.createdAt)
	if record.thread == 0 {
		sources.roots = b.appendMessage(sources.roots, message, record.createdAt)
	} else {
		sources.replies[record.thread] = b.appendMessage(sources.replies[record.thread], message, record.createdAt)
		if replyCount == 1 {
			if root, exists := b.message(record.thread); exists && root.author != 0 && root.author != record.actor {
				b.addTargeted(sources, root.author, record.thread, badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: badgeSourceFirstThreadReply})
			}
		}
	}
	for _, mention := range posted.GetMentions() {
		kind := badgeSourceKindForMention(mention)
		if kind == 0 || mention.GetUserId() == "" || mention.GetUserId() == event.GetActorId() {
			continue
		}
		b.addTargeted(sources, b.ids.intern(mention.GetUserId()), record.thread, badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: kind})
	}
	if parentID := posted.GetInReplyTo(); parentID != "" {
		if parent, ok := b.eventIDs.lookup(parentID); ok {
			if parentRecord, exists := b.message(parent); exists && parentRecord.author != 0 && parentRecord.author != record.actor {
				b.addTargeted(sources, parentRecord.author, record.thread, badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: badgeSourceReply})
			}
		}
	}
}

func (b *notificationBadgeSources) applyReactionAdded(event *evtv1.Event, reaction *evtv1.ReactionAddedEvent, seq uint64) {
	target, ok := b.eventIDs.lookup(reaction.GetMessageEventId())
	createdAt := eventCreatedNanos(event)
	if !ok || reaction.GetEmoji() == "" || event.GetActorId() == "" || createdAt == 0 {
		return
	}
	record, exists := b.message(target)
	if !exists || record.author == 0 {
		return
	}
	reactor := b.ids.intern(event.GetActorId())
	if reactor == record.author {
		return
	}
	key := badgeReactionKey{message: target, reactor: reactor, emoji: b.ids.intern(reaction.GetEmoji())}
	if _, exists := b.reactions[key]; exists {
		return
	}
	b.reactions[key] = struct{}{}
	sources := b.room(record.room)
	b.noteSource(sources, createdAt)
	b.addTargeted(sources, record.author, record.thread, badgeTargetedSource{
		seq: seq, createdAt: createdAt, message: target, reactor: reactor, emoji: key.emoji, kind: badgeSourceReaction,
	})
}

func (b *notificationBadgeSources) applyReactionRemoved(reactorID string, reaction *evtv1.ReactionRemovedEvent) {
	target, targetKnown := b.eventIDs.lookup(reaction.GetMessageEventId())
	reactor, reactorKnown := b.ids.lookup(reactorID)
	emoji, emojiKnown := b.ids.lookup(reaction.GetEmoji())
	if !targetKnown || !reactorKnown || !emojiKnown {
		return
	}
	key := badgeReactionKey{message: target, reactor: reactor, emoji: emoji}
	if _, exists := b.reactions[key]; !exists {
		return
	}
	delete(b.reactions, key)
	record, _ := b.message(target)
	sources := b.rooms[record.room]
	if sources == nil || sources.targeted[record.author] == nil {
		return
	}
	scopes := sources.targeted[record.author]
	scopes[record.thread] = slices.DeleteFunc(scopes[record.thread], func(source badgeTargetedSource) bool {
		return source.kind == badgeSourceReaction && source.message == target && source.reactor == reactor && source.emoji == emoji
	})
}

// badgeQuery asks whether a user has Badge attention in one room or thread.
type badgeQuery struct {
	userID string
	roomID string
	// threadRootEventID limits the query to one thread. An empty value asks
	// for the room, including all of its threads.
	threadRootEventID string
	// before excludes sources at or after this sequence when it is non-zero.
	// The materializer uses it to learn whether a new source turns attention
	// on.
	before uint64
	now    time.Time
	// readBoundary returns the user's read boundary for a scope of the room.
	// It must not block, because the decision projection lock is held.
	readBoundary func(threadRootEventID string) (notificationReadBoundary, bool)
	// unretracted names a message handle that counts as not retracted. A
	// retraction compares attention with and without it to find the users
	// whose state changed.
	unretracted uint32
}

// hasBadgeAttention reports whether a current source gives the user Badge
// attention. A source counts only when all of these hold:
//   - its cause resolves to Badge under the user's current notification policy;
//   - the user can currently see it;
//   - it is after the start of the user's current membership, and for a
//     followed thread after the follow;
//   - the read boundary of its scope does not cover it;
//   - it is not the user's own, not retracted, and younger than notificationTTL.
//
// Each list is scanned from its newest source and stops at the first
// qualifying source or at the first source that a bound excludes.
func (s *notificationDecisionSnapshot) hasBadgeAttention(q badgeQuery) bool {
	b := s.badges
	if _, active := s.activeUsers[q.userID]; !active {
		return false
	}
	room, roomKnown := b.ids.lookup(q.roomID)
	sources := b.rooms[room]
	if !roomKnown || sources == nil {
		return false
	}
	kind, exists := s.roomKind(q.roomID)
	if !exists {
		return false
	}
	broad := s.notificationVisibilityExists(q.userID, q.roomID)
	interaction := !broad && s.notificationInteractionVisibilityExists(q.userID, q.roomID)
	if !broad && !interaction {
		return false
	}
	user, _ := b.ids.lookup(q.userID)
	scopeThread := uint32(0)
	if q.threadRootEventID != "" {
		thread, ok := b.eventIDs.lookup(q.threadRootEventID)
		if !ok {
			return false
		}
		scopeThread = thread
	}
	includes := func(thread uint32) bool {
		return q.threadRootEventID == "" || thread == scopeThread
	}
	lower := s.badgeMembershipStart(user, room, q.userID, q.roomID)
	expired := expiredBefore(q.now.UnixNano())
	var modes badgeModeCache
	badge := func(cause badgeCause) bool {
		return modes.badge(s, q.userID, q.roomID, cause)
	}
	readBoundary := func(thread uint32) (notificationReadBoundary, bool) {
		if q.readBoundary == nil {
			return notificationReadBoundary{}, false
		}
		return q.readBoundary(b.eventIDs.id(thread))
	}
	// floor returns the highest sequence at or below which a scope's messages
	// do not count.
	floor := func(thread uint32, since uint64) uint64 {
		result := max(lower, since)
		if boundary, ok := readBoundary(thread); ok {
			result = max(result, boundary.targetSequence)
		}
		return result
	}
	excluded := func(seq uint64) bool { return q.before != 0 && seq >= q.before }
	// newestMessage reports whether a message list has a qualifying message
	// above the floor.
	newestMessage := func(list []uint32, floor uint64) bool {
		for i := len(list) - 1; i >= 0; i-- {
			message := b.messageRecord(list[i])
			if message.seq <= floor || message.createdAt <= expired {
				return false
			}
			if !excluded(message.seq) && (!message.retracted || list[i] == q.unretracted) && message.actor != user {
				return true
			}
		}
		return false
	}

	// Root activity: every root message by another member.
	rootCause := badgeCauseRoomMessage
	if kind == KindDM {
		rootCause = badgeCauseDirectMessage
	}
	if includes(0) && broad && badge(rootCause) && newestMessage(sources.roots, floor(0, 0)) {
		return true
	}

	// Sources addressed to this user: mentions, replies, reactions, and the
	// first reply in a thread that the user started.
	for thread, targeted := range sources.targeted[user] {
		if !includes(thread) || len(targeted) == 0 {
			continue
		}
		if newest := targeted[len(targeted)-1]; newest.seq <= lower || newest.createdAt <= expired {
			continue
		}
		boundary, hasBoundary := readBoundary(thread)
		// A reaction is covered when its target and the reaction are within
		// the read boundary. A reaction at or below the boundary's target
		// sequence meets both conditions, so every kind can stop there.
		stop := lower
		if hasBoundary {
			stop = max(stop, boundary.targetSequence)
		}
		for i := len(targeted) - 1; i >= 0; i-- {
			source := targeted[i]
			if source.seq <= stop || source.createdAt <= expired {
				break
			}
			if excluded(source.seq) {
				continue
			}
			message, exists := b.message(source.message)
			if !exists || (message.retracted && source.message != q.unretracted) {
				continue
			}
			if !broad && source.kind != badgeSourceDirectMention {
				continue
			}
			if source.kind == badgeSourceFirstThreadReply && b.followStates[badgeFollowKey{user: user, badgeThreadKey: badgeThreadKey{room: room, thread: thread}}] == compactThreadFollowUnfollowed {
				continue
			}
			if cause, ok := source.kind.cause(); !ok || !badge(cause) {
				continue
			}
			if source.kind == badgeSourceReaction && hasBoundary && message.seq <= boundary.targetSequence && source.seq <= boundary.observedSequence {
				continue
			}
			return true
		}
	}

	// Activity in threads that the user follows, after the follow began.
	if broad && user != 0 && badge(badgeCauseFollowedThread) {
		for thread, since := range b.follows[badgeMembershipKey{user: user, room: room}] {
			replies := sources.replies[thread]
			if !includes(thread) || len(replies) == 0 {
				continue
			}
			if newest := b.messageRecord(replies[len(replies)-1]); newest.seq <= max(lower, since) || newest.createdAt <= expired {
				continue
			}
			if newestMessage(replies, floor(thread, since)) {
				return true
			}
		}
	}
	return false
}

// badgeModeCache resolves each cause's delivery mode at most once per query.
type badgeModeCache struct {
	known [badgeCauseCount]bool
	value [badgeCauseCount]bool
}

func (c *badgeModeCache) badge(s *notificationDecisionSnapshot, userID, roomID string, cause badgeCause) bool {
	if !c.known[cause] {
		c.known[cause] = true
		c.value[cause] = s.effectiveNotificationMode(userID, roomID, badgeCauseSignals[cause]) == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE
	}
	return c.value[cause]
}

// badgeMembershipStart returns the sequence at which the user's current
// membership of the room began. Sources at or before it never give Badge
// attention, so joining a room does not mark its history as unread.
func (s *notificationDecisionSnapshot) badgeMembershipStart(user, room uint32, userID, roomID string) uint64 {
	b := s.badges
	if s.rooms.Membership.IsMember(roomID, userID) {
		return b.memberSince[badgeMembershipKey{user: user, room: room}]
	}
	// Implicit universal-room membership begins when both the account and the
	// room's universal access exist.
	return max(b.accountSince[user], b.universalSince[room])
}

// badgeAudience returns the users whose Badge attention a message could have
// given, with the message's scope and handle: room members for a root
// message; for a thread reply, the thread's followers and root author; and the
// users addressed by a current targeted source of the message, including the
// author for reactions on it. A targeted source that is older than
// notificationTTL at the snapshot time does not count: it cannot give
// attention, and a snapshot restore drops it. Only users who can currently see
// the room are returned, so the result never names a room to a user outside
// it.
func (s *notificationDecisionSnapshot) badgeAudience(messageEventID string) (roomID, threadRootEventID string, message uint32, userIDs []string) {
	b := s.badges
	message, ok := b.eventIDs.lookup(messageEventID)
	if !ok {
		return "", "", 0, nil
	}
	record, exists := b.message(message)
	if !exists {
		return "", "", 0, nil
	}
	roomID, threadRootEventID = b.ids.id(record.room), b.eventIDs.id(record.thread)
	users := make(map[string]struct{})
	if record.thread == 0 {
		for _, userID := range s.roomMemberIDs(roomID) {
			users[userID] = struct{}{}
		}
	} else {
		for _, userID := range s.threadFollowerIDs(roomID, threadRootEventID) {
			users[userID] = struct{}{}
		}
		if root, exists := b.message(record.thread); exists && root.author != 0 {
			users[b.ids.id(root.author)] = struct{}{}
		}
	}
	if sources := b.rooms[record.room]; sources != nil {
		expired := expiredBefore(s.at.UnixNano())
		for user, scopes := range sources.targeted {
			if slices.ContainsFunc(scopes[record.thread], func(source badgeTargetedSource) bool {
				return source.message == message && source.createdAt > expired
			}) {
				users[b.ids.id(user)] = struct{}{}
			}
		}
	}
	visible := make([]string, 0, len(users))
	for _, userID := range sortedMapKeys(users) {
		if s.badgeRoomVisible(userID, roomID) {
			visible = append(visible, userID)
		}
	}
	return roomID, threadRootEventID, message, visible
}

// badgeRoomVisible reports whether the user can currently see messages in the
// room, with broad or interaction-scoped read access.
func (s *notificationDecisionSnapshot) badgeRoomVisible(userID, roomID string) bool {
	return s.notificationVisibilityExists(userID, roomID) || s.notificationInteractionVisibilityExists(userID, roomID)
}

// badgeRoomsForUser lists the rooms where the user is an explicit member, plus
// the universal channel rooms that the user can currently join.
func (s *notificationDecisionSnapshot) badgeRoomsForUser(userID string) []string {
	rooms := make(map[string]struct{})
	for _, roomID := range s.rooms.Membership.Rooms(userID) {
		rooms[roomID] = struct{}{}
	}
	for _, room := range s.rooms.Catalog.AllByKind(evtv1.RoomKind_ROOM_KIND_CHANNEL) {
		if room.GetUniversal() && s.membershipExists(userID, room.GetId()) {
			rooms[room.GetId()] = struct{}{}
		}
	}
	return sortedMapKeys(rooms)
}

// estimatedBytes approximates the retained size of the index. A shared event
// ID table is counted by the ServerContentView estimate instead.
func (b *notificationBadgeSources) estimatedBytes() int64 {
	// entry is the cost of one map entry with a key and value of these sizes.
	entry := func(key, value uintptr) int64 { return projectionCompactMapEntryOverhead + int64(key+value) }
	const (
		handle   = unsafe.Sizeof(uint32(0))
		sequence = unsafe.Sizeof(uint64(0))
		list     = unsafe.Sizeof([]uint32(nil))
		table    = unsafe.Sizeof(map[uint32]uint64(nil))
	)
	bytes := b.ids.estimatedBytes()
	if !b.sharedEventIDs {
		bytes += b.eventIDs.estimatedBytes()
	}
	bytes += int64(cap(b.messages)) * int64(unsafe.Sizeof(badgeMessage{}))
	bytes += int64(len(b.reactions)) * entry(unsafe.Sizeof(badgeReactionKey{}), 0)
	bytes += int64(len(b.memberSince)) * entry(unsafe.Sizeof(badgeMembershipKey{}), sequence)
	bytes += int64(len(b.accountSince)+len(b.universalSince)) * entry(handle, sequence)
	for _, sources := range b.rooms {
		bytes += entry(handle, unsafe.Sizeof(sources)) + int64(unsafe.Sizeof(*sources))
		bytes += int64(cap(sources.roots) * int(handle))
		for _, replies := range sources.replies {
			bytes += entry(handle, list) + int64(cap(replies)*int(handle))
		}
		for _, scopes := range sources.targeted {
			bytes += entry(handle, table) + projectionMapEntryOverhead
			for _, targeted := range scopes {
				bytes += entry(handle, list) + int64(cap(targeted)*int(unsafe.Sizeof(badgeTargetedSource{})))
			}
		}
	}
	for _, threads := range b.follows {
		bytes += entry(unsafe.Sizeof(badgeMembershipKey{}), table) + projectionMapEntryOverhead + int64(len(threads))*entry(handle, sequence)
	}
	bytes += int64(len(b.followStates)) * entry(unsafe.Sizeof(badgeFollowKey{}), unsafe.Sizeof(compactThreadFollowState(0)))
	for _, followers := range b.followers {
		bytes += entry(unsafe.Sizeof(badgeThreadKey{}), list) + int64(cap(followers)*int(handle))
	}
	bytes += int64(len(b.replyCounts)) * entry(handle, sequence)
	return bytes
}

// snapshot encodes the index. Messages are in stream order and targeted
// sources are in stream order within each user, room, and scope, so restore
// rebuilds the same ordered lists.
func (b *notificationBadgeSources) snapshot() *projectionv1.NotificationBadgeSourcesSnapshot {
	snapshot := &projectionv1.NotificationBadgeSourcesSnapshot{LatestCreatedAtUnixNanos: b.latestCreatedAt}
	cutoff := expiredBefore(b.latestCreatedAt)
	messages := make([]uint32, 0, len(b.messages))
	for i, record := range b.messages {
		if record != (badgeMessage{}) {
			messages = append(messages, uint32(i+1))
		}
	}
	slices.SortFunc(messages, func(a, c uint32) int { return cmp.Compare(b.messageRecord(a).seq, b.messageRecord(c).seq) })
	for _, message := range messages {
		record := b.messageRecord(message)
		snapshot.Messages = append(snapshot.Messages, &projectionv1.NotificationBadgeMessageSnapshot{
			EventId: b.eventIDs.id(message), RoomId: b.ids.id(record.room), ThreadRootEventId: b.eventIDs.id(record.thread),
			ActorId: b.ids.id(record.actor), AuthorId: b.ids.id(record.author), Sequence: record.seq,
			CreatedAtUnixNanos: record.createdAt, Retracted: record.retracted, Source: record.source,
		})
	}
	for _, room := range sortedHandleKeys(&b.ids, b.rooms) {
		sources := b.rooms[room]
		for _, user := range sortedHandleKeys(&b.ids, sources.targeted) {
			for _, thread := range sortedHandleKeys(b.eventIDs, sources.targeted[user]) {
				for _, source := range sources.targeted[user][thread] {
					if source.createdAt <= cutoff {
						continue
					}
					snapshot.Targets = append(snapshot.Targets, &projectionv1.NotificationBadgeTargetSnapshot{
						UserId: b.ids.id(user), RoomId: b.ids.id(room), MessageEventId: b.eventIDs.id(source.message),
						Kind: uint32(source.kind), Sequence: source.seq, CreatedAtUnixNanos: source.createdAt,
						ReactorId: b.ids.id(source.reactor), Emoji: b.ids.id(source.emoji),
					})
				}
			}
		}
	}
	memberships := make([]badgeMembershipKey, 0, len(b.memberSince))
	for key := range b.memberSince {
		memberships = append(memberships, key)
	}
	slices.SortFunc(memberships, b.compareMembershipKeys)
	for _, key := range memberships {
		snapshot.Memberships = append(snapshot.Memberships, &projectionv1.NotificationBadgeSinceSnapshot{
			UserId: b.ids.id(key.user), RoomId: b.ids.id(key.room), Sequence: b.memberSince[key],
		})
	}
	for _, user := range sortedHandleKeys(&b.ids, b.accountSince) {
		snapshot.Accounts = append(snapshot.Accounts, &projectionv1.NotificationBadgeSinceSnapshot{UserId: b.ids.id(user), Sequence: b.accountSince[user]})
	}
	for _, room := range sortedHandleKeys(&b.ids, b.universalSince) {
		snapshot.UniversalRooms = append(snapshot.UniversalRooms, &projectionv1.NotificationBadgeSinceSnapshot{RoomId: b.ids.id(room), Sequence: b.universalSince[room]})
	}
	follows := make([]badgeMembershipKey, 0, len(b.follows))
	for key := range b.follows {
		follows = append(follows, key)
	}
	slices.SortFunc(follows, b.compareMembershipKeys)
	for _, key := range follows {
		for _, thread := range sortedHandleKeys(b.eventIDs, b.follows[key]) {
			snapshot.Follows = append(snapshot.Follows, &projectionv1.NotificationBadgeFollowSnapshot{
				UserId: b.ids.id(key.user), RoomId: b.ids.id(key.room), ThreadRootEventId: b.eventIDs.id(thread), Sequence: b.follows[key][thread],
			})
		}
	}
	return snapshot
}

// threadStateSnapshot encodes the explicit thread follow states ordered by
// user, room, and thread ID, and the reply counts ordered by thread ID.
func (b *notificationBadgeSources) threadStateSnapshot() ([]*projectionv1.ThreadFollowSnapshot, []*projectionv1.NotificationThreadStateSnapshot) {
	follows := make([]*projectionv1.ThreadFollowSnapshot, 0, len(b.followStates))
	for key, state := range b.followStates {
		follows = append(follows, &projectionv1.ThreadFollowSnapshot{
			UserId: b.ids.id(key.user), RoomId: b.ids.id(key.room), ThreadRootEventId: b.eventIDs.id(key.thread), State: string(state.public()),
		})
	}
	slices.SortFunc(follows, func(a, c *projectionv1.ThreadFollowSnapshot) int {
		return cmp.Or(cmp.Compare(a.GetUserId(), c.GetUserId()), cmp.Compare(a.GetRoomId(), c.GetRoomId()), cmp.Compare(a.GetThreadRootEventId(), c.GetThreadRootEventId()))
	})
	threads := make([]*projectionv1.NotificationThreadStateSnapshot, 0, len(b.replyCounts))
	for _, thread := range sortedHandleKeys(b.eventIDs, b.replyCounts) {
		threads = append(threads, &projectionv1.NotificationThreadStateSnapshot{ThreadRootEventId: b.eventIDs.id(thread), ReplyCount: b.replyCounts[thread]})
	}
	return follows, threads
}

func (b *notificationBadgeSources) compareMembershipKeys(a, c badgeMembershipKey) int {
	if byUser := cmp.Compare(b.ids.id(a.user), b.ids.id(c.user)); byUser != 0 {
		return byUser
	}
	return cmp.Compare(b.ids.id(a.room), b.ids.id(c.room))
}

// restoreNotificationBadgeSources rebuilds the index from a snapshot. It
// interns event IDs in eventIDs; a nil table gives the index a private table.
func restoreNotificationBadgeSources(snapshot *projectionv1.NotificationBadgeSourcesSnapshot, eventIDs *eventIDTable) (*notificationBadgeSources, error) {
	b := newNotificationBadgeSources(eventIDs)
	b.latestCreatedAt = snapshot.GetLatestCreatedAtUnixNanos()
	var previous uint64
	for _, row := range snapshot.GetMessages() {
		if row.GetEventId() == "" || row.GetRoomId() == "" || row.GetSequence() == 0 || row.GetSequence() <= previous {
			return nil, fmt.Errorf("notification badge snapshot has an invalid message")
		}
		previous = row.GetSequence()
		message := b.eventIDs.intern(row.GetEventId())
		record := badgeMessage{
			seq: row.GetSequence(), createdAt: row.GetCreatedAtUnixNanos(),
			room: b.ids.intern(row.GetRoomId()), thread: b.eventIDs.intern(row.GetThreadRootEventId()),
			actor: b.ids.intern(row.GetActorId()), author: b.ids.intern(row.GetAuthorId()),
			retracted: row.GetRetracted(), source: row.GetSource(),
		}
		b.messages.set(message, record)
		if !record.source {
			continue
		}
		sources := b.room(record.room)
		if record.thread == 0 {
			sources.roots = b.appendMessage(sources.roots, message, record.createdAt)
		} else {
			sources.replies[record.thread] = b.appendMessage(sources.replies[record.thread], message, record.createdAt)
		}
	}
	for _, row := range snapshot.GetTargets() {
		kind := badgeSourceKind(row.GetKind())
		message, known := b.eventIDs.lookup(row.GetMessageEventId())
		record, exists := b.message(message)
		if _, valid := kind.cause(); !valid || row.GetUserId() == "" || row.GetRoomId() == "" || !known || !exists {
			return nil, fmt.Errorf("notification badge snapshot has an invalid targeted source")
		}
		source := badgeTargetedSource{
			seq: row.GetSequence(), createdAt: row.GetCreatedAtUnixNanos(), message: message,
			reactor: b.ids.intern(row.GetReactorId()), emoji: b.ids.intern(row.GetEmoji()), kind: kind,
		}
		if kind == badgeSourceReaction {
			b.reactions[badgeReactionKey{message: message, reactor: source.reactor, emoji: source.emoji}] = struct{}{}
		}
		b.addTargeted(b.room(b.ids.intern(row.GetRoomId())), b.ids.intern(row.GetUserId()), record.thread, source)
	}
	for _, row := range snapshot.GetMemberships() {
		if row.GetUserId() == "" || row.GetRoomId() == "" {
			return nil, fmt.Errorf("notification badge snapshot has an invalid membership")
		}
		b.memberSince[badgeMembershipKey{user: b.ids.intern(row.GetUserId()), room: b.ids.intern(row.GetRoomId())}] = row.GetSequence()
	}
	for _, row := range snapshot.GetAccounts() {
		if row.GetUserId() == "" {
			return nil, fmt.Errorf("notification badge snapshot has an invalid account")
		}
		b.accountSince[b.ids.intern(row.GetUserId())] = row.GetSequence()
	}
	for _, row := range snapshot.GetUniversalRooms() {
		if row.GetRoomId() == "" {
			return nil, fmt.Errorf("notification badge snapshot has an invalid universal room")
		}
		b.universalSince[b.ids.intern(row.GetRoomId())] = row.GetSequence()
	}
	for _, row := range snapshot.GetFollows() {
		if row.GetUserId() == "" || row.GetRoomId() == "" || row.GetThreadRootEventId() == "" {
			return nil, fmt.Errorf("notification badge snapshot has an invalid follow")
		}
		key := badgeMembershipKey{user: b.ids.intern(row.GetUserId()), room: b.ids.intern(row.GetRoomId())}
		if b.follows[key] == nil {
			b.follows[key] = make(map[uint32]uint64)
		}
		b.follows[key][b.eventIDs.intern(row.GetThreadRootEventId())] = row.GetSequence()
	}
	return b, nil
}

// restoreThreadState rebuilds the explicit thread follow states and reply
// counts of a decision snapshot.
func (b *notificationBadgeSources) restoreThreadState(follows []*projectionv1.ThreadFollowSnapshot, threads []*projectionv1.NotificationThreadStateSnapshot) error {
	for _, row := range follows {
		state := compactFollowState(ThreadFollowState(row.GetState()))
		if row.GetUserId() == "" || row.GetRoomId() == "" || row.GetThreadRootEventId() == "" || state == compactThreadFollowNone {
			return fmt.Errorf("notification decision snapshot has invalid thread follow")
		}
		b.setFollowState(badgeFollowKey{
			user:           b.ids.intern(row.GetUserId()),
			badgeThreadKey: badgeThreadKey{room: b.ids.intern(row.GetRoomId()), thread: b.eventIDs.intern(row.GetThreadRootEventId())},
		}, state)
	}
	for _, row := range threads {
		if row.GetThreadRootEventId() == "" {
			return fmt.Errorf("notification decision snapshot has empty thread root event ID")
		}
		b.replyCounts[b.eventIDs.intern(row.GetThreadRootEventId())] = row.GetReplyCount()
	}
	return nil
}
