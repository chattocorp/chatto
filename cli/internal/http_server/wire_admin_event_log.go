package http_server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

const (
	defaultWireEventLogPageSize = 50
	maxWireEventLogPageSize     = 200
)

func (c *wireConn) handleWireListAdminEventLog(ctx context.Context, userID, requestID string, body *apiv1.ListAdminEventLogRequest) (*apiv1.ListAdminEventLogResponse, *wirev1.WireError) {
	if err := c.requireWireAdminAuditView(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	stream, err := c.server.core.EventStreamForDebug(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("get EVT stream: %w", err))
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("stream info: %w", err))
	}
	totalCount, err := wireEventLogTotalCount(info.State.Msgs)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	pageSize := defaultWireEventLogPageSize
	if body.GetLimit() != 0 {
		pageSize = int(body.GetLimit())
		if pageSize < 1 {
			pageSize = 1
		}
		if pageSize > maxWireEventLogPageSize {
			pageSize = maxWireEventLogPageSize
		}
	}

	startSeq := info.State.LastSeq
	if before := strings.TrimSpace(body.GetBefore()); before != "" {
		parsed, parseErr := strconv.ParseUint(before, 10, 64)
		if parseErr != nil {
			return nil, c.errorFromRequestErr(requestID, fmt.Errorf("invalid before cursor %q: %w", before, parseErr))
		}
		if parsed == 0 {
			return &apiv1.ListAdminEventLogResponse{TotalCount: totalCount}, nil
		}
		startSeq = parsed - 1
	}

	entries, err := c.fetchWireEventLogPage(ctx, stream, startSeq, info.State.FirstSeq, pageSize)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	response := &apiv1.ListAdminEventLogResponse{
		Entries:    entries,
		TotalCount: totalCount,
	}
	if len(entries) > 0 {
		oldestSeq := entries[len(entries)-1].GetSequence()
		response.EndCursor = oldestSeq
		oldest, _ := strconv.ParseUint(oldestSeq, 10, 64)
		response.HasOlder = oldest > info.State.FirstSeq
	}
	return response, nil
}

func (c *wireConn) handleWireGetAdminEventLogEntry(ctx context.Context, userID, requestID string, body *apiv1.GetAdminEventLogEntryRequest) (*apiv1.GetAdminEventLogEntryResponse, *wirev1.WireError) {
	if err := c.requireWireAdminAuditView(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	sequence := strings.TrimSpace(body.GetSequence())
	seq, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("invalid sequence %q: %w", sequence, err))
	}
	stream, err := c.server.core.EventStreamForDebug(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("get EVT stream: %w", err))
	}
	msg, err := stream.GetMsg(ctx, seq)
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return &apiv1.GetAdminEventLogEntryResponse{}, nil
		}
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("get msg %d: %w", seq, err))
	}
	entry, err := wireStreamMsgToEventLogEntry(msg)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetAdminEventLogEntryResponse{Entry: entry}, nil
}

func (c *wireConn) fetchWireEventLogPage(ctx context.Context, stream jetstream.Stream, startSeq, firstSeq uint64, limit int) ([]*apiv1.AdminEventLogEntryView, error) {
	entries := make([]*apiv1.AdminEventLogEntryView, 0, limit)
	if startSeq < firstSeq {
		return entries, nil
	}

	for seq := startSeq; seq >= firstSeq && len(entries) < limit; seq-- {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				if seq == 0 {
					break
				}
				continue
			}
			return nil, fmt.Errorf("get msg %d: %w", seq, err)
		}

		entry, err := wireStreamMsgToEventLogEntry(msg)
		if err != nil {
			entry = &apiv1.AdminEventLogEntryView{
				Sequence:    strconv.FormatUint(seq, 10),
				Subject:     msg.Subject,
				EventType:   "decode-error",
				PayloadJson: fmt.Sprintf("{\"decode_error\": %q}", err.Error()),
			}
		}
		entries = append(entries, entry)

		if seq == 0 {
			break
		}
	}
	return entries, nil
}

func wireStreamMsgToEventLogEntry(msg *jetstream.RawStreamMsg) (*apiv1.AdminEventLogEntryView, error) {
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}

	aggregateType, aggregateID := wireParseAggregateSubject(msg.Subject)
	eventType := wireEventVariantName(&event)
	payloadJSON, err := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}.Marshal(&event)
	if err != nil {
		return nil, fmt.Errorf("marshal payload json: %w", err)
	}

	return &apiv1.AdminEventLogEntryView{
		Sequence:      strconv.FormatUint(msg.Sequence, 10),
		Subject:       msg.Subject,
		AggregateType: aggregateType,
		AggregateId:   aggregateID,
		EventType:     eventType,
		EventId:       event.GetId(),
		ActorId:       event.GetActorId(),
		CreatedAt:     event.GetCreatedAt(),
		PayloadJson:   string(payloadJSON),
	}, nil
}

func wireEventLogTotalCount(messages uint64) (int64, error) {
	if messages > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("event log total count %d exceeds Int64 range", messages)
	}
	return int64(messages), nil
}

func wireParseAggregateSubject(subject string) (aggregateType, aggregateID string) {
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

func wireEventVariantName(event *corev1.Event) string {
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

func (c *wireConn) requireWireAdminAuditView(ctx context.Context, userID string) error {
	canView, err := c.server.core.CanAdminAuditView(ctx, userID)
	if err != nil {
		return fmt.Errorf("check admin.view-audit: %w", err)
	}
	if !canView {
		return core.ErrPermissionDenied
	}
	return nil
}
