package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	DefaultEventLogPageSize   = 50
	MaxEventLogPageSize       = 200
	FilteredEventLogScanLimit = 5000
)

type EventLogFilter struct {
	EventType     string
	ActorID       string
	CreatedAtFrom *time.Time
	CreatedAtTo   *time.Time
}

type EventLogQuery struct {
	Limit  int
	Before string
	Filter EventLogFilter
}

type EventLogConnection struct {
	Entries      []*EventLogEntry
	HasOlder     bool
	EndCursor    *string
	TotalCount   int64
	ScannedCount int32
	ScanLimit    int32
	ScanLimited  bool
}

type EventLogEntry struct {
	Sequence      string
	Subject       string
	AggregateType string
	AggregateID   string
	EventType     string
	EventID       string
	ActorID       string
	CreatedAt     *timestamppb.Timestamp
	PayloadJSON   string
}

type eventLogPageResult struct {
	entries      []*EventLogEntry
	scannedCount int32
	scanLimit    int32
	scanLimited  bool
	scanCursor   *string
}

type eventLogMessageReader interface {
	GetMsg(ctx context.Context, seq uint64, opts ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error)
}

func (c *ChattoCore) ListEventLog(ctx context.Context, userID string, query EventLogQuery) (*EventLogConnection, error) {
	if err := c.requireCanAdminAuditView(ctx, userID); err != nil {
		return nil, err
	}
	filter, err := normalizeEventLogFilter(query.Filter)
	if err != nil {
		return nil, err
	}

	stream := c.storage.serverEvtStream
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("stream info: %w", err)
	}
	totalCount, err := eventLogTotalCount(info.State.Msgs)
	if err != nil {
		return nil, err
	}

	pageSize := eventLogPageSize(query.Limit)
	startSeq := info.State.LastSeq
	if query.Before != "" {
		parsed, parseErr := strconv.ParseUint(query.Before, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: invalid before cursor %q", ErrInvalidArgument, query.Before)
		}
		if parsed == 0 {
			scanLimit := int32(pageSize)
			if filter.active() {
				scanLimit = FilteredEventLogScanLimit
			}
			return &EventLogConnection{
				Entries:      []*EventLogEntry{},
				HasOlder:     false,
				EndCursor:    nil,
				TotalCount:   totalCount,
				ScannedCount: 0,
				ScanLimit:    scanLimit,
				ScanLimited:  false,
			}, nil
		}
		startSeq = parsed - 1
	}

	page, err := fetchEventLogPage(ctx, stream, startSeq, info.State.FirstSeq, pageSize, filter)
	if err != nil {
		return nil, err
	}

	conn := &EventLogConnection{
		Entries:      page.entries,
		TotalCount:   totalCount,
		ScannedCount: page.scannedCount,
		ScanLimit:    page.scanLimit,
		ScanLimited:  page.scanLimited,
	}
	if page.scanLimited {
		conn.EndCursor = page.scanCursor
		if page.scanCursor != nil {
			oldestScanned, _ := strconv.ParseUint(*page.scanCursor, 10, 64)
			conn.HasOlder = oldestScanned > info.State.FirstSeq
		}
	} else if len(page.entries) > 0 {
		oldestSeq := page.entries[len(page.entries)-1].Sequence
		conn.EndCursor = &oldestSeq
		oldest, _ := strconv.ParseUint(oldestSeq, 10, 64)
		conn.HasOlder = oldest > info.State.FirstSeq
	}
	return conn, nil
}

func (c *ChattoCore) EventLogEventTypes(ctx context.Context, userID string) ([]string, error) {
	if err := c.requireCanAdminAuditView(ctx, userID); err != nil {
		return nil, err
	}
	return durableEventLogEventTypes(), nil
}

func (c *ChattoCore) GetEventLogEntry(ctx context.Context, userID, sequence string) (*EventLogEntry, error) {
	if err := c.requireCanAdminAuditView(ctx, userID); err != nil {
		return nil, err
	}
	seq, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid sequence %q", ErrInvalidArgument, sequence)
	}

	msg, err := c.storage.serverEvtStream.GetMsg(ctx, seq)
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get msg %d: %w", seq, err)
	}
	return streamMsgToEventLogEntry(msg)
}

func (c *ChattoCore) requireCanAdminAuditView(ctx context.Context, userID string) error {
	if userID == "" {
		return ErrNotAuthenticated
	}
	canView, err := c.CanAdminAuditView(ctx, userID)
	if err != nil {
		return fmt.Errorf("check admin.view-audit: %w", err)
	}
	if !canView {
		return ErrPermissionDenied
	}
	return nil
}

func eventLogPageSize(limit int) int {
	if limit <= 0 {
		return DefaultEventLogPageSize
	}
	if limit > MaxEventLogPageSize {
		return MaxEventLogPageSize
	}
	return limit
}

