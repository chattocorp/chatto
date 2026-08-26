package core

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationBoundaryIndexInitialSnapshotAndReplicaChanges(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	kv := chattoCore.storage.runtimeStateKV
	ctx := testContext(t)

	const (
		userID     = "U-boundary-index"
		roomID     = "R-boundary-index"
		threadRoot = "E-boundary-index"
	)
	visibilityKey := notificationVisibilityBoundaryKey(userID, roomID)
	visibilityValue := make([]byte, 8)
	binary.BigEndian.PutUint64(visibilityValue, 41)
	if _, err := kv.Put(ctx, visibilityKey, visibilityValue); err != nil {
		t.Fatalf("seed visibility boundary: %v", err)
	}
	readKey := notificationReadBoundaryKey(userID, roomID, threadRoot)
	initialRead := notificationReadBoundary{targetSequence: 23, observedSequence: 37}
	if _, err := kv.Put(ctx, readKey, encodeNotificationReadBoundary(initialRead)); err != nil {
		t.Fatalf("seed read boundary: %v", err)
	}
	unreadKey := notificationUnreadMarkerKey(userID, roomID, threadRoot)
	unreadMarker := &corev1.NotificationUnreadMarker{
		SourceEventId: "E-source", ActorId: "U-actor", SourceStreamSequence: 42,
		Signal: testNotificationSignal(notificationTestSignalFollowedThread, roomID, "E-source"),
	}
	unreadMarker.GetSignal().GetFollowedThreadActivity().Message.ThreadRootEventId = proto.String(threadRoot)
	unreadValue, err := proto.Marshal(unreadMarker)
	if err != nil {
		t.Fatalf("marshal unread marker: %v", err)
	}
	if _, err := kv.Put(ctx, unreadKey, unreadValue); err != nil {
		t.Fatalf("seed unread marker: %v", err)
	}

	index := newNotificationBoundaryIndex(kv, testCoreLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- index.run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("index Run returned %v, want context.Canceled", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("notification boundary index did not stop")
		}
	})
	if err := index.waitReady(ctx); err != nil {
		t.Fatalf("wait for initial snapshot: %v", err)
	}
	if sequence, exists, err := index.visibilityBoundary(ctx, userID, roomID); err != nil || !exists || sequence != 41 {
		t.Fatalf("visibility boundary = (%d, %v, %v), want (41, true, nil)", sequence, exists, err)
	}
	if boundary, exists, err := index.readBoundary(ctx, userID, roomID, threadRoot); err != nil || !exists || boundary != initialRead {
		t.Fatalf("read boundary = (%+v, %v, %v), want (%+v, true, nil)", boundary, exists, err, initialRead)
	}
	if marker, _, exists, err := index.unreadMarker(ctx, notificationReadBoundaryScope{userID: userID, roomID: roomID, threadRootEventID: threadRoot}); err != nil || !exists || !proto.Equal(marker, unreadMarker) {
		t.Fatalf("unread marker = (%+v, %v, %v), want (%+v, true, nil)", marker, exists, err, unreadMarker)
	}

	updatedRead := notificationReadBoundary{targetSequence: 50, observedSequence: 60}
	revision, err := kv.Put(ctx, readKey, encodeNotificationReadBoundary(updatedRead))
	if err != nil {
		t.Fatalf("update read boundary: %v", err)
	}
	if err := index.waitForRevision(ctx, readKey, revision); err != nil {
		t.Fatalf("wait for read boundary revision: %v", err)
	}
	select {
	case <-index.readChanges():
	case <-ctx.Done():
		t.Fatalf("wait for read change: %v", ctx.Err())
	}
	scopes, full := index.takeReadChanges()
	wantScope := notificationReadBoundaryScope{userID: userID, roomID: roomID, threadRootEventID: threadRoot}
	if full || len(scopes) != 1 || scopes[0] != wantScope {
		t.Fatalf("read changes = (%+v, %v), want ([%+v], false)", scopes, full, wantScope)
	}
	if err := index.resync(ctx); err != nil {
		t.Fatalf("resync index: %v", err)
	}
	select {
	case <-index.readChanges():
	case <-ctx.Done():
		t.Fatalf("wait for resync repair signal: %v", ctx.Err())
	}
	if scopes, full := index.takeReadChanges(); !full || len(scopes) != 0 {
		t.Fatalf("resync changes = (%+v, %v), want ([], true)", scopes, full)
	}
	if boundary, exists, err := index.readBoundary(ctx, userID, roomID, threadRoot); err != nil || !exists || boundary != updatedRead {
		t.Fatalf("resynced read boundary = (%+v, %v, %v), want (%+v, true, nil)", boundary, exists, err, updatedRead)
	}

	if err := kv.Delete(ctx, visibilityKey); err != nil {
		t.Fatalf("delete visibility boundary: %v", err)
	}
	for {
		_, exists, err := index.visibilityBoundary(ctx, userID, roomID)
		if err != nil {
			t.Fatalf("read deleted visibility boundary: %v", err)
		}
		if !exists {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("deleted visibility boundary remained indexed: %v", ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
}

func TestNotificationBoundaryIndexParsesOwnedKeys(t *testing.T) {
	if userID, roomID, ok := parseNotificationVisibilityBoundaryKey("notification_visibility_boundary.U1.R1"); !ok || userID != "U1" || roomID != "R1" {
		t.Fatalf("visibility key = (%q, %q, %v)", userID, roomID, ok)
	}
	if scope, ok := parseNotificationReadBoundaryKey("notification_read_boundary.U1.R1.E1"); !ok || scope != (notificationReadBoundaryScope{userID: "U1", roomID: "R1", threadRootEventID: "E1"}) {
		t.Fatalf("read key = (%+v, %v)", scope, ok)
	}
	if scope, ok := parseNotificationUnreadMarkerKey("notification_unread_marker.U1.R1.E1"); !ok || scope != (notificationReadBoundaryScope{userID: "U1", roomID: "R1", threadRootEventID: "E1"}) {
		t.Fatalf("unread key = (%+v, %v)", scope, ok)
	}
	for _, key := range []string{"notification_visibility_boundary.U1", "notification_read_boundary.U1", "other.U1.R1"} {
		if _, _, ok := parseNotificationVisibilityBoundaryKey(key); ok {
			t.Fatalf("visibility parser accepted %q", key)
		}
		if _, ok := parseNotificationReadBoundaryKey(key); ok {
			t.Fatalf("read parser accepted %q", key)
		}
		if _, ok := parseNotificationUnreadMarkerKey(key); ok {
			t.Fatalf("unread parser accepted %q", key)
		}
	}
}
