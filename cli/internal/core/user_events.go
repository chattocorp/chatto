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
		authorizationSeq := uint64(0)
		if isAuthorizationInputUserEvent(event) {
			var err error
			authorizationSeq, err = c.authorizationFenceSeq(ctx)
			if err != nil {
				return 0, fmt.Errorf("read authorization fence seq: %w", err)
			}
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read user OCC filter seq: %w", err)
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for user projection: %w", err)
		}
		if err := c.userModel.waitForUserAuthCurrent(ctx, "user mutation"); err != nil {
			return 0, fmt.Errorf("wait for user auth projection: %w", err)
		}
		if check != nil {
			if err := check(); err != nil {
				return 0, err
			}
		}

		var seq uint64
		if isAuthorizationInputUserEvent(event) {
			entries := []events.BatchEntry{{
				Subject:       subject,
				Event:         event,
				HasOCC:        true,
				ExpectedSeq:   filterSeq,
				FilterSubject: filter,
			}}
			seqs, appendErr := c.appendAuthorizationFencedBatch(ctx, event.GetActorId(), entries, authorizationSeq)
			err = appendErr
			if appendErr == nil {
				seq = seqs[0]
			}
		} else {
			seq, err = c.EventPublisher.AppendAtFilter(ctx, subject, event, filter, filterSeq)
		}
		if err == nil {
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(subject, seq)); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			if isUserAuthEvent(event) {
				if err := c.userModel.waitForUserAuth(ctx, events.SubjectPosition(subject, seq)); err != nil {
					return 0, fmt.Errorf("wait for user auth projection: %w", err)
				}
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
		authorizationSeq := uint64(0)
		fencesAuthorization := containsAuthorizationInputUserEvent(entries)
		if fencesAuthorization {
			var err error
			authorizationSeq, err = c.authorizationFenceSeq(ctx)
			if err != nil {
				return 0, fmt.Errorf("read authorization fence seq: %w", err)
			}
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read user OCC filter seq: %w", err)
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for user projection: %w", err)
		}
		if err := c.userModel.waitForUserAuthCurrent(ctx, "user mutation"); err != nil {
			return 0, fmt.Errorf("wait for user auth projection: %w", err)
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

		var seqs []uint64
		if fencesAuthorization {
			seqs, err = c.appendAuthorizationFencedBatch(ctx, chunk[0].Event.GetActorId(), chunk, authorizationSeq)
		} else {
			seqs, err = c.EventPublisher.AppendBatch(ctx, chunk)
		}
		if err == nil {
			lastDomainIndex := len(chunk) - 1
			lastSeq := seqs[lastDomainIndex]
			lastSubject := chunk[lastDomainIndex].Subject
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			for i := len(chunk) - 1; i >= 0; i-- {
				if isUserAuthEvent(chunk[i].Event) {
					if err := c.userModel.waitForUserAuth(ctx, events.SubjectPosition(chunk[i].Subject, seqs[i])); err != nil {
						return 0, fmt.Errorf("wait for user auth projection: %w", err)
					}
					break
				}
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

func isUserAuthEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_UserAccountCreated,
		*corev1.Event_UserPasswordHashChanged,
		*corev1.Event_UserOidcSubjectLinked,
		*corev1.Event_UserExternalIdentityLinked,
		*corev1.Event_UserExternalIdentityUnlinked,
		*corev1.Event_OauthConsentGranted,
		*corev1.Event_UserAccountDeleted,
		*corev1.Event_UserKeyShredded:
		return true
	default:
		return false
	}
}

func isAuthorizationInputUserEvent(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_UserAccountCreated,
		*corev1.Event_UserVerifiedEmailAdded,
		*corev1.Event_UserAccountDeleted:
		return true
	default:
		return false
	}
}

func containsAuthorizationInputUserEvent(entries []events.BatchEntry) bool {
	for _, entry := range entries {
		if isAuthorizationInputUserEvent(entry.Event) {
			return true
		}
	}
	return false
}

func (c *ChattoCore) appendUserBatchWithMentionableCheck(ctx context.Context, userID string, entries []events.BatchEntry, check func() error) (uint64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	filter := events.EventSubjectFilter()

	for attempt := 0; attempt < maxUserMutationRetries; attempt++ {
		authorizationSeq := uint64(0)
		fencesAuthorization := containsAuthorizationInputUserEvent(entries)
		if fencesAuthorization {
			var err error
			authorizationSeq, err = c.authorizationFenceSeq(ctx)
			if err != nil {
				return 0, fmt.Errorf("read authorization fence seq: %w", err)
			}
		}
		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return 0, fmt.Errorf("read mentionable OCC filter seq: %w", err)
		}
		if err := c.mentionables.waitFor(ctx, events.SubjectPosition(filter, filterSeq)); err != nil {
			return 0, fmt.Errorf("wait for mentionables projection: %w", err)
		}
		if err := c.userModel.waitForUserAuthCurrent(ctx, "user mutation"); err != nil {
			return 0, fmt.Errorf("wait for user auth projection: %w", err)
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

		var seqs []uint64
		if fencesAuthorization {
			seqs, err = c.appendAuthorizationFencedBatch(ctx, chunk[0].Event.GetActorId(), chunk, authorizationSeq)
		} else {
			seqs, err = c.EventPublisher.AppendBatch(ctx, chunk)
		}
		if err == nil {
			lastDomainIndex := len(chunk) - 1
			lastSeq := seqs[lastDomainIndex]
			lastSubject := chunk[lastDomainIndex].Subject
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
				return 0, fmt.Errorf("wait for user projection: %w", err)
			}
			if err := c.mentionables.waitFor(ctx, events.SubjectPosition(lastSubject, lastSeq)); err != nil {
				return 0, fmt.Errorf("wait for mentionables projection: %w", err)
			}
			for i := len(chunk) - 1; i >= 0; i-- {
				if isUserAuthEvent(chunk[i].Event) {
					if err := c.userModel.waitForUserAuth(ctx, events.SubjectPosition(chunk[i].Subject, seqs[i])); err != nil {
						return 0, fmt.Errorf("wait for user auth projection: %w", err)
					}
					break
				}
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
