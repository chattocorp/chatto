package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	myEventsIngressBuffer       = 256
	myEventsSubscriberBuffer    = 256
	myEventsSubscriberByteLimit = 2 << 20
	myEventsVisibilityWorkers   = 16
	myEventsIngressFlushTimeout = 5 * time.Second
)

type myEventsDelivery struct {
	event EventEnvelope
	bytes int64
}

type myEventsSubscription struct {
	C      <-chan myEventsDelivery
	ch     chan myEventsDelivery
	Done   <-chan struct{}
	done   chan struct{}
	id     uint64
	userID string

	queuedBytes atomic.Int64
}

type myEventsUserState struct {
	memberRooms     map[string]struct{}
	visibleRooms    map[string]struct{}
	roomSnapshotSeq uint64
	subscribers     map[uint64]*myEventsSubscription
}

var errMyEventsIngressChanged = errors.New("myEvents ingress generation changed")

type myEventsRegistration struct {
	generation        uint64
	visibilityVersion uint64
	roomSnapshotSeq   uint64
	userID            string
	memberRooms       map[string]struct{}
	visibleRooms      map[string]struct{}
	sub               *myEventsSubscription
	ctx               context.Context
	result            chan error
}

// MyEventsHub owns the process-wide NATS ingress for realtime events. It
// classifies and decodes each message once, waits for local projections once,
// and then fans immutable event envelopes out through per-session queues.
// Room visibility is shared by all sessions belonging to the same user.
type MyEventsHub struct {
	model *MyEventsModel

	mu          sync.Mutex
	users       map[string]*myEventsUserState
	subscribers map[uint64]*myEventsSubscription
	nextID      uint64
	ready       chan struct{}
	readyOnce   sync.Once
	decoded     atomic.Uint64
	prefiltered atomic.Uint64

	accepting         bool
	generation        uint64
	visibilityVersion uint64
	stateChanged      chan struct{}
	registrations     chan *myEventsRegistration
}

func NewMyEventsHub(model *MyEventsModel) *MyEventsHub {
	return &MyEventsHub{
		model:         model,
		users:         make(map[string]*myEventsUserState),
		subscribers:   make(map[uint64]*myEventsSubscription),
		ready:         make(chan struct{}),
		stateChanged:  make(chan struct{}),
		registrations: make(chan *myEventsRegistration, myEventsIngressBuffer),
	}
}

// Run subscribes to both internal live-delivery roots and processes their
// messages in arrival order. It is started once by ChattoCore.Run.
func (h *MyEventsHub) Run(ctx context.Context) error {
	msgChan := make(chan *nats.Msg, myEventsIngressBuffer)
	h.model.core.logger.Debug("myEvents hub started")
	defer func() {
		h.quarantine("hub stopped")
		h.model.core.logger.Debug("myEvents hub stopped")
	}()

	for {
		liveSyncSub, err := h.model.core.nc.ChanSubscribe(subjects.LiveSyncAllEvents(), msgChan)
		if err != nil {
			return fmt.Errorf("myEvents hub: subscribe to live sync events: %w", err)
		}
		liveEVTSub, err := h.model.core.nc.ChanSubscribe(events.LiveSubjectRoot+">", msgChan)
		if err != nil {
			liveSyncSub.Unsubscribe()
			return fmt.Errorf("myEvents hub: subscribe to live EVT events: %w", err)
		}
		if err := h.flushIngress(ctx); err != nil {
			liveSyncSub.Unsubscribe()
			liveEVTSub.Unsubscribe()
			return fmt.Errorf("myEvents hub: activate ingress: %w", err)
		}
		h.beginGeneration()
		h.readyOnce.Do(func() { close(h.ready) })

		reason, runErr := h.runGeneration(ctx, msgChan, liveSyncSub, liveEVTSub)
		if runErr != nil {
			liveSyncSub.Unsubscribe()
			liveEVTSub.Unsubscribe()
			return runErr
		}
		h.quarantine(reason)
		liveSyncSub.Unsubscribe()
		liveEVTSub.Unsubscribe()
		if err := h.flushIngress(ctx); err != nil {
			return fmt.Errorf("myEvents hub: reset ingress: %w", err)
		}
		h.drainStaleIngress(msgChan)
	}
}

