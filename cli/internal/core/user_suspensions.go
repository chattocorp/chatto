package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const MaxUserSuspensionReasonLength = 1000

// SuspendUser records a server-level user suspension. The caller is
// responsible for permission and rank checks.
func (c *ChattoCore) SuspendUser(ctx context.Context, actorID, targetUserID, reason string, expiresAt *time.Time) (*UserSuspension, error) {
	if actorID == targetUserID {
		return nil, ErrPermissionDenied
	}
	if _, err := c.GetUser(ctx, targetUserID); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("suspension reason is required")
	}
	if len([]rune(reason)) > MaxUserSuspensionReasonLength {
		return nil, fmt.Errorf("suspension reason exceeds %d characters", MaxUserSuspensionReasonLength)
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return nil, fmt.Errorf("suspension expiry must be in the future")
	}

	payload := &corev1.UserSuspendedEvent{
		UserId: targetUserID,
		Reason: reason,
	}
	if expiresAt != nil {
		payload.ExpiresAt = timestamppb.New(*expiresAt)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserSuspended{
		UserSuspended: payload,
	}})
	subject := events.UserAggregate(targetUserID).SubjectFor(event)
	seq, err := c.appendUserEvent(ctx, targetUserID, event, "", nil)
	if err != nil {
		return nil, fmt.Errorf("publish UserSuspendedEvent: %w", err)
	}
	if err := waitForPositionAll(ctx, events.SubjectPosition(subject, seq), waitForProjection("user suspensions", c.UserSuspensionsProjector)); err != nil {
		return nil, err
	}
	suspension, ok := c.UserSuspensions.ActiveSuspension(targetUserID, time.Now())
	if !ok {
		return nil, fmt.Errorf("suspension projection did not contain newly published suspension")
	}
	if err := c.PublishUserSuspensionChanged(ctx, targetUserID, &suspension); err != nil {
		c.logger.Warn("Failed to publish user suspension change", "user_id", targetUserID, "error", err)
	}
	return &suspension, nil
}

// UnsuspendUser clears an active server-level user suspension. It is
// idempotent when no active suspension exists.
func (c *ChattoCore) UnsuspendUser(ctx context.Context, actorID, targetUserID, reason string) error {
	if _, err := c.GetUser(ctx, targetUserID); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("unsuspend reason is required")
	}
	if len([]rune(reason)) > MaxUserSuspensionReasonLength {
		return fmt.Errorf("unsuspend reason exceeds %d characters", MaxUserSuspensionReasonLength)
	}
	if _, ok := c.UserSuspensions.ActiveSuspension(targetUserID, time.Now()); !ok {
		return nil
	}

	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserUnsuspended{
		UserUnsuspended: &corev1.UserUnsuspendedEvent{
			UserId: targetUserID,
			Reason: reason,
		},
	}})
	subject := events.UserAggregate(targetUserID).SubjectFor(event)
	seq, err := c.appendUserEvent(ctx, targetUserID, event, "", nil)
	if err != nil {
		return fmt.Errorf("publish UserUnsuspendedEvent: %w", err)
	}
	if err := waitForPositionAll(ctx, events.SubjectPosition(subject, seq), waitForProjection("user suspensions", c.UserSuspensionsProjector)); err != nil {
		return err
	}
	if err := c.PublishUserSuspensionChanged(ctx, targetUserID, nil); err != nil {
		c.logger.Warn("Failed to publish user suspension change", "user_id", targetUserID, "error", err)
	}
	return nil
}

func (c *ChattoCore) ListActiveUserSuspensions(_ context.Context) ([]UserSuspension, error) {
	return c.UserSuspensions.ActiveSuspensions(time.Now()), nil
}

func (c *ChattoCore) ActiveUserSuspension(_ context.Context, userID string) (UserSuspension, bool) {
	return c.UserSuspensions.ActiveSuspension(userID, time.Now())
}

func (c *ChattoCore) PublishUserSuspensionChanged(ctx context.Context, userID string, suspension *UserSuspension) error {
	payload := &corev1.UserSuspensionChangedEvent{Suspended: suspension != nil}
	if suspension != nil && suspension.ExpiresAt != nil {
		payload.ExpiresAt = timestamppb.New(*suspension.ExpiresAt)
	}
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserSuspensionChanged{
			UserSuspensionChanged: payload,
		},
	})
	return c.publishLiveEvent(ctx, subjects.LiveSyncUserEvent(userID, "suspension_changed"), event)
}
