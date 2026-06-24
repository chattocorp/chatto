package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// liveEVTProjectionWaitTimeout bounds the causal barrier between JetStream's
// raw EVT republish and GraphQL delivery. In the normal case the local
// projectors have already advanced and WaitFor returns immediately; the
// timeout covers replica lag or a stuck projector without wedging a
// subscription goroutine forever.
const liveEVTProjectionWaitTimeout = 2 * time.Second

// MyEventsService owns the server-side myEvents live stream machinery.
//
// ChattoCore remains the public facade, while this service keeps live root
// filtering, projection readiness, and per-subscription room membership state
// together.
type MyEventsService struct {
	core              *ChattoCore
	activeStreams     atomic.Int64
	deliveredEvents   atomic.Uint64
	slowDisconnects   atomic.Uint64
	presenceRefreshes atomic.Uint64
	presenceFailures  atomic.Uint64
}

func NewMyEventsService(core *ChattoCore) *MyEventsService {
	return &MyEventsService{core: core}
}

// StreamMyEventsOptions controls compatibility behavior for a myEvents stream.
type StreamMyEventsOptions struct {
	// ReportPresence preserves the legacy behavior where opening myEvents marks
	// the user online and refreshes the current presence value. New clients that
	// report presence through ConnectRPC set this to false.
	ReportPresence bool
}

func (c *ChattoCore) myEvents() *MyEventsService {
	if c.myEventsService == nil {
		c.myEventsService = NewMyEventsService(c)
	}
	return c.myEventsService
}

// MyEventsMetrics is a process-local snapshot of the GraphQL live-event stream.
type MyEventsMetrics struct {
	ActiveStreams     int64
	DeliveredEvents   uint64
	SlowDisconnects   uint64
	PresenceRefreshes uint64
	PresenceFailures  uint64
}

// MyEventsMetrics returns process-local live-event stream counters.
func (c *ChattoCore) MyEventsMetrics() MyEventsMetrics {
	if c.myEventsService == nil {
		return MyEventsMetrics{}
	}
	return c.myEventsService.Metrics()
}

// Metrics returns process-local live-event stream counters.
func (s *MyEventsService) Metrics() MyEventsMetrics {
	return MyEventsMetrics{
		ActiveStreams:     s.activeStreams.Load(),
		DeliveredEvents:   s.deliveredEvents.Load(),
		SlowDisconnects:   s.slowDisconnects.Load(),
		PresenceRefreshes: s.presenceRefreshes.Load(),
		PresenceFailures:  s.presenceFailures.Load(),
	}
}

// StreamMyEvents creates a unified stream of every event on this deployment
// that is relevant to a specific user.
//
// Events arrive via NATS Core subscriptions on two internal subject roots:
// live.sync.> carries transient LiveEvent messages and live.evt.> is the raw
// singleton republish of committed EVT facts. EVT delivery is not UI-safe by
// itself: filterLiveEvent waits for the relevant local projection(s) to reach
// the republished stream sequence, then applies this user's authorization
// before forwarding the event through GraphQL.
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
// synthetic Heartbeat is emitted every 25s so clients can detect a dead
// subscription on an otherwise-healthy WebSocket.
//
// The returned channel closes when the context is cancelled or when a
// SessionTerminatedEvent is delivered to the user.
func (c *ChattoCore) StreamMyEvents(ctx context.Context, userID string) (<-chan EventEnvelope, error) {
	return c.StreamMyEventsWithOptions(ctx, userID, StreamMyEventsOptions{ReportPresence: true})
}

// StreamMyEventsWithOptions creates a myEvents stream with explicit compatibility options.
func (c *ChattoCore) StreamMyEventsWithOptions(ctx context.Context, userID string, options StreamMyEventsOptions) (<-chan EventEnvelope, error) {
	return c.myEvents().StreamMyEvents(ctx, userID, options)
}