func (h *MyEventsHub) flushIngress(ctx context.Context) error {
	flushCtx, cancel := context.WithTimeout(ctx, myEventsIngressFlushTimeout)
	defer cancel()
	return h.model.core.nc.FlushWithContext(flushCtx)
}

func (h *MyEventsHub) runGeneration(
	ctx context.Context,
	msgChan <-chan *nats.Msg,
	liveSyncSub, liveEVTSub *nats.Subscription,
) (string, error) {
	slowSyncConsumerCh := liveSyncSub.StatusChanged(nats.SubscriptionSlowConsumer)
	slowEVTConsumerCh := liveEVTSub.StatusChanged(nats.SubscriptionSlowConsumer)
	for {
		select {
		case <-ctx.Done():
			return "hub stopped", ctx.Err()
		case <-slowEVTConsumerCh:
			dropped, _ := liveEVTSub.Dropped()
			h.model.core.logger.Warn("Slow consumer on process-wide live EVT subscription", "dropped", dropped)
			return "live EVT ingress discontinuity", nil
		case <-slowSyncConsumerCh:
			dropped, _ := liveSyncSub.Dropped()
			h.model.core.logger.Warn("Slow consumer on process-wide live sync subscription", "dropped", dropped)
			return "live sync ingress discontinuity", nil
		case request := <-h.registrations:
			// Preserve the old per-subscription live boundary: anything the
			// process had already received before admission is not delivered to
			// the new session. Snapshot the count so sustained traffic cannot
			// starve registration.
			queued := len(msgChan)
			for range queued {
				msg := <-msgChan
				if msg != nil && h.handleMessage(ctx, msg) {
					return "projection readiness discontinuity", nil
				}
			}
			h.handleRegistration(request)
		case msg := <-msgChan:
			if msg == nil {
				continue
			}
			if h.handleMessage(ctx, msg) {
				return "projection readiness discontinuity", nil
			}
		}
	}
}

func (h *MyEventsHub) Subscribe(ctx context.Context, userID string) (*myEventsSubscription, error) {
	select {
	case <-h.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	for {
		h.mu.Lock()
		visibilityVersion := h.visibilityVersion
		state, existingUser := h.users[userID]
		needsVisibleRooms := state == nil || state.visibleRooms == nil
		h.mu.Unlock()
		if existingUser {
			var visibleRooms map[string]struct{}
			if needsVisibleRooms {
				var err error
				visibleRooms, err = h.captureVisibleRooms(ctx, userID)
				if err != nil {
					return nil, err
				}
			}
			sub := newMyEventsSubscription(userID)
			if err := h.registerAtIngressBoundary(ctx, sub, nil, visibleRooms, 0, visibilityVersion); err != nil {
				if errors.Is(err, errMyEventsIngressChanged) {
					continue
				}
				return nil, err
			}
			return sub, nil
		}
		memberRooms, roomSnapshotSeq, err := h.captureVisibilitySnapshot(ctx, userID)
		if err != nil {
			return nil, err
		}
		visibleRooms, err := h.captureVisibleRooms(ctx, userID)
		if err != nil {
			return nil, err
		}
		sub := newMyEventsSubscription(userID)
		if err := h.registerAtIngressBoundary(ctx, sub, memberRooms, visibleRooms, roomSnapshotSeq, visibilityVersion); err != nil {
			if errors.Is(err, errMyEventsIngressChanged) {
				continue
			}
			return nil, err
		}
		return sub, nil
	}
}

func newMyEventsSubscription(userID string) *myEventsSubscription {
	ch := make(chan myEventsDelivery, myEventsSubscriberBuffer)
	done := make(chan struct{})
	return &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, userID: userID}
}

