package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/kms"
	runtimestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
)

func atprotoSigningKeyPath(origin string) string {
	return fmt.Sprintf("atproto_signing_key.%x", sha256.Sum256([]byte(origin)))
}

// ATProtoSigningKey opens the signing key shared by all providers and replicas
// for a canonical HTTPS origin. Only an absent record permits generation.
func (c *ChattoCore) ATProtoSigningKey(ctx context.Context, origin string) (*atcrypto.PrivateKeyP256, error) {
	path := atprotoSigningKeyPath(origin)
	kv := c.storage.runtimeStateKV
	entry, err := kv.Get(ctx, path)
	if err == nil {
		return c.unwrapATProtoSigningKey(ctx, path, entry.Value())
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, err
	}
	private, err := atcrypto.GeneratePrivateKeyP256()
	if err != nil {
		return nil, err
	}
	wrapper := c.encryption.keyWrapper
	ref, err := wrapper.CreateKey(ctx, "atproto")
	if err != nil {
		return nil, err
	}
	raw := private.Bytes()
	sealed, err := wrapper.WrapContentKey(ctx, ref, raw, []byte(path))
	clear(raw)
	if err != nil {
		return nil, errors.Join(err, wrapper.ShredKey(ctx, ref))
	}
	record := &runtimestatev1.ATProtoSigningKey{
		WrappingKeyRef: ref, WrappingAlgorithm: sealed.Algorithm, WrappingMetadata: sealed.Metadata,
		Nonce: sealed.Nonce, Ciphertext: sealed.EncryptedContentKey,
	}
	data, err := proto.Marshal(record)
	if err != nil {
		return nil, errors.Join(err, wrapper.ShredKey(ctx, ref))
	}
	_, err = kv.Create(ctx, path, data)
	if err == nil {
		return private, nil
	}
	if !jetstreamutil.IsSequenceConflict(err) {
		// A timeout can follow a committed write. Preserve its wrapping key.
		return nil, err
	}
	// Confirm which record committed before removing an unused candidate KEK.
	entry, err = kv.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(entry.Value(), data) {
		return private, nil
	}
	if err := wrapper.ShredKey(ctx, ref); err != nil {
		return nil, err
	}
	return c.unwrapATProtoSigningKey(ctx, path, entry.Value())
}

func (c *ChattoCore) unwrapATProtoSigningKey(ctx context.Context, path string, data []byte) (*atcrypto.PrivateKeyP256, error) {
	var record runtimestatev1.ATProtoSigningKey
	if err := proto.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record.WrappingAlgorithm == "" {
		return nil, errors.New("missing AT Protocol key wrapping algorithm")
	}
	raw, err := c.encryption.keyWrapper.UnwrapContentKey(ctx, record.WrappingKeyRef, kms.WrappedContentKey{
		EncryptedContentKey: record.Ciphertext, Nonce: record.Nonce,
		Algorithm: record.WrappingAlgorithm, Metadata: record.WrappingMetadata,
	}, []byte(path))
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	return atcrypto.ParsePrivateBytesP256(raw)
}
