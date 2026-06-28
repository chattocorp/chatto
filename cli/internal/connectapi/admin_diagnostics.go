package connectapi

import (
	"context"
	"strconv"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
)

type adminDiagnosticsService struct {
	api *API
}

func (s *adminDiagnosticsService) GetSystemInfo(ctx context.Context, _ *connect.Request[appv1.GetSystemInfoRequest]) (*connect.Response[appv1.GetSystemInfoResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	diagnostics, err := s.api.core.GetAdminDiagnostics(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&appv1.GetSystemInfoResponse{
		SystemInfo:  adminSystemInfo(diagnostics),
		Projections: adminProjectionStates(diagnostics.Projections),
	}), nil
}

func adminSystemInfo(diagnostics *core.AdminDiagnostics) *appv1.AdminSystemInfo {
	return &appv1.AdminSystemInfo{
		Connection: adminConnectionInfo(diagnostics.Connection),
		Account:    adminAccountInfo(diagnostics.Account),
		Nats:       adminNatsStats(diagnostics.JetStream),
		Stats:      adminServerStats(diagnostics.Stats),
	}
}

func adminProjectionStates(states []core.ProjectionAdminState) []*appv1.AdminProjectionState {
	out := make([]*appv1.AdminProjectionState, 0, len(states))
	for _, state := range states {
		out = append(out, adminProjectionState(state))
	}
	return out
}

func adminConnectionInfo(info *core.ConnectionInfo) *appv1.AdminConnectionInfo {
	if info == nil {
		return &appv1.AdminConnectionInfo{}
	}
	return &appv1.AdminConnectionInfo{
		Connected:  info.Connected,
		ServerId:   info.ServerID,
		ServerName: info.ServerName,
		Version:    info.Version,
		MaxPayload: info.MaxPayload,
		Rtt:        info.RTT,
	}
}

func adminAccountInfo(info *core.AccountInfo) *appv1.AdminAccountInfo {
	if info == nil {
		return &appv1.AdminAccountInfo{}
	}
	return &appv1.AdminAccountInfo{
		Memory:        int64(info.Memory),
		MemoryUsed:    int64(info.MemoryUsed),
		Storage:       int64(info.Storage),
		StorageUsed:   int64(info.StorageUsed),
		Streams:       int32(info.Streams),
		StreamsUsed:   int32(info.StreamsUsed),
		Consumers:     int32(info.Consumers),
		ConsumersUsed: int32(info.ConsumersUsed),
	}
}

func adminServerStats(stats *core.ServerStats) *appv1.AdminServerStats {
	if stats == nil {
		return &appv1.AdminServerStats{}
	}
	return &appv1.AdminServerStats{
		UserCount:        int32(stats.UserCount),
		ChannelRoomCount: int32(stats.ChannelRoomCount),
		DmRoomCount:      int32(stats.DMRoomCount),
	}
}

func adminNatsStats(stats *core.JetStreamStats) *appv1.AdminNatsStats {
	if stats == nil {
		return &appv1.AdminNatsStats{}
	}

	streams := make([]*appv1.AdminNatsStreamInfo, 0, len(stats.Streams))
	for _, stream := range stats.Streams {
		streams = append(streams, &appv1.AdminNatsStreamInfo{
			Name:          stream.Name,
			Description:   stream.Description,
			Subjects:      append([]string(nil), stream.Subjects...),
			Storage:       stream.Storage,
			Messages:      int64(stream.Messages),
			Bytes:         int64(stream.Bytes),
			FirstSequence: strconv.FormatUint(stream.FirstSeq, 10),
			LastSequence:  strconv.FormatUint(stream.LastSeq, 10),
			ConsumerCount: int32(stream.ConsumerCount),
			Replicas:      int32(stream.Replicas),
			ClusterLeader: stream.ClusterLeader,
		})
	}

	consumers := make([]*appv1.AdminNatsConsumerInfo, 0, len(stats.Consumers))
	for _, consumer := range stats.Consumers {
		consumers = append(consumers, &appv1.AdminNatsConsumerInfo{
			Stream:                    consumer.Stream,
			Name:                      consumer.Name,
			Durable:                   consumer.Durable,
			FilterSubject:             consumer.FilterSubject,
			FilterSubjects:            append([]string(nil), consumer.FilterSubjects...),
			AckPolicy:                 consumer.AckPolicy,
			PullBased:                 consumer.PullBased,
			PushBound:                 consumer.PushBound,
			Pending:                   int64(consumer.Pending),
			AckPending:                int32(consumer.AckPending),
			Redelivered:               int32(consumer.Redelivered),
			Waiting:                   int32(consumer.Waiting),
			DeliveredConsumerSequence: strconv.FormatUint(consumer.DeliveredConsumerSeq, 10),
			DeliveredStreamSequence:   strconv.FormatUint(consumer.DeliveredStreamSeq, 10),
			AckFloorConsumerSequence:  strconv.FormatUint(consumer.AckFloorConsumerSeq, 10),
			AckFloorStreamSequence:    strconv.FormatUint(consumer.AckFloorStreamSeq, 10),
		})
	}

	return &appv1.AdminNatsStats{
		TotalMessages:        int64(stats.TotalMessages),
		TotalBytes:           int64(stats.TotalBytes),
		TotalConsumerPending: int64(stats.TotalConsumerPending),
		TotalAckPending:      int32(stats.TotalAckPending),
		Streams:              streams,
		Consumers:            consumers,
	}
}

func adminProjectionState(state core.ProjectionAdminState) *appv1.AdminProjectionState {
	metrics := make([]*appv1.AdminProjectionMetric, 0, len(state.Metrics))
	for _, metric := range state.Metrics {
		metrics = append(metrics, &appv1.AdminProjectionMetric{
			Name:  metric.Name,
			Value: metric.Value,
			Bytes: metric.Bytes,
		})
	}

	var startupDurationSeconds *float64
	if state.StartupComplete {
		startupDurationSeconds = &state.StartupDuration
	}

	return &appv1.AdminProjectionState{
		Key:                    state.Key,
		Name:                   state.Name,
		Subjects:               append([]string(nil), state.Subjects...),
		Started:                state.Started,
		StartupDurationSeconds: startupDurationSeconds,
		LastAppliedSequence:    strconv.FormatUint(state.LastAppliedSeq, 10),
		MatchingStreamSequence: strconv.FormatUint(state.MatchingStreamSeq, 10),
		StreamLastSequence:     strconv.FormatUint(state.StreamLastSeq, 10),
		Lag:                    int64(state.Lag),
		Failed:                 state.Failed,
		FailedSequence:         strconv.FormatUint(state.FailedSeq, 10),
		Failure:                state.Failure,
		EntryCount:             state.EntryCount,
		EstimatedBytes:         state.EstimatedBytes,
		AverageEntryBytes:      state.AverageEntryBytes,
		Metrics:                metrics,
	}
}