func (h *MyEventsHub) registerAtIngressBoundary(ctx context.Context, sub *myEventsSubscription, memberRooms, visibleRooms map[string]struct{}, roomSnapshotSeq, visibilityVersion uint64) error {
	for {
		h.mu.Lock()
		if !h.accepting {
			changed := h.stateChanged
			h.mu.Unlock()
			select {
			case <-changed:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		request := &myEventsRegistration{
			generation:        h.generation,
			visibilityVersion: visibilityVersion,
			roomSnapshotSeq:   roomSnapshotSeq,
			userID:            sub.userID,
			memberRooms:       memberRooms,
			visibleRooms:      visibleRooms,
			sub:               sub,
			ctx:               ctx,
			result:            make(chan error, 1),
		}
		changed := h.stateChanged
		h.mu.Unlock()

		select {
		case h.registrations <- request:
		case <-changed:
			return errMyEventsIngressChanged
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case err := <-request.result:
			return err
		case <-changed:
			h.Unsubscribe(sub)
			return errMyEventsIngressChanged
		case <-ctx.Done():
			h.Unsubscribe(sub)
			return ctx.Err()
		}
	}
}

func (h *MyEventsHub) Unsubscribe(sub *myEventsSubscription) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	h.removeSubscriberLocked(sub)
	h.mu.Unlock()
}

func (h *MyEventsHub) handleRegistration(request *myEventsRegistration) {
	if request == nil {
		return
	}
	h.mu.Lock()
	if request.ctx.Err() != nil {
		h.mu.Unlock()
		request.result <- request.ctx.Err()
		return
	}
	if !h.accepting || request.generation != h.generation || request.visibilityVersion != h.visibilityVersion {
		h.mu.Unlock()
		request.result <- errMyEventsIngressChanged
		return
	}
	state := h.users[request.userID]
	if state == nil {
		if request.memberRooms == nil {
			h.mu.Unlock()
			request.result <- errMyEventsIngressChanged
			return
		}
		state = &myEventsUserState{
			memberRooms:     request.memberRooms,
			visibleRooms:    request.visibleRooms,
			roomSnapshotSeq: request.roomSnapshotSeq,
			subscribers:     make(map[uint64]*myEventsSubscription),
		}
		h.users[request.userID] = state
	} else if request.visibleRooms != nil {
		state.visibleRooms = request.visibleRooms
	}
	h.nextID++
	request.sub.id = h.nextID
	state.subscribers[request.sub.id] = request.sub
	h.subscribers[request.sub.id] = request.sub
	h.mu.Unlock()
	request.result <- nil
}

func (h *MyEventsHub) consume(sub *myEventsSubscription, delivery myEventsDelivery) {
	if sub != nil && delivery.bytes > 0 {
		sub.queuedBytes.Add(-delivery.bytes)
	}
}

// handleMessage returns true when projection-safe delivery could not be
// established and every current subscriber must reconnect and catch up.
func (h *MyEventsHub) handleMessage(ctx context.Context, msg *nats.Msg) bool {
	if strings.HasPrefix(msg.Subject, "live.sync.") {
		return h.handleLiveSync(msg)
	}
	if strings.HasPrefix(msg.Subject, events.LiveSubjectRoot) {
		return h.handleLiveEVT(ctx, msg)
	}
	h.model.core.logger.Warn("Unknown live event subject root", "subject", msg.Subject)
	return false
}

func (h *MyEventsHub) handleLiveSync(msg *nats.Msg) bool {
	h.decoded.Add(1)
	var event corev1.LiveEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		h.model.core.logger.Warn("Failed to unmarshal live sync event", "subject", msg.Subject, "error", err)
		return false
	}
	if event.Event == nil {
		h.model.core.logger.Warn("Dropping live sync event without payload", "subject", msg.Subject)
		return false
	}

	bytes := int64(len(msg.Data))
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID, state := range h.users {
		authorized, ok := h.model.filterLiveSyncEvent(context.Background(), userID, state.memberRooms, msg, &event)
		if ok {
			h.enqueueUserLocked(state, authorized, bytes)
		}
	}
	return false
}

