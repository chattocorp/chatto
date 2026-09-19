package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/evtstream"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// syncPreference waits for the authoritative account history before making a
// privacy decision. A lagging replica must not resurrect a hidden account.
func (s *PresenceModel) syncPreference(ctx context.Context, userID string) (uint64, error) {
	if s.publisher == nil {
		return 0, nil
	} // isolated live-presence fixtures
	filter := evtstream.ConfigSubjectAggregate(userID).AllEventsFilter()
	seq, err := s.publisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		return 0, err
	}
	if seq > 0 {
		err = s.preferences.Projector().WaitFor(ctx, events.SubjectPosition(filter, seq))
	}
	return seq, err
}

// GetPresencePreference returns the private saved choice. Callers must enforce
// self-only authorization. Missing choices are left absent for safe migration.
func (c *ChattoCore) GetPresencePreference(ctx context.Context, userID string) (*apiv1.PresencePreference, error) {
	if _, err := c.presenceModel.syncPreference(ctx, userID); err != nil {
		return nil, err
	}
	return c.presenceModel.hub.preference(userID), nil
}

// SetPresencePreference commits an explicit, revision-checked selection. It
// does not retry stale user intent. Heartbeats never call this operation.
func (c *ChattoCore) SetPresencePreference(ctx context.Context, userID string, mode apiv1.PresenceMode, revision string) (*apiv1.PresencePreference, error) {
	if mode < apiv1.PresenceMode_PRESENCE_MODE_ONLINE || mode > apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE {
		return nil, fmt.Errorf("invalid presence mode")
	}
	s := c.presenceModel
	seq, err := s.syncPreference(ctx, userID)
	if err != nil {
		return nil, err
	}
	current := s.hub.preference(userID)
	if current.GetRevision() != revision {
		return nil, events.ErrConflict
	}
	if current != nil && current.Mode == mode {
		return current, nil
	}
	agg := evtstream.ConfigSubjectAggregate(userID)
	event := newEvent(userID, &evtv1.Event{Event: &evtv1.Event_UserPresencePreferenceChanged{UserPresencePreferenceChanged: &evtv1.UserPresencePreferenceChangedEvent{UserId: userID, Mode: evtv1.SavedPresenceMode(mode)}}})
	seqs, err := s.publisher.AppendBatch(ctx, []evtstream.BatchEntry{{Subject: agg.SubjectFor(event), Event: event, ExpectedSeq: seq, FilterSubject: agg.AllEventsFilter(), HasOCC: true}})
	if err != nil {
		return nil, err
	}
	if err := s.preferences.Projector().WaitFor(ctx, events.SubjectPosition(agg.AllEventsFilter(), seqs[0])); err != nil {
		return nil, err
	}
	return s.hub.preference(userID), nil
}

// MayPublishTyping applies the private visibility choice to automatic activity.
func (c *ChattoCore) MayPublishTyping(ctx context.Context, userID string) (bool, error) {
	p, err := c.GetPresencePreference(ctx, userID)
	if err != nil {
		return false, err
	}
	return p == nil || (p.Mode >= apiv1.PresenceMode_PRESENCE_MODE_ONLINE && p.Mode <= apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB), nil
}

// notificationPresence preserves the saved DND policy even with no connected
// device. This private decision must never be used in public user responses.
func (c *ChattoCore) notificationPresence(ctx context.Context, userID string) (string, error) {
	p, err := c.GetPresencePreference(ctx, userID)
	if err != nil {
		return PresenceStatusOffline, err
	}
	if p.GetMode() == apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB {
		return PresenceStatusDoNotDisturb, nil
	}
	return c.GetUserPresence(ctx, userID)
}
