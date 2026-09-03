package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
)

// ============================================================================
// Event Publishing Helpers
// ============================================================================

// natsPublishFlushTimeout bounds how long a fire-and-forget publish will wait
// for the NATS server to acknowledge buffered bytes. Without a timeout, a
// hung server (e.g. network partition) would block the calling goroutine
// indefinitely instead of surfacing as a normal error.
const natsPublishFlushTimeout = 5 * time.Second

type pubsubPublicationScope uint8

const (
	pubsubPublicationScopeUser pubsubPublicationScope = iota + 1
	pubsubPublicationScopeRoom
)

type pubsubEventPublication struct {
	scope    pubsubPublicationScope
	userID   string
	roomKind RoomKind
	roomID   string
	event    *pubsubv1.PubSubEvent
}

func userPubSubEventPublication(userID string, event *pubsubv1.PubSubEvent) pubsubEventPublication {
	return pubsubEventPublication{scope: pubsubPublicationScopeUser, userID: userID, event: event}
}

func roomPubSubEventPublication(kind RoomKind, roomID string, event *pubsubv1.PubSubEvent) pubsubEventPublication {
	return pubsubEventPublication{scope: pubsubPublicationScopeRoom, roomKind: kind, roomID: roomID, event: event}
}

// publishUserPubSubEvent publishes one private latest-value signal to a
// single user's live subject.
func (c *ChattoCore) publishUserPubSubEvent(ctx context.Context, userID string, event *pubsubv1.PubSubEvent) error {
	return c.publishPubSubEvents(ctx, []pubsubEventPublication{userPubSubEventPublication(userID, event)})
}

// publishRoomPubSubEvent publishes one transient room signal to current room
// members. Only typing events are valid at this scope.
func (c *ChattoCore) publishRoomPubSubEvent(ctx context.Context, kind RoomKind, roomID string, event *pubsubv1.PubSubEvent) error {
	return c.publishPubSubEvents(ctx, []pubsubEventPublication{roomPubSubEventPublication(kind, roomID, event)})
}

// publishPubSubEvents publishes a related set of pubsub events and flushes
// once after the complete set has entered the client buffer. This keeps large
// fanouts linear without imposing one network round trip per recipient.
func (c *ChattoCore) publishPubSubEvents(_ context.Context, publications []pubsubEventPublication) error {
	type encodedPublication struct {
		subject string
		data    []byte
	}
	encoded := make([]encodedPublication, 0, len(publications))
	for index, publication := range publications {
		subject, err := publication.subject()
		if err != nil {
			return fmt.Errorf("pubsub publication %d: %w", index, err)
		}
		eventData, err := proto.Marshal(publication.event)
		if err != nil {
			return fmt.Errorf("marshal pubsub publication %d: %w", index, err)
		}
		encoded = append(encoded, encodedPublication{subject: subject, data: eventData})
	}
	if len(encoded) == 0 {
		return nil
	}
	for _, publication := range encoded {
		if err := c.nc.Publish(publication.subject, publication.data); err != nil {
			return fmt.Errorf("publish pubsub event to %s: %w", publication.subject, err)
		}
	}
	if err := c.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return fmt.Errorf("flush %d pubsub events: %w", len(encoded), err)
	}
	return nil
}

func (publication pubsubEventPublication) subject() (string, error) {
	if err := validatePubSubEvent(publication.event); err != nil {
		return "", err
	}
	validToken := func(value string) bool {
		return value != "" && !strings.ContainsAny(value, ".*>")
	}
	switch publication.scope {
	case pubsubPublicationScopeUser:
		if !validToken(publication.userID) {
			return "", fmt.Errorf("invalid user-scoped pubsub target")
		}
		var eventType string
		switch publication.event.GetEvent().(type) {
		case *pubsubv1.PubSubEvent_NotificationOccurrencesChanged:
			eventType = "notification_v2"
		case *pubsubv1.PubSubEvent_NotificationUnreadStateChanged:
			eventType = "notification_unread"
		case *pubsubv1.PubSubEvent_RoomReadStateChanged:
			eventType = "room_read"
		case *pubsubv1.PubSubEvent_ThreadViewerStateChanged:
			eventType = "thread_viewer_state"
		case *pubsubv1.PubSubEvent_SessionTerminated:
			eventType = "session_terminated"
		default:
			return "", fmt.Errorf("pubsub payload is not valid for user scope")
		}
		return subjects.LiveSyncUserEvent(publication.userID, eventType), nil
	case pubsubPublicationScopeRoom:
		if (publication.roomKind != KindChannel && publication.roomKind != KindDM) || !validToken(publication.roomID) {
			return "", fmt.Errorf("invalid room-scoped pubsub target")
		}
		typing := publication.event.GetUserTyping()
		if typing == nil || typing.GetRoomId() != publication.roomID {
			return "", fmt.Errorf("pubsub payload does not match room scope")
		}
		return subjects.LiveSyncRoomEvent(string(publication.roomKind), publication.roomID, "user_typing"), nil
	default:
		return "", fmt.Errorf("invalid pubsub publication scope")
	}
}

func validateEvent(event *evtv1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

func validatePubSubEvent(event *pubsubv1.PubSubEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: pubsub event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

// newEvent fills in the Id, ActorID, and CreatedAt fields of an Event
// envelope if they're not already set. The caller provides the event
// with the concrete oneof variant already populated.
func newEvent(actorID string, event *evtv1.Event) *evtv1.Event {
	if event.Id == "" {
		event.Id = NewEventID()
	}
	if event.ActorId == "" {
		event.ActorId = actorID
	}
	if event.CreatedAt == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}
	return event
}

// newPubSubEvent fills in the ID, actor ID, and creation time of a PubSubEvent
// envelope if they're not already set. The caller provides the event with the
// concrete oneof variant already populated.
func newPubSubEvent(actorID string, event *pubsubv1.PubSubEvent) *pubsubv1.PubSubEvent {
	if event.Id == "" {
		event.Id = NewEventID()
	}
	if event.ActorId == "" {
		event.ActorId = actorID
	}
	if event.CreatedAt == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}
	return event
}

// ============================================================================
// Event Streaming
// ============================================================================

// isTerminalIteratorError returns true if the error indicates the iterator
// cannot be recovered (connection closed, consumer deleted, etc.).
// Recoverable errors (heartbeat missed, leadership changed) return false.
func isTerminalIteratorError(err error) bool {
	if err == nil {
		return false
	}
	// Terminal errors - cannot recover, must stop
	if errors.Is(err, jetstream.ErrMsgIteratorClosed) ||
		errors.Is(err, jetstream.ErrConnectionClosed) ||
		errors.Is(err, jetstream.ErrServerShutdown) ||
		errors.Is(err, jetstream.ErrConsumerDeleted) {
		return true
	}
	return false
}
