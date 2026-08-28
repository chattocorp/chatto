package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	credentialUsageRuntimeStatePrefix = "credential_usage.bot."
	credentialUsageFlushInterval      = time.Minute
	credentialUsageRetryInterval      = 5 * time.Second
	credentialUsageSweepInterval      = time.Minute
	credentialUsageWebhookKind        = "incoming_webhook"
)

// credentialUsageRecorder records approximate credential-use timestamps
// without putting RUNTIME_STATE availability on an authentication path. Its
// merge operation uses KV revision OCC, so concurrent replicas retain the
// greatest observed timestamp for each credential.
type credentialUsageRecorder struct {
	kv            jetstream.KeyValue
	logger        *log.Logger
	flushInterval time.Duration
	retryInterval time.Duration
	sweepInterval time.Duration
	isActive      func(botID, credentialKey string) bool
	wake          chan struct{}

	mu        sync.RWMutex
	pending   map[string]map[string]time.Time
	observed  map[string]map[string]time.Time
	lastFlush map[string]map[string]time.Time
	lastSweep time.Time
}

func newCredentialUsageRecorder(kv jetstream.KeyValue, logger *log.Logger, isActive func(botID, credentialKey string) bool) *credentialUsageRecorder {
	return &credentialUsageRecorder{
		kv:            kv,
		logger:        logger,
		flushInterval: credentialUsageFlushInterval,
		retryInterval: credentialUsageRetryInterval,
		sweepInterval: credentialUsageSweepInterval,
		isActive:      isActive,
		wake:          make(chan struct{}, 1),
		pending:       make(map[string]map[string]time.Time),
		observed:      make(map[string]map[string]time.Time),
		lastFlush:     make(map[string]map[string]time.Time),
	}
}

func credentialUsageRuntimeStateKey(botID string) string {
	return credentialUsageRuntimeStatePrefix + botID
}

func incomingWebhookUsageKey(webhookID string) string {
	return credentialUsageWebhookKind + ":" + webhookID
}

// Record retains the newest process-local observation and wakes the
// best-effort writer. It never waits for NATS and never returns an error.
func (r *credentialUsageRecorder) Record(botID, credentialKey string, usedAt time.Time) {
	r.recordIfActive(botID, credentialKey, usedAt, nil)
}

// recordIfActive records an observation only while the credential is still
// active. The active check and Forget use the same lock, so revocation cannot
// leave a late process-local observation behind.
func (r *credentialUsageRecorder) recordIfActive(botID, credentialKey string, usedAt time.Time, isActive func() bool) {
	if r == nil || botID == "" || credentialKey == "" || usedAt.IsZero() {
		return
	}
	usedAt = usedAt.UTC().Truncate(time.Millisecond)
	r.mu.Lock()
	if isActive != nil && !isActive() {
		r.mu.Unlock()
		return
	}
	setNewestUsage(r.pending, botID, credentialKey, usedAt)
	setNewestUsage(r.observed, botID, credentialKey, usedAt)
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Forget removes one local observation and attempts to remove its persisted
// telemetry. Lifecycle state remains authoritative when this best-effort
// cleanup cannot complete.
func (r *credentialUsageRecorder) Forget(ctx context.Context, botID, credentialKey string) {
	if r == nil || botID == "" || credentialKey == "" {
		return
	}
	r.mu.Lock()
	deleteCredentialUsage(r.pending, botID, credentialKey)
	deleteCredentialUsage(r.observed, botID, credentialKey)
	deleteCredentialUsage(r.lastFlush, botID, credentialKey)
	r.mu.Unlock()
	if err := r.deletePersisted(ctx, botID, credentialKey); err != nil && r.logger != nil {
		r.logger.Warn("Failed to remove revoked credential usage telemetry", "error", err)
	}
}

// ForgetAll removes the operational record for a deleted bot. Account
// deletion remains complete if this optional cleanup fails.
func (r *credentialUsageRecorder) ForgetAll(ctx context.Context, botID string) {
	if r == nil || botID == "" {
		return
	}
	r.mu.Lock()
	delete(r.pending, botID)
	delete(r.observed, botID)
	delete(r.lastFlush, botID)
	r.mu.Unlock()
	if err := r.kv.Delete(ctx, credentialUsageRuntimeStateKey(botID)); err != nil && !isRuntimeStateKeyAbsent(err) && r.logger != nil {
		r.logger.Warn("Failed to remove deleted bot credential usage telemetry", "error", err)
	}
}

// LastUsed reads one per-bot record and merges observations that this process
// has not flushed yet. A missing record means that no use was recorded.
// available is false when persisted telemetry cannot be read or decoded.
func (r *credentialUsageRecorder) LastUsed(ctx context.Context, botID string) (lastUsed map[string]time.Time, available bool) {
	if r == nil || r.kv == nil {
		return nil, false
	}
	lastUsed = make(map[string]time.Time)
	entry, err := r.kv.Get(ctx, credentialUsageRuntimeStateKey(botID))
	if err != nil {
		if !isRuntimeStateKeyAbsent(err) {
			return nil, false
		}
	} else {
		var state corev1.CredentialUsageState
		if err := proto.Unmarshal(entry.Value(), &state); err != nil {
			return nil, false
		}
		for key, millis := range state.GetLastUsedUnixMillis() {
			if millis > 0 {
				lastUsed[key] = time.UnixMilli(millis).UTC()
			}
		}
	}
	r.mu.RLock()
	for key, observedAt := range r.observed[botID] {
		if observedAt.After(lastUsed[key]) {
			lastUsed[key] = observedAt
		}
	}
	r.mu.RUnlock()
	return lastUsed, true
}

// Run flushes due observations until ctx is cancelled. Storage failures are
// logged and retried because this optional telemetry must not affect core
// readiness or webhook requests.
func (r *credentialUsageRecorder) Run(ctx context.Context) error {
	if r == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	ticker := time.NewTicker(r.retryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.wake:
			r.flushDue(ctx, time.Now())
		case now := <-ticker.C:
			r.flushDue(ctx, now)
		}
	}
}

