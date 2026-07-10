package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	// liveEVTProjectionWaitTimeout bounds the causal barrier between JetStream's
	// raw EVT republish and realtime delivery. In the normal case the local
	// projectors have already advanced and WaitFor returns immediately; the
	// timeout covers replica lag or a stuck projector without wedging a
	// subscription goroutine forever.
	liveEVTProjectionWaitTimeout = 2 * time.Second

	// MyEventsHeartbeatInterval controls the synthetic heartbeat cadence used by
	// StreamMyEvents and advertised by the realtime WebSocket protocol.
	MyEventsHeartbeatInterval = 15 * time.Second

	// liveDispatchQueueSize bounds prepared events waiting behind one stream.
	// A full queue disconnects only that stream so reconnect catch-up can restore
	// projected state without slowing the process-wide live intake.
	liveDispatchQueueSize = 64
)

type preparedLiveEventKind uint8

const (
	preparedLiveSync preparedLiveEventKind = iota + 1
	preparedLiveEVTRoom
	preparedLiveEVTAsset
	preparedLiveEVTUser
	preparedPresence
)

type preparedLiveEvent struct {
	kind     preparedLiveEventKind
	subject  string
	roomID   string
	envelope EventEnvelope
}

type liveDispatchEndReason uint8

const (
	liveDispatchStopped liveDispatchEndReason = iota + 1
	liveDispatchSubscriberSlow
	liveDispatchSourceGap
	liveDispatchProjectionFailure
)

type liveDispatchSubscription struct {
	C      <-chan *preparedLiveEvent
	ch     chan *preparedLiveEvent
	Done   <-chan struct{}
	done   chan struct{}
	reason liveDispatchEndReason
	id     uint64
}

// MyEventsModel owns the server-side myEvents live stream machinery.
//
// ChattoCore remains the public facade, while this model keeps live root
// filtering, projection readiness, and per-subscription room membership state
// together.
type MyEventsModel struct {
	core              *ChattoCore
	activeStreams     atomic.Int64
	deliveredEvents   atomic.Uint64
	slowDisconnects   atomic.Uint64
	presenceRefreshes atomic.Uint64
	presenceFailures  atomic.Uint64

	dispatchMu          sync.Mutex
	dispatchSubscribers map[uint64]*liveDispatchSubscription
	nextDispatchID      uint64
	dispatchReady       chan struct{}
	dispatchReadyOnce   sync.Once
	dispatchReadyErr    error
	dispatchStopped     bool
}

func NewMyEventsModel(core *ChattoCore) *MyEventsModel {
	return &MyEventsModel{
		core:                core,
		dispatchSubscribers: make(map[uint64]*liveDispatchSubscription),
		dispatchReady:       make(chan struct{}),
	}
}

// StreamMyEventsOptions controls compatibility behavior for a myEvents stream.
type StreamMyEventsOptions struct {
	// TouchPresence preserves the legacy behavior where opening myEvents marks
	// the user online and refreshes the current presence value. New clients that
	// refresh presence through ConnectRPC set this to false.
	TouchPresence bool
}

func (c *ChattoCore) myEvents() *MyEventsModel {
	if c.myEventsModel == nil {
		c.myEventsModel = NewMyEventsModel(c)
	}
	return c.myEventsModel
}

// MyEventsMetrics is a process-local snapshot of the realtime event stream.
type MyEventsMetrics struct {
	ActiveStreams     int64
	DeliveredEvents   uint64
	SlowDisconnects   uint64
	PresenceRefreshes uint64
	PresenceFailures  uint64
}

// MyEventsMetrics returns process-local live-event stream counters.
func (c *ChattoCore) MyEventsMetrics() MyEventsMetrics {
	if c.myEventsModel == nil {
		return MyEventsMetrics{}
	}
	return c.myEventsModel.Metrics()
}

// Metrics returns process-local live-event stream counters.
func (s *MyEventsModel) Metrics() MyEventsMetrics {
	return MyEventsMetrics{
		ActiveStreams:     s.activeStreams.Load(),
		DeliveredEvents:   s.deliveredEvents.Load(),
		SlowDisconnects:   s.slowDisconnects.Load(),
		PresenceRefreshes: s.presenceRefreshes.Load(),
		PresenceFailures:  s.presenceFailures.Load(),
	}
}

