package connectapi

import (
	"context"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
)

type adminDiagnosticsService struct {
	api *API
}

func (s *adminDiagnosticsService) GetSystemInfo(ctx context.Context, _ *connect.Request[adminv1.GetSystemInfoRequest]) (*connect.Response[adminv1.GetSystemInfoResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	diagnostics, err := s.api.core.GetAdminDiagnostics(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&adminv1.GetSystemInfoResponse{
		SystemInfo:   adminSystemInfo(diagnostics),
		Projections:  adminProjectionStates(diagnostics.Projections),
		AssetCleanup: adminAssetCleanupStatus(diagnostics.AssetCleanup),
	}), nil
}

func adminAssetCleanupStatus(status core.AssetCleanupAdminStatus) *adminv1.AdminAssetCleanupStatus {
	return &adminv1.AdminAssetCleanupStatus{
		Health:                 adminAssetCleanupHealth(status.Health),
		PendingCount:           int64(status.PendingCount),
		OldestPendingAt:        optionalTimestamp(status.OldestPendingAt),
		PassInProgress:         status.PassInProgress,
		LastPassAt:             optionalTimestamp(status.LastPassAt),
		LastSuccessfulPassAt:   optionalTimestamp(status.LastSuccessfulPassAt),
		UpdatedAt:              optionalTimestamp(status.UpdatedAt),
		LastPassFailed:         status.LastPassFailed,
		LastInspectedSequence:  strconv.FormatUint(status.LastInspectedSeq, 10),
		LatestDeletionSequence: strconv.FormatUint(status.LatestDeletionSeq, 10),
	}
}

func adminAssetCleanupHealth(health core.AssetCleanupHealth) adminv1.AdminAssetCleanupHealth {
	switch health {
	case core.AssetCleanupHealthInactive:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_INACTIVE
	case core.AssetCleanupHealthInitializing:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_INITIALIZING
	case core.AssetCleanupHealthHealthy:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_HEALTHY
	case core.AssetCleanupHealthRetrying:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_RETRYING
	case core.AssetCleanupHealthStalled:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_STALLED
	case core.AssetCleanupHealthUnavailable:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_UNAVAILABLE
	default:
		return adminv1.AdminAssetCleanupHealth_ADMIN_ASSET_CLEANUP_HEALTH_UNSPECIFIED
	}
}

func optionalTimestamp(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value)
}

func adminSystemInfo(diagnostics *core.AdminDiagnostics) *adminv1.AdminSystemInfo {
	return &adminv1.AdminSystemInfo{
		Connection: adminConnectionInfo(diagnostics.Connection),
		Account:    adminAccountInfo(diagnostics.Account),
		Nats:       adminNatsStats(diagnostics.JetStream),
		Stats:      adminServerStats(diagnostics.Stats),
	}
}

func adminProjectionStates(states []core.ProjectionAdminState) []*adminv1.AdminProjectionState {
	out := make([]*adminv1.AdminProjectionState, 0, len(states))
	for _, state := range states {
		out = append(out, adminProjectionState(state))
	}
	return out
}

func adminConnectionInfo(info *core.ConnectionInfo) *adminv1.AdminConnectionInfo {
	if info == nil {
		return &adminv1.AdminConnectionInfo{}
	}
	return &adminv1.AdminConnectionInfo{
		Connected:  info.Connected,
		ServerId:   info.ServerID,
		ServerName: info.ServerName,
		Version:    info.Version,
		MaxPayload: info.MaxPayload,
		Rtt:        info.RTT,
	}
}

func adminAccountInfo(info *core.AccountInfo) *adminv1.AdminAccountInfo {
	if info == nil {
		return &adminv1.AdminAccountInfo{}
	}
	return &adminv1.AdminAccountInfo{
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

func adminServerStats(stats *core.ServerStats) *adminv1.AdminServerStats {
	if stats == nil {
		return &adminv1.AdminServerStats{}
	}
	return &adminv1.AdminServerStats{
		UserCount:        int32(stats.UserCount),
		ChannelRoomCount: int32(stats.ChannelRoomCount),
		DmRoomCount:      int32(stats.DMRoomCount),
	}
}

func adminNatsStats(stats *core.JetStreamStats) *adminv1.AdminNatsStats {
	if stats == nil {
		return &adminv1.AdminNatsStats{}
	}

	streams := make([]*adminv1.AdminNatsStreamInfo, 0, len(stats.Streams))
	for _, stream := range stats.Streams {
		streams = append(streams, &adminv1.AdminNatsStreamInfo{
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

	consumers := make([]*adminv1.AdminNatsConsumerInfo, 0, len(stats.Consumers))
	for _, consumer := range stats.Consumers {
		consumers = append(consumers, &adminv1.AdminNatsConsumerInfo{
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

	return &adminv1.AdminNatsStats{
		TotalMessages:        int64(stats.TotalMessages),
		TotalBytes:           int64(stats.TotalBytes),
		TotalConsumerPending: int64(stats.TotalConsumerPending),
		TotalAckPending:      int32(stats.TotalAckPending),
		Streams:              streams,
		Consumers:            consumers,
	}
}

func adminProjectionState(state core.ProjectionAdminState) *adminv1.AdminProjectionState {
	metrics := make([]*adminv1.AdminProjectionMetric, 0, len(state.Metrics))
	for _, metric := range state.Metrics {
		metrics = append(metrics, &adminv1.AdminProjectionMetric{
			Name:  metric.Name,
			Value: metric.Value,
			Bytes: metric.Bytes,
		})
	}

	var startupDurationSeconds *float64
	if state.StartupComplete {
		startupDurationSeconds = &state.StartupDuration
	}

	return &adminv1.AdminProjectionState{
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
