package kms

import (
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/key_material/v1"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/testutil"
)

// getOverrideKV injects read results while retaining real KV writes in lifecycle tests.
type getOverrideKV struct {
	jetstream.KeyValue
	get func(context.Context, string) (jetstream.KeyValueEntry, error)
}

func (kv getOverrideKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	return kv.get(ctx, key)
}

func TestBuiltinMissingKeyRetries(t *testing.T) {
	permanent := errors.New("read unavailable")
	for _, tc := range []struct {
		name     string
		results  []error
		wantErr  error
		wantWait time.Duration
	}{
		{"immediate success", []error{nil}, nil, 0},
		{"follower catches up", []error{jetstream.ErrKeyNotFound, fmt.Errorf("missing: %w", jetstream.ErrKeyNotFound), jetstream.ErrKeyNotFound, nil}, nil, 85 * time.Millisecond},
		{"exhausted", []error{jetstream.ErrKeyNotFound, jetstream.ErrKeyNotFound, jetstream.ErrKeyNotFound, jetstream.ErrKeyNotFound}, jetstream.ErrKeyNotFound, 85 * time.Millisecond},
		{"permanent error", []error{permanent}, permanent, 0},
		{"error after miss", []error{jetstream.ErrKeyNotFound, permanent}, permanent, 10 * time.Millisecond},
		{"deleted", []error{jetstream.ErrKeyDeleted}, jetstream.ErrKeyDeleted, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				calls := 0
				k := NewBuiltin(getOverrideKV{get: func(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
					require.Equal(t, "kek.test", key)
					require.Less(t, calls, len(tc.results))
					err := tc.results[calls]
					calls++
					return nil, err
				}}, nil)
				start := time.Now()
				_, err := k.getEntry(context.Background(), "kek.test")
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, len(tc.results), calls)
				require.Equal(t, tc.wantWait, time.Since(start))
			})
		})
	}
}

func TestBuiltinMissingKeyRetryCancellation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cancelAfter time.Duration
		deadline    bool
		wantCalls   int
	}{
		{"already canceled", 0, false, 0},
		{"canceled during wait", 5 * time.Millisecond, false, 1},
		{"deadline during wait", 20 * time.Millisecond, true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				wantErr := context.Canceled
				if tc.deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), tc.cancelAfter)
					wantErr = context.DeadlineExceeded
				} else if tc.cancelAfter == 0 {
					cancel()
				} else {
					time.AfterFunc(tc.cancelAfter, cancel)
				}
				defer cancel()
				calls := 0
				k := NewBuiltin(getOverrideKV{get: func(context.Context, string) (jetstream.KeyValueEntry, error) {
					calls++
					return nil, jetstream.ErrKeyNotFound
				}}, nil)
				_, err := k.getEntry(ctx, "kek.test")
				require.ErrorIs(t, err, wantErr)
				require.Equal(t, tc.wantCalls, calls)
			})
		})
	}
}

func TestBuiltinWrapAfterDelayedKeyVisibility(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	keyRef, err := k.CreateKey(ctx, "U1")
	require.NoError(t, err)
	kv := k.kv
	calls := 0
	k.kv = getOverrideKV{KeyValue: kv, get: func(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
		calls++
		if calls <= 3 {
			return nil, jetstream.ErrKeyNotFound
		}
		return kv.Get(ctx, key)
	}}
	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	wrapped, err := k.WrapContentKey(ctx, keyRef, contentKey, []byte("aad"))
	require.NoError(t, err)
	require.Equal(t, 4, calls)
	unwrapped, err := k.UnwrapContentKey(ctx, keyRef, *wrapped, []byte("aad"))
	require.NoError(t, err)
	require.Equal(t, contentKey, unwrapped)
}

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
	var stored keymaterialv1.UserKeyEncryptionKey
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
	data, err := proto.Marshal(&keymaterialv1.UserKeyEncryptionKey{
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

func TestBuiltinCallKeyLifecycle(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)

	keyRef, encoded, err := k.CreateCallKey(ctx, "C123")
	require.NoError(t, err)
	require.Equal(t, CallKeyRef("C123"), keyRef)
	require.NotEmpty(t, encoded)

	got, err := k.GetCallKey(ctx, keyRef)
	require.NoError(t, err)
	require.Equal(t, encoded, got)

	exists, err := k.CallKeyExists(ctx, keyRef)
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, k.ShredCallKey(ctx, keyRef))
	exists, err = k.CallKeyExists(ctx, keyRef)
	require.NoError(t, err)
	require.False(t, exists)

	_, err = k.GetCallKey(ctx, keyRef)
	require.ErrorIs(t, err, encryption.ErrKeyNotFound)
}

func TestBuiltinCallKeysAreNotKEKRefs(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)

	keyRef, _, err := k.CreateCallKey(ctx, "C456")
	require.NoError(t, err)

	contentKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	_, err = k.WrapContentKey(ctx, keyRef, contentKey, []byte("aad"))
	require.ErrorIs(t, err, ErrInvalidKeyRef)

	_, err = k.UnwrapContentKey(ctx, keyRef, WrappedContentKey{}, []byte("aad"))
	require.ErrorIs(t, err, ErrInvalidKeyRef)

	require.ErrorIs(t, k.ShredCallKey(ctx, "kek.not-call"), ErrInvalidKeyRef)
}

func TestBuiltinRejectsUserProtobufRecord(t *testing.T) {
	k, ctx := setupBuiltinKMS(t)
	data, err := proto.Marshal(&keymaterialv1.UserKeyEncryptionKey{
		Key:       []byte("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"),
		Algorithm: AlgorithmBuiltinXChaCha20Poly1305V1,
	})
	require.NoError(t, err)
	_, err = k.kv.Create(ctx, LegacyUserKeyRef("U1"), data)
	require.NoError(t, err)

	_, err = k.LegacyUserKey(ctx, "U1")
	require.ErrorContains(t, err, "invalid legacy user key record")
}
