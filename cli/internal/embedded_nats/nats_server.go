package embedded_nats

import (
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsruntime"
)

// StartServer creates and starts the embedded NATS server.
// It blocks until the server is ready for connections, then returns.
// The caller owns shutdown ordering and should stop the embedded server after
// application services have exited and NATS client connections are closed.
func StartServer(cfg *config.EmbeddedNATSConfig) (*natsruntime.Server, error) {
	logger := log.WithPrefix("server.NATS")

	runtime, err := startServer(serverOptions(cfg))
	if err != nil {
		return nil, err
	}

	if cfg.Port == 0 {
		logger.Info("Embedded NATS server is ready (in-process only, no TCP listener)")
	} else {
		logger.Info("Embedded NATS server is ready",
			"address", fmt.Sprintf("%s:%d", cfg.BindAddressOrDefault(), cfg.Port),
			"auth", cfg.AuthToken != "")
	}

	return runtime, nil
}

// ShutdownServer stops an embedded NATS server and waits until it has exited.
func ShutdownServer(runtime *natsruntime.Server) {
	if runtime == nil {
		return
	}
	logger := log.WithPrefix("server.NATS")
	runtime.Shutdown()
	logger.Info("Embedded NATS server shut down")
}

// StartPrivateServer starts a temporary in-process-only NATS server for
// maintenance operations such as restore.
func StartPrivateServer(dataDir string) (*natsruntime.Server, error) {
	return startServer(server.Options{
		JetStream:  true,
		StoreDir:   dataDir,
		DontListen: true,
	})
}

func startServer(options server.Options) (*natsruntime.Server, error) {
	runtime, err := natsruntime.Start(natsruntime.Config{
		Options:      options,
		ReadyTimeout: 4 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("start embedded NATS server: %w", err)
	}
	return runtime, nil
}

// serverOptions maps Chatto configuration to native NATS server options.
// When Port > 0, a TCP listener is enabled with token authentication.
func serverOptions(cfg *config.EmbeddedNATSConfig) server.Options {
	options := server.Options{
		JetStream: true,
		StoreDir:  cfg.DataDir,
	}

	if cfg.Port == 0 {
		options.DontListen = true
	} else {
		options.Port = cfg.Port
		options.Host = cfg.BindAddressOrDefault()
		if cfg.AuthToken != "" {
			options.Authorization = cfg.AuthToken
		}
	}

	if cfg.HTTPPort > 0 {
		options.HTTPPort = cfg.HTTPPort
		options.HTTPHost = cfg.BindAddressOrDefault()
	}

	return options
}
