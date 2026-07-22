package runtimeunit

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/embedded_nats"
	"hmans.de/chatto/pkg/natsauth"
)

// Unit is a Chatto runtime unit that can run either as its own process or
// embedded under `chatto run` after the main process has established config and
// NATS access.
type Unit interface {
	Name() string
	Run(ctx context.Context, env Env) error
}

// Registration describes one optional unit that chatto run may compose into
// the main process. Standalone commands run the same Unit directly and ignore
// StartWithRun.
type Registration struct {
	Unit         Unit
	StartWithRun func(config.ChattoConfig) bool
}

// Enabled reports whether this registration should start under chatto run.
func (r Registration) Enabled(cfg config.ChattoConfig) bool {
	return r.Unit != nil && r.StartWithRun != nil && r.StartWithRun(cfg)
}

// ValidateRegistrations rejects incomplete or duplicate runtime-unit entries.
func ValidateRegistrations(registrations []Registration) error {
	seen := make(map[string]struct{}, len(registrations))
	for i, registration := range registrations {
		if registration.Unit == nil {
			return fmt.Errorf("runtime unit registration %d has no unit", i)
		}
		name := registration.Unit.Name()
		if name == "" {
			return fmt.Errorf("runtime unit registration %d has an empty name", i)
		}
		if registration.StartWithRun == nil {
			return fmt.Errorf("runtime unit %q has no chatto run predicate", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("runtime unit %q is registered more than once", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

// Env is the shared runtime environment given to a unit.
type Env struct {
	Config  config.ChattoConfig
	NC      *nats.Conn
	JS      jetstream.JetStream
	Logger  *log.Logger
	Version string
}

// NewEnv builds a runtime-unit environment from an established NATS connection.
func NewEnv(ctx context.Context, cfg config.ChattoConfig, nc *nats.Conn, logger *log.Logger, version string) (Env, error) {
	if nc == nil {
		return Env{}, fmt.Errorf("nats connection is required")
	}
	js, err := jetstream.New(nc, jetstream.WithDefaultTimeout(30*time.Second))
	if err != nil {
		return Env{}, fmt.Errorf("create JetStream context: %w", err)
	}
	if logger == nil {
		logger = log.WithPrefix("unit")
	}
	if version == "" {
		version = "unknown"
	}
	return Env{
		Config:  cfg,
		NC:      nc,
		JS:      js,
		Logger:  logger,
		Version: version,
	}, nil
}

// Run executes one unit against an already prepared environment.
func Run(ctx context.Context, env Env, unit Unit) error {
	if unit == nil {
		return fmt.Errorf("runtime unit is nil")
	}
	return unit.Run(ctx, env)
}

// ShutdownSignals returns the process signals Chatto treats as graceful stop
// requests for long-running runtime processes.
func ShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM}
}

// NotifyContext returns a context cancelled by Chatto's graceful shutdown
// signals.
func NotifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, ShutdownSignals()...)
}

// RequireStandaloneNATSClientURL enforces that standalone units connect to an
// existing NATS server instead of trying to start or attach to embedded storage.
func RequireStandaloneNATSClientURL(cfg config.ChattoConfig, unitName string) error {
	if cfg.NATS.Client.URL != "" {
		return nil
	}
	return fmt.Errorf("%s needs a NATS client URL. Configure external NATS under [nats.client], or enable nats.embedded.port so chatto can derive nats.client.url for the embedded server", unitName)
}

// ConnectToNATS establishes a NATS connection for Chatto runtime processes.
// When embeddedNATS is non-nil, the connection is in-process and intended only
// for `chatto run` and units embedded within it.
func ConnectToNATS(ctx context.Context, cfg config.ChattoConfig, embeddedNATS *server.Server) (*nats.Conn, error) {
	logger := log.WithPrefix("nats")

	var connectOpts []nats.Option

	if embeddedNATS != nil {
		connectOpts = append(connectOpts, embedded_nats.InProcessConnectOption(embeddedNATS))
		if cfg.NATS.Embedded.AuthToken != "" {
			connectOpts = append(connectOpts, nats.Token(cfg.NATS.Embedded.AuthToken))
		}
	} else {
		authOpts, err := natsauth.ConnectOptions(cfg.NATS.Client.NATSAuthConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to get NATS auth options: %w", err)
		}
		connectOpts = append(connectOpts, authOpts...)
	}

	connectOpts = append(connectOpts,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(100*time.Millisecond),
		nats.ReconnectBufSize(8*1024*1024),
		nats.DrainTimeout(5*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			if sub != nil {
				logger.Error("NATS subscription error", "subject", sub.Subject, "error", err)
			} else {
				logger.Error("NATS error", "error", err)
			}
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
	)

	natsURL := cfg.NATS.Client.URL
	if embeddedNATS != nil {
		natsURL = nats.DefaultURL
	}

	var (
		nc  *nats.Conn
		err error
	)
	for attempt := range 10 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		nc, err = nats.Connect(natsURL, connectOpts...)
		if err == nil {
			break
		}
		if attempt < 9 {
			logger.Warn("Failed to connect to NATS, retrying", "error", err, "attempt", attempt+1)
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if err != nil {
		return nil, err
	}

	if embeddedNATS != nil {
		logger.Info("Connected to embedded NATS server")
	} else {
		logger.Info("Connected to NATS", "url", nc.ConnectedUrl())
	}
	return nc, nil
}

// CloseNATSConnection drains and closes a NATS connection.
func CloseNATSConnection(nc *nats.Conn) {
	if nc == nil || nc.IsClosed() {
		return
	}

	drained := make(chan struct{})
	var closeDrained sync.Once
	previousClosedHandler := nc.ClosedHandler()
	nc.SetClosedHandler(func(conn *nats.Conn) {
		if previousClosedHandler != nil {
			previousClosedHandler(conn)
		}
		closeDrained.Do(func() {
			close(drained)
		})
	})

	if err := nc.Drain(); err != nil {
		log.Warn("Failed to drain NATS connection before close", "error", err)
		nc.Close()
		closeDrained.Do(func() {
			close(drained)
		})
		return
	}

	timeout := nc.Opts.DrainTimeout
	if timeout <= 0 {
		timeout = nats.DefaultDrainTimeout
	}

	waitTimeout := timeout + 6*time.Second
	select {
	case <-drained:
	case <-time.After(waitTimeout):
		log.Warn("Timed out waiting for NATS connection drain to complete", "timeout", waitTimeout)
		nc.Close()
	}
}
