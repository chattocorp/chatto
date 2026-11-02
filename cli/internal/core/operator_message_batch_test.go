package core

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

type historicalBatchRetryContextKey struct{}

// Advance the real room tail between each failed preparation and its retry
// decision. Only the marked import call is intercepted; background readers
// and fixture setup use the original stream behavior.
type historicalBatchAdvancingStream struct {
	jetstream.Stream
	filter  string
	reads   int
	advance func(context.Context) error
}

func (s *historicalBatchAdvancingStream) GetLastMsgForSubject(ctx context.Context, subject string) (*jetstream.RawStreamMsg, error) {
	if ctx.Value(historicalBatchRetryContextKey{}) != nil && subject == s.filter {
		s.reads++
		if s.reads%2 == 0 {
			if err := s.advance(ctx); err != nil {
				return nil, err
			}
		}
	}
	return s.Stream.GetLastMsgForSubject(ctx, subject)
}

func TestHistoricalMessageBatchExhaustionPreservesError(t *testing.T) {
	t.Parallel()
	c, _ := newTestCore(t)
	original := c.EventPublisher
	stream := &historicalBatchAdvancingStream{Stream: c.storage.serverEvtStream}
	c.EventPublisher = evtstream.NewPublisher(c.js, stream, testCoreLogger())
	startCoreServices(t, c)
	ctx := testContext(t)
	_, room, _ := setupReactionTest(t, c, ctx)
	agg := evtstream.RoomAggregate(room.Id)
	stream.filter = agg.AllEventsFilter()
	stream.advance = func(ctx context.Context) error {
		e := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RoomUpdated{RoomUpdated: &evtv1.RoomUpdatedEvent{RoomId: room.Id, Name: room.Name, Description: "concurrent update"}}})
		_, err := original.AppendEventually(ctx, agg.SubjectFor(e), e)
		return err
	}
	_, err := c.ImportHistoricalMessages(context.WithValue(ctx, historicalBatchRetryContextKey{}, true), []HistoricalMessageOperation{{Message: HistoricalMessageInput{RoomID: room.Id, AuthorID: "missing", CreatedAt: time.Date(2020, 3, 14, 0, 0, 0, 0, time.UTC), Body: "body", OperationKey: uuid.NewString()}}})
	require.Equal(t, 16, stream.reads, "all eight attempts must observe concurrent room writes")
	require.ErrorIs(t, err, ErrNotFound, "retry exhaustion must retain the missing-author error")
}

func TestHistoricalMessageBatchReferencesAndRetries(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, c, ctx)
	created := time.Date(2020, 3, 14, 12, 0, 0, 0, time.UTC)
	root := HistoricalMessageOperation{Message: HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: created, Body: "root body", OperationKey: uuid.NewString(), SourceActorID: user.Id, EditedAt: new(created.Add(time.Minute)), LinkPreview: &evtv1.LinkPreview{Url: "https://example.invalid/source", Title: "source"}}}
	reply := HistoricalMessageOperation{Message: HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: created.Add(time.Hour), Deleted: true, OperationKey: uuid.NewString()}, ThreadRootOperation: root.Message.OperationKey, ReplyToOperation: root.Message.OperationKey,
		Reactions: []HistoricalMessageChange{{OperationKey: uuid.NewString(), UserID: user.Id, Emoji: "❤️", Present: true}}}
	other := HistoricalMessageOperation{Message: HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: created.Add(2 * time.Hour), Body: "second reply", OperationKey: uuid.NewString()}, ThreadRootOperation: root.Message.OperationKey, ReplyToOperation: reply.Message.OperationKey}
	input := []HistoricalMessageOperation{root, reply, other}
	posts, err := c.ImportHistoricalMessages(ctx, input)
	require.NoError(t, err)
	require.Len(t, posts, 3)
	require.Equal(t, posts[0].Id, posts[1].GetMessagePosted().GetInThread())
	require.Equal(t, posts[1].Id, posts[2].GetMessagePosted().GetInReplyTo())
	require.Equal(t, 1, c.GetThreadMetadata(posts[0].Id).ReplyCount, "the tombstone is excluded from the visible reply count")
	body, err := c.GetFullMessageBody(ctx, posts[0].Id)
	require.NoError(t, err)
	require.Equal(t, root.Message.Body, body.Body)
	require.Equal(t, *root.Message.EditedAt, *body.UpdatedAt)
	require.Equal(t, "source", body.LinkPreview.Title)
	reactions, err := c.GetReactions(ctx, posts[1].Id)
	require.NoError(t, err)
	require.Equal(t, []ReactionSummary{{Emoji: "heart", UserIDs: []string{user.Id}}}, reactions)
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	again, err := c.ImportHistoricalMessages(ctx, input)
	require.NoError(t, err)
	for i := range posts {
		require.Equal(t, posts[i].Id, again[i].Id)
		resolved := input[i].Message
		resolved.ThreadRootEventID = posts[i].GetMessagePosted().GetInThread()
		resolved.InReplyTo = posts[i].GetMessagePosted().GetInReplyTo()
		one, err := c.ImportHistoricalMessage(ctx, resolved)
		require.NoError(t, err)
		require.Equal(t, posts[i].Id, one.Id)
	}
	r := reply.Reactions[0]
	r.Kind, r.RoomID, r.MessageID = "reaction", room.Id, posts[1].Id
	require.NoError(t, c.ImportHistoricalMessageChange(ctx, r))
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).AllEventsFilter())
	require.NoError(t, err)
	threads, tombstones := 0, 0
	for _, fact := range facts {
		if fact.GetThreadCreated() != nil {
			threads++
		}
		if fact.GetMessageRetracted() != nil {
			tombstones++
			require.True(t, fact.GetImportContext().GetUnknownTime())
		}
		if fact.GetMessagePosted().GetHistoricalImport() || fact.GetReactionAdded().GetHistoricalImport() || fact.GetThreadCreated().GetHistoricalImport() || fact.GetMessageRetracted() != nil {
			require.False(t, isDeliverableLiveEVTRoomEvent(fact))
		}
	}
	require.Equal(t, 1, threads)
	require.Equal(t, 1, tombstones)
	// A subset can use destination IDs rather than local references.
	other.ThreadRootOperation, other.ReplyToOperation = "", ""
	other.Message.ThreadRootEventID, other.Message.InReplyTo = posts[0].Id, posts[1].Id
	_, err = c.ImportHistoricalMessages(ctx, []HistoricalMessageOperation{other})
	require.NoError(t, err)
}