// Run owns the process-wide live event intake. Raw NATS messages and presence
// transitions are decoded or adapted once, projection-gated once, and then
// fanned out as immutable prepared events to every active user stream.
func (s *MyEventsModel) Run(ctx context.Context) error {
	presenceSub, err := s.core.presenceModel.Subscribe(ctx)
	if err != nil {
		return s.failDispatchStartup(fmt.Errorf("subscribe to presence hub: %w", err))
	}
	defer s.core.presenceModel.Unsubscribe(presenceSub)

	msgChan := make(chan *nats.Msg, 256)
	liveSyncSub, err := s.core.nc.ChanSubscribe(subjects.LiveSyncAllEvents(), msgChan)
	if err != nil {
		return s.failDispatchStartup(fmt.Errorf("subscribe to live sync events: %w", err))
	}
	defer liveSyncSub.Unsubscribe()

	liveEVTSub, err := s.core.nc.ChanSubscribe(events.LiveSubjectRoot+">", msgChan)
	if err != nil {
		return s.failDispatchStartup(fmt.Errorf("subscribe to live EVT events: %w", err))
	}
	defer liveEVTSub.Unsubscribe()

	if err := s.core.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return s.failDispatchStartup(fmt.Errorf("flush live event subscriptions: %w", err))
	}

	slowSyncConsumerCh := liveSyncSub.StatusChanged(nats.SubscriptionSlowConsumer)
	slowEVTConsumerCh := liveEVTSub.StatusChanged(nats.SubscriptionSlowConsumer)
	s.finishDispatchStartup(nil)
	defer s.stopDispatcher()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case _, ok := <-slowEVTConsumerCh:
			if !ok {
				slowEVTConsumerCh = nil
				continue
			}
			dropped, _ := liveEVTSub.Dropped()
			disconnected := s.resetDispatchSubscribers(liveDispatchSourceGap, true)
			s.core.logger.Warn("Slow consumer on shared live EVT subscription - resetting streams",
				"dropped", dropped, "streams", disconnected)

		case _, ok := <-slowSyncConsumerCh:
			if !ok {
				slowSyncConsumerCh = nil
				continue
			}
			dropped, _ := liveSyncSub.Dropped()
			disconnected := s.resetDispatchSubscribers(liveDispatchSourceGap, true)
			s.core.logger.Warn("Slow consumer on shared live sync subscription - resetting streams",
				"dropped", dropped, "streams", disconnected)

		case update, ok := <-presenceSub.C:
			if !ok {
				return errors.New("presence hub subscription closed")
			}
			live := newLiveEvent(update.UserID, &corev1.LiveEvent{
				Event: &corev1.LiveEvent_PresenceChanged{
					PresenceChanged: &corev1.PresenceChangedEvent{Status: update.Status},
				},
			})
			s.broadcastPreparedLiveEvent(&preparedLiveEvent{
				kind:     preparedPresence,
				envelope: NewLiveEventEnvelope(live),
			})

		case msg := <-msgChan:
			prepared, ok, err := s.prepareLiveMessage(ctx, msg)
			if err != nil {
				disconnected := s.resetDispatchSubscribers(liveDispatchProjectionFailure, false)
				s.core.logger.Warn("Shared live event projection readiness failed - resetting streams",
					"subject", msg.Subject, "streams", disconnected, "error", err)
				continue
			}
			if ok {
				s.broadcastPreparedLiveEvent(prepared)
			}
		}
	}
}

func (s *MyEventsModel) finishDispatchStartup(err error) {
	s.dispatchMu.Lock()
	s.dispatchReadyErr = err
	if err != nil {
		s.dispatchStopped = true
	}
	s.dispatchReadyOnce.Do(func() { close(s.dispatchReady) })
	s.dispatchMu.Unlock()
}

func (s *MyEventsModel) failDispatchStartup(err error) error {
	s.finishDispatchStartup(err)
	return err
}

