package core

import (
	"math"
	"slices"
	"time"
	"unsafe"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/pkg/events/intern"
)

// notificationIDKind is the pool for the recipient, actor, and room IDs of
// the Notifications projection. The users and rooms of the server limit these
// sets, so a table that never shrinks is safe for them. Notification IDs are
// not interned: occurrences expire, and an append-only table would keep every
// ID.
type notificationIDKind struct{}
type notificationIDHandle = intern.ID[notificationIDKind]

const (
	notificationRowRead uint8 = 1 << iota
	// notificationRowAlertResolved records that alert_delivered is present.
	notificationRowAlertResolved
	notificationRowAlertDelivered
	notificationRowHasAlertExpiry
	notificationRowHasThreadRoot
	// notificationRowHasScope records that the row is in the scope index.
	notificationRowHasScope
	// notificationRowSourceIsMessage records that the source event is the
	// message of the signal, so messageEvent also gives the source event ID.
	notificationRowSourceIsMessage
)

// notificationRow is the retained state of one notification occurrence. It
// replaces a complete NotificationOccurrence protobuf message, which costs
// about five times the memory. Read boundaries build detached protobuf
// messages with occurrence.
//
// recipient, expiresAt, notificationSequence, flags, and the scope fields are
// valid for every row. The other compact fields are valid only when full is
// nil.
type notificationRow struct {
	id string
	// full is set when the compact fields cannot reproduce the occurrence, for
	// example for a signal kind from a newer server version. It holds the
	// occurrence as first stored. Read and alert state come from flags.
	full *notificationv1.NotificationOccurrence
	// source is the source event ID when the source is not the message of the
	// signal, for example a reaction event. Such an ID occurs once and goes
	// away with the occurrence, so it stays outside the append-only tables.
	source string
	emoji  string

	// Times are Unix nanoseconds.
	sourceCreatedAt int64
	expiresAt       int64
	alertExpiresAt  int64

	sourceSequence       uint64
	notificationSequence uint64

	recipient notificationIDHandle
	actor     notificationIDHandle
	// room and threadRoot give the read-boundary scope when
	// notificationRowHasScope is set.
	room         notificationIDHandle
	messageEvent eventHandle
	threadRoot   eventHandle

	attention int32
	// kind is the NotificationSignal oneof field number.
	kind  uint8
	flags uint8
}

func (r *notificationRow) has(flag uint8) bool { return r.flags&flag != 0 }

// notificationScopeKey is notificationReadBoundaryScope in handles.
type notificationScopeKey struct {
	recipient  notificationIDHandle
	room       notificationIDHandle
	threadRoot eventHandle
}

// notificationRows holds the occurrences of the Notifications projection and
// their indexes. The projection lock guards it.
type notificationRows struct {
	// byCall indexes active call alerts for join and end cleanup.
	byCall  map[string][]*notificationRow
	byID    map[string]*notificationRow
	byUser  map[notificationIDHandle][]*notificationRow
	byScope map[notificationScopeKey][]*notificationRow
	ids     projectionIDTable[notificationIDKind]
	// eventIDs interns message and thread root event IDs. Production wiring
	// shares the process-wide table (ADR-110), which holds the IDs of all
	// messages already.
	eventIDs       *eventIDTable
	sharedEventIDs bool
}

func newNotificationRows(eventIDs *eventIDTable) *notificationRows {
	shared := eventIDs != nil
	if !shared {
		eventIDs = newEventIDTable()
	}
	return &notificationRows{
		byCall:         make(map[string][]*notificationRow),
		byID:           make(map[string]*notificationRow),
		byUser:         make(map[notificationIDHandle][]*notificationRow),
		byScope:        make(map[notificationScopeKey][]*notificationRow),
		eventIDs:       eventIDs,
		sharedEventIDs: shared,
	}
}

