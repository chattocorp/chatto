package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
)

const (
	assetCleanupStatusKey      = "asset_cleanup.status"
	assetCleanupHeartbeatEvery = 15 * time.Second
	assetCleanupHeartbeatStale = 45 * time.Second
)

type assetCleanupPassStatus struct {
	InProgress           bool
	LastPassAt           time.Time
	LastSuccessfulPassAt time.Time
	LastPassFailed       bool
}

type assetCleanupStatusRecord struct {
	OwnerID              string    `json:"ownerId"`
	UpdatedAt            time.Time `json:"updatedAt"`
	InitialScanComplete  bool      `json:"initialScanComplete"`
	PassInProgress       bool      `json:"passInProgress"`
	PendingCount         int       `json:"pendingCount"`
	OldestPendingAt      time.Time `json:"oldestPendingAt,omitempty"`
	LastPassAt           time.Time `json:"lastPassAt,omitempty"`
	LastSuccessfulPassAt time.Time `json:"lastSuccessfulPassAt,omitempty"`
	LastPassFailed       bool      `json:"lastPassFailed"`
	LastInspectedSeq     uint64    `json:"lastInspectedSeq"`
}

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

func (s *AssetModel) setAssetCleanupPassStarted() {
	s.cleanupStatusMu.Lock()
	s.cleanupPass.InProgress = true
	s.cleanupStatusMu.Unlock()
}

func (s *AssetModel) setAssetCleanupPassFinished(err error) {
	now := time.Now().UTC()
	s.cleanupStatusMu.Lock()
	s.cleanupPass.InProgress = false
	s.cleanupPass.LastPassAt = now
	s.cleanupPass.LastPassFailed = err != nil
	if err == nil {
		s.cleanupPass.LastSuccessfulPassAt = now
	}
	s.cleanupStatusMu.Unlock()
}

func (s *AssetModel) assetCleanupStatusRecord(now time.Time) assetCleanupStatusRecord {
	consumer := s.cleanupConsumer.Status()
	s.cleanupStatusMu.RLock()
	pass := s.cleanupPass
	s.cleanupStatusMu.RUnlock()
	return assetCleanupStatusRecord{
		OwnerID:              s.cleanupLease.OwnerID(),
		UpdatedAt:            now.UTC(),
		InitialScanComplete:  consumer.Initialized,
		PassInProgress:       pass.InProgress,
		PendingCount:         consumer.PendingCount,
		OldestPendingAt:      consumer.OldestPendingAt,
		LastPassAt:           pass.LastPassAt,
		LastSuccessfulPassAt: pass.LastSuccessfulPassAt,
		LastPassFailed:       pass.LastPassFailed,
		LastInspectedSeq:     consumer.AfterSeq,
	}
}

func (s *AssetModel) writeAssetCleanupStatus(ctx context.Context) error {
	if s == nil || s.cleanupLease == nil || s.storage == nil || s.storage.memoryCacheKV == nil {
		return fmt.Errorf("asset cleanup status storage is not configured")
	}
	data, err := json.Marshal(s.assetCleanupStatusRecord(time.Now()))
	if err != nil {
		return fmt.Errorf("marshal asset cleanup status: %w", err)
	}
	if _, err := s.storage.memoryCacheKV.Put(ctx, assetCleanupStatusKey, data); err != nil {
		return fmt.Errorf("write asset cleanup status: %w", err)
	}
	return nil
}

func (s *AssetModel) AdminCleanupStatus(ctx context.Context) (AssetCleanupAdminStatus, error) {
	status := AssetCleanupAdminStatus{Health: AssetCleanupHealthInactive}
	if s == nil || s.storage == nil || s.storage.memoryCacheKV == nil {
		return status, fmt.Errorf("asset cleanup status storage is not configured")
	}

	latestSeq, err := s.EventPublisher.LastSubjectSeq(ctx, events.AssetEventTypeFilter(events.EventAssetDeleted))
	if err != nil {
		return status, fmt.Errorf("read latest asset deletion sequence: %w", err)
	}
	status.LatestDeletionSeq = latestSeq

	var record assetCleanupStatusRecord
	entry, err := s.storage.memoryCacheKV.Get(ctx, assetCleanupStatusKey)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return status, fmt.Errorf("read asset cleanup status: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(entry.Value(), &record); err != nil {
			return status, fmt.Errorf("decode asset cleanup status: %w", err)
		}
		status = AssetCleanupAdminStatus{
			PendingCount:         record.PendingCount,
			OldestPendingAt:      record.OldestPendingAt,
			PassInProgress:       record.PassInProgress,
			LastPassAt:           record.LastPassAt,
			LastSuccessfulPassAt: record.LastSuccessfulPassAt,
			UpdatedAt:            record.UpdatedAt,
			LastPassFailed:       record.LastPassFailed,
			LastInspectedSeq:     record.LastInspectedSeq,
			LatestDeletionSeq:    latestSeq,
		}
	}

	leaseEntry, err := s.storage.memoryCacheKV.Get(ctx, "lease."+assetCleanupLeaseName)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return status, nil
		}
		return status, fmt.Errorf("read asset cleanup lease: %w", err)
	}
	leaseRecord, err := lease.DecodeRecord(leaseEntry.Value())
	if err != nil {
		return status, fmt.Errorf("decode asset cleanup lease: %w", err)
	}

	if record.OwnerID == "" {
		status.Health = AssetCleanupHealthInitializing
		return status, nil
	}

	now := time.Now()
	switch {
	case record.OwnerID != leaseRecord.OwnerID:
		status.Health = AssetCleanupHealthInitializing
	case record.UpdatedAt.IsZero() || now.Sub(record.UpdatedAt) > assetCleanupHeartbeatStale:
		status.Health = AssetCleanupHealthStalled
	case !record.InitialScanComplete:
		status.Health = AssetCleanupHealthInitializing
	case record.PendingCount > 0 || record.LastPassFailed || record.LastInspectedSeq < latestSeq:
		status.Health = AssetCleanupHealthRetrying
	default:
		status.Health = AssetCleanupHealthHealthy
	}
	return status, nil
}
