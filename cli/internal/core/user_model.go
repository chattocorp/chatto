package core

import (
	"context"
	"errors"
	"sort"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var errContentKeyProjectionUnavailable = errors.New("content key projection is unavailable")

// UserModel owns user-derived projection reads and readiness barriers.
type UserModel struct {
	publisher *events.Publisher

	users          *UserProjection
	usersProjector *events.Projector
	auth           *UserAuthProjection
	authProjector  *events.Projector

	contentKeys          *ContentKeyProjection
	contentKeysProjector *events.Projector
}

func newUserModel(
	publisher *events.Publisher,
	users *UserProjection,
	usersProjector *events.Projector,
	auth *UserAuthProjection,
	authProjector *events.Projector,
	contentKeys *ContentKeyProjection,
	contentKeysProjector *events.Projector,
) *UserModel {
	return &UserModel{
		publisher:            publisher,
		users:                users,
		usersProjector:       usersProjector,
		auth:                 auth,
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
	if m.publisher == nil || m.authProjector == nil || m.auth == nil {
		return nil
	}
	return waitForProjectionSubjectsCurrent(ctx, m.publisher, name+" auth", m.authProjector, m.auth.Subjects()...)
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

func (m *UserModel) user(ctx context.Context, userID string) (*corev1.User, bool, error) {
	return m.users.GetContext(ctx, userID)
}

func (m *UserModel) userReference(ctx context.Context, userID string) (*corev1.User, bool, error) {
	return m.users.GetReferenceContext(ctx, userID)
}

func (m *UserModel) userReferences(ctx context.Context, userIDs []string) ([]*corev1.User, error) {
	return m.users.GetReferencesContext(ctx, userIDs)
}

func (m *UserModel) userByLogin(ctx context.Context, login string) (*corev1.User, bool, error) {
	return m.users.GetByLoginContext(ctx, login)
}

func (m *UserModel) userByEmail(ctx context.Context, email string) (*corev1.User, bool, error) {
	return m.users.GetByEmailContext(ctx, email)
}

func (m *UserModel) userByExternalIdentity(ctx context.Context, issuer, subject string) (*corev1.User, bool, error) {
	userID, ok := m.auth.ExternalIdentityOwnerID(issuer, subject)
	if !ok {
		return nil, false, nil
	}
	return m.users.GetContext(ctx, userID)
}

func (m *UserModel) loginExists(login string) bool {
	return m.users.LoginExists(login)
}

func (m *UserModel) emailClaimed(email string) bool {
	return m.users.EmailClaimed(email)
}

func (m *UserModel) emailOwnerID(email string) (string, bool) {
	return m.users.EmailOwnerID(email)
}

func (m *UserModel) externalIdentityOwnerID(issuer, subject string) (string, bool) {
	return m.auth.ExternalIdentityOwnerID(issuer, subject)
}

func (m *UserModel) externalIdentities(userID string) []ExternalIdentity {
	return m.auth.ExternalIdentities(userID)
}

func (m *UserModel) passwordHash(userID string) ([]byte, bool) {
	hash, _, ok := m.auth.PasswordHashWithSetAt(userID)
	return hash, ok
}

func (m *UserModel) passwordHashWithSetAt(userID string) ([]byte, time.Time, bool) {
	return m.auth.PasswordHashWithSetAt(userID)
}

func (m *UserModel) authGeneration(userID string) (uint64, bool) {
	return m.auth.AuthGeneration(userID)
}

func (m *UserModel) avatar(userID string) (*corev1.AssetRecord, bool) {
	return m.users.Avatar(userID)
}

func (m *UserModel) isPublicAvatarAsset(assetID string) bool {
	if m == nil || m.users == nil {
		return false
	}
	return m.users.IsPublicAvatarAsset(assetID)
}

func (m *UserModel) verifiedEmails(ctx context.Context, userID string) ([]VerifiedEmail, error) {
	return m.users.VerifiedEmailsContext(ctx, userID)
}

func (m *UserModel) hasVerifiedEmail(userID string) bool {
	return m.users.HasVerifiedEmail(userID)
}

func (m *UserModel) hasVerifiedFactor(userID string) bool {
	return m.users.HasVerifiedEmail(userID) || m.auth.HasExternalIdentity(userID)
}

func (m *UserModel) hasOAuthConsent(userID, redirectOrigin string) bool {
	return m.auth.HasOAuthConsent(userID, redirectOrigin)
}

func (m *UserModel) loginChangedAt(userID string) time.Time {
	return m.users.LoginChangedAt(userID)
}

func (m *UserModel) allUsers(ctx context.Context) ([]*corev1.User, error) {
	return m.users.UsersContext(ctx)
}

func (m *UserModel) verifiedUserIDs() []string {
	return m.users.VerifiedUserIDs()
}

func (m *UserModel) verifiedAccountIDs() []string {
	seen := make(map[string]struct{})
	for _, userID := range m.users.VerifiedUserIDs() {
		seen[userID] = struct{}{}
	}
	for _, userID := range m.auth.VerifiedAccountIDs() {
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
	return m.users.Count()
}
