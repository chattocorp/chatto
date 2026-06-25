package core

import (
	"context"

	"hmans.de/chatto/internal/events"
)

// UserModel owns user-derived projections and their readiness barriers.
type UserModel struct {
	publisher *events.Publisher

	users          *UserProjection
	usersProjector *events.Projector

	contentKeys          *ContentKeyProjection
	contentKeysProjector *events.Projector
}

func newUserModel(
	publisher *events.Publisher,
	users *UserProjection,
	usersProjector *events.Projector,
	contentKeys *ContentKeyProjection,
	contentKeysProjector *events.Projector,
) *UserModel {
	return &UserModel{
		publisher:            publisher,
		users:                users,
		usersProjector:       usersProjector,
		contentKeys:          contentKeys,
		contentKeysProjector: contentKeysProjector,
	}
}

func (m *UserModel) waitForUsers(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("users", m.usersProjector))
}

func (m *UserModel) waitForContentKeys(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("content key", m.contentKeysProjector))
}

func (m *UserModel) waitForUsersCurrent(ctx context.Context, name string, subjects ...string) error {
	if m.publisher == nil || m.usersProjector == nil {
		return nil
	}
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, name, m.usersProjector, subjects...)
}

func (m *UserModel) waitForContentKeysCurrent(ctx context.Context, userID string) error {
	if m.publisher == nil || m.contentKeysProjector == nil {
		return nil
	}
	agg := events.UserAggregate(userID)
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, "content key", m.contentKeysProjector,
		agg.Subject(events.EventUserDEKGenerated),
		agg.Subject(events.EventUserKeyShredded),
	)
}
