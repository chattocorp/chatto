// Package mcpserver provides Chatto's optional network MCP runtime unit.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/time/rate"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/runtimeunit"
)

const (
	runtimeUnitName       = "mcp"
	maxRequestBodyBytes   = 256 * 1024
	defaultListRoomsLimit = 50
	maxListRoomsLimit     = 100
	requestTimeout        = 15 * time.Second
	shutdownTimeout       = 5 * time.Second
)

// Unit serves the configured network MCP endpoint. Core is injected by the
// main Chatto application so tools use the canonical operation layer.
type Unit struct {
	Core *core.ChattoCore
}

// Name returns the stable runtime-unit name.
func (Unit) Name() string { return runtimeUnitName }

// Run serves MCP until the process context stops.
func (u Unit) Run(ctx context.Context, env runtimeunit.Env) error {
	if u.Core == nil {
		return fmt.Errorf("MCP runtime unit needs Chatto core")
	}
	handler, err := NewHandler(u.Core, env.Config, env.Version)
	if err != nil {
		return err
	}
	address := net.JoinHostPort(env.Config.MCP.BindAddressOrDefault(), fmt.Sprint(env.Config.MCP.PortOrDefault()))
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       requestTimeout,
		WriteTimeout:      requestTimeout + 5*time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	env.Logger.Info("MCP runtime unit started", "address", address, "resource", env.Config.MCP.ResourceURL())

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("stop MCP server: %w", err)
		}
		if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

// NewHandler constructs the protected MCP and metadata endpoints.
func NewHandler(chattoCore *core.ChattoCore, cfg config.ChattoConfig, version string) (http.Handler, error) {
	if chattoCore == nil {
		return nil, fmt.Errorf("Chatto core is required")
	}
	resource := cfg.MCP.ResourceURL()
	resourceURL, err := url.Parse(resource)
	if err != nil || resourceURL.Path != "/mcp" {
		return nil, fmt.Errorf("valid MCP resource URL is required")
	}
	issuerURL, err := url.Parse(cfg.Webserver.URL)
	if err != nil || issuerURL.Scheme == "" || issuerURL.Host == "" {
		return nil, fmt.Errorf("valid webserver URL is required for MCP OAuth")
	}
	issuerURL.Path, issuerURL.RawQuery, issuerURL.Fragment = "", "", ""
	issuer := issuerURL.String()
	metadataURL := resourceURL.Scheme + "://" + resourceURL.Host + "/.well-known/oauth-protected-resource/mcp"

	server := mcp.NewServer(&mcp.Implementation{Name: "Chatto", Version: version}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_rooms",
		Description: "List a bounded page of Chatto rooms visible to the authenticated account.",
	}, listRoomsHandler(chattoCore))
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		MaxRequestBodyBytes:          maxRequestBodyBytes,
		PropagateRequestCancellation: true,
		// The outer handler validates the configured public Host exactly.
		DisableLocalhostProtection: true,
	})
	protected := auth.RequireBearerToken(tokenVerifier(chattoCore, resource), &auth.RequireBearerTokenOptions{
		ResourceMetadataURL:    metadataURL,
		Scopes:                 []string{config.MCPRoomsReadScope},
		AllowMissingExpiration: true,
	})(streamable)

	mux := http.NewServeMux()
	mcpHandler := http.NewCrossOriginProtection().Handler(withRequestDeadline(protected))
	mcpHandler = withAdmissionLimit(rate.NewLimiter(20, 40), mcpHandler)
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/.well-known/oauth-protected-resource/mcp", auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               resource,
		AuthorizationServers:   []string{issuer},
		ScopesSupported:        []string{config.MCPRoomsReadScope},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Chatto MCP",
	}))
	return requireCanonicalHost(resourceURL.Host, mux), nil
}

