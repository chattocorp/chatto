package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	runtimeCredentialExpiryPrefix        = "expiry."
	cookieSessionExpiryMarkerFilter      = "expiry.session.*"
	renewableSessionExpiryMarkerFilter   = "expiry.renewable_session.*"
	runtimeCredentialExpiryCleanupDelay  = time.Second
	runtimeCredentialExpiryRetryInterval = time.Second
)

// runtimeCredentialExpiryModel keeps physical KV cleanup separate from the
// security expiry stored in each mutable session record. Chatto creates each
// marker with KeyTTL through the public KV API. Cookie markers remain fixed;
// renewable-session markers are deleted and recreated when their session
// window advances.
type runtimeCredentialExpiryModel struct {
	core   *ChattoCore
	logger *log.Logger
}

func newRuntimeCredentialExpiryModel(core *ChattoCore, logger *log.Logger) *runtimeCredentialExpiryModel {
	return &runtimeCredentialExpiryModel{core: core, logger: logger}
}

func runtimeCredentialExpiryMarkerKey(dataKey string) string {
	return runtimeCredentialExpiryPrefix + dataKey
}

func runtimeCredentialDataKeyForExpiryMarker(markerKey string) (string, bool) {
	dataKey, ok := strings.CutPrefix(markerKey, runtimeCredentialExpiryPrefix)
	if !ok {
		return "", false
	}
	if strings.HasPrefix(dataKey, authTokenKeyPrefix) || strings.HasPrefix(dataKey, renewableSessionKeyPrefix) {
		return dataKey, true
	}
	return "", false
}

func (m *runtimeCredentialExpiryModel) ensureMarker(ctx context.Context, dataKey string, expiresAt time.Time) error {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return ErrAuthTokenNotFound
	}
	value := []byte(expiresAt.UTC().Format(time.RFC3339Nano))
	if _, err := m.core.storage.runtimeStateKV.Create(
		ctx,
		runtimeCredentialExpiryMarkerKey(dataKey),
		value,
		// Cleanup is deliberately later than security expiry. JetStream can
		// publish a TTL deletion a fraction before the process clock reaches
		// the stored deadline; the delay prevents physical cleanup from ending
		// an otherwise valid request early.
		jetstream.KeyTTL(remaining+runtimeCredentialExpiryCleanupDelay),
	); err != nil && !errors.Is(err, jetstream.ErrKeyExists) {
		return fmt.Errorf("create runtime credential expiry marker: %w", err)
	}
	return nil
}

func (m *runtimeCredentialExpiryModel) deleteMarker(ctx context.Context, dataKey string) error {
	err := m.core.storage.runtimeStateKV.Delete(ctx, runtimeCredentialExpiryMarkerKey(dataKey))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return fmt.Errorf("delete runtime credential expiry marker: %w", err)
	}
	return nil
}

// deleteRecord removes the authoritative session record before its expiry
// marker. Marker cleanup is best-effort because a revoked session must stay
// revoked even when physical cleanup has a transient failure.
func (m *runtimeCredentialExpiryModel) deleteRecord(
	ctx context.Context,
	dataKey string,
	opts ...jetstream.KVDeleteOpt,
) error {
	err := m.core.storage.runtimeStateKV.Delete(ctx, dataKey, opts...)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
		return fmt.Errorf("delete runtime credential %s: %w", dataKey, err)
	}
	if err := m.deleteMarker(ctx, dataKey); err != nil {
		m.logger.Warn("Failed to remove runtime credential expiry marker", "key", dataKey, "error", err)
	}
	return nil
}

// renewMarker replaces a marker through ordinary KV operations. Watchers
// reconcile the related record when they observe the delete, so a concurrent
// repair and this create are both safe.
func (m *runtimeCredentialExpiryModel) renewMarker(ctx context.Context, dataKey string, expiresAt time.Time) error {
	if err := m.deleteMarker(ctx, dataKey); err != nil {
		return err
	}
	return m.ensureMarker(ctx, dataKey, expiresAt)
}

