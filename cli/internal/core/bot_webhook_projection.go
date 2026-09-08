package core

import (
	"sort"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// botWebhookEndpoint keeps the immutable creation event separate from mutable
// delivery state. Sequence is the activation cutoff: resume never replays work
// from before the latest state change. Credentials retain their original AAD.
type botWebhookEndpoint struct {
	Configuration *evtv1.Event
	Sequence      uint64
	Enabled       bool
	Latest        *evtv1.Event
}

// botWebhookProjection retains encrypted endpoints and their latest failures.
// Legacy single-endpoint events still replace only the legacy slot for that bot.
type botWebhookProjection struct {
	events.MemoryProjection
	endpoints map[string]*botWebhookEndpoint
	legacy    map[string]string
}

func newBotWebhookProjection() *botWebhookProjection {
	return &botWebhookProjection{endpoints: map[string]*botWebhookEndpoint{}, legacy: map[string]string{}}
}
func (p *botWebhookProjection) Subjects() []string {
	return []string{evtstream.UserEventTypeFilter("bot_outbound_webhook_configured"), evtstream.UserEventTypeFilter("bot_outbound_webhook_state_changed"), evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted), "evt.bot_webhook_delivery.*.bot_webhook_delivery_completed"}
}
func (p *botWebhookProjection) Apply(event *evtv1.Event, seq uint64) error {
	p.Lock()
	defer p.Unlock()
	switch x := event.GetEvent().(type) {
	case *evtv1.Event_BotOutboundWebhookConfigured:
		cfg := x.BotOutboundWebhookConfigured
		if !cfg.GetIndependent() {
			delete(p.endpoints, p.legacy[cfg.GetBotUserId()])
			delete(p.legacy, cfg.GetBotUserId())
		}
		if cfg.GetCredentials() != nil {
			p.endpoints[cfg.GetWebhookId()] = &botWebhookEndpoint{Configuration: cloneWebhookEvent(event), Sequence: seq, Enabled: cfg.GetEnabled()}
			if !cfg.GetIndependent() {
				p.legacy[cfg.GetBotUserId()] = cfg.GetWebhookId()
			}
		}
	case *evtv1.Event_BotOutboundWebhookStateChanged:
		state := x.BotOutboundWebhookStateChanged
		endpoint := p.endpoints[state.GetWebhookId()]
		if endpoint == nil || endpoint.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() != state.GetBotUserId() {
			return nil
		}
		if state.GetRevoked() {
			delete(p.endpoints, state.GetWebhookId())
			if p.legacy[state.GetBotUserId()] == state.GetWebhookId() {
				delete(p.legacy, state.GetBotUserId())
			}
		} else if endpoint.Enabled != state.GetEnabled() {
			endpoint.Enabled = state.GetEnabled()
			endpoint.Sequence = seq
		}
	case *evtv1.Event_BotWebhookDeliveryCompleted:
		failure := x.BotWebhookDeliveryCompleted
		endpoint := p.endpoints[failure.GetWebhookId()]
		if endpoint != nil && endpoint.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() == failure.GetBotUserId() {
			endpoint.Latest = cloneWebhookEvent(event)
		}
	case *evtv1.Event_UserAccountDeleted:
		id := x.UserAccountDeleted.GetUserId()
		for webhookID, endpoint := range p.endpoints {
			if endpoint.Configuration.GetBotOutboundWebhookConfigured().GetBotUserId() == id {
				delete(p.endpoints, webhookID)
			}
		}
		delete(p.legacy, id)
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
	return &botWebhookEndpoint{Configuration: cloneWebhookEvent(e.Configuration), Sequence: e.Sequence, Enabled: e.Enabled, Latest: cloneWebhookEvent(e.Latest)}
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
		size += int64(proto.Size(e.Configuration) + proto.Size(e.Latest))
	}
	return int64(len(p.endpoints)), size, nil
}