func (h *MyEventsHub) handleLiveEVT(ctx context.Context, msg *nats.Msg) bool {
	evtSubject := events.SubjectRoot + strings.TrimPrefix(msg.Subject, events.LiveSubjectRoot)
	isRBACSubject := strings.HasPrefix(evtSubject, strings.TrimSuffix(events.RBACSubjectFilter(), ">"))
	if !isRBACSubject {
		eventType := liveEventType(msg.Subject)
		_, roomSubject := events.ParseRoomSubject(msg.Subject)
		_, assetSubject := events.ParseAssetSubject(msg.Subject)
		_, userSubject := events.ParseUserSubject(msg.Subject)
		if roomSubject && !isDeliverableLiveEVTRoomEventType(eventType) {
			h.prefiltered.Add(1)
			return false
		}
		if assetSubject && !isDeliverableLiveEVTAssetEventType(eventType) {
			h.prefiltered.Add(1)
			return false
		}
		if userSubject && !isDeliverableLiveEVTUserEventType(eventType) {
			h.prefiltered.Add(1)
			return false
		}
		if !roomSubject && !assetSubject && !userSubject {
			h.prefiltered.Add(1)
			return false
		}
	}

	seq := liveEVTMsgSeq(msg)
	if seq == 0 {
		h.model.core.logger.Warn("Deliverable live EVT message missing stream sequence", "subject", msg.Subject, "sequence", msg.Header.Get(nats.JSSequence))
		return true
	}

	if isRBACSubject {
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := h.model.core.rbacModel.waitFor(waitCtx, events.SubjectPosition(events.RBACSubjectFilter(), seq)); err != nil {
			h.model.core.logger.Warn("Live EVT RBAC projection readiness failed", "subject", msg.Subject, "sequence", seq, "error", err)
			return true
		}
		if err := h.refreshMemberRooms(waitCtx); err != nil {
			h.model.core.logger.Warn("Live EVT room visibility refresh failed", "subject", msg.Subject, "sequence", seq, "error", err)
			return true
		}
		var event corev1.Event
		if err := proto.Unmarshal(msg.Data, &event); err != nil {
			h.model.core.logger.Warn("Failed to unmarshal live RBAC event", "subject", msg.Subject, "error", err)
			return true
		}
		// Protocol v2 turns this durable fact into a projection reset while
		// legacy clients ignore the unsupported internal payload and stay live.
		h.fanoutAll(NewEVTEventEnvelopeWithDeliverySeq(&event, seq), int64(len(msg.Data)))
		return false
	}

	eventType := liveEventType(msg.Subject)
	roomID, roomSubject := events.ParseRoomSubject(msg.Subject)
	_, assetSubject := events.ParseAssetSubject(msg.Subject)

	h.decoded.Add(1)
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		h.model.core.logger.Warn("Failed to unmarshal live event", "subject", msg.Subject, "error", err)
		return true
	}
	if payloadType := events.EventTypeOf(&event); payloadType != eventType {
		h.model.core.logger.Warn("Live EVT subject and payload types disagree", "subject", msg.Subject, "subject_type", eventType, "payload_type", payloadType)
		return true
	}
	bytes := int64(len(msg.Data))

	if roomSubject {
		if !isDeliverableLiveEVTRoomEvent(&event) {
			return true
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := h.model.waitForLiveEVTRoomEvent(waitCtx, evtSubject, &event, seq); err != nil {
			h.model.core.logger.Warn("Live EVT projection readiness failed", "subject", msg.Subject, "sequence", seq, "error", err)
			return true
		}
		h.fanoutReadyRoomEvent(ctx, roomID, &event, seq, bytes)
		return false
	}

	if assetSubject {
		if !isDeliverableLiveEVTAssetEvent(&event) {
			return true
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := h.model.waitForLiveEVTAssetEvent(waitCtx, evtSubject, seq); err != nil {
			h.model.core.logger.Warn("Live EVT asset projection readiness failed", "subject", msg.Subject, "sequence", seq, "error", err)
			return true
		}
		assetID := assetIDOfLifecycleEvent(&event)
		assetRoomID, ok := h.model.core.assetLifecycle().AssetRoomID(assetID)
		if ok {
			h.fanoutReadyAssetEvent(assetRoomID, &event, seq, bytes)
		}
		return false
	}

	if !isDeliverableLiveEVTUserEvent(&event) {
		return true
	}
	waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
	defer cancel()
	if err := h.model.waitForLiveEVTUserEvent(waitCtx, evtSubject, seq); err != nil {
		h.model.core.logger.Warn("Live EVT user projection readiness failed", "subject", msg.Subject, "sequence", seq, "error", err)
		return true
	}
	if event.GetUserKeyShredded() != nil {
		// One shredded author can invalidate plaintext in many room windows.
		// Reconnect all clients so protocol v2 compacts current tombstones.
		return true
	}
	h.fanoutAll(NewEVTEventEnvelopeWithDeliverySeq(&event, seq), bytes)
	return false
}

type roomProjectionFanoutCandidate struct {
	userID     string
	state      *myEventsUserState
	envelope   EventEnvelope
	wasVisible bool
	visible    bool
	err        error
}

func (h *MyEventsHub) fanoutReadyRoomEvent(ctx context.Context, roomID string, event *corev1.Event, seq uint64, bytes int64) {
	h.mu.Lock()
	if eventChangesRoomVisibility(event) {
		h.visibilityVersion++
	}
	candidates := make([]roomProjectionFanoutCandidate, 0, len(h.users))
	for userID, state := range h.users {
		if seq <= state.roomSnapshotSeq {
			continue
		}
		envelope, ok := h.model.filterReadyEVTRoomSubjectEvent(userID, state.memberRooms, roomID, event, seq)
		projectionVisibilityChange := eventChangesUserRoomVisibility(event, userID)
		if !isRoomDirectoryProjectionEvent(event) && !projectionVisibilityChange {
			if ok {
				h.enqueueUserLocked(state, envelope, bytes)
			}
			continue
		}

		if state.visibleRooms == nil {
			continue
		}
		_, wasVisible := state.visibleRooms[roomID]
		candidates = append(candidates, roomProjectionFanoutCandidate{
			userID: userID, state: state, envelope: envelope, wasVisible: wasVisible,
		})
	}
	h.mu.Unlock()

	visibilityCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
	defer cancel()
	g, visibilityCtx := errgroup.WithContext(visibilityCtx)
	g.SetLimit(myEventsVisibilityWorkers)
	for i := range candidates {
		i := i
		g.Go(func() error {
			candidates[i].visible, candidates[i].err = h.canSeeProjectionRoom(visibilityCtx, candidates[i].userID, roomID, event)
			return nil
		})
	}
	_ = g.Wait()

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, candidate := range candidates {
		state := h.users[candidate.userID]
		if state == nil || state != candidate.state || state.visibleRooms == nil {
			continue
		}
		switch {
		case candidate.visible:
			state.visibleRooms[roomID] = struct{}{}
		case candidate.err == nil || errors.Is(candidate.err, ErrNotFound) || errors.Is(candidate.err, ErrPermissionDenied):
			if !candidate.wasVisible {
				continue
			}
			delete(state.visibleRooms, roomID)
		default:
			// An uncertain visibility decision must never disclose a room fact.
			// Reconnect this user's sessions rather than risk disclosing a room.
			for _, sub := range state.subscribers {
				h.removeSubscriberLocked(sub)
			}
			continue
		}

		envelope := candidate.envelope
		if envelope == nil {
			envelope = NewEVTEventEnvelopeWithDeliverySeq(event, seq)
		}
		for _, sub := range state.subscribers {
			h.enqueueSubscriberLocked(sub, envelope, bytes)
		}
	}
}

func (h *MyEventsHub) canSeeProjectionRoom(ctx context.Context, userID, roomID string, event *corev1.Event) (bool, error) {
	if event.GetRoomDeleted() != nil || event.GetRoomArchived() != nil {
		return false, nil
	}
	room, err := h.model.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return false, err
	}
	if room.GetArchived() {
		return false, nil
	}
	kind := KindOfRoom(room)
	if kind == KindDM {
		return h.model.core.RoomMembershipExists(ctx, kind, userID, roomID)
	}
	return h.model.core.CanSeeRoom(ctx, userID, kind, roomID)
}