// Run watches marker expiry and reconciles records that expired while every
// Chatto process was stopped. The explicit record expiry remains authoritative
// if cleanup is delayed.
func (m *runtimeCredentialExpiryModel) Run(ctx context.Context) error {
	for {
		if err := m.watch(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			m.logger.Warn("Runtime credential expiry watcher restarting", "error", err)
			timer := time.NewTimer(runtimeCredentialExpiryRetryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func (m *runtimeCredentialExpiryModel) watch(ctx context.Context) error {
	watcher, err := m.core.storage.runtimeStateKV.WatchFiltered(ctx, []string{
		cookieSessionExpiryMarkerFilter,
		renewableSessionExpiryMarkerFilter,
	})
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Stop()

	reconcileCtx, cancelReconcile := context.WithCancel(ctx)
	defer cancelReconcile()
	var reconcileDone <-chan error

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-reconcileDone:
			reconcileDone = nil
			if err != nil && ctx.Err() == nil {
				m.logger.Warn("Runtime credential expiry reconciliation failed", "error", err)
			}
		case entry, ok := <-watcher.Updates():
			if !ok {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return errors.New("watcher stopped")
			}
			if entry == nil {
				if reconcileDone == nil {
					done := make(chan error, 1)
					reconcileDone = done
					go func() { done <- m.reconcile(reconcileCtx) }()
				}
				continue
			}
			if entry.Operation() != jetstream.KeyValueDelete && entry.Operation() != jetstream.KeyValuePurge {
				continue
			}
			dataKey, ok := runtimeCredentialDataKeyForExpiryMarker(entry.Key())
			if !ok {
				continue
			}
			if err := m.reconcileDataKey(ctx, dataKey); err != nil {
				m.logger.Warn("Failed to reconcile runtime credential expiry", "key", dataKey, "error", err)
			}
		}
	}
}

func (m *runtimeCredentialExpiryModel) reconcileDataKey(ctx context.Context, dataKey string) error {
	entry, err := m.core.storage.runtimeStateKV.Get(ctx, dataKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		return fmt.Errorf("get runtime credential %s: %w", dataKey, err)
	}

	switch {
	case strings.HasPrefix(dataKey, authTokenKeyPrefix):
		var token AuthTokenData
		if err := json.Unmarshal(entry.Value(), &token); err != nil ||
			token.presentationOrDefault() != AuthTokenPresentationCookie ||
			token.CreatedAt.IsZero() {
			return nil
		}
		expiresAt := token.ExpiresAt
		if expiresAt.IsZero() {
			expiresAt = token.CreatedAt.Add(m.core.cookieSessionTTL())
		}
		return m.reconcileEntry(ctx, entry, expiresAt)
	case strings.HasPrefix(dataKey, renewableSessionKeyPrefix):
		var session RenewableSession
		if err := json.Unmarshal(entry.Value(), &session); err != nil || session.ExpiresAt.IsZero() {
			return nil
		}
		return m.reconcileEntry(ctx, entry, session.ExpiresAt)
	default:
		return nil
	}
}

func (m *runtimeCredentialExpiryModel) reconcile(ctx context.Context) error {
	if err := m.reconcileAuthTokens(ctx); err != nil {
		return err
	}
	return m.reconcileRenewableSessions(ctx)
}

func (m *runtimeCredentialExpiryModel) reconcileAuthTokens(ctx context.Context) error {
	return m.forEachKey(ctx, authTokenKeyPrefix+"*", func(entry jetstream.KeyValueEntry) error {
		var token AuthTokenData
		if err := json.Unmarshal(entry.Value(), &token); err != nil {
			return nil
		}
		if token.presentationOrDefault() != AuthTokenPresentationCookie || token.CreatedAt.IsZero() {
			return nil
		}
		expiresAt := token.ExpiresAt
		if expiresAt.IsZero() {
			expiresAt = token.CreatedAt.Add(m.core.cookieSessionTTL())
		}
		return m.reconcileEntry(ctx, entry, expiresAt)
	})
}

func (m *runtimeCredentialExpiryModel) reconcileRenewableSessions(ctx context.Context) error {
	return m.forEachKey(ctx, renewableSessionKeyPrefix+"*", func(entry jetstream.KeyValueEntry) error {
		var session RenewableSession
		if err := json.Unmarshal(entry.Value(), &session); err != nil || session.ExpiresAt.IsZero() {
			return nil
		}
		return m.reconcileEntry(ctx, entry, session.ExpiresAt)
	})
}

func (m *runtimeCredentialExpiryModel) reconcileEntry(ctx context.Context, entry jetstream.KeyValueEntry, expiresAt time.Time) error {
	if !time.Now().Before(expiresAt) {
		if err := m.deleteRecord(ctx, entry.Key(), jetstream.LastRevision(entry.Revision())); err != nil &&
			!isRuntimeStateRevisionConflict(err) {
			return fmt.Errorf("delete expired runtime credential %s: %w", entry.Key(), err)
		}
		return nil
	}
	return m.ensureMarker(ctx, entry.Key(), expiresAt)
}

func (m *runtimeCredentialExpiryModel) forEachKey(
	ctx context.Context,
	filter string,
	visit func(jetstream.KeyValueEntry) error,
) error {
	lister, err := m.core.storage.runtimeStateKV.ListKeysFiltered(ctx, filter)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list %s: %w", filter, err)
	}
	for key := range lister.Keys() {
		entry, err := m.core.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				continue
			}
			return fmt.Errorf("get %s: %w", key, err)
		}
		if err := visit(entry); err != nil {
			return err
		}
	}
	return nil
}
