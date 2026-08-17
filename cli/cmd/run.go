package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/embedded_nats"
	"hmans.de/chatto/internal/exporter"
	"hmans.de/chatto/internal/http_server"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/push"
	"hmans.de/chatto/internal/runtimeunit"
	searchbleve "hmans.de/chatto/internal/search/bleve"
	"hmans.de/chatto/internal/video"
	"hmans.de/chatto/pkg/natsruntime"
)

// devStartupHook is called after core is initialized. Set by build-tagged init().
// Receives the loaded config so dev-only setup paths can read sections like
// `[bootstrap]` without a separate env-var or sidecar file. In bootstrap-tag
// builds this applies the [bootstrap] section from chatto.toml; in release
// builds this is a no-op.
var devStartupHook func(ctx context.Context, core *core.ChattoCore, cfg config.ChattoConfig)

const (
	optionalRuntimeUnitInitialRetry = time.Second
	optionalRuntimeUnitMaxRetry     = 30 * time.Second
)

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

func runtimeUnitRegistrations() []runtimeunit.Registration {
	return []runtimeunit.Registration{
		{
			Unit: exporter.Unit{},
			StartWithRun: func(cfg config.ChattoConfig) bool {
				return cfg.Exporter.Enabled
			},
		},
		{
			Unit: searchbleve.Unit{},
			StartWithRun: func(cfg config.ChattoConfig) bool {
				return cfg.SearchProvider.Enabled
			},
		},
		{
			Unit: video.Unit{},
			StartWithRun: func(cfg config.ChattoConfig) bool {
				return cfg.AssetProcessing.Enabled
			},
		},
	}
}

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

	configureLogging(cfg.General)
	if shouldPrintBanner(cfg.General.LogFormat, isLogOutputTerminal()) {
		printBanner()
	}

	stopStartupCPUProfile := startStartupCPUProfile(cfg.Diagnostics.StartupCPUProfile)
	startupCPUProfileStopped := false
	defer func() {
		if !startupCPUProfileStopped {
			stopStartupCPUProfile()
		}
	}()

	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()

	// Conductor stops foreground run scripts with SIGHUP before escalating.
	// Chatto has no reload-on-HUP behavior, so treat it as graceful shutdown
	// alongside the usual terminal and supervisor stop signals.
	shutdownSignals := runtimeunit.ShutdownSignals()
	signalLog := make(chan os.Signal, 1)
	stopSignalLog := make(chan struct{})
	signal.Notify(signalLog, shutdownSignals...)
	defer func() {
		signal.Stop(signalLog)
		close(stopSignalLog)
	}()
	go func() {
		select {
		case sig := <-signalLog:
			log.Info("Received shutdown signal", "signal", sig.String())
		case <-stopSignalLog:
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals...)
	defer stop()

	// Use errgroup to coordinate services
	g, ctx := errgroup.WithContext(ctx)

	// Start embedded NATS if enabled (must be ready before other services)
	var embeddedNATS *natsruntime.Server
	if cfg.NATS.Embedded.Enabled {
		var err error
		embeddedNATS, err = embedded_nats.StartServer(&cfg.NATS.Embedded)
		if err != nil {
			log.Fatal("Failed to start embedded NATS server", "error", err)
		}
		defer embedded_nats.ShutdownServer(embeddedNATS)
	}

	// Connect to NATS
	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, embeddedNATS)
	if err != nil {
		log.Error("Failed to connect to NATS", "error", err)
		exitCode = 1
		return
	}
	defer runtimeunit.CloseNATSConnection(nc)

	// Create Chatto core
	cfg.Core.AuthTokenTTL = cfg.Auth.TokenTTLOrDefault()
	cfg.Core.EmailOTP = cfg.Auth.EmailOTP
	cfg.Core.Replicas = cfg.NATS.ReplicasOrDefault()
	cfg.Core.Limits = cfg.Limits
	cfg.Core.Owners = cfg.Owners
	cfg.Core.Version = Version
	chattoCore, err := core.NewChattoCore(ctx, nc, cfg.Core)
	if err != nil {
		log.Error("Failed to create Chatto core", "error", err)
		exitCode = 1
		return
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
		chattoCore.VideoUploadsEnabled = true
	}

	if err := chattoCore.EnableLiveKitCallReconciliation(cfg.LiveKit); err != nil {
		log.Error("Failed to configure LiveKit call-state reconciliation", "error", err)
		exitCode = 1
		return
	}

	// Set up push notification callback if push is enabled
	setupPushNotifications(chattoCore, cfg)

	// Start core's background services (PresenceHub + projectors) BEFORE
	// bootstrap. Bootstrap triggers JoinRoom, which calls WaitForSeq on
	// the room-membership projector — if it's not running yet, the wait
	// blocks until the bootstrap context cancels.
	g.Go(func() error {
		return chattoCore.Run(ctx)
	})

	// Block until core.Run has finished its boot phase (projectors
	// started + ensureChannelRoomsAreInAGroup done). SeedDefaultRooms
	// issues CreateRoom calls whose default-group lookup hits the
	// RoomGroups projection — without this wait, the projection is
	// still empty and the seeded rooms land without a group.
	if err := chattoCore.WaitForBoot(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error("Core boot never completed", "error", err)
		exitCode = 1
		return
	}
	stopStartupCPUProfile()
	startupCPUProfileStopped = true

	// Seed `announcements` + `general` on first boot of a fresh server.
	// Idempotent — no-op once any channel room exists.
	if err := chattoCore.SeedDefaultRooms(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Error("Failed to seed default rooms", "error", err)
		exitCode = 1
		return
	}

	// Run dev startup hook (auto-bootstrap in dev builds, no-op in prod)
	devStartupHook(ctx, chattoCore, cfg)

	unitRegistrations := runtimeUnitRegistrations()
	if err := runtimeunit.ValidateRegistrations(unitRegistrations); err != nil {
		log.Error("Failed to configure runtime units", "error", err)
		exitCode = 1
		return
	}
	for _, registration := range unitRegistrations {
		if !registration.Enabled(cfg) {
			continue
		}
		unit := registration.Unit
		env, err := runtimeunit.NewEnv(ctx, cfg, nc, log.WithPrefix(unit.Name()), Version)
		if err != nil {
			log.Error("Failed to create runtime unit environment", "unit", unit.Name(), "error", err)
			exitCode = 1
			return
		}
		g.Go(func() error {
			return runOptionalRuntimeUnit(ctx, env, unit)
		})
	}

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
		log.Error("Failed to create HTTP server", "error", err)
		exitCode = 1
		return
	}
	g.Go(func() error {
		return httpServer.Run(ctx)
	})

	// Wait for all services to complete (or one to fail)
	if err := g.Wait(); err != nil && err != context.Canceled {
		log.Error("Server failed", "error", err)
		exitCode = 1
	}
}

