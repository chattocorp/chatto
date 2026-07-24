// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
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
	encodedRefs    []byte
	referenceCount int
	lastSequence   uint64
	resident       bool
	residentBytes  int64
	loading        *timelineBucketLoad
	lastAccess     time.Time
}

type timelineBucketLoad struct {
	done    chan struct{}
	err     error
	waiters int
}

type roomTimelineEventLoader interface {
	loadRoomTimelineEvents(context.Context, []timelineBucketEventRef) ([]*corev1.Event, error)
}

type jetStreamRoomTimelineEventLoader struct {
	stream jetstream.Stream
}

func appendTimelineBucketEventRef(bucket *timelineBucket, sequence uint64, optionalBody bool) error {
	if sequence == 0 {
		return errors.New("Room Timeline bucket reference has zero sequence")
	}
	if bucket.referenceCount > 0 && sequence <= bucket.lastSequence {
		return fmt.Errorf("Room Timeline bucket reference sequence %d does not follow %d", sequence, bucket.lastSequence)
	}
	delta := sequence - bucket.lastSequence
	if delta > math.MaxUint64>>1 {
		return fmt.Errorf("Room Timeline bucket reference delta %d is too large", delta)
	}
	value := delta << 1
	if optionalBody {
		value |= 1
	}
	bucket.encodedRefs = binary.AppendUvarint(bucket.encodedRefs, value)
	bucket.referenceCount++
	bucket.lastSequence = sequence
	return nil
}

func decodeTimelineBucketEventRefs(encoded []byte, capacityHint int) ([]timelineBucketEventRef, error) {
	refs := make([]timelineBucketEventRef, 0, capacityHint)
	_, _, err := walkTimelineBucketEventRefs(encoded, func(ref timelineBucketEventRef) {
		refs = append(refs, ref)
	})
	return refs, err
}

func inspectTimelineBucketEventRefs(encoded []byte) (count int, lastSequence uint64, err error) {
	return walkTimelineBucketEventRefs(encoded, nil)
}

