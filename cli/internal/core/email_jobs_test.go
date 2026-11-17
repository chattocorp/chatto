package core

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/email"
	"github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

type retryMailSender struct {
	*email.MockSender
	calls atomic.Int32
}

func (s *retryMailSender) SendContext(ctx context.Context, msg email.Message) error {
	if s.calls.Add(1) == 1 {
		return errors.New("secret@example.invalid code 123456 SMTP reply")
	}
	return s.MockSender.SendContext(ctx, msg)
}

func TestEmailJobsPersistPrivateContentAndRetry(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	transport := &retryMailSender{MockSender: email.NewMockSender(true)}
	sender := c.NewEmailJobs(transport)
	msg := email.Message{To: "secret@example.invalid", Subject: "Private subject", Body: "code 123456", ExpiresAt: time.Now().Add(time.Minute)}
	require.NoError(t, sender.SendContext(ctx, msg))
	require.Zero(t, transport.calls.Load(), "request must not call transport")
	info, err := c.storage.jobsStream.Info(ctx)
	require.NoError(t, err)
	raw, err := c.storage.jobsStream.GetMsg(ctx, info.State.LastSeq)
	require.NoError(t, err)
	id := strings.Split(raw.Subject, ".")[2]
	for _, secret := range []string{msg.To, msg.Subject, msg.Body} {
		require.NotContains(t, string(raw.Data), secret)
	}
	entry, err := c.storage.runtimeStateKV.Get(ctx, emailDeliveryKey(id))
	require.NoError(t, err)
	for _, secret := range []string{msg.To, msg.Subject, msg.Body} {
		require.NotContains(t, string(entry.Value()), secret)
	}
	// A fresh core and sender can recover the accepted job without request memory.
	replica, err := NewChattoCore(ctx, c.nc, testCoreConfig())
	require.NoError(t, err)
	recovered := replica.NewEmailJobs(transport)
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- replica.jobs.Work(workCtx, replica.jobConsumers[jobTypeEmail], recovered.deliver, 2) }()
	defer func() { cancel(); <-done }()
	require.Eventually(t, func() bool { return len(transport.Messages()) == 1 }, 10*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(2), transport.calls.Load())
	require.Eventually(t, func() bool {
		_, err := c.storage.runtimeStateKV.Get(ctx, emailDeliveryKey(id))
		return errors.Is(err, jetstream.ErrKeyNotFound)
	}, time.Second, time.Millisecond)
	require.NoError(t, recovered.deliver(ctx, jobs.Job{ID: id, ExpiresAt: msg.ExpiresAt}))
	require.Equal(t, int32(2), transport.calls.Load(), "ack replay must not resend removed content")
	info, err = c.storage.jobsStream.Info(ctx)
	require.NoError(t, err)
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		fact, err := c.storage.jobsStream.GetMsg(ctx, seq)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue
		}
		require.NoError(t, err)
		require.NotContains(t, string(fact.Data), msg.To)
		require.NotContains(t, string(fact.Data), "123456")
	}
}

func TestEmailJobsRejectExpiredMessages(t *testing.T) {
	c, _ := newTestCore(t)
	transport := email.NewMockSender(true)
	sender := c.NewEmailJobs(transport)
	require.Error(t, sender.SendContext(testContext(t), email.Message{ExpiresAt: time.Now().Add(-time.Second)}))
	require.Empty(t, transport.Messages())
}

func TestEmailJobsFailClosedForExpiredOrCorruptContent(t *testing.T) {
	for _, scenario := range []string{"expired", "corrupt"} {
		t.Run(scenario, func(t *testing.T) {
			c, _ := newTestCore(t)
			ctx := testContext(t)
			transport := email.NewMockSender(true)
			sender := c.NewEmailJobs(transport)
			expiresAt := time.Now().Add(time.Minute)
			require.NoError(t, sender.SendContext(ctx, email.Message{To: "private@example.invalid", Body: "secret code", ExpiresAt: expiresAt}))
			info, err := c.storage.jobsStream.Info(ctx)
			require.NoError(t, err)
			fact, err := c.storage.jobsStream.GetMsg(ctx, info.State.LastSeq)
			require.NoError(t, err)
			id := strings.Split(fact.Subject, ".")[2]
			if scenario == "corrupt" {
				_, err = c.storage.runtimeStateKV.Put(ctx, emailDeliveryKey(id), []byte("corrupt"))
				require.NoError(t, err)
			} else {
				expiresAt = time.Now().Add(-time.Second)
			}
			err = sender.deliver(ctx, jobs.Job{ID: id, ExpiresAt: expiresAt})
			require.Error(t, err)
			require.Empty(t, transport.Messages())
			require.NotContains(t, err.Error(), "private@example.invalid")
		})
	}
}
