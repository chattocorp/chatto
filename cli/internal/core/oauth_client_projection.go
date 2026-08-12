package core

import (
	"sort"
	"time"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

// OAuthClientState is the durable administrator-visible state of one public
// client that completed a successful user authorization.
type OAuthClientState struct {
	ClientID             string
	ClientName           string
	ClientOrigin         string
	Source               corev1.OAuthClientSource
	Policy               corev1.OAuthClientPolicy
	FirstAuthorizationAt time.Time
	LastAuthorizationAt  time.Time
	RedirectOrigins      []string
	AuthorizedUserCount  uint32
	// Clients are never deleted, so their first EVT sequence provides an
	// append-stable order for the administration API's offset pagination.
	firstAuthorizationSeq uint64
	authorizedUsers       map[string]struct{}
}

type OAuthClientProjection struct {
	events.MemoryProjection
	clients map[string]*OAuthClientState
}

func NewOAuthClientProjection() *OAuthClientProjection {
	return &OAuthClientProjection{clients: make(map[string]*OAuthClientState)}
}

func (p *OAuthClientProjection) Subjects() []string {
	return []string{evtstream.OAuthClientSubjectFilter()}
}

func (p *OAuthClientProjection) Apply(event *corev1.Event, sequence uint64) error {
	if event == nil {
		return nil
	}
	p.Lock()
	defer p.Unlock()
	switch e := event.GetEvent().(type) {
	case *corev1.Event_OauthClientAuthorizationRecorded:
		payload := e.OauthClientAuthorizationRecorded
		clientID := payload.GetClientId()
		if clientID == "" {
			return nil
		}
		state := p.clients[clientID]
		authorizedAt := event.GetCreatedAt().AsTime()
		if state == nil {
			state = &OAuthClientState{
				ClientID:              clientID,
				Policy:                corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT,
				FirstAuthorizationAt:  authorizedAt,
				firstAuthorizationSeq: sequence,
				authorizedUsers:       make(map[string]struct{}),
			}
			p.clients[clientID] = state
		}
		state.ClientName = payload.GetClientName()
		state.ClientOrigin = payload.GetClientUri()
		state.Source = payload.GetSource()
		state.LastAuthorizationAt = authorizedAt
		if origin := payload.GetRedirectOrigin(); origin != "" && !containsString(state.RedirectOrigins, origin) {
			state.RedirectOrigins = append(state.RedirectOrigins, origin)
			sort.Strings(state.RedirectOrigins)
		}
		if actorID := event.GetActorId(); actorID != "" {
			state.authorizedUsers[actorID] = struct{}{}
			state.AuthorizedUserCount = uint32(len(state.authorizedUsers))
		}
	case *corev1.Event_OauthClientPolicyChanged:
		payload := e.OauthClientPolicyChanged
		if state := p.clients[payload.GetClientId()]; state != nil {
			state.Policy = payload.GetPolicy()
		}
	}
	return nil
}

func (p *OAuthClientProjection) get(clientID string) (OAuthClientState, bool) {
	p.RLock()
	defer p.RUnlock()
	state, ok := p.clients[clientID]
	if !ok {
		return OAuthClientState{}, false
	}
	return cloneOAuthClientState(state), true
}

func (p *OAuthClientProjection) all() []OAuthClientState {
	p.RLock()
	defer p.RUnlock()
	result := make([]OAuthClientState, 0, len(p.clients))
	for _, state := range p.clients {
		result = append(result, cloneOAuthClientState(state))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].firstAuthorizationSeq < result[j].firstAuthorizationSeq
	})
	return result
}

func cloneOAuthClientState(state *OAuthClientState) OAuthClientState {
	clone := *state
	clone.RedirectOrigins = append([]string(nil), state.RedirectOrigins...)
	clone.authorizedUsers = nil
	return clone
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (p *OAuthClientProjection) adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var estimatedBytes int64
	for _, state := range p.clients {
		estimatedBytes += int64(len(state.ClientID) + len(state.ClientName) + len(state.ClientOrigin) + 64)
		for _, origin := range state.RedirectOrigins {
			estimatedBytes += int64(len(origin))
		}
		estimatedBytes += int64(len(state.authorizedUsers) * 32)
	}
	return int64(len(p.clients)), estimatedBytes, nil
}
