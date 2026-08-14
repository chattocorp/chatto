package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const notificationOccurrenceWatchFilter = "notification_v2.>"

type notificationOccurrenceIndexEntry struct {
	key        string
	revision   uint64
	deleted    bool
	occurrence *corev1.NotificationOccurrence
}

type notificationOccurrenceIndexSnapshot struct {
	entriesByUser    map[string]map[string]notificationOccurrenceIndexEntry
	keyRevisions     map[string]uint64
	observedRevision uint64
}

func newNotificationOccurrenceIndexSnapshot() *notificationOccurrenceIndexSnapshot {
	return &notificationOccurrenceIndexSnapshot{
		entriesByUser: make(map[string]map[string]notificationOccurrenceIndexEntry),
		keyRevisions:  make(map[string]uint64),
	}
}

// NotificationOccurrenceIndex mirrors the versioned notification occurrence
// keyspace through one process-wide RUNTIME_STATE watcher. KV remains the
// authority; this index makes user lists, counts, and realtime reconciliation
// finite in-memory reads rather than one key scan per request or socket.
type NotificationOccurrenceIndex struct {
	kv     jetstream.KeyValue
	logger *log.Logger

	mu            sync.RWMutex
	entriesByUser map[string]map[string]notificationOccurrenceIndexEntry
	keyRevisions  map[string]uint64
	// observedRevision includes non-occurrence markers delivered by the same
	// ordered watcher and is therefore usable as a local KV read fence.
	observedRevision uint64
	changed          chan struct{}
	ready            chan struct{}
	readyOnce        sync.Once
	resyncRequests   chan chan error
}

func NewNotificationOccurrenceIndex(kv jetstream.KeyValue, logger *log.Logger) *NotificationOccurrenceIndex {
	return &NotificationOccurrenceIndex{
		kv:             kv,
		logger:         logger,
		entriesByUser:  make(map[string]map[string]notificationOccurrenceIndexEntry),
		keyRevisions:   make(map[string]uint64),
		changed:        make(chan struct{}),
		ready:          make(chan struct{}),
		resyncRequests: make(chan chan error),
	}
}

func (i *NotificationOccurrenceIndex) Run(ctx context.Context) error {
	if i.logger != nil {
		i.logger.Debug("Notification occurrence index started")
		defer i.logger.Debug("Notification occurrence index stopped")
	}

	var pendingResync chan error
	for {
		watcher, err := i.kv.Watch(ctx, notificationOccurrenceWatchFilter)
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
			return fmt.Errorf("notification occurrence index: create watcher: %w", err)
		}

		// A watcher always begins with a complete snapshot followed by a nil
		// sentinel. Build that snapshot away from readers and publish it in one
		// swap. In particular, a recovery resync must not expose a transiently
		// empty but still "ready" index to list or delivery paths.
		staged := newNotificationOccurrenceIndexSnapshot()
		initialSync := true
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
				restart = true
			case entry, ok := <-watcher.Updates():
				if !ok {
					watcher.Stop()
					if err := ctx.Err(); err != nil {
						return err
					}
					return fmt.Errorf("notification occurrence index: watcher stopped")
				}
				if entry == nil {
					if initialSync {
						i.installSnapshot(staged)
						initialSync = false
					}
					i.readyOnce.Do(func() { close(i.ready) })
					if pendingResync != nil {
						pendingResync <- nil
						pendingResync = nil
					}
					if i.logger != nil {
						i.logger.Debug("Notification occurrence index sync complete", "occurrences", i.entryCount())
					}
					continue
				}
				if initialSync {
					i.applyToSnapshot(staged, entry)
				} else {
					i.apply(entry)
				}
			}
		}
		watcher.Stop()
	}
}