func requireCanonicalHost(host string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Host, host) {
			http.Error(w, "request host does not match the configured MCP URL", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withAdmissionLimit(limiter *rate.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "MCP request rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tokenVerifier(chattoCore *core.ChattoCore, resource string) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		credential, err := chattoCore.ValidatePresentedRuntimeCredential(ctx, token, core.AuthTokenPresentationBearer)
		if err == nil {
			if credential.Kind != core.AuthTokenKindOAuthAccessToken || credential.Resource != resource || !slices.Contains(credential.Scopes, config.MCPRoomsReadScope) {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{Scopes: credential.Scopes, Expiration: credential.ExpiresAt, UserID: credential.UserID}, nil
		}
		if !errors.Is(err, core.ErrAuthTokenNotFound) {
			return nil, err
		}
		bot, _, err := chattoCore.ValidateBotAPIKeyCredential(ctx, token)
		if err != nil {
			if errors.Is(err, core.ErrAuthTokenNotFound) {
				return nil, auth.ErrInvalidToken
			}
			return nil, err
		}
		return &auth.TokenInfo{Scopes: []string{config.MCPRoomsReadScope}, UserID: bot.GetId()}, nil
	}
}

type listRoomsInput struct {
	Limit       int    `json:"limit,omitempty" jsonschema:"Minimum 1, maximum 100. Default 50."`
	AfterRoomID string `json:"after_room_id,omitempty" jsonschema:"Return rooms after this room ID."`
}

type roomResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"`
	Archived    bool   `json:"archived,omitempty"`
	IsMember    bool   `json:"isMember"`
}

type listRoomsOutput struct {
	Rooms           []roomResult `json:"rooms"`
	NextAfterRoomID string       `json:"nextAfterRoomId,omitempty"`
}

func listRoomsHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[listRoomsInput, listRoomsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input listRoomsInput) (*mcp.CallToolResult, listRoomsOutput, error) {
		token := auth.TokenInfoFromContext(ctx)
		if token == nil || token.UserID == "" {
			return nil, listRoomsOutput{}, auth.ErrInvalidToken
		}
		limit := input.Limit
		if limit == 0 {
			limit = defaultListRoomsLimit
		}
		if limit < 1 || limit > maxListRoomsLimit {
			return nil, listRoomsOutput{}, fmt.Errorf("limit must be between 1 and %d", maxListRoomsLimit)
		}
		if len(input.AfterRoomID) > 256 || strings.TrimSpace(input.AfterRoomID) != input.AfterRoomID {
			return nil, listRoomsOutput{}, fmt.Errorf("after_room_id is invalid")
		}
		rooms, err := chattoCore.RoomDirectoryReads().ListRooms(ctx, token.UserID, core.RoomDirectoryListOptions{IncludeChannels: true, IncludeDMs: true})
		if err != nil {
			return nil, listRoomsOutput{}, err
		}
		sort.Slice(rooms, func(i, j int) bool { return rooms[i].Room.GetId() < rooms[j].Room.GetId() })
		start := sort.Search(len(rooms), func(i int) bool { return rooms[i].Room.GetId() > input.AfterRoomID })
		end := min(start+limit, len(rooms))
		output := listRoomsOutput{Rooms: make([]roomResult, 0, end-start)}
		for _, directoryRoom := range rooms[start:end] {
			room := directoryRoom.Room
			output.Rooms = append(output.Rooms, roomResult{
				ID: room.GetId(), Name: room.GetName(), Description: room.GetDescription(),
				Kind: roomKind(room.GetKind()), Archived: room.GetArchived(), IsMember: directoryRoom.ViewerState.IsMember,
			})
		}
		if end < len(rooms) && end > start {
			output.NextAfterRoomID = rooms[end-1].Room.GetId()
		}
		return nil, output, nil
	}
}

func roomKind(kind evtv1.RoomKind) string {
	switch kind {
	case evtv1.RoomKind_ROOM_KIND_CHANNEL:
		return "channel"
	case evtv1.RoomKind_ROOM_KIND_DM:
		return "dm"
	default:
		return "unknown"
	}
}

var _ runtimeunit.Unit = Unit{}