func (s *MyEventsModel) stopDispatcher() {
	s.dispatchMu.Lock()
	s.dispatchStopped = true
	for id, sub := range s.dispatchSubscribers {
		delete(s.dispatchSubscribers, id)
		s.endDispatchSubscriptionLocked(sub, liveDispatchStopped)
	}
	s.dispatchMu.Unlock()
}

func (s *MyEventsModel) subscribeToLiveDispatch(ctx context.Context) (*liveDispatchSubscription, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.dispatchReady:
	}

	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	if s.dispatchReadyErr != nil {
		return nil, s.dispatchReadyErr
	}
	if s.dispatchStopped {
		return nil, errors.New("live event dispatcher is stopped")
	}

	ch := make(chan *preparedLiveEvent, liveDispatchQueueSize)
	done := make(chan struct{})
	sub := &liveDispatchSubscription{
		C:    ch,
		ch:   ch,
		Done: done,
		done: done,
		id:   s.nextDispatchID,
	}
	s.nextDispatchID++
	s.dispatchSubscribers[sub.id] = sub
	return sub, nil
}

func (s *MyEventsModel) unsubscribeFromLiveDispatch(sub *liveDispatchSubscription) {
	if sub == nil {
		return
	}
	s.dispatchMu.Lock()
	if s.dispatchSubscribers[sub.id] == sub {
		delete(s.dispatchSubscribers, sub.id)
	}
	s.dispatchMu.Unlock()
}

func (s *MyEventsModel) broadcastPreparedLiveEvent(event *preparedLiveEvent) {
	if event == nil || event.envelope == nil {
		return
	}

	s.dispatchMu.Lock()
	for id, sub := range s.dispatchSubscribers {
		select {
		case sub.ch <- event:
		default:
			delete(s.dispatchSubscribers, id)
			s.endDispatchSubscriptionLocked(sub, liveDispatchSubscriberSlow)
			s.slowDisconnects.Add(1)
		}
	}
	s.dispatchMu.Unlock()
}

func (s *MyEventsModel) resetDispatchSubscribers(reason liveDispatchEndReason, slow bool) int {
	s.dispatchMu.Lock()
	disconnected := len(s.dispatchSubscribers)
	for id, sub := range s.dispatchSubscribers {
		delete(s.dispatchSubscribers, id)
		s.endDispatchSubscriptionLocked(sub, reason)
	}
	s.dispatchMu.Unlock()
	if slow && disconnected > 0 {
		s.slowDisconnects.Add(uint64(disconnected))
	}
	return disconnected
}

func (s *MyEventsModel) endDispatchSubscriptionLocked(sub *liveDispatchSubscription, reason liveDispatchEndReason) {
	if sub == nil || sub.reason != 0 {
		return
	}
	sub.reason = reason
	close(sub.done)
}

func (s *MyEventsModel) prepareLiveMessage(ctx context.Context, msg *nats.Msg) (*preparedLiveEvent, bool, error) {
	if msg == nil {
		return nil, false, nil
	}
	if strings.HasPrefix(msg.Subject, "live.sync.") {
		var live corev1.LiveEvent
		if err := proto.Unmarshal(msg.Data, &live); err != nil {
			s.core.logger.Warn("Failed to unmarshal live sync event", "subject", msg.Subject, "error", err)
			return nil, false, nil
		}
		if live.Event == nil {
			s.core.logger.Warn("Dropping live sync event without payload", "subject", msg.Subject)
			return nil, false, nil
		}
		return &preparedLiveEvent{
			kind:     preparedLiveSync,
			subject:  msg.Subject,
			envelope: NewLiveEventEnvelope(&live),
		}, true, nil
	}

	if !strings.HasPrefix(msg.Subject, events.LiveSubjectRoot) {
		s.core.logger.Warn("Unknown live event subject root", "subject", msg.Subject)
		return nil, false, nil
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		s.core.logger.Warn("Failed to unmarshal live event", "subject", msg.Subject, "error", err)
		return nil, false, nil
	}
	return s.prepareLiveEVTEvent(ctx, msg, &event)
}

