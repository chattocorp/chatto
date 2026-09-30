package core

import (
	"cmp"
	"slices"
	"strings"

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
	// messages interns message event IDs. The ServerContentView shares one
	// table with the room timeline and thread components; a standalone
	// projection owns a private table. It does not change after construction.
	messages       *eventIDTable
	sharedEventIDs bool
	reactionState
}

// reactionState is the snapshot-restorable state of a ReactionProjection.
// Restore builds a new value and replaces the complete state at once.
type reactionState struct {
	// ids interns emoji, user, and room IDs as handles. Reaction source event
	// IDs are unique per reaction, so entries keep them as strings; interning
	// them would only grow the append-only table.
	ids projectionIDTable
	// byMessage maps a canonical message handle to its active reactions,
	// sorted by emoji handle and then user handle. Each pair appears once.
	byMessage map[uint32][]reactionProjectionEntry
	roomSeq   map[string]uint64
	// messageRooms holds the ids room handle of each posted message handle.
	messageRooms handleSlice[uint32]
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

// reactionProjectionEntry is one active reaction. emoji and user are ID-table
// handles.
type reactionProjectionEntry struct {
	// addedAtNanos is the reaction's compact creation time (see
	// projectionTime).
	addedAtNanos int64
	source       string
	emoji        uint32
	user         uint32
}

// NewReactionProjection returns an empty projection with a private message ID
// table.
func NewReactionProjection() *ReactionProjection {
	return newReactionProjection(nil)
}

// newReactionProjection returns an empty projection that interns message IDs
// in messages. A nil table gives the projection a private table.
func newReactionProjection(messages *eventIDTable) *ReactionProjection {
	shared := messages != nil
	if !shared {
		messages = newEventIDTable()
	}
	return &ReactionProjection{messages: messages, sharedEventIDs: shared, reactionState: reactionState{
		ids:          newProjectionIDTable(),
		byMessage:    make(map[uint32][]reactionProjectionEntry),
		roomSeq:      make(map[string]uint64),
		echoOriginal: make(map[uint32]uint32),
		assetRoom:    make(map[string]string),
		replayGuard:  newProjectionReplayGuard(),
	}}
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
			message := p.messages.intern(event.GetId())
			p.messageRooms.set(message, p.ids.intern(roomID))
			if originalID := e.MessagePosted.GetEchoOfEventId(); originalID != "" {
				p.echoOriginal[message] = p.messages.intern(originalID)
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
	message, ok := p.messages.lookup(messageEventID)
	if !ok {
		return ""
	}
	room, _ := p.messageRooms.get(message)
	return p.ids.id(room)
}

func (p *ReactionProjection) applyAdded(e *evtv1.ReactionAddedEvent, userID string, nanos int64, sourceEventID string) {
	if e == nil || userID == "" || e.GetMessageEventId() == "" || e.GetEmoji() == "" {
		return
	}
	message := p.canonicalMessageLocked(p.messages.intern(e.GetMessageEventId()))
	emoji := p.ids.intern(e.GetEmoji())
	user := p.ids.intern(userID)
	reactions := p.byMessage[message]
	index, exists := reactionSearch(reactions, emoji, user)
	if exists {
		return
	}
	p.byMessage[message] = slices.Insert(reactions, index, reactionProjectionEntry{
		addedAtNanos: nanos, source: sourceEventID, emoji: emoji, user: user,
	})
}

func (p *ReactionProjection) applyRemoved(e *evtv1.ReactionRemovedEvent, userID string) {
	if e == nil || userID == "" || e.GetMessageEventId() == "" || e.GetEmoji() == "" {
		return
	}
	message, messageKnown := p.messages.lookup(e.GetMessageEventId())
	emoji, emojiKnown := p.ids.lookup(e.GetEmoji())
	user, userKnown := p.ids.lookup(userID)
	if !messageKnown || !emojiKnown || !userKnown {
		return
	}
	message = p.canonicalMessageLocked(message)
	reactions := p.byMessage[message]
	index, exists := reactionSearch(reactions, emoji, user)
	if !exists {
		return
	}
	if len(reactions) == 1 {
		delete(p.byMessage, message)
		return
	}
	reactions = slices.Delete(reactions, index, index+1)
	if cap(reactions) > 2*len(reactions)+8 {
		// Release the capacity left by a burst of removed reactions.
		reactions = slices.Clone(reactions)
	}
	p.byMessage[message] = reactions
}

// compareReactions orders reactions by emoji handle and then user handle.
func compareReactions(a, b reactionProjectionEntry) int {
	if byEmoji := cmp.Compare(a.emoji, b.emoji); byEmoji != 0 {
		return byEmoji
	}
	return cmp.Compare(a.user, b.user)
}

// reactionSearch finds the emoji and user pair in sorted reactions. It returns
// the pair's position, or its insertion position when the pair is absent.
func reactionSearch(reactions []reactionProjectionEntry, emoji, user uint32) (int, bool) {
	return slices.BinarySearchFunc(reactions, reactionProjectionEntry{emoji: emoji, user: user}, compareReactions)
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
	message, ok := p.messages.lookup(messageEventID)
	if !ok {
		return nil
	}
	return p.byMessage[p.canonicalMessageLocked(message)]
}

// RoomSequence returns the highest applied EVT sequence among the events that
// this projection attributes to the room. Every reaction in the room that this
// projection has applied has a sequence at or below it.
func (p *ReactionProjection) RoomSequence(roomID string) uint64 {
	p.RLock()
	defer p.RUnlock()
	return p.roomSeq[roomID]
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
			snapshot.SourceEventID = reaction.source
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
	}
	var groups []group
	// Reactions are sorted by emoji handle, so each emoji is one contiguous run.
	for start := 0; start < len(reactions); {
		end := start + 1
		for end < len(reactions) && reactions[end].emoji == reactions[start].emoji {
			end++
		}
		g := group{
			summary:       ReactionSummary{Emoji: p.ids.id(reactions[start].emoji), UserIDs: make([]string, 0, end-start)},
			earliestNanos: reactions[start].addedAtNanos,
		}
		for _, reaction := range reactions[start:end] {
			g.summary.UserIDs = append(g.summary.UserIDs, p.ids.id(reaction.user))
			if g.earliestNanos == 0 || reaction.addedAtNanos < g.earliestNanos {
				g.earliestNanos = reaction.addedAtNanos
			}
		}
		slices.Sort(g.summary.UserIDs)
		groups = append(groups, g)
		start = end
	}
	slices.SortFunc(groups, func(a, b group) int {
		if a.earliestNanos != b.earliestNanos {
			return cmp.Compare(a.earliestNanos, b.earliestNanos)
		}
		return strings.Compare(a.summary.Emoji, b.summary.Emoji)
	})
	result := make([]ReactionSummary, len(groups))
	for i, g := range groups {
		result[i] = g.summary
	}
	return result
}