type credentialUsageFlush struct {
	botID         string
	credentialKey string
	usedAt        time.Time
	due           bool
}

func (r *credentialUsageRecorder) flushDue(ctx context.Context, now time.Time) {
	r.mu.RLock()
	flushes := make([]credentialUsageFlush, 0)
	for botID, credentials := range r.pending {
		for key, usedAt := range credentials {
			last := r.lastFlush[botID][key]
			flushes = append(flushes, credentialUsageFlush{
				botID: botID, credentialKey: key, usedAt: usedAt,
				due: last.IsZero() || now.Sub(last) >= r.flushInterval,
			})
		}
	}
	r.mu.RUnlock()
	for _, item := range flushes {
		if !r.credentialIsActive(item.botID, item.credentialKey) {
			r.forgetLocal(item.botID, item.credentialKey)
			if cleanupErr := r.deletePersisted(ctx, item.botID, item.credentialKey); cleanupErr != nil && ctx.Err() == nil && r.logger != nil {
				r.logger.Warn("Failed to remove revoked credential usage telemetry", "error", cleanupErr)
			}
			continue
		}
		if !item.due {
			continue
		}
		err := r.writeMax(ctx, item.botID, item.credentialKey, item.usedAt)
		credentialActive := r.credentialIsActive(item.botID, item.credentialKey)
		r.mu.Lock()
		observedAt, locallyTracked := r.observed[item.botID][item.credentialKey]
		if err == nil && locallyTracked && credentialActive {
			setNewestUsage(r.lastFlush, item.botID, item.credentialKey, now)
			if pendingAt := r.pending[item.botID][item.credentialKey]; !pendingAt.After(item.usedAt) {
				deleteCredentialUsage(r.pending, item.botID, item.credentialKey)
			}
			if !observedAt.After(item.usedAt) {
				deleteCredentialUsage(r.observed, item.botID, item.credentialKey)
			}
		}
		r.mu.Unlock()
		if !locallyTracked || !credentialActive {
			// Forget can run while writeMax is in flight. Remove a late or
			// ambiguously committed write after the local observation is gone.
			// The projected check also covers revocation on another replica.
			r.forgetLocal(item.botID, item.credentialKey)
			if cleanupErr := r.deletePersisted(ctx, item.botID, item.credentialKey); cleanupErr != nil && ctx.Err() == nil && r.logger != nil {
				r.logger.Warn("Failed to remove revoked credential usage telemetry", "error", cleanupErr)
			}
			continue
		}
		if err != nil && ctx.Err() == nil && r.logger != nil {
			r.logger.Warn("Failed to record credential usage telemetry", "error", err)
		}
	}
	r.sweepInactive(ctx, now)
}

func (r *credentialUsageRecorder) credentialIsActive(botID, credentialKey string) bool {
	return r.isActive == nil || r.isActive(botID, credentialKey)
}