func (s *MyEventsService) StreamMyEvents(ctx context.Context, userID string, options StreamMyEventsOptions) (<-chan EventEnvelope, error) {
	c := s.core

	// memberRooms is the per-subscription visibility cache: the user receives
	// live events for rooms they are an explicit member of. Seeded from room
	// membership projections and mutated by relevant room facts.
	memberRooms := make(map[string]struct{})
	if err := s.populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
		return nil, err
	}

	// live.sync.> is the transient LiveEvent subject root. live.evt.> is the
	// raw committed-event feed from the EVT stream. The 256-message buffer
	// absorbs bursts; slow-consumer notifications tear the resolver down so the
	// client can reconnect and refresh projected state.
	msgChan := make(chan *nats.Msg, 256)
	liveSyncSub, err := c.nc.ChanSubscribe(subjects.LiveSyncAllEvents(), msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live sync events: %w", err)
	}
	slowSyncConsumerCh := liveSyncSub.StatusChanged(nats.SubscriptionSlowConsumer)

	liveEVTSub, err := c.nc.ChanSubscribe(events.LiveSubjectRoot+">", msgChan)
	if err != nil {
		liveSyncSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to live EVT events: %w", err)
	}
	slowEVTConsumerCh := liveEVTSub.StatusChanged(nats.SubscriptionSlowConsumer)

	presenceSub, err := c.presenceService.Subscribe(ctx)
	if err != nil {
		liveSyncSub.Unsubscribe()
		liveEVTSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to presence hub: %w", err)
	}

	eventChan := make(chan EventEnvelope)

	s.activeStreams.Add(1)
	go func() {
		c.logger.Debug("Server event stream started", "user_id", userID, "member_rooms", len(memberRooms))

		var presenceTicker *time.Ticker
		var presenceTickerC <-chan time.Time
		if options.ReportPresence {
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

		heartbeatTicker := time.NewTicker(25 * time.Second)
		defer heartbeatTicker.Stop()

		lastKnownPresence := make(map[string]string, len(presenceSub.Snapshot))
		for k, v := range presenceSub.Snapshot {
			lastKnownPresence[k] = v
		}

		defer func() {
			s.activeStreams.Add(-1)
			c.logger.Debug("Server event stream closed", "user_id", userID)
			liveSyncSub.Unsubscribe()
			liveEVTSub.Unsubscribe()
			c.presenceService.Unsubscribe(presenceSub)
			close(eventChan)
		}()

		send := func(event EventEnvelope) bool {
			select {
			case <-ctx.Done():
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

			case <-slowEVTConsumerCh:
				dropped, _ := liveEVTSub.Dropped()
				s.slowDisconnects.Add(1)
				c.logger.Warn("Slow consumer on live EVT subscription - tearing down",
					"user_id", userID, "dropped", dropped)
				return

			case <-slowSyncConsumerCh:
				dropped, _ := liveSyncSub.Dropped()
				s.slowDisconnects.Add(1)
				c.logger.Warn("Slow consumer on live sync subscription - tearing down",
					"user_id", userID, "dropped", dropped)
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

			case msg := <-msgChan:
				event, ok, closeStream := s.filterLiveEvent(ctx, userID, memberRooms, msg)
				if closeStream {
					return
				}
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

			case update := <-presenceSub.C:
				if last, exists := lastKnownPresence[update.UserID]; exists && last == update.Status {
					continue
				}
				if update.Status == PresenceStatusOffline {
					delete(lastKnownPresence, update.UserID)
				} else {
					lastKnownPresence[update.UserID] = update.Status
				}
				live := newLiveEvent(update.UserID, &corev1.LiveEvent{
					Event: &corev1.LiveEvent_PresenceChanged{
						PresenceChanged: &corev1.PresenceChangedEvent{Status: update.Status},
					},
				})
				if !send(NewLiveEventEnvelope(live)) {
					return
				}
			}
		}
	}()

	return eventChan, nil
}

// populateMemberRoomsCache (re)builds the per-subscription room visibility set
// in place. The cache contains every channel room the user is an explicit
// member of, plus every DM room they participate in.
func (s *MyEventsService) populateMemberRoomsCache(ctx context.Context, userID string, memberRooms map[string]struct{}) error {
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

// filterLiveEvent unmarshals a message from one of the live delivery roots and
// applies per-user authorization. The third return value tells the caller to
// close the stream because a deliverable event could not be made projection-safe;
// the client will resubscribe and refresh projected state. Mutates memberRooms
// when the subscriber themselves joins/leaves a room or when a room is deleted.
func (s *MyEventsService) filterLiveEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg) (EventEnvelope, bool, bool) {
	if strings.HasPrefix(msg.Subject, "live.sync.") {
		var live corev1.LiveEvent
		if err := proto.Unmarshal(msg.Data, &live); err != nil {
			s.core.logger.Warn("Failed to unmarshal live sync event", "subject", msg.Subject, "error", err)
			return nil, false, false
		}
		event, ok := s.filterLiveSyncEvent(ctx, userID, memberRooms, msg, &live)
		return event, ok, false
	}

	if !strings.HasPrefix(msg.Subject, events.LiveSubjectRoot) {
		s.core.logger.Warn("Unknown live event subject root", "subject", msg.Subject)
		return nil, false, false
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		s.core.logger.Warn("Failed to unmarshal live event", "subject", msg.Subject, "error", err)
		return nil, false, false
	}

	return s.filterLiveEVTEvent(ctx, userID, memberRooms, msg, &event)
}

