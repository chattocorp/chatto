package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// Stop at the first access-record deletion, after the renewable authority
// has been removed. Only the crashing worker uses this KV handle.
type blockedCredentialDelete struct {
	jetstream.KeyValue
	reached chan struct{}
}

func (kv *blockedCredentialDelete) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if strings.HasPrefix(key, authTokenKeyPrefix) {
		close(kv.reached)
		<-ctx.Done()
		return ctx.Err()
	}
	return kv.KeyValue.Delete(ctx, key, opts...)
}

func TestRuntimeCredentialCleanupAfterMidAttemptCrash(t *testing.T) {
	ctx := testContext(t)
	ns, conn := testutil.StartNATS(t)
	writer, err := NewChattoCore(ctx, conn, testCoreConfig())
	require.NoError(t, err)
	survivor, err := NewChattoCore(ctx, newReplicaConn(t, ns), testCoreConfig())
	require.NoError(t, err)
	stopWriter := testutil.StartService(t, "writer", writer.Run, writer.WaitForBoot)
	name := JobsConsumerName(jobTypeRuntimeCredentialCleanup)
	stream := writer.storage.jobsStream
	_, err = stream.PauseConsumer(ctx, name, time.Now().Add(time.Minute))
	require.NoError(t, err)
	issue := func(login string) (string, []string) {
		t.Helper()
		user, err := writer.CreateUser(ctx, SystemActorID, login, login, "password123")
		require.NoError(t, err)
		cookie, _, err := writer.CreateCookieSession(ctx, user.Id, "password_login")
		require.NoError(t, err)
		bearer, err := writer.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
		require.NoError(t, err)
		id, _, ok := writer.parseRefreshToken(bearer.RefreshToken)
		require.True(t, ok)
		return user.Id, []string{writer.renewableSessionKey(id), writer.authTokenKey(bearer.AccessToken), writer.authTokenKey(cookie)}
	}
	target, removed := issue("cleanup-target")
	_, kept := issue("cleanup-survivor")
	kv := survivor.storage.runtimeStateKV
	var revisions []uint64
	for _, key := range kept {
		entry, err := kv.Get(ctx, key)
		require.NoError(t, err)
		revisions = append(revisions, entry.Revision())
	}
	require.NoError(t, writer.DeleteUser(ctx, SystemActorID, target))
	require.ErrorIs(t, writer.RequireAuthenticationAllowed(ctx, target, 0), ErrAuthenticationRevoked)
	require.Eventually(t, func() bool { return len(jobsOfType(t, ctx, writer, jobTypeRuntimeCredentialCleanup)) == 1 }, 3*time.Second, 10*time.Millisecond)
	for _, key := range removed {
		_, err := kv.Get(ctx, key)
		require.NoError(t, err)
	}
	// Run the accepted job in isolation so the only way it can move to the
	// survivor is a broker redelivery after the first connection is lost.
	stopWriter()
	gate := &blockedCredentialDelete{writer.storage.runtimeStateKV.KeyValue, make(chan struct{})}
	writer.storage.runtimeStateKV = bindTestKeyValue(t, writer.js, gate)
	cfg := JobsConsumerConfig(writer.jobs, jobTypeRuntimeCredentialCleanup)
	cfg.AckWait = time.Second
	consumer, err := stream.CreateOrUpdateConsumer(ctx, cfg)
	require.NoError(t, err)
	_, err = stream.ResumeConsumer(ctx, name)
	require.NoError(t, err)
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- writer.jobs.Work(workCtx, consumer, writer.effectJobs[jobTypeRuntimeCredentialCleanup].Handle, 1)
	}()
	t.Cleanup(func() { cancel(); require.NoError(t, <-done) })
	select {
	case <-gate.reached:
	case <-ctx.Done():
		t.Fatal("first worker did not reach access records")
	}
	_, err = kv.Get(ctx, removed[0])
	require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
	for _, key := range removed[1:] {
		_, err := kv.Get(ctx, key)
		require.NoError(t, err)
	}
	conn.Close()
	cancel()
	startCoreServices(t, survivor)
	require.Eventually(t, func() bool {
		jobs := jobsOfType(t, ctx, survivor, jobTypeRuntimeCredentialCleanup)
		return len(jobs) == 1 && jobs[0].Status == jobqueue.Succeeded && jobs[0].Attempts == 2
	}, 5*time.Second, 10*time.Millisecond)
	for _, key := range removed {
		_, err := kv.Get(ctx, key)
		require.ErrorIs(t, err, jetstream.ErrKeyNotFound)
	}
	for i, key := range kept {
		entry, err := kv.Get(ctx, key)
		require.NoError(t, err)
		require.Equal(t, revisions[i], entry.Revision())
	}
}

func TestRuntimeCredentialCleanupRejectsInvalidSourceAndIncompleteScan(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	require.Error(t, c.cleanDeletedAccountCredentials(ctx, pushSubscriptionCleanupDelivery(t, "one", "two")))
	c.storage.runtimeStateKV = bindTestKeyValue(t, c.js, &partialKeyListingKV{KeyValue: c.storage.runtimeStateKV, filter: authTokenKeyPrefix + "*"})
	require.ErrorContains(t, c.cleanDeletedAccountCredentials(ctx, pushSubscriptionCleanupDelivery(t, "one", "one")), "snapshot ended before completion")
}

// Recheck ownership on redelivery if the key changed after the snapshot.
func TestRuntimeCredentialCleanupPreservesReplacedRecord(t *testing.T) {
	c, _ := newTestCore(t)
	ctx := testContext(t)
	kv := c.storage.runtimeStateKV
	key := c.authTokenKey("test-credential")
	_, err := kv.Put(ctx, key, []byte(`{"user_id":"one"}`))
	require.NoError(t, err)
	replacement := []byte(`{"user_id":"two"}`)
	gate := &pushCleanupDeleteGate{KeyValue: kv, key: key, beforeDelete: func() {
		_, err := kv.Put(ctx, key, replacement)
		require.NoError(t, err)
	}}
	c.storage.runtimeStateKV = bindTestKeyValue(t, c.js, gate)
	fact := pushSubscriptionCleanupDelivery(t, "one", "one")
	require.True(t, jetstreamutil.IsSequenceConflict(c.cleanDeletedAccountCredentials(ctx, fact)))
	require.NoError(t, c.cleanDeletedAccountCredentials(ctx, fact))
	entry, err := kv.Get(ctx, key)
	require.NoError(t, err)
	require.Equal(t, replacement, entry.Value())
}
