package dekstore

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func setupStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "TEST_ENCRYPTION_KEYS",
		History: 1,
	})
	require.NoError(t, err)
	return New(kv, nil), ctx
}

func TestStoreCreateGetAndShred(t *testing.T) {
	store, ctx := setupStore(t)

	stored := &corev1.StoredUserDEK{
		EncryptedContentKey: []byte("wrapped"),
		ContentKeyNonce:     []byte("nonce"),
		WrappingAlgorithm:   "test-wrap",
		WrappingKeyRef:      "kek.test",
	}
	ref, err := store.Create(ctx, stored)
	require.NoError(t, err)
	require.Contains(t, ref, "dek.")

	loaded, err := store.Get(ctx, ref)
	require.NoError(t, err)
	require.True(t, proto.Equal(stored, loaded))

	require.NoError(t, store.Shred(ctx, ref))
	_, err = store.Get(ctx, ref)
	require.ErrorIs(t, err, encryption.ErrKeyNotFound)
}