func eventLogTotalCount(messages uint64) (int64, error) {
	if messages > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("event log total count %d exceeds Int64 range", messages)
	}
	return int64(messages), nil
}

func normalizeEventLogFilter(filter EventLogFilter) (EventLogFilter, error) {
	normalized := EventLogFilter{
		EventType:     strings.TrimSpace(filter.EventType),
		ActorID:       strings.TrimSpace(filter.ActorID),
		CreatedAtFrom: filter.CreatedAtFrom,
		CreatedAtTo:   filter.CreatedAtTo,
	}
	if normalized.CreatedAtFrom != nil && normalized.CreatedAtTo != nil && normalized.CreatedAtFrom.After(*normalized.CreatedAtTo) {
		return EventLogFilter{}, fmt.Errorf("%w: event log filter created_at_from must be before or equal to created_at_to", ErrInvalidArgument)
	}
	return normalized, nil
}

func (f EventLogFilter) active() bool {
	return f.EventType != "" || f.ActorID != "" || f.CreatedAtFrom != nil || f.CreatedAtTo != nil
}

func (f EventLogFilter) matches(entry *EventLogEntry) bool {
	if f.EventType != "" && entry.EventType != f.EventType {
		return false
	}
	if f.ActorID != "" && entry.ActorID != f.ActorID {
		return false
	}
	if f.CreatedAtFrom != nil || f.CreatedAtTo != nil {
		if entry.CreatedAt == nil {
			return false
		}
		createdAt := entry.CreatedAt.AsTime()
		if f.CreatedAtFrom != nil && createdAt.Before(*f.CreatedAtFrom) {
			return false
		}
		if f.CreatedAtTo != nil && createdAt.After(*f.CreatedAtTo) {
			return false
		}
	}
	return true
}

func durableEventLogEventTypes() []string {
	eventMessage := corev1.File_chatto_core_v1_event_proto.Messages().ByName("Event")
	if eventMessage == nil {
		return []string{"decode-error"}
	}
	oneof := eventMessage.Oneofs().ByName("event")
	if oneof == nil {
		return []string{"decode-error"}
	}

	types := make([]string, 0, oneof.Fields().Len()+1)
	for i := 0; i < oneof.Fields().Len(); i++ {
		field := oneof.Fields().Get(i)
		if field.Kind() == protoreflect.MessageKind && field.Message() != nil {
			types = append(types, string(field.Message().Name()))
		}
	}
	types = append(types, "decode-error")
	sort.Strings(types)
	return types
}

func fetchEventLogPage(
	ctx context.Context,
	stream eventLogMessageReader,
	startSeq uint64,
	firstSeq uint64,
	limit int,
	filter EventLogFilter,
) (eventLogPageResult, error) {
	entries := make([]*EventLogEntry, 0, limit)
	result := eventLogPageResult{
		entries:      entries,
		scannedCount: 0,
		scanLimit:    int32(limit),
	}
	if filter.active() {
		result.scanLimit = FilteredEventLogScanLimit
	}
	if startSeq < firstSeq {
		return result, nil
	}

	filterActive := filter.active()
	for seq := startSeq; seq >= firstSeq && len(entries) < limit; seq-- {
		if filterActive && result.scannedCount >= result.scanLimit {
			result.scanLimited = true
			break
		}
		result.scannedCount++
		scanCursor := strconv.FormatUint(seq, 10)
		result.scanCursor = &scanCursor

		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				if seq == 0 {
					break
				}
				continue
			}
			return eventLogPageResult{}, fmt.Errorf("get msg %d: %w", seq, err)
		}

		entry, err := streamMsgToEventLogEntry(msg)
		if err != nil {
			entry = &EventLogEntry{
				Sequence:    strconv.FormatUint(seq, 10),
				Subject:     msg.Subject,
				EventType:   "decode-error",
				PayloadJSON: fmt.Sprintf("{\"decode_error\": %q}", err.Error()),
			}
		}
		if filterActive && !filter.matches(entry) {
			if seq == 0 {
				break
			}
			continue
		}
		entries = append(entries, entry)

		if seq == 0 {
			break
		}
	}
	result.entries = entries
	return result, nil
}

func streamMsgToEventLogEntry(msg *jetstream.RawStreamMsg) (*EventLogEntry, error) {
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

	return &EventLogEntry{
		Sequence:      strconv.FormatUint(msg.Sequence, 10),
		Subject:       msg.Subject,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		EventID:       event.GetId(),
		ActorID:       event.GetActorId(),
		CreatedAt:     event.GetCreatedAt(),
		PayloadJSON:   string(payloadJSON),
	}, nil
}

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
