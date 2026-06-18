package graph

import (
	"context"
	"testing"

	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomGroupItemsIncludesRoomEntriesWithoutViewerRoomMap(t *testing.T) {
	resolver := (&Resolver{}).RoomGroup()
	items, err := resolver.Items(context.Background(), &model.RoomGroupModel{
		Entries: []*corev1.SidebarGroupEntry{
			{Kind: corev1.SidebarGroupEntry_ROOM, Id: "R1"},
			{Kind: corev1.SidebarGroupEntry_SIDEBAR_LINK, Id: "L1"},
		},
		SidebarLinks: []*corev1.SidebarLink{
			{Id: "L1", Label: "Docs", Url: "https://example.com/docs"},
		},
	})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v, want room and link entries", items)
	}
	if items[0].Type != model.RoomGroupItemTypeRoom || items[0].ID != "R1" || items[0].Room != nil {
		t.Fatalf("room item = %+v, want ROOM R1 with nil room payload", items[0])
	}
	if items[1].Type != model.RoomGroupItemTypeSidebarLink || items[1].ID != "L1" || items[1].Link == nil {
		t.Fatalf("link item = %+v, want SIDEBAR_LINK L1 with link payload", items[1])
	}
}

func TestRoomGroupItemsFiltersMissingViewerRoomsWhenMapProvided(t *testing.T) {
	resolver := (&Resolver{}).RoomGroup()
	items, err := resolver.Items(context.Background(), &model.RoomGroupModel{
		Entries: []*corev1.SidebarGroupEntry{
			{Kind: corev1.SidebarGroupEntry_ROOM, Id: "R1"},
			{Kind: corev1.SidebarGroupEntry_ROOM, Id: "R2"},
		},
		ViewerRooms: map[string]*corev1.Room{
			"R2": {Id: "R2", Name: "Visible"},
		},
	})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want only visible room entry", items)
	}
	if items[0].Type != model.RoomGroupItemTypeRoom || items[0].ID != "R2" || items[0].Room.GetId() != "R2" {
		t.Fatalf("room item = %+v, want ROOM R2 with room payload", items[0])
	}
}
