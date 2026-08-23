package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	dmArchiveMarkerFilter      = "archive.dm.>"
	dmArchiveMarkerChangeLimit = 4096
)

type dmArchiveIndexEntry struct {
	value    []byte
	revision uint64
	deleted  bool
}

type dmArchiveMarkerChange struct {
	generation uint64
	userID     string
	roomID     string
}

// DMArchiveIndex is the process-local mirror of viewer-specific DM archive
// markers. RUNTIME_STATE remains authoritative; one filtered watcher per
// process applies both the initial latest-value snapshot and remote updates.
type DMArchiveIndex struct {
	kv     jetstream.KeyValue
	logger *log.Logger

	mu               sync.RWMutex
	markers          map[string]map[string]dmArchiveIndexEntry
	changeGeneration uint64
	changeFloor      uint64
	changes          []dmArchiveMarkerChange
	changed          chan struct{}
	ready            chan struct{}
	readyOnce        sync.Once
	resyncRequests   chan chan error
}

// NewDMArchiveIndex creates an empty index. Run must be started before reads.
func NewDMArchiveIndex(kv jetstream.KeyValue, logger *log.Logger) *DMArchiveIndex {
	return &DMArchiveIndex{
		kv:             kv,
		logger:         logger,
		markers:        make(map[string]map[string]dmArchiveIndexEntry),
		changed:        make(chan struct{}),
		ready:          make(chan struct{}),
		resyncRequests: make(chan chan error),
	}
}

// Run maintains the process-wide archive-marker snapshot until ctx is
// cancelled.
func (i *DMArchiveIndex) Run(ctx context.Context) error {
	if i.logger != nil {
		i.logger.Debug("DM archive index started")
		defer i.logger.Debug("DM archive index stopped")
	}

	var pendingResync chan error
	for {
		watcher, err := i.kv.WatchFiltered(ctx, []string{dmArchiveMarkerFilter})
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
			return fmt.Errorf("DM archive index: create watcher: %w", err)
		}

		restart := false
		for !restart {
			var resyncRequests <-chan chan error
			if pendingResync == nil {
				resyncRequests = i.resyncRequests
			}
			select {
			case <-ctx.Done():
				watcher.Stop()
				if pendingResync != nil {
					pendingResync <- ctx.Err()
				}
				return ctx.Err()
			case pendingResync = <-resyncRequests:
				i.resetSnapshot()
				restart = true
			case entry, ok := <-watcher.Updates():
				if !ok {
					watcher.Stop()
					if err := ctx.Err(); err != nil {
						return err
					}
					return fmt.Errorf("DM archive index: watcher stopped")
				}
				if entry == nil {
					i.readyOnce.Do(func() { close(i.ready) })
					if pendingResync != nil {
						pendingResync <- nil
						pendingResync = nil
					}
					if i.logger != nil {
						i.logger.Debug("DM archive index sync complete", "markers", i.entryCount())
					}
					continue
				}
				i.apply(entry)
			}
		}
		watcher.Stop()
	}
}

func (i *DMArchiveIndex) resetSnapshot() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.markers = make(map[string]map[string]dmArchiveIndexEntry)
	i.changeGeneration++
	i.changeFloor = i.changeGeneration
	i.changes = nil
	close(i.changed)
	i.changed = make(chan struct{})
}

