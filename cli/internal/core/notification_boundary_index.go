package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
)

const (
	notificationVisibilityBoundaryFilterAll = notificationVisibilityKeyPrefix + ">"
	notificationReadBoundaryFilterAll       = notificationReadBoundaryKeyPrefix + ">"
	notificationUnreadMarkerFilterAll       = notificationUnreadMarkerKeyPrefix + ">"
)

type notificationReadBoundaryScope struct {
	userID            string
	roomID            string
	threadRootEventID string
}

type notificationVisibilityBoundaryEntry struct {
	sequence uint64
	revision uint64
	deleted  bool
}

type notificationReadBoundaryEntry struct {
	boundary notificationReadBoundary
	revision uint64
	deleted  bool
}

type notificationUnreadMarkerEntry struct {
	marker   *runtimestatev1.NotificationUnreadMarker
	revision uint64
	deleted  bool
}

// notificationBoundaryIndex maintains the process-local mirror of notification
// visibility boundaries, read boundaries, and Badge markers in RUNTIME_STATE.
// The KV bucket remains authoritative; one filtered watcher supplies the
// initial latest-value snapshot and every later update from local or remote
// replicas.
type notificationBoundaryIndex struct {
	kv     jetstream.KeyValue
	logger *log.Logger

	mu                 sync.RWMutex
	visibility         map[string]notificationVisibilityBoundaryEntry
	read               map[string]notificationReadBoundaryEntry
	unreadMarkerByUser map[string]map[string]map[string]notificationUnreadMarkerEntry
	changed            chan struct{}
	ready              chan struct{}
	synced             bool
	readWake           chan struct{}
	readDirty          map[notificationReadBoundaryScope]struct{}
	fullRepair         bool

	resyncRequests chan chan error
}

func newNotificationBoundaryIndex(kv jetstream.KeyValue, logger *log.Logger) *notificationBoundaryIndex {
	return &notificationBoundaryIndex{
		kv:                 kv,
		logger:             logger,
		visibility:         make(map[string]notificationVisibilityBoundaryEntry),
		read:               make(map[string]notificationReadBoundaryEntry),
		unreadMarkerByUser: make(map[string]map[string]map[string]notificationUnreadMarkerEntry),
		changed:            make(chan struct{}),
		ready:              make(chan struct{}),
		readWake:           make(chan struct{}, 1),
		readDirty:          make(map[notificationReadBoundaryScope]struct{}),
		resyncRequests:     make(chan chan error),
	}
}

// run watches notification boundary keys until ctx is cancelled.
func (i *notificationBoundaryIndex) run(ctx context.Context) error {
	if i.logger != nil {
		i.logger.Debug("Notification boundary index started")
		defer i.logger.Debug("Notification boundary index stopped")
	}

	var pendingResync chan error
	for {
		watcher, err := i.kv.WatchFiltered(ctx, []string{
			notificationVisibilityBoundaryFilterAll,
			notificationReadBoundaryFilterAll,
			notificationUnreadMarkerFilterAll,
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
			return fmt.Errorf("notification boundary index: create watcher: %w", err)
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
					return fmt.Errorf("notification boundary index: watcher stopped")
				}
				if entry == nil {
					i.completeSync(pendingResync != nil)
					if pendingResync != nil {
						pendingResync <- nil
						pendingResync = nil
					}
					continue
				}
				if err := i.apply(entry); err != nil {
					watcher.Stop()
					return err
				}
			}
		}
		watcher.Stop()
	}
}

func (i *notificationBoundaryIndex) resetSnapshot() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.visibility = make(map[string]notificationVisibilityBoundaryEntry)
	i.read = make(map[string]notificationReadBoundaryEntry)
	i.unreadMarkerByUser = make(map[string]map[string]map[string]notificationUnreadMarkerEntry)
	i.synced = false
	i.ready = make(chan struct{})
	close(i.changed)
	i.changed = make(chan struct{})
}

func (i *notificationBoundaryIndex) completeSync(fullRepair bool) {
	i.mu.Lock()
	if !i.synced {
		i.synced = true
		close(i.ready)
	}
	if fullRepair {
		i.fullRepair = true
		i.signalReadChangeLocked()
	}
	i.mu.Unlock()
}

