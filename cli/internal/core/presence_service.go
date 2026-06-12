package core

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

// PresenceService owns live presence state and the per-process presence hub.
type PresenceService struct {
	js            jetstream.JetStream
	memoryCacheKV jetstream.KeyValue
	logger        *log.Logger
	hub           *PresenceHub
}

func NewPresenceService(js jetstream.JetStream, memoryCacheKV jetstream.KeyValue, logger *log.Logger) *PresenceService {
	return &PresenceService{
		js:            js,
		memoryCacheKV: memoryCacheKV,
		logger:        logger,
		hub:           NewPresenceHub(memoryCacheKV, logger),
	}
}

func (s *PresenceService) Run(ctx context.Context) error {
	return s.hub.Run(ctx)
}

func (s *PresenceService) Subscribe(ctx context.Context) (*PresenceSubscription, error) {
	return s.hub.Subscribe(ctx)
}

func (s *PresenceService) Unsubscribe(sub *PresenceSubscription) {
	s.hub.Unsubscribe(sub)
}