// WaitReady blocks until the watcher has applied its initial snapshot.
func (i *DMArchiveIndex) WaitReady(ctx context.Context) error {
	select {
	case <-i.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Resync replaces the watcher and waits for its current snapshot.
func (i *DMArchiveIndex) Resync(ctx context.Context) error {
	done := make(chan error, 1)
	select {
	case i.resyncRequests <- done:
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

func (i *DMArchiveIndex) marker(ctx context.Context, userID, roomID string) (dmArchiveIndexEntry, bool, error) {
	if err := i.WaitReady(ctx); err != nil {
		return dmArchiveIndexEntry{}, false, err
	}
	i.mu.RLock()
	entry, known := i.markers[userID][roomID]
	i.mu.RUnlock()
	if !known || entry.deleted {
		return entry, false, nil
	}
	entry.value = append([]byte(nil), entry.value...)
	return entry, true, nil
}

func (i *DMArchiveIndex) fence(ctx context.Context) (uint64, error) {
	if err := i.WaitReady(ctx); err != nil {
		return 0, err
	}
	i.mu.RLock()
	generation := i.changeGeneration
	i.mu.RUnlock()
	return generation, nil
}

func (i *DMArchiveIndex) roomIDsChangedAfter(ctx context.Context, userID string, fence uint64) ([]string, error) {
	if err := i.WaitReady(ctx); err != nil {
		return nil, err
	}
	i.mu.RLock()
	if fence < i.changeFloor {
		i.mu.RUnlock()
		return nil, fmt.Errorf("DM archive change fence %d precedes retained generation %d", fence, i.changeFloor)
	}
	changed := make(map[string]struct{})
	for _, change := range i.changes {
		if change.generation > fence && change.userID == userID {
			changed[change.roomID] = struct{}{}
		}
	}
	i.mu.RUnlock()
	roomIDs := make([]string, 0, len(changed))
	for roomID := range changed {
		roomIDs = append(roomIDs, roomID)
	}
	slices.Sort(roomIDs)
	return roomIDs, nil
}

func (i *DMArchiveIndex) waitForRevision(ctx context.Context, key string, revision uint64) error {
	return i.waitForKeyRevision(ctx, key, func(current uint64) bool { return current >= revision })
}

func (i *DMArchiveIndex) waitForRevisionAfter(ctx context.Context, key string, revision uint64) error {
	return i.waitForKeyRevision(ctx, key, func(current uint64) bool { return current > revision })
}

func (i *DMArchiveIndex) waitForKeyRevision(ctx context.Context, key string, done func(uint64) bool) error {
	if err := i.WaitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		userID, roomID, ok := parseDMArchiveMarkerKey(key)
		current := uint64(0)
		if ok {
			current = i.markers[userID][roomID].revision
		}
		changed := i.changed
		satisfied := done(current)
		i.mu.RUnlock()
		if satisfied {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (i *DMArchiveIndex) apply(entry jetstream.KeyValueEntry) {
	userID, roomID, ok := parseDMArchiveMarkerKey(entry.Key())
	if !ok {
		return
	}
	deleted := entry.Operation() == jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge

	i.mu.Lock()
	defer i.mu.Unlock()
	if i.markers[userID] == nil {
		i.markers[userID] = make(map[string]dmArchiveIndexEntry)
	}
	if entry.Revision() <= i.markers[userID][roomID].revision {
		return
	}
	i.markers[userID][roomID] = dmArchiveIndexEntry{
		value:    append([]byte(nil), entry.Value()...),
		revision: entry.Revision(),
		deleted:  deleted,
	}
	i.changeGeneration++
	i.changes = append(i.changes, dmArchiveMarkerChange{
		generation: i.changeGeneration,
		userID:     userID,
		roomID:     roomID,
	})
	if len(i.changes) > dmArchiveMarkerChangeLimit {
		i.changeFloor = i.changes[0].generation
		i.changes = i.changes[1:]
	}
	close(i.changed)
	i.changed = make(chan struct{})
}

func (i *DMArchiveIndex) entryCount() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	count := 0
	for _, markers := range i.markers {
		for _, entry := range markers {
			if !entry.deleted {
				count++
			}
		}
	}
	return count
}

func dmArchiveMarkerKey(userID, roomID string) string {
	return fmt.Sprintf("archive.dm.%s.%s", userID, roomID)
}

func dmArchiveMarkerUserFilter(userID string) string {
	return fmt.Sprintf("archive.dm.%s.*", userID)
}

func parseDMArchiveMarkerKey(key string) (userID, roomID string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 4 || parts[0] != "archive" || parts[1] != "dm" || parts[2] == "" || parts[3] == "" {
		return "", "", false
	}
	return parts[2], parts[3], true
}
