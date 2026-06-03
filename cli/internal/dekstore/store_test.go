package dekstore

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/kms"
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

	stored := &corev1.UserDataEncryptionKey{
		EncryptedContentKey: []byte("wrapped"),
		ContentKeyNonce:     []byte("nonce"),
		WrappingAlgorithm:   kms.AlgorithmBuiltinXChaCha20Poly1305V1,
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

func TestStoreRejectsWrongPrefixRefs(t *testing.T) {
	store, ctx := setupStore(t)

	_, err := store.Get(ctx, "kek.test")
	require.ErrorIs(t, err, ErrInvalidRef)
	require.ErrorIs(t, store.Shred(ctx, "kek.test"), ErrInvalidRef)
	require.ErrorIs(t, store.Shred(ctx, "user.test"), ErrInvalidRef)
}

func TestStoreRejectsMalformedRecords(t *testing.T) {
	store, ctx := setupStore(t)
	data, err := proto.Marshal(&corev1.UserDataEncryptionKey{
		EncryptedContentKey: []byte("wrapped"),
		WrappingAlgorithm:   kms.AlgorithmBuiltinXChaCha20Poly1305V1,
		WrappingKeyRef:      "kek.test",
	})
	require.NoError(t, err)
	_, err = store.kv.Create(ctx, "dek.bad", data)
	require.NoError(t, err)

	_, err = store.Get(ctx, "dek.bad")
	require.ErrorContains(t, err, "content key nonce is empty")

	_, err = store.Create(ctx, &corev1.UserDataEncryptionKey{
		EncryptedContentKey: []byte("wrapped"),
		ContentKeyNonce:     []byte("nonce"),
		WrappingAlgorithm:   "unsupported",
		WrappingKeyRef:      "kek.test",
	})
	require.Error(t, err)
}
