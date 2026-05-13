package model

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

// RoomLayoutModel is the GraphQL model for RoomLayout.
// It wraps the proto RoomLayout with pre-resolved viewer room data
// so sub-resolvers can efficiently resolve room IDs to Room objects.
type RoomLayoutModel struct {
	// Sets from the proto layout, in display order.
	Sets []*RoomSetModel

	// ViewerRooms maps room ID → Room for all rooms in the space.
	// Used by sub-resolvers to resolve room IDs.
	ViewerRooms map[string]*corev1.Room
}

// RoomSetModel is the GraphQL model for RoomSet.
type RoomSetModel struct {
	ID          string
	Name        string
	Description string
	RoomIds     []string

	// ViewerRooms is a reference to the parent layout's ViewerRooms map.
	ViewerRooms map[string]*corev1.Room
}
