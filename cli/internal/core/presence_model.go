package core

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

// PresenceModel owns live presence state and the per-process presence hub.
type PresenceModel struct {
	js             jetstream.JetStream
	memoryCacheKV  jetstream.KeyValue
	runtimeStateKV jetstream.KeyValue
	logger         *log.Logger
	hub            *PresenceHub
	putWithTTL     func(context.Context, string, []byte, uint64) (uint64, error)
}

// NewPresenceModel uses the existing shared buckets: liveness in MEMORY_CACHE
// and one current private choice per user in RUNTIME_STATE. Run starts their
// watchers; no presence projection or event-log writer is installed.
func NewPresenceModel(js jetstream.JetStream, memoryCacheKV, runtimeStateKV jetstream.KeyValue, logger *log.Logger) *PresenceModel {
	model := &PresenceModel{
		js:             js,
		memoryCacheKV:  memoryCacheKV,
		runtimeStateKV: runtimeStateKV,
		logger:         logger,
		hub:            NewPresenceHub(memoryCacheKV, runtimeStateKV, logger),
	}
	model.putWithTTL = model.putPresenceWithTTL
	model.hub.beforeLiveRead = func(ctx context.Context, userID string) error {
		_, err := model.syncPreference(ctx, userID)
		return err
	}
	return model
}

func (s *PresenceModel) Run(ctx context.Context) error {
	return s.hub.Run(ctx)
}

// Resync replaces the presence watcher and waits for a current snapshot.
func (s *PresenceModel) Resync(ctx context.Context) error {
	return s.hub.Resync(ctx)
}

func (s *PresenceModel) Subscribe(ctx context.Context) (*PresenceSubscription, error) {
	return s.hub.Subscribe(ctx)
}

// GetUserPresences returns watcher-backed presence for bulk read hydration.
func (s *PresenceModel) GetUserPresences(ctx context.Context, userIDs []string) (map[string]string, error) {
	if err := s.waitPreferencesCurrent(ctx); err != nil {
		return nil, err
	}
	return s.hub.GetUserPresences(ctx, userIDs)
}

func (s *PresenceModel) Unsubscribe(sub *PresenceSubscription) {
	s.hub.Unsubscribe(sub)
}

// LivePresenceCount returns the number of users with any current live presence
// record, including Online, Away, and Do Not Disturb.
func (s *PresenceModel) LivePresenceCount(ctx context.Context) (int, error) {
	if err := s.waitPreferencesCurrent(ctx); err != nil {
		return 0, err
	}
	return s.hub.LivePresenceCount(ctx)
}
