// Package notificationstream adapts Chatto's bounded notification lifecycle
// envelope to the reusable Loom event-log and projection mechanics.
package notificationstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	StreamName           = "NOTIFICATIONS"
	SignalledSubject     = "notifications.signalled"
	ReadSubject          = "notifications.read"
	RemovedSubject       = "notifications.removed"
	AlertResolvedSubject = "notifications.alert_resolved"

	IdentityMetadataKey = "chatto.notifications.incarnation"
	identityPrefix      = "notifications-incarnation-v1:"
)

// Subjects returns the complete, fixed subject set owned by NOTIFICATIONS.
// Returning a fresh slice keeps callers from mutating the stream contract.
func Subjects() []string {
	return []string{
		SignalledSubject,
		ReadSubject,
		RemovedSubject,
		AlertResolvedSubject,
	}
}

var (
	ErrInvalidEvent = errors.New("invalid notification event")
	ErrExpiredEvent = errors.New("notification event is already expired")
)

// Publisher validates and protobuf-encodes notification events while the
// shared event log owns OCC, de-duplication, and JetStream publication.
type Publisher struct {
	log            *events.EncodedEventLog
	retentionGrace time.Duration
	now            func() time.Time
}

func NewPublisher(js jetstream.JetStream, stream jetstream.Stream, retentionGrace time.Duration, logger events.Logger) *Publisher {
	return &Publisher{
		log:            events.NewEncodedEventLog(js, stream, logger),
		retentionGrace: retentionGrace,
		now:            time.Now,
	}
}

// AppendEventually publishes an immutable lifecycle event. Retrying after a
// bounded-subject OCC conflict is safe because event IDs and state transitions
// are idempotent.
func (p *Publisher) AppendEventually(ctx context.Context, event *corev1.NotificationEvent) (events.StreamPosition, error) {
	results, err := p.AppendBatchEventuallyResults(ctx, []*corev1.NotificationEvent{event})
	if err != nil {
		return events.StreamPosition{}, err
	}
	return results[0].Position, nil
}

// AppendResult distinguishes a newly committed lifecycle fact from a
// JetStream de-duplication acknowledgement for the same logical event ID.
type AppendResult struct {
	Position  events.StreamPosition
	Committed bool
}

type preparedEvent struct {
	id             string
	data           []byte
	physicalExpiry time.Time
}

