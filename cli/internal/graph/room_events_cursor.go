package graph

import (
	"fmt"
	"strconv"
	"strings"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// buildRoomEventsConnection unwraps core.RoomEvent (which carries the
// JetStream sequence) into bare SpaceEvent pointers for the GraphQL
// model and renders the cursor sequences as opaque strings.
func buildRoomEventsConnection(r *core.RoomEventsResult) *model.RoomEventsConnection {
	events := make([]*corev1.SpaceEvent, len(r.Events))
	for i, e := range r.Events {
		events[i] = e.SpaceEvent
	}
	conn := &model.RoomEventsConnection{
		Events:   events,
		HasOlder: r.HasOlder,
		HasNewer: r.HasNewer,
	}
	if start := formatRoomEventCursor(r.StartCursorSeq); start != "" {
		conn.StartCursor = &start
	}
	if end := formatRoomEventCursor(r.EndCursorSeq); end != "" {
		conn.EndCursor = &end
	}
	return conn
}

// Room-event pagination cursors.
//
// Internally a cursor is a JetStream stream sequence (`uint64`). At the
// GraphQL boundary it's an opaque string of the form `seq:<n>`. The
// `seq:` prefix exists so a future migration to a richer cursor (e.g.,
// time-based, or carrying a stream identifier) can be detected without
// silently mis-parsing old cursors as the new format.
//
// Cursors are exposed via `RoomEventsConnection.startCursor` and
// `endCursor` and consumed via the `before`/`after` query args. Clients
// must treat them as opaque.

const cursorSeqPrefix = "seq:"

// formatRoomEventCursor renders a JetStream sequence as the opaque cursor
// string clients see. Returns "" for sequence 0 so the GraphQL field can
// be a nullable String — empty pages have no cursor.
func formatRoomEventCursor(seq uint64) string {
	if seq == 0 {
		return ""
	}
	return cursorSeqPrefix + strconv.FormatUint(seq, 10)
}

// parseRoomEventCursor decodes an opaque cursor back to a sequence.
// Returns 0 with no error if the cursor is the empty string (treated as
// "no cursor"). Any other malformed input is an error so a stale or
// hand-edited cursor surfaces clearly rather than silently paging from
// the start of the stream.
func parseRoomEventCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	rest, ok := strings.CutPrefix(cursor, cursorSeqPrefix)
	if !ok {
		return 0, fmt.Errorf("invalid cursor format")
	}
	seq, err := strconv.ParseUint(rest, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor sequence: %w", err)
	}
	return seq, nil
}