func (c *ChattoCore) filterLiveSyncEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.LiveEvent) (EventEnvelope, bool) {
	return c.myEvents().filterLiveSyncEvent(ctx, userID, memberRooms, msg, event)
}

func (s *MyEventsService) filterLiveSyncEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.LiveEvent) (EventEnvelope, bool) {
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

func (s *MyEventsService) filterLiveEVTEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.Event) (EventEnvelope, bool, bool) {
	seq := liveEVTMsgSeq(msg)
	if seq == 0 {
		s.core.logger.Warn("live EVT message missing stream sequence", "subject", msg.Subject, "sequence", msg.Header.Get(nats.JSSequence))
		return nil, false, false
	}

	if roomID, ok := events.ParseRoomSubject(msg.Subject); ok {
		if !isDeliverableLiveEVTRoomEvent(event) {
			return nil, false, false
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		evtSubject := events.SubjectRoot + strings.TrimPrefix(msg.Subject, events.LiveSubjectRoot)
		if err := s.waitForLiveEVTRoomEvent(waitCtx, evtSubject, event, seq); err != nil {
			s.core.logger.Warn("Live EVT projection readiness failed - tearing down stream", "subject", msg.Subject, "sequence", seq, "error", err)
			return nil, false, true
		}

		filtered, ok := s.filterReadyEVTRoomSubjectEvent(userID, memberRooms, roomID, event, seq)
		return filtered, ok, false
	}

	if _, ok := events.ParseAssetSubject(msg.Subject); ok {
		if !isDeliverableLiveEVTAssetEvent(event) {
			return nil, false, false
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		evtSubject := events.SubjectRoot + strings.TrimPrefix(msg.Subject, events.LiveSubjectRoot)
		if err := s.waitForLiveEVTAssetEvent(waitCtx, evtSubject, seq); err != nil {
			s.core.logger.Warn("Live EVT asset projection readiness failed - tearing down stream", "subject", msg.Subject, "sequence", seq, "error", err)
			return nil, false, true
		}
		assetID := assetIDOfLifecycleEvent(event)
		roomID, ok := s.core.assetLifecycle().AssetRoomID(assetID)
		if !ok {
			return nil, false, false
		}
		filtered, ok := s.filterReadyEVTAssetSubjectEvent(userID, memberRooms, roomID, event, seq)
		return filtered, ok, false
	}

	if _, ok := events.ParseUserSubject(msg.Subject); ok {
		if !isDeliverableLiveEVTUserEvent(event) {
			return nil, false, false
		}
		waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
		defer cancel()
		evtSubject := events.SubjectRoot + strings.TrimPrefix(msg.Subject, events.LiveSubjectRoot)
		if err := s.waitForLiveEVTUserEvent(waitCtx, evtSubject, seq); err != nil {
			s.core.logger.Warn("Live EVT user projection readiness failed - tearing down stream", "subject", msg.Subject, "sequence", seq, "error", err)
			return nil, false, true
		}
		return NewEVTEventEnvelopeWithDeliverySeq(event, seq), true, false
	}

	return nil, false, false
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

func (s *MyEventsService) filterReadyEVTRoomSubjectEvent(userID string, memberRooms map[string]struct{}, roomID string, event *corev1.Event, seq uint64) (EventEnvelope, bool) {
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

func (s *MyEventsService) filterReadyEVTAssetSubjectEvent(userID string, memberRooms map[string]struct{}, roomID string, event *corev1.Event, seq uint64) (EventEnvelope, bool) {
	if roomID == "" || event == nil || !isDeliverableLiveEVTAssetEvent(event) || seq == 0 {
		return nil, false
	}
	if _, isMember := memberRooms[roomID]; !isMember {
		return nil, false
	}
	return NewEVTEventEnvelopeWithDeliverySeq(event, seq), true
}

func (s *MyEventsService) waitForLiveEVTRoomEvent(ctx context.Context, subject string, event *corev1.Event, seq uint64) error {
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

func (s *MyEventsService) waitForLiveEVTAssetEvent(ctx context.Context, subject string, seq uint64) error {
	return s.core.assetLifecycle().waitForAssets(ctx, events.SubjectPosition(subject, seq))
}

func (s *MyEventsService) waitForLiveEVTUserEvent(ctx context.Context, subject string, seq uint64) error {
	return s.core.userService.waitForUsers(ctx, events.SubjectPosition(subject, seq))
}

// isAuthorizedForLiveEvent checks whether a user can receive a non-room
// transient live event based on its live.sync subject.
func (c *ChattoCore) isAuthorizedForLiveEvent(ctx context.Context, userID, subject string) bool {
	return c.myEvents().isAuthorizedForLiveEvent(ctx, userID, subject)
}

func (s *MyEventsService) isAuthorizedForLiveEvent(_ context.Context, userID, subject string) bool {
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
