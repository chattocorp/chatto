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

// badgeSourceKind identifies the notification cause of one targeted Badge
// source. Root messages and followed-thread replies are not targeted; they are
// found through the room's root and reply lists.
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

// badgeCauseSignals holds one content-free signal per cause. The delivery-mode
// lookup only inspects the signal variant, so these values avoid allocating a
// signal for every evaluated source.
var badgeCauseSignals = map[badgeSourceKind]*notificationv1.NotificationSignal{
	badgeSourceDirectMention:    {Kind: &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{}}},
	badgeSourceRoleMention:      {Kind: &notificationv1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &notificationv1.RoleMentionReceived{}}},
	badgeSourceHereMention:      {Kind: &notificationv1.NotificationSignal_HereMentionReceived{HereMentionReceived: &notificationv1.HereMentionReceived{}}},
	badgeSourceAllMention:       {Kind: &notificationv1.NotificationSignal_AllMentionReceived{AllMentionReceived: &notificationv1.AllMentionReceived{}}},
	badgeSourceReply:            {Kind: &notificationv1.NotificationSignal_ReplyReceived{ReplyReceived: &notificationv1.ReplyReceived{}}},
	badgeSourceReaction:         {Kind: &notificationv1.NotificationSignal_ReactionReceived{ReactionReceived: &notificationv1.ReactionReceived{}}},
	badgeSourceFirstThreadReply: badgeFollowedThreadSignal,
}

var (
	badgeRoomMessageSignal    = &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoomMessageReceived{RoomMessageReceived: &notificationv1.RoomMessageReceived{}}}
	badgeDirectMessageSignal  = &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &notificationv1.DirectMessageReceived{}}}
	badgeFollowedThreadSignal = &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &notificationv1.FollowedThreadActivity{}}}
)

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
	// createdAt is the post's Unix time in nanoseconds, or zero when unknown.
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
	// source is false for echoes and historical imports, which are never Badge
	// sources. They stay indexed as reply parents and reaction targets.
	source bool
}

// badgeTargetedSource is one source addressed to one user.
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

// badgeRoomSources holds one room's Badge sources in stream order.
type badgeRoomSources struct {
	roots    []uint32
	replies  map[uint32][]uint32
	targeted map[uint32][]badgeTargetedSource
}

type badgeMembershipKey struct {
	user uint32
	room uint32
}

// notificationBadgeSources indexes every fact that can give a user Badge
// attention, so Badge attention is computed from current state when it is
// read instead of being stored per recipient. The NotificationDecisionProjection
// lock guards it.
type notificationBadgeSources struct {
	ids      projectionIDTable
	messages map[uint32]badgeMessage
	rooms    map[uint32]*badgeRoomSources
	// memberSince holds the sequence of each current explicit membership's
	// latest join. Sources at or before it predate the membership.
	memberSince map[badgeMembershipKey]uint64
	// accountSince and universalSince bound implicit universal-room
	// membership: it starts when both the account and universal access exist.
	accountSince   map[uint32]uint64
	universalSince map[uint32]uint64
	// follows maps a (user, room) pair to the threads that the user currently
	// follows there, with the sequence of the latest follow.
	follows map[badgeMembershipKey]map[uint32]uint64
}

func newNotificationBadgeSources() *notificationBadgeSources {
	return &notificationBadgeSources{
		ids:            newProjectionIDTable(),
		messages:       make(map[uint32]badgeMessage),
		rooms:          make(map[uint32]*badgeRoomSources),
		memberSince:    make(map[badgeMembershipKey]uint64),
		accountSince:   make(map[uint32]uint64),
		universalSince: make(map[uint32]uint64),
		follows:        make(map[badgeMembershipKey]map[uint32]uint64),
	}
}

func (b *notificationBadgeSources) room(room uint32) *badgeRoomSources {
	sources := b.rooms[room]
	if sources == nil {
		sources = &badgeRoomSources{replies: make(map[uint32][]uint32), targeted: make(map[uint32][]badgeTargetedSource)}
		b.rooms[room] = sources
	}
	return sources
}

