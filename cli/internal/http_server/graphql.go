package http_server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/vektah/gqlparser/v2/ast"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/graph"
	"hmans.de/chatto/internal/graph/auth"
	"hmans.de/chatto/pkg/gqldepthlimit"
)

func (s *HTTPServer) setupGraphQLAPI(allowedOrigins []string) {
	// Ensure logger is initialized (tests may bypass NewHTTPServer)
	if s.logger == nil {
		s.logger = log.WithPrefix("server.HTTP")
	}

	// Configure GraphQL server with injected dependencies
	resolver := graph.NewResolver(s.core, s.config.Owners, s.config.Auth, s.config.Push, s.config.Video, s.config.LiveKit, s.version)

	config := graph.Config{
		Resolvers: resolver,
	}

	h := handler.New(graph.NewExecutableSchema(config))

	// Add request timing middleware
	h.AroundOperations(func(ctx context.Context, next graphql.OperationHandler) graphql.ResponseHandler {
		oc := graphql.GetOperationContext(ctx)

		// Extract user if authenticated
		userID := ""
		if user := auth.ForContext(ctx); user != nil {
			userID = user.Id
		}

		logger := log.WithPrefix("graphql")

		// For subscriptions, just log the start
		if oc.Operation.Operation == ast.Subscription {
			logger.Debug("GraphQL subscription started",
				"operation", oc.OperationName,
				"type", "subscription",
				"userId", userID)

			return next(ctx)
		}

		// For queries and mutations, measure duration
		start := time.Now()
		resp := next(ctx)
		duration := time.Since(start)

		logger.Debug("GraphQL operation completed",
			"duration", duration.String(),
			"operation", oc.OperationName,
			"type", string(oc.Operation.Operation),
			"userId", userID)

		return resp
	})

	h.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		Upgrader: websocket.Upgrader{
			EnableCompression: s.config.Webserver.WebSocketCompressionEnabled(),
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				// Allow requests with no Origin header (non-browser clients like CLI tools, bots)
				if origin == "" {
					return true
				}
				// Check against the shared allowed origins list
				if s.matchOrigin(origin, allowedOrigins) != originNotAllowed {
					return true
				}
				// Auto-allow same-origin: if Origin matches the request's Host header,
				// it's a legitimate same-origin request. This handles multi-hostname
				// deployments (blue/green previews, staging, etc.) without requiring
				// each hostname to be listed in allowed_origins.
				host := r.Host
				if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
					host = forwarded
				}
				if parsedOrigin, err := url.Parse(origin); err == nil {
					if strings.EqualFold(parsedOrigin.Host, host) {
						return true
					}
				}
				// Block requests from other origins (potential CSRF attacks)
				s.logger.Warn("WebSocket connection rejected: origin mismatch",
					"origin", origin, "host", host, "allowed", allowedOrigins)
				return false
			},
		},
		InitFunc: func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
			// For WebSocket connections, the user may already be injected into the context
			// from the upgrade request (session cookie is sent with upgrade request).
			user := auth.ForContext(ctx)

			// If no user from cookie, check connectionParams.token (bearer token auth).
			// This is the standard graphql-ws pattern for cross-origin clients.
			if user == nil {
				if tokenRaw, ok := initPayload["token"]; ok {
					if token, ok := tokenRaw.(string); ok && token != "" {
						userID, err := s.core.ValidateAuthToken(ctx, token)
						if err == nil {
							loadedUser, err := s.core.GetUser(ctx, userID)
							if err == nil {
								user = loadedUser
							} else {
								s.logger.Warn("WebSocket token valid but user not found", "userId", userID, "error", err)
							}
						}
					}
				}
			}

			if user != nil {
				s.logger.Debug("WebSocket connection authenticated", "userId", user.Id, "login", user.Login)
			}

			// Create a fresh context for the WebSocket connection WITHOUT dataloaders.
			// The HTTP handler at /api/graphql injects dataloaders for the upgrade request,
			// but we must strip them here. WebSocket connections are long-lived and loaders
			// cache results for their lifetime - this causes stale user data (e.g., deleted
			// users still appearing) across subscription events and queries.
			// Instead, subscription resolvers fall back to direct core calls via r.getUser().
			// HTTP-only requests still get dataloaders for batching within a single request.
			//
			// We create a new context that preserves the auth user but not the dataloaders.
			newCtx := context.Background()
			if user != nil {
				newCtx = auth.WithUser(newCtx, user)
			}

			// Return version in the connection_ack payload so the frontend can
			// detect version changes and reload to get new frontend code.
			ackPayload := transport.InitPayload{
				"version": s.version,
			}
			return newCtx, &ackPayload, nil
		},
	})

	h.AddTransport(transport.Options{})
	h.AddTransport(transport.GET{})
	h.AddTransport(transport.POST{})

	// MaxMemory controls in-memory buffering before spilling to temp files.
	// MaxUploadSize is the total request body limit. When video is enabled,
	// the body limit must accommodate larger video uploads.
	maxMemory := int64(s.config.Core.Assets.MaxUploadSize)
	if maxMemory == 0 {
		maxMemory = assets.DefaultMaxUploadSize
	}
	maxUploadSize := maxMemory
	if s.config.Video.Enabled {
		videoMax := int64(s.config.Video.MaxUploadSizeOrDefault())
		if videoMax > maxUploadSize {
			maxUploadSize = videoMax
		}
	}
	h.AddTransport(transport.MultipartForm{
		MaxMemory:     maxMemory,
		MaxUploadSize: maxUploadSize,
	})

	h.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	// Introspection and the /api/playground UI below are intentionally
	// enabled unconditionally — exposing Chatto's GraphQL API for
	// experimentation is part of the product. See .claude/rules/general.md.
	h.Use(extension.Introspection{})
	h.Use(extension.FixedComplexityLimit(500))
	h.Use(&gqldepthlimit.Extension{MaxDepth: 12})
	h.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	s.router.Any("/api/graphql", func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)

		session := sessions.Default(c)
		if userID, _, ok := cookieSessionIDs(session); !ok {
			// Log unauthenticated requests to help diagnose cookie/session issues
			// (e.g., reverse proxy stripping Set-Cookie headers)
			s.logger.Debug("GraphQL request without session",
				"host", c.Request.Host,
				"hasCookie", c.Request.Header.Get("Cookie") != "")
		} else if userID == "" {
			clearCookieSessionAuth(session)
		}

		// Inject authenticated user into request context
		r := s.injectUserIntoContext(c)
		// Inject dataloaders for this request
		r = injectDataloadersIntoContext(r, s.core)
		h.ServeHTTP(c.Writer, r)
	})

	p := playground.Handler("CHATTO API Playground", "/api/graphql")
	s.router.GET("/api/playground", func(c *gin.Context) {
		p.ServeHTTP(c.Writer, c.Request)
	})
}
