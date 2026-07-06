package core

import (
	"context"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
)

// PresenceModel owns live presence state and the per-process presence hub.
type PresenceModel struct {
	js            jetstream.JetStream
	memoryCacheKV jetstream.KeyValue
	logger        *log.Logger
	hub           *PresenceHub
}

func NewPresenceModel(js jetstream.JetStream, memoryCacheKV jetstream.KeyValue, logger *log.Logger) *PresenceModel {
	return &PresenceModel{
		js:            js,
		memoryCacheKV: memoryCacheKV,
		logger:        logger,
		hub:           NewPresenceHub(memoryCacheKV, logger),
	}
}

func (s *PresenceModel) Run(ctx context.Context) error {
	return s.hub.Run(ctx)
}

func (s *PresenceModel) Subscribe(ctx context.Context) (*PresenceSubscription, error) {
	return s.hub.Subscribe(ctx)
}

func (s *PresenceModel) Unsubscribe(sub *PresenceSubscription) {
	s.hub.Unsubscribe(sub)
}
