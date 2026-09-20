package http_server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

// TestOIDCConformance drives the production login and callback handlers against
// the official suite. It is opt-in because the suite is a separate local process.
// Negative tests assert Chatto's decision as well as the suite's protocol result.
func TestOIDCConformance(t *testing.T) {
	issuer := os.Getenv("CHATTO_CONFORMANCE_ISSUER")
	if issuer == "" {
		t.Skip("requires the local official OIDC suite")
	}
	// Keep hostname and certificate verification intact while resolving the
	// isolated suite locally. No machine-wide DNS or trust changes are needed.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	certificate, err := os.ReadFile(os.Getenv("SSL_CERT_FILE"))
	if err != nil {
		t.Fatal("cannot read conformance proxy certificate")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		t.Fatal("invalid conformance proxy certificate")
	}
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err == nil && host == "conformance.localhost" {
			address = net.JoinHostPort("127.0.0.1", port)
		}
		return dialer.DialContext(ctx, network, address)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous; transport.CloseIdleConnections() })
	ts, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = "http://localhost:19410"
		s.config.Auth.Providers = []config.AuthProviderConfig{{
			ID: "conformance", Type: config.AuthProviderTypeOpenIDConnect,
			IssuerURL: issuer, ClientID: "chatto-conformance",
			ClientSecret: os.Getenv("CHATTO_CONFORMANCE_SECRET"), AutoProvision: boolPtr(true),
			TokenEndpointAuthMethod: os.Getenv("CHATTO_CONFORMANCE_AUTH_METHOD"),
		}}
		s.setupOIDCRoutes()
	})
	client.Timeout = 20 * time.Second
	get := func(target string) *http.Response {
		t.Helper()
		response, err := client.Get(target)
		if err != nil {
			if failure, ok := err.(*url.Error); ok {
				t.Fatalf("conformance transport failure: %T", failure.Err)
			}
			t.Fatal("conformance HTTP request failed")
		}
		response.Body.Close()
		return response
	}
	login := get(ts.URL + "/auth/providers/conformance")
	if login.StatusCode != http.StatusTemporaryRedirect {
		t.Fatal("login did not redirect")
	}
	authorization := get(login.Header.Get("Location"))
	callback, err := url.Parse(authorization.Header.Get("Location"))
	if err != nil || callback.Path != "/auth/providers/conformance/callback" || callback.Query().Get("code") == "" {
		t.Fatal("suite did not return an authorization code")
	}
	// The suite's registered callback is stable; the local test listener uses
	// an ephemeral port. Preserve the exact path, state, and code at the handler.
	response := get(ts.URL + callback.RequestURI())
	destination, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal("invalid callback redirect")
	}
	if os.Getenv("CHATTO_CONFORMANCE_REJECT") == "true" {
		if destination.Path != "/login" || destination.Query().Get("error") != "provider_failed" {
			t.Fatal("Chatto accepted an invalid provider response")
		}
		return
	}
	if destination.Path != "/sso/confirm" {
		t.Fatalf("Chatto rejected a valid provider response (HTTP %d, provider_failed=%t)", response.StatusCode, destination.Query().Get("error") == "provider_failed")
	}
	flow, err := chattoCore.GetPendingExternalIdentityCreateFlow(t.Context(), destination.Query().Get("token"))
	if err != nil || flow.Subject == "" || flow.LoginHint != "d.tu" || flow.DisplayNameHint != "Demo T. User" {
		t.Fatal("Chatto did not preserve the suite's profile claims")
	}
}
