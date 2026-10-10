package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	runtimestatev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
)

func setupMessageRetry(t *testing.T) (*ChattoCore, *nats.Conn, MessagePostInput) {
	t.Helper()
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "retry-user", "Retry User", "password123")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, user.Id, KindChannel, "", "retry-room", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	return c, nc, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "one intended send", IdempotencyKey: uuid.NewString()}
}

func TestMessagePostIdempotencyConcurrentReplicas(t *testing.T) {
	t.Parallel()
	c, nc, input := setupMessageRetry(t)
	ctx := testContext(t)
	asset := uploadRoomAttachmentForUser(t, c, ctx, input.ActorID, input.RoomID, "retry.png")
	input.AttachmentAssetIDs = []string{asset.Id}
	input.CreateThread = true
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	start := make(chan struct{})
	type result struct {
		post *MessagePostResult
		err  error
	}
	results := make(chan result, 12)
	var wg sync.WaitGroup
	for index := range 12 {
		wg.Go(func() {
			<-start
			posting := []*ChattoCore{c, replica}[index%2]
			post, err := posting.Messages().PostMessage(ctx, input)
			results <- result{post, err}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	var id string
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.post)
		if id == "" {
			id = result.post.Event.Id
		}
		require.Equal(t, id, result.post.Event.Id)
	}
	posts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posts, 1)
	threads, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(evtstream.EventThreadCreated))
	require.NoError(t, err)
	require.Len(t, threads, 1)
	attached, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.AssetAggregate(asset.Id).Subject(evtstream.EventAssetAttached))
	require.NoError(t, err)
	require.Len(t, attached, 1)
	data, err := proto.Marshal(posts[0])
	require.NoError(t, err)
	require.NotContains(t, string(data), input.IdempotencyKey)
}

func TestMessagePostIdempotencyMismatchAndAccountScope(t *testing.T) {
	t.Parallel()
	c, _, input := setupMessageRetry(t)
	ctx := testContext(t)
	first, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	for _, change := range []func(*MessagePostInput){
		func(in *MessagePostInput) { in.Body = "different" },
		func(in *MessagePostInput) { in.RoomID = "missing-room" },
		func(in *MessagePostInput) { in.CreateThread = true },
		func(in *MessagePostInput) { in.LinkPreviewToken = "changed-token" },
	} {
		changed := input
		change(&changed)
		_, err := c.Messages().PostMessage(ctx, changed)
		require.ErrorIs(t, err, ErrMessageIdempotencyConflict)
	}
	input.IdempotencyKey = strings.ToUpper(input.IdempotencyKey)
	replay, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.Event.Id, replay.Event.Id)
	other, err := c.CreateUser(ctx, SystemActorID, "other-retry-user", "Other", "password123")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, other.Id, KindChannel, other.Id, input.RoomID)
	require.NoError(t, err)
	input.ActorID = other.Id
	otherPost, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, first.Event.Id, otherPost.Event.Id)
}

func TestMessagePostIdempotencyReservedIdentityAndLostResponse(t *testing.T) {
	t.Parallel()
	c, nc, input := setupMessageRetry(t)
	ctx := testContext(t)
	claim, err := c.Messages().claimMessagePost(ctx, input)
	require.NoError(t, err)
	// A new core cold-replays EVT and reads the reservation left by an attempt
	// which stopped after KV Create, before any message event was appended.
	recovered, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, recovered)
	posted, err := recovered.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, claim.messageID, posted.Event.Id)
	// Discard that successful result, as a transport disconnect would do.
	restarted, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, restarted)
	replay, err := restarted.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.True(t, proto.Equal(posted.Event, replay.Event))
	posts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posts, 1)
}

func TestMessagePostIdempotencyOCCRechecksBeforeWritePolicy(t *testing.T) {
	t.Parallel()
	c, nc, input := setupMessageRetry(t)
	ctx := testContext(t)
	claim, err := c.Messages().claimMessagePost(ctx, input)
	require.NoError(t, err)
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	attempts := 0
	posted, err := c.PostMessage(ctx, KindChannel, input.RoomID, input.ActorID, input.Body, nil, "", "", nil, false,
		func(options *postMessageOptions) { options.claim = claim },
		withPostMessageCommitAuthorization(func(ctx context.Context, threadID string) error {
			_, err := c.Messages().AuthorizePost(ctx, authorizationInputForPost(input, threadID))
			return err
		}),
		withPostMessageAttemptPrepared(func(ctx context.Context) error {
			attempts++
			if attempts != 1 {
				return nil
			}
			if _, err := replica.Messages().PostMessage(ctx, input); err != nil {
				return err
			}
			return replica.DenyUserRoomPermission(ctx, SystemActorID, input.RoomID, input.ActorID, PermMessagePost)
		}))
	require.NoError(t, err)
	require.Equal(t, claim.messageID, posted.Id)
	require.Equal(t, 1, attempts, "the OCC retry must return before preparing another write")
	posts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posts, 1)
}

func TestMessagePostIdempotencyExpiredMappingCanBeClaimedAgain(t *testing.T) {
	t.Parallel()
	c, _, input := setupMessageRetry(t)
	ctx := testContext(t)
	first, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	key, err := messagePostClaimKey(input.ActorID, input.IdempotencyKey)
	require.NoError(t, err)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	require.NoError(t, err)
	// Shorten only this fixture's TTL to exercise the real broker expiry and
	// marker path. The production 30-minute header is checked separately.
	_, err = c.storage.runtimeStateKV.UpdateWithTTL(ctx, key, entry.Value(), entry.Revision(), time.Second)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := c.storage.runtimeStateKV.Get(ctx, key)
		return errors.Is(err, jetstream.ErrKeyNotFound)
	}, 5*time.Second, 25*time.Millisecond)
	second, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, first.Event.Id, second.Event.Id)
}

