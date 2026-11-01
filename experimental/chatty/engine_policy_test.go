package main

import (
	"testing"

	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestChattyHidesArchivedRoomsFromNavigation(t *testing.T) {
	rooms := []chatto.RoomEntry{
		{Room: chatto.Room{ID: "archived", Archived: true}},
		{Room: chatto.Room{ID: "active"}},
	}
	visible := activeRooms(rooms)
	if len(visible) != 1 || visible[0].Room.ID != "active" {
		t.Fatal("archived room remained in navigation")
	}
	if rooms[0].Room.ID != "archived" {
		t.Fatal("display filter mutated source data")
	}
}
