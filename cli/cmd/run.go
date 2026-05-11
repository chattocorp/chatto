package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/embedded_nats"
	"hmans.de/chatto/internal/http_server"
	"hmans.de/chatto/pkg/natsauth"
	"hmans.de/chatto/internal/push"
	"hmans.de/chatto/internal/video"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// devStartupHook is called after core is initialized. Set by build-tagged init().
// Receives the loaded config so dev-only setup paths can read sections like
// `[bootstrap]` without a separate env-var or sidecar file. In bootstrap-tag
// builds this applies the [bootstrap] section from chatto.toml; in release
// builds this is a no-op.
var devStartupHook func(ctx context.Context, core *core.ChattoCore, cfg config.ChattoConfig)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

var banner = `
   ::::::::  :::    :::     ::: ::::::::::: ::::::::::: ::::::::
  :+:    :+: :+:    :+:   :+: :+:   :+:         :+:    :+:    :+:
  +:+        +:+    +:+  +:+   +:+  +:+         +:+    +:+    +:+
  +#+        +#++:++#++ +#++:++#++: +#+         +#+    +#+    +:+
  +#+        +#+    +#+ +#+     +#+ +#+         +#+    +#+    +#+
  #+#    #+# #+#    #+# #+#     #+# #+#         #+#    #+#    #+#
   ########  ###    ### ###     ### ###         ###     ########
`

var configFile string

var runCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"start"},
	Short:   "Runs the chatto server",
	Run: func(cmd *cobra.Command, args []string) {
		runServer(configFile)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringVarP(&configFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
}

func runServer(configPath string) {
	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}

	setLogLevel(cfg.General.LogLevel)
	printBanner()

	// Create context that cancels on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Use errgroup to coordinate services
	g, ctx := errgroup.WithContext(ctx)

	// Start embedded NATS if enabled (must be ready before other services)
	var embeddedNATS *server.Server
	if cfg.NATS.Embedded.Enabled {
		var err error
		embeddedNATS, err = embedded_nats.Start(ctx, g, &cfg.NATS.Embedded)
		if err != nil {
			log.Fatal("Failed to start embedded NATS server", "error", err)
		}
	}

	// Connect to NATS
	nc, err := connectToNATS(cfg, embeddedNATS)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer nc.Close()

	// Create Chatto core
	cfg.Core.AuthTokenTTL = cfg.Auth.TokenTTLOrDefault()
	cfg.Core.Replicas = cfg.NATS.ReplicasOrDefault()
	cfg.Core.Limits = cfg.Limits
	cfg.Core.Owners = cfg.Owners
	chattoCore, err := core.NewChattoCore(ctx, nc, cfg.Core)
	if err != nil {
		log.Fatal("Failed to create Chatto core", "error", err)
	}

	// Set asset base URL for absolute asset URLs (required for cross-origin clients)
	if cfg.Webserver.URL != "" {
		if parsed, err := url.Parse(cfg.Webserver.URL); err == nil {
			chattoCore.AssetBaseURL = parsed.Scheme + "://" + parsed.Host
		}
	}

	// Set video upload limit if video processing is enabled
	if cfg.Video.Enabled {
		chattoCore.VideoMaxUploadSize = int64(cfg.Video.MaxUploadSizeOrDefault())
	}

	// Set up push notification callback if push is enabled
	setupPushNotifications(chattoCore, cfg)

	// Run dev startup hook (auto-bootstrap in dev builds, no-op in prod)
	devStartupHook(ctx, chattoCore, cfg)

	// Run health checks in background (non-blocking)
	go runHealthChecks(ctx, chattoCore)

	// Start presence hub (single KV watcher per process for presence fan-out)
	g.Go(func() error {
		return chattoCore.PresenceHub.Run(ctx)
	})

	// Create and run HTTP server
	addr := fmt.Sprintf(":%d", cfg.Webserver.EffectivePort())
	httpServer, err := http_server.NewHTTPServer(http_server.HTTPServerConfig{
		Config:  cfg,
		NC:      nc,
		Core:    chattoCore,
		Addr:    addr,
		Version: Version,
	})
	if err != nil {
		log.Fatal("Failed to create HTTP server", "error", err)
	}
	g.Go(func() error {
		return httpServer.Run(ctx)
	})

	// Start video processing service if enabled
	if cfg.Video.Enabled {
		videoSvc := video.NewService(chattoCore, nc, cfg.Video, log.WithPrefix("video"))
		g.Go(func() error {
			return videoSvc.Run(ctx)
		})
	}

	// Wait for all services to complete (or one to fail)
	if err := g.Wait(); err != nil && err != context.Canceled {
		log.Fatal("Server failed", "error", err)
	}
}

