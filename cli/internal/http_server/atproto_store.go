package http_server

import (
	"context"
	"errors"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"hmans.de/chatto/internal/core"
	atprotov1 "hmans.de/chatto/internal/pb/chatto/atproto/v1"
)

// atprotoOAuthStore adapts indigo's oauth.ClientAuthStore interface to the
// NATS-KV-backed CRUD on ChattoCore. The adapter is the only place that
// knows about both the indigo struct shapes and our protobuf shapes — core
// stays library-agnostic and the HTTP handler doesn't touch KV.
type atprotoOAuthStore struct {
	core *core.ChattoCore
}

func newATProtoOAuthStore(c *core.ChattoCore) *atprotoOAuthStore {
	return &atprotoOAuthStore{core: c}
}

// Ensure the adapter implements the interface at compile time.
var _ oauth.ClientAuthStore = (*atprotoOAuthStore)(nil)

func (s *atprotoOAuthStore) SaveAuthRequestInfo(ctx context.Context, info oauth.AuthRequestData) error {
	return s.core.SaveATProtoAuthRequest(ctx, authRequestToProto(info))
}

func (s *atprotoOAuthStore) GetAuthRequestInfo(ctx context.Context, state string) (*oauth.AuthRequestData, error) {
	pb, err := s.core.GetATProtoAuthRequest(ctx, state)
	if err != nil {
		return nil, err
	}
	out := authRequestFromProto(pb)
	return &out, nil
}

func (s *atprotoOAuthStore) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	return s.core.DeleteATProtoAuthRequest(ctx, state)
}

func (s *atprotoOAuthStore) SaveSession(ctx context.Context, sess oauth.ClientSessionData) error {
	return s.core.SaveATProtoSession(ctx, sessionToProto(sess))
}

func (s *atprotoOAuthStore) GetSession(ctx context.Context, did syntax.DID, sessionID string) (*oauth.ClientSessionData, error) {
	pb, err := s.core.GetATProtoSession(ctx, did.String(), sessionID)
	if err != nil {
		return nil, err
	}
	return sessionFromProto(pb)
}

func (s *atprotoOAuthStore) DeleteSession(ctx context.Context, did syntax.DID, sessionID string) error {
	return s.core.DeleteATProtoSession(ctx, did.String(), sessionID)
}

// ---- conversions -----------------------------------------------------------

func authRequestToProto(in oauth.AuthRequestData) *atprotov1.ATProtoAuthRequest {
	var accountDID string
	if in.AccountDID != nil {
		accountDID = in.AccountDID.String()
	}
	return &atprotov1.ATProtoAuthRequest{
		State:                        in.State,
		AuthServerUrl:                in.AuthServerURL,
		AccountDid:                   accountDID,
		Scopes:                       in.Scopes,
		RequestUri:                   in.RequestURI,
		AuthServerTokenEndpoint:      in.AuthServerTokenEndpoint,
		AuthServerRevocationEndpoint: in.AuthServerRevocationEndpoint,
		PkceVerifier:                 in.PKCEVerifier,
		DpopAuthServerNonce:          in.DPoPAuthServerNonce,
		DpopPrivateKeyMultibase:      in.DPoPPrivateKeyMultibase,
	}
}

func authRequestFromProto(in *atprotov1.ATProtoAuthRequest) oauth.AuthRequestData {
	var accountDID *syntax.DID
	if in.AccountDid != "" {
		if did, err := syntax.ParseDID(in.AccountDid); err == nil {
			accountDID = &did
		}
	}
	return oauth.AuthRequestData{
		State:                        in.State,
		AuthServerURL:                in.AuthServerUrl,
		AccountDID:                   accountDID,
		Scopes:                       in.Scopes,
		RequestURI:                   in.RequestUri,
		AuthServerTokenEndpoint:      in.AuthServerTokenEndpoint,
		AuthServerRevocationEndpoint: in.AuthServerRevocationEndpoint,
		PKCEVerifier:                 in.PkceVerifier,
		DPoPAuthServerNonce:          in.DpopAuthServerNonce,
		DPoPPrivateKeyMultibase:      in.DpopPrivateKeyMultibase,
	}
}

func sessionToProto(in oauth.ClientSessionData) *atprotov1.ATProtoSession {
	return &atprotov1.ATProtoSession{
		AccountDid:                   in.AccountDID.String(),
		SessionId:                    in.SessionID,
		HostUrl:                      in.HostURL,
		AuthServerUrl:                in.AuthServerURL,
		AuthServerTokenEndpoint:      in.AuthServerTokenEndpoint,
		AuthServerRevocationEndpoint: in.AuthServerRevocationEndpoint,
		Scopes:                       strings.Join(in.Scopes, " "),
		AccessToken:                  in.AccessToken,
		RefreshToken:                 in.RefreshToken,
		DpopAuthServerNonce:          in.DPoPAuthServerNonce,
		DpopHostNonce:                in.DPoPHostNonce,
		DpopPrivateKeyMultibase:      in.DPoPPrivateKeyMultibase,
	}
}

func sessionFromProto(in *atprotov1.ATProtoSession) (*oauth.ClientSessionData, error) {
	did, err := syntax.ParseDID(in.AccountDid)
	if err != nil {
		// Stored DID is corrupt — treat as not-found so indigo's caller
		// surfaces a clean error rather than a parse failure.
		return nil, errors.Join(core.ErrNotFound, err)
	}
	return &oauth.ClientSessionData{
		AccountDID:                   did,
		SessionID:                    in.SessionId,
		HostURL:                      in.HostUrl,
		AuthServerURL:                in.AuthServerUrl,
		AuthServerTokenEndpoint:      in.AuthServerTokenEndpoint,
		AuthServerRevocationEndpoint: in.AuthServerRevocationEndpoint,
		Scopes:                       splitScopes(in.Scopes),
		AccessToken:                  in.AccessToken,
		RefreshToken:                 in.RefreshToken,
		DPoPAuthServerNonce:          in.DpopAuthServerNonce,
		DPoPHostNonce:                in.DpopHostNonce,
		DPoPPrivateKeyMultibase:      in.DpopPrivateKeyMultibase,
	}, nil
}

// splitScopes parses a space-joined scope string, skipping empty entries so
// leading/trailing/duplicate spaces don't produce blank scopes.
func splitScopes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, " ")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