func TestHistoricalMessageBatchRejectsWithoutPartialWrites(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, c, ctx)
	makeInput := func() []HistoricalMessageOperation {
		result := []HistoricalMessageOperation{}
		for range 2 {
			result = append(result, HistoricalMessageOperation{Message: HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: time.Date(2020, 3, 14, 0, 0, 0, 0, time.UTC), Body: "body", OperationKey: uuid.NewString()}})
		}
		return result
	}
	for _, tc := range []struct {
		name string
		edit func([]HistoricalMessageOperation)
	}{
		{"forward reference", func(in []HistoricalMessageOperation) { in[0].ThreadRootOperation = in[1].Message.OperationKey }},
		{"self reference", func(in []HistoricalMessageOperation) { in[0].ReplyToOperation = in[0].Message.OperationKey }},
		{"ambiguous reference", func(in []HistoricalMessageOperation) {
			in[1].ReplyToOperation = in[0].Message.OperationKey
			in[1].Message.InReplyTo = "another"
		}},
		{"duplicate key", func(in []HistoricalMessageOperation) { in[1].Message.OperationKey = in[0].Message.OperationKey }},
		{"bad key", func(in []HistoricalMessageOperation) { in[1].Message.OperationKey = "invalid" }},
		{"other room", func(in []HistoricalMessageOperation) { in[1].Message.RoomID = "missing" }},
		{"missing author", func(in []HistoricalMessageOperation) { in[1].Message.AuthorID = "missing" }},
		{"too long", func(in []HistoricalMessageOperation) {
			in[1].Message.Body = strings.Repeat("a", MaxMessageBodyLength+1)
		}},
		{"attachments", func(in []HistoricalMessageOperation) { in[1].Message.AttachmentAssetIDs = []string{"asset"} }},
		{"empty body", func(in []HistoricalMessageOperation) { in[1].Message.Body = "" }},
		{"deleted body", func(in []HistoricalMessageOperation) { in[1].Message.Deleted = true }},
		{"invalid reactor", func(in []HistoricalMessageOperation) {
			in[1].Reactions = []HistoricalMessageChange{{OperationKey: uuid.NewString(), UserID: "missing", Emoji: "heart", Present: true}}
		}},
		{"too many reactions", func(in []HistoricalMessageOperation) {
			in[1].Reactions = make([]HistoricalMessageChange, MaxHistoricalReactionChanges+1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := makeInput()
			tc.edit(input)
			before, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			_, err = c.ImportHistoricalMessages(ctx, input)
			require.Error(t, err)
			after, err := c.EventPublisher.LastStreamSeq(ctx)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
	_, err := c.ImportHistoricalMessages(ctx, nil)
	require.Error(t, err)
	_, err = c.ImportHistoricalMessages(ctx, make([]HistoricalMessageOperation, MaxHistoricalMessages+1))
	require.Error(t, err)
}

func TestHistoricalMessageBatchConcurrentReplicas(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	c := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	user, room, _ := setupReactionTest(t, c, ctx)
	replica := startReplica(t, ns, testCoreConfig())
	input := make([]HistoricalMessageOperation, MaxHistoricalMessages)
	for i := range input {
		input[i].Message = HistoricalMessageInput{RoomID: room.Id, AuthorID: user.Id, CreatedAt: time.Date(2020, 3, 14, 0, 0, i, 0, time.UTC), Body: "body", OperationKey: uuid.NewString()}
		if i > 0 {
			input[i].ThreadRootOperation = input[0].Message.OperationKey
		}
	}
	results := make([][]*evtv1.Event, 2)
	var wg sync.WaitGroup
	for i, writer := range []*ChattoCore{c, replica} {
		wg.Go(func() {
			var err error
			results[i], err = writer.ImportHistoricalMessages(ctx, input)
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	require.Len(t, results[0], len(input))
	require.Len(t, results[1], len(input))
	for i := range input {
		require.Equal(t, results[0][i].Id, results[1][i].Id)
	}
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, facts, MaxHistoricalMessages+1, "one setup post plus the batch")
	// A changed retry fails without committing a new earlier item.
	changed := input[1]
	changed.ThreadRootOperation = ""
	changed.Message.ThreadRootEventID = results[0][0].Id
	changed.Message.Body = "changed"
	fresh := input[0]
	fresh.Message.OperationKey = uuid.NewString()
	before, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	_, err = c.ImportHistoricalMessages(ctx, []HistoricalMessageOperation{fresh, changed})
	require.Error(t, err)
	after, err := c.EventPublisher.LastStreamSeq(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