func runOptionalRuntimeUnit(ctx context.Context, env runtimeunit.Env, unit runtimeunit.Unit) error {
	return superviseOptionalRuntimeUnit(ctx, env, unit, optionalRuntimeUnitRetryDelay)
}

func superviseOptionalRuntimeUnit(
	ctx context.Context,
	env runtimeunit.Env,
	unit runtimeunit.Unit,
	retryDelay func(int) time.Duration,
) error {
	for attempt := 1; ; attempt++ {
		err := runtimeunit.Run(ctx, env, unit)
		if ctx.Err() != nil {
			return nil
		}
		delay := retryDelay(attempt)
		if err != nil {
			env.Logger.Error("Optional runtime unit stopped; restarting", "error", err, "restart_attempt", attempt, "retry_delay", delay)
		} else {
			env.Logger.Warn("Optional runtime unit stopped unexpectedly; restarting", "restart_attempt", attempt, "retry_delay", delay)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func optionalRuntimeUnitRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return optionalRuntimeUnitInitialRetry
	}
	delay := optionalRuntimeUnitInitialRetry
	for range attempt - 1 {
		if delay >= optionalRuntimeUnitMaxRetry/2 {
			return optionalRuntimeUnitMaxRetry
		}
		delay *= 2
	}
	return min(delay, optionalRuntimeUnitMaxRetry)
}

func printBanner() {
	for line := range strings.SplitSeq(banner, "\n") {
		log.Info(line)
	}
}

func configureLogging(cfg config.GeneralConfig) {
	setLogFormat(cfg.LogFormat, isLogOutputTerminal())
	setLogLevel(cfg.LogLevel)
}

func setLogFormat(format string, outputIsTerminal bool) {
	switch effectiveLogFormat(format, outputIsTerminal) {
	case "json":
		log.SetFormatter(log.JSONFormatter)
	case "logfmt":
		log.SetFormatter(log.LogfmtFormatter)
	default:
		log.SetFormatter(log.TextFormatter)
	}
}

func effectiveLogFormat(format string, outputIsTerminal bool) string {
	switch strings.ToLower(format) {
	case "", "auto":
		if outputIsTerminal {
			return "text"
		}
		return "json"
	case "json", "logfmt", "text":
		return strings.ToLower(format)
	default:
		return "text"
	}
}

func shouldPrintBanner(format string, outputIsTerminal bool) bool {
	return effectiveLogFormat(format, outputIsTerminal) == "text"
}

func isLogOutputTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
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

	chattoCore.OnPushTestRequested = func(ctx context.Context, userID string) error {
		subscriptions, err := chattoCore.GetUserPushSubscriptions(ctx, userID)
		if err != nil {
			return err
		}
		if len(subscriptions) == 0 {
			return errors.New("no push subscriptions registered")
		}
		subscriptions, err = filterOwnedPushSubscriptions(ctx, chattoCore, userID, subscriptions)
		if err != nil {
			return fmt.Errorf("revalidate push endpoint ownership: %w", err)
		}
		if len(subscriptions) == 0 {
			return errors.New("no current push subscriptions registered")
		}
		results := sender.SendToMany(ctx, subscriptions, &push.Payload{
			Title: "Test notification",
			Body:  "Push notifications are working.",
			URL:   cfg.Webserver.URL,
			Icon:  "/icons/icon-192.png",
			Badge: "/icons/icon-192.png",
			Tag:   "push-test",
		})
		var sendErr error
		accepted := false
		for _, result := range results {
			if result.Gone {
				_ = chattoCore.DeletePushSubscription(ctx, userID, result.Endpoint)
			}
			if result.Error == nil && result.Success {
				accepted = true
			}
			if result.Error != nil {
				sendErr = result.Error
			}
		}
		if accepted {
			return nil
		}
		if sendErr != nil {
			return sendErr
		}
		return errors.New("push provider did not accept the test notification")
	}

	chattoCore.SetNotificationAlertHandler(notificationAlertHandler(chattoCore, cfg, sender, logger))
}