func eventCreatedNanosOrZero(event *evtv1.Event) int64 {
	if created := event.GetCreatedAt(); created != nil {
		return created.AsTime().UnixNano()
	}
	return 0
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
			b.memberSince[badgeMembershipKey{user: b.ids.intern(event.GetActorId()), room: b.ids.intern(payload.UserJoinedRoom.GetRoomId())}] = seq
		}
	case *evtv1.Event_UserLeftRoom:
		b.endMembership(event.GetActorId(), payload.UserLeftRoom.GetRoomId())
	case *evtv1.Event_RoomMemberBanned:
		b.endMembership(payload.RoomMemberBanned.GetUserId(), payload.RoomMemberBanned.GetRoomId())
	case *evtv1.Event_RoomMemberRemoved:
		b.endMembership(payload.RoomMemberRemoved.GetUserId(), payload.RoomMemberRemoved.GetRoomId())
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
			delete(b.rooms, room)
			delete(b.universalSince, room)
		}
	case *evtv1.Event_UserAccountCreated:
		if userID := payload.UserAccountCreated.GetUserId(); userID != "" {
			b.accountSince[b.ids.intern(userID)] = seq
		}
	case *evtv1.Event_UserAccountDeleted:
		if user, ok := b.ids.lookup(payload.UserAccountDeleted.GetUserId()); ok {
			delete(b.accountSince, user)
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
		source:    posted.GetEchoOfEventId() == "" && !posted.GetHistoricalImport(),
	}
	b.messages[message] = record
	if !record.source {
		return
	}
	sources := b.room(record.room)
	if record.thread == 0 {
		sources.roots = append(sources.roots, message)
	} else {
		sources.replies[record.thread] = append(sources.replies[record.thread], message)
		if replyCount == 1 {
			if root, exists := b.messages[record.thread]; exists && root.author != 0 && root.author != record.actor {
				sources.addTargeted(root.author, badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: badgeSourceFirstThreadReply})
			}
		}
	}
	for _, mention := range posted.GetMentions() {
		kind := badgeSourceKindForMention(mention)
		if kind == 0 || mention.GetUserId() == "" || mention.GetUserId() == event.GetActorId() {
			continue
		}
		sources.addTargeted(b.ids.intern(mention.GetUserId()), badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: kind})
	}
	if parentID := posted.GetInReplyTo(); parentID != "" {
		if parent, ok := b.ids.lookup(parentID); ok {
			if parentRecord, exists := b.messages[parent]; exists && parentRecord.author != 0 && parentRecord.author != record.actor {
				sources.addTargeted(parentRecord.author, badgeTargetedSource{seq: seq, createdAt: record.createdAt, message: message, kind: badgeSourceReply})
			}
		}
	}
}

func (s *badgeRoomSources) addTargeted(user uint32, source badgeTargetedSource) {
	s.targeted[user] = append(s.targeted[user], source)
}

