package core

import (
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MessagesProjection holds per-message state derived from
// MessagePostedEvent / MessageEditedEvent / MessageRetractedEvent off
// the EVT stream (issue #597 phase 2). It coexists with the room and
// membership projections on the same evt.room.> subject family — each
// subscribes to a narrow event-type filter and ignores variants it
// doesn't recognise.
//
// Two indices are kept so both "what is this message?" and "what's in
// this room's timeline?" are O(1) hash lookups. The per-room list is
// append-only in stream order, so pagination by stream sequence is a
// straight slice. Both indices stay in sync — entries never appear in
// the room timeline without a corresponding byEventID entry.
//
// Scope explicitly NOT covered here:
//   - Hard deletion / GDPR. Retracted messages keep their body in the
//     projection; tombstone is a flag, not a body wipe. True content
//     erasure happens by deleting the author's encryption key (the
//     UserKeyShreddedEvent in #597's follow-up scope).
//   - Reactions. Live-only; KV-backed; separate concern.
//   - Thread reply indexing. ThreadsProjection consumes the same
//     event filter and maintains its own index.
type MessagesProjection struct {
	events.MemoryProjection
	// byEventID: event_id → entry (full state for any message we've seen,
	// including tombstoned ones).
	byEventID map[string]*messageEntry
	// byRoom: room_id → []event_id in stream-arrival order. Append-only;
	// tombstoning doesn't remove the entry.
	byRoom map[string][]string
}

// messageEntry is the in-memory shape held per message. Not exposed
// directly — callers go through Get / RoomTimeline which clone into
// the public Message shape for type symmetry with the rest of the
// codebase.
type messageEntry struct {
	eventID                   string
	roomID                    string
	authorID                  string
	body                      *corev1.MessageBody // current body (overwritten on edit)
	inReplyTo                 string
	inThread                  string
	mentionedUserIDs          []string
	echoOfEventID             string
	echoFromThreadRootEventID string
	postedAt                  time.Time
	editedAt                  *time.Time // nil until first MessageEditedEvent
	streamSeq                 uint64
	tombstoned                bool
	retractReason             string
}

// Message is the public view of a single message in the projection.
// Fresh value per call; callers may mutate freely.
type Message struct {
	EventID                   string
	RoomID                    string
	AuthorID                  string
	Body                      *corev1.MessageBody
	InReplyTo                 string
	InThread                  string
	MentionedUserIDs          []string
	EchoOfEventID             string
	EchoFromThreadRootEventID string
	PostedAt                  time.Time
	EditedAt                  *time.Time
	StreamSeq                 uint64
	Tombstoned                bool
	RetractReason             string
}

// NewMessagesProjection returns an empty projection. Wire into a
// Projector to populate from the EVT stream.
func NewMessagesProjection() *MessagesProjection {
	return &MessagesProjection{
		byEventID: make(map[string]*messageEntry),
		byRoom:    make(map[string][]string),
	}
}

// Subjects implements events.Projection. Narrow filters — only the
// event-types this projection cares about, so the per-projection
// consumer's delivery volume scales with message activity rather than
// total room activity.
func (p *MessagesProjection) Subjects() []string {
	return []string{
		events.RoomEventTypeFilter(events.EventMessagePosted),
		events.RoomEventTypeFilter(events.EventMessageEdited),
		events.RoomEventTypeFilter(events.EventMessageRetracted),
	}
}

// Apply implements events.Projection. Called from a single goroutine
// in stream order by the Projector; write-side locking only guards
// concurrent readers.
//
// Out-of-order arrivals (an edit or retract for an event_id we haven't
// seen yet) are silently dropped. The Projector replays the stream
// from sequence 0, so under normal operation we'll see the original
// MessagePosted before any of its mutations. The drop-silently
// behaviour is a defensive measure for the case where someone reuses
// this projection against a partial stream slice — it should never
// trip in production.
func (p *MessagesProjection) Apply(event *corev1.Event, seq uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		p.applyPostedLocked(event, e.MessagePosted, seq)
	case *corev1.Event_MessageEdited:
		p.applyEditedLocked(event, e.MessageEdited)
	case *corev1.Event_MessageRetracted:
		p.applyRetractedLocked(e.MessageRetracted)
	default:
		// Other event types may share the evt.room.> namespace; ignore
		// silently per the projection forward-compat rule.
	}
	return nil
}

func (p *MessagesProjection) applyPostedLocked(envelope *corev1.Event, posted *corev1.MessagePostedEvent, seq uint64) {
	eventID := posted.GetEventId()
	roomID := posted.GetRoomId()
	if eventID == "" || roomID == "" {
		return
	}
	// Idempotency: a re-applied MessagePosted for the same event_id is a
	// no-op. This matches the Projection.Apply idempotency contract
	// (Apply(e,n) twice == Apply(e,n) once).
	if _, exists := p.byEventID[eventID]; exists {
		return
	}
	entry := &messageEntry{
		eventID:                   eventID,
		roomID:                    roomID,
		authorID:                  envelope.GetActorId(),
		body:                      cloneMessageBody(posted.GetBody()),
		inReplyTo:                 posted.GetInReplyTo(),
		inThread:                  posted.GetInThread(),
		mentionedUserIDs:          append([]string(nil), posted.GetMentionedUserIds()...),
		echoOfEventID:             posted.GetEchoOfEventId(),
		echoFromThreadRootEventID: posted.GetEchoFromThreadRootEventId(),
		postedAt:                  envelope.GetCreatedAt().AsTime(),
		streamSeq:                 seq,
	}
	p.byEventID[eventID] = entry
	p.byRoom[roomID] = append(p.byRoom[roomID], eventID)
}

