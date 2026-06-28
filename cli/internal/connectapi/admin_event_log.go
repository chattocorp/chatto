package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type adminEventLogService struct {
	api *API
}

func (s *adminEventLogService) ListEvents(ctx context.Context, req *connect.Request[apiv1.ListEventsRequest]) (*connect.Response[apiv1.ListEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	filter, err := adminEventLogFilterFromAPI(req.Msg.GetFilter())
	if err != nil {
		return nil, err
	}
	conn, err := s.api.core.ListEventLog(ctx, caller.UserID, core.EventLogQuery{
		Limit:  int(req.Msg.GetLimit()),
		Before: req.Msg.GetBefore(),
		Filter: filter,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(adminEventLogConnectionToAPI(conn)), nil
}

func (s *adminEventLogService) ListEventTypes(ctx context.Context, _ *connect.Request[apiv1.ListEventTypesRequest]) (*connect.Response[apiv1.ListEventTypesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	eventTypes, err := s.api.core.EventLogEventTypes(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListEventTypesResponse{EventTypes: eventTypes}), nil
}

func (s *adminEventLogService) GetEvent(ctx context.Context, req *connect.Request[apiv1.GetEventRequest]) (*connect.Response[apiv1.GetEventResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	entry, err := s.api.core.GetEventLogEntry(ctx, caller.UserID, req.Msg.GetSequence())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetEventResponse{
		Entry: adminEventLogEntryToAPI(entry),
	}), nil
}

func adminEventLogFilterFromAPI(filter *apiv1.AdminEventLogFilter) (core.EventLogFilter, error) {
	if filter == nil {
		return core.EventLogFilter{}, nil
	}
	from, err := checkedEventLogTimestamp(filter.GetCreatedAtFrom(), "created_at_from")
	if err != nil {
		return core.EventLogFilter{}, err
	}
	to, err := checkedEventLogTimestamp(filter.GetCreatedAtTo(), "created_at_to")
	if err != nil {
		return core.EventLogFilter{}, err
	}
	return core.EventLogFilter{
		EventType:     filter.GetEventType(),
		ActorID:       filter.GetActorId(),
		CreatedAtFrom: from,
		CreatedAtTo:   to,
	}, nil
}

func checkedEventLogTimestamp(ts *timestamppb.Timestamp, field string) (*time.Time, error) {
	if ts == nil {
		return nil, nil
	}
	if err := ts.CheckValid(); err != nil {
		return nil, invalidArgument(field + " is invalid")
	}
	value := ts.AsTime()
	return &value, nil
}

func adminEventLogConnectionToAPI(conn *core.EventLogConnection) *apiv1.ListEventsResponse {
	if conn == nil {
		return &apiv1.ListEventsResponse{}
	}
	entries := make([]*apiv1.AdminEventLogEntry, 0, len(conn.Entries))
	for _, entry := range conn.Entries {
		entries = append(entries, adminEventLogEntryToAPI(entry))
	}
	return &apiv1.ListEventsResponse{
		Entries:      entries,
		HasOlder:     conn.HasOlder,
		EndCursor:    conn.EndCursor,
		TotalCount:   conn.TotalCount,
		ScannedCount: conn.ScannedCount,
		ScanLimit:    conn.ScanLimit,
		ScanLimited:  conn.ScanLimited,
	}
}

func adminEventLogEntryToAPI(entry *core.EventLogEntry) *apiv1.AdminEventLogEntry {
	if entry == nil {
		return nil
	}
	return &apiv1.AdminEventLogEntry{
		Sequence:      entry.Sequence,
		Subject:       entry.Subject,
		AggregateType: entry.AggregateType,
		AggregateId:   entry.AggregateID,
		EventType:     entry.EventType,
		EventId:       entry.EventID,
		ActorId:       entry.ActorID,
		CreatedAt:     entry.CreatedAt,
		PayloadJson:   entry.PayloadJSON,
	}
}
