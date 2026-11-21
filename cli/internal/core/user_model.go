package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/chattocorp/chatto/cli/internal/parallel"
	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

var errContentKeyProjectionUnavailable = errors.New("content key projection is unavailable")

// UserModel owns user-derived projection reads and readiness barriers.
type UserModel struct {
	publisher *evtstream.Publisher

	users       events.ProjectionHandle[*UserProjection]
	auth        events.ProjectionHandle[*UserAuthProjection]
	contentKeys events.ProjectionHandle[*ContentKeyProjection]
}

// UserContentSnapshot is an immutable capture of one user's content-view
// state. It contains encrypted PII and can be hydrated after the content-view
// read barrier is released.
type UserContentSnapshot struct {
	state      *projectedUserSnapshot
	capturedAt time.Time
}

// Deleted reports whether the captured account was deleted or crypto-shredded.
// Hydration returns no content for such an account; callers that must keep
// its ID show it as a deleted user instead.
func (s *UserContentSnapshot) Deleted() bool {
	return s != nil && s.state != nil && (s.state.deleted || s.state.shredded)
}

// HydratedUserContent contains the public user state that was captured in one
// UserContentSnapshot. Presence is runtime state and is not included.
type HydratedUserContent struct {
	User        *readmodel.User
	Avatar      *evtv1.AssetRecord
	Preferences *evtv1.ServerUserPreferences
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

// waitForUsersCurrent waits until the Users projection has applied the newest
// event on subjects. It does not wait for the User Auth projection. A caller
// that also reads User Auth state waits for it separately.
func (m *UserModel) waitForUsersCurrent(ctx context.Context, name string, subjects ...string) error {
	return m.waitForUserProjectionsCurrent(ctx, name, subjects, nil)
}

func (m *UserModel) waitForUserAuth(ctx context.Context, pos events.StreamPosition) error {
	if m.auth.Projector() == nil {
		return nil
	}
	return waitForPositionAll(ctx, pos, waitForProjection("user auth", m.auth.Projector()))
}

// waitForUserAuthCurrent waits until the User Auth projection is current for
// all users and all of its subject filters. Each filter is a wildcard read on
// the EVT stream leader. Use it only where a read depends on the state of
// users that the caller cannot name, such as a lookup or a uniqueness check
// across all users. Otherwise use waitForUserAuthSubjectsCurrent with the
// subjects that can change the read.
func (m *UserModel) waitForUserAuthCurrent(ctx context.Context, name string) error {
	if m.auth.Projection() == nil {
		return nil
	}
	return m.waitForUserProjectionsCurrent(ctx, name+" auth", nil, m.auth.Projection().Subjects())
}

// waitForUserAuthSubjectsCurrent waits until the User Auth projection has
// applied the newest event on subjects.
func (m *UserModel) waitForUserAuthSubjectsCurrent(ctx context.Context, name string, subjects ...string) error {
	return m.waitForUserProjectionsCurrent(ctx, name+" auth", nil, subjects)
}

// waitForUserProjectionsCurrent waits until the Users projection has applied
// the newest event on usersSubjects and the User Auth projection has applied
// the newest event on authSubjects. It reads the position of each distinct
// subject once, and it reads all of them concurrently. Then it waits for the
// two projections concurrently. Thus, the wait costs about two round trips to
// the EVT stream leader, whatever the number of subjects. A projection that
// is not wired is not waited for.
func (m *UserModel) waitForUserProjectionsCurrent(ctx context.Context, name string, usersSubjects, authSubjects []string) error {
	if m.publisher == nil {
		return nil
	}
	if m.users.Projector() == nil {
		usersSubjects = nil
	}
	if m.auth.Projector() == nil || m.auth.Projection() == nil {
		authSubjects = nil
	}
	subjects := make([]string, 0, len(usersSubjects)+len(authSubjects))
	for _, subject := range slices.Concat(usersSubjects, authSubjects) {
		if !slices.Contains(subjects, subject) {
			subjects = append(subjects, subject)
		}
	}
	positions, err := parallel.Map(ctx, 0, subjects, func(ctx context.Context, _ int, subject string) (events.StreamPosition, error) {
		return m.publisher.LastSubjectPosition(ctx, subject)
	})
	if err != nil {
		return fmt.Errorf("read %s projection target seq: %w", name, err)
	}
	newest := func(subset []string) events.StreamPosition {
		var newest events.StreamPosition
		for i, subject := range subjects {
			if positions[i].Seq > newest.Seq && slices.Contains(subset, subject) {
				newest = positions[i]
			}
		}
		return newest
	}
	waits, waitCtx := errgroup.WithContext(ctx)
	wait := func(projectionName string, projector *events.Projector, pos events.StreamPosition) {
		if pos.IsZero() {
			return
		}
		waits.Go(func() error {
			if err := projector.WaitFor(waitCtx, pos); err != nil {
				return fmt.Errorf("wait for %s %s projection: %w", name, projectionName, err)
			}
			return nil
		})
	}
	wait("users", m.users.Projector(), newest(usersSubjects))
	wait("user auth", m.auth.Projector(), newest(authSubjects))
	return waits.Wait()
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
func (m *UserModel) activeContentKey(userID string, purpose evtv1.UserDEKPurpose) (*evtv1.UserDEKGeneratedEvent, bool, error) {
	if m.contentKeys.Projection() == nil {
		return nil, false, errContentKeyProjectionUnavailable
	}
	event, ok := m.contentKeys.Projection().Active(userID, purpose)
	return event, ok, nil
}

// contentKeyAtEpoch returns a projected DEK at an exact epoch. The projection
// preserves compatibility with legacy purpose-unspecified DEKs.
func (m *UserModel) contentKeyAtEpoch(userID string, purpose evtv1.UserDEKPurpose, epoch int32) (*evtv1.UserDEKGeneratedEvent, bool, error) {
	if m.contentKeys.Projection() == nil {
		return nil, false, errContentKeyProjectionUnavailable
	}
	event, ok := m.contentKeys.Projection().Get(userID, purpose, epoch)
	return event, ok, nil
}

func (m *UserModel) user(ctx context.Context, userID string) (*readmodel.User, bool, error) {
	return m.users.Projection().GetContext(ctx, userID)
}

// CaptureUserContentSnapshot copies one user's projected state without
// resolving encryption keys. Callers can use it inside a content-view read
// barrier because it performs bounded in-memory work only.
func (c *ChattoCore) CaptureUserContentSnapshot(userID string) *UserContentSnapshot {
	if c == nil || c.userModel == nil || c.userModel.users.Projection() == nil {
		return nil
	}
	state := c.userModel.users.Projection().contentSnapshot(userID)
	if state == nil {
		return nil
	}
	return &UserContentSnapshot{state: state, capturedAt: time.Now()}
}

// HydrateUserContentSnapshot resolves encrypted PII after the content-view
// read barrier is released. The returned state still represents the captured
// content generation.
func (c *ChattoCore) HydrateUserContentSnapshot(ctx context.Context, snapshot *UserContentSnapshot) (*HydratedUserContent, bool, error) {
	if c == nil || c.userModel == nil || c.userModel.users.Projection() == nil || snapshot == nil || snapshot.state == nil {
		return nil, false, nil
	}
	state := cloneProjectedUserSnapshot(snapshot.state)
	user, ok, err := c.userModel.users.Projection().hydrateUserSnapshot(WithDEKRequestCache(ctx), state, snapshot.capturedAt)
	if err != nil || !ok {
		return nil, ok, err
	}
	result := &HydratedUserContent{User: user}
	if state.avatar != nil {
		result.Avatar = proto.Clone(state.avatar).(*evtv1.AssetRecord)
	}
	if state.preferences != nil {
		result.Preferences = proto.Clone(state.preferences).(*evtv1.ServerUserPreferences)
	}
	return result, true, nil
}

func (m *UserModel) userReference(ctx context.Context, userID string) (*readmodel.User, bool, error) {
	return m.users.Projection().GetReferenceContext(ctx, userID)
}

func (m *UserModel) userReferences(ctx context.Context, userIDs []string) ([]*readmodel.User, error) {
	return m.users.Projection().GetReferencesContext(ctx, userIDs)
}

func (m *UserModel) userByLogin(ctx context.Context, login string) (*readmodel.User, bool, error) {
	return m.users.Projection().GetByLoginContext(ctx, login)
}

func (m *UserModel) userByEmail(ctx context.Context, email string) (*readmodel.User, bool, error) {
	return m.users.Projection().GetByEmailContext(ctx, email)
}

func (m *UserModel) userByExternalIdentity(ctx context.Context, issuer, subject string) (*readmodel.User, bool, error) {
	userID, ok := m.auth.Projection().ExternalIdentityOwnerID(issuer, subject)
	if !ok {
		return nil, false, nil
	}
	return m.users.Projection().GetContext(ctx, userID)
}

func (m *UserModel) userByExternalIdentityForAuthentication(ctx context.Context, issuer, subject string) (*readmodel.User, uint64, bool, error) {
	userID, authGeneration, ok := m.auth.Projection().ExternalIdentityAuthentication(issuer, subject)
	if !ok {
		return nil, 0, false, nil
	}
	user, ok, err := m.users.Projection().GetContext(ctx, userID)
	return user, authGeneration, ok, err
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
	return m.auth.Projection().PasswordHash(userID)
}

func (m *UserModel) authGeneration(userID string) (uint64, bool) {
	return m.auth.Projection().AuthGeneration(userID)
}

func (m *UserModel) matchBotAPIKeyCredential(userID string, verifier []byte) (BotAPIKeyCredential, bool) {
	return m.auth.Projection().MatchBotAPIKeyCredential(userID, verifier)
}

func (m *UserModel) botAPIKeyCredentials(userID string) []BotAPIKeyCredential {
	return m.auth.Projection().BotAPIKeyCredentials(userID)
}

func (m *UserModel) botAPIKeyLegacyCreatedAt(userID string) time.Time {
	return m.auth.Projection().BotAPIKeyLegacyCreatedAt(userID)
}

func (m *UserModel) botIncomingWebhookCredential(userID, webhookID string) (BotIncomingWebhookCredential, bool) {
	return m.auth.Projection().BotIncomingWebhookCredential(userID, webhookID)
}

func (m *UserModel) botIncomingWebhookCredentials(userID string) []BotIncomingWebhookCredential {
	return m.auth.Projection().BotIncomingWebhookCredentials(userID)
}

func (m *UserModel) avatar(userID string) (*evtv1.AssetRecord, bool) {
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

func (m *UserModel) allUsers(ctx context.Context) ([]*readmodel.User, error) {
	return m.users.Projection().UsersContext(ctx)
}

// adminDirectoryCandidates avoids profile reads for timestamp-ordered users.
// Legacy users without timestamps still need their login for the fallback sort.
func (m *UserModel) adminDirectoryCandidates(ctx context.Context, search string) ([]*readmodel.User, error) {
	if strings.TrimSpace(search) != "" {
		return m.allUsers(ctx)
	}
	ctx = WithDEKRequestCache(ctx)
	entries := m.users.Projection().ActiveDirectoryMetadata()
	users := make([]*readmodel.User, 0, len(entries))
	for _, entry := range entries {
		user := &readmodel.User{Id: entry.ID, CreatedAt: entry.CreatedAt}
		if entry.CreatedAt == nil {
			legacy, ok, err := m.users.Projection().GetContext(ctx, entry.ID)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			user.Login = legacy.GetLogin()
		}
		users = append(users, user)
	}
	return users, nil
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

func (m *UserModel) deactivated(userID string) bool {
	return m.users.Projection().Deactivated(userID)
}

func (m *UserModel) botIDsOwnedBy(ownerUserID string) []string {
	return m.users.Projection().BotIDsOwnedBy(ownerUserID)
}

func (m *UserModel) botIDs() []string {
	return m.users.Projection().BotIDs()
}

func (m *UserModel) isBotAndOwner(userID string) (bool, string, bool) {
	return m.users.Projection().IsBotAndOwner(userID)
}

// activeHumanIDs returns current human account IDs without decrypting profiles.
func (m *UserModel) activeHumanIDs() []string {
	ids := m.users.Projection().AllActiveIDs()
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if isBot, _, exists := m.users.Projection().IsBotAndOwner(id); exists && !isBot {
			result = append(result, id)
		}
	}
	return result
}
