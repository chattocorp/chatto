package exporter

import (
	"context"

	"hmans.de/chatto/internal/runtimeunit"
)

// Unit runs the deployment-wide Prometheus exporter as a Chatto runtime unit.
type Unit struct{}

func (Unit) Name() string {
	return "exporter"
}

func (Unit) Run(ctx context.Context, env runtimeunit.Env) error {
	server, err := New(Options{
		Config:  env.Config.Exporter,
		Core:    env.Config.Core,
		NC:      env.NC,
		JS:      env.JS,
		Version: env.Version,
		Logger:  env.Logger,
	})
	if err != nil {
		return err
	}
	return server.Run(ctx)
}

var _ runtimeunit.Unit = Unit{}
