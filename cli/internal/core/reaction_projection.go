package core

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// ReactionProjection derives current reaction state from durable room
// aggregate events. It consumes the full room namespace so mutation snapshots
// can carry the per-room OCC position even when the latest room fact is not a
// reaction. v1 intentionally keeps the whole current reaction set in RAM;
// bounded/windowed variants can build on this once real access patterns are
// known.
type ReactionProjection struct {
	events.MemoryProjection
	// ids interns message, emoji, user, room, and source event IDs as handles.
	ids projectionIDTable
	// byMessage maps a canonical message handle to its active reactions. Each
	// emoji and user pair appears at most once.
	byMessage map[uint32][]reactionProjectionEntry
	roomSeq   map[string]uint64
	// messageRooms is indexed by message handle minus one and holds the room
	// handle of each posted message; zero means unknown.
	messageRooms []uint32
	// echoOriginal maps an echo message handle to its original message handle.
	echoOriginal map[uint32]uint32
	assetRoom    map[string]string
	replayGuard  projectionReplayGuard
}

type ReactionMutationSnapshot struct {
	Exists            bool
	UserReactionCount int
	Seq               uint64
	SourceEventID     string
}

// reactionProjectionEntry is one active reaction. IDs are ID-table handles.
type reactionProjectionEntry struct {
	addedAtNanos int64
	emoji        uint32
	user         uint32
	source       uint32
}

func NewReactionProjection() *ReactionProjection {
	return &ReactionProjection{
		ids:          newProjectionIDTable(),
		byMessage:    make(map[uint32][]reactionProjectionEntry),
		roomSeq:      make(map[string]uint64),
		echoOriginal: make(map[uint32]uint32),
		assetRoom:    make(map[string]string),
		replayGuard:  newProjectionReplayGuard(),
	}
}

func (p *ReactionProjection) Subjects() []string {
	return []string{evtstream.RoomSubjectFilter()}
}

func (p *ReactionProjection) Apply(event *evtv1.Event, seq uint64) error {
	if event == nil {
		return nil
	}

	p.Lock()
	defer p.Unlock()

	roomID := p.roomSeqIDLocked(event)
	if roomID == "" {
		return nil
	}
	p.noteRoomSeqLocked(roomID, seq)
	p.noteRoomOwnershipLocked(event, roomID)

	payload := event.GetEvent()
	switch payload.(type) {
	case *evtv1.Event_ReactionAdded, *evtv1.Event_ReactionRemoved:
	default:
		return nil
	}

	if p.replayGuard.seenOrMark(event, seq) {
		return nil
	}

	switch e := payload.(type) {
	case *evtv1.Event_ReactionAdded:
		p.applyAdded(e.ReactionAdded, event.GetActorId(), eventCreatedNanos(event), event.GetId())
	case *evtv1.Event_ReactionRemoved:
		p.applyRemoved(e.ReactionRemoved, event.GetActorId())
	}
	return nil
}

func (p *ReactionProjection) CompleteStartupReplay() {
	p.Lock()
	defer p.Unlock()
	p.replayGuard.completeReplay()
}

func (p *ReactionProjection) roomSeqIDLocked(event *evtv1.Event) string {
	if roomID := roomIDOfEvent(event); roomID != "" {
		return roomID
	}
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_AssetCreated:
		return assetCreatedRoomID(e.AssetCreated)
	case *evtv1.Event_AssetProcessingStarted:
		if roomID := p.messageRoomLocked(e.AssetProcessingStarted.GetMessageEventId()); roomID != "" {
			return roomID
		}
		return p.assetRoom[e.AssetProcessingStarted.GetAssetId()]
	case *evtv1.Event_AssetProcessingSucceeded:
		if roomID := p.messageRoomLocked(e.AssetProcessingSucceeded.GetMessageEventId()); roomID != "" {
			return roomID
		}
		return p.assetRoom[e.AssetProcessingSucceeded.GetAssetId()]
	case *evtv1.Event_AssetProcessingFailed:
		if roomID := p.messageRoomLocked(e.AssetProcessingFailed.GetMessageEventId()); roomID != "" {
			return roomID
		}
		return p.assetRoom[e.AssetProcessingFailed.GetAssetId()]
	case *evtv1.Event_AssetDeleted:
		return p.assetRoom[e.AssetDeleted.GetAssetId()]
	default:
		return ""
	}
}