// connectToNATS establishes a connection to NATS with appropriate options.
func connectToNATS(cfg config.ChattoConfig, embeddedNATS *server.Server) (*nats.Conn, error) {
	logger := log.WithPrefix("nats")

	var connectOpts []nats.Option

	if embeddedNATS != nil {
		// Use in-process connection for embedded NATS
		connectOpts = append(connectOpts, embedded_nats.InProcessConnectOption(embeddedNATS))
		// Provide token if server has auth enabled
		if cfg.NATS.Embedded.AuthToken != "" {
			connectOpts = append(connectOpts, nats.Token(cfg.NATS.Embedded.AuthToken))
		}
	} else {
		// Get auth options for external NATS
		authOpts, err := natsauth.ConnectOptions(cfg.NATS.Client.NATSAuthConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to get NATS auth options: %w", err)
		}
		connectOpts = append(connectOpts, authOpts...)
	}

	// Add resilience options
	connectOpts = append(connectOpts,
		nats.MaxReconnects(-1),                   // Unlimited reconnection attempts
		nats.ReconnectWait(100*time.Millisecond), // Quick initial reconnection
		nats.ReconnectBufSize(8*1024*1024),       // 8MB buffer for pending messages during reconnect
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
			} else {
				logger.Info("NATS disconnected (graceful)")
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			logger.Info("NATS connection closed")
		}),
	)

	// Connect to NATS (URL is ignored for in-process connections)
	natsURL := cfg.NATS.Client.URL
	if embeddedNATS != nil {
		natsURL = nats.DefaultURL // Not used for in-process, but nats.Connect requires a valid URL
	}

	// Retry initial connection to handle transient failures at startup
	// (e.g. Kubernetes secret volume mounts not yet propagated).
	var (
		nc  *nats.Conn
		err error
	)
	for attempt := range 10 {
		nc, err = nats.Connect(natsURL, connectOpts...)
		if err == nil {
			break
		}
		if attempt < 9 {
			logger.Warn("Failed to connect to NATS, retrying", "error", err, "attempt", attempt+1)
			time.Sleep(2 * time.Second)
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

func printBanner() {
	for line := range strings.SplitSeq(banner, "\n") {
		log.Info(line)
	}
}

func setLogLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	default:
		log.Warn("Unknown log level in configuration, defaulting to 'info'", "log_level", level)
		log.SetLevel(log.InfoLevel)
	}
}

// setupPushNotifications configures the push notification callback if push is enabled.
func setupPushNotifications(chattoCore *core.ChattoCore, cfg config.ChattoConfig) {
	if !cfg.Push.IsConfigured() {
		return
	}

	logger := log.WithPrefix("push")
	sender := push.NewSender(cfg.Push, logger)
	if sender == nil {
		return
	}

	logger.Info("Push notifications enabled")

	// Set the callback that will be invoked when notifications are created
	chattoCore.OnNotificationCreated = func(ctx context.Context, notification *corev1.Notification) {
		// Get user's push subscriptions
		subscriptions, err := chattoCore.GetUserPushSubscriptions(ctx, notification.RecipientId)
		if err != nil {
			logger.Warn("Failed to get push subscriptions",
				"user_id", notification.RecipientId,
				"error", err)
			return
		}

		if len(subscriptions) == 0 {
			return
		}

		// Get actor's display name for the notification
		actorName := "Someone"
		if notification.ActorId != "" {
			actor, err := chattoCore.GetUser(ctx, notification.ActorId)
			if err == nil && actor != nil {
				actorName = actor.DisplayName
				if actorName == "" {
					actorName = actor.Login
				}
			}
		}

		// Build payload context with message preview and room name
		payloadCtx := fetchPayloadContext(ctx, chattoCore, notification, logger)

		// Build and send push notification
		payload := push.BuildPayloadFromNotification(notification, actorName, cfg.Webserver.URL, payloadCtx)
		results := sender.SendToMany(ctx, subscriptions, payload)

		// Process results - clean up expired subscriptions
		for _, result := range results {
			if result.Gone {
				// Subscription is no longer valid, delete it
				if err := chattoCore.DeletePushSubscription(ctx, notification.RecipientId, result.Endpoint); err != nil {
					logger.Warn("Failed to delete expired push subscription",
						"endpoint", result.Endpoint[:min(50, len(result.Endpoint))],
						"error", err)
				} else {
					logger.Debug("Deleted expired push subscription",
						"endpoint", result.Endpoint[:min(50, len(result.Endpoint))])
				}
			} else if result.Error != nil {
				logger.Warn("Failed to send push notification",
					"endpoint", result.Endpoint[:min(50, len(result.Endpoint))],
					"error", result.Error)
			} else if result.Success {
				logger.Debug("Push notification sent",
					"user_id", notification.RecipientId,
					"notification_id", notification.Id)
			}
		}
	}

	// Set the callback that will be invoked when notifications are dismissed
	chattoCore.OnNotificationDismissed = func(ctx context.Context, userID string, notification *corev1.Notification) {
		// Get user's push subscriptions
		subscriptions, err := chattoCore.GetUserPushSubscriptions(ctx, userID)
		if err != nil {
			logger.Warn("Failed to get push subscriptions for dismiss",
				"user_id", userID,
				"error", err)
			return
		}

		if len(subscriptions) == 0 {
			return
		}

		// Get the notification tag for dismissal
		tag := push.NotificationTag(notification)
		if tag == "" {
			return
		}

		// Send dismiss push to all devices
		payload := &push.Payload{
			Action: "dismiss",
			Tag:    tag,
		}
		results := sender.SendToMany(ctx, subscriptions, payload)

		// Process results - clean up expired subscriptions
		for _, result := range results {
			if result.Gone {
				if err := chattoCore.DeletePushSubscription(ctx, userID, result.Endpoint); err != nil {
					logger.Warn("Failed to delete expired push subscription",
						"endpoint", result.Endpoint[:min(50, len(result.Endpoint))],
						"error", err)
				}
			} else if result.Error != nil {
				logger.Debug("Failed to send dismiss push",
					"endpoint", result.Endpoint[:min(50, len(result.Endpoint))],
					"error", result.Error)
			} else if result.Success {
				logger.Debug("Dismiss push sent",
					"user_id", userID,
					"tag", tag)
			}
		}
	}
}

