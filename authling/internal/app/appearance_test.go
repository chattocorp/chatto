package app

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/evtstream"
	"github.com/chattocorp/chatto/authling/internal/sitesettings"
	"github.com/chattocorp/chatto/authling/internal/storage"
	"github.com/chattocorp/chatto/authling/internal/web"
)

func TestAppearanceAuthorizationSaveAndRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.Site = config.SiteConfig{Name: "Configured name"}
	cfg.Administration.AdminEmails = []string{"admin@example.invalid"}
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntimeWithOptions(t, cfg, sender, nil)
	defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
	admin, err := runtime.Accounts.CreateLocal(t.Context(), "admin@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := runtime.Accounts.CreateLocal(t.Context(), "ordinary@example.invalid", "another deliberately uncommon password")
	if err != nil {
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
	handler := func() http.Handler {
		return web.Handler(web.Dependencies{Accounts: runtime.Accounts, Sessions: runtime.Sessions, Administration: cfg.Administration, SiteSettings: runtime.SiteSettings, PublicURL: "http://localhost"})
	}
	request := func(method, path, token, origin string, form url.Values, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(form.Encode()))
		if method == "POST" {
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Origin", origin)
		}
		if token != "" {
			r.AddCookie(&http.Cookie{Name: "authling_session", Value: token})
		}
		w := httptest.NewRecorder()
		handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s status %d, want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	const path = "/admin/appearance"
	form := url.Values{"name": {"New <script>name</script>"}, "description": {"Public description"}, "introduction": {"Welcome <img src=x onerror=alert(1)>"}, "revision": {"0"}, "action": {"preview"}}
	request("GET", path, "", "", nil, http.StatusSeeOther)
	request("GET", path, ordinaryToken, "", nil, http.StatusForbidden)
	request("POST", path, ordinaryToken, "http://localhost", form, http.StatusForbidden)
	request("POST", path, adminToken, "https://attacker.invalid", form, http.StatusForbidden)
	request("POST", path, adminToken, "", form, http.StatusForbidden)
	request("POST", path, adminToken, "http://localhost", form, http.StatusBadRequest)
	snapshot, err := runtime.SiteSettings.Read(t.Context())
	if err != nil || snapshot.Revision != 0 || snapshot.Site.Name != cfg.Site.Name {
		t.Fatalf("unsupported action persisted: %v", err)
	}
	form.Set("action", "save")
	form.Set("name", "Bad\r\nBcc: header")
	request("POST", path, adminToken, "http://localhost", form, http.StatusUnprocessableEntity)
	form.Set("name", "New identity")
	request("POST", path, adminToken, "http://localhost", form, http.StatusSeeOther)
	js, err := runtime.connection.NATS.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	record, err := js.GetLastMsg(storage.EventStreamName, evtstream.SiteSettingsSubject())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := evtstream.Decode(record.Data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Event.GetSiteSettingsChanged().GetActorAccountId() != admin.ID || decoded.Event.GetCreatedAt() == nil {
		t.Fatal("settings event lacks actor or timestamp")
	}
	if decoded.Event.GetSiteSettingsChanged().GetSettings().GetName() != "New identity" || decoded.Event.GetSiteSettingsChanged().GetSettings().GetIntroduction() != form.Get("introduction") {
		t.Fatal("settings event does not record public replacement values")
	}
	if bytes.Contains(record.Data, []byte("admin@example.invalid")) {
		t.Fatal("settings event leaked administrator email")
	}
	if _, err := js.KeyValue("AUTHLING_SETTINGS"); err == nil {
		t.Fatal("obsolete settings bucket was provisioned")
	}
	request("POST", path, adminToken, "http://localhost", form, http.StatusConflict)
	login := request("GET", "/login", "", "", nil, http.StatusOK)
	if !strings.Contains(login.Body.String(), "New identity") || !strings.Contains(login.Body.String(), "Welcome &lt;img") || !strings.Contains(login.Body.String(), `content="Public description"`) {
		t.Fatal("saved branding not applied to login")
	}
	if _, err := runtime.Registration.Start(t.Context(), "new@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sender.messages[len(sender.messages)-1].Subject, "New identity") {
		t.Fatal("email used stale branding")
	}
	stopTestRuntime(t, runtime, cancel, runErrors)
	cfg.Site.Name = "Changed configuration"
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	snapshot, err = runtime.SiteSettings.Read(t.Context())
	if err != nil || snapshot.Site.Name != "New identity" || snapshot.Revision != record.Sequence || snapshot.ActorAccountID != admin.ID || !snapshot.ChangedAt.Equal(decoded.Event.CreatedAt.AsTime()) {
		t.Fatalf("saved settings lost on restart: %v", err)
	}
	// Two editors of one revision can never both commit. No stale input is
	// automatically retried with a newer revision.
	results := make(chan error, 2)
	for _, name := range []string{"First editor", "Second editor"} {
		go func(name string) {
			input := snapshot
			input.Site.Name = name
			results <- runtime.SiteSettings.Save(t.Context(), admin.ID, input)
		}(name)
	}
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, sitesettings.ErrConflict)) || (second == nil && errors.Is(first, sitesettings.ErrConflict))) {
		t.Fatalf("concurrent saves: %v, %v", first, second)
	}
	js, err = runtime.connection.NATS.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	historical, err := js.GetMsg(storage.EventStreamName, record.Sequence)
	if err != nil || !bytes.Equal(historical.Data, record.Data) {
		t.Fatal("settings change history was not retained")
	}
	// A stale form cannot confer administrator access after the flag is removed.
	if err := runtime.Accounts.SetAdministrator(t.Context(), ordinary.ID, true); err != nil {
		t.Fatal(err)
	}
	current, err := runtime.SiteSettings.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Accounts.SetAdministrator(t.Context(), ordinary.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SiteSettings.Save(t.Context(), ordinary.ID, current); !errors.Is(err, sitesettings.ErrForbidden) {
		t.Fatalf("revoked admin save: %v", err)
	}
}