func (p *ReactionProjection) noteRoomSeqLocked(roomID string, seq uint64) {
	if seq > p.roomSeq[roomID] {
		p.roomSeq[roomID] = seq
	}
}

func (p *ReactionProjection) noteRoomOwnershipLocked(event *evtv1.Event, roomID string) {
	if roomID == "" {
		return
	}
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		if event.GetId() != "" {
			message := p.ids.intern(event.GetId())
			p.setMessageRoomLocked(message, p.ids.intern(roomID))
			if originalID := e.MessagePosted.GetEchoOfEventId(); originalID != "" {
				p.echoOriginal[message] = p.ids.intern(originalID)
			}
		}
	case *evtv1.Event_AssetCreated:
		if assetID := e.AssetCreated.GetAsset().GetId(); assetID != "" {
			p.assetRoom[assetID] = roomID
		}
	case *evtv1.Event_AssetDeleted:
		delete(p.assetRoom, e.AssetDeleted.GetAssetId())
	}
}

func (p *ReactionProjection) messageRoomLocked(messageEventID string) string {
	message, ok := p.ids.lookup(messageEventID)
	if !ok || int(message) > len(p.messageRooms) {
		return ""
	}
	return p.ids.id(p.messageRooms[message-1])
}

func (p *ReactionProjection) setMessageRoomLocked(message, room uint32) {
	if missing := int(message) - len(p.messageRooms); missing > 0 {
		p.messageRooms = append(p.messageRooms, make([]uint32, missing)...)
	}
	p.messageRooms[message-1] = room
}

func (p *ReactionProjection) applyAdded(e *evtv1.ReactionAddedEvent, userID string, nanos int64, sourceEventID string) {
	if e == nil || userID == "" || e.GetMessageEventId() == "" || e.GetEmoji() == "" {
		return
	}
	message := p.canonicalMessageLocked(p.ids.intern(e.GetMessageEventId()))
	emoji := p.ids.intern(e.GetEmoji())
	user := p.ids.intern(userID)
	reactions := p.byMessage[message]
	if reactionIndex(reactions, emoji, user) >= 0 {
		return
	}
	p.byMessage[message] = append(reactions, reactionProjectionEntry{
		addedAtNanos: nanos, emoji: emoji, user: user, source: p.ids.intern(sourceEventID),
	})
}

func (p *ReactionProjection) applyRemoved(e *evtv1.ReactionRemovedEvent, userID string) {
	if e == nil || userID == "" || e.GetMessageEventId() == "" || e.GetEmoji() == "" {
		return
	}
	message, messageKnown := p.ids.lookup(e.GetMessageEventId())
	emoji, emojiKnown := p.ids.lookup(e.GetEmoji())
	user, userKnown := p.ids.lookup(userID)
	if !messageKnown || !emojiKnown || !userKnown {
		return
	}
	message = p.canonicalMessageLocked(message)
	reactions := p.byMessage[message]
	index := reactionIndex(reactions, emoji, user)
	if index < 0 {
		return
	}
	if len(reactions) == 1 {
		delete(p.byMessage, message)
		return
	}
	p.byMessage[message] = slices.Delete(reactions, index, index+1)
}

// reactionIndex returns the position of the emoji and user pair, or -1.
func reactionIndex(reactions []reactionProjectionEntry, emoji, user uint32) int {
	for i, reaction := range reactions {
		if reaction.emoji == emoji && reaction.user == user {
			return i
		}
	}
	return -1
}

func (p *ReactionProjection) canonicalMessageLocked(message uint32) uint32 {
	if original := p.echoOriginal[message]; original != 0 {
		return original
	}
	return message
}

// reactionsForMessageLocked returns the active reactions of a message ID,
// following an echo to its original message.
func (p *ReactionProjection) reactionsForMessageLocked(messageEventID string) []reactionProjectionEntry {
	message, ok := p.ids.lookup(messageEventID)
	if !ok {
		return nil
	}
	return p.byMessage[p.canonicalMessageLocked(message)]
}

