package core

import (
	"context"
	"fmt"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
)

type AssetCleanupHealth int

const (
	AssetCleanupHealthInactive AssetCleanupHealth = iota
	AssetCleanupHealthInitializing
	AssetCleanupHealthHealthy
	AssetCleanupHealthRetrying
	AssetCleanupHealthStalled
	AssetCleanupHealthUnavailable
)

type AssetCleanupAdminStatus struct {
	Health               AssetCleanupHealth
	PendingCount         int
	OldestPendingAt      time.Time
	PassInProgress       bool
	LastPassAt           time.Time
	LastSuccessfulPassAt time.Time
	UpdatedAt            time.Time
	LastPassFailed       bool
	LastInspectedSeq     uint64
	LatestDeletionSeq    uint64
}

// AdminCleanupStatus derives shared queue health directly from JetStream.
// Pending work is the deletions that the source consumer has not handed to the
// job queue, plus the cleanup jobs that have not finished, including jobs that
// wait for a retry. Pass-oriented fields are retained for API compatibility
// but remain unknown for continuously delivered durable work.
func (s *AssetModel) AdminCleanupStatus(ctx context.Context) (AssetCleanupAdminStatus, error) {
	status := AssetCleanupAdminStatus{Health: AssetCleanupHealthUnavailable}
	if s == nil || s.EventPublisher == nil || s.cleanupConsumer == nil {
		return status, fmt.Errorf("asset cleanup consumer is not configured")
	}
	jobsConsumer := s.jobConsumers[jobTypeAssetCleanup]
	if jobsConsumer == nil {
		return status, fmt.Errorf("asset cleanup job consumer is not configured")
	}
	latestSeq, err := s.EventPublisher.LastSubjectSeq(ctx, evtstream.AssetEventTypeFilter(evtstream.EventAssetDeleted))
	if err != nil {
		return status, fmt.Errorf("read latest asset deletion sequence: %w", err)
	}
	status.LatestDeletionSeq = latestSeq
	info, err := currentConsumerInfo(ctx, s.js, s.cleanupConsumer)
	if err != nil {
		return status, fmt.Errorf("read asset cleanup consumer: %w", err)
	}
	status.LastInspectedSeq = info.Delivered.Stream
	jobs, err := currentConsumerInfo(ctx, s.js, jobsConsumer)
	if err != nil {
		return status, fmt.Errorf("read asset cleanup job consumer: %w", err)
	}
	pending := info.NumPending + uint64(info.NumAckPending) + jobs.NumPending + uint64(jobs.NumAckPending)
	if pending > uint64(^uint(0)>>1) {
		status.PendingCount = int(^uint(0) >> 1)
	} else {
		status.PendingCount = int(pending)
	}
	switch {
	case status.PendingCount > 0 || info.NumRedelivered > 0 || jobs.NumRedelivered > 0:
		status.Health = AssetCleanupHealthRetrying
	case status.LastInspectedSeq < status.LatestDeletionSeq:
		status.Health = AssetCleanupHealthInitializing
	default:
		status.Health = AssetCleanupHealthHealthy
	}
	return status, nil
}
