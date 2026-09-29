package core

import "sync/atomic"

// uint32ColdCodec stores one uint32 per row.
type uint32ColdCodec struct{}

func (uint32ColdCodec) columns() int { return 1 }

func (uint32ColdCodec) encode(value uint32) (fields coldFields) {
	fields[0] = uint64(value)
	return fields
}

func (uint32ColdCodec) decode(fields coldFields) uint32 { return uint32(fields[0]) }

// timelineRowColdCodec stores one timeline row.
type timelineRowColdCodec struct{}

func (timelineRowColdCodec) columns() int { return 10 }

func (timelineRowColdCodec) encode(row timelineRow) (fields coldFields) {
	fields[0] = row.streamSeq
	fields[1] = uint64(row.createdAt)
	fields[2] = uint64(row.event)
	fields[3] = uint64(row.threadRoot)
	fields[4] = uint64(row.echoOf)
	fields[5] = uint64(row.room)
	fields[6] = uint64(row.actor)
	fields[7] = uint64(row.author)
	fields[8] = uint64(row.bodyIndex)
	flags := uint64(row.kind)
	if row.reply {
		flags |= 1 << 8
	}
	if row.historicalImport {
		flags |= 1 << 9
	}
	fields[9] = flags
	return fields
}

func (timelineRowColdCodec) decode(fields coldFields) timelineRow {
	return timelineRow{
		streamSeq:        fields[0],
		createdAt:        int64(fields[1]),
		event:            uint32(fields[2]),
		threadRoot:       uint32(fields[3]),
		echoOf:           uint32(fields[4]),
		room:             uint32(fields[5]),
		actor:            uint32(fields[6]),
		author:           uint32(fields[7]),
		bodyIndex:        uint32(fields[8]),
		kind:             timelineEventKind(fields[9] & 0xff),
		reply:            fields[9]&(1<<8) != 0,
		historicalImport: fields[9]&(1<<9) != 0,
	}
}

// timelineBodyStateColdCodec stores one timeline body state.
type timelineBodyStateColdCodec struct{}

func (timelineBodyStateColdCodec) columns() int { return 4 }

func (timelineBodyStateColdCodec) encode(state timelineBodyState) (fields coldFields) {
	fields[0] = state.currentSequence
	fields[1] = uint64(state.currentEventID)
	fields[2] = uint64(state.author)
	fields[3] = uint64(state.flags)
	return fields
}

func (timelineBodyStateColdCodec) decode(fields coldFields) timelineBodyState {
	return timelineBodyState{
		currentSequence: fields[0],
		currentEventID:  idLocation(fields[1]),
		author:          uint32(fields[2]),
		flags:           uint32(fields[3]),
	}
}

// coldWatermark publishes the event ID handle below which the Server Content
// View treats messages as cold. The room timeline advances it; other
// components and the event ID table read it to freeze their handle-indexed
// state. Handles are assigned in first-intern order, which follows stream
// order closely, so the watermark approximates message age. It affects only
// memory layout, never projection results.
type coldWatermark struct {
	handle atomic.Uint32
}

// below returns the handle below which state can be frozen, or zero when the
// watermark is not set.
func (w *coldWatermark) below() uint32 {
	if w == nil {
		return 0
	}
	return w.handle.Load()
}

// advance raises the watermark to handle.
func (w *coldWatermark) advance(handle uint32) {
	if w == nil {
		return
	}
	for {
		current := w.handle.Load()
		if handle <= current || w.handle.CompareAndSwap(current, handle) {
			return
		}
	}
}

// threadMessageRefColdCodec stores one thread message reference.
type threadMessageRefColdCodec struct{}

func (threadMessageRefColdCodec) columns() int { return 2 }

func (threadMessageRefColdCodec) encode(ref threadMessageRef) (fields coldFields) {
	fields[0], fields[1] = uint64(ref.room), uint64(ref.root)
	return fields
}

func (threadMessageRefColdCodec) decode(fields coldFields) threadMessageRef {
	return threadMessageRef{room: uint32(fields[0]), root: uint32(fields[1])}
}

// badgeMessageColdCodec stores one Badge source index message record.
type badgeMessageColdCodec struct{}

func (badgeMessageColdCodec) columns() int { return 7 }

func (badgeMessageColdCodec) encode(record badgeMessage) (fields coldFields) {
	fields[0] = record.seq
	fields[1] = uint64(record.createdAt)
	fields[2] = uint64(record.room)
	fields[3] = uint64(record.thread)
	fields[4] = uint64(record.actor)
	fields[5] = uint64(record.author)
	if record.retracted {
		fields[6] |= 1
	}
	if record.source {
		fields[6] |= 2
	}
	return fields
}

func (badgeMessageColdCodec) decode(fields coldFields) badgeMessage {
	return badgeMessage{
		seq:       fields[0],
		createdAt: int64(fields[1]),
		room:      uint32(fields[2]),
		thread:    uint32(fields[3]),
		actor:     uint32(fields[4]),
		author:    uint32(fields[5]),
		retracted: fields[6]&1 != 0,
		source:    fields[6]&2 != 0,
	}
}
