package core

import (
	"context"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/cache_state/v1"
	"sync"
	"sync/atomic"
	"time"

	"hmans.de/chatto/internal/evtstream"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"

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

// PresenceHub runs a single MEMORY_CACHE watcher on presence.> and fans out
// per-user presence updates. Each Chatto process has one PresenceHub instance,
// reducing KV watcher count from O(users × spaces) to 1 per process.
type PresenceHub struct {
	// beforeLiveRead catches up private choices before processing shared liveness.
	// It runs outside mu so the preference projector can advance while it waits.
	beforeLiveRead func(context.Context, string) error
	memoryCacheKV  jetstream.KeyValue
	logger         *log.Logger

	mu             sync.Mutex
	subscribers    map[uint64]*PresenceSubscription
	nextID         uint64
	snapshot       map[string]string // current presence state (built during init sync)
	live           map[string]string // heartbeat state, never exposed without the saved choice
	preferences    map[string]*apiv1.PresencePreference
	ready          chan struct{} // closed when initial sync is complete
	readyOnce      sync.Once     // ensures ready is closed exactly once
	resyncRequests chan chan error
}

// NewPresenceHub creates a PresenceHub. Call Run() to start it.
func NewPresenceHub(memoryCacheKV jetstream.KeyValue, logger *log.Logger) *PresenceHub {
	return &PresenceHub{
		memoryCacheKV:  memoryCacheKV,
		logger:         logger,
		subscribers:    make(map[uint64]*PresenceSubscription),
		snapshot:       make(map[string]string),
		live:           make(map[string]string),
		preferences:    make(map[string]*apiv1.PresencePreference),
		ready:          make(chan struct{}),
		resyncRequests: make(chan chan error),
	}
}

// Subjects binds the private choice projection to its durable account facts.
func (h *PresenceHub) Subjects() []string {
	return []string{evtstream.ConfigSubjectFilter(), evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted)}
}

// Apply derives private choices and public transitions locally. It never writes
// to NATS. Every replica independently combines choices with its KV watcher.
func (h *PresenceHub) Apply(event *evtv1.Event, _ uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if choice := event.GetUserPresencePreferenceChanged(); choice != nil {
		h.preferences[choice.UserId] = &apiv1.PresencePreference{Mode: apiv1.PresenceMode(choice.Mode), Revision: event.Id}
		h.updatePublicLocked(choice.UserId, true)
	}
	if deleted := event.GetUserAccountDeleted(); deleted != nil {
		delete(h.preferences, deleted.UserId)
		delete(h.live, deleted.UserId)
		h.updatePublicLocked(deleted.UserId, true)
	}
	return nil
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
		switch p.Mode {
		case apiv1.PresenceMode_PRESENCE_MODE_ONLINE:
			return PresenceStatusOnline
		case apiv1.PresenceMode_PRESENCE_MODE_AWAY:
			return PresenceStatusAway
		case apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB:
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

		syncComplete := false
		restart := false
		for !restart {
			var resyncRequests <-chan chan error
			if pendingResync == nil {
				resyncRequests = h.resyncRequests
			}
			select {
			case <-ctx.Done():
				watcher.Stop()
				if pendingResync != nil {
					pendingResync <- ctx.Err()
				}
				return ctx.Err()
			case pendingResync = <-resyncRequests:
				h.mu.Lock()
				h.snapshot = make(map[string]string)
				h.live = make(map[string]string)
				h.mu.Unlock()
				restart = true
			case entry, ok := <-watcher.Updates():
				if !ok {
					watcher.Stop()
					if err := ctx.Err(); err != nil {
						return err
					}
					return fmt.Errorf("presence hub: watcher stopped")
				}
				if entry == nil {
					syncComplete = true
					h.readyOnce.Do(func() { close(h.ready) })
					if pendingResync != nil {
						pendingResync <- nil
						pendingResync = nil
					}
					h.mu.Lock()
					entries := len(h.snapshot)
					h.mu.Unlock()
					h.logger.Debug("Presence hub sync complete", "entries", entries)
					continue
				}
				if h.beforeLiveRead != nil {
					if userID, valid := parsePresenceKey(entry.Key()); valid {
						if err := h.beforeLiveRead(ctx, userID); err != nil {
							watcher.Stop()
							return fmt.Errorf("presence privacy readiness: %w", err)
						}
					}
				}
				h.applyWatcherEntry(entry, syncComplete)
			}
		}
		watcher.Stop()
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