func (p *MessagesProjection) applyEditedLocked(envelope *corev1.Event, edited *corev1.MessageEditedEvent) {
	eventID := edited.GetEventId()
	if eventID == "" {
		return
	}
	entry, ok := p.byEventID[eventID]
	if !ok {
		// Out-of-order: edit arrived before the original post. Drop.
		return
	}
	entry.body = cloneMessageBody(edited.GetBody())
	t := envelope.GetCreatedAt().AsTime()
	entry.editedAt = &t
}

func (p *MessagesProjection) applyRetractedLocked(retracted *corev1.MessageRetractedEvent) {
	eventID := retracted.GetEventId()
	if eventID == "" {
		return
	}
	entry, ok := p.byEventID[eventID]
	if !ok {
		// Out-of-order: retract arrived before the original post. Drop.
		return
	}
	entry.tombstoned = true
	entry.retractReason = retracted.GetReason()
	// Body intentionally preserved — see type doc.
}

// Get returns the message by event ID, or (nil, false) if no such
// message has been projected. Tombstoned entries are returned (with
// Tombstoned=true); resolvers decide presentation.
func (p *MessagesProjection) Get(eventID string) (*Message, bool) {
	p.RLock()
	defer p.RUnlock()
	entry, ok := p.byEventID[eventID]
	if !ok {
		return nil, false
	}
	return entryToMessage(entry), true
}

// RoomMessageCount returns the total number of messages in the room,
// including tombstoned and thread-reply messages. Used by the future
// resolver's small-room fast-path equivalent.
func (p *MessagesProjection) RoomMessageCount(roomID string) int {
	p.RLock()
	defer p.RUnlock()
	return len(p.byRoom[roomID])
}

// RoomTimeline returns up to `limit` root messages from a room in
// newest-first order, optionally bounded by a stream-sequence cursor
// (exclusive upper bound). Thread replies are excluded — they're
// served via ThreadsProjection. Tombstoned entries ARE included so
// resolvers can render placeholders.
//
// Returns a fresh slice; callers may mutate freely. Order is
// newest-first to match the existing GetRoomEvents API shape.
//
// Echoes (root messages that mirror a thread reply) pass through as
// regular root entries — they have inThread == "" so they satisfy the
// root filter naturally.
func (p *MessagesProjection) RoomTimeline(roomID string, limit int, beforeStreamSeq uint64) []*Message {
	if limit <= 0 {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	ids := p.byRoom[roomID]
	if len(ids) == 0 {
		return nil
	}
	// Walk newest-first. byRoom is append-only in stream order, so
	// the last entry is the most recent.
	out := make([]*Message, 0, limit)
	for i := len(ids) - 1; i >= 0 && len(out) < limit; i-- {
		entry, ok := p.byEventID[ids[i]]
		if !ok {
			// Should never happen — the two indices are kept in sync.
			// Defensive skip rather than panic.
			continue
		}
		if entry.inThread != "" {
			continue // thread reply — excluded from room timeline
		}
		if beforeStreamSeq > 0 && entry.streamSeq >= beforeStreamSeq {
			continue
		}
		out = append(out, entryToMessage(entry))
	}
	return out
}

// cloneMessageBody returns a deep copy of the proto, or nil if the
// input is nil. Used on every store / read so the projection never
// hands out aliases into its internal state.
func cloneMessageBody(body *corev1.MessageBody) *corev1.MessageBody {
	if body == nil {
		return nil
	}
	return proto.Clone(body).(*corev1.MessageBody)
}

// entryToMessage converts the private entry into the public Message
// shape. Caller holds at least p.RLock.
func entryToMessage(entry *messageEntry) *Message {
	msg := &Message{
		EventID:                   entry.eventID,
		RoomID:                    entry.roomID,
		AuthorID:                  entry.authorID,
		Body:                      cloneMessageBody(entry.body),
		InReplyTo:                 entry.inReplyTo,
		InThread:                  entry.inThread,
		MentionedUserIDs:          append([]string(nil), entry.mentionedUserIDs...),
		EchoOfEventID:             entry.echoOfEventID,
		EchoFromThreadRootEventID: entry.echoFromThreadRootEventID,
		PostedAt:                  entry.postedAt,
		StreamSeq:                 entry.streamSeq,
		Tombstoned:                entry.tombstoned,
		RetractReason:             entry.retractReason,
	}
	if entry.editedAt != nil {
		t := *entry.editedAt
		msg.EditedAt = &t
	}
	return msg
}
