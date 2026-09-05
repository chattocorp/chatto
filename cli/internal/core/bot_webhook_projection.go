package core

import (
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// botWebhookProjection retains encrypted settings and one latest terminal
// failure per configured bot. Successful and skipped jobs are acknowledged
// without an EVT fact; later successes do not clear the latest failure.
type botWebhookProjection struct {
	events.MemoryProjection
	configurations map[string]*evtv1.Event
	sequences      map[string]uint64
	latest         map[string]*evtv1.Event
}

func newBotWebhookProjection() *botWebhookProjection {
	return &botWebhookProjection{configurations: map[string]*evtv1.Event{}, sequences: map[string]uint64{}, latest: map[string]*evtv1.Event{}}
}
func (p *botWebhookProjection) Subjects() []string {
	return []string{evtstream.UserEventTypeFilter("bot_outbound_webhook_configured"), evtstream.UserEventTypeFilter(evtstream.EventUserAccountDeleted), "evt.bot_webhook_delivery.*.bot_webhook_delivery_completed"}
}
func (p *botWebhookProjection) Apply(event *evtv1.Event, seq uint64) error {
	p.Lock()
	defer p.Unlock()
	switch x := event.GetEvent().(type) {
	case *evtv1.Event_BotOutboundWebhookConfigured:
		id := x.BotOutboundWebhookConfigured.GetBotUserId()
		if seq <= p.sequences[id] {
			return nil
		}
		p.sequences[id] = seq
		delete(p.latest, id)
		if x.BotOutboundWebhookConfigured.GetCredentials() == nil {
			delete(p.configurations, id)
		} else {
			p.configurations[id] = proto.Clone(event).(*evtv1.Event)
		}
	case *evtv1.Event_BotWebhookDeliveryCompleted:
		id := x.BotWebhookDeliveryCompleted.GetBotUserId()
		if p.configurations[id].GetBotOutboundWebhookConfigured().GetWebhookId() == x.BotWebhookDeliveryCompleted.GetWebhookId() {
			p.latest[id] = proto.Clone(event).(*evtv1.Event)
		}
	case *evtv1.Event_UserAccountDeleted:
		id := x.UserAccountDeleted.GetUserId()
		delete(p.configurations, id)
		delete(p.latest, id)
		delete(p.sequences, id)
	}
	return nil
}
func (p *botWebhookProjection) get(id string) (*evtv1.Event, uint64, *evtv1.Event) {
	p.RLock()
	defer p.RUnlock()
	return cloneWebhookEvent(p.configurations[id]), p.sequences[id], cloneWebhookEvent(p.latest[id])
}
func cloneWebhookEvent(e *evtv1.Event) *evtv1.Event {
	if e == nil {
		return nil
	}
	return proto.Clone(e).(*evtv1.Event)
}
func (p *botWebhookProjection) activeBefore(seq uint64) []string {
	p.RLock()
	defer p.RUnlock()
	var ids []string
	for id, e := range p.configurations {
		if e.GetBotOutboundWebhookConfigured().GetEnabled() && p.sequences[id] < seq {
			ids = append(ids, id)
		}
	}
	return ids
}
func (p *botWebhookProjection) estimate() (int64, int64, []ProjectionAdminMetric) {
	p.RLock()
	defer p.RUnlock()
	var size int64
	for _, e := range p.configurations {
		size += int64(proto.Size(e))
	}
	for _, e := range p.latest {
		size += int64(proto.Size(e))
	}
	return int64(len(p.configurations)), size, nil
}
