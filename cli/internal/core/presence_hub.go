package core

import (
	"context"
	"fmt"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// PresenceUpdate represents a raw presence change from the KV watcher.
// Subscribers perform their own filtering (space membership) and deduplication.
type PresenceUpdate struct {
	UserID string
	Status string // PresenceStatusOnline, PresenceStatusAway, etc., or PresenceStatusOffline for delete
}

// PresenceSubscription represents a subscriber to the PresenceHub.
type PresenceSubscription struct {
	// C receives presence updates. Closed when Unsubscribe is called.
	C  <-chan PresenceUpdate
	ch chan PresenceUpdate // internal writable channel

	// Snapshot contains the current presence state at subscription time.
	// Use this to initialize deduplication maps.
	Snapshot map[string]string // userID -> status

	id uint64
}

// PresenceHub runs a single MEMORY_CACHE watcher on presence.> and fans out
// per-user presence updates. Each Chatto process has one PresenceHub instance,
// reducing KV watcher count from O(users × spaces) to 1 per process.
type PresenceHub struct {
	memoryCacheKV jetstream.KeyValue
	logger        *log.Logger

	mu          sync.Mutex
	subscribers map[uint64]*PresenceSubscription
	nextID      uint64
	snapshot    map[string]string // current presence state (built during init sync)
	ready       chan struct{}     // closed when initial sync is complete
	readyOnce   sync.Once         // ensures ready is closed exactly once
}

// NewPresenceHub creates a PresenceHub. Call Run() to start it.
func NewPresenceHub(memoryCacheKV jetstream.KeyValue, logger *log.Logger) *PresenceHub {
	return &PresenceHub{
		memoryCacheKV: memoryCacheKV,
		logger:        logger,
		subscribers:   make(map[uint64]*PresenceSubscription),
		snapshot:      make(map[string]string),
		ready:         make(chan struct{}),
	}
}

// Run starts the KV watcher and fans out updates to subscribers.
// Blocks until ctx is cancelled. Should be started in an errgroup.
func (h *PresenceHub) Run(ctx context.Context) error {
	watcher, err := h.memoryCacheKV.Watch(ctx, "presence.>")
	if err != nil {
		return fmt.Errorf("presence hub: failed to create watcher: %w", err)
	}
	defer watcher.Stop()

	h.logger.Debug("Presence hub started")
	defer h.logger.Debug("Presence hub stopped")

	syncComplete := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case entry := <-watcher.Updates():
			if entry == nil {
				// Initial sync complete (may fire more than once on watcher reconnect)
				syncComplete = true
				h.readyOnce.Do(func() { close(h.ready) })
				h.logger.Debug("Presence hub initial sync complete", "entries", len(h.snapshot))
				continue
			}

			userID, ok := parsePresenceKey(entry.Key())
			if !ok {
				continue
			}

			var status string

			if entry.Operation() == jetstream.KeyValueDelete ||
				entry.Operation() == jetstream.KeyValuePurge {
				status = PresenceStatusOffline
			} else {
				var presence corev1.UserPresence
				if err := proto.Unmarshal(entry.Value(), &presence); err != nil {
					h.logger.Warn("Presence hub: failed to unmarshal", "error", err, "user_id", userID)
					continue
				}
				status = presenceStatusToString(presence.Status)
			}

			h.mu.Lock()

			previous, hadPrevious := h.snapshot[userID]
			if status == PresenceStatusOffline {
				delete(h.snapshot, userID)
			} else {
				h.snapshot[userID] = status
			}

			// Only fan out after initial sync is complete, and only when the
			// per-user status actually changed.
			changed := previous != status
			if status == PresenceStatusOffline && !hadPrevious {
				changed = false
			}
			if syncComplete && changed {
				update := PresenceUpdate{UserID: userID, Status: status}
				for _, sub := range h.subscribers {
					select {
					case sub.ch <- update:
					default:
						// Slow consumer — drop update. Presence refreshes every 30s
						// so the subscriber will self-correct on the next cycle.
					}
				}
			}

			h.mu.Unlock()
		}
	}
}

// Subscribe registers a new subscriber. The returned PresenceSubscription
// contains a channel for receiving updates and a snapshot of current presence
// state for initializing deduplication maps.
//
// The subscription is registered atomically with the snapshot copy, so no
// events can be missed between reading the snapshot and starting to receive
// from the channel.
//
// The caller must call Unsubscribe() when done.
func (h *PresenceHub) Subscribe(ctx context.Context) (*PresenceSubscription, error) {
	// Wait for initial sync to complete
	select {
	case <-h.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	ch := make(chan PresenceUpdate, 64)

	h.mu.Lock()
	id := h.nextID
	h.nextID++

	// Copy snapshot under the same lock that registers the subscriber —
	// this ensures no events are missed between snapshot and channel registration.
	snapshot := make(map[string]string, len(h.snapshot))
	for k, v := range h.snapshot {
		snapshot[k] = v
	}

	sub := &PresenceSubscription{
		C:        ch,
		ch:       ch,
		Snapshot: snapshot,
		id:       id,
	}
	h.subscribers[id] = sub
	h.mu.Unlock()

	return sub, nil
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *PresenceHub) Unsubscribe(sub *PresenceSubscription) {
	h.mu.Lock()
	delete(h.subscribers, sub.id)
	h.mu.Unlock()
	// Close channel after removing from map to prevent sends to closed channel
	close(sub.ch)
}
