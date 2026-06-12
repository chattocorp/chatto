package graph

import (
	"fmt"
	"strconv"
	"strings"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// buildRoomEventsConnection unwraps core.RoomEvent (which carries the
// timeline position) into delivered Event envelopes for the GraphQL
// model and renders opaque cursors.
func buildRoomEventsConnection(r *core.RoomEventsResult) *model.RoomEventsConnection {
	events := make([]core.EventEnvelope, len(r.Events))
	for i, e := range r.Events {
		events[i] = core.NewEVTEventEnvelope(e.Event)
	}
	conn := &model.RoomEventsConnection{
		Events:   events,
		HasOlder: r.HasOlder,
		HasNewer: r.HasNewer,
	}
	if start := formatRoomEventCursor(r.StartCursor); start != "" {
		conn.StartCursor = &start
	}
	if end := formatRoomEventCursor(r.EndCursor); end != "" {
		conn.EndCursor = &end
	}
	return conn
}

// Room-event pagination cursors.
//
// Internally, room-visible pagination uses (created_at, stream sequence) so
// migrated historical events can be displayed chronologically even when their
// EVT append order was grouped by migration step. At the GraphQL boundary the
// rich cursor is `pos:<unix-nano>:<seq>`. Legacy `seq:<n>` cursors remain
// accepted for old in-flight clients and thread pagination.
//
// Cursors are exposed via `RoomEventsConnection.startCursor` and
// `endCursor` and consumed via the `before`/`after` query args. Clients
// must treat them as opaque.

const cursorSeqPrefix = "seq:"
const cursorPosPrefix = "pos:"

// formatRoomEventCursor renders a room timeline position as the opaque cursor
// string clients see. Returns "" for zero-value cursors so the GraphQL field
// can be nullable — empty pages have no cursor.
func formatRoomEventCursor(cursor core.RoomTimelineCursor) string {
	if cursor.StreamSeq == 0 {
		return ""
	}
	if cursor.HasCreatedAt {
		return cursorPosPrefix + strconv.FormatInt(cursor.CreatedAtUnixNano, 10) + ":" + strconv.FormatUint(cursor.StreamSeq, 10)
	}
	return cursorSeqPrefix + strconv.FormatUint(cursor.StreamSeq, 10)
}

// parseRoomEventCursor decodes an opaque cursor back to a timeline position.
// Returns 0 with no error if the cursor is the empty string (treated as
// "no cursor"). Any other malformed input is an error so a stale or
// hand-edited cursor surfaces clearly rather than silently paging from
// the start of the stream.
func parseRoomEventCursor(cursor string) (core.RoomTimelineCursor, error) {
	if cursor == "" {
		return core.RoomTimelineCursor{}, nil
	}
	if rest, ok := strings.CutPrefix(cursor, cursorSeqPrefix); ok {
		seq, err := strconv.ParseUint(rest, 10, 64)
		if err != nil {
			return core.RoomTimelineCursor{}, fmt.Errorf("invalid cursor sequence: %w", err)
		}
		return core.RoomTimelineCursor{StreamSeq: seq}, nil
	}
	rest, ok := strings.CutPrefix(cursor, cursorPosPrefix)
	if !ok {
		return core.RoomTimelineCursor{}, fmt.Errorf("invalid cursor format")
	}
	timePart, seqPart, ok := strings.Cut(rest, ":")
	if !ok {
		return core.RoomTimelineCursor{}, fmt.Errorf("invalid cursor position")
	}
	nanos, err := strconv.ParseInt(timePart, 10, 64)
	if err != nil {
		return core.RoomTimelineCursor{}, fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	seq, err := strconv.ParseUint(seqPart, 10, 64)
	if err != nil {
		return core.RoomTimelineCursor{}, fmt.Errorf("invalid cursor sequence: %w", err)
	}
	return core.RoomTimelineCursor{
		StreamSeq:         seq,
		CreatedAtUnixNano: nanos,
		HasCreatedAt:      true,
	}, nil
}

func roomEventPositionCursor(event *core.RoomEvent) core.RoomTimelineCursor {
	if event == nil {
		return core.RoomTimelineCursor{}
	}
	cursor := core.RoomTimelineCursor{StreamSeq: event.Sequence}
	if event.Event != nil && event.Event.GetCreatedAt() != nil {
		cursor.CreatedAtUnixNano = event.Event.GetCreatedAt().AsTime().UnixNano()
		cursor.HasCreatedAt = true
	}
	return cursor
}