func (s *MyEventsModel) prepareLiveEVTEvent(ctx context.Context, msg *nats.Msg, event *corev1.Event) (*preparedLiveEvent, bool, error) {
	seq := liveEVTMsgSeq(msg)
	if seq == 0 {
		s.core.logger.Warn("live EVT message missing stream sequence", "subject", msg.Subject, "sequence", msg.Header.Get(nats.JSSequence))
		return nil, false, nil
	}

	evtSubject := events.SubjectRoot + strings.TrimPrefix(msg.Subject, events.LiveSubjectRoot)
	if roomID, ok := events.ParseRoomSubject(msg.Subject); ok {
		if !isDeliverableLiveEVTRoomEvent(event) {
			return nil, false, nil
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := s.waitForLiveEVTRoomEvent(waitCtx, evtSubject, event, seq); err != nil {
			return nil, false, fmt.Errorf("wait for room event at sequence %d: %w", seq, err)
		}
		return &preparedLiveEvent{
			kind:     preparedLiveEVTRoom,
			subject:  msg.Subject,
			roomID:   roomID,
			envelope: NewEVTEventEnvelopeWithDeliverySeq(event, seq),
		}, true, nil
	}

	if _, ok := events.ParseAssetSubject(msg.Subject); ok {
		if !isDeliverableLiveEVTAssetEvent(event) {
			return nil, false, nil
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := s.waitForLiveEVTAssetEvent(waitCtx, evtSubject, seq); err != nil {
			return nil, false, fmt.Errorf("wait for asset event at sequence %d: %w", seq, err)
		}
		assetID := assetIDOfLifecycleEvent(event)
		roomID, ok := s.core.assetLifecycle().AssetRoomID(assetID)
		if !ok {
			return nil, false, nil
		}
		return &preparedLiveEvent{
			kind:     preparedLiveEVTAsset,
			subject:  msg.Subject,
			roomID:   roomID,
			envelope: NewEVTEventEnvelopeWithDeliverySeq(event, seq),
		}, true, nil
	}

	if _, ok := events.ParseUserSubject(msg.Subject); ok {
		if !isDeliverableLiveEVTUserEvent(event) {
			return nil, false, nil
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		if err := s.waitForLiveEVTUserEvent(waitCtx, evtSubject, seq); err != nil {
			return nil, false, fmt.Errorf("wait for user event at sequence %d: %w", seq, err)
		}
		return &preparedLiveEvent{
			kind:     preparedLiveEVTUser,
			subject:  msg.Subject,
			envelope: NewEVTEventEnvelopeWithDeliverySeq(event, seq),
		}, true, nil
	}

	return nil, false, nil
}

// StreamMyEvents creates a unified stream of every event on this deployment
// that is relevant to a specific user.
//
// Events arrive from the process-wide dispatcher, which owns the NATS Core
// subscriptions for live.sync.> and live.evt.> plus one PresenceHub
// subscription. The dispatcher decodes and projection-gates each event once;
// this stream applies only the subscribing user's authorization and membership
// transitions before forwarding the event through the realtime API.
//
// Authorization:
//   - Room events (live.sync.room.> and deliverable live.evt.room.>) are
//     delivered only for rooms where the user is a member. The membership set
//     is pre-loaded across both kinds (channel + dm) and updated as
//     join/leave/room-deleted events arrive.
//   - User/config/member subjects are filtered by isAuthorizedForLiveEvent.
//   - Presence updates from the per-process PresenceHub are deployment-wide;
//     the hub dedups status flapping.
//
// The subscription also tracks presence liveness: subscribing implies the user
// is online, and a ticker refreshes the KV TTL while the connection lives. A
// synthetic Heartbeat is emitted every 15s so clients can detect a dead
// subscription on an otherwise-healthy WebSocket.
//
// The returned channel closes when the context is cancelled or when a
// SessionTerminatedEvent is delivered to the user.
func (c *ChattoCore) StreamMyEvents(ctx context.Context, userID string) (<-chan EventEnvelope, error) {
	return c.StreamMyEventsWithOptions(ctx, userID, StreamMyEventsOptions{TouchPresence: true})
}

// StreamMyEventsWithOptions creates a myEvents stream with explicit compatibility options.
func (c *ChattoCore) StreamMyEventsWithOptions(ctx context.Context, userID string, options StreamMyEventsOptions) (<-chan EventEnvelope, error) {
	return c.myEvents().StreamMyEvents(ctx, userID, options)
}

func (s *MyEventsModel) StreamMyEvents(ctx context.Context, userID string, options StreamMyEventsOptions) (<-chan EventEnvelope, error) {
	c := s.core

	// memberRooms is the per-subscription visibility cache: the user receives
	// live events for rooms they are an explicit member of. Seeded from room
	// membership projections and mutated by relevant room facts.
	memberRooms := make(map[string]struct{})
	if err := s.populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
		return nil, err
	}

	dispatchSub, err := s.subscribeToLiveDispatch(ctx)
	if err != nil {
		return nil, fmt.Errorf("subscribe to live event dispatcher: %w", err)
	}

	eventChan := make(chan EventEnvelope)

	s.activeStreams.Add(1)
	go func() {
		c.logger.Debug("Server event stream started", "user_id", userID, "member_rooms", len(memberRooms))

		var presenceTicker *time.Ticker
		var presenceTickerC <-chan time.Time
		if options.TouchPresence {
			// Legacy behavior: subscribing implies the user is online; refresh on
			// a ticker so the KV TTL doesn't expire while the connection is open.
			if err := c.SetPresence(ctx, userID, PresenceStatusOnline); err != nil {
				c.logger.Warn("Failed to set initial presence", "error", err, "user_id", userID)
			}
			presenceTicker = time.NewTicker(PresenceRefreshInterval)
			presenceTickerC = presenceTicker.C
		}
		if presenceTicker != nil {
			defer presenceTicker.Stop()
		}

		heartbeatTicker := time.NewTicker(MyEventsHeartbeatInterval)
		defer heartbeatTicker.Stop()

		defer func() {
			s.activeStreams.Add(-1)
			c.logger.Debug("Server event stream closed", "user_id", userID)
			s.unsubscribeFromLiveDispatch(dispatchSub)
			close(eventChan)
		}()

		send := func(event EventEnvelope) bool {
			select {
			case <-ctx.Done():
				return false
			case <-dispatchSub.Done:
				return false
			case eventChan <- event:
				s.deliveredEvents.Add(1)
				return true
			}
		}

		for {
			select {
			case <-ctx.Done():
				return

			case <-dispatchSub.Done:
				c.logger.Debug("Server event stream reset by live dispatcher", "reason", dispatchSub.reason)
				return

			case <-presenceTickerC:
				if err := c.refreshPresence(ctx, userID); err != nil {
					s.presenceFailures.Add(1)
					c.logger.Warn("Failed to refresh presence", "error", err, "user_id", userID)
				} else {
					s.presenceRefreshes.Add(1)
				}

			case <-heartbeatTicker.C:
				if !send(NewHeartbeatEventEnvelope(NewEventID(), timestamppb.Now())) {
					return
				}

			case prepared := <-dispatchSub.C:
				event, ok := s.filterPreparedLiveEvent(ctx, userID, memberRooms, prepared)
				if !ok {
					continue
				}
				if !send(event) {
					return
				}
				// Session termination tears down the subscription. The frontend
				// handles logout on receipt; closing the channel ensures the server
				// tears down too.
				if EventSessionTerminated(event) != nil {
					c.logger.Info("Session terminated - closing event stream", "user_id", userID)
					return
				}
			}
		}
	}()

	return eventChan, nil
}

func (s *MyEventsModel) filterPreparedLiveEvent(
	ctx context.Context,
	userID string,
	memberRooms map[string]struct{},
	prepared *preparedLiveEvent,
) (EventEnvelope, bool) {
	if prepared == nil || prepared.envelope == nil {
		return nil, false
	}

	switch prepared.kind {
	case preparedPresence, preparedLiveEVTUser:
		return prepared.envelope, true
	case preparedLiveSync:
		return s.filterLiveSyncEvent(ctx, userID, memberRooms, &nats.Msg{Subject: prepared.subject}, prepared.envelope.LiveEvent())
	case preparedLiveEVTRoom:
		return s.filterReadyEVTRoomSubjectEvent(
			userID,
			memberRooms,
			prepared.roomID,
			prepared.envelope.EVTEvent(),
			prepared.envelope.DeliverySeq(),
		)
	case preparedLiveEVTAsset:
		return s.filterReadyEVTAssetSubjectEvent(
			userID,
			memberRooms,
			prepared.roomID,
			prepared.envelope.EVTEvent(),
			prepared.envelope.DeliverySeq(),
		)
	default:
		return nil, false
	}
}

// populateMemberRoomsCache (re)builds the per-subscription room visibility set
// in place. The cache contains every channel room the user is an explicit
// member of, plus every DM room they participate in.
func (s *MyEventsModel) populateMemberRoomsCache(ctx context.Context, userID string, memberRooms map[string]struct{}) error {
	for k := range memberRooms {
		delete(memberRooms, k)
	}

	// Explicit channel memberships. Membership alone qualifies: a user who has
	// joined the room receives its live events regardless of whether they could
	// re-join today.
	channelRooms, err := s.core.ListMemberRooms(ctx, KindChannel, userID, MemberRoomListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list channel member rooms: %w", err)
	}
	for _, room := range channelRooms {
		memberRooms[room.Id] = struct{}{}
	}

	dmRooms, err := s.core.ListMemberRooms(ctx, KindDM, userID, MemberRoomListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list DM member rooms: %w", err)
	}
	for _, room := range dmRooms {
		memberRooms[room.Id] = struct{}{}
	}

	return nil
}

// filterLiveEvent is the single-message compatibility path used by focused
// tests. Production delivery prepares each message once in Run before fanout.
func (s *MyEventsModel) filterLiveEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg) (EventEnvelope, bool, bool) {
	prepared, ok, err := s.prepareLiveMessage(ctx, msg)
	if err != nil {
		return nil, false, true
	}
	if !ok {
		return nil, false, false
	}
	event, ok := s.filterPreparedLiveEvent(ctx, userID, memberRooms, prepared)
	return event, ok, false
}

func (c *ChattoCore) filterLiveSyncEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.LiveEvent) (EventEnvelope, bool) {
	return c.myEvents().filterLiveSyncEvent(ctx, userID, memberRooms, msg, event)
}

