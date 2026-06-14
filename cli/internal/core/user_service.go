package core

import (
	"context"

	"hmans.de/chatto/internal/events"
)

// UserService owns user-derived projections and their readiness barriers.
type UserService struct {
	publisher *events.Publisher

	users          *UserProjection
	usersProjector *events.Projector

	contentKeys          *ContentKeyProjection
	contentKeysProjector *events.Projector
}

func newUserService(
	publisher *events.Publisher,
	users *UserProjection,
	usersProjector *events.Projector,
	contentKeys *ContentKeyProjection,
	contentKeysProjector *events.Projector,
) *UserService {
	return &UserService{
		publisher:            publisher,
		users:                users,
		usersProjector:       usersProjector,
		contentKeys:          contentKeys,
		contentKeysProjector: contentKeysProjector,
	}
}

func (m *UserService) waitForUsers(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("users", m.usersProjector))
}

func (m *UserService) waitForContentKeys(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("content key", m.contentKeysProjector))
}

func (m *UserService) waitForUsersCurrent(ctx context.Context, name string, subjects ...string) error {
	if m.publisher == nil || m.usersProjector == nil {
		return nil
	}
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, name, m.usersProjector, subjects...)
}

func (m *UserService) waitForContentKeysCurrent(ctx context.Context, userID string) error {
	if m.publisher == nil || m.contentKeysProjector == nil {
		return nil
	}
	agg := events.UserAggregate(userID)
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, "content key", m.contentKeysProjector,
		agg.Subject(events.EventUserDEKGenerated),
		agg.Subject(events.EventUserKeyShredded),
	)
}
