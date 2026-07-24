// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	for range 8 {
		go func() {
			_, _, err := projection.GetContext(context.Background(), "M1")
			errs <- err
		}()
	}
	<-loader.started
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
	snapshot := &corev1.RoomTimelineProjectionSnapshotV3{}
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
