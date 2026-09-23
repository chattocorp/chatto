package cmd

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestOperatorRoomList(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	group, err := env.core.CreateRoomGroup(env.ctx, core.SystemActorID, "Import Test", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	rooms := make(map[string]string)
	for _, name := range []string{"alpha", "beta", "gamma"} {
		room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.GetId(), name, "Description for "+name)
		if err != nil {
			t.Fatalf("CreateRoom(%q): %v", name, err)
		}
		rooms[name] = room.GetId()
	}
	if _, err := env.core.ArchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, rooms["beta"]); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}

	decode := func(args ...string) *operatorv1.ListRoomsResponse {
		t.Helper()
		output := env.run(t, append([]string{"operator", "room", "list", "--json"}, args...)...)
		var response operatorv1.ListRoomsResponse
		if err := protojson.Unmarshal([]byte(output), &response); err != nil {
			t.Fatalf("decode room list: %v\n%s", err, output)
		}
		return &response
	}

	all := decode("--limit", "100")
	if all.GetPage().GetTotalCount() != int64(len(all.GetRooms())) || all.GetPage().GetHasMore() {
		t.Fatalf("all rooms page = %+v, rooms = %d", all.GetPage(), len(all.GetRooms()))
	}
	ids := make([]string, 0, len(all.GetRooms()))
	for _, room := range all.GetRooms() {
		if room.GetKind() != apiv1.RoomKind_ROOM_KIND_CHANNEL {
			t.Fatalf("non-channel room in operator list: %+v", room)
		}
		ids = append(ids, room.GetId())
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("room IDs are not ordered: %v", ids)
	}
	for name, id := range rooms {
		var found bool
		for _, room := range all.GetRooms() {
			if room.GetId() == id {
				found = true
				if room.GetName() != name || room.GetGroupId() != group.GetId() || room.GetDescription() != "Description for "+name || room.GetArchived() != (name == "beta") {
					t.Fatalf("room %s = %+v", name, room)
				}
			}
		}
		if !found {
			t.Fatalf("room %s (%s) is missing", name, id)
		}
	}

	first := decode("--limit", "1", "--offset", "0")
	if len(first.GetRooms()) != 1 || first.GetRooms()[0].GetId() != ids[0] || first.GetPage().GetTotalCount() != int64(len(ids)) || !first.GetPage().GetHasMore() {
		t.Fatalf("first page = %+v", first)
	}
	second := decode("--limit", "1", "--offset", "1")
	if len(second.GetRooms()) != 1 || second.GetRooms()[0].GetId() != ids[1] {
		t.Fatalf("second page = %+v", second)
	}
	end := decode("--offset", fmt.Sprint(len(ids)))
	if len(end.GetRooms()) != 0 || end.GetPage().GetTotalCount() != int64(len(ids)) || end.GetPage().GetHasMore() {
		t.Fatalf("end page = %+v", end)
	}

	beta := decode("--name", "beta")
	if len(beta.GetRooms()) != 1 || beta.GetRooms()[0].GetId() != rooms["beta"] || !beta.GetRooms()[0].GetArchived() || beta.GetPage().GetTotalCount() != 1 {
		t.Fatalf("archived exact-name lookup = %+v", beta)
	}
	noMatch := decode("--name", "BETA")
	if len(noMatch.GetRooms()) != 0 || noMatch.GetPage().GetTotalCount() != 0 || noMatch.GetPage().GetHasMore() {
		t.Fatalf("empty exact-name lookup = %+v", noMatch)
	}

	human := env.run(t, "operator", "room", "list", "--name", "beta")
	for _, want := range []string{rooms["beta"], "group=" + group.GetId(), "archived=true", `description="Description for beta"`, "total=1 has_more=false"} {
		if !strings.Contains(human, want) {
			t.Fatalf("human output missing %q: %s", want, human)
		}
	}
	if _, err := env.execute(t, "operator", "room", "list", "--offset", "-1"); err == nil || err.Error() != "--offset must be greater than or equal to 0" {
		t.Fatalf("negative offset error = %v", err)
	}
}