func (s *MyEventsModel) filterLiveSyncEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.LiveEvent) (EventEnvelope, bool) {
	if event == nil || event.Event == nil {
		s.core.logger.Warn("Dropping live sync event without payload", "subject", msg.Subject)
		return nil, false
	}

	if kind := subjects.ParseKindFromRoomSubject(msg.Subject); kind != "" {
		roomID := subjects.ParseRoomIDFromSubject(msg.Subject)
		if roomID == "" {
			return nil, false
		}

		_, isMember := memberRooms[roomID]

		// Skip own typing events; the sender doesn't need to see them.
		if event.GetUserTyping() != nil && event.ActorId == userID {
			return nil, false
		}

		if !isMember {
			return nil, false
		}
		return NewLiveEventEnvelope(event), true
	}

	if !s.isAuthorizedForLiveEvent(ctx, userID, msg.Subject) {
		return nil, false
	}

	return NewLiveEventEnvelope(event), true
}

func (s *MyEventsModel) filterLiveEVTEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.Event) (EventEnvelope, bool, bool) {
	prepared, ok, err := s.prepareLiveEVTEvent(ctx, msg, event)
	if err != nil {
		return nil, false, true
	}
	if !ok {
		return nil, false, false
	}
	filtered, ok := s.filterPreparedLiveEvent(ctx, userID, memberRooms, prepared)
	return filtered, ok, false
}