// AppendBatchEventuallyResults publishes lifecycle facts for one fixed
// subject and reports which facts were newly committed. A batch whose IDs are
// all new is atomic. If JetStream identifies a duplicate ID, the publisher
// retries each fact idempotently; a non-nil error can then accompany partial
// results that callers must wait for and account for before returning.
func (p *Publisher) AppendBatchEventuallyResults(ctx context.Context, notificationEvents []*corev1.NotificationEvent) ([]AppendResult, error) {
	if len(notificationEvents) == 0 {
		return nil, nil
	}
	subject := ""
	prepared := make([]preparedEvent, 0, len(notificationEvents))
	for i, event := range notificationEvents {
		eventSubject, err := subjectFor(event)
		if err != nil {
			return nil, fmt.Errorf("event %d: %w", i, err)
		}
		if subject == "" {
			subject = eventSubject
		} else if eventSubject != subject {
			return nil, fmt.Errorf("%w: batch events must share one lifecycle subject", ErrInvalidEvent)
		}
		if event.GetExpiresAt() == nil || !event.GetExpiresAt().IsValid() {
			return nil, fmt.Errorf("%w: expires_at is required", ErrInvalidEvent)
		}
		data, err := proto.Marshal(event)
		if err != nil {
			return nil, fmt.Errorf("marshal notification event %d: %w", i, err)
		}
		prepared = append(prepared, preparedEvent{
			id:             event.GetId(),
			data:           data,
			physicalExpiry: event.GetExpiresAt().AsTime().UTC().Add(p.retentionGrace),
		})
	}
	for _, event := range prepared {
		if !p.now().UTC().Before(event.physicalExpiry) {
			return nil, ErrExpiredEvent
		}
	}
	for {
		result, err := p.log.ExecuteMutation(ctx, events.AtSubject(subject), func(context.Context, events.MutationAttempt) ([]events.EncodedMutationEntry, error) {
			now := p.now().UTC()
			entries := make([]events.EncodedMutationEntry, 0, len(prepared))
			for _, event := range prepared {
				ttl := event.physicalExpiry.Sub(now)
				if ttl <= 0 {
					return nil, ErrExpiredEvent
				}
				entries = append(entries, events.EncodedMutationEntry{
					Subject: subject,
					Record:  events.EncodedRecord{ID: event.id, Data: event.data, TTL: ttl},
				})
			}
			return entries, nil
		})
		if err == nil {
			results := make([]AppendResult, len(result.Sequences))
			for i, sequence := range result.Sequences {
				results[i] = AppendResult{
					Position:  events.SubjectPosition(subject, sequence),
					Committed: result.Committed,
				}
			}
			return results, nil
		}
		if errors.Is(err, events.ErrDuplicateBatchMessageID) && len(notificationEvents) > 1 {
			return appendNotificationEventsIndividually(notificationEvents, func(event *corev1.NotificationEvent) (AppendResult, error) {
				single, appendErr := p.AppendBatchEventuallyResults(ctx, []*corev1.NotificationEvent{event})
				if appendErr != nil {
					return AppendResult{}, appendErr
				}
				return single[0], nil
			})
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

func appendNotificationEventsIndividually(
	notificationEvents []*corev1.NotificationEvent,
	appendOne func(*corev1.NotificationEvent) (AppendResult, error),
) ([]AppendResult, error) {
	results := make([]AppendResult, 0, len(notificationEvents))
	for _, event := range notificationEvents {
		result, err := appendOne(event)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func subjectFor(event *corev1.NotificationEvent) (string, error) {
	if event == nil || event.GetId() == "" || event.GetRecipientId() == "" || event.GetNotificationId() == "" || event.GetOccurredAt() == nil {
		return "", fmt.Errorf("%w: id, recipient_id, notification_id, and occurred_at are required", ErrInvalidEvent)
	}
	switch event.GetEvent().(type) {
	case *corev1.NotificationEvent_Signalled:
		return SignalledSubject, nil
	case *corev1.NotificationEvent_Read:
		return ReadSubject, nil
	case *corev1.NotificationEvent_Removed:
		return RemovedSubject, nil
	case *corev1.NotificationEvent_AlertResolved:
		return AlertResolvedSubject, nil
	default:
		return "", fmt.Errorf("%w: event payload is unset or unsupported", ErrInvalidEvent)
	}
}

type ProjectionPointer[T any] interface {
	events.EventProjection[*corev1.NotificationEvent]
	*T
}

func NewProjectionHandle[T any, P ProjectionPointer[T]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	logger events.Logger,
) events.ProjectionHandle[P] {
	return events.NewDecodedProjectionHandle(js, stream, projection, decodeEvent, logger)
}

func decodeEvent(data []byte) (events.DecodedEvent[*corev1.NotificationEvent], error) {
	var event corev1.NotificationEvent
	if err := proto.Unmarshal(data, &event); err != nil {
		return events.DecodedEvent[*corev1.NotificationEvent]{}, err
	}
	return events.DecodedEvent[*corev1.NotificationEvent]{Event: &event, ID: event.GetId()}, nil
}

func NewIdentity(created time.Time) (string, error) {
	if created.IsZero() {
		return "", fmt.Errorf("NOTIFICATIONS stream creation time is required")
	}
	sum := sha256.Sum256([]byte("chatto/notifications-incarnation/v1\x00" + created.UTC().Format(time.RFC3339Nano)))
	return identityPrefix + hex.EncodeToString(sum[:16]), nil
}

func ValidIdentity(identity string) bool {
	if len(identity) != len(identityPrefix)+32 || !strings.HasPrefix(identity, identityPrefix) {
		return false
	}
	_, err := hex.DecodeString(identity[len(identityPrefix):])
	return err == nil
}

func IdentityFromInfo(info *jetstream.StreamInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("NOTIFICATIONS stream info is unavailable")
	}
	identity := info.Config.Metadata[IdentityMetadataKey]
	if !ValidIdentity(identity) {
		return "", fmt.Errorf("NOTIFICATIONS stream identity is missing or invalid")
	}
	return identity, nil
}
