package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	atprotov1 "hmans.de/chatto/internal/pb/chatto/atproto/v1"
)

// AT Protocol OAuth flow state lives in the MEMORY_CACHE KV bucket under two
// key prefixes:
//
//   atproto.auth_request.{state}          - in-flight PAR state, 10-min TTL
//   atproto.session.{sha256(did+sid)}     - post-callback session, 10-min TTL
//
// In the current sign-in flow, session entries are written and deleted within
// milliseconds (the callback handler revokes immediately once the user's DID
// is known). The store nevertheless exists to make sign-in survive a server
// multi-replica deployment where the callback may land on a different replica
// than the one that started the flow. See ADR-036 (runtime-state boundary) and
// FDR-029 for context.
//
// Session entries contain plaintext access tokens, refresh tokens, and a
// DPoP private key. This matches the in-memory exposure of the previous
// MemStore-based implementation. Any future feature that retains sessions
// beyond identification would need to wrap these via the KMS service — see
// the ATProtoSession proto for the secret-field call-out.

const (
	atprotoAuthRequestKeyPrefix = "atproto.auth_request."
	atprotoSessionKeyPrefix     = "atproto.session."

	// atprotoAuthRequestTTL bounds how long a user can sit on the consent
	// screen before the PAR record we stashed for them gets garbage-collected.
	// 10 minutes is generous compared to the 5-minute hard cap on Chatto's
	// own authorization codes (auth_codes.go); the ATProto flow includes a
	// human-mediated approval that can pause indefinitely, so we want more
	// slack here.
	atprotoAuthRequestTTL = 10 * time.Minute

	// atprotoSessionTTL bounds the exposure window for ATProto access tokens,
	// refresh tokens, and DPoP key material if a callback crashes before the
	// handler can revoke and delete the session.
	atprotoSessionTTL = 10 * time.Minute
)

// atprotoSessionKey hashes the DID + session ID into a NATS-clean key. DIDs
// contain colons (`did:plc:…`) which aren't valid in NATS subjects.
func atprotoSessionKey(did, sessionID string) string {
	h := sha256.Sum256([]byte(did + ":" + sessionID))
	return atprotoSessionKeyPrefix + hex.EncodeToString(h[:])
}

// SaveATProtoAuthRequest persists the state of an in-flight ATProto OAuth
// flow. The `state` token is a fresh random nonce, so we use Create (which
// also accepts the per-key TTL); collision is a programmer error.
func (c *ChattoCore) SaveATProtoAuthRequest(ctx context.Context, req *atprotov1.ATProtoAuthRequest) error {
	if req == nil || req.State == "" {
		return errors.New("ATProto auth request: state is required")
	}

	bytes, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal ATProto auth request: %w", err)
	}

	if _, err := c.storage.memoryCacheKV.Create(ctx, atprotoAuthRequestKeyPrefix+req.State, bytes, jetstream.KeyTTL(atprotoAuthRequestTTL)); err != nil {
		return fmt.Errorf("store ATProto auth request: %w", err)
	}
	return nil
}

// GetATProtoAuthRequest looks up an in-flight ATProto OAuth flow by its
// `state` token. Returns ErrNotFound if the entry is missing (typically
// because the TTL elapsed before the callback arrived).
func (c *ChattoCore) GetATProtoAuthRequest(ctx context.Context, state string) (*atprotov1.ATProtoAuthRequest, error) {
	entry, err := c.storage.memoryCacheKV.Get(ctx, atprotoAuthRequestKeyPrefix+state)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get ATProto auth request: %w", err)
	}

	var req atprotov1.ATProtoAuthRequest
	if err := proto.Unmarshal(entry.Value(), &req); err != nil {
		return nil, fmt.Errorf("unmarshal ATProto auth request: %w", err)
	}
	return &req, nil
}

// DeleteATProtoAuthRequest removes an in-flight ATProto OAuth flow's state.
// Idempotent — succeeds if the entry is already gone.
func (c *ChattoCore) DeleteATProtoAuthRequest(ctx context.Context, state string) error {
	if err := c.storage.memoryCacheKV.Delete(ctx, atprotoAuthRequestKeyPrefix+state); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("delete ATProto auth request: %w", err)
	}
	return nil
}

// SaveATProtoSession persists a post-callback ATProto OAuth session. The
// callback handler revokes and deletes this immediately after reading the
// account email; the per-key TTL keeps crash leftovers short-lived.
func (c *ChattoCore) SaveATProtoSession(ctx context.Context, sess *atprotov1.ATProtoSession) error {
	if sess == nil || sess.AccountDid == "" || sess.SessionId == "" {
		return errors.New("ATProto session: account_did and session_id are required")
	}

	bytes, err := proto.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal ATProto session: %w", err)
	}

	key := atprotoSessionKey(sess.AccountDid, sess.SessionId)
	if _, err := c.storage.memoryCacheKV.Create(ctx, key, bytes, jetstream.KeyTTL(atprotoSessionTTL)); err != nil {
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("store ATProto session: %w", err)
		}
		entry, err := c.storage.memoryCacheKV.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get existing ATProto session for update: %w", err)
		}
		if _, err := c.updateMemoryCacheTokenTTL(ctx, key, bytes, entry.Revision(), atprotoSessionTTL); err != nil {
			return fmt.Errorf("update ATProto session: %w", err)
		}
	}
	return nil
}

// GetATProtoSession retrieves a session by DID + session ID. Returns
// ErrNotFound if no entry exists for that pair.
func (c *ChattoCore) GetATProtoSession(ctx context.Context, did, sessionID string) (*atprotov1.ATProtoSession, error) {
	entry, err := c.storage.memoryCacheKV.Get(ctx, atprotoSessionKey(did, sessionID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get ATProto session: %w", err)
	}

	var sess atprotov1.ATProtoSession
	if err := proto.Unmarshal(entry.Value(), &sess); err != nil {
		return nil, fmt.Errorf("unmarshal ATProto session: %w", err)
	}
	return &sess, nil
}

// DeleteATProtoSession removes a session. Idempotent.
func (c *ChattoCore) DeleteATProtoSession(ctx context.Context, did, sessionID string) error {
	if err := c.storage.memoryCacheKV.Delete(ctx, atprotoSessionKey(did, sessionID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("delete ATProto session: %w", err)
	}
	return nil
}

func (c *ChattoCore) updateMemoryCacheTokenTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error) {
	msg := nats.NewMsg("$KV.MEMORY_CACHE." + key)
	msg.Data = value
	ack, err := c.js.PublishMsg(ctx, msg,
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(ttl),
	)
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}
