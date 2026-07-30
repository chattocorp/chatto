package core

import (
	"context"
	"errors"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var errContentKeyProjectionUnavailable = errors.New("content key projection is unavailable")

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

// activeContentKey returns the newest projected DEK for a purpose. The
// projection preserves compatibility with legacy purpose-unspecified DEKs.
func (m *UserModel) activeContentKey(userID string, purpose corev1.UserDEKPurpose) (*corev1.UserDEKGeneratedEvent, bool, error) {
	if m.contentKeys == nil {
		return nil, false, errContentKeyProjectionUnavailable
	}
	event, ok := m.contentKeys.Active(userID, purpose)
	return event, ok, nil
}

// contentKeyAtEpoch returns a projected DEK at an exact epoch. The projection
// preserves compatibility with legacy purpose-unspecified DEKs.
func (m *UserModel) contentKeyAtEpoch(userID string, purpose corev1.UserDEKPurpose, epoch int32) (*corev1.UserDEKGeneratedEvent, bool, error) {
	if m.contentKeys == nil {
		return nil, false, errContentKeyProjectionUnavailable
	}
	event, ok := m.contentKeys.Get(userID, purpose, epoch)
	return event, ok, nil
}

// keyRefsForShredding returns the stored content-key and wrapping-key
// references associated with a user. Callers still inspect stored DEK records
// before shredding because their wrapping-key reference may be newer than EVT.
func (m *UserModel) keyRefsForShredding(userID string) (contentKeyRefs, wrappingKeyRefs []string, err error) {
	if m.contentKeys == nil {
		return nil, nil, errContentKeyProjectionUnavailable
	}
	return m.contentKeys.ContentKeyRefs(userID), m.contentKeys.KeyRefs(userID), nil
}