func liveEVTMsgSeq(msg *nats.Msg) uint64 {
	if msg == nil {
		return 0
	}
	seq, err := strconv.ParseUint(msg.Header.Get(nats.JSSequence), 10, 64)
	if err != nil {
		return 0
	}
	return seq
}

func (s *MyEventsModel) filterReadyEVTRoomSubjectEvent(userID string, memberRooms map[string]struct{}, roomID string, event *corev1.Event, seq uint64) (EventEnvelope, bool) {
	if roomID == "" || event == nil || !isDeliverableLiveEVTRoomEvent(event) || seq == 0 {
		return nil, false
	}

	_, isMember := memberRooms[roomID]
	switch e := event.Event.(type) {
	case *corev1.Event_RoomCreated:
		if e.RoomCreated.GetUniversal() {
			if isEffective, err := s.core.RoomMembershipExists(context.Background(), KindChannel, userID, roomID); err == nil && isEffective {
				memberRooms[roomID] = struct{}{}
				isMember = true
			}
		}
	case *corev1.Event_RoomUniversalChanged:
		isEffective, err := s.core.RoomMembershipExists(context.Background(), KindChannel, userID, roomID)
		if err == nil && isEffective {
			memberRooms[roomID] = struct{}{}
			isMember = true
		} else if err == nil {
			wasMember := isMember
			delete(memberRooms, roomID)
			isMember = wasMember
		}
	case *corev1.Event_UserJoinedRoom:
		joinedUserID := event.ActorId
		if joinedUserID == userID {
			memberRooms[roomID] = struct{}{}
			isMember = true
		}
	case *corev1.Event_UserLeftRoom:
		leftUserID := event.ActorId
		if leftUserID == userID {
			delete(memberRooms, roomID)
		}
	case *corev1.Event_RoomMemberBanned:
		if e.RoomMemberBanned.GetUserId() == userID {
			delete(memberRooms, roomID)
		}
	case *corev1.Event_RoomDeleted:
		delete(memberRooms, roomID)
	}
	if !isMember {
		return nil, false
	}
	return NewEVTEventEnvelopeWithDeliverySeq(event, seq), true
}

