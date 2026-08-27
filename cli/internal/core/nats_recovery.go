package core

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	natsRecoveryAttemptTimeout = 5 * time.Second
	natsRecoveryRetryWait      = 250 * time.Millisecond
	natsRecoveryPollInterval   = 250 * time.Millisecond
	natsRecoveryLivenessLimit  = 5 * time.Minute
)

const (
	natsRecoveryStarting int32 = iota
	natsRecoveryReady
	natsRecoveryInProgress
)

type natsRecoveryResult struct {
	generation uint64
	err        error
}

// runNATSRecovery turns a NATS transport reconnect into an application-level
// recovery boundary. Core NATS subscriptions and JetStream consumers recover
// through the SDK, but realtime clients must reconnect across the unobservable
// gap and serving readiness must wait for local projections to catch up.
func (c *ChattoCore) runNATSRecovery(ctx context.Context, statuses <-chan nats.Status) error {
	ticker := time.NewTicker(natsRecoveryPollInterval)
	defer ticker.Stop()

	results := make(chan natsRecoveryResult, 1)
	continuityLost := false
	var generation uint64
	var recoveryCancel context.CancelFunc

	suspend := func() {
		if !continuityLost || recoveryCancel != nil {
			generation++
		}
		continuityLost = true
		if recoveryCancel != nil {
			recoveryCancel()
			recoveryCancel = nil
		}
		c.suspendForNATSRecovery()
	}
	startRecovery := func() {
		if !continuityLost || recoveryCancel != nil || !c.nc.IsConnected() {
			return
		}
		recoveryCtx, cancel := context.WithCancel(ctx)
		recoveryCancel = cancel
		workerGeneration := generation
		go func() {
			err := c.recoverNATSGeneration(recoveryCtx)
			select {
			case results <- natsRecoveryResult{generation: workerGeneration, err: err}:
			case <-ctx.Done():
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			if recoveryCancel != nil {
				recoveryCancel()
			}
			return ctx.Err()
		case status, ok := <-statuses:
			if !ok {
				if err := ctx.Err(); err != nil {
					return err
				}
				return fmt.Errorf("NATS connection status listener stopped")
			}
			switch status {
			case nats.DISCONNECTED, nats.RECONNECTING:
				if !continuityLost {
					c.logger.Warn("NATS continuity lost; suspending readiness and realtime delivery")
				}
				suspend()
			case nats.CONNECTED:
				startRecovery()
			case nats.CLOSED:
				suspend()
				return fmt.Errorf("NATS connection permanently closed")
			}
		case result := <-results:
			if result.generation != generation {
				continue
			}
			if recoveryCancel != nil {
				recoveryCancel()
			}
			recoveryCancel = nil
			if result.err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				startRecovery()
				continue
			}
			if !c.nc.IsConnected() {
				suspend()
				continue
			}
			if c.myEventsModel != nil && c.myEventsModel.hub != nil {
				c.myEventsModel.hub.beginGeneration()
			}
			c.natsRecoveredReconnects.Store(c.nc.Stats().Reconnects)
			c.natsRecoveryState.Store(natsRecoveryReady)
			c.natsRecoveryStartedAt.Store(0)
			continuityLost = false
		case <-ticker.C:
			switch {
			case c.nc.IsClosed():
				suspend()
				return fmt.Errorf("NATS connection permanently closed")
			case !c.nc.IsConnected():
				if !continuityLost || recoveryCancel != nil {
					suspend()
				}
			case c.natsRecoveredReconnects.Load() != c.nc.Stats().Reconnects:
				// Status callbacks are advisory and may be coalesced during a
				// fast local restart. The reconnect counter is monotonic, so it
				// also fences readiness when the transition notifications race.
				if !continuityLost {
					c.logger.Warn("NATS reconnect detected; suspending readiness and realtime delivery")
					suspend()
				}
				startRecovery()
			case continuityLost:
				startRecovery()
			}
		}
	}
}

func (c *ChattoCore) suspendForNATSRecovery() {
	previous := c.natsRecoveryState.Swap(natsRecoveryInProgress)
	if previous != natsRecoveryInProgress {
		c.natsRecoveryStartedAt.CompareAndSwap(0, time.Now().UnixNano())
	}
	if c.myEventsModel != nil && c.myEventsModel.hub != nil {
		c.myEventsModel.hub.quarantine("NATS connection interrupted")
	}
}

func (c *ChattoCore) recoverNATSGeneration(ctx context.Context) error {
	startedAt := time.Now()
	attempt := 0
	for c.nc.IsConnected() {
		attempt++
		attemptCtx, cancel := context.WithTimeout(ctx, natsRecoveryAttemptTimeout)
		err := c.verifyNATSRecovery(attemptCtx)
		cancel()
		if err == nil {
			c.logger.Info("NATS recovery complete", "duration", time.Since(startedAt), "attempts", attempt)
			return nil
		}
		if attempt == 1 || attempt%10 == 0 {
			c.logger.Warn("NATS connected but application recovery is incomplete", "attempt", attempt, "error", err)
		}
		timer := time.NewTimer(natsRecoveryRetryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return context.Canceled
}

func (c *ChattoCore) verifyNATSRecovery(ctx context.Context) error {
	if _, err := c.storage.runtimeStateKV.Status(ctx); err != nil {
		return fmt.Errorf("RUNTIME_STATE recovery: %w", err)
	}
	if _, err := c.storage.serverEvtStream.Info(ctx); err != nil {
		return fmt.Errorf("EVT recovery: %w", err)
	}
	if _, err := c.js.CreateOrUpdateKeyValue(ctx, memoryCacheConfig(c.config)); err != nil {
		return fmt.Errorf("MEMORY_CACHE recovery: %w", err)
	}
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("projection recovery: %w", err)
	}
	if err := c.readStateModel.Resync(ctx); err != nil {
		return fmt.Errorf("read state recovery: %w", err)
	}
	if err := c.notificationOccurrences.Resync(ctx); err != nil {
		return fmt.Errorf("notification occurrence recovery: %w", err)
	}
	if err := c.presenceModel.Resync(ctx); err != nil {
		return fmt.Errorf("presence recovery: %w", err)
	}
	return nil
}

// NATSRecoveryLivenessError reports recovery that has exceeded the grace
// period in which keeping this process alive is more useful than replacing it.
func (c *ChattoCore) NATSRecoveryLivenessError(now time.Time) error {
	startedAt := c.natsRecoveryStartedAt.Load()
	if startedAt == 0 || now.Sub(time.Unix(0, startedAt)) < natsRecoveryLivenessLimit {
		return nil
	}
	return fmt.Errorf("NATS recovery exceeded %s", natsRecoveryLivenessLimit)
}
