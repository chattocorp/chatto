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

	"golang.org/x/sync/errgroup"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authentication"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/emailchange"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/issuer"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/oidcprovider"
	"hmans.de/authling/internal/passwordreset"
	"hmans.de/authling/internal/registration"
	"hmans.de/authling/internal/sessions"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
	"hmans.de/chatto/pkg/events"
)

// Runtime owns Authling's NATS connection, event stream, projections, and
// domain services.
type Runtime struct {
	connection *natsruntime.Connection
	projectors []*events.Projector
	issuer     *issuer.Service

	// Accounts is Authling's account command and read boundary.
	Accounts *accounts.Service
	// Registration owns the verified-email signup workflow.
	Registration *registration.Service
	// PasswordReset owns verified-email password recovery.
	PasswordReset *passwordreset.Service
	// EmailChange owns signed-in verified email-address replacement.
	EmailChange *emailchange.Service
	// Authentication owns local login throttling and credential verification.
	Authentication *authentication.Service
	// Sessions owns first-party browser session runtime state.
	Sessions *sessions.Service
	// OIDC provides standards-based identity to configured and CIMD clients.
	OIDC *oidcprovider.Service
}

// New creates Authling's storage and model wiring without starting background
// projection consumption.
func New(
	ctx context.Context,
	cfg config.Config,
	logger events.Logger,
) (*Runtime, error) {
	return newRuntime(ctx, cfg, logger, email.NewMailer(cfg.SMTP))
}

func newRuntime(ctx context.Context, cfg config.Config, logger events.Logger, sender email.Sender) (*Runtime, error) {
	return newRuntimeWithEmailChangeOptions(ctx, cfg, logger, sender)
}

func newRuntimeWithEmailChangeOptions(ctx context.Context, cfg config.Config, logger events.Logger, sender email.Sender, emailChangeOptions ...emailchange.Option) (*Runtime, error) {
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
	stores, err := storage.OpenStores(ctx, js, cfg.NATS.ReplicasOrDefault())
	if err != nil {
		return closeOnError(err)
	}
	vault := keyvault.New(stores.Keys)
	workflowKey, err := vault.WorkflowKey(ctx)
	if err != nil {
		return closeOnError(fmt.Errorf("open workflow key: %w", err))
	}
	publisher := evtstream.NewPublisher(eventLog)
	projection := accounts.NewProjection(vault, workflowKey)
	handle := events.NewDecodedProjectionHandle(
		js,
		stream,
		projection,
		evtstream.Decode,
		logger,
	)
	accountService, err := accounts.NewService(ctx, publisher, handle, vault, cfg.Authentication.PasswordMinimumLengthOrDefault())
	if err != nil {
		return closeOnError(fmt.Errorf("open account service: %w", err))
	}
	sessionService := sessions.New(stores.RuntimeState, js, workflowKey, accountService.AuthenticationVersion)
	issuerProjection := issuer.NewProjection()
	issuerHandle := events.NewDecodedProjectionHandle(js, stream, issuerProjection, evtstream.Decode, logger)
	issuerService := issuer.NewService(publisher, issuerHandle, vault, cfg.HTTP.PublicURLOrDefault())
	cimd, err := oidcprovider.NewCIMDResolver(
		cfg.HTTP.PublicURLOrDefault(),
		nil,
		cfg.OIDC.TrustedPrivateCIMDHosts(),
		cfg.OIDC.TrustedLoopbackCIMDHosts(),
	)
	if err != nil {
		return closeOnError(fmt.Errorf("construct CIMD resolver: %w", err))
	}
	clients := oidcprovider.NewResolver(cfg, cimd)
	oidcStorage := oidcprovider.NewStorage(stores.RuntimeState, js, workflowKey, clients, issuerService)
	oidcService := oidcprovider.New(cfg, issuerService, oidcStorage)
	authenticationService := authentication.New(stores.RuntimeState, js, workflowKey, accountService)
	return &Runtime{
		connection:     connection,
		projectors:     []*events.Projector{handle.Projector(), issuerHandle.Projector()},
		issuer:         issuerService,
		Accounts:       accountService,
		Registration:   registration.New(stores.RuntimeState, js, workflowKey, sender, accountService),
		PasswordReset:  passwordreset.New(stores.RuntimeState, js, workflowKey, sender, accountService),
		EmailChange:    emailchange.New(stores.RuntimeState, js, workflowKey, sender, accountService, authenticationService, emailChangeOptions...),
		Authentication: authenticationService,
		Sessions:       sessionService,
		OIDC:           oidcService,
	}, nil
}

// Run starts Authling's required projection lifecycle and blocks until the
// context ends or the projection fails.
func (r *Runtime) Run(ctx context.Context) error {
	group, groupContext := errgroup.WithContext(ctx)
	for _, projector := range r.projectors {
		projector := projector
		group.Go(func() error { return projector.Run(groupContext) })
	}
	group.Go(func() error { return r.Sessions.RunInventory(groupContext) })
	return group.Wait()
}

// WaitReady blocks until every required model has replayed its startup
// history.
func (r *Runtime) WaitReady(ctx context.Context) error {
	for _, projector := range r.projectors {
		if err := projector.WaitForStartup(ctx); err != nil {
			return err
		}
	}
	if err := r.Sessions.WaitForInventoryStartup(ctx); err != nil {
		return err
	}
	if err := r.issuer.Initialize(ctx); err != nil {
		return err
	}
	return r.OIDC.Initialize()
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
		Handler: web.Handler(web.Dependencies{
			Accounts:          runtime.Accounts,
			Authentication:    runtime.Authentication,
			Registration:      runtime.Registration,
			PasswordReset:     runtime.PasswordReset,
			EmailChange:       runtime.EmailChange,
			Sessions:          runtime.Sessions,
			OIDC:              runtime.OIDC,
			SecureCookies:     cfg.HTTP.SecureCookies(),
			PublicURL:         cfg.HTTP.PublicURLOrDefault(),
			TrustProxyHeaders: cfg.HTTP.TrustProxyHeaders,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Minute,
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
