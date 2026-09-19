package connectapi

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
)

func TestAdminMemberBatchAvoidsPresenceKVReads(t *testing.T) {
	env := newConnectAPITestEnv(t)
	if err := env.core.AssignOwnerRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatal(err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	// Observe direct reads, including missing presence keys. The singular read
	// below proves this observer can see the path the batch must avoid.
	reads, err := env.nc.SubscribeSync("$JS.API.DIRECT.GET.KV_MEMORY_CACHE.>")
	if err != nil {
		t.Fatal(err)
	}
	defer reads.Unsubscribe()
	if err := env.nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := env.adminUsers.GetMember(ctx, connect.NewRequest(&adminv1.GetMemberRequest{
		Target: &adminv1.GetMemberRequest_UserId{UserId: env.viewer.Id},
	})); err != nil {
		t.Fatal(err)
	}
	if _, err := reads.NextMsg(time.Second); err != nil {
		t.Fatalf("singular presence read was not observed: %v", err)
	}
	if _, err := env.adminUsers.ListMembers(ctx, connect.NewRequest(&adminv1.ListMembersRequest{})); err != nil {
		t.Fatal(err)
	}
	if _, err := env.adminUsers.BatchGetMembers(ctx, connect.NewRequest(&adminv1.BatchGetMembersRequest{
		UserIds: []string{env.viewer.Id},
	})); err != nil {
		t.Fatal(err)
	}
	if err := env.nc.Flush(); err != nil {
		t.Fatal(err)
	}
	if count, _, err := reads.Pending(); err != nil || count != 0 {
		t.Fatalf("list/batch direct cache reads = %d, %v; want zero", count, err)
	}
}
