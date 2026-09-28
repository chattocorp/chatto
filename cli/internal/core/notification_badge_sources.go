package core

import (
	"cmp"
	"fmt"
	"slices"
	"time"

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

// badgeMessage is one indexed message post. IDs are ID-table handles.
type badgeMessage struct {
	seq uint64
	// createdAt is the post's Unix time in nanoseconds.
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
	// message is the source message, or the reaction target for a reaction.
	message uint32
	// reactor and emoji identify a reaction so its removal drops this source.
	reactor uint32
	emoji   uint32
	kind    badgeSourceKind
}

// badgeRoomSources holds one room's Badge sources. Every list is in stream
// order and holds only sources younger than notificationTTL at the time of the
// latest append.
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

// notificationBadgeSources indexes every fact that can give a user Badge
// attention, so Badge attention is computed from current state when it is
// read instead of being stored per recipient. The NotificationDecisionProjection
// lock guards it.
type notificationBadgeSources struct {
	ids      projectionIDTable
	messages map[uint32]badgeMessage
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
	// latestCreatedAt is the creation time of the newest indexed source. The
	// sweep and the snapshot drop sources that are expired relative to it.
	latestCreatedAt int64
}

func newNotificationBadgeSources() *notificationBadgeSources {
	return &notificationBadgeSources{
		ids:            newProjectionIDTable(),
		messages:       make(map[uint32]badgeMessage),
		rooms:          make(map[uint32]*badgeRoomSources),
		reactions:      make(map[badgeReactionKey]struct{}),
		memberSince:    make(map[badgeMembershipKey]uint64),
		accountSince:   make(map[uint32]uint64),
		universalSince: make(map[uint32]uint64),
		follows:        make(map[badgeMembershipKey]map[uint32]uint64),
	}
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
		for drop < len(replies) && b.messages[replies[drop]].createdAt <= cutoff {
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

func eventCreatedNanosOrZero(event *evtv1.Event) int64 {
	if created := event.GetCreatedAt(); created != nil {
		return created.AsTime().UnixNano()
	}
	return 0
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
	for drop < len(list) && b.messages[list[drop]].createdAt <= cutoff {
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

// apply indexes one decision-projection fact. replyCount is the thread's reply
// count after this fact, so the first reply can address the root's author.
func (b *notificationBadgeSources) apply(event *evtv1.Event, seq uint64, replyCount uint64) {
	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		b.applyMessagePosted(event, payload.MessagePosted, seq, replyCount)
	case *evtv1.Event_MessageRetracted:
		if message, ok := b.ids.lookup(payload.MessageRetracted.GetEventId()); ok {
			if record, exists := b.messages[message]; exists {
				record.retracted = true
				b.messages[message] = record
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
		key := badgeMembershipKey{user: b.ids.intern(follow.GetUserId()), room: b.ids.intern(follow.GetRoomId())}
		threads := b.follows[key]
		if threads == nil {
			threads = make(map[uint32]uint64)
			b.follows[key] = threads
		}
		thread := b.ids.intern(follow.GetThreadRootEventId())
		if _, following := threads[thread]; !following {
			threads[thread] = seq
		}
	case *evtv1.Event_ThreadUnfollowed:
		unfollow := payload.ThreadUnfollowed
		user, userKnown := b.ids.lookup(unfollow.GetUserId())
		room, roomKnown := b.ids.lookup(unfollow.GetRoomId())
		thread, threadKnown := b.ids.lookup(unfollow.GetThreadRootEventId())
		if !userKnown || !roomKnown || !threadKnown {
			return
		}
		key := badgeMembershipKey{user: user, room: room}
		delete(b.follows[key], thread)
		if len(b.follows[key]) == 0 {
			delete(b.follows, key)
		}
	}
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
	for message, record := range b.messages {
		if record.room == room {
			delete(b.messages, message)
		}
	}
	for key := range b.reactions {
		if _, exists := b.messages[key.message]; !exists {
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
	message := b.ids.intern(event.GetId())
	if _, exists := b.messages[message]; exists {
		return
	}
	record := badgeMessage{
		seq:       seq,
		createdAt: eventCreatedNanosOrZero(event),
		room:      b.ids.intern(posted.GetRoomId()),
		thread:    b.ids.intern(posted.GetInThread()),
		actor:     b.ids.intern(event.GetActorId()),
		author:    b.ids.intern(messageAuthorID(event)),
	}
	record.source = record.createdAt != 0 && posted.GetEchoOfEventId() == "" && !posted.GetHistoricalImport()
	b.messages[message] = record
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
			if root, exists := b.messages[record.thread]; exists && root.author != 0 && root.author != record.actor {
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
		if parent, ok := b.ids.lookup(parentID); ok {
			if parentRecord, exists := b.messages[parent]; exists && parentRecord.author != 0 && parentRecord.author != record.actor {
				b.addTargeted(sources, parentRecord.author, record.thread, badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: badgeSourceReply})
			}
		}
	}
}

func (b *notificationBadgeSources) applyReactionAdded(event *evtv1.Event, reaction *evtv1.ReactionAddedEvent, seq uint64) {
	target, ok := b.ids.lookup(reaction.GetMessageEventId())
	createdAt := eventCreatedNanosOrZero(event)
	if !ok || reaction.GetEmoji() == "" || event.GetActorId() == "" || createdAt == 0 {
		return
	}
	record, exists := b.messages[target]
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
	target, targetKnown := b.ids.lookup(reaction.GetMessageEventId())
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
	record := b.messages[target]
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
		thread, ok := b.ids.lookup(q.threadRootEventID)
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
		return q.readBoundary(b.ids.id(thread))
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
			message := b.messages[list[i]]
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
			message, exists := b.messages[source.message]
			if !exists || (message.retracted && source.message != q.unretracted) {
				continue
			}
			if !broad && source.kind != badgeSourceDirectMention {
				continue
			}
			if source.kind == badgeSourceFirstThreadReply && s.threadFollowState(q.userID, q.roomID, b.ids.id(thread)) == ThreadFollowStateUnfollowed {
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
			if newest := b.messages[replies[len(replies)-1]]; newest.seq <= max(lower, since) || newest.createdAt <= expired {
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
// users addressed by a targeted source of the message, including the author
// for reactions on it. Only users who can currently see the room are returned,
// so the result never names a room to a user outside it.
func (s *notificationDecisionSnapshot) badgeAudience(messageEventID string) (roomID, threadRootEventID string, message uint32, userIDs []string) {
	b := s.badges
	message, ok := b.ids.lookup(messageEventID)
	if !ok {
		return "", "", 0, nil
	}
	record, exists := b.messages[message]
	if !exists {
		return "", "", 0, nil
	}
	roomID, threadRootEventID = b.ids.id(record.room), b.ids.id(record.thread)
	users := make(map[string]struct{})
	if record.thread == 0 {
		for _, userID := range s.roomMemberIDs(roomID) {
			users[userID] = struct{}{}
		}
	} else {
		for _, userID := range s.threadFollowerIDs(roomID, threadRootEventID) {
			users[userID] = struct{}{}
		}
		if root, exists := b.messages[record.thread]; exists && root.author != 0 {
			users[b.ids.id(root.author)] = struct{}{}
		}
	}
	if sources := b.rooms[record.room]; sources != nil {
		for user, scopes := range sources.targeted {
			if slices.ContainsFunc(scopes[record.thread], func(source badgeTargetedSource) bool { return source.message == message }) {
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

// estimatedBytes approximates the retained size of the index.
func (b *notificationBadgeSources) estimatedBytes() int64 {
	bytes := b.ids.estimatedBytes()
	bytes += int64(len(b.messages)) * (projectionCompactMapEntryOverhead + 44)
	bytes += int64(len(b.reactions)) * (projectionCompactMapEntryOverhead + 12)
	bytes += int64(len(b.memberSince)+len(b.accountSince)+len(b.universalSince)) * (projectionCompactMapEntryOverhead + 16)
	for _, sources := range b.rooms {
		bytes += int64(len(sources.roots)) * 4
		for _, replies := range sources.replies {
			bytes += projectionCompactMapEntryOverhead + projectionSliceEntryOverhead + int64(len(replies))*4
		}
		for _, scopes := range sources.targeted {
			for _, targeted := range scopes {
				bytes += projectionCompactMapEntryOverhead + projectionSliceEntryOverhead + int64(len(targeted))*40
			}
		}
	}
	for _, threads := range b.follows {
		bytes += projectionCompactMapEntryOverhead + int64(len(threads))*(projectionCompactMapEntryOverhead+12)
	}
	return bytes
}

// snapshot encodes the index. Messages are in stream order and targeted
// sources are in stream order within each user, room, and scope, so restore
// rebuilds the same ordered lists.
func (b *notificationBadgeSources) snapshot() *projectionv1.NotificationBadgeSourcesSnapshot {
	snapshot := &projectionv1.NotificationBadgeSourcesSnapshot{LatestCreatedAtUnixNanos: b.latestCreatedAt}
	cutoff := expiredBefore(b.latestCreatedAt)
	messages := make([]uint32, 0, len(b.messages))
	for message := range b.messages {
		messages = append(messages, message)
	}
	slices.SortFunc(messages, func(a, c uint32) int { return cmp.Compare(b.messages[a].seq, b.messages[c].seq) })
	for _, message := range messages {
		record := b.messages[message]
		snapshot.Messages = append(snapshot.Messages, &projectionv1.NotificationBadgeMessageSnapshot{
			EventId: b.ids.id(message), RoomId: b.ids.id(record.room), ThreadRootEventId: b.ids.id(record.thread),
			ActorId: b.ids.id(record.actor), AuthorId: b.ids.id(record.author), Sequence: record.seq,
			CreatedAtUnixNanos: record.createdAt, Retracted: record.retracted, Source: record.source,
		})
	}
	for _, room := range sortedHandleKeys(&b.ids, b.rooms) {
		sources := b.rooms[room]
		for _, user := range sortedHandleKeys(&b.ids, sources.targeted) {
			for _, thread := range sortedHandleKeys(&b.ids, sources.targeted[user]) {
				for _, source := range sources.targeted[user][thread] {
					if source.createdAt <= cutoff {
						continue
					}
					snapshot.Targets = append(snapshot.Targets, &projectionv1.NotificationBadgeTargetSnapshot{
						UserId: b.ids.id(user), RoomId: b.ids.id(room), MessageEventId: b.ids.id(source.message),
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
		for _, thread := range sortedHandleKeys(&b.ids, b.follows[key]) {
			snapshot.Follows = append(snapshot.Follows, &projectionv1.NotificationBadgeFollowSnapshot{
				UserId: b.ids.id(key.user), RoomId: b.ids.id(key.room), ThreadRootEventId: b.ids.id(thread), Sequence: b.follows[key][thread],
			})
		}
	}
	return snapshot
}

func (b *notificationBadgeSources) compareMembershipKeys(a, c badgeMembershipKey) int {
	if byUser := cmp.Compare(b.ids.id(a.user), b.ids.id(c.user)); byUser != 0 {
		return byUser
	}
	return cmp.Compare(b.ids.id(a.room), b.ids.id(c.room))
}

// restoreNotificationBadgeSources rebuilds the index from a snapshot.
func restoreNotificationBadgeSources(snapshot *projectionv1.NotificationBadgeSourcesSnapshot) (*notificationBadgeSources, error) {
	b := newNotificationBadgeSources()
	b.latestCreatedAt = snapshot.GetLatestCreatedAtUnixNanos()
	var previous uint64
	for _, row := range snapshot.GetMessages() {
		if row.GetEventId() == "" || row.GetRoomId() == "" || row.GetSequence() == 0 || row.GetSequence() <= previous {
			return nil, fmt.Errorf("notification badge snapshot has an invalid message")
		}
		previous = row.GetSequence()
		message := b.ids.intern(row.GetEventId())
		record := badgeMessage{
			seq: row.GetSequence(), createdAt: row.GetCreatedAtUnixNanos(),
			room: b.ids.intern(row.GetRoomId()), thread: b.ids.intern(row.GetThreadRootEventId()),
			actor: b.ids.intern(row.GetActorId()), author: b.ids.intern(row.GetAuthorId()),
			retracted: row.GetRetracted(), source: row.GetSource(),
		}
		b.messages[message] = record
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
		message, known := b.ids.lookup(row.GetMessageEventId())
		record, exists := b.messages[message]
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
		b.follows[key][b.ids.intern(row.GetThreadRootEventId())] = row.GetSequence()
	}
	return b, nil
}
