package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/authctx"
)

// credentialChanges watches the two runtime credential families once per
// process. It retains only listeners for connected credentials, not session
// values or secrets. Notifications are hints: callers must validate the exact
// credential again before changing authority.
type credentialChanges struct {
	core      *ChattoCore
	mu        sync.Mutex
	listeners map[string]map[chan struct{}]struct{}
	ready     chan struct{}
}

func newCredentialChanges(c *ChattoCore) *credentialChanges {
	return &credentialChanges{core: c, listeners: make(map[string]map[chan struct{}]struct{}), ready: make(chan struct{})}
}

func (w *credentialChanges) run(ctx context.Context) error {
	watcher, err := watchKeyFilters(ctx, w.core.storage.runtimeStateKV, []string{authTokenKeyPrefix + "*", renewableSessionKeyPrefix + "*"}, jetstream.MetaOnly())
	if err != nil {
		return fmt.Errorf("watch runtime credential changes: %w", err)
	}
	defer watcher.Stop()
	ready := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry, ok := <-watcher.Updates():
			if !ok {
				return fmt.Errorf("runtime credential watcher stopped")
			}
			if entry == nil {
				if !ready {
					close(w.ready)
					ready = true
				}
				continue
			}
			w.mu.Lock()
			for changed := range w.listeners[entry.Key()] {
				select {
				case changed <- struct{}{}:
				default:
				}
			}
			w.mu.Unlock()
		}
	}
}

// WatchRuntimeCredentialChanges registers for changes to one cookie session
// or bearer credential and its renewable session. There are no per-socket KV
// watchers. Register before revalidating the credential to close the read/watch
// gap. The caller must stop the listener when its connection ends.
func (c *ChattoCore) WatchRuntimeCredentialChanges(ctx context.Context, credential authctx.RuntimeCredential) (<-chan struct{}, func(), error) {
	if credential.Kind != authctx.RuntimeCredentialKindCookieSession && credential.Kind != authctx.RuntimeCredentialKindBearerToken {
		return nil, func() {}, nil
	}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-c.credentialChanges.ready:
	}
	keys := []string{c.authTokenKey(credential.Handle)}
	if credential.Kind == authctx.RuntimeCredentialKindBearerToken {
		validated, err := c.ValidatePublicBearerCredential(ctx, credential.Handle)
		if err != nil {
			return nil, nil, err
		}
		if validated.RenewableSessionID != "" {
			keys = append(keys, c.renewableSessionKey(validated.RenewableSessionID))
		}
	}
	changed := make(chan struct{}, 1)
	w := c.credentialChanges
	w.mu.Lock()
	for _, key := range keys {
		if w.listeners[key] == nil {
			w.listeners[key] = make(map[chan struct{}]struct{})
		}
		w.listeners[key][changed] = struct{}{}
	}
	w.mu.Unlock()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			for _, key := range keys {
				delete(w.listeners[key], changed)
				if len(w.listeners[key]) == 0 {
					delete(w.listeners, key)
				}
			}
		})
	}
	return changed, stop, nil
}
