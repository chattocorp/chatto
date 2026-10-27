package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	ExternalIdentityFlowTTL = 15 * time.Minute

	externalIdentityCreateTokenKeyPrefix = "external_identity_create."
	externalIdentityLinkTokenKeyPrefix   = "external_identity_link."
	externalIdentityLinkStartKeyPrefix   = "external_identity_link_start."

	ExternalIdentityFlowKindCreate = "create"
	ExternalIdentityFlowKindLink   = "link"
)

var (
	ErrExternalIdentityFlowNotFound  = errors.New("external identity flow not found")
	ErrExternalIdentityFlowExpired   = errors.New("external identity flow expired")
	ErrExternalIdentityFlowWrongKind = errors.New("external identity flow has the wrong kind")
	ErrExternalIdentityFlowUserBound = errors.New("external identity flow is bound to a different user")
	ErrExternalIdentityNotFound      = errors.New("external identity is not linked to this account")
	ErrExternalIdentityLastMethod    = errors.New("cannot disconnect the last sign-in method")
)

type ExternalIdentity struct {
	ProviderID   string
	ProviderType string
	Issuer       string
	Subject      string
	SubjectHash  string
}

type PendingExternalIdentityLinkStart struct {
	ProviderID   string    `json:"provider_id"`
	RedirectPath string    `json:"redirect_path,omitempty"`
	BoundUserID  string    `json:"bound_user_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type PendingExternalIdentityFlow struct {
	Kind          string `json:"kind"`
	ProviderID    string `json:"provider_id"`
	ProviderType  string `json:"provider_type"`
	ProviderLabel string `json:"provider_label"`
	Issuer        string `json:"issuer"`
	Subject       string `json:"subject"`
	SubjectHash   string `json:"subject_hash"`
	VerifiedEmail string `json:"verified_email,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	LoginHint     string `json:"login_hint,omitempty"`
	// ProviderUsername is the verified username without email/display-name fallback.
	ProviderUsername string    `json:"provider_username,omitempty"`
	DisplayNameHint  string    `json:"display_name_hint,omitempty"`
	RedirectPath     string    `json:"redirect_path,omitempty"`
	BoundUserID      string    `json:"bound_user_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	InvitationID     string    `json:"invitation_id,omitempty"`
	// Groups and GroupsKnown carry the provider groups claim to account
	// creation for group role sync. Flows from older versions have neither.
	Groups      []string `json:"groups,omitempty"`
	GroupsKnown bool     `json:"groups_known,omitempty"`
}

func (c *ChattoCore) externalIdentityCreateTokenKey(token string) string {
	return c.runtimeTokenKey(externalIdentityCreateTokenKeyPrefix, token)
}

func (c *ChattoCore) externalIdentityLinkTokenKey(token string) string {
	return c.runtimeTokenKey(externalIdentityLinkTokenKeyPrefix, token)
}

func (c *ChattoCore) externalIdentityLinkStartKey(token string) string {
	return c.runtimeTokenKey(externalIdentityLinkStartKeyPrefix, token)
}

func (c *ChattoCore) CreatePendingExternalIdentityLinkStart(ctx context.Context, providerID, redirectPath, userID string) (string, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return "", err
	}
	start := PendingExternalIdentityLinkStart{
		ProviderID:   strings.TrimSpace(providerID),
		RedirectPath: strings.TrimSpace(redirectPath),
		BoundUserID:  strings.TrimSpace(userID),
		CreatedAt:    time.Now(),
	}
	if start.ProviderID == "" || start.BoundUserID == "" {
		return "", ErrInvalidArgument
	}
	token := NewExternalIdentityLinkStartToken()
	data, err := json.Marshal(start)
	if err != nil {
		return "", fmt.Errorf("marshal external identity link start: %w", err)
	}
	_, err = c.storage.runtimeStateKV.Create(ctx, c.externalIdentityLinkStartKey(token), data, jetstream.KeyTTL(ExternalIdentityFlowTTL))
	if err != nil {
		return "", fmt.Errorf("store external identity link start: %w", err)
	}
	return token, nil
}

func (c *ChattoCore) ConsumePendingExternalIdentityLinkStart(ctx context.Context, token string) (*PendingExternalIdentityLinkStart, error) {
	key := c.externalIdentityLinkStartKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrExternalIdentityFlowNotFound
		}
		return nil, fmt.Errorf("get external identity link start: %w", err)
	}
	var start PendingExternalIdentityLinkStart
	if err := json.Unmarshal(entry.Value(), &start); err != nil {
		return nil, fmt.Errorf("unmarshal external identity link start: %w", err)
	}
	if time.Since(start.CreatedAt) > ExternalIdentityFlowTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrExternalIdentityFlowExpired
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, fmt.Errorf("delete external identity link start: %w", err)
	}
	return &start, nil
}

func (c *ChattoCore) CreatePendingExternalIdentityCreateFlow(ctx context.Context, flow PendingExternalIdentityFlow) (string, error) {
	flow.Kind = ExternalIdentityFlowKindCreate
	token := NewExternalIdentityCreateToken()
	if err := c.storePendingExternalIdentityFlow(ctx, token, flow); err != nil {
		return "", err
	}
	return token, nil
}

func (c *ChattoCore) CreatePendingExternalIdentityLinkFlow(ctx context.Context, flow PendingExternalIdentityFlow, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrInvalidArgument
	}
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return "", err
	}
	flow.Kind = ExternalIdentityFlowKindLink
	flow.BoundUserID = userID
	token := NewExternalIdentityLinkToken()
	if err := c.storePendingExternalIdentityFlow(ctx, token, flow); err != nil {
		return "", err
	}
	return token, nil
}

func (c *ChattoCore) storePendingExternalIdentityFlow(ctx context.Context, token string, flow PendingExternalIdentityFlow) error {
	flow.ProviderID = strings.TrimSpace(flow.ProviderID)
	flow.ProviderType = strings.TrimSpace(flow.ProviderType)
	flow.Issuer = strings.TrimSpace(flow.Issuer)
	flow.Subject = strings.TrimSpace(flow.Subject)
	if flow.SubjectHash == "" && flow.Issuer != "" && flow.Subject != "" {
		flow.SubjectHash = externalIdentityHash(flow.Issuer, flow.Subject)
	}
	if flow.ProviderID == "" || flow.ProviderType == "" || flow.Issuer == "" || flow.Subject == "" || flow.SubjectHash == "" {
		return fmt.Errorf("external identity flow requires provider and identity fields")
	}
	if flow.ProviderLabel == "" {
		flow.ProviderLabel = flow.ProviderID
	}
	if flow.CreatedAt.IsZero() {
		flow.CreatedAt = time.Now()
	}

	data, err := json.Marshal(flow)
	if err != nil {
		return fmt.Errorf("marshal external identity flow: %w", err)
	}

	var key string
	switch flow.Kind {
	case ExternalIdentityFlowKindCreate:
		key = c.externalIdentityCreateTokenKey(token)
	case ExternalIdentityFlowKindLink:
		key = c.externalIdentityLinkTokenKey(token)
	default:
		return ErrExternalIdentityFlowWrongKind
	}
	_, err = c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(ExternalIdentityFlowTTL))
	if err != nil {
		return fmt.Errorf("store external identity flow: %w", err)
	}
	return nil
}

func (c *ChattoCore) GetPendingExternalIdentityFlow(ctx context.Context, token string) (*PendingExternalIdentityFlow, error) {
	if flow, err := c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityCreateTokenKey(token)); err == nil {
		return flow, nil
	} else if !errors.Is(err, ErrExternalIdentityFlowNotFound) {
		return nil, err
	}
	return c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityLinkTokenKey(token))
}

func (c *ChattoCore) GetPendingExternalIdentityCreateFlow(ctx context.Context, token string) (*PendingExternalIdentityFlow, error) {
	flow, err := c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityCreateTokenKey(token))
	if err != nil {
		return nil, err
	}
	if flow.Kind != ExternalIdentityFlowKindCreate {
		return nil, ErrExternalIdentityFlowWrongKind
	}
	return flow, nil
}

func (c *ChattoCore) GetPendingExternalIdentityLinkFlow(ctx context.Context, token, userID string) (*PendingExternalIdentityFlow, error) {
	flow, err := c.getPendingExternalIdentityFlowByKey(ctx, c.externalIdentityLinkTokenKey(token))
	if err != nil {
		return nil, err
	}
	if flow.Kind != ExternalIdentityFlowKindLink {
		return nil, ErrExternalIdentityFlowWrongKind
	}
	if flow.BoundUserID != userID {
		return nil, ErrExternalIdentityFlowUserBound
	}
	return flow, nil
}

func (c *ChattoCore) getPendingExternalIdentityFlowByKey(ctx context.Context, key string) (*PendingExternalIdentityFlow, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrExternalIdentityFlowNotFound
		}
		return nil, fmt.Errorf("get external identity flow: %w", err)
	}
	var flow PendingExternalIdentityFlow
	if err := json.Unmarshal(entry.Value(), &flow); err != nil {
		return nil, fmt.Errorf("unmarshal external identity flow: %w", err)
	}
	if time.Since(flow.CreatedAt) > ExternalIdentityFlowTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrExternalIdentityFlowExpired
	}
	return &flow, nil
}

func (c *ChattoCore) DeletePendingExternalIdentityFlow(ctx context.Context, token string) error {
	var firstErr error
	for _, key := range []string{c.externalIdentityCreateTokenKey(token), c.externalIdentityLinkTokenKey(token)} {
		err := c.storage.runtimeStateKV.Delete(ctx, key)
		if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("delete external identity flow: %w", firstErr)
	}
	return nil
}

func (c *ChattoCore) CreateUserForExternalIdentity(ctx context.Context, login, displayName string, flow *PendingExternalIdentityFlow) (*readmodel.User, error) {
	if flow == nil || flow.Kind != ExternalIdentityFlowKindCreate {
		return nil, ErrExternalIdentityFlowWrongKind
	}
	user, err := c.createProviderUser(ctx, login, displayName, flow)
	if err != nil {
		return nil, err
	}
	if flow.AvatarURL != "" {
		if err := c.ImportUserAvatarFromURL(ctx, user.Id, flow.AvatarURL); err != nil {
			c.logger.Warn("Failed to import provider avatar", "provider_id", flow.ProviderID, "provider_type", flow.ProviderType, "user_id", user.Id, "error", err)
		}
	}
	return user, nil
}

func (c *ChattoCore) LinkPendingExternalIdentity(ctx context.Context, userID string, flow *PendingExternalIdentityFlow) (ExternalIdentity, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return ExternalIdentity{}, err
	}
	if flow == nil || flow.Kind != ExternalIdentityFlowKindLink {
		return ExternalIdentity{}, ErrExternalIdentityFlowWrongKind
	}
	if flow.BoundUserID != userID {
		return ExternalIdentity{}, ErrExternalIdentityFlowUserBound
	}
	if err := c.LinkExternalIdentity(ctx, flow.ProviderID, flow.ProviderType, flow.Issuer, flow.Subject, userID); err != nil {
		return ExternalIdentity{}, err
	}
	return ExternalIdentity{
		ProviderID:   flow.ProviderID,
		ProviderType: flow.ProviderType,
		Issuer:       flow.Issuer,
		Subject:      flow.Subject,
		SubjectHash:  flow.SubjectHash,
	}, nil
}

func (c *ChattoCore) ConfirmPendingExternalIdentityLink(ctx context.Context, flow *PendingExternalIdentityFlow) (ExternalIdentity, error) {
	if flow == nil || flow.Kind != ExternalIdentityFlowKindLink {
		return ExternalIdentity{}, ErrExternalIdentityFlowWrongKind
	}
	if flow.BoundUserID == "" {
		return ExternalIdentity{}, ErrExternalIdentityFlowUserBound
	}
	if err := c.requireHumanUser(ctx, flow.BoundUserID); err != nil {
		return ExternalIdentity{}, err
	}
	if err := c.LinkExternalIdentity(ctx, flow.ProviderID, flow.ProviderType, flow.Issuer, flow.Subject, flow.BoundUserID); err != nil {
		return ExternalIdentity{}, err
	}
	return ExternalIdentity{
		ProviderID:   flow.ProviderID,
		ProviderType: flow.ProviderType,
		Issuer:       flow.Issuer,
		Subject:      flow.Subject,
		SubjectHash:  flow.SubjectHash,
	}, nil
}

// ExternalIdentitiesForUser returns detached identity links for an existing
// human account, ordered by provider ID and hash. It catches up both user and
// authentication projections. The raw issuer and subject require an account-
// owner or operator response boundary; they must not be logged.
func (c *ChattoCore) ExternalIdentitiesForUser(ctx context.Context, userID string) ([]ExternalIdentity, error) {
	// Wait before the account check, so a replica that has not yet applied the
	// user's creation does not reject it. The fixed filter keeps the unvalidated
	// user ID out of the subject.
	if err := c.userModel.waitForUsersCurrent(ctx, "external identities", evtstream.UserSubjectFilter()); err != nil {
		return nil, err
	}
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return nil, err
	}
	return c.userModel.externalIdentities(userID), nil
}

// ExternalIdentitySignInPolicy describes the sign-in methods enabled on the
// serving replica. Issuers contains exact OIDC issuer URLs and configured
// provider IDs for OAuth-only providers. Stored event-time provider IDs do not
// determine whether an identity is usable.
type ExternalIdentitySignInPolicy struct {
	// PasswordLoginEnabled counts a stored password as an available method.
	PasswordLoginEnabled bool
	// Issuers contains identity namespaces accepted by configured providers.
	Issuers []string
}

// AllowsIdentity reports whether a configured provider accepts this namespace.
func (p ExternalIdentitySignInPolicy) AllowsIdentity(identity ExternalIdentity) bool {
	return slices.Contains(p.Issuers, identity.Issuer)
}

// CanDisconnect reports whether removing subjectHash from this snapshot leaves
// an enabled sign-in method. It does not verify that the target exists or that
// the caller is authorized. Mutations must check again inside the user OCC loop.
func (p ExternalIdentitySignInPolicy) CanDisconnect(subjectHash string, identities []ExternalIdentity, hasPassword bool) bool {
	if hasPassword && p.PasswordLoginEnabled {
		return true
	}
	for _, identity := range identities {
		if identity.SubjectHash != subjectHash && p.AllowsIdentity(identity) {
			return true
		}
	}
	return false
}

// DisconnectExternalIdentityAs removes a link with explicit actor attribution.
// It refuses removal when no method enabled by policy would remain. The check
// runs inside user OCC, so concurrent removals cannot remove both last methods.
// Existing sessions remain valid. A missing link returns ErrExternalIdentityNotFound.
func (c *ChattoCore) DisconnectExternalIdentityAs(ctx context.Context, actorID, userID, subjectHash string, policy ExternalIdentitySignInPolicy) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	userID = strings.TrimSpace(userID)
	subjectHash = strings.TrimSpace(subjectHash)
	if userID == "" || subjectHash == "" {
		return ErrInvalidArgument
	}
	unlink := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_UserExternalIdentityUnlinked{
		UserExternalIdentityUnlinked: &evtv1.UserExternalIdentityUnlinkedEvent{
			UserId:                      userID,
			SubjectHash:                 subjectHash,
			PreserveExistingCredentials: true,
		},
	}})
	unlinkSubject := evtstream.UserAggregate(userID).SubjectFor(unlink)
	userFilter := evtstream.UserSubjectFilter()
	rbacFilter := evtstream.RBACSubjectFilter()

	// The unlink and the revocation of the group roles that its provider gave
	// commit in one atomic batch with user and RBAC OCC. A failure therefore
	// cannot leave an unlinked account with the provider's roles.
	var source ProviderRoleSource
	for attempt := range maxUserMutationRetries {
		userSeq, err := c.EventPublisher.LastSubjectSeq(ctx, userFilter)
		if err != nil {
			return fmt.Errorf("read user OCC filter seq: %w", err)
		}
		rbacSeq, err := c.EventPublisher.LastSubjectSeq(ctx, rbacFilter)
		if err != nil {
			return fmt.Errorf("read RBAC OCC filter seq: %w", err)
		}
		if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(userFilter, userSeq)); err != nil {
			return fmt.Errorf("wait for user projection: %w", err)
		}
		if err := c.userModel.waitForUserAuthCurrent(ctx, "user mutation"); err != nil {
			return fmt.Errorf("wait for user auth projection: %w", err)
		}
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(rbacFilter, rbacSeq)); err != nil {
			return fmt.Errorf("wait for RBAC projection: %w", err)
		}
		// Check the account after the projection wait, so a replica that has
		// not yet applied the user's creation does not reject it.
		unlinked, err := c.checkExternalIdentityDisconnect(ctx, userID, subjectHash, policy)
		if err != nil {
			return err
		}
		if actorID != SystemActorID && c.IdentityUnlinkDisabled(unlinked) {
			return ErrExternalIdentityUnlinkDisabled
		}

		entries := []evtstream.BatchEntry{{Subject: unlinkSubject, Event: unlink, Expect: events.ExpectFilterSeq(userFilter, userSeq)}}
		// Another identity with the same issuer keeps the account under that
		// provider's group role sync, so its roles stay.
		source = ProviderRoleSource{ProviderID: unlinked.ProviderID, Issuer: unlinked.Issuer}
		if !hasOtherIssuerIdentity(c.userModel.externalIdentities(userID), unlinked) {
			for _, roleName := range c.rbacModel.providerAssignments(userID, unlinked.Issuer) {
				entries = append(entries, source.revokedEntry(userID, roleName))
			}
		}
		if len(entries) > 1 {
			entries[1].Expect = events.ExpectFilterSeq(rbacFilter, rbacSeq)
		}

		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
		if err == nil {
			// A concurrent group role sync can commit a grant after the batch
			// read the RBAC state. Check again; the check waits until the
			// unlink is visible. It runs before the waits below, which end with
			// the request. The unlink is committed, so a failed check is
			// logged, not returned.
			if err := c.revokeProviderGroupRolesWithoutIdentity(ctx, userID, source); err != nil {
				c.logger.Error("Provider group role check after unlink failed", "provider_id", source.ProviderID, "user_id", userID, "error", err)
			}
			if err := c.userModel.waitForUsers(ctx, events.SubjectPosition(unlinkSubject, seqs[0])); err != nil {
				return fmt.Errorf("wait for user projection: %w", err)
			}
			if err := c.userModel.waitForUserAuth(ctx, events.SubjectPosition(unlinkSubject, seqs[0])); err != nil {
				return fmt.Errorf("wait for user auth projection: %w", err)
			}
			if last := len(entries) - 1; last > 0 {
				if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(entries[last].Subject, seqs[last])); err != nil {
					return fmt.Errorf("wait for RBAC projection: %w", err)
				}
			}
			return nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return fmt.Errorf("external identity unlink OCC retry exhausted after %d attempts: %w", maxUserMutationRetries, events.ErrConflict)
}

// checkExternalIdentityDisconnect returns the identity to unlink. It refuses
// removal when no sign-in method that policy enables would remain.
func (c *ChattoCore) checkExternalIdentityDisconnect(ctx context.Context, userID, subjectHash string, policy ExternalIdentitySignInPolicy) (ExternalIdentity, error) {
	if err := c.requireHumanUser(ctx, userID); err != nil {
		return ExternalIdentity{}, err
	}
	var unlinked ExternalIdentity
	found := false
	identities := c.userModel.externalIdentities(userID)
	for _, identity := range identities {
		if identity.SubjectHash == subjectHash {
			found = true
			unlinked = identity
		}
	}
	if !found {
		return ExternalIdentity{}, ErrExternalIdentityNotFound
	}
	_, hasPassword := c.userModel.passwordHash(userID)
	if !policy.CanDisconnect(subjectHash, identities, hasPassword) {
		return ExternalIdentity{}, ErrExternalIdentityLastMethod
	}
	return unlinked, nil
}

// hasOtherIssuerIdentity reports whether identities contain another identity
// with the same issuer as identity. The issuer is the identity namespace of a
// provider, also for links made under an older provider ID.
func hasOtherIssuerIdentity(identities []ExternalIdentity, identity ExternalIdentity) bool {
	return slices.ContainsFunc(identities, func(other ExternalIdentity) bool {
		return other.Issuer == identity.Issuer && other.SubjectHash != identity.SubjectHash
	})
}