func TestMessagePostIdempotencyCurrentReadAuthorityAndRetraction(t *testing.T) {
	t.Parallel()
	c, _, input := setupMessageRetry(t)
	ctx := testContext(t)
	first, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, input.RoomID, input.ActorID, PermMessagePost))
	replay, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err, "replay must not rerun new-write permission checks")
	require.Equal(t, first.Event.Id, replay.Event.Id)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, input.RoomID, input.ActorID, PermMessageRead))
	_, err = c.Messages().PostMessage(ctx, input)
	var applied *MessagePostAppliedError
	require.ErrorAs(t, err, &applied)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, input.RoomID, input.ActorID, PermMessageRead))
	require.NoError(t, c.Messages().DeleteMessage(ctx, MessageDeleteInput{ActorID: input.ActorID, RoomID: input.RoomID, EventID: first.Event.Id}))
	replay, err = c.Messages().PostMessage(ctx, input)
	if err != nil {
		require.ErrorAs(t, err, &applied)
	} else {
		require.Equal(t, first.Event.Id, replay.Event.Id)
	}
	posts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posts, 1)
}

func TestMessagePostIdempotencyFixedTTLAndNoKey(t *testing.T) {
	t.Parallel()
	c, _, input := setupMessageRetry(t)
	ctx := testContext(t)
	first, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	key, err := messagePostClaimKey(input.ActorID, input.IdempotencyKey)
	require.NoError(t, err)
	before, err := c.storage.runtimeStateKV.Get(ctx, key)
	require.NoError(t, err)
	var claim runtimestatev1.MessagePostClaim
	require.NoError(t, proto.Unmarshal(before.Value(), &claim))
	require.Equal(t, first.Event.Id, claim.MessageId)
	require.Len(t, claim.RequestFingerprint, 32)
	_, err = c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	after, err := c.storage.runtimeStateKV.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, before.Revision(), after.Revision())
	js, err := jetstream.New(c.nc)
	require.NoError(t, err)
	stream, err := js.Stream(ctx, "KV_RUNTIME_STATE")
	require.NoError(t, err)
	stored, err := stream.GetLastMsgForSubject(ctx, "$KV.RUNTIME_STATE."+key)
	require.NoError(t, err)
	require.Equal(t, MessageIdempotencyWindow.String(), stored.Header.Get(jetstream.MsgTTLHeader))
	input.IdempotencyKey = ""
	for range 2 {
		posted, err := c.Messages().PostMessage(ctx, input)
		require.NoError(t, err)
		require.NotEqual(t, first.Event.Id, posted.Event.Id)
	}
	input.IdempotencyKey = "not-a-uuid"
	_, err = c.Messages().PostMessage(ctx, input)
	require.ErrorIs(t, err, ErrInvalidArgument)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	input.IdempotencyKey = uuid.NewString()
	_, err = c.Messages().PostMessage(canceled, input)
	require.Error(t, err)
}

func TestMessagePostIdempotencyRetryAfterCanceledCommit(t *testing.T) {
	t.Parallel()
	c, nc, input := setupMessageRetry(t)
	ctx := testContext(t)
	claim, err := c.Messages().claimMessagePost(ctx, input)
	require.NoError(t, err)
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err = c.PostMessage(attemptCtx, KindChannel, input.RoomID, input.ActorID, input.Body, nil, "", "", nil, false,
		func(options *postMessageOptions) { options.claim = claim },
		withPostMessageAttemptPrepared(func(context.Context) error {
			_, err := replica.Messages().PostMessage(ctx, input)
			cancel() // No response can reach this caller after the other replica commits.
			return err
		}))
	require.ErrorIs(t, err, context.Canceled)
	replay, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, claim.messageID, replay.Event.Id)
	posts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posts, 1)
}

// A conflicting in-flight Create can fail without leaving any stored claim.
type contestedMessageClaimKV struct {
	jetstream.KeyValue
	contested bool
}

func (kv *contestedMessageClaimKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	if strings.HasPrefix(key, messagePostClaimPrefix) && !kv.contested {
		kv.contested = true
		return 0, jetstream.ErrKeyExists
	}
	return kv.KeyValue.Create(ctx, key, value, opts...)
}

func TestMessagePostIdempotencyCreateConflictIsNotCommitEvidence(t *testing.T) {
	t.Parallel()
	c, _, input := setupMessageRetry(t)
	ctx := testContext(t)
	contested := &contestedMessageClaimKV{KeyValue: c.storage.runtimeStateKV.KeyValue}
	c.storage.runtimeStateKV = bindTestKeyValue(t, c.js, contested)
	posted, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.True(t, contested.contested)
	require.NotEmpty(t, posted.Event.Id)
}

func TestMessagePostIdempotencyPreviewExpiryAndNormalizedIntent(t *testing.T) {
	t.Parallel()
	c, _, input := setupMessageRetry(t)
	ctx := testContext(t)
	url := "https://example.test/retry-preview"
	require.NoError(t, c.linkPreviewCache.Set(ctx, url, &evtv1.LinkPreview{Url: url, Title: "Preview"}))
	token, err := c.CreateLinkPreviewToken(ctx, url)
	require.NoError(t, err)
	input.LinkPreviewToken = token
	first, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.NoError(t, c.storage.runtimeStateKV.Delete(ctx, c.linkPreviewTokenKey(token)))
	input.VideoProcessingAssetIDs = []string{}
	replay, err := c.Messages().PostMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first.Event.Id, replay.Event.Id, "a completed retry must not resolve an expired preview token")
}
