package http_server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/auth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AT Protocol sign-in (FDR-056, ADR-123).
//
// The client signs in to the user's PDS itself. To sign in to Chatto, it asks
// the PDS for a short-lived service token. The PDS signs the token with the
// account's key and addresses it to this server's did:web identity, for the
// method run.chatto.signIn. The server verifies the token against the DID
// document and accepts each token once. It runs no OAuth flow and receives no
// PDS access.

const (
	// atprotoSignInMethod is the only method that a sign-in token may name.
	atprotoSignInMethod = "run.chatto.signIn"
	// atprotoServiceFragment names the Chatto service in the server's DID.
	atprotoServiceFragment = "#chatto"
	// atprotoMaxTokenLifetime bounds tokens and the replay records they need.
	// Clients request 60 seconds.
	atprotoMaxTokenLifetime = 2 * time.Minute
	// atprotoSignInTimeout bounds DID resolution and verification.
	atprotoSignInTimeout = 10 * time.Second
)

// atprotoSignIn verifies service tokens for one configured atproto provider.
type atprotoSignIn struct {
	provider  config.AuthProviderConfig
	validator *auth.ServiceAuthValidator
}

// setupATProtoRoutes mounts the server's DID document and the sign-in
// endpoints for the first configured atproto provider.
func (s *HTTPServer) setupATProtoRoutes() {
	var provider *config.AuthProviderConfig
	for i := range s.config.Auth.Providers {
		if s.config.Auth.Providers[i].Type == config.AuthProviderTypeATProto {
			provider = &s.config.Auth.Providers[i]
			break
		}
	}
	if provider == nil {
		return
	}
	did := config.AtprotoServiceDID(s.config.Webserver.URL)
	if did == "" {
		s.logger.Error("AT Protocol sign-in needs an HTTPS webserver.url without a port", "provider_id", provider.ID)
		return
	}
	dir := s.atprotoDirectory
	if dir == nil {
		dir = identity.DefaultDirectory()
	}
	signIn := &atprotoSignIn{
		provider:  *provider,
		validator: &auth.ServiceAuthValidator{Audience: did + atprotoServiceFragment, Dir: dir},
	}
	if strings.HasPrefix(did, "did:web:") && !strings.HasPrefix(did, "did:web:localhost") {
		origin := configuredWebserverOrigin(s.config.Webserver.URL)
		s.router.GET("/.well-known/did.json", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=300")
			c.JSON(http.StatusOK, gin.H{
				"@context": []string{"https://www.w3.org/ns/did/v1"},
				"id":       did,
				"service": []gin.H{{
					"id":              atprotoServiceFragment,
					"type":            "ChattoServer",
					"serviceEndpoint": origin,
				}},
			})
		})
	}

	group := s.router.Group("/auth")
	group.Use(limitLegacyRequestBody())
	group.Use(func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		c.Next()
	})
	handler := func(c *gin.Context) { s.handleATProtoSignIn(c, signIn) }
	group.POST("atproto", requireJSONAuthenticationRequest, handler)
	group.POST("browser/atproto", s.requireBrowserAuthenticationRequest, handler)
}

// atprotoSignInRequest is the body of POST /auth/atproto.
type atprotoSignInRequest struct {
	// Token is a PDS-signed service token for run.chatto.signIn.
	Token string `json:"token"`
	// LinkToken links the identity to the account that
	// StartExternalIdentityLink bound it to, instead of signing in.
	LinkToken string `json:"link_token"`
	// Admission is an accepted registration invitation for a new account.
	Admission string `json:"admission"`
}

