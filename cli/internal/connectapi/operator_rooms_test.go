package connectapi

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

func TestOperatorRoomPageKeepsDistinctMatchingIDs(t *testing.T) {
	t.Parallel()

	rooms := []*readmodel.Room{
		{Id: "R2", Name: "same", Archived: true},
		{Id: "R1", Name: "same"},
		{Id: "R3", Name: "other"},
	}
	page, total, more := operatorRoomPage(rooms, "same", 1, 0)
	if total != 2 || !more || len(page) != 1 || page[0].GetId() != "R1" {
		t.Fatalf("first matching page = %v, total=%d, more=%t", page, total, more)
	}
	page, total, more = operatorRoomPage(rooms, "same", 1, 1)
	if total != 2 || more || len(page) != 1 || page[0].GetId() != "R2" || !page[0].GetArchived() {
		t.Fatalf("second matching page = %v, total=%d, more=%t", page, total, more)
	}
}
