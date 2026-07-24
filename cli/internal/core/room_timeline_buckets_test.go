// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type fakeRoomTimelineEventLoader struct {
	mu      sync.Mutex
	events  map[uint64]*corev1.Event
	calls   int
	started chan struct{}
	release chan struct{}
}

func (l *fakeRoomTimelineEventLoader) loadRoomTimelineEvents(_ context.Context, refs []timelineBucketEventRef) ([]*corev1.Event, error) {
	l.mu.Lock()
	l.calls++
	if l.started != nil && l.calls == 1 {
		close(l.started)
	}
	l.mu.Unlock()
	if l.release != nil {
		<-l.release
	}
	out := make([]*corev1.Event, len(refs))
	for index, ref := range refs {
		if event := l.events[ref.sequence]; event != nil {
			out[index] = proto.Clone(event).(*corev1.Event)
			continue
		}
		if !ref.optionalBody {
			return nil, errors.New("required event missing")
		}
	}
	return out, nil
}

func (l *fakeRoomTimelineEventLoader) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func waitForRoomTimelineBucketWaiters(t *testing.T, projection *RoomTimelineProjection, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		projection.RLock()
		waiters := 0
		for _, bucket := range projection.buckets {
			if bucket.loading != nil {
				waiters = bucket.loading.waiters
			}
		}
		projection.RUnlock()
		if waiters == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("bucket load waiters = %d, want %d", waiters, want)
		}
		runtime.Gosched()
	}
}

func bucketTestMessageEvents(at time.Time, bodyID, messageID string) (*corev1.Event, *corev1.Event) {
	body := &corev1.Event{
		Id:        bodyID,
		CreatedAt: timestamppb.New(at),
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{
			RoomId:  "R1",
			EventId: messageID,
			Body: &corev1.MessageBody{
				AuthorId:      "U1",
				BodyEventId:   bodyID,
				EncryptedBody: []byte("ciphertext"),
			},
		}},
	}
	post := &corev1.Event{
		Id:        messageID,
		ActorId:   "U1",
		CreatedAt: timestamppb.New(at),
		Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
			RoomId: "R1",
		}},
	}
	return body, post
}

func TestRoomTimelineColdBucketLoadsFromExactEVTReferences(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	body, post := bucketTestMessageEvents(at, "B1", "M1")
	loader := &fakeRoomTimelineEventLoader{events: map[uint64]*corev1.Event{10: body, 11: post}}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   14 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})

	if err := projection.Apply(body, 10); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(post, 11); err != nil {
		t.Fatal(err)
	}
	projection.RLock()
	entry, ok := projection.entryByEventIDLocked("M1")
	if !ok || entry.Event != nil {
		t.Fatalf("cold entry = %#v, want metadata without payload", entry)
	}
	state := projection.bodyStates["M1"]
	bucket := projection.buckets[state.bucket]
	if state.body != nil || bucket == nil || bucket.resident || len(bucket.refs) != 2 {
		t.Fatalf("cold body/bucket = %#v / %#v", state, bucket)
	}
	projection.RUnlock()

	loadedEntry, ok, err := projection.GetContext(context.Background(), "M1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || loadedEntry.Event.GetId() != "M1" {
		t.Fatalf("loaded entry = %#v", loadedEntry)
	}
	loadedBody, retracted, known, err := projection.LatestBodyContext(context.Background(), "M1")
	if err != nil {
		t.Fatal(err)
	}
	if !known || retracted || loadedBody.GetBodyEventId() != "B1" {
		t.Fatalf("loaded body = %#v, retracted=%v known=%v", loadedBody, retracted, known)
	}
	if loader.callCount() != 1 {
		t.Fatalf("EVT loads = %d, want one bucket load", loader.callCount())
	}
}