// add stores occurrence, which must have a valid expires_at and an ID that is
// not stored. The store keeps no reference to occurrence.
func (s *notificationRows) add(occurrence *notificationv1.NotificationOccurrence) *notificationRow {
	row := &notificationRow{
		id:                   occurrence.GetId(),
		emoji:                occurrence.GetSignal().GetReactionReceived().GetEmoji(),
		sourceCreatedAt:      occurrence.GetSourceCreatedAt().AsTime().UnixNano(),
		expiresAt:            saturatedUnixNano(occurrence.GetExpiresAt().AsTime()),
		alertExpiresAt:       occurrence.GetAlertExpiresAt().AsTime().UnixNano(),
		sourceSequence:       occurrence.GetSourceStreamSequence(),
		notificationSequence: occurrence.GetNotificationStreamSequence(),
		recipient:            s.ids.Intern(occurrence.GetRecipientId()),
		actor:                s.ids.Intern(occurrence.GetActorId()),
		attention:            int32(occurrence.GetAttentionLevel()),
		kind:                 notificationSignalKindNumber(occurrence.GetSignal()),
	}
	if occurrence.GetRead() {
		row.flags |= notificationRowRead
	}
	if occurrence.AlertDelivered != nil {
		row.flags |= notificationRowAlertResolved
		if occurrence.GetAlertDelivered() {
			row.flags |= notificationRowAlertDelivered
		}
	}
	if occurrence.GetAlertExpiresAt() != nil {
		row.flags |= notificationRowHasAlertExpiry
	}
	if message := notificationSignalMessage(occurrence.GetSignal()); message != nil {
		row.room = s.ids.Intern(message.GetRoomId())
		row.messageEvent = s.eventIDs.Intern(message.GetEventId())
		row.threadRoot = s.eventIDs.Intern(message.GetThreadRootEventId())
		if message.ThreadRootEventId != nil {
			row.flags |= notificationRowHasThreadRoot
		}
		if message.GetEventId() == occurrence.GetSourceEventId() {
			row.flags |= notificationRowSourceIsMessage
		}
	}
	if !row.has(notificationRowSourceIsMessage) {
		row.source = occurrence.GetSourceEventId()
	}
	if _, ok := notificationOccurrenceReadScope(occurrence); ok {
		row.flags |= notificationRowHasScope
	}
	// The round trip decides whether the compact fields are sufficient. Thus,
	// a field or signal kind that this code does not know cannot be lost.
	if !proto.Equal(s.occurrence(row), occurrence) {
		row.full = proto.Clone(occurrence).(*notificationv1.NotificationOccurrence)
	}

	if call := occurrence.GetSignal().GetIncomingCall(); call != nil {
		s.byCall[call.GetCallId()] = append(s.byCall[call.GetCallId()], row)
	}
	s.byID[row.id] = row
	s.byUser[row.recipient] = append(s.byUser[row.recipient], row)
	if row.has(notificationRowHasScope) {
		s.byScope[row.scopeKey()] = append(s.byScope[row.scopeKey()], row)
	}
	return row
}

func (r *notificationRow) scopeKey() notificationScopeKey {
	return notificationScopeKey{recipient: r.recipient, room: r.room, threadRoot: r.threadRoot}
}

func (s *notificationRows) remove(row *notificationRow) {
	if call := row.full.GetSignal().GetIncomingCall(); call != nil {
		removeNotificationRow(s.byCall, call.GetCallId(), row)
	}
	delete(s.byID, row.id)
	removeNotificationRow(s.byUser, row.recipient, row)
	if row.has(notificationRowHasScope) {
		removeNotificationRow(s.byScope, row.scopeKey(), row)
	}
}

// removeNotificationRow removes row from the index entry for key. Order in an
// index entry has no meaning.
func removeNotificationRow[K comparable](index map[K][]*notificationRow, key K, row *notificationRow) {
	rows := index[key]
	i := slices.Index(rows, row)
	if i < 0 {
		return
	}
	last := len(rows) - 1
	rows[i] = rows[last]
	rows[last] = nil
	if last == 0 {
		delete(index, key)
		return
	}
	index[key] = rows[:last]
}

// get returns the row for notificationID when it belongs to recipientID.
func (s *notificationRows) get(recipientID, notificationID string) *notificationRow {
	row := s.byID[notificationID]
	if row == nil || s.ids.Resolve(row.recipient) != recipientID {
		return nil
	}
	return row
}

// userRows returns the rows of one recipient. The caller must not change the
// result.
func (s *notificationRows) userRows(userID string) []*notificationRow {
	recipient, ok := s.ids.Lookup(userID)
	if !ok {
		return nil
	}
	return s.byUser[recipient]
}

// scopeRows returns the rows of one recipient and timeline. The caller must
// not change the result. It only reads the store, so a read lock is
// sufficient.
func (s *notificationRows) scopeRows(scope notificationReadBoundaryScope) []*notificationRow {
	recipient, ok := s.ids.Lookup(scope.userID)
	if !ok {
		return nil
	}
	room, ok := s.ids.Lookup(scope.roomID)
	if !ok {
		return nil
	}
	var threadRoot eventHandle
	if scope.threadRootEventID != "" {
		if threadRoot, ok = s.eventIDs.Lookup(scope.threadRootEventID); !ok {
			return nil
		}
	}
	return s.byScope[notificationScopeKey{recipient: recipient, room: room, threadRoot: threadRoot}]
}

