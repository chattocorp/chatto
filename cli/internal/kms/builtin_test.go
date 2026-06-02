package kms

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/testutil"
)

func setupBuiltinKMS(t *testing.T) (*Builtin, context.Context) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "TEST_ENCRYPTION_KEYS",
	})
	require.NoError(t, err)
	return NewBuiltin(kv, nil), ctx
}

func TestBuiltinWrapUnwrapAndShred(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)

	require.NoError(t, k.CreateUserKey(ctx, "U1"))
	exists, err := k.UserKeyExists(ctx, "U1")
	require.NoError(t, err)
	require.True(t, exists)

	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	wrapped, err := k.WrapContentKey(ctx, "U1", contentKey, []byte("user=U1\x00epoch=1"))
	require.NoError(t, err)
	require.Equal(t, AlgorithmBuiltinXChaCha20Poly1305V1, wrapped.Algorithm)
	require.NotEmpty(t, wrapped.EncryptedContentKey)
	require.Len(t, wrapped.Nonce, encryption.XNonceSize)

	unwrapped, err := k.UnwrapContentKey(ctx, "U1", *wrapped, []byte("user=U1\x00epoch=1"))
	require.NoError(t, err)
	require.Equal(t, contentKey, unwrapped)

	require.NoError(t, k.ShredUserKey(ctx, "U1"))
	exists, err = k.UserKeyExists(ctx, "U1")
	require.NoError(t, err)
	require.False(t, exists)
	_, err = k.UnwrapContentKey(ctx, "U1", *wrapped, []byte("user=U1\x00epoch=1"))
	require.ErrorIs(t, err, encryption.ErrKeyNotFound)
}

func TestBuiltinRejectsUnsupportedWrappingAlgorithm(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	require.NoError(t, k.CreateUserKey(ctx, "U1"))

	_, err := k.UnwrapContentKey(ctx, "U1", WrappedContentKey{
		Algorithm: "external-kms-v9",
	}, []byte("aad"))
	require.ErrorIs(t, err, ErrUnsupportedWrappingAlgorithm)
}