func eventCreatedNanos(event *evtv1.Event) int64 {
	if ts := event.GetCreatedAt(); ts != nil {
		return ts.AsTime().UnixNano()
	}
	return time.Now().UnixNano()
}

func (p *ReactionProjection) HasReaction(messageEventID, emoji, userID string) bool {
	return p.ReactionMutationSnapshot("", messageEventID, emoji, userID).Exists
}

func (p *ReactionProjection) ReactionMutationSnapshot(roomID, messageEventID, emoji, userID string) ReactionMutationSnapshot {
	p.RLock()
	defer p.RUnlock()

	snapshot := ReactionMutationSnapshot{Seq: p.roomSeq[roomID]}
	user, ok := p.ids.lookup(userID)
	if !ok {
		return snapshot
	}
	emojiHandle, emojiKnown := p.ids.lookup(emoji)
	for _, reaction := range p.reactionsForMessageLocked(messageEventID) {
		if reaction.user != user {
			continue
		}
		snapshot.UserReactionCount++
		if emojiKnown && reaction.emoji == emojiHandle {
			snapshot.Exists = true
			snapshot.SourceEventID = p.ids.id(reaction.source)
		}
	}
	return snapshot
}

func (p *ReactionProjection) Reactions(messageEventID string) []ReactionSummary {
	p.RLock()
	defer p.RUnlock()
	return p.reactionSummariesLocked(p.reactionsForMessageLocked(messageEventID))
}

func (p *ReactionProjection) ReactionsBatch(messageEventIDs []string) map[string][]ReactionSummary {
	p.RLock()
	defer p.RUnlock()

	result := make(map[string][]ReactionSummary, len(messageEventIDs))
	for _, eventID := range messageEventIDs {
		if reactions := p.reactionsForMessageLocked(eventID); len(reactions) > 0 {
			result[eventID] = p.reactionSummariesLocked(reactions)
		}
	}
	return result
}

// Stats returns aggregate counts useful for import/rollout diagnostics.
func (p *ReactionProjection) Stats() (messages int, activeReactions int) {
	p.RLock()
	defer p.RUnlock()
	messages = len(p.byMessage)
	for _, reactions := range p.byMessage {
		activeReactions += len(reactions)
	}
	return messages, activeReactions
}

// reactionSummariesLocked groups reactions by emoji. Groups are ordered by
// their earliest reaction, then by emoji; user IDs are sorted.
func (p *ReactionProjection) reactionSummariesLocked(reactions []reactionProjectionEntry) []ReactionSummary {
	if len(reactions) == 0 {
		return nil
	}
	type group struct {
		summary       ReactionSummary
		earliestNanos int64
		emoji         uint32
		users         int
	}
	// Messages carry few distinct emojis, so linear scans beat a map. The
	// first pass sizes each user list exactly.
	groups := make([]group, 0, len(reactions))
	groupOf := func(emoji uint32) int {
		return slices.IndexFunc(groups, func(g group) bool { return g.emoji == emoji })
	}
	for _, reaction := range reactions {
		index := groupOf(reaction.emoji)
		if index < 0 {
			index = len(groups)
			groups = append(groups, group{
				summary:       ReactionSummary{Emoji: p.ids.id(reaction.emoji)},
				earliestNanos: reaction.addedAtNanos,
				emoji:         reaction.emoji,
			})
		}
		g := &groups[index]
		g.users++
		if g.earliestNanos == 0 || reaction.addedAtNanos < g.earliestNanos {
			g.earliestNanos = reaction.addedAtNanos
		}
	}
	for _, reaction := range reactions {
		g := &groups[groupOf(reaction.emoji)]
		if g.summary.UserIDs == nil {
			g.summary.UserIDs = make([]string, 0, g.users)
		}
		g.summary.UserIDs = append(g.summary.UserIDs, p.ids.id(reaction.user))
	}
	slices.SortFunc(groups, func(a, b group) int {
		if a.earliestNanos != b.earliestNanos {
			return cmp.Compare(a.earliestNanos, b.earliestNanos)
		}
		return strings.Compare(a.summary.Emoji, b.summary.Emoji)
	})
	result := make([]ReactionSummary, len(groups))
	for i, g := range groups {
		slices.Sort(g.summary.UserIDs)
		result[i] = g.summary
	}
	return result
}
