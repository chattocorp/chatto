package core

import (
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// botWebhookEndpoint retains the current encrypted configuration and original
// creation time separately from delivery state. Sequence is the activation cutoff:
// edits and resume never replay work from before the latest change. Each encrypted
// configuration retains its event identity for authenticated decryption.
type botWebhookEndpoint struct {
	Configuration *evtv1.Event
	CreatedAt     time.Time // First creation, preserved when destination credentials change.
	Sequence      uint64
	Enabled       bool
}

// botWebhookProjection retains encrypted endpoints.
type botWebhookProjection struct {
	events.MemoryProjection
	endpoints map[string]*botWebhookEndpoint
}

func newBotWebhookProjection() *botWebhookProjection {
	return &botWebhookProjection{endpoints: map[string]*botWebhookEndpoint{}}
}
func (p *botWebhookProjection) Subjects() []string {
	return []string{evtstream.UserEventTypeFilter("bot_outbound_webhook_configured"), evtstream.UserEventTypeFilter("bot_outbound_webhook_updated"), evtstream.UserEventTypeFilter("bot_outbound_webhook_revoked"), evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted)}
}
func (p *botWebhookProjection) Apply(event *evtv1.Event, seq uint64) error {
	p.Lock()
	defer p.Unlock()
	switch x := event.GetEvent().(type) {
	case *evtv1.Event_BotOutboundWebhookConfigured:
		cfg := x.BotOutboundWebhookConfigured
		if cfg.GetCredentials() != nil {
			createdAt := event.GetCreatedAt().AsTime()
			if existing := p.endpoints[cfg.GetWebhookId()]; existing != nil {
				createdAt = existing.CreatedAt
			}
			p.endpoints[cfg.GetWebhookId()] = &botWebhookEndpoint{Configuration: cloneWebhookEvent(event), CreatedAt: createdAt, Sequence: seq, Enabled: cfg.GetEnabled()}
		}
	case *evtv1.Event_BotOutboundWebhookUpdated:
		state := x.BotOutboundWebhookUpdated
		endpoint := p.endpoints[state.GetWebhookId()]
		if endpoint == nil || endpoint.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() != state.GetBotUserId() {
			return nil
		}
		if endpoint.Enabled != state.GetEnabled() {
			endpoint.Enabled = state.GetEnabled()
			endpoint.Sequence = seq
		}
	case *evtv1.Event_BotOutboundWebhookRevoked:
		revoked := x.BotOutboundWebhookRevoked
		endpoint := p.endpoints[revoked.GetWebhookId()]
		if endpoint != nil && endpoint.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() == revoked.GetBotUserId() {
			delete(p.endpoints, revoked.GetWebhookId())
		}
	case *evtv1.Event_UserAccountDeleted:
		id := x.UserAccountDeleted.GetUserId()
		for webhookID, endpoint := range p.endpoints {
			if endpoint.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() == id {
				delete(p.endpoints, webhookID)
			}
		}
	}
	return nil
}
func cloneWebhookEvent(e *evtv1.Event) *evtv1.Event {
	if e == nil {
		return nil
	}
	return proto.Clone(e).(*evtv1.Event)
}
func cloneWebhookEndpoint(e *botWebhookEndpoint) *botWebhookEndpoint {
	if e == nil {
		return nil
	}
	return &botWebhookEndpoint{Configuration: cloneWebhookEvent(e.Configuration), CreatedAt: e.CreatedAt, Sequence: e.Sequence, Enabled: e.Enabled}
}
func (p *botWebhookProjection) get(botID, webhookID string) *botWebhookEndpoint {
	p.RLock()
	defer p.RUnlock()
	e := p.endpoints[webhookID]
	if e == nil || e.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() != botID {
		return nil
	}
	return cloneWebhookEndpoint(e)
}
func (p *botWebhookProjection) list(botID string) []*botWebhookEndpoint {
	p.RLock()
	defer p.RUnlock()
	result := []*botWebhookEndpoint{}
	for _, e := range p.endpoints {
		if e.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() == botID {
			result = append(result, cloneWebhookEndpoint(e))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Configuration.GetId() < result[j].Configuration.GetId() })
	return result
}
func (p *botWebhookProjection) activeBefore(seq uint64) []*botWebhookEndpoint {
	p.RLock()
	defer p.RUnlock()
	var endpoints []*botWebhookEndpoint
	for _, e := range p.endpoints {
		if e.Enabled && e.Sequence < seq {
			endpoints = append(endpoints, cloneWebhookEndpoint(e))
		}
	}
	return endpoints
}
func (p *botWebhookProjection) estimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var size int64
	for _, e := range p.endpoints {
		size += int64(proto.Size(e.Configuration))
	}
	return int64(len(p.endpoints)), size, nil
}
