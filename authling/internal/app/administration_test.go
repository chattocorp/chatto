package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/accounts"
	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/web"
)

func TestAdministrationAccessAndReplay(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
	password := "a deliberately uncommon administrator password"
	admin, err := runtime.Accounts.CreateLocal(t.Context(), "admin@example.invalid", password)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := runtime.Accounts.CreateLocal(t.Context(), "ordinary@example.invalid", password)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Accounts.UpdateProfile(t.Context(), ordinary.ID, "ordinary", "<script>alert(1)</script>"); err != nil {
		t.Fatal(err)
	}
	adminToken, _, err := runtime.Sessions.Create(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryToken, _, err := runtime.Sessions.Create(t.Context(), ordinary.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := func(emails ...string) http.Handler {
		return web.Handler(web.Dependencies{Accounts: runtime.Accounts, Sessions: runtime.Sessions, Administration: config.AdministrationConfig{AdminEmails: emails}})
	}
	request := func(h http.Handler, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/admin", nil)
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "authling_session", Value: token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("status = %d, want %d", w.Code, want)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("admin response can be cached")
		}
		if want != http.StatusOK && strings.Contains(w.Body.String(), "ordinary@example.invalid") {
			t.Fatal("directory leaked")
		}
		return w
	}
	h := handler(" ADMIN@EXAMPLE.INVALID ")
	request(h, "", http.StatusSeeOther)
	request(h, ordinaryToken, http.StatusForbidden)
	w := request(h, adminToken, http.StatusOK)
	if !strings.Contains(w.Body.String(), "Via server configuration") || !strings.Contains(w.Body.String(), "ordinary@example.invalid") || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("directory content or escaping is incorrect")
	}
	request(handler(), adminToken, http.StatusForbidden)
	if err := runtime.Accounts.SetAdministrator(t.Context(), ordinary.ID, true); err != nil {
		t.Fatal(err)
	}
	request(handler(), ordinaryToken, http.StatusOK)
	if err := runtime.Accounts.SetAdministrator(t.Context(), ordinary.ID, false); err != nil {
		t.Fatal(err)
	}
	request(handler(), ordinaryToken, http.StatusForbidden)
	// Moving away from the configured address invalidates the old session and
	// removes the override when the user signs in again.
	target, err := runtime.Accounts.PrepareEmailChange(t.Context(), admin.ID, password, "changed@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	target, err = runtime.Accounts.RecordEmailChangeRequested(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Accounts.ChangeEmail(t.Context(), target, "changed@example.invalid"); err != nil {
		t.Fatal(err)
	}
	request(h, adminToken, http.StatusSeeOther)
	adminToken, _, err = runtime.Sessions.Create(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	request(h, adminToken, http.StatusForbidden)
	if err := runtime.Accounts.SetAdministrator(t.Context(), ordinary.ID, true); err != nil {
		t.Fatal(err)
	}
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	access, err := runtime.Accounts.Administrator(t.Context(), ordinary.ID, nil)
	if err != nil || !access.Stored {
		t.Fatalf("replayed administrator: %v", err)
	}
	access, err = runtime.Accounts.Administrator(t.Context(), admin.ID, []string{"admin@example.invalid"})
	if err != nil || access.Allowed() {
		t.Fatalf("override persisted or ignored email change: %v", err)
	}
	rows, next, err := runtime.Accounts.RegisteredAccounts(t.Context(), "", 1, nil)
	if err != nil || len(rows) != 1 || next == "" {
		t.Fatalf("first page: %v", err)
	}
	second, next, err := runtime.Accounts.RegisteredAccounts(t.Context(), next, 1, nil)
	if err != nil || len(second) != 1 || next != "" || second[0].ID == rows[0].ID {
		t.Fatalf("second page: %v", err)
	}
	current, _ := runtime.Accounts.Get(ordinary.ID)
	if err := runtime.Accounts.RequestErasure(t.Context(), ordinary.ID, current.AuthenticationVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Accounts.Administrator(t.Context(), ordinary.ID, nil); err != accounts.ErrInvalidCredentials {
		t.Fatalf("erased admin: %v", err)
	}
	rows, _, err = runtime.Accounts.RegisteredAccounts(t.Context(), "", 50, nil)
	if err != nil || len(rows) != 1 || rows[0].ID != admin.ID {
		t.Fatalf("erased account remains in directory: %v", err)
	}
}