// occurrence builds a detached protobuf message for row.
func (s *notificationRows) occurrence(row *notificationRow) *notificationv1.NotificationOccurrence {
	var occurrence *notificationv1.NotificationOccurrence
	if row.full != nil {
		occurrence = proto.Clone(row.full).(*notificationv1.NotificationOccurrence)
	} else {
		occurrence = &notificationv1.NotificationOccurrence{
			Id:                         row.id,
			RecipientId:                s.ids.Resolve(row.recipient),
			SourceEventId:              s.sourceEventID(row),
			SourceCreatedAt:            timestamppb.New(time.Unix(0, row.sourceCreatedAt)),
			ActorId:                    s.ids.Resolve(row.actor),
			Signal:                     s.signal(row),
			ExpiresAt:                  timestamppb.New(time.Unix(0, row.expiresAt)),
			SourceStreamSequence:       row.sourceSequence,
			AttentionLevel:             notificationv1.NotificationAttentionLevel(row.attention),
			NotificationStreamSequence: row.notificationSequence,
		}
		if row.has(notificationRowHasAlertExpiry) {
			occurrence.AlertExpiresAt = timestamppb.New(time.Unix(0, row.alertExpiresAt))
		}
	}
	occurrence.Read = row.has(notificationRowRead)
	occurrence.AlertDelivered = nil
	if row.has(notificationRowAlertResolved) {
		occurrence.AlertDelivered = new(row.has(notificationRowAlertDelivered))
	}
	return occurrence
}

func (s *notificationRows) sourceEventID(row *notificationRow) string {
	if row.has(notificationRowSourceIsMessage) {
		return s.eventIDs.Resolve(row.messageEvent)
	}
	return row.source
}

// signal builds the signal of a compact row. It returns nil for a kind that
// has no compact form.
func (s *notificationRows) signal(row *notificationRow) *notificationv1.NotificationSignal {
	message := &notificationv1.NotificationMessageReference{
		RoomId:  s.ids.Resolve(row.room),
		EventId: s.eventIDs.Resolve(row.messageEvent),
	}
	if row.has(notificationRowHasThreadRoot) {
		message.ThreadRootEventId = new(s.eventIDs.Resolve(row.threadRoot))
	}
	signal := &notificationv1.NotificationSignal{}
	switch row.kind {
	case 1:
		signal.Kind = &notificationv1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &notificationv1.DirectMessageReceived{Message: message}}
	case 2:
		signal.Kind = &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{Message: message}}
	case 3:
		signal.Kind = &notificationv1.NotificationSignal_ReplyReceived{ReplyReceived: &notificationv1.ReplyReceived{Message: message}}
	case 4:
		signal.Kind = &notificationv1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &notificationv1.RoleMentionReceived{Message: message}}
	case 5:
		signal.Kind = &notificationv1.NotificationSignal_HereMentionReceived{HereMentionReceived: &notificationv1.HereMentionReceived{Message: message}}
	case 6:
		signal.Kind = &notificationv1.NotificationSignal_AllMentionReceived{AllMentionReceived: &notificationv1.AllMentionReceived{Message: message}}
	case 7:
		signal.Kind = &notificationv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &notificationv1.FollowedThreadActivity{Message: message}}
	case 8:
		signal.Kind = &notificationv1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &notificationv1.FollowedRoomActivity{Message: message}}
	case 9:
		signal.Kind = &notificationv1.NotificationSignal_ReactionReceived{ReactionReceived: &notificationv1.ReactionReceived{Message: message, Emoji: row.emoji}}
	case 10:
		signal.Kind = &notificationv1.NotificationSignal_RoomMessageReceived{RoomMessageReceived: &notificationv1.RoomMessageReceived{Message: message}}
	default:
		return nil
	}
	return signal
}

// notificationSignalKindNumber returns the oneof field number of the signal
// kind, or zero when no kind is set.
func notificationSignalKindNumber(signal *notificationv1.NotificationSignal) uint8 {
	if signal == nil {
		return 0
	}
	message := signal.ProtoReflect()
	field := message.WhichOneof(message.Descriptor().Oneofs().ByName("kind"))
	if field == nil || field.Number() > math.MaxUint8 {
		return 0
	}
	return uint8(field.Number())
}

// saturatedUnixNano returns value in Unix nanoseconds. A time outside the
// int64 range, which ends in the year 2262, gives the nearest limit. Thus,
// comparisons keep their order inside the range and cannot wrap outside it.
func saturatedUnixNano(value time.Time) int64 {
	if value.Before(time.Unix(0, math.MinInt64)) {
		return math.MinInt64
	}
	if value.After(time.Unix(0, math.MaxInt64)) {
		return math.MaxInt64
	}
	return value.UnixNano()
}

// estimatedBytes approximates the retained size of the rows, their indexes,
// and the ID table. It excludes a shared event ID table.
func (s *notificationRows) estimatedBytes() int64 {
	// One row allocation, its ID string, and one entry in each index.
	const rowBytes = int64(unsafe.Sizeof(notificationRow{})) + projectionMapEntryOverhead + 2*8
	bytes := s.ids.EstimatedBytes()
	if !s.sharedEventIDs {
		bytes += s.eventIDs.EstimatedBytes()
	}
	for id, row := range s.byID {
		bytes += rowBytes + int64(len(id)+len(row.source)+len(row.emoji))
		if row.full != nil {
			// A decoded message needs about three times its encoded size.
			bytes += 3 * int64(proto.Size(row.full))
		}
	}
	bytes += int64(len(s.byUser)+len(s.byScope)+len(s.byCall)) * (projectionCompactMapEntryOverhead + projectionSliceEntryOverhead)
	return bytes
}
