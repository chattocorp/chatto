package kms

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

func setupBuiltinKMS(t *testing.T) (*Builtin, context.Context) {
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
	return NewBuiltin(kv, nil), ctx
}

func TestBuiltinWrapUnwrapAndShred(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)

	keyRef, err := k.CreateKey(ctx, "U1")
	require.NoError(t, err)
	require.NotEmpty(t, keyRef)
	require.NotEqual(t, LegacyUserKeyRef("U1"), keyRef)

	entry, err := k.kv.Get(ctx, keyRef)
	require.NoError(t, err)
	var stored corev1.UserKeyEncryptionKey
	require.NoError(t, proto.Unmarshal(entry.Value(), &stored))
	require.Equal(t, AlgorithmBuiltinXChaCha20Poly1305V1, stored.GetAlgorithm())
	require.Len(t, stored.GetKey(), encryption.KeySize)

	exists, err := k.KeyExists(ctx, keyRef)
	require.NoError(t, err)
	require.True(t, exists)

	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	wrapped, err := k.WrapContentKey(ctx, keyRef, contentKey, []byte("user=U1\x00epoch=1"))
	require.NoError(t, err)
	require.Equal(t, AlgorithmBuiltinXChaCha20Poly1305V1, wrapped.Algorithm)
	require.NotEmpty(t, wrapped.EncryptedContentKey)
	require.Len(t, wrapped.Nonce, encryption.XNonceSize)

	unwrapped, err := k.UnwrapContentKey(ctx, keyRef, *wrapped, []byte("user=U1\x00epoch=1"))
	require.NoError(t, err)
	require.Equal(t, contentKey, unwrapped)

	require.NoError(t, k.ShredKey(ctx, keyRef))
	exists, err = k.KeyExists(ctx, keyRef)
	require.NoError(t, err)
	require.False(t, exists)
	_, err = k.UnwrapContentKey(ctx, keyRef, *wrapped, []byte("user=U1\x00epoch=1"))
	require.ErrorIs(t, err, encryption.ErrKeyNotFound)
}

func TestBuiltinReadsLegacyRawKEK(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	for _, keyRef := range []string{LegacyUserKeyRef("U1"), "kek.legacyRaw"} {
		_, err = k.kv.Create(ctx, keyRef, key)
		require.NoError(t, err)

		contentKey, err := encryption.GenerateKey()
		require.NoError(t, err)
		wrapped, err := k.WrapContentKey(ctx, keyRef, contentKey, []byte("aad"))
		require.NoError(t, err)

		unwrapped, err := k.UnwrapContentKey(ctx, keyRef, *wrapped, []byte("aad"))
		require.NoError(t, err)
		require.Equal(t, contentKey, unwrapped)
	}
}

func TestBuiltinRejectsMalformedUserKeyEncryptionKey(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	keyRef := "kek.malformed"
	data, err := proto.Marshal(&corev1.UserKeyEncryptionKey{
		Key:       []byte("too-short"),
		Algorithm: AlgorithmBuiltinXChaCha20Poly1305V1,
	})
	require.NoError(t, err)
	_, err = k.kv.Create(ctx, keyRef, data)
	require.NoError(t, err)

	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	_, err = k.WrapContentKey(ctx, keyRef, contentKey, []byte("aad"))
	require.ErrorContains(t, err, "invalid key-encryption-key length")
}

func TestBuiltinRejectsUnsupportedWrappingAlgorithm(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	keyRef, err := k.CreateKey(ctx, "U1")
	require.NoError(t, err)

	_, err = k.UnwrapContentKey(ctx, keyRef, WrappedContentKey{
		Algorithm: "external-kms-v9",
	}, []byte("aad"))
	require.ErrorIs(t, err, ErrUnsupportedWrappingAlgorithm)
}

func TestBuiltinRejectsWrongPrefixRefs(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)

	exists, err := k.KeyExists(ctx, "dek.content")
	require.ErrorIs(t, err, ErrInvalidKeyRef)
	require.False(t, exists)

	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	_, err = k.WrapContentKey(ctx, "dek.content", contentKey, []byte("aad"))
	require.ErrorIs(t, err, ErrInvalidKeyRef)

	_, err = k.UnwrapContentKey(ctx, "dek.content", WrappedContentKey{}, []byte("aad"))
	require.ErrorIs(t, err, ErrInvalidKeyRef)

	require.ErrorIs(t, k.ShredKey(ctx, "dek.content"), ErrInvalidKeyRef)
	require.ErrorIs(t, k.ShredKey(ctx, "other.content"), ErrInvalidKeyRef)
}

func TestBuiltinRejectsUserProtobufRecord(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	data, err := proto.Marshal(&corev1.UserKeyEncryptionKey{
		Key:       []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		Algorithm: AlgorithmBuiltinXChaCha20Poly1305V1,
	})
	require.NoError(t, err)
	_, err = k.kv.Create(ctx, LegacyUserKeyRef("U1"), data)
	require.NoError(t, err)

	_, err = k.LegacyUserKey(ctx, "U1")
	require.ErrorContains(t, err, "invalid legacy user key record")
}