func (s *MyEventsModel) filterReadyEVTAssetSubjectEvent(userID string, memberRooms map[string]struct{}, roomID string, event *corev1.Event, seq uint64) (EventEnvelope, bool) {
	if roomID == "" || event == nil || !isDeliverableLiveEVTAssetEvent(event) || seq == 0 {
		return nil, false
	}
	if _, isMember := memberRooms[roomID]; !isMember {
		return nil, false
	}
	return NewEVTEventEnvelopeWithDeliverySeq(event, seq), true
}

func (s *MyEventsModel) waitForLiveEVTRoomEvent(ctx context.Context, subject string, event *corev1.Event, seq uint64) error {
	pos := events.SubjectPosition(subject, seq)
	if err := s.core.rooms().waitForLiveEVTEvent(ctx, pos, event); err != nil {
		return err
	}

	if eventNeedsCallStateProjection(event) {
		if err := s.core.CallStateProjector.WaitFor(ctx, pos); err != nil {
			return err
		}
	}

	if isAssetLifecycleEvent(event) {
		if err := s.core.assetLifecycle().waitForAssets(ctx, pos); err != nil {
			return err
		}
	}
	return nil
}

func (s *MyEventsModel) waitForLiveEVTAssetEvent(ctx context.Context, subject string, seq uint64) error {
	return s.core.assetLifecycle().waitForAssets(ctx, events.SubjectPosition(subject, seq))
}

func (s *MyEventsModel) waitForLiveEVTUserEvent(ctx context.Context, subject string, seq uint64) error {
	return s.core.userModel.waitForUsers(ctx, events.SubjectPosition(subject, seq))
}

// isAuthorizedForLiveEvent checks whether a user can receive a non-room
// transient live event based on its live.sync subject.
func (c *ChattoCore) isAuthorizedForLiveEvent(ctx context.Context, userID, subject string) bool {
	return c.myEvents().isAuthorizedForLiveEvent(ctx, userID, subject)
}

func (s *MyEventsModel) isAuthorizedForLiveEvent(_ context.Context, userID, subject string) bool {
	parts := strings.Split(subject, ".")
	if len(parts) < 3 || parts[0] != "live" || parts[1] != "sync" {
		s.core.logger.Warn("Invalid live event subject format", "subject", subject)
		return false
	}

	switch parts[2] {
	case "config", "member":
		return true
	case "user":
		if len(parts) < 5 {
			s.core.logger.Warn("Invalid user-scoped live event subject", "subject", subject)
			return false
		}
		if parts[4] == "profile_updated" {
			return true
		}
		return parts[3] == userID
	case "room":
		s.core.logger.Warn("Room subject reached isAuthorizedForLiveEvent - should be filtered upstream", "subject", subject)
		return false
	default:
		s.core.logger.Warn("Unknown live event scope", "scope", parts[2], "subject", subject)
		return false
	}
}
