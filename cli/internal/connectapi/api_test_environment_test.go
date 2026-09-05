package connectapi

import (
	"bytes"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/core"
)

// A second test environment must not delete or inherit the first event log.
// Keep both cores open to test isolation without relying on cleanup timing.
func TestConnectAPITestEnvironmentsKeepIndependentEventLogs(t *testing.T) {
	first := newConnectAPITestEnv(t)
	if err := first.core.GrantServerPermission(first.ctx, core.SystemActorID, core.RoleEveryone, core.PermRoomManage); err != nil {
		t.Fatal(err)
	}
	js, err := jetstream.New(first.nc)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(first.ctx, "EVT")
	if err != nil {
		t.Fatal(err)
	}
	before, err := stream.GetLastMsgForSubject(first.ctx, "evt.>")
	if err != nil {
		t.Fatal(err)
	}

	second := newConnectAPITestEnv(t)
	after, err := stream.GetMsg(first.ctx, before.Sequence)
	if err != nil {
		t.Fatalf("first event log changed after second environment started: %v", err)
	}
	if after.Subject != before.Subject || !bytes.Equal(after.Data, before.Data) {
		t.Fatal("second environment replaced an event in the first environment")
	}
	for _, check := range []struct {
		name string
		env  *connectAPITestEnv
		want bool
	}{
		{"first", first, true},
		{"second", second, false},
	} {
		allowed, err := check.env.core.HasServerPermission(check.env.ctx, check.env.viewer.Id, core.PermRoomManage)
		if err != nil {
			t.Fatalf("%s permission: %v", check.name, err)
		}
		if allowed != check.want {
			t.Fatalf("%s room.manage = %v, want %v", check.name, allowed, check.want)
		}
	}
}