func (b *notificationBadgeSources) applyReactionAdded(event *evtv1.Event, reaction *evtv1.ReactionAddedEvent, seq uint64) {
	target, ok := b.ids.lookup(reaction.GetMessageEventId())
	if !ok || reaction.GetEmoji() == "" || event.GetActorId() == "" {
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
	emoji := b.ids.intern(reaction.GetEmoji())
	sources := b.room(record.room)
	// The reaction projection keeps the first of repeated adds; so does the
	// index, so a later removal drops exactly the reaction that is current.
	for _, existing := range sources.targeted[record.author] {
		if existing.kind == badgeSourceReaction && existing.message == target && existing.reactor == reactor && existing.emoji == emoji {
			return
		}
	}
	sources.addTargeted(record.author, badgeTargetedSource{
		seq: seq, createdAt: eventCreatedNanosOrZero(event), message: target, reactor: reactor, emoji: emoji, kind: badgeSourceReaction,
	})
}

func (b *notificationBadgeSources) applyReactionRemoved(reactorID string, reaction *evtv1.ReactionRemovedEvent) {
	target, targetKnown := b.ids.lookup(reaction.GetMessageEventId())
	reactor, reactorKnown := b.ids.lookup(reactorID)
	emoji, emojiKnown := b.ids.lookup(reaction.GetEmoji())
	if !targetKnown || !reactorKnown || !emojiKnown {
		return
	}
	record, exists := b.messages[target]
	if !exists {
		return
	}
	sources := b.rooms[record.room]
	if sources == nil {
		return
	}
	list := sources.targeted[record.author]
	for i, existing := range list {
		if existing.kind == badgeSourceReaction && existing.message == target && existing.reactor == reactor && existing.emoji == emoji {
			sources.targeted[record.author] = append(list[:i:i], list[i+1:]...)
			return
		}
	}
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
	// visibilityBoundary is the latest recorded visibility loss, or zero.
	visibilityBoundary uint64
	// readBoundary returns the user's read boundary for a scope of the room.
	readBoundary func(threadRootEventID string) (notificationReadBoundary, bool)
	// ignoreVisibility evaluates as if the user could see the room. Visibility
	// repair uses it to find sources that could give attention again after
	// access returns.
	ignoreVisibility bool
}

// hasBadgeAttention reports whether a current source gives the user Badge
// attention. A source counts only when all of these hold:
//   - its cause resolves to Badge under the user's current notification policy;
//   - the user can currently see it;
//   - it follows the user's membership start and latest visibility loss;
//   - the read boundary of its scope does not cover it;
//   - it is not the user's own, not retracted, and younger than notificationTTL.
func (s *notificationDecisionSnapshot) hasBadgeAttention(q badgeQuery) bool {
	b := s.badges
	if _, active := s.activeUsers[q.userID]; !active {
		return false
	}
	room, roomKnown := b.ids.lookup(q.roomID)
	if !roomKnown {
		return false
	}
	kind, exists := s.roomKind(q.roomID)
	if !exists {
		return false
	}
	broad := q.ignoreVisibility || s.notificationVisibilityExists(q.userID, q.roomID)
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
	lower := max(q.visibilityBoundary, s.badgeMembershipStart(user, room, q.userID, q.roomID))
	expired := q.now.Add(-notificationTTL).UnixNano()
	isExpired := func(createdAt int64) bool { return createdAt != 0 && createdAt <= expired }
	excluded := func(seq uint64) bool { return q.before != 0 && seq >= q.before }
	badge := func(signal *notificationv1.NotificationSignal) bool {
		return s.effectiveNotificationMode(q.userID, q.roomID, signal) == evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE
	}
	readTarget := func(thread uint32) (notificationReadBoundary, bool) {
		if q.readBoundary == nil {
			return notificationReadBoundary{}, false
		}
		return q.readBoundary(b.ids.id(thread))
	}
	sources := b.rooms[room]
	if sources == nil {
		return false
	}

	// Root activity: every root message by another member.
	rootSignal := badgeRoomMessageSignal
	if kind == KindDM {
		rootSignal = badgeDirectMessageSignal
	}
	if includes(0) && broad && badge(rootSignal) {
		floor := lower
		if boundary, ok := readTarget(0); ok {
			floor = max(floor, boundary.targetSequence)
		}
		for i := len(sources.roots) - 1; i >= 0; i-- {
			message := b.messages[sources.roots[i]]
			if message.seq <= floor || isExpired(message.createdAt) {
				break
			}
			if excluded(message.seq) || message.retracted || message.actor == user {
				continue
			}
			return true
		}
	}

	// Sources addressed to this user: mentions, replies, reactions, and the
	// first reply in a thread that the user started.
	targeted := sources.targeted[user]
	for i := len(targeted) - 1; i >= 0 && user != 0; i-- {
		source := targeted[i]
		if source.seq <= lower || isExpired(source.createdAt) {
			break
		}
		if excluded(source.seq) {
			continue
		}
		message, exists := b.messages[source.message]
		if !exists || message.retracted || !includes(message.thread) {
			continue
		}
		if source.kind == badgeSourceDirectMention {
			if !broad && !interaction {
				continue
			}
		} else if !broad {
			continue
		}
		if source.kind == badgeSourceFirstThreadReply && s.threadFollowState(q.userID, q.roomID, b.ids.id(message.thread)) == ThreadFollowStateUnfollowed {
			continue
		}
		if !badge(badgeCauseSignals[source.kind]) {
			continue
		}
		if boundary, ok := readTarget(message.thread); ok {
			if source.kind == badgeSourceReaction {
				if message.seq <= boundary.targetSequence && source.seq <= boundary.observedSequence {
					continue
				}
			} else if source.seq <= boundary.targetSequence {
				continue
			}
		}
		return true
	}

	// Activity in threads that the user follows, after the follow began.
	if broad && user != 0 && badge(badgeFollowedThreadSignal) {
		for thread, since := range b.follows[badgeMembershipKey{user: user, room: room}] {
			if !includes(thread) {
				continue
			}
			floor := max(lower, since)
			if boundary, ok := readTarget(thread); ok {
				floor = max(floor, boundary.targetSequence)
			}
			replies := sources.replies[thread]
			for i := len(replies) - 1; i >= 0; i-- {
				message := b.messages[replies[i]]
				if message.seq <= floor || isExpired(message.createdAt) {
					break
				}
				if excluded(message.seq) || message.retracted || message.actor == user {
					continue
				}
				return true
			}
		}
	}
	return false
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

// snapshot encodes the index. Messages and targeted sources keep stream order
// so restore rebuilds the same ordered lists.
func (b *notificationBadgeSources) snapshot() *projectionv1.NotificationBadgeSourcesSnapshot {
	snapshot := &projectionv1.NotificationBadgeSourcesSnapshot{}
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
			for _, source := range sources.targeted[user] {
				snapshot.Targets = append(snapshot.Targets, &projectionv1.NotificationBadgeTargetSnapshot{
					UserId: b.ids.id(user), RoomId: b.ids.id(room), MessageEventId: b.ids.id(source.message),
					Kind: uint32(source.kind), Sequence: source.seq, CreatedAtUnixNanos: source.createdAt,
					ReactorId: b.ids.id(source.reactor), Emoji: b.ids.id(source.emoji),
				})
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
			sources.roots = append(sources.roots, message)
		} else {
			sources.replies[record.thread] = append(sources.replies[record.thread], message)
		}
	}
	for _, row := range snapshot.GetTargets() {
		kind := badgeSourceKind(row.GetKind())
		if row.GetUserId() == "" || row.GetRoomId() == "" || row.GetMessageEventId() == "" || badgeCauseSignals[kind] == nil {
			return nil, fmt.Errorf("notification badge snapshot has an invalid targeted source")
		}
		b.room(b.ids.intern(row.GetRoomId())).addTargeted(b.ids.intern(row.GetUserId()), badgeTargetedSource{
			seq: row.GetSequence(), createdAt: row.GetCreatedAtUnixNanos(), message: b.ids.intern(row.GetMessageEventId()),
			reactor: b.ids.intern(row.GetReactorId()), emoji: b.ids.intern(row.GetEmoji()), kind: kind,
		})
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

// badgeAudience returns the users whose Badge attention a message can affect,
// with the message's scope: room members for a root message; for a thread
// reply, the thread's followers and root author. Users addressed by a targeted
// source of the message, including reaction recipients, are included too.
// Callers use it to send invalidations when a message stops being a source.
func (s *notificationDecisionSnapshot) badgeAudience(messageEventID string) (roomID, threadRootEventID string, userIDs []string) {
	b := s.badges
	message, ok := b.ids.lookup(messageEventID)
	if !ok {
		return "", "", nil
	}
	record, exists := b.messages[message]
	if !exists {
		return "", "", nil
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
		for user, targeted := range sources.targeted {
			for _, source := range targeted {
				if source.message == message {
					users[b.ids.id(user)] = struct{}{}
					break
				}
			}
		}
	}
	return roomID, threadRootEventID, sortedMapKeys(users)
}

// badgeCandidatePairs lists the (user, room) pairs that visibility repair must
// check. An empty user or room ID selects every user or every room that has
// Badge sources.
func (s *notificationDecisionSnapshot) badgeCandidatePairs(userID, roomID string) []badgeUserRoom {
	b := s.badges
	var rooms []string
	if roomID != "" {
		if room, ok := b.ids.lookup(roomID); ok && b.rooms[room] != nil {
			rooms = []string{roomID}
		}
	} else {
		for room := range b.rooms {
			rooms = append(rooms, b.ids.id(room))
		}
	}
	var pairs []badgeUserRoom
	for _, candidateRoom := range rooms {
		users := make(map[string]struct{})
		if userID != "" {
			users[userID] = struct{}{}
		} else {
			for _, member := range s.rooms.Membership.Members(candidateRoom) {
				users[member] = struct{}{}
			}
			if room, exists := s.rooms.Catalog.Get(candidateRoom); exists && room.GetUniversal() {
				for active := range s.activeUsers {
					users[active] = struct{}{}
				}
			}
		}
		for _, user := range sortedMapKeys(users) {
			pairs = append(pairs, badgeUserRoom{userID: user, roomID: candidateRoom})
		}
	}
	return pairs
}

// badgeRoomsForUser lists the rooms where the user is an explicit member, plus
// every universal channel room.
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

// badgeHiddenCandidates lists the current room members, explicit or
// universal, who cannot see the room's messages now: they lack both broad and
// interaction-scoped read access. The actor is excluded, because their own
// source never gives them attention.
func (s *notificationDecisionSnapshot) badgeHiddenCandidates(roomID, actorID string) []string {
	var hidden []string
	for _, userID := range s.roomMemberIDs(roomID) {
		if userID == actorID {
			continue
		}
		if _, active := s.activeUsers[userID]; !active {
			continue
		}
		if s.notificationVisibilityExists(userID, roomID) || s.notificationInteractionVisibilityExists(userID, roomID) {
			continue
		}
		hidden = append(hidden, userID)
	}
	return hidden
}