func TestRoomTimelineColdBucketLoggingLevels(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) }

	t.Run("successful load stays below info", func(t *testing.T) {
		body, post := bucketTestMessageEvents(at, "B1", "M1")
		var output bytes.Buffer
		logger := log.New(&output)
		logger.SetLevel(log.InfoLevel)
		projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
			eventLoader: &fakeRoomTimelineEventLoader{events: map[uint64]*corev1.Event{10: body, 11: post}},
			hotWindow:   7 * 24 * time.Hour,
			now:         now,
			logger:      logger,
		})
		if err := projection.Apply(body, 10); err != nil {
			t.Fatal(err)
		}
		if err := projection.Apply(post, 11); err != nil {
			t.Fatal(err)
		}
		if _, _, err := projection.GetContext(context.Background(), "M1"); err != nil {
			t.Fatal(err)
		}
		if output.Len() != 0 {
			t.Fatalf("successful cold load logged at INFO or beyond: %s", output.String())
		}
	})

	t.Run("failed load warns once", func(t *testing.T) {
		body, post := bucketTestMessageEvents(at, "B1", "M1")
		var output bytes.Buffer
		logger := log.New(&output)
		logger.SetLevel(log.InfoLevel)
		projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
			eventLoader: &fakeRoomTimelineEventLoader{events: map[uint64]*corev1.Event{10: body}},
			hotWindow:   7 * 24 * time.Hour,
			now:         now,
			logger:      logger,
		})
		if err := projection.Apply(body, 10); err != nil {
			t.Fatal(err)
		}
		if err := projection.Apply(post, 11); err != nil {
			t.Fatal(err)
		}
		if _, _, err := projection.GetContext(context.Background(), "M1"); err == nil {
			t.Fatal("cold load unexpectedly succeeded")
		}
		got := output.String()
		if strings.Count(got, "Failed to load cold Room Timeline bucket") != 1 {
			t.Fatalf("failure log = %q, want one warning", got)
		}
		for _, field := range []string{"room_id", "bucket_start", "event_references", "duration"} {
			if !strings.Contains(got, field) {
				t.Errorf("failure log %q does not contain %q", got, field)
			}
		}
	})
}

func TestRoomTimelineColdBucketSharesConcurrentLoad(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	body, post := bucketTestMessageEvents(at, "B1", "M1")
	loader := &fakeRoomTimelineEventLoader{
		events:  map[uint64]*corev1.Event{10: body, 11: post},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   7 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})
	if err := projection.Apply(body, 10); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(post, 11); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 8)
	go func() {
		_, _, err := projection.GetContext(context.Background(), "M1")
		errs <- err
	}()
	<-loader.started
	for range 7 {
		go func() {
			_, _, err := projection.GetContext(context.Background(), "M1")
			errs <- err
		}()
	}
	waitForRoomTimelineBucketWaiters(t, projection, 7)
	close(loader.release)
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if loader.callCount() != 1 {
		t.Fatalf("EVT loads = %d, want one shared load", loader.callCount())
	}
}

func TestRoomTimelineColdBucketSharesConcurrentLoadError(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	body, post := bucketTestMessageEvents(at, "B1", "M1")
	loader := &fakeRoomTimelineEventLoader{
		events:  map[uint64]*corev1.Event{10: body},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   7 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})
	if err := projection.Apply(body, 10); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(post, 11); err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 8)
	go func() {
		_, _, err := projection.GetContext(context.Background(), "M1")
		errs <- err
	}()
	<-loader.started
	for range 7 {
		go func() {
			_, _, err := projection.GetContext(context.Background(), "M1")
			errs <- err
		}()
	}
	waitForRoomTimelineBucketWaiters(t, projection, 7)
	close(loader.release)
	for range 8 {
		if err := <-errs; err == nil {
			t.Fatal("concurrent cold load unexpectedly succeeded")
		}
	}
	if loader.callCount() != 1 {
		t.Fatalf("EVT loads = %d, want one shared failed load", loader.callCount())
	}
}

func TestRoomTimelineColdBucketDoesNotRestoreBodyShreddedDuringLoad(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	body, post := bucketTestMessageEvents(at, "B1", "M1")
	loader := &fakeRoomTimelineEventLoader{
		events:  map[uint64]*corev1.Event{10: body, 11: post},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   7 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})
	if err := projection.Apply(body, 10); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(post, 11); err != nil {
		t.Fatal(err)
	}

	type bodyResult struct {
		body      *corev1.MessageBody
		retracted bool
		known     bool
		err       error
	}
	result := make(chan bodyResult, 1)
	go func() {
		loaded, retracted, known, err := projection.LatestBodyContext(context.Background(), "M1")
		result <- bodyResult{body: loaded, retracted: retracted, known: known, err: err}
	}()
	<-loader.started
	shredded := &corev1.Event{
		Id:        "S1",
		CreatedAt: timestamppb.New(at.Add(time.Hour)),
		Event: &corev1.Event_UserKeyShredded{UserKeyShredded: &corev1.UserKeyShreddedEvent{
			UserId: "U1",
		}},
	}
	if err := projection.Apply(shredded, 12); err != nil {
		t.Fatal(err)
	}
	close(loader.release)

	got := <-result
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.body != nil || !got.retracted || !got.known {
		t.Fatalf("LatestBodyContext() = body %#v, retracted %v, known %v", got.body, got.retracted, got.known)
	}
	projection.RLock()
	defer projection.RUnlock()
	if projection.bodyStates["M1"].body != nil {
		t.Fatal("shredded body was reinstalled by the completing cold load")
	}
}

