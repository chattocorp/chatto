package graph

const (
	defaultRoomEventsLimit = 50
	maxRoomEventsLimit     = 500
)

func roomEventsLimit(limit *int32) int {
	limitVal, _ := paginationArgs(limit, nil, defaultRoomEventsLimit, maxRoomEventsLimit)
	return limitVal
}
