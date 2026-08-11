package core

import (
	"context"
	"errors"
	"sort"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

var errContentKeyProjectionUnavailable = errors.New("content key projection is unavailable")

// UserModel owns user-derived projection reads and readiness barriers.
type UserModel struct {
	publisher *evtstream.Publisher

	users       events.ProjectionHandle[*UserProjection]
	auth        events.ProjectionHandle[*UserAuthProjection]
	contentKeys events.ProjectionHandle[*ContentKeyProjection]
}

func newUserModel(
	publisher *evtstream.Publisher,
	users events.ProjectionHandle[*UserProjection],
	auth events.ProjectionHandle[*UserAuthProjection],
	contentKeys events.ProjectionHandle[*ContentKeyProjection],
) *UserModel {
	return &UserModel{
		publisher:   publisher,
		users:       users,
		auth:        auth,
		contentKeys: contentKeys,
	}
}

func (m *UserModel) waitForUsers(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("users", m.users.Projector()))
}

func (m *UserModel) waitForContentKeys(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("content key", m.contentKeys.Projector()))
}

func (m *UserModel) waitForUsersCurrent(ctx context.Context, name string, subjects ...string) error {
	if m.publisher == nil || m.users.Projector() == nil {
		return nil
	}
	if err := waitForProjectionSubjectsCurrent(ctx, m.publisher, name, m.users.Projector(), subjects...); err != nil {
		return err
	}
	return m.waitForUserAuthCurrent(ctx, name)
}

func (m *UserModel) waitForUserAuth(ctx context.Context, pos events.StreamPosition) error {
	if m.auth.Projector() == nil {
		return nil
	}
	return waitForPositionAll(ctx, pos, waitForProjection("user auth", m.auth.Projector()))
}

func (m *UserModel) waitForUserAuthCurrent(ctx context.Context, name string) error {
	if m.publisher == nil || m.auth.Projector() == nil || m.auth.Projection() == nil {
		return nil
	}
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, name+" auth", m.auth.Projector(), m.auth.Projection().Subjects()...)
}

func (m *UserModel) waitForContentKeysCurrent(ctx context.Context, userID string) error {
	if m.publisher == nil || m.contentKeys.Projector() == nil {
		return nil
	}
	agg := evtstream.UserAggregate(userID)
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, "content key", m.contentKeys.Projector(),
		agg.Subject(evtstream.EventUserDEKGenerated),
		agg.Subject(evtstream.EventUserKeyShreddingRequested),
		agg.Subject(evtstream.EventUserKeyShredded),
	)
}

func (m *UserModel) keyShreddingRequested(userID string) bool {
	return m.users.Projection() != nil && m.users.Projection().KeyShreddingRequested(userID)
}

// activeContentKey returns the newest projected DEK for a purpose. The
// projection preserves compatibility with legacy purpose-unspecified DEKs.
func (m *UserModel) activeContentKey(userID string, purpose corev1.UserDEKPurpose) (*corev1.UserDEKGeneratedEvent, bool, error) {
	if m.contentKeys.Projection() == nil {
		return nil, false, errContentKeyProjectionUnavailable
	}
	event, ok := m.contentKeys.Projection().Active(userID, purpose)
	return event, ok, nil
}

// contentKeyAtEpoch returns a projected DEK at an exact epoch. The projection
// preserves compatibility with legacy purpose-unspecified DEKs.
func (m *UserModel) contentKeyAtEpoch(userID string, purpose corev1.UserDEKPurpose, epoch int32) (*corev1.UserDEKGeneratedEvent, bool, error) {
	if m.contentKeys.Projection() == nil {
		return nil, false, errContentKeyProjectionUnavailable
	}
	event, ok := m.contentKeys.Projection().Get(userID, purpose, epoch)
	return event, ok, nil
}

func (m *UserModel) user(ctx context.Context, userID string) (*corev1.User, bool, error) {
	return m.users.Projection().GetContext(ctx, userID)
}

func (m *UserModel) userReference(ctx context.Context, userID string) (*corev1.User, bool, error) {
	return m.users.Projection().GetReferenceContext(ctx, userID)
}