// fetchPayloadContext builds the payload context with message preview and room name.
// This is best-effort - if fetching fails, returns nil and the notification will have a generic body.
func fetchPayloadContext(ctx context.Context, chattoCore *core.ChattoCore, notification *corev1.Notification, logger *log.Logger) *push.PayloadContext {
	var spaceID, roomID, eventID string

	switch n := notification.Notification.(type) {
	case *corev1.Notification_DmMessage:
		spaceID = core.DMSpaceID
		roomID = n.DmMessage.RoomId
		eventID = n.DmMessage.EventId
	case *corev1.Notification_Mention:
		spaceID = n.Mention.SpaceId
		roomID = n.Mention.RoomId
		eventID = n.Mention.EventId
	case *corev1.Notification_Reply:
		spaceID = n.Reply.SpaceId
		roomID = n.Reply.RoomId
		eventID = n.Reply.EventId
	default:
		return nil
	}

	if eventID == "" {
		return nil
	}

	payloadCtx := &push.PayloadContext{}

	// Fetch the message to get its body
	event, err := chattoCore.GetRoomEventByEventID(ctx, spaceID, roomID, eventID)
	if err != nil {
		logger.Debug("Failed to fetch event for push notification preview",
			"event_id", eventID,
			"error", err)
		return nil
	}
	if event == nil {
		return nil
	}

	// Extract message body from the event
	if msgPosted, ok := event.Event.(*corev1.ServerEvent_MessagePosted); ok {
		body, err := chattoCore.GetMessageBody(ctx, spaceID, msgPosted.MessagePosted.MessageBodyId)
		if err != nil {
			logger.Debug("Failed to fetch message body for push notification preview",
				"message_body_id", msgPosted.MessagePosted.MessageBodyId,
				"error", err)
		} else {
			payloadCtx.MessagePreview = body
		}
	}

	// For mentions and replies, also fetch the room name
	switch notification.Notification.(type) {
	case *corev1.Notification_Mention, *corev1.Notification_Reply:
		room, err := chattoCore.GetRoom(ctx, spaceID, roomID)
		if err != nil {
			logger.Debug("Failed to fetch room for push notification",
				"room_id", roomID,
				"error", err)
		} else if room != nil {
			payloadCtx.RoomName = room.Name
		}
	}

	return payloadCtx
}

// runHealthChecks runs startup health checks in the background.
// This ensures data integrity without blocking server startup.
func runHealthChecks(ctx context.Context, chattoCore *core.ChattoCore) {
	logger := log.WithPrefix("health")

	// Space RBAC health check
	report, err := chattoCore.SpaceRBACHealthCheck(ctx)
	if err != nil {
		logger.Error("Space RBAC health check failed", "error", err)
		return
	}

	if report.SpacesInitialized > 0 {
		logger.Info("Space RBAC health check complete",
			"spaces_checked", report.SpacesChecked,
			"spaces_initialized", report.SpacesInitialized)
	} else {
		logger.Debug("Space RBAC health check complete",
			"spaces_checked", report.SpacesChecked)
	}

	for _, errMsg := range report.Errors {
		logger.Warn("Space RBAC health check error", "error", errMsg)
	}
}
