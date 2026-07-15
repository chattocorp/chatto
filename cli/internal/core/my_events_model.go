package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
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
)

// MyEventsModel owns the server-side myEvents live stream machinery.
//
// ChattoCore remains the public facade, while this model keeps live root
// filtering, projection readiness, shared per-user room visibility, and
// per-session delivery together.
type MyEventsModel struct {
	core              *ChattoCore
	hub               *MyEventsHub
	activeStreams     atomic.Int64
	deliveredEvents   atomic.Uint64
	slowDisconnects   atomic.Uint64
	presenceRefreshes atomic.Uint64
	presenceFailures  atomic.Uint64
}

func NewMyEventsModel(core *ChattoCore) *MyEventsModel {
	model := &MyEventsModel{core: core}
	model.hub = NewMyEventsHub(model)
	return model
}

// Run starts the process-wide live-event ingress and blocks until ctx ends.
func (s *MyEventsModel) Run(ctx context.Context) error {
	return s.hub.Run(ctx)
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

// StreamMyEvents creates a unified stream of every event on this deployment
// that is relevant to a specific user.
//
// The process-wide MyEventsHub receives two internal NATS Core subject roots:
// live.sync.> carries transient LiveEvent messages and live.evt.> is the raw
// singleton republish of committed EVT facts. EVT delivery is not UI-safe by
// itself: the hub waits for the relevant local projection(s) to reach the
// republished stream sequence, then applies each user's authorization before
// forwarding the event through the realtime API.
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

	hubSub, err := s.hub.Subscribe(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to myEvents hub: %w", err)
	}

	presenceSub, err := c.presenceModel.Subscribe(ctx)
	if err != nil {
		s.hub.Unsubscribe(hubSub)
		return nil, fmt.Errorf("failed to subscribe to presence hub: %w", err)
	}

	eventChan := make(chan EventEnvelope)

	s.activeStreams.Add(1)
	go func() {
		c.logger.Debug("Server event stream started", "user_id", userID)

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
			s.hub.Unsubscribe(hubSub)
			c.presenceModel.Unsubscribe(presenceSub)
			close(eventChan)
		}()

		send := func(event EventEnvelope) bool {
			select {
			case <-ctx.Done():
				return false
			case <-hubSub.Done:
				return false
			case <-presenceSub.Done:
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
			case <-hubSub.Done:
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

			case delivery, ok := <-hubSub.C:
				if !ok {
					return
				}
				select {
				case <-hubSub.Done:
					return
				default:
				}
				s.hub.consume(hubSub, delivery)
				event := delivery.event
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

			case update, ok := <-presenceSub.C:
				if !ok {
					if presenceSub.Lagged() {
						s.slowDisconnects.Add(1)
					}
					return
				}
				if presenceSub.Lagged() {
					s.slowDisconnects.Add(1)
					return
				}
				live := newLiveEvent(update.UserID, &corev1.LiveEvent{
					Event: &corev1.LiveEvent_PresenceChanged{
						PresenceChanged: &corev1.PresenceChangedEvent{Status: update.Status},
					},
				})
				if !send(NewLiveEventEnvelope(live)) {
					return
				}
			case <-presenceSub.Done:
				if presenceSub.Lagged() {
					s.slowDisconnects.Add(1)
				}
				return
			}
		}
	}()

	return eventChan, nil
}

// populateMemberRoomsCache (re)builds one user's room visibility set in place.
// The cache contains every channel room the user is an explicit or effective
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
