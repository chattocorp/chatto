package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	roomReadMarkerFilter   = "read.room.>"
	threadReadMarkerFilter = "read.thread.>"
)

type threadReadMarkerKey struct {
	roomID            string
	threadRootEventID string
}

type readStateIndexEntry struct {
	value    []byte
	revision uint64
	deleted  bool
}

// userReadStateSnapshot is a detached copy of one user's room and thread read
// markers from the process-wide RUNTIME_STATE watcher.
type userReadStateSnapshot struct {
	roomMarkers   map[string][]byte
	threadMarkers map[threadReadMarkerKey][]byte
}

// ReadStateIndex maintains the process-local mirror of persisted room and
// thread read markers. RUNTIME_STATE remains authoritative; one filtered KV
// watcher per Chatto process supplies both the initial latest-value snapshot
// and every later update from local or remote replicas.
type ReadStateIndex struct {
	kv     jetstream.KeyValue
	logger *log.Logger

	mu             sync.RWMutex
	roomMarkers    map[string]map[string]readStateIndexEntry
	threadMarkers  map[string]map[threadReadMarkerKey]readStateIndexEntry
	changed        chan struct{}
	ready          chan struct{}
	readyOnce      sync.Once
	resyncRequests chan chan error
}

// NewReadStateIndex creates an empty index. Run must be started before reads.
func NewReadStateIndex(kv jetstream.KeyValue, logger *log.Logger) *ReadStateIndex {
	return &ReadStateIndex{
		kv:             kv,
		logger:         logger,
		roomMarkers:    make(map[string]map[string]readStateIndexEntry),
		threadMarkers:  make(map[string]map[threadReadMarkerKey]readStateIndexEntry),
		changed:        make(chan struct{}),
		ready:          make(chan struct{}),
		resyncRequests: make(chan chan error),
	}
}

// Run watches every read marker, applies the initial latest-value snapshot,
// and then keeps the index current until ctx is cancelled.
func (i *ReadStateIndex) Run(ctx context.Context) error {
	if i.logger != nil {
		i.logger.Debug("Read state index started")
		defer i.logger.Debug("Read state index stopped")
	}

	var pendingResync chan error
	for {
		watcher, err := i.kv.WatchFiltered(ctx, []string{
			roomReadMarkerFilter,
			threadReadMarkerFilter,
		})
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
			return fmt.Errorf("read state index: create watcher: %w", err)
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
					return fmt.Errorf("read state index: watcher stopped")
				}
				if entry == nil {
					i.readyOnce.Do(func() { close(i.ready) })
					if pendingResync != nil {
						pendingResync <- nil
						pendingResync = nil
					}
					if i.logger != nil {
						rooms, threads := i.entryCounts()
						i.logger.Debug("Read state index sync complete",
							"room_markers", rooms,
							"thread_markers", threads,
						)
					}
					continue
				}
				i.apply(entry)
			}
		}
		watcher.Stop()
	}
}

func (i *ReadStateIndex) resetSnapshot() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.roomMarkers = make(map[string]map[string]readStateIndexEntry)
	i.threadMarkers = make(map[string]map[threadReadMarkerKey]readStateIndexEntry)
	close(i.changed)
	i.changed = make(chan struct{})
}

