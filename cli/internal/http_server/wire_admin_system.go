package http_server

import (
	"context"
	"fmt"
	"strconv"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetAdminSystemInfo(ctx context.Context, userID, requestID string) (*apiv1.GetAdminSystemInfoResponse, *wirev1.WireError) {
	if err := c.requireWireServerOwner(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.requireWireAdminSystemView(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	systemInfo, err := c.adminSystemInfoView(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	projections, err := c.adminProjectionStateViews(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.GetAdminSystemInfoResponse{
		SystemInfo:  systemInfo,
		Projections: projections,
	}, nil
}

func (c *wireConn) adminSystemInfoView(ctx context.Context) (*apiv1.AdminSystemInfoView, error) {
	connInfo := c.server.core.GetConnectionInfo()
	connection := &apiv1.AdminConnectionInfoView{
		Connected:  connInfo.Connected,
		ServerId:   connInfo.ServerID,
		ServerName: connInfo.ServerName,
		Version:    connInfo.Version,
		MaxPayload: connInfo.MaxPayload,
		Rtt:        connInfo.RTT,
	}

	accInfo, err := c.server.core.GetAccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}
	account := &apiv1.AdminAccountInfoView{
		Memory:        accInfo.Memory,
		MemoryUsed:    accInfo.MemoryUsed,
		Storage:       accInfo.Storage,
		StorageUsed:   accInfo.StorageUsed,
		Streams:       int32(accInfo.Streams),
		StreamsUsed:   int32(accInfo.StreamsUsed),
		Consumers:     int32(accInfo.Consumers),
		ConsumersUsed: int32(accInfo.ConsumersUsed),
	}

	coreStats, err := c.server.core.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server stats: %w", err)
	}
	stats := &apiv1.AdminServerStatsView{
		UserCount:        int32(coreStats.UserCount),
		ChannelRoomCount: int32(coreStats.ChannelRoomCount),
		DmRoomCount:      int32(coreStats.DMRoomCount),
	}

	natsStats, err := c.server.core.GetJetStreamStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get NATS stats: %w", err)
	}

	return &apiv1.AdminSystemInfoView{
		Connection: connection,
		Account:    account,
		Nats:       adminNatsStatsView(natsStats),
		Stats:      stats,
	}, nil
}

func adminNatsStatsView(stats *core.JetStreamStats) *apiv1.AdminNatsStatsView {
	if stats == nil {
		return &apiv1.AdminNatsStatsView{}
	}
	streams := make([]*apiv1.AdminNatsStreamInfoView, 0, len(stats.Streams))
	for _, stream := range stats.Streams {
		streams = append(streams, &apiv1.AdminNatsStreamInfoView{
			Name:          stream.Name,
			Description:   stream.Description,
			Subjects:      append([]string(nil), stream.Subjects...),
			Storage:       stream.Storage,
			Messages:      stream.Messages,
			Bytes:         stream.Bytes,
			FirstSequence: strconv.FormatUint(stream.FirstSeq, 10),
			LastSequence:  strconv.FormatUint(stream.LastSeq, 10),
			ConsumerCount: int32(stream.ConsumerCount),
			Replicas:      int32(stream.Replicas),
			ClusterLeader: stream.ClusterLeader,
		})
	}

	consumers := make([]*apiv1.AdminNatsConsumerInfoView, 0, len(stats.Consumers))
	for _, consumer := range stats.Consumers {
		consumers = append(consumers, &apiv1.AdminNatsConsumerInfoView{
			Stream:                    consumer.Stream,
			Name:                      consumer.Name,
			Durable:                   consumer.Durable,
			FilterSubject:             consumer.FilterSubject,
			FilterSubjects:            append([]string(nil), consumer.FilterSubjects...),
			AckPolicy:                 consumer.AckPolicy,
			PullBased:                 consumer.PullBased,
			PushBound:                 consumer.PushBound,
			Pending:                   consumer.Pending,
			AckPending:                int32(consumer.AckPending),
			Redelivered:               int32(consumer.Redelivered),
			Waiting:                   int32(consumer.Waiting),
			DeliveredConsumerSequence: strconv.FormatUint(consumer.DeliveredConsumerSeq, 10),
			DeliveredStreamSequence:   strconv.FormatUint(consumer.DeliveredStreamSeq, 10),
			AckFloorConsumerSequence:  strconv.FormatUint(consumer.AckFloorConsumerSeq, 10),
			AckFloorStreamSequence:    strconv.FormatUint(consumer.AckFloorStreamSeq, 10),
		})
	}

	return &apiv1.AdminNatsStatsView{
		Streams:              streams,
		Consumers:            consumers,
		TotalMessages:        stats.TotalMessages,
		TotalBytes:           stats.TotalBytes,
		TotalConsumerPending: stats.TotalConsumerPending,
		TotalAckPending:      int32(stats.TotalAckPending),
	}
}

func (c *wireConn) adminProjectionStateViews(ctx context.Context) ([]*apiv1.AdminProjectionStateView, error) {
	states, err := c.server.core.ProjectionAdminStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("projection states: %w", err)
	}
	out := make([]*apiv1.AdminProjectionStateView, 0, len(states))
	for _, state := range states {
		out = append(out, &apiv1.AdminProjectionStateView{
			Key:                    state.Key,
			Name:                   state.Name,
			Subjects:               append([]string(nil), state.Subjects...),
			Started:                state.Started,
			LastAppliedSequence:    strconv.FormatUint(state.LastAppliedSeq, 10),
			MatchingStreamSequence: strconv.FormatUint(state.MatchingStreamSeq, 10),
			StreamLastSequence:     strconv.FormatUint(state.StreamLastSeq, 10),
			Lag:                    state.Lag,
			Failed:                 state.Failed,
			FailedSequence:         strconv.FormatUint(state.FailedSeq, 10),
			Failure:                state.Failure,
			EntryCount:             state.EntryCount,
			EstimatedBytes:         state.EstimatedBytes,
			AverageEntryBytes:      state.AverageEntryBytes,
		})
	}
	return out, nil
}

func (c *wireConn) requireWireServerOwner(ctx context.Context, userID string) error {
	isOwner, err := c.server.core.IsServerOwner(ctx, userID)
	if err != nil {
		return fmt.Errorf("check owner role: %w", err)
	}
	if !isOwner {
		return core.ErrPermissionDenied
	}
	return nil
}

func (c *wireConn) requireWireAdminSystemView(ctx context.Context, userID string) error {
	canView, err := c.server.core.CanAdminSystemView(ctx, userID)
	if err != nil {
		return fmt.Errorf("check admin.view-system: %w", err)
	}
	if !canView {
		return core.ErrPermissionDenied
	}
	return nil
}
