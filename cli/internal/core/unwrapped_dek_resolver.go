package core

import (
	"context"
	"fmt"
	"sync"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/kms"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type unwrappedDEKCacheKey struct {
	userID        string
	purpose       corev1.UserDEKPurpose
	epoch         int32
	contentKeyRef string
}

type unwrappedDEKRequestCacheKey struct{}

type unwrappedDEKRequestCache struct {
	mu              sync.Mutex
	values          map[unwrappedDEKCacheKey][]byte
	inFlight        map[unwrappedDEKCacheKey]*unwrappedDEKRequestCacheCall
	userGenerations map[string]uint64
}

type unwrappedDEKRequestCacheCall struct {
	done           chan struct{}
	userGeneration uint64
	key            []byte
	err            error
}

// WithDEKRequestCache returns a child context that caches unwrapped DEKs for
// one request or batch operation. The cache is intentionally not process-wide:
// once a DEK record has been physically shredded from the shared store, later
// requests fail closed instead of using a stale in-memory key.
func WithDEKRequestCache(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := dekRequestCache(ctx); ok {
		return ctx
	}
	return context.WithValue(ctx, unwrappedDEKRequestCacheKey{}, &unwrappedDEKRequestCache{
		values:          make(map[unwrappedDEKCacheKey][]byte),
		inFlight:        make(map[unwrappedDEKCacheKey]*unwrappedDEKRequestCacheCall),
		userGenerations: make(map[string]uint64),
	})
}

func dekRequestCache(ctx context.Context) (*unwrappedDEKRequestCache, bool) {
	if ctx == nil {
		return nil, false
	}
	cache, ok := ctx.Value(unwrappedDEKRequestCacheKey{}).(*unwrappedDEKRequestCache)
	return cache, ok && cache != nil
}

func forgetDEKRequestCacheUser(ctx context.Context, userID string) {
	cache, ok := dekRequestCache(ctx)
	if !ok || userID == "" {
		return
	}
	cache.forgetUser(userID)
}

// unwrappedDEKResolver unwraps user DEKs from the shared DEK store. It owns no
// long-lived plaintext key cache; callers that hydrate multiple bodies in one
// request can opt into WithDEKRequestCache for request-scoped reuse.
type unwrappedDEKResolver struct {
	keyWrapper kms.KeyWrapper
	dekStore   dekstore.Reader
}

// newUnwrappedDEKResolver creates a resolver backed by the configured KMS
// wrapper and Chatto-owned DEK record store.
func newUnwrappedDEKResolver(keyWrapper kms.KeyWrapper, dekStore dekstore.Reader) *unwrappedDEKResolver {
	return &unwrappedDEKResolver{
		keyWrapper: keyWrapper,
		dekStore:   dekStore,
	}
}

// Resolve returns a cloned unwrapped DEK for the supplied persisted DEK event.
// The requested purpose is checked against the event purpose, while legacy
// unspecified-purpose DEKs remain accepted for historical payloads. Cache hits
// are limited to an optional request cache carried by ctx.
func (r *unwrappedDEKResolver) Resolve(ctx context.Context, event *corev1.UserDEKGeneratedEvent, purpose corev1.UserDEKPurpose) (*userDEK, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if event == nil {
		return nil, fmt.Errorf("DEK event is nil")
	}
	userID := event.GetUserId()
	epoch := event.GetEpoch()
	contentKeyRef := event.GetContentKeyRef()
	if userID == "" || epoch <= 0 || contentKeyRef == "" {
		return nil, fmt.Errorf("invalid DEK event")
	}
	eventPurpose := event.GetPurpose()
	if eventPurpose != corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED && purpose != corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED && eventPurpose != purpose {
		return nil, fmt.Errorf("DEK purpose mismatch: event has %s, want %s", eventPurpose.String(), purpose.String())
	}
	if r == nil || r.keyWrapper == nil || r.dekStore == nil {
		return nil, encryption.ErrKeyNotFound
	}

	cacheKey := unwrappedDEKCacheKey{
		userID:        userID,
		purpose:       eventPurpose,
		epoch:         epoch,
		contentKeyRef: contentKeyRef,
	}
	if requestCache, ok := dekRequestCache(ctx); ok {
		return requestCache.resolve(ctx, cacheKey, func() ([]byte, error) {
			return r.unwrap(ctx, event, eventPurpose)
		})
	}

	key, err := r.unwrap(ctx, event, eventPurpose)
	if err != nil {
		return nil, err
	}
	return &userDEK{epoch: epoch, purpose: eventPurpose, key: append([]byte(nil), key...)}, nil
}

func (c *unwrappedDEKRequestCache) resolve(ctx context.Context, cacheKey unwrappedDEKCacheKey, unwrap func() ([]byte, error)) (*userDEK, error) {
	c.mu.Lock()
	value := c.values[cacheKey]
	if len(value) != 0 {
		c.mu.Unlock()
		return cachedUserDEK(cacheKey, value), nil
	}
	if call := c.inFlight[cacheKey]; call != nil {
		done := call.done
		c.mu.Unlock()
		select {
		case <-done:
			if call.err != nil {
				return nil, call.err
			}
			return cachedUserDEK(cacheKey, call.key), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := &unwrappedDEKRequestCacheCall{
		done:           make(chan struct{}),
		userGeneration: c.userGenerations[cacheKey.userID],
	}
	c.inFlight[cacheKey] = call
	c.mu.Unlock()

	key, err := unwrap()

	c.mu.Lock()
	if err == nil {
		if c.userGenerations[cacheKey.userID] == call.userGeneration {
			call.key = append([]byte(nil), key...)
			c.values[cacheKey] = append([]byte(nil), key...)
		} else {
			err = encryption.ErrKeyNotFound
		}
	}
	call.err = err
	if c.inFlight[cacheKey] == call {
		delete(c.inFlight, cacheKey)
	}
	close(call.done)
	c.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return cachedUserDEK(cacheKey, key), nil
}

func (c *unwrappedDEKRequestCache) forgetUser(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userGenerations[userID]++
	for key := range c.values {
		if key.userID == userID {
			delete(c.values, key)
		}
	}
}

func cachedUserDEK(cacheKey unwrappedDEKCacheKey, key []byte) *userDEK {
	return &userDEK{epoch: cacheKey.epoch, purpose: cacheKey.purpose, key: append([]byte(nil), key...)}
}

func (r *unwrappedDEKResolver) unwrap(ctx context.Context, event *corev1.UserDEKGeneratedEvent, eventPurpose corev1.UserDEKPurpose) ([]byte, error) {
	stored, err := r.dekStore.Get(ctx, event.GetContentKeyRef())
	if err != nil {
		return nil, fmt.Errorf("failed to load DEK: %w", err)
	}
	keyRef := stored.WrappingKeyRef
	if keyRef == "" {
		keyRef = kms.LegacyUserKeyRef(event.GetUserId())
	}
	key, err := r.keyWrapper.UnwrapContentKey(ctx, keyRef, kms.WrappedContentKey{
		EncryptedContentKey: stored.EncryptedContentKey,
		Nonce:               stored.ContentKeyNonce,
		Algorithm:           stored.WrappingAlgorithm,
		Metadata:            stored.WrappingMetadata,
	}, userDEKAAD(event.GetUserId(), eventPurpose, event.GetEpoch()))
	if err != nil {
		return nil, fmt.Errorf("failed to unwrap DEK: %w", err)
	}
	return key, nil
}