func (m *UserModel) userReferences(ctx context.Context, userIDs []string) ([]*corev1.User, error) {
	return m.users.Projection().GetReferencesContext(ctx, userIDs)
}

func (m *UserModel) userByLogin(ctx context.Context, login string) (*corev1.User, bool, error) {
	return m.users.Projection().GetByLoginContext(ctx, login)
}

func (m *UserModel) userByEmail(ctx context.Context, email string) (*corev1.User, bool, error) {
	return m.users.Projection().GetByEmailContext(ctx, email)
}

func (m *UserModel) userByExternalIdentity(ctx context.Context, issuer, subject string) (*corev1.User, bool, error) {
	userID, ok := m.auth.Projection().ExternalIdentityOwnerID(issuer, subject)
	if !ok {
		return nil, false, nil
	}
	return m.users.Projection().GetContext(ctx, userID)
}

func (m *UserModel) loginExists(login string) bool {
	return m.users.Projection().LoginExists(login)
}

func (m *UserModel) emailClaimed(email string) bool {
	return m.users.Projection().EmailClaimed(email)
}

func (m *UserModel) emailOwnerID(email string) (string, bool) {
	return m.users.Projection().EmailOwnerID(email)
}

func (m *UserModel) externalIdentityOwnerID(issuer, subject string) (string, bool) {
	return m.auth.Projection().ExternalIdentityOwnerID(issuer, subject)
}

func (m *UserModel) externalIdentities(userID string) []ExternalIdentity {
	return m.auth.Projection().ExternalIdentities(userID)
}

func (m *UserModel) passwordHash(userID string) ([]byte, bool) {
	hash, _, ok := m.auth.Projection().PasswordHashWithSetAt(userID)
	return hash, ok
}

func (m *UserModel) passwordHashWithSetAt(userID string) ([]byte, time.Time, bool) {
	return m.auth.Projection().PasswordHashWithSetAt(userID)
}

func (m *UserModel) authGeneration(userID string) (uint64, bool) {
	return m.auth.Projection().AuthGeneration(userID)
}

func (m *UserModel) avatar(userID string) (*corev1.AssetRecord, bool) {
	return m.users.Projection().Avatar(userID)
}

func (m *UserModel) isPublicAvatarAsset(assetID string) bool {
	if m == nil || m.users.Projection() == nil {
		return false
	}
	return m.users.Projection().IsPublicAvatarAsset(assetID)
}

func (m *UserModel) verifiedEmails(ctx context.Context, userID string) ([]VerifiedEmail, error) {
	return m.users.Projection().VerifiedEmailsContext(ctx, userID)
}

func (m *UserModel) hasVerifiedEmail(userID string) bool {
	return m.users.Projection().HasVerifiedEmail(userID)
}

func (m *UserModel) hasVerifiedFactor(userID string) bool {
	return m.users.Projection().HasVerifiedEmail(userID) || m.auth.Projection().HasExternalIdentity(userID)
}

func (m *UserModel) hasOAuthConsent(userID, redirectOrigin string) bool {
	return m.auth.Projection().HasOAuthConsent(userID, redirectOrigin)
}

func (m *UserModel) loginChangedAt(userID string) time.Time {
	return m.users.Projection().LoginChangedAt(userID)
}

func (m *UserModel) allUsers(ctx context.Context) ([]*corev1.User, error) {
	return m.users.Projection().UsersContext(ctx)
}

func (m *UserModel) verifiedUserIDs() []string {
	return m.users.Projection().VerifiedUserIDs()
}

func (m *UserModel) verifiedAccountIDs() []string {
	seen := make(map[string]struct{})
	for _, userID := range m.users.Projection().VerifiedUserIDs() {
		seen[userID] = struct{}{}
	}
	for _, userID := range m.auth.Projection().VerifiedAccountIDs() {
		seen[userID] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for userID := range seen {
		out = append(out, userID)
	}
	sort.Strings(out)
	return out
}

func (m *UserModel) userCount() int {
	return m.users.Projection().Count()
}