func (h *MyEventsHub) fanoutReadyAssetEvent(roomID string, event *corev1.Event, seq uint64, bytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID, state := range h.users {
		envelope, ok := h.model.filterReadyEVTAssetSubjectEvent(userID, state.memberRooms, roomID, event, seq)
		if ok {
			h.enqueueUserLocked(state, envelope, bytes)
		}
	}
}

func (h *MyEventsHub) fanoutAll(event EventEnvelope, bytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, state := range h.users {
		h.enqueueUserLocked(state, event, bytes)
	}
}

func (h *MyEventsHub) enqueueUserLocked(state *myEventsUserState, event EventEnvelope, bytes int64) {
	for _, sub := range state.subscribers {
		h.enqueueSubscriberLocked(sub, event, bytes)
	}
}

func (h *MyEventsHub) enqueueSubscriberLocked(sub *myEventsSubscription, event EventEnvelope, bytes int64) {
	queuedBytes := sub.queuedBytes.Load()
	if queuedBytes+bytes > myEventsSubscriberByteLimit {
		h.model.slowDisconnects.Add(1)
		h.model.core.logger.Warn("Slow myEvents subscriber exceeded byte limit - tearing down", "user_id", sub.userID, "queued_bytes", queuedBytes, "event_bytes", bytes)
		h.removeSubscriberLocked(sub)
		return
	}
	delivery := myEventsDelivery{event: event, bytes: bytes}
	sub.queuedBytes.Add(bytes)
	select {
	case sub.ch <- delivery:
	default:
		sub.queuedBytes.Add(-bytes)
		h.model.slowDisconnects.Add(1)
		h.model.core.logger.Warn("Slow myEvents subscriber filled event queue - tearing down", "user_id", sub.userID, "queued_bytes", sub.queuedBytes.Load())
		h.removeSubscriberLocked(sub)
	}
}