func (i *NotificationOccurrenceIndex) WaitReady(ctx context.Context) error {
	select {
	case <-i.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *NotificationOccurrenceIndex) Resync(ctx context.Context) error {
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

func (i *NotificationOccurrenceIndex) installSnapshot(snapshot *notificationOccurrenceIndexSnapshot) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.entriesByUser = snapshot.entriesByUser
	i.keyRevisions = snapshot.keyRevisions
	i.observedRevision = snapshot.observedRevision
	close(i.changed)
	i.changed = make(chan struct{})
}

func (i *NotificationOccurrenceIndex) applyToSnapshot(snapshot *notificationOccurrenceIndexSnapshot, entry jetstream.KeyValueEntry) {
	snapshot.observedRevision = max(snapshot.observedRevision, entry.Revision())
	userID, _, ok := parseNotificationOccurrenceKey(entry.Key())
	if !ok {
		return
	}
	if entry.Revision() <= snapshot.keyRevisions[entry.Key()] {
		return
	}
	if entry.Operation() == jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge {
		delete(snapshot.keyRevisions, entry.Key())
		if entries := snapshot.entriesByUser[userID]; entries != nil {
			delete(entries, entry.Key())
			if len(entries) == 0 {
				delete(snapshot.entriesByUser, userID)
			}
		}
		return
	}
	var occurrence corev1.NotificationOccurrence
	if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
		if i.logger != nil {
			i.logger.Warn("Ignoring malformed notification occurrence", "key", entry.Key(), "error", err)
		}
		return
	}
	if occurrence.GetRecipientId() != userID {
		if i.logger != nil {
			i.logger.Warn("Ignoring notification occurrence with mismatched recipient", "key", entry.Key())
		}
		return
	}
	if snapshot.entriesByUser[userID] == nil {
		snapshot.entriesByUser[userID] = make(map[string]notificationOccurrenceIndexEntry)
	}
	snapshot.keyRevisions[entry.Key()] = entry.Revision()
	snapshot.entriesByUser[userID][entry.Key()] = notificationOccurrenceIndexEntry{
		key: entry.Key(), revision: entry.Revision(), occurrence: &occurrence,
	}
}

func (i *NotificationOccurrenceIndex) apply(entry jetstream.KeyValueEntry) {
	userID, _, ok := parseNotificationOccurrenceKey(entry.Key())
	if !ok {
		i.advanceObservedRevision(entry.Revision())
		return
	}

	indexed := notificationOccurrenceIndexEntry{
		key:      entry.Key(),
		revision: entry.Revision(),
		deleted: entry.Operation() == jetstream.KeyValueDelete ||
			entry.Operation() == jetstream.KeyValuePurge,
	}
	if !indexed.deleted {
		var occurrence corev1.NotificationOccurrence
		if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
			if i.logger != nil {
				i.logger.Warn("Ignoring malformed notification occurrence", "key", entry.Key(), "error", err)
			}
		} else if occurrence.GetRecipientId() != userID {
			if i.logger != nil {
				i.logger.Warn("Ignoring notification occurrence with mismatched recipient", "key", entry.Key())
			}
		} else {
			indexed.occurrence = &occurrence
		}
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	if entry.Revision() <= i.keyRevisions[entry.Key()] {
		if entry.Revision() > i.observedRevision {
			i.observedRevision = entry.Revision()
			close(i.changed)
			i.changed = make(chan struct{})
		}
		return
	}
	i.observedRevision = max(i.observedRevision, entry.Revision())
	if indexed.deleted || indexed.occurrence == nil {
		delete(i.keyRevisions, entry.Key())
		if entries := i.entriesByUser[userID]; entries != nil {
			delete(entries, entry.Key())
			if len(entries) == 0 {
				delete(i.entriesByUser, userID)
			}
		}
		close(i.changed)
		i.changed = make(chan struct{})
		return
	}
	i.keyRevisions[entry.Key()] = entry.Revision()
	if i.entriesByUser[userID] == nil {
		i.entriesByUser[userID] = make(map[string]notificationOccurrenceIndexEntry)
	}
	i.entriesByUser[userID][entry.Key()] = indexed
	close(i.changed)
	i.changed = make(chan struct{})
}

func (i *NotificationOccurrenceIndex) advanceObservedRevision(revision uint64) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if revision <= i.observedRevision {
		return
	}
	i.observedRevision = revision
	close(i.changed)
	i.changed = make(chan struct{})
}

func (i *NotificationOccurrenceIndex) userEntries(ctx context.Context, userID string) ([]notificationOccurrenceIndexEntry, error) {
	if err := i.WaitReady(ctx); err != nil {
		return nil, err
	}
	i.mu.Lock()
	i.pruneExpiredUserLocked(userID, time.Now().UTC())
	entries := make([]notificationOccurrenceIndexEntry, 0, len(i.entriesByUser[userID]))
	for _, entry := range i.entriesByUser[userID] {
		if entry.deleted || entry.occurrence == nil {
			continue
		}
		entry.occurrence = proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
		entries = append(entries, entry)
	}
	i.mu.Unlock()
	return entries, nil
}

func (i *NotificationOccurrenceIndex) allEntries(ctx context.Context) ([]notificationOccurrenceIndexEntry, error) {
	if err := i.WaitReady(ctx); err != nil {
		return nil, err
	}
	i.mu.Lock()
	i.pruneExpiredLocked(time.Now().UTC())
	entries := make([]notificationOccurrenceIndexEntry, 0)
	for _, userEntries := range i.entriesByUser {
		for _, entry := range userEntries {
			if entry.deleted || entry.occurrence == nil {
				continue
			}
			entry.occurrence = proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
			entries = append(entries, entry)
		}
	}
	i.mu.Unlock()
	return entries, nil
}

