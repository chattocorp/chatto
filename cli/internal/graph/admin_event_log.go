package graph

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// eventLogPaginationLimits caps how many EVT entries one request
// can pull back. Walking the stream backwards is one NATS GetMsg per
// entry, so the cap keeps a worst-case admin browse cheap.
const (
	defaultEventLogPageSize = 50
	maxEventLogPageSize     = 200
)

// fetchEventLogPage walks EVT backwards from startSeq (inclusive)
// and returns up to `limit` entries newest-first. Stops early at the
// stream's first sequence. Skips holes (deleted messages) and only
// surfaces an error if a NATS call fails for a non-NotFound reason.
func (r *Resolver) fetchEventLogPage(
	ctx context.Context,
	stream jetstream.Stream,
	startSeq uint64,
	firstSeq uint64,
	limit int,
) ([]*model.EventLogEntry, error) {
	entries := make([]*model.EventLogEntry, 0, limit)
	if startSeq < firstSeq {
		return entries, nil
	}

	for seq := startSeq; seq >= firstSeq && len(entries) < limit; seq-- {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				// A hole in the sequence — shouldn't happen on
				// EVT in practice, but tolerated.
				if seq == 0 {
					break
				}
				continue
			}
			return nil, fmt.Errorf("get msg %d: %w", seq, err)
		}

		entry, err := streamMsgToEventLogEntry(msg)
		if err != nil {
			// Decode failures are surfaced as an entry with the
			// failure noted, so the admin can still see the row
			// instead of losing the whole page.
			entry = &model.EventLogEntry{
				Sequence:    strconv.FormatUint(seq, 10),
				Subject:     msg.Subject,
				EventType:   "decode-error",
				PayloadJSON: fmt.Sprintf("{\"decode_error\": %q}", err.Error()),
			}
		}
		entries = append(entries, entry)

		if seq == 0 {
			break
		}
	}
	return entries, nil
}

// streamMsgToEventLogEntry decodes one NATS stream message into the
// GraphQL surface. Subject parsing is generic: anything matching
// "evt.{type}.{id}" splits into aggregateType + aggregateId; subjects
// outside that shape come back with empty parts and the full subject
// preserved.
func streamMsgToEventLogEntry(msg *jetstream.RawStreamMsg) (*model.EventLogEntry, error) {
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}

	aggregateType, aggregateID := parseAggregateSubject(msg.Subject)
	eventType := eventVariantName(&event)

	payloadJSON, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}.Marshal(&event)
	if err != nil {
		return nil, fmt.Errorf("marshal payload json: %w", err)
	}

	entry := &model.EventLogEntry{
		Sequence:      strconv.FormatUint(msg.Sequence, 10),
		Subject:       msg.Subject,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		EventID:       event.GetId(),
		ActorID:       event.GetActorId(),
		CreatedAt:     event.GetCreatedAt(),
		PayloadJSON:   string(payloadJSON),
	}
	return entry, nil
}

// parseAggregateSubject splits an event subject into (aggregateType,
// aggregateId) for the canonical "evt.{type}.{id}.{eventType}" shape.
// The trailing event-type segment is intentionally dropped — it's
// rendered separately in the admin UI from the protobuf oneof name.
// Subjects outside the canonical shape (legacy, malformed) come back
// empty so the resolver still has something to display.
func parseAggregateSubject(subject string) (aggregateType, aggregateID string) {
	rest, ok := strings.CutPrefix(subject, events.SubjectRoot)
	if !ok {
		return "", ""
	}
	parts := strings.SplitN(rest, ".", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// eventVariantName returns the protobuf message name of the oneof
// variant set on the event (e.g. "UserJoinedRoomEvent",
// "ServerNameChangedEvent"). Empty string if no variant is set
// (shouldn't happen for events that came off the wire, but we don't
// trust the input).
func eventVariantName(event *corev1.Event) string {
	rm := event.ProtoReflect()
	oneof := rm.Descriptor().Oneofs().ByName("event")
	if oneof == nil {
		return ""
	}
	field := rm.WhichOneof(oneof)
	if field == nil {
		return ""
	}
	if field.Kind() == protoreflect.MessageKind {
		return string(field.Message().Name())
	}
	return string(field.Name())
}
