package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestImportArchiveConversation(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	a, err := interchange.ReadDirectory("../../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	naming := ImportNaming{ChannelPrefix: "source-", UsernamePrefix: "old-", UsernameSuffix: "-test"}
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	result, err := c.ImportArchive(ctx, a, naming, true, false)
	require.NoError(t, err)
	require.False(t, result.Complete)
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	_, err = c.ImportArchive(ctx, a, naming, false, false)
	require.ErrorContains(t, err, "accept-omissions")
	result, err = c.ImportArchive(ctx, a, naming, false, true)
	require.NoError(t, err)
	require.True(t, result.Complete)
	rooms, err := c.ListRooms(ctx, KindChannel)
	require.NoError(t, err)
	var roomID string
	for _, room := range rooms {
		if room.Name == "source-general" {
			roomID = room.Id
			require.Equal(t, "An example conversation", room.Description)
			require.Equal(t, "Synthetic format fixture", room.Purpose)
		}
	}
	require.NotEmpty(t, roomID)
	users, err := c.ListUsers(ctx)
	require.NoError(t, err)
	var alice string
	for _, u := range users {
		if u.Login == "old-alice-test" {
			alice = u.Id
		}
	}
	require.NotEmpty(t, alice)
	members, err := c.GetRoomMembersList(ctx, KindChannel, roomID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	progress, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.ImportAggregate().AllEventsFilter())
	require.NoError(t, err)
	m := &archiveImport{archive: a, receipts: []*evtv1.ImportProgressedEvent{progress[0].GetImportProgressed()}}
	body, err := c.GetFullMessageBody(ctx, m.nativeID("message", "message-2"))
	require.NoError(t, err)
	require.Equal(t, "An edited reply.", body.Body)
	require.NotNil(t, body.UpdatedAt)
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).AllEventsFilter())
	require.NoError(t, err)
	joins, edits, tombstones, reactions := 0, 0, 0, 0
	for _, fact := range facts {
		require.NotNil(t, fact.ImportContext)
		require.False(t, isDeliverableLiveEVTRoomEvent(fact))
		if fact.GetUserJoinedRoom() != nil {
			joins++
			require.Equal(t, int32(2020), int32(fact.CreatedAt.AsTime().Year()))
		}
		if fact.GetMessageEdited() != nil {
			edits++
		}
		if fact.GetMessageRetracted() != nil {
			tombstones++
			require.True(t, fact.ImportContext.UnknownTime)
		}
		if fact.GetReactionAdded() != nil {
			reactions++
		}
		if fact.ImportContext.HiddenTimeline {
			require.False(t, IsVisibleRoomTimelineEntry(fact))
		}
	}
	require.Equal(t, 2, joins)
	require.Equal(t, 1, edits)
	require.Equal(t, 1, tombstones)
	require.Equal(t, 2, reactions)
	store, err := c.mediaModel.GetAttachmentsStore(ctx)
	require.NoError(t, err)
	data, err := store.GetBytes(ctx, m.nativeID("asset", "asset-1"))
	require.NoError(t, err)
	require.Equal(t, a.Files["files/asset-1.txt"], data)
	before, err = c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	result, err = c.ImportArchive(ctx, a, naming, false, true)
	require.NoError(t, err)
	require.True(t, result.Complete)
	after, err = c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	naming.ChannelPrefix = "changed-"
	_, err = c.ImportArchive(ctx, a, naming, false, true)
	require.ErrorContains(t, err, "different import options")
}

func TestImportArchivePreflightRejectsWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, old, replacement, want string }{
		{"private channel", `"visibility":"public"`, `"visibility":"private"`, "unsupported destination channel"},
		{"username collision", `"username":"bob"`, `"username":"alice"`, "username collision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			a, err := interchange.ReadDirectory("../../../schema/interchange/v1/examples/conversation")
			require.NoError(t, err)
			changed := false
			for key, raw := range a.Metadata {
				s := strings.ReplaceAll(string(raw), tc.old, tc.replacement)
				if s != string(raw) {
					changed = true
				}
				a.Metadata[key] = []byte(s)
			}
			require.True(t, changed)
			before, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			_, err = c.ImportArchive(ctx, a, ImportNaming{}, false, true)
			require.ErrorContains(t, err, tc.want)
			after, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestImportArchiveReconcilesMembershipWithoutInventedJoin(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	a, err := interchange.ReadDirectory("../../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	raw := string(a.Metadata["events.jsonl"])
	a.Metadata["events.jsonl"] = []byte(strings.ReplaceAll(raw, `"user_ids":["user-1"]}}`, `"user_ids":["user-1","user-2"]}}`))
	require.NotEqual(t, raw, string(a.Metadata["events.jsonl"]))
	_, err = c.ImportArchive(ctx, a, ImportNaming{}, false, true)
	require.NoError(t, err)
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, "evt.room.*.user_joined")
	require.NoError(t, err)
	require.Len(t, facts, 3)
	visible := 0
	for _, fact := range facts {
		if IsVisibleRoomTimelineEntry(fact) {
			visible++
		} else {
			require.True(t, fact.ImportContext.UnknownTime)
		}
	}
	require.Equal(t, 2, visible)
	members, err := c.GetRoomMembersList(ctx, KindChannel, facts[0].GetUserJoinedRoom().RoomId)
	require.NoError(t, err)
	require.Len(t, members, 2)
}

func TestImportArchiveResumesAcrossFreshCore(t *testing.T) {
	t.Parallel()
	for _, cut := range []uint64{1, 2, 3, 4, 5, 8, 15, 16} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			ns, nc := testutil.StartNATS(t)
			c, err := NewChattoCore(testContext(t), nc, testCoreConfig())
			require.NoError(t, err)
			original := c.EventPublisher.TypedEventLog
			injected := errors.New("injected interruption")
			// Fail encoding before one atomic batch. Every earlier operation is
			// durable; this batch must leave neither facts nor a receipt.
			c.EventPublisher.TypedEventLog = *events.NewTypedEventLog(original.EncodedEventLog, func(e *evtv1.Event) (events.EncodedRecord, error) {
				if r := e.GetImportProgressed(); r != nil && r.OperationIndex == cut {
					return events.EncodedRecord{}, injected
				}
				data, err := proto.Marshal(e)
				return events.EncodedRecord{ID: e.Id, Data: data}, err
			}, func(data []byte) (*evtv1.Event, error) {
				e := &evtv1.Event{}
				err := proto.Unmarshal(data, e)
				return e, err
			})
			startCoreServices(t, c)
			ctx := testContext(t)
			a := archiveWithEmails(t, []string{"alice@example.com"}, []string{"bob@example.com"})
			_, err = c.ImportArchive(ctx, a, ImportNaming{}, false, true)
			require.ErrorIs(t, err, injected)
			receipts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.ImportAggregate().AllEventsFilter())
			require.NoError(t, err)
			require.Len(t, receipts, int(cut))
			// A new core cold-replays EVT; it has no import memory from c.
			restarted, err := NewChattoCore(ctx, newReplicaConn(t, ns), testCoreConfig())
			require.NoError(t, err)
			startCoreServices(t, restarted)
			result, err := restarted.ImportArchive(ctx, a, ImportNaming{}, false, true)
			require.NoError(t, err)
			require.True(t, result.Complete)
			users, err := restarted.ListUsers(ctx)
			require.NoError(t, err)
			require.Len(t, users, 2)
			posts, _, err := restarted.EventPublisher.SubjectEvents(ctx, "evt.room.*.message_posted")
			require.NoError(t, err)
			require.Len(t, posts, 3)
			for _, post := range posts {
				require.NotNil(t, post.ImportContext)
			}
			before, err := restarted.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			_, err = restarted.ImportArchive(ctx, a, ImportNaming{}, false, true)
			require.NoError(t, err)
			after, err := restarted.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestImportArchiveConcurrentWriters(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	other := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	finished := make(chan error, 2)
	for _, writer := range []*ChattoCore{c, other} {
		a, err := interchange.ReadDirectory("../../../schema/interchange/v1/examples/conversation")
		require.NoError(t, err)
		go func() { _, err := writer.ImportArchive(ctx, a, ImportNaming{}, false, true); finished <- err }()
	}
	first, second := <-finished, <-finished
	if first != nil && second != nil {
		t.Fatalf("neither concurrent import completed: %v; %v", first, second)
	}
	a, err := interchange.ReadDirectory("../../../schema/interchange/v1/examples/conversation")
	require.NoError(t, err)
	_, err = c.ImportArchive(ctx, a, ImportNaming{}, false, true)
	require.NoError(t, err)
	users, err := c.ListUsers(ctx)
	require.NoError(t, err)
	require.Len(t, users, 2)
	posts, _, err := c.EventPublisher.SubjectEvents(ctx, "evt.room.*.message_posted")
	require.NoError(t, err)
	require.Len(t, posts, 3)
}