type notificationPushSender interface {
	SendToMany(context.Context, []*corev1.PushSubscription, *push.Payload) []*push.SendResult
}

// notificationAlertHandler keeps the production provider seam independently
// testable while ChattoCore owns durable signal consumption and terminal occurrence state.
func notificationAlertHandler(chattoCore *core.ChattoCore, cfg config.ChattoConfig, sender notificationPushSender, logger *log.Logger) func(context.Context, *corev1.NotificationOccurrence) error {
	return func(ctx context.Context, occurrence *corev1.NotificationOccurrence) error {
		if core.NotificationOccurrenceHasUnsupportedSignal(occurrence) {
			return core.ErrUnsupportedNotificationSignal
		}
		if core.NotificationOccurrenceMessageReference(occurrence) == nil {
			return core.ErrNotificationAlertSuppressed
		}
		subscriptions, err := chattoCore.GetUserPushSubscriptions(ctx, occurrence.GetRecipientId())
		if err != nil {
			return fmt.Errorf("get push subscriptions: %w", err)
		}
		if len(subscriptions) == 0 {
			return core.ErrNotificationAlertSuppressed
		}
		actorName := "Someone"
		if occurrence.GetActorId() != "" {
			if actor, actorErr := chattoCore.GetUser(ctx, occurrence.GetActorId()); actorErr == nil && actor != nil {
				actorName = actor.DisplayName
				if actorName == "" {
					actorName = actor.Login
				}
			}
		}
		payload := push.BuildPayloadFromOccurrence(occurrence, actorName, cfg.Webserver.URL, fetchOccurrencePayloadContext(ctx, chattoCore, occurrence, logger))
		if count, countErr := chattoCore.NotificationOccurrences().UnreadCount(ctx, occurrence.GetRecipientId()); countErr == nil {
			payload.AppBadge = strconv.Itoa(count)
		}

		// Revalidate after hydration so a concurrent delete or visibility purge
		// cannot overtake a slow alert preparation.
		eligible, err := chattoCore.NotificationAlertEligible(ctx, occurrence)
		if err != nil {
			return err
		}
		if !eligible {
			return core.ErrNotificationAlertSuppressed
		}
		subscriptions, err = filterOwnedPushSubscriptions(ctx, chattoCore, occurrence.GetRecipientId(), subscriptions)
		if err != nil {
			return fmt.Errorf("revalidate push endpoint ownership: %w", err)
		}
		if len(subscriptions) == 0 {
			return core.ErrNotificationAlertSuppressed
		}
		status, err := chattoCore.GetUserPresence(ctx, occurrence.GetRecipientId())
		if err != nil {
			return fmt.Errorf("revalidate notification presence before delivery: %w", err)
		}
		if status == core.PresenceStatusDoNotDisturb {
			return core.ErrNotificationAlertSuppressed
		}
		alertDeadline := core.NotificationAlertDeadline(occurrence)
		remaining := time.Until(alertDeadline)
		if remaining <= 0 {
			return core.ErrNotificationAlertSuppressed
		}
		payload.DeliveryDeadline = alertDeadline
		sendCtx, cancel := context.WithDeadline(ctx, alertDeadline)
		defer cancel()
		results := sender.SendToMany(sendCtx, subscriptions, payload)
		var sendErr error
		accepted := false
		for _, result := range results {
			if result.Gone {
				_ = chattoCore.DeletePushSubscription(ctx, occurrence.GetRecipientId(), result.Endpoint)
				continue
			}
			if result.Success {
				accepted = true
				continue
			}
			if result.Error != nil {
				sendErr = result.Error
			}
		}
		// Delivery is occurrence-scoped: once any current device accepts the
		// alert, complete delivery. Retrying the whole occurrence for another
		// failing endpoint would duplicate alerts on every successful device.
		if accepted {
			return nil
		}
		if sendErr == nil {
			return core.ErrNotificationAlertSuppressed
		}
		return sendErr
	}
}

