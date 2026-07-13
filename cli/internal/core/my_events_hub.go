package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

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
	memberRooms map[string]struct{}
	subscribers map[uint64]*myEventsSubscription
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
}

func NewMyEventsHub(model *MyEventsModel) *MyEventsHub {
	return &MyEventsHub{
		model:       model,
		users:       make(map[string]*myEventsUserState),
		subscribers: make(map[uint64]*myEventsSubscription),
		ready:       make(chan struct{}),
	}
}

// Run subscribes to both internal live-delivery roots and processes their
// messages in arrival order. It is started once by ChattoCore.Run.
func (h *MyEventsHub) Run(ctx context.Context) error {
	msgChan := make(chan *nats.Msg, myEventsIngressBuffer)
	liveSyncSub, err := h.model.core.nc.ChanSubscribe(subjects.LiveSyncAllEvents(), msgChan)
	if err != nil {
		return fmt.Errorf("myEvents hub: subscribe to live sync events: %w", err)
	}
	defer liveSyncSub.Unsubscribe()

	liveEVTSub, err := h.model.core.nc.ChanSubscribe(events.LiveSubjectRoot+">", msgChan)
	if err != nil {
		return fmt.Errorf("myEvents hub: subscribe to live EVT events: %w", err)
	}
	defer liveEVTSub.Unsubscribe()

	slowSyncConsumerCh := liveSyncSub.StatusChanged(nats.SubscriptionSlowConsumer)
	slowEVTConsumerCh := liveEVTSub.StatusChanged(nats.SubscriptionSlowConsumer)
	h.readyOnce.Do(func() { close(h.ready) })
	h.model.core.logger.Debug("myEvents hub started")
	defer func() {
		h.disconnectAll("hub stopped")
		h.model.core.logger.Debug("myEvents hub stopped")
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-slowEVTConsumerCh:
			dropped, _ := liveEVTSub.Dropped()
			h.model.core.logger.Warn("Slow consumer on process-wide live EVT subscription", "dropped", dropped)
			h.disconnectAll("live EVT ingress discontinuity")
		case <-slowSyncConsumerCh:
			dropped, _ := liveSyncSub.Dropped()
			h.model.core.logger.Warn("Slow consumer on process-wide live sync subscription", "dropped", dropped)
			h.disconnectAll("live sync ingress discontinuity")
		case msg := <-msgChan:
			if msg == nil {
				continue
			}
			if h.handleMessage(ctx, msg) {
				h.disconnectAll("projection readiness discontinuity")
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

	ch := make(chan myEventsDelivery, myEventsSubscriberBuffer)
	done := make(chan struct{})
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.users[userID]
	if state == nil {
		memberRooms := make(map[string]struct{})
		// Holding the hub lock closes the gap between the projection snapshot
		// and registration: live events queue at the dispatcher until this user
		// state is visible, then update it in order.
		if err := h.model.populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
			return nil, err
		}
		state = &myEventsUserState{
			memberRooms: memberRooms,
			subscribers: make(map[uint64]*myEventsSubscription),
		}
		h.users[userID] = state
	}
	id := h.nextID
	h.nextID++
	sub := &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, id: id, userID: userID}
	state.subscribers[id] = sub
	h.subscribers[id] = sub
	return sub, nil
}

func (h *MyEventsHub) Unsubscribe(sub *myEventsSubscription) {
	if sub == nil {
		return
	}
	h.mu.Lock()
	h.removeSubscriberLocked(sub)
	h.mu.Unlock()
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
	seq := liveEVTMsgSeq(msg)
	if seq == 0 {
		h.model.core.logger.Warn("live EVT message missing stream sequence", "subject", msg.Subject, "sequence", msg.Header.Get(nats.JSSequence))
		return false
	}
	evtSubject := events.SubjectRoot + strings.TrimPrefix(msg.Subject, events.LiveSubjectRoot)

	if strings.HasPrefix(evtSubject, strings.TrimSuffix(events.RBACSubjectFilter(), ">")) {
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
		return false
	}

	eventType := liveEventType(msg.Subject)
	roomID, roomSubject := events.ParseRoomSubject(msg.Subject)
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

	h.decoded.Add(1)
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		h.model.core.logger.Warn("Failed to unmarshal live event", "subject", msg.Subject, "error", err)
		return false
	}
	bytes := int64(len(msg.Data))

	if roomSubject {
		if !isDeliverableLiveEVTRoomEvent(&event) {
			return false
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := h.model.waitForLiveEVTRoomEvent(waitCtx, evtSubject, &event, seq); err != nil {
			h.model.core.logger.Warn("Live EVT projection readiness failed", "subject", msg.Subject, "sequence", seq, "error", err)
			return true
		}
		h.fanoutReadyRoomEvent(roomID, &event, seq, bytes)
		return false
	}

	if assetSubject {
		if !isDeliverableLiveEVTAssetEvent(&event) {
			return false
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
		return false
	}
	waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
	defer cancel()
	if err := h.model.waitForLiveEVTUserEvent(waitCtx, evtSubject, seq); err != nil {
		h.model.core.logger.Warn("Live EVT user projection readiness failed", "subject", msg.Subject, "sequence", seq, "error", err)
		return true
	}
	h.fanoutAll(NewEVTEventEnvelopeWithDeliverySeq(&event, seq), bytes)
	return false
}

func (h *MyEventsHub) fanoutReadyRoomEvent(roomID string, event *corev1.Event, seq uint64, bytes int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for userID, state := range h.users {
		envelope, ok := h.model.filterReadyEVTRoomSubjectEvent(userID, state.memberRooms, roomID, event, seq)
		if ok {
			h.enqueueUserLocked(state, envelope, bytes)
		}
	}
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
		queuedBytes := sub.queuedBytes.Load()
		if queuedBytes+bytes > myEventsSubscriberByteLimit {
			h.model.slowDisconnects.Add(1)
			h.model.core.logger.Warn("Slow myEvents subscriber exceeded byte limit - tearing down", "user_id", sub.userID, "queued_bytes", queuedBytes, "event_bytes", bytes)
			h.removeSubscriberLocked(sub)
			continue
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
	h.mu.Unlock()
	return nil
}

func (h *MyEventsHub) disconnectAll(reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
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
