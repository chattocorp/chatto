package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	runtimestatev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
	"hmans.de/chatto/pkg/events"
)

// syncPreference reads only the latest RUNTIME_STATE choice before a privacy
// decision. Neither choices nor changes are written to the durable event log.
func (s *PresenceModel) syncPreference(ctx context.Context, userID string) (uint64, error) {
	if !validPresenceUserID(userID) {
		return 0, nil
	}
	entry, err := s.readPreferenceRecord(ctx, presenceKey(userID))
	if errors.Is(err, nats.ErrMsgNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if entry.Header.Get("KV-Operation") != "" {
		s.hub.applyPreference(userID, nil, entry.Sequence)
		return 0, nil
	}
	var value runtimestatev1.PresencePreference
	if err := proto.Unmarshal(entry.Data, &value); err != nil {
		return 0, fmt.Errorf("decode current presence choice: %w", err)
	}
	s.hub.applyPreference(userID, &apiv1.PresencePreference{Mode: apiv1.PresenceMode(value.Mode), Revision: value.Revision}, entry.Sequence)
	return entry.Sequence, nil
}

// readPreferenceRecord deliberately uses the leader-routed management read.
// The newer KV/Stream Get helpers use DirectGet when available, which can read
// a lagging NATS replica. Privacy decisions must not resurrect an older mode.
func (s *PresenceModel) readPreferenceRecord(ctx context.Context, key string) (*nats.RawStreamMsg, error) {
	opts := s.js.Options()
	options := []nats.JSOpt{nats.MaxWait(opts.DefaultTimeout)}
	if opts.Domain != "" {
		options = append(options, nats.Domain(opts.Domain))
	} else if opts.APIPrefix != "" {
		options = append(options, nats.APIPrefix(opts.APIPrefix))
	}
	reader, err := s.js.Conn().JetStream(options...)
	if err != nil {
		return nil, err
	}
	return reader.GetLastMsg("KV_RUNTIME_STATE", "$KV.RUNTIME_STATE."+key, nats.Context(ctx))
}

// waitPreferencesCurrent uses one shared watcher barrier for a bulk read.
// It does not issue a separate KV request for every hydrated user.
func (s *PresenceModel) waitPreferencesCurrent(ctx context.Context) error {
	last, err := s.readPreferenceRecord(ctx, "presence.>")
	if errors.Is(err, nats.ErrMsgNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	for {
		s.hub.mu.Lock()
		seen, changed := s.hub.preferenceWatchRevision, s.hub.preferenceChanged
		s.hub.mu.Unlock()
		if seen >= last.Sequence {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// GetPresencePreference returns the private saved choice. Callers must enforce
// self-only authorization. Missing choices are left absent for safe migration.
func (c *ChattoCore) GetPresencePreference(ctx context.Context, userID string) (*apiv1.PresencePreference, error) {
	if _, err := c.presenceModel.syncPreference(ctx, userID); err != nil {
		return nil, err
	}
	return c.presenceModel.hub.preference(userID), nil
}

// forgetUser removes the current private choice on account deletion. The KV
// purge also removes the stored value; watchers clear their local copy.
func (s *PresenceModel) forgetUser(ctx context.Context, userID string) error {
	return s.runtimeStateKV.Purge(ctx, presenceKey(userID))
}

// SetPresencePreference commits an explicit, revision-checked selection. It
// does not retry stale user intent. Heartbeats never call this operation.
func (c *ChattoCore) SetPresencePreference(ctx context.Context, userID string, mode apiv1.PresenceMode, revision string) (*apiv1.PresencePreference, error) {
	if !validPresenceUserID(userID) || mode < apiv1.PresenceMode_PRESENCE_MODE_ONLINE || mode > apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE {
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
	value := &runtimestatev1.PresencePreference{Mode: runtimestatev1.PresenceMode(mode), Revision: newID("P")}
	data, err := proto.Marshal(value)
	if err != nil {
		return nil, err
	}
	key := presenceKey(userID)
	var saved uint64
	if seq == 0 {
		saved, err = s.runtimeStateKV.Create(ctx, key, data)
	} else {
		saved, err = s.runtimeStateKV.Update(ctx, key, data, seq)
	}
	if errors.Is(err, jetstream.ErrKeyExists) {
		return nil, events.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	s.hub.applyPreference(userID, &apiv1.PresencePreference{Mode: mode, Revision: value.Revision}, saved)
	// This signal is private, transient, and contains no choice. A lost signal
	// is recovered by the next authoritative heartbeat read.
	event := newPubSubEvent(userID, &pubsubv1.PubSubEvent{Event: &pubsubv1.PubSubEvent_ViewerPresencePreferenceChanged{ViewerPresencePreferenceChanged: &realtimev1.ViewerPresencePreferenceChangedEvent{}}})
	if err := c.publishUserPubSubEvent(ctx, userID, event); err != nil {
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