func walkTimelineBucketEventRefs(encoded []byte, visit func(timelineBucketEventRef)) (count int, lastSequence uint64, err error) {
	var sequence uint64
	for len(encoded) > 0 {
		value, n := binary.Uvarint(encoded)
		if n <= 0 {
			return 0, 0, errors.New("Room Timeline bucket references contain an invalid varint")
		}
		delta := value >> 1
		if delta == 0 || sequence > math.MaxUint64-delta {
			return 0, 0, errors.New("Room Timeline bucket references contain an invalid sequence delta")
		}
		sequence += delta
		ref := timelineBucketEventRef{
			sequence:     sequence,
			optionalBody: value&1 != 0,
		}
		if visit != nil {
			visit(ref)
		}
		count++
		encoded = encoded[n:]
	}
	return count, sequence, nil
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

func (p *RoomTimelineProjection) recordBucketRefLocked(key timelineBucketKey, sequence uint64, optionalBody bool) error {
	if key.roomID == "" || sequence == 0 {
		return nil
	}
	bucket := p.buckets[key]
	if bucket == nil {
		bucket = &timelineBucket{resident: p.bucketIsHotLocked(key)}
		p.buckets[key] = bucket
	}
	if err := appendTimelineBucketEventRef(bucket, sequence, optionalBody); err != nil {
		return err
	}
	if bucket.resident {
		bucket.lastAccess = p.now()
	}
	return nil
}

func (p *RoomTimelineProjection) bucketResidentLocked(key timelineBucketKey) bool {
	bucket := p.buckets[key]
	return bucket == nil || bucket.resident
}

func (p *RoomTimelineProjection) ensureBucket(ctx context.Context, key timelineBucketKey) error {
	if key.roomID == "" {
		return nil
	}
	bucketStart := time.Unix(key.weekStart, 0).UTC()
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
			loading.waiters++
			p.Unlock()
			select {
			case <-ctx.Done():
				p.Lock()
				loading.waiters--
				p.Unlock()
				p.logger.Debug("Stopped waiting for cold Room Timeline bucket",
					"room_id", key.roomID,
					"bucket_start", bucketStart,
					"error", ctx.Err(),
				)
				return ctx.Err()
			case <-loading.done:
				p.Lock()
				loading.waiters--
				p.Unlock()
				if loading.err != nil {
					return loading.err
				}
				continue
			}
		}
		if p.eventLoader == nil {
			p.Unlock()
			err := fmt.Errorf("Room Timeline bucket %s/%d is cold and has no EVT loader", key.roomID, key.weekStart)
			p.logger.Warn("Cannot load cold Room Timeline bucket",
				"room_id", key.roomID,
				"bucket_start", bucketStart,
				"event_references", bucket.referenceCount,
				"error", err,
			)
			return err
		}
		loading := &timelineBucketLoad{done: make(chan struct{})}
		bucket.loading = loading
		encodedRefs := bytes.Clone(bucket.encodedRefs)
		referenceCount := bucket.referenceCount
		p.Unlock()

		startedAt := time.Now()
		p.logger.Debug("Loading cold Room Timeline bucket",
			"room_id", key.roomID,
			"bucket_start", bucketStart,
			"event_references", referenceCount,
		)
		refs, err := decodeTimelineBucketEventRefs(encodedRefs, referenceCount)
		var loaded []*corev1.Event
		if err == nil && len(refs) != referenceCount {
			err = fmt.Errorf("Room Timeline bucket %s/%d decoded %d events for %d references", key.roomID, key.weekStart, len(refs), referenceCount)
		}
		if err == nil {
			loaded, err = p.eventLoader.loadRoomTimelineEvents(ctx, refs)
		}

		p.Lock()
		bucket = p.buckets[key]
		if err == nil && bucket.referenceCount != referenceCount {
			currentRefs := bucket.referenceCount
			waiters := loading.waiters
			bucket.loading = nil
			close(loading.done)
			p.Unlock()
			p.logger.Debug("Restarting cold Room Timeline bucket load after concurrent projection updates",
				"room_id", key.roomID,
				"bucket_start", bucketStart,
				"loaded_event_references", referenceCount,
				"current_event_references", currentRefs,
				"shared_waiters", waiters,
				"duration", time.Since(startedAt),
			)
			continue
		}
		if err == nil {
			err = p.installBucketPayloadLocked(key, refs, loaded)
		}
		if err == nil {
			bucket.resident = true
			bucket.lastAccess = p.now()
		}
		residentBytes := bucket.residentBytes
		waiters := loading.waiters
		loading.err = err
		bucket.loading = nil
		close(loading.done)
		p.Unlock()
		duration := time.Since(startedAt)
		if err == nil {
			p.logger.Debug("Loaded cold Room Timeline bucket",
				"room_id", key.roomID,
				"bucket_start", bucketStart,
				"event_references", referenceCount,
				"resident_payload_bytes", residentBytes,
				"shared_waiters", waiters,
				"duration", duration,
			)
		} else if errors.Is(err, context.Canceled) {
			p.logger.Debug("Cold Room Timeline bucket load was canceled",
				"room_id", key.roomID,
				"bucket_start", bucketStart,
				"event_references", referenceCount,
				"shared_waiters", waiters,
				"duration", duration,
				"error", err,
			)
		} else {
			p.logger.Warn("Failed to load cold Room Timeline bucket",
				"room_id", key.roomID,
				"bucket_start", bucketStart,
				"event_references", referenceCount,
				"shared_waiters", waiters,
				"duration", duration,
				"error", err,
			)
		}
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
		body := bodyPayloads[eventID]
		if _, retracted := p.retractedFlags[eventID]; retracted {
			body = nil
		}
		if _, hidden := p.hiddenEchoes[eventID]; hidden {
			body = nil
		}
		if entry, ok := p.entryByEventIDLocked(eventID); ok {
			if _, shredded := p.shreddedUsers[entry.authorID]; shredded {
				body = nil
			}
		}
		state.body = body
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
