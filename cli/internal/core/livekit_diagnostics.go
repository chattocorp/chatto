package core

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	lkauth "github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"github.com/twitchtv/twirp"
	"hmans.de/chatto/internal/config"
)

// liveKitProbeTimeout bounds the LiveKit server API check so an unreachable
// LiveKit server cannot stall the admin diagnostics response.
const liveKitProbeTimeout = 5 * time.Second

// LiveKitConnectionState is the result of the LiveKit server API check.
type LiveKitConnectionState int

const (
	// LiveKitConnectionNotConfigured means LiveKit is not configured, so no check ran.
	LiveKitConnectionNotConfigured LiveKitConnectionState = iota + 1
	// LiveKitConnectionOK means LiveKit accepted an API call with the configured credentials.
	LiveKitConnectionOK
	// LiveKitConnectionUnreachable means Chatto could not connect to the LiveKit URL.
	LiveKitConnectionUnreachable
	// LiveKitConnectionUnauthorized means LiveKit rejected the configured API key or secret.
	LiveKitConnectionUnauthorized
	// LiveKitConnectionError means the server at the LiveKit URL replied with another error.
	LiveKitConnectionError
)

// LiveKitAdminStatus describes the LiveKit setup for owner diagnostics. It
// never contains the API secret or the webhook signing secret.
type LiveKitAdminStatus struct {
	Enabled            bool
	Configured         bool
	URL                string
	APIKey             string
	SeparateWebhookKey bool
	// WebhookURL is the endpoint LiveKit must call, derived from the public server URL.
	WebhookURL      string
	ConnectionState LiveKitConnectionState
	// ConnectionError is the check failure detail. It is empty when the check passed or did not run.
	ConnectionError string
	// InsecureURL reports an unencrypted LiveKit URL on a non-local deployment.
	InsecureURL bool
	// LastWebhookAt and LastRejectedWebhookAt are process-local and reset on restart.
	LastWebhookAt         time.Time
	LastRejectedWebhookAt time.Time
}

// liveKitDiagnostics holds the LiveKit configuration that diagnostics report
// and the process-local webhook activity. The zero value reports LiveKit as
// not configured.
type liveKitDiagnostics struct {
	mu             sync.Mutex
	cfg            config.LiveKitConfig
	publicURL      string
	lastAcceptedAt time.Time
	lastRejectedAt time.Time
}

// ConfigureLiveKitDiagnostics sets the LiveKit configuration and public server
// URL that owner diagnostics report and check. Call it once during startup.
func (c *ChattoCore) ConfigureLiveKitDiagnostics(cfg config.LiveKitConfig, publicURL string) {
	c.liveKitDiagnostics.mu.Lock()
	defer c.liveKitDiagnostics.mu.Unlock()
	c.liveKitDiagnostics.cfg = cfg
	c.liveKitDiagnostics.publicURL = publicURL
}

// RecordLiveKitWebhook records that this process received a LiveKit webhook.
// accepted is false when the webhook signature was invalid.
func (c *ChattoCore) RecordLiveKitWebhook(accepted bool) {
	now := time.Now()
	c.liveKitDiagnostics.mu.Lock()
	defer c.liveKitDiagnostics.mu.Unlock()
	if accepted {
		c.liveKitDiagnostics.lastAcceptedAt = now
	} else {
		c.liveKitDiagnostics.lastRejectedAt = now
	}
}

// liveKitAdminStatus reports the LiveKit setup and checks the LiveKit server
// API with one ListRooms call. Callers must authorize the actor.
func (c *ChattoCore) liveKitAdminStatus(ctx context.Context) LiveKitAdminStatus {
	d := &c.liveKitDiagnostics
	d.mu.Lock()
	cfg := d.cfg
	publicURL := d.publicURL
	status := LiveKitAdminStatus{
		LastWebhookAt:         d.lastAcceptedAt,
		LastRejectedWebhookAt: d.lastRejectedAt,
	}
	d.mu.Unlock()

	status.Enabled = cfg.Enabled
	status.Configured = cfg.IsConfigured()
	status.URL = cfg.URL
	status.APIKey = cfg.APIKey
	webhookKey, _ := cfg.WebhookKeyPair()
	status.SeparateWebhookKey = webhookKey != "" && webhookKey != cfg.APIKey
	if publicURL != "" {
		status.WebhookURL = strings.TrimRight(publicURL, "/") + "/webhooks/livekit"
	}
	status.InsecureURL = liveKitURLInsecure(cfg.URL, publicURL)

	if !status.Configured {
		status.ConnectionState = LiveKitConnectionNotConfigured
		return status
	}
	httpURL, err := liveKitHTTPURL(cfg.URL)
	if err != nil {
		status.ConnectionState = LiveKitConnectionError
		status.ConnectionError = err.Error()
		return status
	}
	service := livekit.NewRoomServiceProtobufClient(httpURL, &http.Client{})
	client := &liveKitRoomClient{service: service, apiKey: cfg.APIKey, apiSecret: cfg.APISecret}

	probeCtx, cancel := context.WithTimeout(ctx, liveKitProbeTimeout)
	defer cancel()
	_, err = service.ListRooms(client.withVideoGrant(probeCtx, &lkauth.VideoGrant{RoomList: true}), &livekit.ListRoomsRequest{})
	status.ConnectionState = classifyLiveKitProbeError(err)
	if err != nil {
		status.ConnectionError = err.Error()
	}
	return status
}

// classifyLiveKitProbeError maps a LiveKit RoomService error to a connection
// state. Transport failures arrive as twirp internal errors that wrap the
// network or context error. Every transport failure, including DNS and TLS
// errors, counts as unreachable. A reverse proxy in front of a stopped
// LiveKit replies 502, 503, or 504, which twirp reports as unavailable or
// deadline exceeded. LiveKit answers bad credentials with a plain 401, which
// twirp reports as unauthenticated.
func classifyLiveKitProbeError(err error) LiveKitConnectionState {
	if err == nil {
		return LiveKitConnectionOK
	}
	if _, ok := errors.AsType[net.Error](err); ok || errors.Is(err, context.DeadlineExceeded) {
		return LiveKitConnectionUnreachable
	}
	if twerr, ok := errors.AsType[twirp.Error](err); ok {
		switch twerr.Code() {
		case twirp.Unauthenticated, twirp.PermissionDenied:
			return LiveKitConnectionUnauthorized
		case twirp.Unavailable, twirp.DeadlineExceeded:
			return LiveKitConnectionUnreachable
		}
	}
	return LiveKitConnectionError
}

// liveKitURLInsecure reports whether the LiveKit URL is unencrypted while the
// public server URL is not a local address. Browsers on an HTTPS Chatto page
// cannot open plain ws:// connections, and unencrypted media signaling is
// unsafe on a public network.
func liveKitURLInsecure(liveKitURL, publicURL string) bool {
	u, err := url.Parse(liveKitURL)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "http") {
		return false
	}
	public, err := url.Parse(publicURL)
	if err != nil || publicURL == "" {
		return true
	}
	return !isLocalHost(public.Hostname())
}

func isLocalHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
