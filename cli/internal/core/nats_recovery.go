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
)

// runNATSRecovery turns a NATS transport reconnect into an application-level
// recovery boundary. Core NATS subscriptions and JetStream consumers recover
// through the SDK, but realtime clients must reconnect across the unobservable
// gap and serving readiness must wait for local projections to catch up.
func (c *ChattoCore) runNATSRecovery(ctx context.Context, statuses <-chan nats.Status) error {
	recovering := false
	for {
		select {
		case <-ctx.Done():
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
				if !recovering {
					c.logger.Warn("NATS continuity lost; suspending readiness and realtime delivery")
				}
				recovering = true
				c.suspendForNATSRecovery()
			case nats.CONNECTED:
				if !recovering {
					continue
				}
				if err := c.recoverNATSGeneration(ctx); err != nil {
					return err
				}
				recovering = false
			case nats.CLOSED:
				c.suspendForNATSRecovery()
				return fmt.Errorf("NATS connection permanently closed")
			}
		}
	}
}

func (c *ChattoCore) suspendForNATSRecovery() {
	c.natsReady.Store(false)
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
			if c.myEventsModel != nil && c.myEventsModel.hub != nil {
				c.myEventsModel.hub.beginGeneration()
			}
			c.natsReady.Store(true)
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
	return nil
}

func (c *ChattoCore) verifyNATSRecovery(ctx context.Context) error {
	if _, err := c.storage.runtimeStateKV.Status(ctx); err != nil {
		return fmt.Errorf("RUNTIME_STATE recovery: %w", err)
	}
	if _, err := c.storage.serverEvtStream.Info(ctx); err != nil {
		return fmt.Errorf("EVT recovery: %w", err)
	}
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return fmt.Errorf("projection recovery: %w", err)
	}
	if err := c.readStateModel.WaitReady(ctx); err != nil {
		return fmt.Errorf("read state recovery: %w", err)
	}
	return nil
}