func filterOwnedPushSubscriptions(
	ctx context.Context,
	chattoCore *core.ChattoCore,
	userID string,
	subscriptions []*corev1.PushSubscription,
) ([]*corev1.PushSubscription, error) {
	owned := make([]*corev1.PushSubscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		isOwned, err := chattoCore.PushSubscriptionCurrentForUser(ctx, userID, subscription)
		if err != nil {
			return nil, err
		}
		if isOwned {
			owned = append(owned, subscription)
		}
	}
	return owned, nil
}

// fetchOccurrencePayloadContext builds a best-effort message preview and room
// name for an occurrence-backed push payload.
func fetchOccurrencePayloadContext(ctx context.Context, chattoCore *core.ChattoCore, occurrence *corev1.NotificationOccurrence, logger *log.Logger) *push.PayloadContext {
	target := core.NotificationOccurrenceMessageReference(occurrence)
	if target == nil {
		return nil
	}
	roomID := target.GetRoomId()
	eventID := target.GetEventId()

	if eventID == "" {
		return nil
	}

	payloadCtx := &push.PayloadContext{}

	kind, err := chattoCore.FindRoomKind(ctx, roomID)
	if err != nil {
		logger.Debug("Failed to resolve room kind for push notification preview",
			"room_id", roomID, "error", err)
		return nil
	}

	// Fetch the message to get its body
	event, err := chattoCore.GetRoomEventByEventID(ctx, kind, roomID, eventID)
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
	if _, ok := event.Event.(*corev1.Event_MessagePosted); ok {
		body, err := chattoCore.GetMessageBody(ctx, event.Id)
		if err != nil {
			logger.Debug("Failed to fetch message body for push notification preview",
				"event_id", event.Id,
				"error", err)
		} else {
			payloadCtx.MessagePreview = body
		}
	}

	if kind != core.KindDM {
		room, err := chattoCore.GetRoom(ctx, kind, roomID)
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
