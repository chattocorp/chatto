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
	authProjector  *events.Projector

	contentKeys          *ContentKeyProjection
	contentKeysProjector *events.Projector
}

func newUserModel(
	publisher *events.Publisher,
	users *UserProjection,
	usersProjector *events.Projector,
	authProjector *events.Projector,
	contentKeys *ContentKeyProjection,
	contentKeysProjector *events.Projector,
) *UserModel {
	return &UserModel{
		publisher:            publisher,
		users:                users,
		usersProjector:       usersProjector,
		authProjector:        authProjector,
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
	if err := waitForProjectionSubjectsCurrent(ctx, m.publisher, name, m.usersProjector, subjects...); err != nil {
		return err
	}
	return m.waitForUserAuthCurrent(ctx, name)
}

func (m *UserModel) waitForUserAuth(ctx context.Context, pos events.StreamPosition) error {
	if m.authProjector == nil {
		return nil
	}
	return waitForPositionAll(ctx, pos, waitForProjection("user auth", m.authProjector))
}

func (m *UserModel) waitForUserAuthCurrent(ctx context.Context, name string) error {
	if m.publisher == nil || m.authProjector == nil || m.users == nil || m.users.AuthProjection() == nil {
		return nil
	}
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, name+" auth", m.authProjector, m.users.AuthProjection().Subjects()...)
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
