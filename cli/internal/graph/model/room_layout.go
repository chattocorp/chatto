package model

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

// RoomGroupModel is the GraphQL model for RoomGroup. It wraps the proto RoomGroup
// with a pre-resolved viewer-rooms map so the per-set rooms sub-resolver can
// turn room IDs into Room objects without an extra round trip.
type RoomGroupModel struct {
	ID          string
	Name        string
	Description string
	RoomIds     []string

	// ViewerRooms is shared across all sets in a single response and contains
	// only the rooms the caller can see; entries the caller can't see are
	// dropped before this map is built.
	ViewerRooms map[string]*corev1.Room
}
