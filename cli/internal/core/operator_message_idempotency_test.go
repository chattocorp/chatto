package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

func TestHistoricalMessageIdempotencyConcurrentReplay(t *testing.T) {
	t.Parallel()
	c, nc, post := setupMessageRetry(t)
	ctx := testContext(t)
	asset := uploadRoomAttachmentForUser(t, c, ctx, post.ActorID, post.RoomID, "history.png")
	input := HistoricalMessageInput{RoomID: post.RoomID, AuthorID: post.ActorID,
		Body: "historical", CreatedAt: time.Date(2015, 8, 27, 12, 0, 0, 123456000, time.UTC),
		CreateThread: true, AttachmentAssetIDs: []string{asset.Id}, IdempotencyKey: uuid.NewString()}
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	type outcome struct {
		event *evtv1.Event
		err   error
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			event, err := []*ChattoCore{c, replica}[i%2].ImportHistoricalMessage(ctx, input)
			results <- outcome{event, err}
		})
	}
	wg.Wait()
	close(results)
	var id string
	for result := range results {
		require.NoError(t, result.err)
		if id == "" {
			id = result.event.Id
		}
		require.Equal(t, id, result.event.Id)
		require.Equal(t, input.CreatedAt, result.event.CreatedAt.AsTime())
		require.Equal(t, input.AuthorID, messageAuthorID(result.event))
	}
	for _, eventType := range []string{evtstream.EventMessagePosted, evtstream.EventThreadCreated} {
		stored, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(input.RoomID).Subject(eventType))
		require.NoError(t, err)
		require.Len(t, stored, 1)
	}
	// A completed retry must not fail because the assets are now attached or
	// because the original post has since been retracted and its room archived.
	require.NoError(t, c.DeleteMessage(ctx, SystemActorID, KindChannel, input.RoomID, id))
	_, err = c.ArchiveRoom(ctx, SystemActorID, KindChannel, input.RoomID)
	require.NoError(t, err)
	restarted, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, restarted)
	replayed, err := restarted.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, id, replayed.Id)
}

func TestHistoricalMessageIdempotencyFingerprintAndReservation(t *testing.T) {
	t.Parallel()
	c, _, post := setupMessageRetry(t)
	ctx := testContext(t)
	input := HistoricalMessageInput{RoomID: post.RoomID, AuthorID: post.ActorID,
		Body: "source", CreatedAt: time.Now().UTC(), IdempotencyKey: uuid.NewString()}
	claim, err := c.claimHistoricalMessage(ctx, input, func(_ context.Context) error { return nil })
	require.NoError(t, err)
	first, err := c.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	require.Equal(t, claim.messageID, first.Id, "resume a reserved identity before its first write")
	for _, change := range []func(*HistoricalMessageInput){
		func(v *HistoricalMessageInput) { v.Body = "changed" },
		func(v *HistoricalMessageInput) { v.CreatedAt = v.CreatedAt.Add(time.Second) },
		func(v *HistoricalMessageInput) { v.EditedAt = &v.CreatedAt },
		func(v *HistoricalMessageInput) { v.CreateThread = true },
		func(v *HistoricalMessageInput) { v.RoomID = "different-room" },
		func(v *HistoricalMessageInput) { v.LinkPreview = &evtv1.LinkPreview{Url: "https://example.com"} },
	} {
		changed := input
		change(&changed)
		_, err := c.ImportHistoricalMessage(ctx, changed)
		require.ErrorIs(t, err, ErrMessageIdempotencyConflict)
	}
	input.IdempotencyKey = ""
	unkeyed, err := c.ImportHistoricalMessage(ctx, input)
	require.NoError(t, err)
	require.NotEqual(t, first.Id, unkeyed.Id)
	_, err = c.ImportHistoricalMessage(ctx, HistoricalMessageInput{IdempotencyKey: "invalid"})
	require.ErrorContains(t, err, "idempotency_key must")
}
