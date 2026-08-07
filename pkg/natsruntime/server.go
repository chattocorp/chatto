// Package natsruntime owns application-neutral embedded NATS server lifecycle
// mechanics.
package natsruntime

import (
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// Config describes one embedded NATS server lifecycle.
//
// Options remains native to nats-server so applications can choose listeners,
// authentication, monitoring, logging, and storage without this package
// mirroring that configuration surface. Start uses nats-server's Clone method
// before nats-server applies defaults, then always enables NoSigs so the
// embedding application retains ownership of process signals. Callers must
// still treat references that nats-server itself does not clone as immutable
// after Start.
type Config struct {
	Options      server.Options
	ReadyTimeout time.Duration
}

// Server is a running embedded NATS server.
type Server struct {
	natsServer *server.Server
	shutdown   sync.Once
}

// Start creates and starts an embedded NATS server, then waits until it accepts
// connections. A startup failure shuts the partial server down before returning.
func Start(config Config) (*Server, error) {
	if config.ReadyTimeout <= 0 {
		return nil, fmt.Errorf("embedded NATS readiness timeout must be positive")
	}

	options := config.Options.Clone()
	options.NoSigs = true
	natsServer, err := server.NewServer(options)
	if err != nil {
		return nil, fmt.Errorf("create embedded NATS server: %w", err)
	}

	runtime := &Server{natsServer: natsServer}
	natsServer.Start()
	if !natsServer.ReadyForConnections(config.ReadyTimeout) {
		runtime.Shutdown()
		return nil, fmt.Errorf(
			"embedded NATS server did not become ready within %s",
			config.ReadyTimeout,
		)
	}
	return runtime, nil
}

// InProcessOption returns a NATS client option connected directly to the
// embedded server without a TCP listener.
func (s *Server) InProcessOption() nats.Option {
	return nats.InProcessServer(s.natsServer)
}

// Shutdown stops the embedded server and waits until it has exited. Repeated
// calls are safe.
func (s *Server) Shutdown() {
	if s == nil {
		return
	}
	s.shutdown.Do(func() {
		s.natsServer.Shutdown()
		s.natsServer.WaitForShutdown()
	})
}
