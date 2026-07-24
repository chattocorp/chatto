// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	roomTimelineBucketWidth     = 7 * 24 * time.Hour
	roomTimelineLoadConcurrency = 16
)

// timelineBucketKey deliberately includes the room. A busy room therefore
// cannot force an unrelated room's historical payload into memory.
type timelineBucketKey struct {
	roomID    string
	weekStart int64
}

type timelineBucketEventRef struct {
	sequence     uint64
	optionalBody bool
}

type timelineBucket struct {
	refs          []timelineBucketEventRef
	resident      bool
	residentBytes int64
	loading       chan struct{}
	lastAccess    time.Time
}

type roomTimelineEventLoader interface {
	loadRoomTimelineEvents(context.Context, []timelineBucketEventRef) ([]*corev1.Event, error)
}

type jetStreamRoomTimelineEventLoader struct {
	stream jetstream.Stream
}

func (l jetStreamRoomTimelineEventLoader) loadRoomTimelineEvents(ctx context.Context, refs []timelineBucketEventRef) ([]*corev1.Event, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	eventsByIndex := make([]*corev1.Event, len(refs))
	workers := min(roomTimelineLoadConcurrency, len(refs))
	jobs := make(chan int)
	loadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				ref := refs[index]
				msg, err := l.stream.GetMsg(loadCtx, ref.sequence)
				if err != nil {
					if ref.optionalBody && errors.Is(err, jetstream.ErrMsgNotFound) {
						continue
					}
					errOnce.Do(func() {
						firstErr = fmt.Errorf("load Room Timeline EVT sequence %d: %w", ref.sequence, err)
						cancel()
					})
					continue
				}
				var event corev1.Event
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					errOnce.Do(func() {
						firstErr = fmt.Errorf("decode Room Timeline EVT sequence %d: %w", ref.sequence, err)
						cancel()
					})
					continue
				}
				eventsByIndex[index] = &event
			}
		}()
	}
sendJobs:
	for index := range refs {
		select {
		case jobs <- index:
		case <-loadCtx.Done():
			break sendJobs
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return eventsByIndex, nil
}

func roomTimelineWeekStart(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	utc := at.UTC()
	dayStart := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	daysSinceMonday := (int(dayStart.Weekday()) + 6) % 7
	return dayStart.AddDate(0, 0, -daysSinceMonday).Unix()
}

func (p *RoomTimelineProjection) bucketForEventLocked(event *corev1.Event) timelineBucketKey {
	return timelineBucketKey{
		roomID:    roomIDOfEvent(event),
		weekStart: roomTimelineWeekStart(eventCreatedAt(event)),
	}
}

func (p *RoomTimelineProjection) messageBucketLocked(eventID, roomID string, event *corev1.Event) timelineBucketKey {
	if state, ok := p.bodyStates[eventID]; ok && state.bucket.roomID != "" {
		return state.bucket
	}
	if idx, ok := p.byEventID[eventID]; ok {
		if entry := p.entryAtLocked(idx); entry != nil && entry.bucket.roomID != "" {
			return entry.bucket
		}
	}
	return timelineBucketKey{roomID: roomID, weekStart: roomTimelineWeekStart(eventCreatedAt(event))}
}

func (p *RoomTimelineProjection) bucketForMutationLocked(event *corev1.Event, roomID string) timelineBucketKey {
	if posted := event.GetMessagePosted(); posted != nil {
		return p.messageBucketLocked(event.GetId(), roomID, event)
	}
	if retracted := event.GetMessageRetracted(); retracted != nil {
		return p.messageBucketLocked(retracted.GetEventId(), roomID, event)
	}
	return timelineBucketKey{roomID: roomID, weekStart: roomTimelineWeekStart(eventCreatedAt(event))}
}

func (p *RoomTimelineProjection) bucketIsHotLocked(key timelineBucketKey) bool {
	if p.retainAll || key.weekStart == 0 {
		return true
	}
	bucketEnd := time.Unix(key.weekStart, 0).UTC().Add(roomTimelineBucketWidth)
	return !bucketEnd.Before(p.now().UTC().Add(-p.hotWindow))
}

func (p *RoomTimelineProjection) recordBucketRefLocked(key timelineBucketKey, sequence uint64, optionalBody bool) {
	if key.roomID == "" || sequence == 0 {
		return
	}
	bucket := p.buckets[key]
	if bucket == nil {
		bucket = &timelineBucket{resident: p.bucketIsHotLocked(key)}
		p.buckets[key] = bucket
	}
	bucket.refs = append(bucket.refs, timelineBucketEventRef{
		sequence:     sequence,
		optionalBody: optionalBody,
	})
	if bucket.resident {
		bucket.lastAccess = p.now()
	}
}

func (p *RoomTimelineProjection) bucketResidentLocked(key timelineBucketKey) bool {
	bucket := p.buckets[key]
	return bucket == nil || bucket.resident
}

func (p *RoomTimelineProjection) ensureBucket(ctx context.Context, key timelineBucketKey) error {
	if key.roomID == "" {
		return nil
	}
	for {
		p.Lock()
		bucket := p.buckets[key]
		if bucket == nil || bucket.resident {
			if bucket != nil {
				bucket.lastAccess = p.now()
			}
			p.Unlock()
			return nil
		}
		if loading := bucket.loading; loading != nil {
			p.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-loading:
				continue
			}
		}
		if p.eventLoader == nil {
			p.Unlock()
			return fmt.Errorf("Room Timeline bucket %s/%d is cold and has no EVT loader", key.roomID, key.weekStart)
		}
		loading := make(chan struct{})
		bucket.loading = loading
		refs := append([]timelineBucketEventRef(nil), bucket.refs...)
		p.Unlock()

		loaded, err := p.eventLoader.loadRoomTimelineEvents(ctx, refs)

		p.Lock()
		bucket = p.buckets[key]
		if err == nil && len(bucket.refs) != len(refs) {
			bucket.loading = nil
			close(loading)
			p.Unlock()
			continue
		}
		if err == nil {
			err = p.installBucketPayloadLocked(key, refs, loaded)
		}
		if err == nil {
			bucket.resident = true
			bucket.lastAccess = p.now()
		}
		bucket.loading = nil
		close(loading)
		p.Unlock()
		return err
	}
}

