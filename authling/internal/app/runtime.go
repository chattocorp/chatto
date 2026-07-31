// Package app composes Authling's standalone runtime.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
	"hmans.de/chatto/pkg/events"
)

// Runtime owns Authling's NATS connection, event stream, projections, and
// domain services.
type Runtime struct {
	connection *natsruntime.Connection
	projector  *events.Projector

	// Accounts is Authling's account command and read boundary.
	Accounts *accounts.Service
}

// New creates Authling's storage and model wiring without starting background
// projection consumption.
func New(
	ctx context.Context,
	cfg config.Config,
	logger events.Logger,
) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, fmt.Errorf("event logger is required")
	}
	connection, err := natsruntime.Open(ctx, cfg.NATS)
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*Runtime, error) {
		if closeErr := connection.Close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}

	js, stream, err := storage.Open(ctx, connection.NATS, cfg.NATS.ReplicasOrDefault())
	if err != nil {
		return closeOnError(err)
	}
	eventLog := events.NewEncodedEventLog(js, stream, logger)
	publisher := evtstream.NewPublisher(eventLog)
	projection := &accounts.Projection{}
	handle := events.NewDecodedProjectionHandle(
		js,
		stream,
		projection,
		evtstream.Decode,
		logger,
	)
	return &Runtime{
		connection: connection,
		projector:  handle.Projector(),
		Accounts:   accounts.NewService(publisher, handle),
	}, nil
}

// Run starts Authling's required projection lifecycle and blocks until the
// context ends or the projection fails.
func (r *Runtime) Run(ctx context.Context) error {
	return r.projector.Run(ctx)
}

// WaitReady blocks until every required model has replayed its startup
// history.
func (r *Runtime) WaitReady(ctx context.Context) error {
	return r.projector.WaitForStartup(ctx)
}

// Close releases Authling's NATS client and any embedded server. Run must have
// returned before Close is called.
func (r *Runtime) Close() error {
	return r.connection.Close()
}

// Serve runs the standalone Authling process until the context is cancelled.
func Serve(ctx context.Context, cfg config.Config, logger *slog.Logger) (serveErr error) {
	if logger == nil {
		return fmt.Errorf("logger is required")
	}
	eventLogger := logging.Events{Logger: logger}
	runtime, err := New(ctx, cfg, eventLogger)
	if err != nil {
		return err
	}
	defer func() {
		serveErr = errors.Join(serveErr, runtime.Close())
	}()

	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	runErrors := make(chan error, 1)
	go func() {
		runErrors <- runtime.Run(runContext)
	}()

	if err := runtime.WaitReady(ctx); err != nil {
		cancel()
		<-runErrors
		return fmt.Errorf("wait for Authling readiness: %w", err)
	}

	listener, err := net.Listen("tcp", cfg.HTTP.BindAddressOrDefault())
	if err != nil {
		cancel()
		<-runErrors
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	httpServer := &http.Server{
		Handler:           web.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	httpErrors := make(chan error, 1)
	go func() {
		httpErrors <- httpServer.Serve(listener)
	}()
	logger.Info("Authling is ready", "http_address", listener.Addr().String())

	select {
	case <-ctx.Done():
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		shutdownErr := httpServer.Shutdown(shutdownContext)
		shutdownCancel()
		httpErr := <-httpErrors
		cancel()
		runErr := <-runErrors
		if errors.Is(httpErr, http.ErrServerClosed) {
			httpErr = nil
		}
		if errors.Is(runErr, context.Canceled) {
			runErr = nil
		}
		return errors.Join(shutdownErr, httpErr, runErr)
	case runErr := <-runErrors:
		closeErr := httpServer.Close()
		httpErr := <-httpErrors
		if errors.Is(httpErr, http.ErrServerClosed) {
			httpErr = nil
		}
		return errors.Join(runErr, closeErr, httpErr)
	case httpErr := <-httpErrors:
		cancel()
		runErr := <-runErrors
		if errors.Is(runErr, context.Canceled) {
			runErr = nil
		}
		return errors.Join(httpErr, runErr)
	}
}
