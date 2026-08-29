package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

var (
	ErrOAuthClientBlocked                = errors.New("OAuth client is blocked")
	errOAuthClientMutationRetryExhausted = errors.New("OAuth client mutation OCC retry exhausted")
)

type OAuthClientModel struct {
	projection events.ProjectionHandle[*OAuthClientProjection]
}

// OAuthClientAuthorization describes the validated client identity attached to
// one user-approved authorization request.
type OAuthClientAuthorization struct {
	UserID         string
	ClientID       string
	ClientName     string
	ClientOrigin   string
	RedirectOrigin string
	Source         evtv1.OAuthClientSource
}

func newOAuthClientModel(projection events.ProjectionHandle[*OAuthClientProjection]) *OAuthClientModel {
	return &OAuthClientModel{projection: projection}
}

func (c *ChattoCore) RequireOAuthClientAllowed(ctx context.Context, clientID string) error {
	clientID = strings.TrimSpace(clientID)
	if c.oauthClientModel == nil || clientID == "" {
		return nil
	}
	if err := c.oauthClientModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return err
	}
	if c.oauthClientAccessDenied(clientID) {
		return ErrOAuthClientBlocked
	}
	return nil
}

func (c *ChattoCore) oauthClientAccessDenied(clientID string) bool {
	if c.oauthClientModel == nil || clientID == "" {
		return false
	}
	state, ok := c.oauthClientModel.projection.Projection().get(clientID)
	return ok && oauthClientPolicyDeniesAccess(state.Policy)
}

func oauthClientPolicyDeniesAccess(policy evtv1.OAuthClientPolicy) bool {
	return policy != evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT &&
		policy != evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED
}

func oauthClientPolicySupported(policy evtv1.OAuthClientPolicy) bool {
	return policy == evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT ||
		policy == evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED ||
		policy == evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED
}

// WatchOAuthClientAccessDenied returns a process-local notification backed by
// the durable OAuth-client projection. The channel closes when this replica
// observes a blocked or unsupported policy. Callers must invoke the returned
// cleanup function when they stop watching.
func (c *ChattoCore) WatchOAuthClientAccessDenied(clientID string) (<-chan struct{}, func()) {
	clientID = strings.TrimSpace(clientID)
	if c.oauthClientModel == nil || clientID == "" {
		return nil, func() {}
	}
	return c.oauthClientModel.projection.Projection().watchAccessDenied(clientID)
}

// CreateOAuthClientAuthorizationCode creates an authorization code before it
// records the successful client authorization. If the durable record cannot be
// committed, the undisclosed code is removed so callers cannot complete an
// authorization that is absent from the administrator inventory.
func (c *ChattoCore) CreateOAuthClientAuthorizationCode(ctx context.Context, authorization OAuthClientAuthorization, redirectURI, codeChallenge, codeChallengeMethod string, authGeneration uint64) (string, error) {
	return c.CreateOAuthClientAuthorizationCodeForGrant(ctx, authorization, "", nil, redirectURI, codeChallenge, codeChallengeMethod, authGeneration)
}

// CreateOAuthClientAuthorizationCodeForGrant creates a code bound to one
// delegated resource and normalized scope set before it records authorization.
func (c *ChattoCore) CreateOAuthClientAuthorizationCodeForGrant(ctx context.Context, authorization OAuthClientAuthorization, resource string, scopes []string, redirectURI, codeChallenge, codeChallengeMethod string, authGeneration uint64) (string, error) {
	return c.createOAuthClientAuthorizationCode(ctx, authorization, resource, scopes, redirectURI, codeChallenge, codeChallengeMethod, authGeneration, c.oauthClientModel.projection.Projector().WaitFor)
}

func (c *ChattoCore) createOAuthClientAuthorizationCode(ctx context.Context, authorization OAuthClientAuthorization, resource string, scopes []string, redirectURI, codeChallenge, codeChallengeMethod string, authGeneration uint64, waitFor func(context.Context, events.StreamPosition) error) (string, error) {
	code, err := c.CreateAuthCodeForClientGrantGeneration(ctx, authorization.UserID, authorization.ClientID, resource, scopes, redirectURI, codeChallenge, codeChallengeMethod, authGeneration)
	if err != nil {
		return "", err
	}
	position, err := c.appendOAuthClientAuthorization(ctx, authorization.UserID, authorization.ClientID, authorization.ClientName, authorization.ClientOrigin, authorization.RedirectOrigin, authorization.Source)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		cleanupErr := c.storage.runtimeStateKV.Delete(cleanupCtx, c.authCodeKey(code))
		if cleanupErr != nil && !errors.Is(cleanupErr, jetstream.ErrKeyNotFound) && !errors.Is(cleanupErr, jetstream.ErrKeyDeleted) {
			return "", fmt.Errorf("record OAuth client authorization: %w; discard authorization code: %v", err, cleanupErr)
		}
		return "", err
	}
	if err := waitFor(ctx, position); err != nil {
		// The event is already committed. The code and durable fact now describe
		// the same successful authorization, so a local catch-up failure must not
		// turn it into a phantom record by deleting the code.
		c.logger.Warn("OAuth client authorization committed before projection catch-up failed", "error", err)
	}
	return code, nil
}