// sweepInactive removes state left by a request that another replica revoked.
// It checks projected lifecycle state outside the recorder lock to keep the
// projection and recorder lock order acyclic.
func (r *credentialUsageRecorder) sweepInactive(ctx context.Context, now time.Time) {
	if r.isActive == nil {
		return
	}
	r.mu.Lock()
	if !r.lastSweep.IsZero() && now.Sub(r.lastSweep) < r.sweepInterval {
		r.mu.Unlock()
		return
	}
	r.lastSweep = now
	tracked := make(map[string]map[string]struct{})
	for botID, credentials := range r.observed {
		for key := range credentials {
			setCredentialUsageTracked(tracked, botID, key)
		}
	}
	for botID, credentials := range r.lastFlush {
		for key := range credentials {
			setCredentialUsageTracked(tracked, botID, key)
		}
	}
	r.mu.Unlock()

	for botID, credentials := range tracked {
		for key := range credentials {
			if r.credentialIsActive(botID, key) {
				continue
			}
			r.forgetLocal(botID, key)
			if err := r.deletePersisted(ctx, botID, key); err != nil && ctx.Err() == nil && r.logger != nil {
				r.logger.Warn("Failed to remove revoked credential usage telemetry", "error", err)
			}
		}
	}
}

func (r *credentialUsageRecorder) forgetLocal(botID, credentialKey string) {
	r.mu.Lock()
	deleteCredentialUsage(r.pending, botID, credentialKey)
	deleteCredentialUsage(r.observed, botID, credentialKey)
	deleteCredentialUsage(r.lastFlush, botID, credentialKey)
	r.mu.Unlock()
}

func (r *credentialUsageRecorder) writeMax(ctx context.Context, botID, credentialKey string, usedAt time.Time) error {
	key := credentialUsageRuntimeStateKey(botID)
	for attempt := 0; attempt < 10; attempt++ {
		entry, err := r.kv.Get(ctx, key)
		state := &corev1.CredentialUsageState{LastUsedUnixMillis: make(map[string]int64)}
		if err != nil {
			if !isRuntimeStateKeyAbsent(err) {
				return err
			}
		} else if err := proto.Unmarshal(entry.Value(), state); err != nil {
			return fmt.Errorf("decode credential usage state: %w", err)
		}
		if state.LastUsedUnixMillis == nil {
			state.LastUsedUnixMillis = make(map[string]int64)
		}
		millis := usedAt.UnixMilli()
		if state.GetLastUsedUnixMillis()[credentialKey] >= millis {
			return nil
		}
		state.LastUsedUnixMillis[credentialKey] = millis
		value, err := proto.Marshal(state)
		if err != nil {
			return fmt.Errorf("encode credential usage state: %w", err)
		}
		if entry == nil {
			_, err = r.kv.Create(ctx, key, value)
		} else {
			_, err = r.kv.Update(ctx, key, value, entry.Revision())
		}
		if err == nil {
			return nil
		}
		if !isRuntimeStateRevisionConflict(err) {
			return err
		}
	}
	return fmt.Errorf("credential usage OCC retry exhausted")
}

func (r *credentialUsageRecorder) deletePersisted(ctx context.Context, botID, credentialKey string) error {
	key := credentialUsageRuntimeStateKey(botID)
	for attempt := 0; attempt < 10; attempt++ {
		entry, err := r.kv.Get(ctx, key)
		if err != nil {
			if isRuntimeStateKeyAbsent(err) {
				return nil
			}
			return err
		}
		var state corev1.CredentialUsageState
		if err := proto.Unmarshal(entry.Value(), &state); err != nil {
			return fmt.Errorf("decode credential usage state: %w", err)
		}
		if _, exists := state.GetLastUsedUnixMillis()[credentialKey]; !exists {
			return nil
		}
		delete(state.LastUsedUnixMillis, credentialKey)
		if len(state.LastUsedUnixMillis) == 0 {
			err = r.kv.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
		} else {
			var value []byte
			value, err = proto.Marshal(&state)
			if err == nil {
				_, err = r.kv.Update(ctx, key, value, entry.Revision())
			}
		}
		if err == nil || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if !isRuntimeStateRevisionConflict(err) {
			return err
		}
	}
	return fmt.Errorf("credential usage cleanup OCC retry exhausted")
}

func setNewestUsage(target map[string]map[string]time.Time, botID, key string, usedAt time.Time) {
	if target[botID] == nil {
		target[botID] = make(map[string]time.Time)
	}
	if usedAt.After(target[botID][key]) {
		target[botID][key] = usedAt
	}
}

func deleteCredentialUsage(target map[string]map[string]time.Time, botID, key string) {
	delete(target[botID], key)
	if len(target[botID]) == 0 {
		delete(target, botID)
	}
}

func setCredentialUsageTracked(target map[string]map[string]struct{}, botID, key string) {
	if target[botID] == nil {
		target[botID] = make(map[string]struct{})
	}
	target[botID][key] = struct{}{}
}
