package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// Both replicas finish boot before any message exists. The surviving replica
// must use JOBS, even if the writer disappears before the first handoff.
func TestMessageBodyErasureAfterWriterCrash(t *testing.T) {
	ns, conn := testutil.StartNATS(t)
	ctx := testContext(t)
	writer, err := NewChattoCore(ctx, conn, testCoreConfig())
	require.NoError(t, err)
	stopWriter := testutil.StartService(t, "writer", writer.Run, writer.WaitForBoot)
	survivor := startReplica(t, ns, testCoreConfig())
	stream := survivor.storage.serverEvtStream
	_, err = stream.PauseConsumer(ctx, messageBodyErasureConsumerName, time.Now().Add(time.Minute))
	require.NoError(t, err)

	room, err := writer.CreateRoom(ctx, SystemActorID, KindChannel, "", "erasure", "")
	require.NoError(t, err)
	user, err := writer.CreateUser(ctx, SystemActorID, "erasure-user", "Erasure User", "password123")
	require.NoError(t, err)
	_, err = writer.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	message, err := writer.PostMessage(ctx, KindChannel, room.Id, user.Id, "A", nil, "", "", nil, false)
	require.NoError(t, err)
	for _, body := range []string{"B", "C"} {
		require.NoError(t, writer.EditMessage(ctx, user.Id, KindChannel, room.Id, message.Id, body))
	}
	seqs, current, ok := writer.roomModel.bodyEventSeqs(message.Id)
	require.True(t, ok)
	require.Len(t, seqs, 3)
	require.Equal(t, seqs[2], current)
	for _, seq := range seqs {
		_, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)
	}
	stopWriter()
	conn.Close()
	resumeConsumerAndWakePulls(t, ctx, stream, messageBodyErasureConsumerName)

	waitSucceeded := func(count int) {
		t.Helper()
		require.Eventually(t, func() bool {
			jobs := jobsOfType(t, ctx, survivor, jobTypeMessageBodyErasure)
			if len(jobs) != count {
				return false
			}
			for _, job := range jobs {
				if job.Status != jobqueue.Succeeded {
					return false
				}
			}
			return true
		}, 5*time.Second, 10*time.Millisecond)
	}
	waitSucceeded(2)
	for _, seq := range seqs[:2] {
		_, err := stream.GetMsg(ctx, seq)
		require.ErrorIs(t, err, jetstream.ErrMsgNotFound)
	}
	body, err := survivor.GetMessageBody(ctx, message.Id)
	require.NoError(t, err)
	require.Equal(t, "C", body)
	_, err = stream.GetMsg(ctx, current)
	require.NoError(t, err)
	require.NoError(t, survivor.DeleteMessage(ctx, user.Id, KindChannel, room.Id, message.Id))
	waitSucceeded(3)
	_, err = stream.GetMsg(ctx, current)
	require.ErrorIs(t, err, jetstream.ErrMsgNotFound)
}

// resumeConsumerAndWakePulls ends a consumer pause so that waiting pulls get
// the pending messages. nats-server does not wake waiting pulls when
// ResumeConsumer ends a pause early. They wait until their expiry, which is 30
// seconds for an idle durable worker. A pause deadline that ends by itself
// wakes them. The response reports Paused only when the server scheduled that
// wake-up, so the loop retries a deadline that passed before the server set it.
func resumeConsumerAndWakePulls(t *testing.T, ctx context.Context, stream jetstream.Stream, name string) {
	t.Helper()
	for {
		response, err := stream.PauseConsumer(ctx, name, time.Now().Add(50*time.Millisecond))
		require.NoError(t, err)
		if response.Paused {
			return
		}
	}
}

// failBodyDeletion leaves one physical body intact until storage recovers.
type failBodyDeletion struct {
	jetstream.Stream
	blocked uint64
}

func (s *failBodyDeletion) SecureDeleteMsg(ctx context.Context, seq uint64) error {
	if seq == s.blocked {
		return errors.New("temporary erasure failure")
	}
	return s.Stream.SecureDeleteMsg(ctx, seq)
}

func TestMessageBodyErasureRetriesPartialDeletion(t *testing.T) {
	core, _ := newTestCore(t)
	ctx := testContext(t)
	stream := &failBodyDeletion{Stream: core.storage.serverEvtStream}
	core.storage.serverEvtStream = stream
	subject := evtstream.RoomAggregate("R-erasure").Subject(evtstream.EventMessageBody)
	var seqs []uint64
	var tail uint64
	for range 2 {
		fact := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: "R-erasure", EventId: "M-erasure"}}})
		seq, err := core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: subject, Event: fact, Expect: events.ExpectFilterSeq(subject, tail)}})
		require.NoError(t, err)
		tail = seq[0]
		seqs = append(seqs, tail)
	}
	stream.blocked = seqs[1]
	require.Error(t, core.secureDeleteMessageBodyEvents(ctx, seqs))
	_, err := stream.GetMsg(ctx, seqs[0])
	require.ErrorIs(t, err, jetstream.ErrMsgNotFound)
	_, err = stream.GetMsg(ctx, seqs[1])
	require.NoError(t, err)
	stream.blocked = 0
	require.NoError(t, core.secureDeleteMessageBodyEvents(ctx, seqs))
	_, err = stream.GetMsg(ctx, seqs[1])
	require.ErrorIs(t, err, jetstream.ErrMsgNotFound)
}