// RecordOAuthClientAuthorization records one successful user authorization.
// Every authorization advances the durable last-authorization timestamp; the
// projection de-duplicates callback origins and authorizing users.
func (c *ChattoCore) RecordOAuthClientAuthorization(ctx context.Context, actorID, clientID, clientName, clientOrigin, redirectOrigin string, source evtv1.OAuthClientSource) error {
	position, err := c.appendOAuthClientAuthorization(ctx, actorID, clientID, clientName, clientOrigin, redirectOrigin, source)
	if err != nil {
		return err
	}
	return c.oauthClientModel.projection.Projector().WaitFor(ctx, position)
}

func (c *ChattoCore) appendOAuthClientAuthorization(ctx context.Context, actorID, clientID, clientName, clientOrigin, redirectOrigin string, source evtv1.OAuthClientSource) (events.StreamPosition, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return events.StreamPosition{}, ErrInvalidArgument
	}
	if source != evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD && source != evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_BUILT_IN {
		return events.StreamPosition{}, ErrInvalidArgument
	}
	agg := evtstream.OAuthClientAggregate(clientID)
	for attempt := 0; attempt < 5; attempt++ {
		seq, err := c.EventPublisher.LastSubjectSeq(ctx, agg.AllEventsFilter())
		if err != nil {
			return events.StreamPosition{}, err
		}
		if err := c.oauthClientModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.AllEventsFilter(), seq)); err != nil {
			return events.StreamPosition{}, err
		}
		state, exists := c.oauthClientModel.projection.Projection().get(clientID)
		origin := OAuthConsentOrigin(redirectOrigin)
		if exists && oauthClientPolicyDeniesAccess(state.Policy) {
			return events.StreamPosition{}, ErrOAuthClientBlocked
		}
		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_OauthClientAuthorizationRecorded{
			OauthClientAuthorizationRecorded: &evtv1.OAuthClientAuthorizationRecordedEvent{
				ClientId: clientID, ClientName: strings.TrimSpace(clientName), ClientUri: strings.TrimSpace(clientOrigin),
				RedirectOrigin: origin, Source: source,
			},
		}})
		published, err := c.EventPublisher.AppendAtFilter(ctx, agg.SubjectFor(event), event, agg.AllEventsFilter(), seq)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return events.StreamPosition{}, err
		}
		return events.SubjectPosition(agg.SubjectFor(event), published), nil
	}
	return events.StreamPosition{}, errOAuthClientMutationRetryExhausted
}

func (c *ChattoCore) GetOAuthClient(ctx context.Context, actorID, clientID string) (OAuthClientState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return OAuthClientState{}, err
	}
	if err := c.oauthClientModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return OAuthClientState{}, err
	}
	state, ok := c.oauthClientModel.projection.Projection().get(strings.TrimSpace(clientID))
	if !ok {
		return OAuthClientState{}, ErrNotFound
	}
	return state, nil
}

func (c *ChattoCore) ListOAuthClients(ctx context.Context, actorID string) ([]OAuthClientState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return nil, err
	}
	if err := c.oauthClientModel.projection.Projector().WaitForCurrent(ctx); err != nil {
		return nil, err
	}
	return c.oauthClientModel.projection.Projection().all(), nil
}

func (c *ChattoCore) UpdateOAuthClientPolicy(ctx context.Context, actorID, clientID string, policy evtv1.OAuthClientPolicy) (OAuthClientState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return OAuthClientState{}, err
	}
	if !oauthClientPolicySupported(policy) {
		return OAuthClientState{}, ErrInvalidArgument
	}
	clientID = strings.TrimSpace(clientID)
	agg := evtstream.OAuthClientAggregate(clientID)
	for attempt := 0; attempt < 5; attempt++ {
		seq, err := c.EventPublisher.LastSubjectSeq(ctx, agg.AllEventsFilter())
		if err != nil {
			return OAuthClientState{}, err
		}
		if err := c.oauthClientModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.AllEventsFilter(), seq)); err != nil {
			return OAuthClientState{}, err
		}
		state, ok := c.oauthClientModel.projection.Projection().get(clientID)
		if !ok {
			return OAuthClientState{}, ErrNotFound
		}
		if !oauthClientPolicySupported(state.Policy) {
			return OAuthClientState{}, ErrInvalidArgument
		}
		if state.Policy == policy {
			return state, nil
		}
		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_OauthClientPolicyChanged{
			OauthClientPolicyChanged: &evtv1.OAuthClientPolicyChangedEvent{ClientId: clientID, Policy: policy},
		}})
		published, err := c.EventPublisher.AppendAtFilter(ctx, agg.SubjectFor(event), event, agg.AllEventsFilter(), seq)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return OAuthClientState{}, err
		}
		if err := c.oauthClientModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.SubjectFor(event), published)); err != nil {
			return OAuthClientState{}, err
		}
		state, _ = c.oauthClientModel.projection.Projection().get(clientID)
		if policy == evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
			if _, err := c.RevokeOAuthClientTokens(ctx, clientID); err != nil {
				c.logger.Warn("OAuth client blocked but token cleanup was incomplete", "error", err)
			}
		}
		return state, nil
	}
	return OAuthClientState{}, errOAuthClientMutationRetryExhausted
}
