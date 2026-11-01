package chatto

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBrowserLoginPKCEAndState(t *testing.T) {
	var challenge, redirect string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" || r.Header.Get("Authorization") != "" {
			t.Error("incorrect code exchange")
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		h := sha256.Sum256([]byte(in["code_verifier"]))
		if base64.RawURLEncoding.EncodeToString(h[:]) != challenge || in["redirect_uri"] != redirect || in["code"] != "test-code" || in["client_id"] != "chatto://loopback" {
			t.Error("PKCE or callback binding lost")
		}
		fmtJSON(w, `{"access_token":"access","refresh_token":"refresh","expires_in":300,"refresh_token_expires_in":86400}`)
	}))
	defer s.Close()
	got, err := LoginBrowser(context.Background(), s.URL, func(raw string) error {
		u, _ := url.Parse(raw)
		q := u.Query()
		challenge = q.Get("code_challenge")
		redirect = q.Get("redirect_uri")
		if q.Get("code_challenge_method") != "S256" || q.Get("state") == "" {
			t.Error("missing login protection")
		}
		callback, _ := url.Parse(redirect)
		cq := callback.Query()
		cq.Set("code", "test-code")
		cq.Set("state", "wrong")
		callback.RawQuery = cq.Encode()
		res, err := http.Get(callback.String())
		if err != nil {
			return err
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Error("invalid state accepted")
		}
		cq.Set("state", q.Get("state"))
		callback.RawQuery = cq.Encode()
		res, err = http.Get(callback.String())
		if err != nil {
			return err
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return nil
	})
	if err != nil || got.AccessToken != "access" || got.ClientID != "chatto://loopback" {
		t.Fatal("login failed", err)
	}
	client := http.Client{Timeout: time.Second}
	if res, err := client.Get(redirect); err == nil {
		res.Body.Close()
		t.Fatal("callback listener left open")
	}
}

func TestHumanSessionRefreshAndLogout(t *testing.T) {
	var refreshIDs []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/auth/login":
			if in["login"] != "tester" || in["password"] != "test-password" || r.Header.Get("Authorization") != "" {
				t.Error("bad direct login")
			}
			fmtJSON(w, `{"token":"old","refreshToken":"refresh","expiresIn":300,"refreshTokenExpiresIn":86400}`)
		case "/oauth/token":
			refreshIDs = append(refreshIDs, in["refresh_request_id"])
			if in["refresh_token"] != "refresh" || in["client_id"] != "" {
				t.Error("changed refresh scope")
			}
			if len(refreshIDs) == 1 {
				http.Error(w, "private provider error", 503)
				return
			}
			fmtJSON(w, `{"access_token":"new","refresh_token":"rotated","expires_in":300,"refresh_token_expires_in":86000}`)
		case "/auth/logout":
			if in["refreshToken"] != "rotated" || r.Header.Get("Authorization") != "Bearer new" {
				t.Error("wrong logout credentials")
			}
			fmtJSON(w, `{"success":true}`)
		default:
			t.Error("unexpected route")
		}
	}))
	defer s.Close()
	session, err := LoginPassword(context.Background(), s.URL, "tester", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.Refresh(context.Background()); err == nil {
		t.Fatal("refresh without durable request identity accepted")
	}
	session.PrepareRefresh()
	id := session.RefreshRequestID
	session.PrepareRefresh()
	if session.RefreshRequestID != id {
		t.Fatal("changed retry identity")
	}
	if _, err := session.Refresh(context.Background()); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("missing safe failure")
	}
	rotated, err := session.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(refreshIDs) != 2 || refreshIDs[0] != refreshIDs[1] || rotated.RefreshRequestID != "" {
		t.Fatal("refresh retry contract lost")
	}
	if err := rotated.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserLoginCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := LoginBrowser(ctx, "https://example.test", func(string) error { cancel(); return nil })
	if err != context.Canceled {
		t.Fatal(err)
	}
}
