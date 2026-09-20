package core

import (
	"context"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/cache_state/v1"
	"sync"
	"sync/atomic"
	"time"

	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	runtimestatev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

// PresenceUpdate represents a deduplicated presence change from the KV watcher.
type PresenceUpdate struct {
	UserID string
	Status string // PresenceStatusOnline, PresenceStatusAway, etc., or PresenceStatusOffline for delete
}

// PresenceSubscription represents a subscriber to the PresenceHub.
type PresenceSubscription struct {
	// C receives presence updates. Closed when Unsubscribe is called.
	C  <-chan PresenceUpdate
	ch chan PresenceUpdate // internal writable channel
	// Done closes as soon as the subscription ends, even if C still contains
	// buffered updates. Callers should stop immediately and reconnect when
	// Lagged reports true.
	Done <-chan struct{}
	done chan struct{}

	id     uint64
	lagged atomic.Bool
}

// PresenceHub combines one liveness watcher in MEMORY_CACHE and one private
// choice watcher in RUNTIME_STATE. Each process has one hub, independent of
// the number of connected users and rooms.
type PresenceHub struct {
	// beforeLiveRead catches up private choices before processing shared liveness.
	// It runs outside mu and reads the current runtime choice.
	beforeLiveRead func(context.Context, string) error
	memoryCacheKV  jetstream.KeyValue
	runtimeStateKV jetstream.KeyValue
	logger         *log.Logger

	mu                      sync.Mutex
	subscribers             map[uint64]*PresenceSubscription
	nextID                  uint64
	snapshot                map[string]string // current presence state (built during init sync)
	live                    map[string]string // heartbeat state, never exposed without the saved choice
	preferences             map[string]*apiv1.PresencePreference
	preferenceRevisions     map[string]uint64 // prevents delayed reads from replacing newer KV values
	preferenceWatchRevision uint64            // advanced only by the ordered preference watcher
	preferenceChanged       chan struct{}     // wakes current-state barriers
	ready                   chan struct{}     // closed when initial sync is complete
	readyOnce               sync.Once         // ensures ready is closed exactly once
	resyncRequests          chan chan error
}

// NewPresenceHub creates a PresenceHub. Call Run() to start it.
func NewPresenceHub(memoryCacheKV, runtimeStateKV jetstream.KeyValue, logger *log.Logger) *PresenceHub {
	return &PresenceHub{
		memoryCacheKV:       memoryCacheKV,
		runtimeStateKV:      runtimeStateKV,
		logger:              logger,
		subscribers:         make(map[uint64]*PresenceSubscription),
		snapshot:            make(map[string]string),
		live:                make(map[string]string),
		preferences:         make(map[string]*apiv1.PresencePreference),
		preferenceRevisions: make(map[string]uint64),
		preferenceChanged:   make(chan struct{}),
		ready:               make(chan struct{}),
		resyncRequests:      make(chan chan error),
	}
}

// applyPreference installs only newer current-state reads. No presence history
// is retained; public transitions contain only the effective status.
func (h *PresenceHub) applyPreference(userID string, preference *apiv1.PresencePreference, revision uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if revision <= h.preferenceRevisions[userID] {
		return
	}
	h.preferenceRevisions[userID] = revision
	if preference == nil {
		delete(h.preferences, userID)
		delete(h.live, userID)
	} else {
		h.preferences[userID] = proto.Clone(preference).(*apiv1.PresencePreference)
	}
	h.updatePublicLocked(userID, true)
}

func (h *PresenceHub) preference(userID string) *apiv1.PresencePreference {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p := h.preferences[userID]; p != nil {
		return proto.Clone(p).(*apiv1.PresencePreference)
	}
	return nil
}

// effectiveStatusLocked fails closed for unknown saved modes.
func (h *PresenceHub) effectiveStatusLocked(userID, live string) string {
	if live == "" || live == PresenceStatusOffline {
		return PresenceStatusOffline
	}
	if p := h.preferences[userID]; p != nil {
		switch p.Status {
		case apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE:
			return PresenceStatusOnline
		case apiv1.PresenceStatus_PRESENCE_STATUS_AWAY:
			return PresenceStatusAway
		case apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB:
			return PresenceStatusDoNotDisturb
		default:
			return PresenceStatusOffline
		}
	}
	return live
}

// GetUserPresences returns the current status for each requested user from the
// process-wide watcher snapshot. Missing and invalid users are reported as
// offline. The returned map is detached from the hub's internal state.
//
// This is intended for bulk read hydration. Mutation responses that require
// read-your-writes should continue to read the backing KV directly.
func (h *PresenceHub) GetUserPresences(ctx context.Context, userIDs []string) (map[string]string, error) {
	statuses := make(map[string]string, len(userIDs))
	if len(userIDs) == 0 {
		return statuses, nil
	}

	select {
	case <-h.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, userID := range userIDs {
		status := PresenceStatusOffline
		if validPresenceUserID(userID) {
			if current, ok := h.snapshot[userID]; ok {
				status = current
			}
		}
		statuses[userID] = status
	}
	return statuses, nil
}

// Run starts the KV watcher and fans out updates to subscribers.
// Blocks until ctx is cancelled. Should be started in an errgroup.
func (h *PresenceHub) Run(ctx context.Context) error {
	h.logger.Debug("Presence hub started")
	defer h.logger.Debug("Presence hub stopped")

	var pendingResync chan error
	for {
		watcher, err := h.memoryCacheKV.Watch(ctx, "presence.>")
		if err != nil {
			if pendingResync != nil {
				select {
				case <-ctx.Done():
					pendingResync <- ctx.Err()
					return ctx.Err()
				case <-time.After(natsRecoveryRetryWait):
					continue
				}
			}
			return fmt.Errorf("presence hub: failed to create watcher: %w", err)
		}

		preferences, err := h.runtimeStateKV.Watch(ctx, "presence.>")
		if err != nil {
			watcher.Stop()
			if pendingResync != nil {
				pendingResync <- err
			}
			return fmt.Errorf("presence hub: private choice watcher: %w", err)
		}
		syncComplete := false
		preferencesComplete := false
		complete := func() {
			if !syncComplete || !preferencesComplete {
				return
			}
			h.readyOnce.Do(func() { close(h.ready) })
			if pendingResync != nil {
				pendingResync <- nil
				pendingResync = nil
			}
		}
		restart := false
		for !restart {
			var resyncRequests <-chan chan error
			if pendingResync == nil {
				resyncRequests = h.resyncRequests
			}
			select {
			case <-ctx.Done():
				watcher.Stop()
				preferences.Stop()
				if pendingResync != nil {
					pendingResync <- ctx.Err()
				}
				return ctx.Err()
			case pendingResync = <-resyncRequests:
				h.mu.Lock()
				h.snapshot = make(map[string]string)
				h.live = make(map[string]string)
				h.preferences = make(map[string]*apiv1.PresencePreference)
				h.preferenceRevisions = make(map[string]uint64)
				h.preferenceWatchRevision = 0
				close(h.preferenceChanged)
				h.preferenceChanged = make(chan struct{})
				h.mu.Unlock()
				restart = true
			case entry, ok := <-preferences.Updates():
				if !ok {
					watcher.Stop()
					preferences.Stop()
					return fmt.Errorf("presence hub: private choice watcher stopped")
				}
				if entry == nil {
					preferencesComplete = true
					complete()
					continue
				}
				userID, valid := parsePresenceKey(entry.Key())
				if !valid {
					continue
				}
				var choice *apiv1.PresencePreference
				if entry.Operation() == jetstream.KeyValuePut {
					var value runtimestatev1.PresencePreference
					if err := proto.Unmarshal(entry.Value(), &value); err != nil {
						watcher.Stop()
						preferences.Stop()
						return fmt.Errorf("decode private presence choice: %w", err)
					}
					choice = &apiv1.PresencePreference{Status: value.Status, Revision: value.Revision}
				}
				h.applyPreference(userID, choice, entry.Revision())
				h.mu.Lock()
				h.preferenceWatchRevision = entry.Revision()
				close(h.preferenceChanged)
				h.preferenceChanged = make(chan struct{})
				h.mu.Unlock()
			case entry, ok := <-watcher.Updates():
				if !ok {
					watcher.Stop()
					preferences.Stop()
					if err := ctx.Err(); err != nil {
						return err
					}
					return fmt.Errorf("presence hub: watcher stopped")
				}
				if entry == nil {
					syncComplete = true
					complete()
					h.mu.Lock()
					entries := len(h.snapshot)
					h.mu.Unlock()
					h.logger.Debug("Presence hub sync complete", "entries", entries)
					continue
				}
				if h.beforeLiveRead != nil {
					userID, valid := parsePresenceKey(entry.Key())
					if valid {
						if err := h.beforeLiveRead(ctx, userID); err != nil {
							watcher.Stop()
							preferences.Stop()
							return fmt.Errorf("presence privacy readiness: %w", err)
						}
					}
				}
				h.applyWatcherEntry(entry, syncComplete)
			}
		}
		watcher.Stop()
		preferences.Stop()
	}
}

// Resync replaces the watcher and waits for its latest-value snapshot.
func (h *PresenceHub) Resync(ctx context.Context) error {
	done := make(chan error, 1)
	select {
	case h.resyncRequests <- done:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *PresenceHub) applyWatcherEntry(entry jetstream.KeyValueEntry, fanOut bool) {
	userID, ok := parsePresenceKey(entry.Key())
	if !ok {
		return
	}

	status := PresenceStatusOffline
	if entry.Operation() != jetstream.KeyValueDelete && entry.Operation() != jetstream.KeyValuePurge {
		var presence cachestatev1.UserPresence
		if err := proto.Unmarshal(entry.Value(), &presence); err != nil {
			h.logger.Warn("Presence hub: failed to unmarshal", "error", err, "user_id", userID)
			return
		}
		status = presenceStatusToString(presence.Status)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if status == PresenceStatusOffline {
		delete(h.live, userID)
	} else {
		h.live[userID] = status
	}
	h.updatePublicLocked(userID, fanOut)
}

func (h *PresenceHub) updatePublicLocked(userID string, fanOut bool) {
	status := h.effectiveStatusLocked(userID, h.live[userID])
	previous, hadPrevious := h.snapshot[userID]
	if status == PresenceStatusOffline {
		delete(h.snapshot, userID)
	} else {
		h.snapshot[userID] = status
	}
	changed := previous != status
	if status == PresenceStatusOffline && !hadPrevious {
		changed = false
	}
	if fanOut && changed {
		update := PresenceUpdate{UserID: userID, Status: status}
		for _, sub := range h.subscribers {
			select {
			case sub.ch <- update:
			default:
				sub.lagged.Store(true)
				delete(h.subscribers, sub.id)
				close(sub.done)
				close(sub.ch)
			}
		}
	}
}

// Lagged reports whether the hub closed this subscription after its queue
// overflowed. Callers must reconnect and refetch latest-value presence state.
func (s *PresenceSubscription) Lagged() bool {
	return s != nil && s.lagged.Load()
}

// Subscribe registers a new subscriber for future presence transitions. The
// hub owns the process-wide current-state snapshot and already suppresses
// unchanged status refreshes, so subscribers do not need private snapshot
// copies for deduplication.
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
	done := make(chan struct{})

	h.mu.Lock()
	id := h.nextID
	h.nextID++

	sub := &PresenceSubscription{
		C:    ch,
		ch:   ch,
		Done: done,
		done: done,
		id:   id,
	}
	h.subscribers[id] = sub
	h.mu.Unlock()

	return sub, nil
}

// LivePresenceCount returns the number of users with a current live presence
// record in MEMORY_CACHE. Offline users are represented by absence and are not
// included. The call waits for the initial watcher snapshot so callers see a
// process-local count derived from the same state used for live presence fanout.
func (h *PresenceHub) LivePresenceCount(ctx context.Context) (int, error) {
	select {
	case <-h.ready:
	case <-ctx.Done():
		return 0, ctx.Err()
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	count := 0
	for _, status := range h.snapshot {
		if status != PresenceStatusOffline {
			count++
		}
	}
	return count, nil
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *PresenceHub) Unsubscribe(sub *PresenceSubscription) {
	h.mu.Lock()
	if _, ok := h.subscribers[sub.id]; ok {
		delete(h.subscribers, sub.id)
		close(sub.done)
		close(sub.ch)
	}
	h.mu.Unlock()
}