// Resync replaces the watcher and waits for its latest-value snapshot.
func (i *ReadStateIndex) Resync(ctx context.Context) error {
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

// WaitReady blocks until the watcher's initial latest-value snapshot has been
// applied or ctx is cancelled.
func (i *ReadStateIndex) WaitReady(ctx context.Context) error {
	select {
	case <-i.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *ReadStateIndex) roomMarker(ctx context.Context, userID, roomID string) (readStateIndexEntry, bool, error) {
	if err := i.WaitReady(ctx); err != nil {
		return readStateIndexEntry{}, false, err
	}
	i.mu.RLock()
	entry, known := i.roomMarkers[userID][roomID]
	i.mu.RUnlock()
	if !known || entry.deleted {
		return entry, false, nil
	}
	entry.value = append([]byte(nil), entry.value...)
	return entry, true, nil
}

func (i *ReadStateIndex) threadMarker(ctx context.Context, userID, roomID, threadRootEventID string) (readStateIndexEntry, bool, error) {
	if err := i.WaitReady(ctx); err != nil {
		return readStateIndexEntry{}, false, err
	}
	key := threadReadMarkerKey{roomID: roomID, threadRootEventID: threadRootEventID}
	i.mu.RLock()
	entry, known := i.threadMarkers[userID][key]
	i.mu.RUnlock()
	if !known || entry.deleted {
		return entry, false, nil
	}
	entry.value = append([]byte(nil), entry.value...)
	return entry, true, nil
}

func (i *ReadStateIndex) userSnapshot(ctx context.Context, userID string) (userReadStateSnapshot, error) {
	if err := i.WaitReady(ctx); err != nil {
		return userReadStateSnapshot{}, err
	}
	snapshot := userReadStateSnapshot{
		roomMarkers:   make(map[string][]byte),
		threadMarkers: make(map[threadReadMarkerKey][]byte),
	}
	i.mu.RLock()
	for roomID, entry := range i.roomMarkers[userID] {
		if entry.deleted {
			continue
		}
		snapshot.roomMarkers[roomID] = append([]byte(nil), entry.value...)
	}
	for key, entry := range i.threadMarkers[userID] {
		if entry.deleted {
			continue
		}
		snapshot.threadMarkers[key] = append([]byte(nil), entry.value...)
	}
	i.mu.RUnlock()
	return snapshot, nil
}

// waitForRevision provides local read-your-writes after a successful KV
// mutation. Revisions are tracked per key so unrelated RUNTIME_STATE traffic
// cannot satisfy the barrier.
func (i *ReadStateIndex) waitForRevision(ctx context.Context, key string, revision uint64) error {
	return i.waitForKeyRevision(ctx, key, func(current uint64) bool {
		return current >= revision
	})
}

// waitForRevisionAfter waits for a conflicting remote/local writer to become
// visible before an OCC retry re-reads the index.
func (i *ReadStateIndex) waitForRevisionAfter(ctx context.Context, key string, revision uint64) error {
	return i.waitForKeyRevision(ctx, key, func(current uint64) bool {
		return current > revision
	})
}

func (i *ReadStateIndex) waitForKeyRevision(ctx context.Context, key string, done func(uint64) bool) error {
	if err := i.WaitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		current := i.revisionForKeyLocked(key)
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

func (i *ReadStateIndex) revisionForKeyLocked(key string) uint64 {
	if userID, roomID, ok := parseRoomReadMarkerKey(key); ok {
		return i.roomMarkers[userID][roomID].revision
	}
	if userID, marker, ok := parseThreadReadMarkerKey(key); ok {
		return i.threadMarkers[userID][marker].revision
	}
	return 0
}

func (i *ReadStateIndex) apply(entry jetstream.KeyValueEntry) {
	key := entry.Key()
	revision := entry.Revision()
	roomUserID, roomID, isRoom := parseRoomReadMarkerKey(key)
	threadUserID, threadKey, isThread := parseThreadReadMarkerKey(key)
	if !isRoom && !isThread {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	deleted := entry.Operation() == jetstream.KeyValueDelete ||
		entry.Operation() == jetstream.KeyValuePurge
	switch {
	case isRoom:
		if i.roomMarkers[roomUserID] == nil {
			i.roomMarkers[roomUserID] = make(map[string]readStateIndexEntry)
		}
		if revision <= i.roomMarkers[roomUserID][roomID].revision {
			return
		}
		i.roomMarkers[roomUserID][roomID] = readStateIndexEntry{
			value:    append([]byte(nil), entry.Value()...),
			revision: revision,
			deleted:  deleted,
		}
	case isThread:
		if i.threadMarkers[threadUserID] == nil {
			i.threadMarkers[threadUserID] = make(map[threadReadMarkerKey]readStateIndexEntry)
		}
		if revision <= i.threadMarkers[threadUserID][threadKey].revision {
			return
		}
		i.threadMarkers[threadUserID][threadKey] = readStateIndexEntry{
			value:    append([]byte(nil), entry.Value()...),
			revision: revision,
			deleted:  deleted,
		}
	}
	close(i.changed)
	i.changed = make(chan struct{})
}

func (i *ReadStateIndex) entryCounts() (rooms, threads int) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	for _, markers := range i.roomMarkers {
		for _, entry := range markers {
			if !entry.deleted {
				rooms++
			}
		}
	}
	for _, markers := range i.threadMarkers {
		for _, entry := range markers {
			if !entry.deleted {
				threads++
			}
		}
	}
	return rooms, threads
}

func parseRoomReadMarkerKey(key string) (userID, roomID string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 4 || parts[0] != "read" || parts[1] != "room" ||
		parts[2] == "" || parts[3] == "" {
		return "", "", false
	}
	return parts[2], parts[3], true
}

func parseThreadReadMarkerKey(key string) (userID string, marker threadReadMarkerKey, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 5 || parts[0] != "read" || parts[1] != "thread" ||
		parts[2] == "" || parts[3] == "" || parts[4] == "" {
		return "", threadReadMarkerKey{}, false
	}
	return parts[2], threadReadMarkerKey{
		roomID:            parts[3],
		threadRootEventID: parts[4],
	}, true
}