func (h *MyEventsHub) refreshMemberRooms(ctx context.Context) error {
	h.mu.Lock()
	userIDs := make([]string, 0, len(h.users))
	for userID := range h.users {
		userIDs = append(userIDs, userID)
	}
	h.mu.Unlock()

	refreshed := make([]map[string]struct{}, len(userIDs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(myEventsVisibilityWorkers)
	for i, userID := range userIDs {
		i, userID := i, userID
		g.Go(func() error {
			rooms := make(map[string]struct{})
			if err := h.model.populateMemberRoomsCache(gctx, userID, rooms); err != nil {
				return err
			}
			refreshed[i] = rooms
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	h.mu.Lock()
	for i, userID := range userIDs {
		if state := h.users[userID]; state != nil {
			state.memberRooms = refreshed[i]
		}
	}
	h.visibilityVersion++
	h.mu.Unlock()
	return nil
}

// captureVisibilitySnapshot returns a membership snapshot tied to a stable
// authoritative EVT tail. A second tail read detects facts committed while the
// projections were being read; callers retry until the read is stable.
func (h *MyEventsHub) captureVisibilitySnapshot(ctx context.Context, userID string) (map[string]struct{}, uint64, error) {
	for {
		roomSeqs, roomTail, err := h.roomVisibilityTails(ctx)
		if err != nil {
			return nil, 0, err
		}
		rbacPos, err := h.model.core.EventPublisher.LastSubjectPosition(ctx, events.RBACSubjectFilter())
		if err != nil {
			return nil, 0, fmt.Errorf("read RBAC visibility tail: %w", err)
		}
		if roomTail > 0 {
			if err := h.model.core.rooms().waitForDirectory(ctx, events.SubjectPosition(events.RoomSubjectFilter(), roomTail)); err != nil {
				return nil, 0, fmt.Errorf("wait for room visibility snapshot: %w", err)
			}
		}
		if !rbacPos.IsZero() {
			if err := h.model.core.rbacModel.waitFor(ctx, rbacPos); err != nil {
				return nil, 0, fmt.Errorf("wait for RBAC visibility snapshot: %w", err)
			}
		}

		memberRooms := make(map[string]struct{})
		if err := h.model.populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
			return nil, 0, err
		}

		verifiedRoomSeqs, verifiedRoomTail, err := h.roomVisibilityTails(ctx)
		if err != nil {
			return nil, 0, err
		}
		rbacTail, err := h.model.core.EventPublisher.LastSubjectSeq(ctx, events.RBACSubjectFilter())
		if err != nil {
			return nil, 0, fmt.Errorf("verify RBAC visibility tail: %w", err)
		}
		if roomSeqs == verifiedRoomSeqs && rbacTail == rbacPos.Seq {
			return memberRooms, verifiedRoomTail, nil
		}
	}
}

func (h *MyEventsHub) captureVisibleRooms(ctx context.Context, userID string) (map[string]struct{}, error) {
	rooms, err := h.model.core.RoomDirectoryReads().ListRooms(ctx, userID, RoomDirectoryListOptions{
		IncludeChannels: true,
		IncludeDMs:      true,
		IncludeEmptyDMs: true,
	})
	if err != nil {
		return nil, fmt.Errorf("capture visible rooms: %w", err)
	}
	visibleRooms := make(map[string]struct{}, len(rooms))
	for _, room := range rooms {
		if room != nil && room.Room != nil {
			visibleRooms[room.Room.Id] = struct{}{}
		}
	}
	return visibleRooms, nil
}

type roomVisibilitySeqs [5]uint64

func (h *MyEventsHub) roomVisibilityTails(ctx context.Context) (roomVisibilitySeqs, uint64, error) {
	filters := [...]string{
		events.RoomEventTypeFilter(events.EventRoomCreated),
		events.RoomEventTypeFilter(events.EventRoomDeleted),
		events.RoomEventTypeFilter(events.EventRoomUniversalChanged),
		events.RoomEventTypeFilter(events.EventUserJoinedRoom),
		events.RoomEventTypeFilter(events.EventUserLeftRoom),
	}
	var seqs roomVisibilitySeqs
	var tail uint64
	for i, filter := range filters {
		seq, err := h.model.core.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return roomVisibilitySeqs{}, 0, fmt.Errorf("read room visibility tail for %s: %w", filter, err)
		}
		seqs[i] = seq
		if seq > tail {
			tail = seq
		}
	}
	return seqs, tail, nil
}

func (h *MyEventsHub) beginGeneration() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.generation++
	h.accepting = true
	h.signalStateChangedLocked()
}

func (h *MyEventsHub) quarantine(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.accepting = false
	h.signalStateChangedLocked()
	if len(h.subscribers) > 0 {
		h.model.core.logger.Warn("Closing all myEvents subscribers", "reason", reason, "subscribers", len(h.subscribers))
	}
	for _, sub := range h.subscribers {
		close(sub.done)
		close(sub.ch)
	}
	h.subscribers = make(map[uint64]*myEventsSubscription)
	h.users = make(map[string]*myEventsUserState)
}

func (h *MyEventsHub) signalStateChangedLocked() {
	close(h.stateChanged)
	h.stateChanged = make(chan struct{})
}

func (h *MyEventsHub) drainStaleIngress(msgChan <-chan *nats.Msg) {
	for {
		select {
		case <-msgChan:
		default:
			return
		}
	}
}

func (h *MyEventsHub) removeSubscriberLocked(sub *myEventsSubscription) {
	if _, ok := h.subscribers[sub.id]; !ok {
		return
	}
	delete(h.subscribers, sub.id)
	if state := h.users[sub.userID]; state != nil {
		delete(state.subscribers, sub.id)
		if len(state.subscribers) == 0 {
			delete(h.users, sub.userID)
		}
	}
	close(sub.done)
	close(sub.ch)
}

func liveEventType(subject string) string {
	if i := strings.LastIndexByte(subject, '.'); i >= 0 && i < len(subject)-1 {
		return subject[i+1:]
	}
	return ""
}

func eventChangesRoomVisibility(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch event.Event.(type) {
	case *corev1.Event_RoomCreated,
		*corev1.Event_RoomDeleted,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_RoomUniversalChanged,
		*corev1.Event_UserJoinedRoom,
		*corev1.Event_UserLeftRoom,
		*corev1.Event_RoomMemberAdded,
		*corev1.Event_RoomMemberRemoved,
		*corev1.Event_RoomMemberBanned:
		return true
	default:
		return false
	}
}

func eventChangesUserRoomVisibility(event *corev1.Event, userID string) bool {
	if event == nil {
		return false
	}
	switch payload := event.Event.(type) {
	case *corev1.Event_RoomCreated,
		*corev1.Event_RoomDeleted,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_RoomUniversalChanged:
		return true
	case *corev1.Event_UserJoinedRoom, *corev1.Event_UserLeftRoom:
		return event.GetActorId() == userID
	case *corev1.Event_RoomMemberAdded:
		return payload.RoomMemberAdded.GetUserId() == userID
	case *corev1.Event_RoomMemberRemoved:
		return payload.RoomMemberRemoved.GetUserId() == userID
	case *corev1.Event_RoomMemberBanned:
		return payload.RoomMemberBanned.GetUserId() == userID
	default:
		return false
	}
}

func isRoomDirectoryProjectionEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch event.Event.(type) {
	case *corev1.Event_RoomCreated,
		*corev1.Event_RoomUpdated,
		*corev1.Event_RoomDeleted,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_RoomUniversalChanged:
		return true
	default:
		return false
	}
}