func TestRoomTimelineColdBucketInstallIsTransactional(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	body, post := bucketTestMessageEvents(at, "B1", "M1")
	wrongPost := proto.Clone(post).(*corev1.Event)
	wrongPost.Id = "OTHER"
	loader := &fakeRoomTimelineEventLoader{events: map[uint64]*corev1.Event{10: body, 11: wrongPost}}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   7 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})
	if err := projection.Apply(body, 10); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(post, 11); err != nil {
		t.Fatal(err)
	}

	if _, _, err := projection.GetContext(context.Background(), "M1"); err == nil {
		t.Fatal("cold load unexpectedly succeeded")
	}
	projection.RLock()
	defer projection.RUnlock()
	entry, _ := projection.entryByEventIDLocked("M1")
	if entry.Event != nil || projection.bodyStates["M1"].body != nil || projection.buckets[entry.bucket].resident {
		t.Fatal("failed bucket load installed partial payload")
	}
}

func TestRoomTimelineColdBucketAllowsSecurelyDeletedObsoleteBody(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	firstBody, post := bucketTestMessageEvents(at, "B1", "M1")
	secondBody, _ := bucketTestMessageEvents(at.Add(time.Hour), "B2", "M1")
	loader := &fakeRoomTimelineEventLoader{events: map[uint64]*corev1.Event{
		11: secondBody,
		12: post,
	}}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   7 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})
	for index, event := range []*corev1.Event{firstBody, secondBody, post} {
		if err := projection.Apply(event, uint64(10+index)); err != nil {
			t.Fatal(err)
		}
	}

	body, retracted, known, err := projection.LatestBodyContext(context.Background(), "M1")
	if err != nil {
		t.Fatal(err)
	}
	if !known || retracted || body.GetBodyEventId() != "B2" {
		t.Fatalf("body = %#v, retracted=%v known=%v", body, retracted, known)
	}
}

func TestRoomTimelineSnapshotKeepsColdBucketPayloadOut(t *testing.T) {
	at := time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC)
	body, post := bucketTestMessageEvents(at, "B1", "M1")
	loader := &fakeRoomTimelineEventLoader{events: map[uint64]*corev1.Event{10: body, 11: post}}
	projection := newRoomTimelineProjection(roomTimelineProjectionOptions{
		eventLoader: loader,
		hotWindow:   7 * 24 * time.Hour,
		now:         func() time.Time { return time.Date(2026, time.July, 24, 0, 0, 0, 0, time.UTC) },
	})
	if err := projection.Apply(body, 10); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(post, 11); err != nil {
		t.Fatal(err)
	}
	if _, _, err := projection.GetContext(context.Background(), "M1"); err != nil {
		t.Fatal(err)
	}

	payload, err := projection.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &corev1.RoomTimelineProjectionSnapshot{}
	if err := proto.Unmarshal(payload, snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.GetBuckets()) != 1 || snapshot.GetBuckets()[0].GetPayloadResident() {
		t.Fatalf("snapshot buckets = %#v, want cold metadata", snapshot.GetBuckets())
	}
	if snapshot.GetEntries()[0].GetResidentEvent() != nil || snapshot.GetBodies()[0].GetResidentBody() != nil {
		t.Fatal("historical payload leaked into snapshot")
	}
}

func TestRoomTimelineSnapshotRejectsMissingResidentBody(t *testing.T) {
	for _, includePost := range []bool{false, true} {
		t.Run(map[bool]string{false: "body before post", true: "posted message"}[includePost], func(t *testing.T) {
			at := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
			body, post := bucketTestMessageEvents(at, "B1", "M1")
			projection := NewRoomTimelineProjection()
			if err := projection.Apply(body, 10); err != nil {
				t.Fatal(err)
			}
			if includePost {
				if err := projection.Apply(post, 11); err != nil {
					t.Fatal(err)
				}
			}

			payload, err := projection.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			snapshot := &corev1.RoomTimelineProjectionSnapshot{}
			if err := proto.Unmarshal(payload, snapshot); err != nil {
				t.Fatal(err)
			}
			snapshot.Bodies[0].ResidentBody = nil
			payload, err = proto.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}

			restored := NewRoomTimelineProjection()
			if err := restored.Restore(payload); err == nil {
				t.Fatal("Restore() accepted a resident bucket without its current body")
			}
		})
	}
}