func (i *NotificationOccurrenceIndex) occurrenceByID(ctx context.Context, userID, occurrenceID string) (notificationOccurrenceIndexEntry, bool, error) {
	entries, err := i.userEntries(ctx, userID)
	if err != nil {
		return notificationOccurrenceIndexEntry{}, false, err
	}
	for _, entry := range entries {
		if entry.occurrence.GetId() == occurrenceID {
			return entry, true, nil
		}
	}
	return notificationOccurrenceIndexEntry{}, false, nil
}

func (i *NotificationOccurrenceIndex) occurrenceBySource(ctx context.Context, userID, sourceEventID string) (notificationOccurrenceIndexEntry, bool, error) {
	if err := i.WaitReady(ctx); err != nil {
		return notificationOccurrenceIndexEntry{}, false, err
	}
	key := notificationOccurrenceKey(userID, sourceEventID)
	i.mu.Lock()
	i.pruneExpiredUserLocked(userID, time.Now().UTC())
	entry, ok := i.entriesByUser[userID][key]
	i.mu.Unlock()
	if !ok || entry.deleted || entry.occurrence == nil {
		return notificationOccurrenceIndexEntry{}, false, nil
	}
	entry.occurrence = proto.Clone(entry.occurrence).(*corev1.NotificationOccurrence)
	return entry, true, nil
}

func (i *NotificationOccurrenceIndex) pruneExpiredLocked(now time.Time) {
	changed := false
	for userID := range i.entriesByUser {
		changed = i.pruneExpiredUserEntriesLocked(userID, now) || changed
	}
	if changed {
		close(i.changed)
		i.changed = make(chan struct{})
	}
}

func (i *NotificationOccurrenceIndex) pruneExpiredUserLocked(userID string, now time.Time) {
	if i.pruneExpiredUserEntriesLocked(userID, now) {
		close(i.changed)
		i.changed = make(chan struct{})
	}
}

func (i *NotificationOccurrenceIndex) pruneExpiredUserEntriesLocked(userID string, now time.Time) bool {
	entries := i.entriesByUser[userID]
	changed := false
	for key, entry := range entries {
		if entry.deleted || entry.occurrence == nil {
			continue
		}
		expiresAt := entry.occurrence.GetExpiresAt()
		if expiresAt == nil || expiresAt.AsTime().After(now) {
			continue
		}
		delete(entries, key)
		delete(i.keyRevisions, key)
		changed = true
	}
	if len(entries) == 0 {
		delete(i.entriesByUser, userID)
	}
	return changed
}

func (i *NotificationOccurrenceIndex) waitForRevision(ctx context.Context, key string, revision uint64) error {
	if err := i.WaitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		current := i.keyRevisions[key]
		changed := i.changed
		i.mu.RUnlock()
		if current >= revision {
			return nil
		}
		if current == 0 {
			gone, err := i.authoritativeRevisionGone(ctx, key)
			if err != nil {
				return err
			}
			if gone {
				// Expiry or purge is already authoritative. There is no live
				// occurrence left for a realtime assembler to wait for.
				return nil
			}
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (i *NotificationOccurrenceIndex) waitForRevisionAfter(ctx context.Context, key string, revision uint64) error {
	if err := i.WaitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		current := i.keyRevisions[key]
		changed := i.changed
		i.mu.RUnlock()
		if current > revision {
			return nil
		}
		if current == 0 {
			gone, err := i.authoritativeRevisionGone(ctx, key)
			if err != nil {
				return err
			}
			if gone {
				return nil
			}
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (i *NotificationOccurrenceIndex) waitForObservedRevision(ctx context.Context, revision uint64) error {
	if revision == 0 {
		return nil
	}
	if err := i.WaitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		current := i.observedRevision
		changed := i.changed
		i.mu.RUnlock()
		if current >= revision {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (i *NotificationOccurrenceIndex) authoritativeRevisionGone(ctx context.Context, key string) (bool, error) {
	entry, err := i.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var occurrence corev1.NotificationOccurrence
	if err := proto.Unmarshal(entry.Value(), &occurrence); err != nil {
		return false, fmt.Errorf("decode notification occurrence revision fence: %w", err)
	}
	expiresAt := occurrence.GetExpiresAt()
	return expiresAt != nil && !expiresAt.AsTime().After(time.Now().UTC()), nil
}

func (i *NotificationOccurrenceIndex) entryCount() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	count := 0
	for _, entries := range i.entriesByUser {
		for _, entry := range entries {
			if !entry.deleted && entry.occurrence != nil {
				count++
			}
		}
	}
	return count
}

func parseNotificationOccurrenceKey(key string) (userID, sourceEventID string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != "notification_v2" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}
