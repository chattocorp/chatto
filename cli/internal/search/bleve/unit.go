package bleve

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/internal/search"
)

// Unit runs the bundled Bleve provider either under chatto run or standalone.
type Unit struct{}

func (Unit) Name() string { return "search-provider" }

func (Unit) Run(ctx context.Context, env runtimeunit.Env) error {
	evt, err := env.JS.Stream(ctx, "EVT")
	if err != nil {
		return fmt.Errorf("open EVT stream: %w", err)
	}
	encryptionKeys, err := env.JS.KeyValue(ctx, "ENCRYPTION_KEYS")
	if err != nil {
		return fmt.Errorf("open ENCRYPTION_KEYS bucket: %w", err)
	}
	runtimeState, err := env.JS.KeyValue(ctx, "RUNTIME_STATE")
	if err != nil {
		return fmt.Errorf("open RUNTIME_STATE bucket: %w", err)
	}
	keyStore := kms.NewBuiltin(encryptionKeys, env.Logger)
	projection, err := NewProjection(
		env.Config.SearchProvider.DirectoryOrDefault(),
		env.Config.SearchProvider.LanguagesOrDefault(),
		keyStore,
		keyStore,
		dekstore.New(runtimeState, env.Logger),
		env.Logger,
	)
	if err != nil {
		return err
	}
	defer projection.Close()

	projector := events.NewProjector(env.JS, evt, projection, env.Logger)
	if err := projector.ConfigureCheckpoint("message_search"); err != nil {
		return err
	}
	provider := &Provider{Projection: projection, Projector: projector}
	service, err := search.AddService(ctx, env.NC, provider, search.ServiceOptions{ImplementationVersion: env.Version})
	if err != nil {
		return fmt.Errorf("register search provider service: %w", err)
	}
	defer service.Stop()

	err = projector.Run(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

var _ runtimeunit.Unit = Unit{}