// handleATProtoSignIn signs in, starts a new-account confirmation, or links
// the identity. Browser routes create a cookie session; programmatic routes
// return a bearer session.
//
// Responses: {"status":"signed_in","user":{...}}, {"status":"linked"}, or
// {"status":"confirm_account","token":...}. Errors use stable codes:
// invalid_token, external_identity_unlinked, external_identity_conflict,
// provider_failed.
func (s *HTTPServer) handleATProtoSignIn(c *gin.Context, signIn *atprotoSignIn) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	var request atprotoSignInRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_token"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), atprotoSignInTimeout)
	defer cancel()

	did, err := s.verifyATProtoServiceToken(ctx, signIn, request.Token)
	if err != nil {
		log.Warn("AT Protocol service token rejected", "provider_id", signIn.provider.ID, "reason", atprotoTokenRejectionReason(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	issuer, subject := signIn.provider.ID, did.String()

	if request.LinkToken != "" {
		start, err := s.core.ConsumePendingExternalIdentityLinkStart(ctx, request.LinkToken)
		if err != nil || start.ProviderID != signIn.provider.ID {
			c.JSON(http.StatusForbidden, gin.H{"error": "provider_failed"})
			return
		}
		if err := s.core.LinkExternalIdentity(ctx, signIn.provider.ID, signIn.provider.Type, issuer, subject, start.BoundUserID); err != nil {
			if errors.Is(err, core.ErrExternalIdentityAlreadyClaimed) {
				c.JSON(http.StatusConflict, gin.H{"error": "external_identity_conflict"})
				return
			}
			log.Error("AT Protocol identity link failed", "provider_id", signIn.provider.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "linked", "user": gin.H{"id": start.BoundUserID}})
		return
	}

	handle := s.atprotoHandle(ctx, signIn, did)
	user, authGeneration, err := s.core.GetUserByExternalIdentityForAuthentication(ctx, issuer, subject)
	if err != nil {
		log.Error("Failed to look up AT Protocol identity", "provider_id", signIn.provider.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
		return
	}
	if user == nil {
		s.startATProtoAccountConfirmation(c, signIn, subject, handle, request.Admission)
		return
	}
	if signIn.provider.ManageUsernames && handle != "" {
		if err := s.core.SyncProviderUsername(ctx, user.Id, signIn.provider.ID, handle); err != nil {
			log.Warn("Provider username sync failed", "provider_id", signIn.provider.ID, "user_id", user.Id, "error", err)
		}
	}

	response := gin.H{"status": "signed_in", "user": gin.H{"id": user.Id, "login": user.Login}}
	source := config.AuthProviderTypeATProto + "_login"
	if requestsCookieOnlyAuthentication(c) {
		if err := s.createCookieSessionForGeneration(c, user.Id, source, authGeneration); err != nil {
			log.Error("Failed to create AT Protocol cookie session", "user_id", user.Id, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
			return
		}
		if err := s.ensureCSRFToken(c); err != nil {
			s.revokeCurrentCookieSession(c)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
			return
		}
	} else {
		credentials, err := s.core.CreateBearerSessionWithSourceGeneration(ctx, user.Id, source, authGeneration)
		if err != nil {
			log.Error("Failed to create AT Protocol bearer session", "user_id", user.Id, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
			return
		}
		addBearerSessionResponse(response, credentials)
	}
	if err := s.core.RecordLoginSucceeded(ctx, user.Id, signIn.provider.Type+":"+signIn.provider.ID); err != nil {
		log.Error("Failed to append login audit event", "user_id", user.Id, "error", err)
		if refresh, ok := response["refreshToken"].(string); ok {
			_ = s.core.RevokeRefreshTokenWithReason(ctx, refresh, "login_audit_failed")
		} else {
			s.revokeCurrentCookieSession(c)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
		return
	}
	c.JSON(http.StatusOK, response)
}

// startATProtoAccountConfirmation responds with a pending new-account token,
// or with external_identity_unlinked when the provider creates no accounts.
// Programmatic clients confirm accounts through the server's own sign-in page.
func (s *HTTPServer) startATProtoAccountConfirmation(c *gin.Context, signIn *atprotoSignIn, subject, handle, admission string) {
	if !signIn.provider.AutoProvisionOrDefault() || !requestsCookieOnlyAuthentication(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "external_identity_unlinked"})
		return
	}
	invitationID, _ := s.registrationInvitation(c, admission)
	flow := core.PendingExternalIdentityFlow{
		ProviderID:      signIn.provider.ID,
		ProviderType:    signIn.provider.Type,
		ProviderLabel:   signIn.provider.LabelOrDefault(),
		Issuer:          signIn.provider.ID,
		Subject:         subject,
		LoginHint:       loginHintFromParts(handle),
		DisplayNameHint: displayNameHintFromParts(handle),
		RedirectPath:    "/",
		InvitationID:    invitationID,
	}
	if signIn.provider.ManageUsernames {
		flow.ProviderUsername = handle
	}
	token, err := s.core.CreatePendingExternalIdentityCreateFlow(c.Request.Context(), flow)
	if err != nil {
		log.Error("Failed to create pending external identity flow", "provider_id", signIn.provider.ID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "provider_failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "confirm_account", "token": token})
}

// verifyATProtoServiceToken returns the token's verified DID. It checks the
// signature, audience, method, and lifetime, and claims the token ID.
func (s *HTTPServer) verifyATProtoServiceToken(ctx context.Context, signIn *atprotoSignIn, token string) (syntax.DID, error) {
	method := syntax.NSID(atprotoSignInMethod)
	did, err := signIn.validator.Validate(ctx, token, &method)
	if err != nil {
		return "", err
	}
	// The signature is valid, so the claims can be read without a second check.
	var claims jwt.RegisteredClaims
	if _, _, err := jwt.NewParser().ParseUnverified(token, &claims); err != nil {
		return "", err
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > atprotoMaxTokenLifetime {
		return "", errATProtoTokenLifetime
	}
	if err := s.core.ClaimAtprotoServiceToken(ctx, claims.ID, claims.ExpiresAt.Time); err != nil {
		return "", err
	}
	return did, nil
}

var errATProtoTokenLifetime = errors.New("service token lifetime too long")

// atprotoTokenRejectionReason maps a rejection to a log-safe reason. Token
// contents and DIDs identify users and are not logged.
func atprotoTokenRejectionReason(err error) string {
	switch {
	case errors.Is(err, core.ErrAtprotoServiceTokenReplayed):
		return "replayed"
	case errors.Is(err, errATProtoTokenLifetime):
		return "lifetime"
	case errors.Is(err, jwt.ErrTokenExpired):
		return "expired"
	case errors.Is(err, jwt.ErrTokenInvalidAudience):
		return "audience"
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return "signature"
	case errors.Is(err, jwt.ErrTokenInvalidIssuer):
		return "issuer"
	default:
		return "invalid"
	}
}

// atprotoHandle returns the DID's handle when it resolves back to the DID.
func (s *HTTPServer) atprotoHandle(ctx context.Context, signIn *atprotoSignIn, did syntax.DID) string {
	ident, err := signIn.validator.Dir.LookupDID(ctx, did)
	if err != nil || ident.Handle.IsInvalidHandle() {
		return ""
	}
	return ident.Handle.String()
}

// revokeCurrentCookieSession ends the cookie session that this request created.
func (s *HTTPServer) revokeCurrentCookieSession(c *gin.Context) {
	if id, ok := s.browserSessionID(c); ok {
		_ = s.core.RevokeCookieSession(c.Request.Context(), id)
	}
	s.clearBrowserSessionCookie(c)
	clearCSRFCookie(c)
}
