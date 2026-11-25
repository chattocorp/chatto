package core

import (
	"bytes"
	"sync"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	runtimestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
)

func TestATProtoSigningKeySharedByReplicas(t *testing.T) {
	ctx := t.Context()
	first, nc := newTestCore(t)
	second, err := NewChattoCore(ctx, nc, testCoreConfig())
	require.NoError(t, err)
	const origin = "https://chat.example"
	replicas := []*ChattoCore{first, second}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var keys [8]*atcrypto.PrivateKeyP256
	var errs [8]error
	for i := range keys {
		wg.Go(func() {
			<-start
			keys[i], errs[i] = replicas[i%len(replicas)].ATProtoSigningKey(ctx, origin)
		})
	}
	close(start)
	wg.Wait()
	for i := range keys {
		require.NoError(t, errs[i])
		require.True(t, keys[0].Equal(keys[i]))
	}
	other, err := second.ATProtoSigningKey(ctx, "https://another.example")
	require.NoError(t, err)
	require.False(t, keys[0].Equal(other))
}

func TestATProtoSigningKeyRestoreRequiresMatchingKEK(t *testing.T) {
	ctx := t.Context()
	source, _ := newTestCore(t)
	const origin = "https://chat.example"
	key, err := source.ATProtoSigningKey(ctx, origin)
	require.NoError(t, err)
	path := atprotoSigningKeyPath(origin)
	entry, err := source.storage.runtimeStateKV.Get(ctx, path)
	require.NoError(t, err)
	require.False(t, bytes.Contains(entry.Value(), key.Bytes()))
	var record runtimestatev1.ATProtoSigningKey
	require.NoError(t, proto.Unmarshal(entry.Value(), &record))
	kek, err := source.storage.encryptionKV.Get(ctx, record.WrappingKeyRef)
	require.NoError(t, err)

	restored, _ := newTestCore(t)
	revision, err := restored.storage.runtimeStateKV.Create(ctx, path, entry.Value())
	require.NoError(t, err)
	_, err = restored.ATProtoSigningKey(ctx, origin)
	require.Error(t, err)
	stored, err := restored.storage.runtimeStateKV.Get(ctx, path)
	require.NoError(t, err)
	require.Equal(t, revision, stored.Revision())
	_, err = restored.storage.encryptionKV.Create(ctx, record.WrappingKeyRef, kek.Value())
	require.NoError(t, err)
	opened, err := restored.ATProtoSigningKey(ctx, origin)
	require.NoError(t, err)
	require.True(t, opened.Equal(key))

	// A copied record must not authenticate on another origin, even with its KEK.
	const clone = "https://clone.example"
	_, err = restored.storage.runtimeStateKV.Create(ctx, atprotoSigningKeyPath(clone), entry.Value())
	require.NoError(t, err)
	_, err = restored.ATProtoSigningKey(ctx, clone)
	require.Error(t, err)
}