func (i *notificationBoundaryIndex) apply(entry jetstream.KeyValueEntry) error {
	key := entry.Key()
	deleted := entry.Operation() == jetstream.KeyValueDelete || entry.Operation() == jetstream.KeyValuePurge
	visibilityUserID, visibilityRoomID, isVisibility := parseNotificationVisibilityBoundaryKey(key)
	readScope, isRead := parseNotificationReadBoundaryKey(key)
	unreadScope, isUnread := parseNotificationUnreadMarkerKey(key)
	if !isVisibility && !isRead && !isUnread {
		return nil
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	wasSynced := i.synced
	switch {
	case isVisibility:
		current := i.visibility[key]
		if entry.Revision() <= current.revision {
			return nil
		}
		next := notificationVisibilityBoundaryEntry{revision: entry.Revision(), deleted: deleted}
		if !deleted {
			if len(entry.Value()) != 8 {
				return fmt.Errorf("notification visibility boundary %s has invalid length %d", key, len(entry.Value()))
			}
			next.sequence = binary.BigEndian.Uint64(entry.Value())
		}
		i.visibility[notificationVisibilityBoundaryKey(visibilityUserID, visibilityRoomID)] = next
	case isRead:
		current := i.read[key]
		if entry.Revision() <= current.revision {
			return nil
		}
		next := notificationReadBoundaryEntry{revision: entry.Revision(), deleted: deleted}
		if !deleted {
			boundary, err := decodeNotificationReadBoundary(entry.Value())
			if err != nil {
				return fmt.Errorf("decode %s: %w", key, err)
			}
			next.boundary = boundary
		}
		i.read[key] = next
		if wasSynced {
			i.readDirty[readScope] = struct{}{}
			i.signalReadChangeLocked()
		}
	case isUnread:
		current := i.unreadMarkerEntryLocked(unreadScope)
		if entry.Revision() <= current.revision {
			return nil
		}
		next := notificationUnreadMarkerEntry{revision: entry.Revision(), deleted: deleted}
		if !deleted {
			var marker runtimestatev1.NotificationUnreadMarker
			if err := proto.Unmarshal(entry.Value(), &marker); err != nil {
				return fmt.Errorf("decode %s: %w", key, err)
			}
			message := notificationSignalMessage(marker.GetSignal())
			if marker.GetSourceStreamSequence() == 0 || message == nil || message.GetRoomId() != unreadScope.roomID || message.GetThreadRootEventId() != unreadScope.threadRootEventID {
				return fmt.Errorf("notification unread marker %s does not match its key scope", key)
			}
			next.marker = &marker
		}
		i.setUnreadMarkerEntryLocked(unreadScope, next)
	}
	close(i.changed)
	i.changed = make(chan struct{})
	return nil
}

func (i *notificationBoundaryIndex) signalReadChangeLocked() {
	select {
	case i.readWake <- struct{}{}:
	default:
	}
}

func (i *notificationBoundaryIndex) readChanges() <-chan struct{} { return i.readWake }

func (i *notificationBoundaryIndex) takeReadChanges() ([]notificationReadBoundaryScope, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	scopes := make([]notificationReadBoundaryScope, 0, len(i.readDirty))
	for scope := range i.readDirty {
		scopes = append(scopes, scope)
	}
	clear(i.readDirty)
	full := i.fullRepair
	i.fullRepair = false
	return scopes, full
}

func (i *notificationBoundaryIndex) requeueReadChanges(scopes []notificationReadBoundaryScope, full bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if full {
		i.fullRepair = true
		return
	}
	for _, scope := range scopes {
		i.readDirty[scope] = struct{}{}
	}
}

func (i *notificationBoundaryIndex) visibilityBoundary(ctx context.Context, userID, roomID string) (uint64, bool, error) {
	if err := i.waitReady(ctx); err != nil {
		return 0, false, err
	}
	i.mu.RLock()
	entry, exists := i.visibility[notificationVisibilityBoundaryKey(userID, roomID)]
	i.mu.RUnlock()
	return entry.sequence, exists && !entry.deleted, nil
}

func (i *notificationBoundaryIndex) readBoundary(ctx context.Context, userID, roomID, threadRootEventID string) (notificationReadBoundary, bool, error) {
	if err := i.waitReady(ctx); err != nil {
		return notificationReadBoundary{}, false, err
	}
	i.mu.RLock()
	entry, exists := i.read[notificationReadBoundaryKey(userID, roomID, threadRootEventID)]
	i.mu.RUnlock()
	return entry.boundary, exists && !entry.deleted, nil
}

func (i *notificationBoundaryIndex) unreadMarker(ctx context.Context, scope notificationReadBoundaryScope) (*runtimestatev1.NotificationUnreadMarker, uint64, bool, error) {
	if err := i.waitReady(ctx); err != nil {
		return nil, 0, false, err
	}
	i.mu.RLock()
	entry := i.unreadMarkerEntryLocked(scope)
	i.mu.RUnlock()
	if entry.deleted || entry.marker == nil {
		return nil, entry.revision, false, nil
	}
	return proto.Clone(entry.marker).(*runtimestatev1.NotificationUnreadMarker), entry.revision, true, nil
}

// unreadMarkers returns detached marker values for one room. An empty thread
// root includes the room marker and all thread markers so room unread state can
// aggregate nested Badge attention.
func (i *notificationBoundaryIndex) unreadMarkers(ctx context.Context, userID, roomID, threadRootEventID string) ([]*runtimestatev1.NotificationUnreadMarker, error) {
	if err := i.waitReady(ctx); err != nil {
		return nil, err
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	byThread := i.unreadMarkerByUser[userID][roomID]
	if threadRootEventID != "" {
		entry := byThread[threadRootEventID]
		if entry.deleted || entry.marker == nil {
			return nil, nil
		}
		return []*runtimestatev1.NotificationUnreadMarker{proto.Clone(entry.marker).(*runtimestatev1.NotificationUnreadMarker)}, nil
	}
	result := make([]*runtimestatev1.NotificationUnreadMarker, 0, len(byThread))
	for _, entry := range byThread {
		if entry.deleted || entry.marker == nil {
			continue
		}
		result = append(result, proto.Clone(entry.marker).(*runtimestatev1.NotificationUnreadMarker))
	}
	return result, nil
}

func (i *notificationBoundaryIndex) unreadMarkerScopes(userID, roomID string, beforeSequence uint64) []notificationReadBoundaryScope {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make([]notificationReadBoundaryScope, 0)
	for candidateUserID, byRoom := range i.unreadMarkerByUser {
		if userID != "" && candidateUserID != userID {
			continue
		}
		for candidateRoomID, byThread := range byRoom {
			if roomID != "" && candidateRoomID != roomID {
				continue
			}
			for threadRootEventID, entry := range byThread {
				if entry.deleted || entry.marker == nil || (beforeSequence != 0 && entry.marker.GetSourceStreamSequence() >= beforeSequence) {
					continue
				}
				result = append(result, notificationReadBoundaryScope{userID: candidateUserID, roomID: candidateRoomID, threadRootEventID: threadRootEventID})
			}
		}
	}
	return result
}

func (i *notificationBoundaryIndex) unreadMarkerEntryLocked(scope notificationReadBoundaryScope) notificationUnreadMarkerEntry {
	return i.unreadMarkerByUser[scope.userID][scope.roomID][scope.threadRootEventID]
}

func (i *notificationBoundaryIndex) setUnreadMarkerEntryLocked(scope notificationReadBoundaryScope, entry notificationUnreadMarkerEntry) {
	if i.unreadMarkerByUser[scope.userID] == nil {
		i.unreadMarkerByUser[scope.userID] = make(map[string]map[string]notificationUnreadMarkerEntry)
	}
	if i.unreadMarkerByUser[scope.userID][scope.roomID] == nil {
		i.unreadMarkerByUser[scope.userID][scope.roomID] = make(map[string]notificationUnreadMarkerEntry)
	}
	i.unreadMarkerByUser[scope.userID][scope.roomID][scope.threadRootEventID] = entry
}

func (i *notificationBoundaryIndex) waitForRevision(ctx context.Context, key string, revision uint64) error {
	if err := i.waitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		current := i.revisionForKeyLocked(key)
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

func (i *notificationBoundaryIndex) waitForRevisionAfter(ctx context.Context, key string, revision uint64) error {
	if err := i.waitReady(ctx); err != nil {
		return err
	}
	for {
		i.mu.RLock()
		current := i.revisionForKeyLocked(key)
		changed := i.changed
		i.mu.RUnlock()
		if current > revision {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (i *notificationBoundaryIndex) revisionForKeyLocked(key string) uint64 {
	if _, _, ok := parseNotificationVisibilityBoundaryKey(key); ok {
		return i.visibility[key].revision
	}
	if _, ok := parseNotificationReadBoundaryKey(key); ok {
		return i.read[key].revision
	}
	if scope, ok := parseNotificationUnreadMarkerKey(key); ok {
		return i.unreadMarkerEntryLocked(scope).revision
	}
	return 0
}

func (i *notificationBoundaryIndex) waitReady(ctx context.Context) error {
	for {
		i.mu.RLock()
		if i.synced {
			i.mu.RUnlock()
			return nil
		}
		ready := i.ready
		i.mu.RUnlock()
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (i *notificationBoundaryIndex) resync(ctx context.Context) error {
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

func parseNotificationVisibilityBoundaryKey(key string) (userID, roomID string, ok bool) {
	parts := strings.Split(key, ".")
	if len(parts) != 3 || parts[0] != strings.TrimSuffix(notificationVisibilityKeyPrefix, ".") || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func parseNotificationReadBoundaryKey(key string) (notificationReadBoundaryScope, bool) {
	parts := strings.Split(key, ".")
	prefix := strings.TrimSuffix(notificationReadBoundaryKeyPrefix, ".")
	if (len(parts) != 3 && len(parts) != 4) || parts[0] != prefix || parts[1] == "" || parts[2] == "" {
		return notificationReadBoundaryScope{}, false
	}
	scope := notificationReadBoundaryScope{userID: parts[1], roomID: parts[2]}
	if len(parts) == 4 {
		if parts[3] == "" {
			return notificationReadBoundaryScope{}, false
		}
		scope.threadRootEventID = parts[3]
	}
	return scope, true
}

func parseNotificationUnreadMarkerKey(key string) (notificationReadBoundaryScope, bool) {
	parts := strings.Split(key, ".")
	prefix := strings.TrimSuffix(notificationUnreadMarkerKeyPrefix, ".")
	if (len(parts) != 3 && len(parts) != 4) || parts[0] != prefix || parts[1] == "" || parts[2] == "" {
		return notificationReadBoundaryScope{}, false
	}
	scope := notificationReadBoundaryScope{userID: parts[1], roomID: parts[2]}
	if len(parts) == 4 {
		if parts[3] == "" {
			return notificationReadBoundaryScope{}, false
		}
		scope.threadRootEventID = parts[3]
	}
	return scope, true
}
