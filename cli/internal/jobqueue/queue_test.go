package jobqueue_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/jobqueue"
	"hmans.de/chatto/internal/testutil"
)

func queueFixture(t *testing.T, age time.Duration) (context.Context, jetstream.JetStream, *jobqueue.Queue) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	q, err := jobqueue.New(ctx, js, age, 1)
	require.NoError(t, err)
	return ctx, js, q
}
func consumer(t *testing.T, ctx context.Context, q *jobqueue.Queue, kind string) jetstream.Consumer {
	t.Helper()
	subject, err := jobqueue.Subject(kind)
	require.NoError(t, err)
	c, err := q.Stream().CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Durable: "worker-" + kind, FilterSubject: subject, AckPolicy: jetstream.AckExplicitPolicy, DeliverPolicy: jetstream.DeliverAllPolicy})
	require.NoError(t, err)
	return c
}

func TestQueueIsolatesJobTypesAndScopesDeduplication(t *testing.T) {
	ctx, js, q := queueFixture(t, time.Hour)
	// A second instance opens the same queue and publishes without creating
	// another stream. Equal IDs in distinct job types must remain distinct.
	replica, err := jobqueue.New(ctx, js, time.Hour, 1)
	require.NoError(t, err)
	mail := consumer(t, ctx, q, "mail")
	webhook := consumer(t, ctx, replica, "webhook")
	require.NoError(t, q.Enqueue(ctx, "mail", "same-id", []byte("mail payload")))
	require.NoError(t, replica.Enqueue(ctx, "webhook", "same-id", []byte("webhook payload")))
	require.NoError(t, replica.Enqueue(ctx, "mail", "same-id", []byte("duplicate mail")))
	info, err := q.Stream().Info(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(2), info.State.Msgs)
	require.Equal(t, int64(-1), info.Config.MaxBytes)
	require.Equal(t, int64(-1), info.Config.MaxMsgs)
	message, err := mail.Next(jetstream.FetchContext(ctx))
	require.NoError(t, err)
	require.Equal(t, "mail payload", string(message.Data()))
	require.NoError(t, message.DoubleAck(ctx))
	// Acknowledging mail leaves the webhook available for its own worker.
	info, err = q.Stream().Info(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(1), info.State.Msgs)
	message, err = webhook.Next(jetstream.FetchContext(ctx))
	require.NoError(t, err)
	require.Equal(t, "webhook payload", string(message.Data()))
	require.NoError(t, message.DoubleAck(ctx))
	info, err = q.Stream().Info(ctx)
	require.NoError(t, err)
	require.Zero(t, info.State.Msgs)
}

func TestQueueRetiresUntouchedAndUnacknowledgedJobs(t *testing.T) {
	ctx, _, q := queueFixture(t, 300*time.Millisecond)
	worker := consumer(t, ctx, q, "picked")
	require.NoError(t, q.Enqueue(ctx, "untouched", "one", []byte("no worker")))
	require.NoError(t, q.Enqueue(ctx, "picked", "two", []byte("not acknowledged")))
	_, err := worker.Next(jetstream.FetchContext(ctx))
	require.NoError(t, err)
	// Neither application acknowledgement nor failure logging is needed for
	// hard retention to remove a job that outlives the configured age.
	require.Eventually(t, func() bool {
		info, err := q.Stream().Info(ctx)
		return err == nil && info.State.Msgs == 0
	}, 5*time.Second, 10*time.Millisecond)
}

func TestQueueRejectsInvalidRouting(t *testing.T) {
	ctx, js, q := queueFixture(t, time.Hour)
	for _, kind := range []string{"", "mail.*", "mail.>", ".mail", "mail..send", "mail send"} {
		require.Error(t, q.Enqueue(ctx, kind, "id", nil))
	}
	require.Error(t, q.Enqueue(ctx, "mail", "", nil))
	require.Error(t, q.Enqueue(ctx, "mail", "injected\r\nheader", nil))
	_, err := jobqueue.New(ctx, js, 0, 1)
	require.Error(t, err)
	info, err := q.Stream().Info(ctx)
	require.NoError(t, err)
	require.Zero(t, info.State.Msgs)
}
