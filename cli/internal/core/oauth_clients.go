package core

import (
	"context"
	"errors"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

var (
	ErrOAuthClientBlocked                = errors.New("OAuth client is blocked")
	errOAuthClientMutationRetryExhausted = errors.New("OAuth client mutation OCC retry exhausted")
)

type OAuthClientModel struct {
	projection events.ProjectionHandle[*OAuthClientProjection]
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
	if c.oauthClientBlocked(clientID) {
		return ErrOAuthClientBlocked
	}
	return nil
}

func (c *ChattoCore) oauthClientBlocked(clientID string) bool {
	if c.oauthClientModel == nil || clientID == "" {
		return false
	}
	state, ok := c.oauthClientModel.projection.Projection().get(clientID)
	return ok && state.Policy == corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED
}

// ObserveOAuthClient records a client after a successful user authorization.
// Every authorization advances the durable last-observed timestamp; the
// projection de-duplicates callback origins and authorizing users.
func (c *ChattoCore) ObserveOAuthClient(ctx context.Context, actorID, clientID, clientName, clientURI, redirectOrigin string, source corev1.OAuthClientSource) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return ErrInvalidArgument
	}
	if source != corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD && source != corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_BUILT_IN {
		return ErrInvalidArgument
	}
	agg := evtstream.OAuthClientAggregate(clientID)
	for attempt := 0; attempt < 5; attempt++ {
		seq, err := c.EventPublisher.LastSubjectSeq(ctx, agg.AllEventsFilter())
		if err != nil {
			return err
		}
		if err := c.oauthClientModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.AllEventsFilter(), seq)); err != nil {
			return err
		}
		state, exists := c.oauthClientModel.projection.Projection().get(clientID)
		origin := OAuthConsentOrigin(redirectOrigin)
		if exists && state.Policy == corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
			return ErrOAuthClientBlocked
		}
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_OauthClientObserved{
			OauthClientObserved: &corev1.OAuthClientObservedEvent{
				ClientId: clientID, ClientName: strings.TrimSpace(clientName), ClientUri: strings.TrimSpace(clientURI),
				RedirectOrigin: origin, Source: source,
			},
		}})
		published, err := c.EventPublisher.AppendAtFilter(ctx, agg.SubjectFor(event), event, agg.AllEventsFilter(), seq)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return c.oauthClientModel.projection.Projector().WaitFor(ctx, events.SubjectPosition(agg.SubjectFor(event), published))
	}
	return errOAuthClientMutationRetryExhausted
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

func (c *ChattoCore) UpdateOAuthClientPolicy(ctx context.Context, actorID, clientID string, policy corev1.OAuthClientPolicy) (OAuthClientState, error) {
	if err := c.requireServerPermission(ctx, actorID, PermServerManage); err != nil {
		return OAuthClientState{}, err
	}
	if policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT && policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED && policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
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
		if state.Policy == policy {
			return state, nil
		}
		event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_OauthClientPolicyChanged{
			OauthClientPolicyChanged: &corev1.OAuthClientPolicyChangedEvent{ClientId: clientID, Policy: policy},
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
		if policy == corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
			if _, err := c.RevokeOAuthClientTokens(ctx, clientID); err != nil {
				c.logger.Warn("OAuth client blocked but token cleanup was incomplete", "error", err)
			}
		}
		return state, nil
	}
	return OAuthClientState{}, errOAuthClientMutationRetryExhausted
}
