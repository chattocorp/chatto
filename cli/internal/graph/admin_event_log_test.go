package graph

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// TestEventLog_BrowseNewestFirst exercises the EventLog resolver
// against a populated EVT stream. Joining a couple of rooms
// produces both a UserJoinedRoom event (from the membership migration
// happening during test setup as the bootstrap user lands in
// `general`/`announcements`) and additional joins as we go, so the
// stream is non-empty and we can assert ordering + pagination.
func TestEventLog_BrowseNewestFirst(t *testing.T) {
	env := setupTestResolver(t)
	ctx := env.authContext()

	// Drive a few more membership events into the stream so we have
	// something definite to page through. JoinRoom writes one durable
	// event on evt.room.{R}.
	for i := 0; i < 3; i++ {
		login := "logbrowser" + strconv.Itoa(i)
		u := env.createVerifiedUser(t, login, "Log Browser "+strconv.Itoa(i), "password123")
		_, err := env.core.JoinRoom(ctx, u.Id, core.KindChannel, u.Id, env.testRoom.Id)
		require.NoError(t, err)
	}

	adminQ := env.resolver.AdminQueries()
	adminCtx := &struct{}{} // parent admin resolver only needs auth context, not obj fields

	conn, err := adminQ.EventLog(ctx, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.GreaterOrEqual(t, len(conn.Entries), 3)
	require.GreaterOrEqual(t, int(conn.TotalCount), len(conn.Entries))

	// Newest-first: sequence numbers must descend.
	for i := 1; i < len(conn.Entries); i++ {
		prev, _ := strconv.ParseUint(conn.Entries[i-1].Sequence, 10, 64)
		curr, _ := strconv.ParseUint(conn.Entries[i].Sequence, 10, 64)
		require.Greater(t, prev, curr, "entries should be ordered newest-first")
	}

	// Aggregate parsing works for evt.room.{R}.
	for _, e := range conn.Entries {
		if e.AggregateType == "room" {
			require.NotEmpty(t, e.AggregateID)
			require.NotEmpty(t, e.EventType)
			require.NotEmpty(t, e.PayloadJSON)
		}
	}

	// Pagination via the endCursor: next page is older.
	limit := int32(2)
	page1, err := adminQ.EventLog(ctx, nil, &limit, nil)
	require.NoError(t, err)
	require.Len(t, page1.Entries, 2)
	require.NotNil(t, page1.EndCursor)
	require.True(t, page1.HasOlder)

	page2, err := adminQ.EventLog(ctx, nil, &limit, page1.EndCursor)
	require.NoError(t, err)
	require.NotEmpty(t, page2.Entries)
	page1Min, _ := strconv.ParseUint(*page1.EndCursor, 10, 64)
	page2Max, _ := strconv.ParseUint(page2.Entries[0].Sequence, 10, 64)
	require.Less(t, page2Max, page1Min, "page 2's newest entry must be older than page 1's oldest cursor")

	_ = adminCtx
}

// TestEventLogEntry_LookupBySequence exercises the single-entry
// resolver, including the "no such sequence" path.
func TestEventLogEntry_LookupBySequence(t *testing.T) {
	env := setupTestResolver(t)
	ctx := env.authContext()

	adminQ := env.resolver.AdminQueries()
	conn, err := adminQ.EventLog(ctx, nil, nil, nil)
	require.NoError(t, err)
	require.NotEmpty(t, conn.Entries, "expected at least one event on EVT for this test")

	// Look up the first entry by its sequence.
	target := conn.Entries[0]
	entry, err := adminQ.EventLogEntry(ctx, nil, target.Sequence)
	require.NoError(t, err)
	require.NotNil(t, entry)
	require.Equal(t, target.Sequence, entry.Sequence)
	require.Equal(t, target.Subject, entry.Subject)
	require.Equal(t, target.EventType, entry.EventType)

	// A sequence beyond the end returns (nil, nil) — admin code can
	// distinguish "not found" from "error" cleanly.
	missing, err := adminQ.EventLogEntry(ctx, nil, "9999999")
	require.NoError(t, err)
	require.Nil(t, missing)

	// Malformed sequence input is a real error.
	_, err = adminQ.EventLogEntry(ctx, nil, "not-a-number")
	require.Error(t, err)
}

// TestEventLog_AuthorizationDenied confirms admin.view-audit is the
// gate — a regular verified user without it gets ErrPermissionDenied
// from both resolvers even though admin.* is also a parent gate. The
// frontend hides the tab from non-auditors; this is the defence-in-
// depth check on the backend.
func TestEventLog_AuthorizationDenied(t *testing.T) {
	env := setupTestResolver(t)

	regular := env.createVerifiedUser(t, "no-audit", "No Audit User", "password123")
	ctx := env.authContextForUser(regular)

	adminQ := env.resolver.AdminQueries()

	_, err := adminQ.EventLog(ctx, nil, nil, nil)
	require.True(t, errors.Is(err, core.ErrPermissionDenied), "EventLog should deny non-auditor, got: %v", err)

	_, err = adminQ.EventLogEntry(ctx, nil, "1")
	require.True(t, errors.Is(err, core.ErrPermissionDenied), "EventLogEntry should deny non-auditor, got: %v", err)
}

func TestRBACMutationsUseAuthenticatedActorInEventLog(t *testing.T) {
	env := setupTestResolver(t)
	ctx := env.authContext()
	mutation := env.resolver.Mutation()

	roleName := "auditactor"
	_, err := mutation.CreateRole(ctx, model.CreateRoleInput{
		Name:        roleName,
		DisplayName: "Audit Actor",
		Description: "Role used to verify RBAC audit attribution",
	})
	require.NoError(t, err)

	pingable := true
	_, err = mutation.UpdateRole(ctx, model.UpdateRoleInput{
		Name:        roleName,
		DisplayName: "Audit Actor",
		Description: "Role used to verify RBAC audit attribution",
		Pingable:    &pingable,
	})
	require.NoError(t, err)

	_, err = mutation.GrantPermission(ctx, model.GrantPermissionInput{
		RoleName:   roleName,
		Permission: string(core.PermRoomJoin),
	})
	require.NoError(t, err)

	_, err = mutation.DeleteRole(ctx, model.DeleteRoleInput{Name: roleName})
	require.NoError(t, err)

	limit := int32(50)
	conn, err := env.resolver.AdminQueries().EventLog(ctx, nil, &limit, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)

	requireEventLogActor(t, conn.Entries, "RbacRoleCreatedEvent", env.testUser.Id, roleName)
	requireEventLogActor(t, conn.Entries, "RbacRolePingableChangedEvent", env.testUser.Id, roleName)
	requireEventLogActor(t, conn.Entries, "RbacPermissionGrantedEvent", env.testUser.Id, roleName)
	requireEventLogActor(t, conn.Entries, "RbacRoleDeletedEvent", env.testUser.Id, roleName)
}

func requireEventLogActor(t *testing.T, entries []*model.EventLogEntry, eventType, actorID, payloadSubstring string) {
	t.Helper()
	for _, entry := range entries {
		if entry.EventType == eventType && entry.ActorID == actorID && strings.Contains(entry.PayloadJSON, payloadSubstring) {
			return
		}
	}
	t.Fatalf("missing %s entry for actor %s containing %q", eventType, actorID, payloadSubstring)
}

func TestEventLogTotalCountUsesWideInteger(t *testing.T) {
	count, err := eventLogTotalCount(uint64(math.MaxInt32) + 1)
	require.NoError(t, err)
	require.Equal(t, int64(math.MaxInt32)+1, count)

	_, err = eventLogTotalCount(uint64(math.MaxInt64) + 1)
	require.Error(t, err)
}

func TestStreamMsgToEventLogEntryParsesAuthAggregate(t *testing.T) {
	event := &corev1.Event{
		Id:      "E1",
		ActorId: "system",
		Event: &corev1.Event_RegistrationVerificationCodeIssued{
			RegistrationVerificationCodeIssued: &corev1.RegistrationVerificationCodeIssuedEvent{EmailHash: "hash"},
		},
	}
	data, err := proto.Marshal(event)
	require.NoError(t, err)

	entry, err := streamMsgToEventLogEntry(&jetstream.RawStreamMsg{
		Subject:  events.AuthAggregate().Subject(events.EventRegistrationVerificationCodeIssued),
		Sequence: 7,
		Data:     data,
	})
	require.NoError(t, err)
	require.Equal(t, events.AggregateAuth, entry.AggregateType)
	require.Equal(t, events.AuthServerID, entry.AggregateID)
	require.Equal(t, "RegistrationVerificationCodeIssuedEvent", entry.EventType)
}