func (p *RoomTimelineProjection) installBucketPayloadLocked(key timelineBucketKey, refs []timelineBucketEventRef, loaded []*corev1.Event) error {
	if len(refs) != len(loaded) {
		return fmt.Errorf("Room Timeline bucket %s/%d loaded %d events for %d references", key.roomID, key.weekStart, len(loaded), len(refs))
	}
	entryPayloads := make(map[int]*corev1.Event)
	bodyPayloads := make(map[string]*corev1.MessageBody)
	for index, event := range loaded {
		if event == nil {
			continue
		}
		ref := refs[index]
		if bodyEvent := event.GetMessageBody(); bodyEvent != nil {
			targetID := bodyEvent.GetEventId()
			state, ok := p.bodyStates[targetID]
			if !ok || state.bucket != key || state.currentSequence != ref.sequence {
				continue
			}
			body := bodyEvent.GetBody()
			if body == nil || (body.GetBodyEventId() != "" && body.GetBodyEventId() != event.GetId()) {
				return fmt.Errorf("Room Timeline bucket %s/%d has invalid body envelope at sequence %d", key.roomID, key.weekStart, ref.sequence)
			}
			if _, shredded := p.shreddedUsers[body.GetAuthorId()]; shredded {
				continue
			}
			loadedBody := cloneMessageBody(body)
			if loadedBody.GetBodyEventId() == "" {
				loadedBody.BodyEventId = event.GetId()
			}
			bodyPayloads[targetID] = loadedBody
			continue
		}

		eventID := event.GetId()
		entry, ok := p.entryByEventIDLocked(eventID)
		if !ok || entry.bucket != key || entry.StreamSeq != ref.sequence {
			continue
		}
		entryPayloads[p.byEventID[eventID]] = event
	}

	for eventID, state := range p.bodyStates {
		if state.bucket != key || state.currentSequence == 0 {
			continue
		}
		if _, retracted := p.retractedFlags[eventID]; retracted {
			continue
		}
		if _, hidden := p.hiddenEchoes[eventID]; hidden {
			continue
		}
		entry, _ := p.entryByEventIDLocked(eventID)
		if entry != nil {
			if _, shredded := p.shreddedUsers[entry.authorID]; shredded {
				continue
			}
		}
		if _, loaded := bodyPayloads[eventID]; !loaded {
			return fmt.Errorf("Room Timeline bucket %s/%d is missing current body sequence %d for %s", key.roomID, key.weekStart, state.currentSequence, eventID)
		}
	}
	for _, idx := range p.byEventID {
		entry := p.entryAtLocked(idx)
		if entry != nil && entry.bucket == key {
			if _, loaded := entryPayloads[idx]; loaded {
				continue
			}
			return fmt.Errorf("Room Timeline bucket %s/%d is missing event sequence %d for %s", key.roomID, key.weekStart, entry.StreamSeq, entry.eventID)
		}
	}
	for idx, event := range entryPayloads {
		p.entries[idx].Event = event
	}
	var residentBytes int64
	for _, event := range entryPayloads {
		residentBytes += int64(proto.Size(event))
	}
	for eventID, state := range p.bodyStates {
		if state.bucket != key {
			continue
		}
		state.body = bodyPayloads[eventID]
		p.bodyStates[eventID] = state
		residentBytes += int64(proto.Size(state.body))
	}
	p.buckets[key].residentBytes = residentBytes
	return nil
}

func (p *RoomTimelineProjection) ensureEntryIndexes(ctx context.Context, indexes []int) error {
	p.RLock()
	keys := make(map[timelineBucketKey]struct{})
	for _, idx := range indexes {
		if entry := p.entryAtLocked(idx); entry != nil {
			keys[entry.bucket] = struct{}{}
		}
	}
	p.RUnlock()
	for key := range keys {
		if err := p.ensureBucket(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
