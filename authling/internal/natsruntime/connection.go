// Package natsruntime owns Authling's NATS process and client lifecycle.
package natsruntime

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/authling/internal/config"
)

// Connection is an Authling NATS client and any private embedded server that
// backs it. Close the client before stopping the embedded server.
type Connection struct {
	NATS     *nats.Conn
	embedded *server.Server
}

// Open connects to Authling's configured external account or starts a private
// in-process NATS server with JetStream enabled.
func Open(ctx context.Context, cfg config.NATSConfig) (*Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Embedded.Enabled {
		return openEmbedded(cfg.Embedded)
	}
	return openExternal(cfg.Client)
}

func openEmbedded(cfg config.EmbeddedNATSConfig) (*Connection, error) {
	natsServer, err := server.NewServer(&server.Options{
		JetStream:  true,
		StoreDir:   cfg.DataDir,
		DontListen: true,
		NoSigs:     true,
		NoLog:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("create embedded NATS server: %w", err)
	}
	natsServer.Start()
	if !natsServer.ReadyForConnections(5 * time.Second) {
		natsServer.Shutdown()
		natsServer.WaitForShutdown()
		return nil, fmt.Errorf("embedded NATS server did not become ready")
	}

	connection, err := nats.Connect(
		nats.DefaultURL,
		nats.Name("authling"),
		nats.InProcessServer(natsServer),
	)
	if err != nil {
		natsServer.Shutdown()
		natsServer.WaitForShutdown()
		return nil, fmt.Errorf("connect to embedded NATS server: %w", err)
	}
	return &Connection{NATS: connection, embedded: natsServer}, nil
}

func openExternal(cfg config.NATSClientConfig) (*Connection, error) {
	connection, err := nats.Connect(
		cfg.URL,
		nats.Name("authling"),
		nats.UserCredentials(cfg.CredentialsFile),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to external NATS account: %w", err)
	}
	return &Connection{NATS: connection}, nil
}

// Close drains the client and then shuts down any embedded NATS server.
func (c *Connection) Close() error {
	if c == nil {
		return nil
	}
	var drainErr error
	if c.NATS != nil {
		drainErr = c.NATS.Drain()
		c.NATS.Close()
	}
	if c.embedded != nil {
		c.embedded.Shutdown()
		c.embedded.WaitForShutdown()
	}
	if drainErr != nil {
		return fmt.Errorf("drain NATS connection: %w", drainErr)
	}
	return nil
}
