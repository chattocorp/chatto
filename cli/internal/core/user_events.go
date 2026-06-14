package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const maxUserMutationRetries = 5

func (c *ChattoCore) appendUserEvent(ctx context.Context, userID string, event *corev1.Event, filter string, check func() error) (uint64, error) {
	if filter == "" {
		filter = events.UserAggregate(userID).AllEventsFilter()
	}
	subject := events.UserAggregate(userID).SubjectFor(event)

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read user OCC filter seq: %w", err)
		}
		if err := c.userService.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for user projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}

		seq, err := c.EventPublisher.AppendAtFilter(ctx, subject, event, filter, filterSeq)
		if err == nil {
			if err := c.userService.waitForUsers(ctx, events.SubjectPosition(subject, seq)); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			return seq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("user OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) appendUserBatch(ctx context.Context, userID string, entries []events.BatchEntry, filter string, check func() error) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	if filter == "" {
		filter = events.UserAggregate(userID).AllEventsFilter()
	}

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read user OCC filter seq: %w", err)
		}
		if err := c.userService.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for user projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}

		chunk := append([]events.BatchEntry(nil), entries...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = filterSeq
		chunk[0].FilterSubject = filter

		seqs, err := c.EventPublisher.AppendBatch(ctx, chunk)
		if err == nil {
			lastSeq := seqs[len(seqs)-1]
			lastSubject := chunk[len(chunk)-1].Subject
			if err := c.userService.waitForUsers(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			return lastSeq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("user batch OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

func (c *ChattoCore) appendUserBatchWithMentionableCheck(ctx context.Context, userID string, entries []events.BatchEntry, check func() error) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	filter := events.EventSubjectFilter()

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read mentionable OCC filter seq: %w", err)
		}
		if err := c.mentionables.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for mentionables projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}

		chunk := append([]events.BatchEntry(nil), entries...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = filterSeq
		chunk[0].FilterSubject = filter

		seqs, err := c.EventPublisher.AppendBatch(ctx, chunk)
		if err == nil {
			lastSeq := seqs[len(seqs)-1]
			lastSubject := chunk[len(chunk)-1].Subject
			if err := c.userService.waitForUsers(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			if err := c.mentionables.waitFor(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
				return 0, fmt.Errorf("wait for mentionables projection: %w", err)
			}
			return lastSeq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(mentionableRetryDelay(attempt)):
		}
	}
	return 0, fmt.Errorf("mentionable user batch OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}
