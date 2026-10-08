package core

import "hmans.de/chatto/pkg/events/intern"

// eventIDTable is shared by independently replayed Chatto projections.
// Core wiring owns its lifetime and chooses which models share it.
type eventIDKind struct{}
type eventHandle = intern.ID[eventIDKind]
type eventIDTable = intern.ConcurrentTable[eventIDKind]

// The timeline uses the same byte storage without an interning index for
// body-event IDs. Its projection lock guards writes.
type idArena = intern.Arena
type idLocation = intern.Location

func newEventIDTable() *eventIDTable {
	return intern.NewConcurrentTable[eventIDKind]()
}
