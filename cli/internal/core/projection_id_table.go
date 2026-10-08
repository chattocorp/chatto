package core

import (
	"cmp"
	"slices"

	"hmans.de/chatto/pkg/events/intern"
)

// projectionIDTable stores local IDs under the owning projection's lock.
type projectionIDTable[Kind any] = intern.Table[Kind]

// Kinds follow the existing pool boundaries. Mixed namespaces retain one kind;
// a kind is not a runtime table identity.
type userIDKind struct{}
type roomIDKind struct{}
type principalIDKind struct{}
type reactionIDKind struct{}
type badgeIDKind struct{}

type userHandle = intern.ID[userIDKind]
type roomHandle = intern.ID[roomIDKind]
type principalHandle = intern.ID[principalIDKind]
type reactionHandle = intern.ID[reactionIDKind]
type badgeHandle = intern.ID[badgeIDKind]

// handleIDResolver resolves a handle from projectionIDTable or eventIDTable.
type handleIDResolver[H ~uint32] interface {
	Resolve(handle H) string
}

// sortedHandleKeys returns the handle keys of values ordered by their IDs, so
// snapshots stay deterministic regardless of interning order.
func sortedHandleKeys[H ~uint32, V any](ids handleIDResolver[H], values map[H]V) []H {
	handles := make([]H, 0, len(values))
	for handle := range values {
		handles = append(handles, handle)
	}
	slices.SortFunc(handles, func(a, b H) int { return cmp.Compare(ids.Resolve(a), ids.Resolve(b)) })
	return handles
}

// handleSlice stores one value per event handle in a dense slice indexed by
// handle minus one. The zero value of T means "no value", so handles of IDs
// without a value cost only one zero element.
type handleSlice[T comparable] []T

// get returns the value for handle and whether it is set.
func (s handleSlice[T]) get(handle eventHandle) (T, bool) {
	var zero T
	if handle == 0 || int(handle) > len(s) {
		return zero, false
	}
	value := s[handle-1]
	return value, value != zero
}

// set stores value for handle and grows the slice when necessary. Handle zero
// is ignored.
func (s *handleSlice[T]) set(handle eventHandle, value T) {
	if handle == 0 {
		return
	}
	if missing := int(handle) - len(*s); missing > 0 {
		*s = append(*s, make([]T, missing)...)
	}
	(*s)[handle-1] = value
}
