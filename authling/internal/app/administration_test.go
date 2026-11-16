package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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
	if !strings.Contains(w.Body.String(), "Via server configuration") || !strings.Contains(w.Body.String(), "o•••@e•••.invalid") || !strings.Contains(w.Body.String(), "&lt;•••") || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("directory content or escaping is incorrect")
	}
	// The directory shows only masked personal data.
	if strings.Contains(w.Body.String(), "ordinary@example.invalid") || strings.Contains(w.Body.String(), "alert(1)") {
		t.Fatal("directory shows verbatim personal data")
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
	rows, more, err := runtime.Accounts.RegisteredAccounts(t.Context(), accounts.DirectoryQuery{Sort: accounts.SortByID, Limit: 1}, nil)
	if err != nil || len(rows) != 1 || !more {
		t.Fatalf("first page: %v", err)
	}
	second, more, err := runtime.Accounts.RegisteredAccounts(t.Context(), accounts.DirectoryQuery{Sort: accounts.SortByID, Offset: 1, Limit: 1}, nil)
	if err != nil || len(second) != 1 || more || second[0].ID == rows[0].ID {
		t.Fatalf("second page: %v", err)
	}
	current, _ := runtime.Accounts.Get(ordinary.ID)
	if err := runtime.Accounts.RequestErasure(t.Context(), ordinary.ID, current.AuthenticationVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Accounts.Administrator(t.Context(), ordinary.ID, nil); err != accounts.ErrInvalidCredentials {
		t.Fatalf("erased admin: %v", err)
	}
	rows, _, err = runtime.Accounts.RegisteredAccounts(t.Context(), accounts.DirectoryQuery{Sort: accounts.SortByID, Limit: 50}, nil)
	if err != nil || len(rows) != 1 || rows[0].ID != admin.ID {
		t.Fatalf("erased account remains in directory: %v", err)
	}
}

func TestAdministrationDirectorySort(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer func() { stopTestRuntime(t, runtime, cancel, runErrors) }()
	password := "a deliberately uncommon administrator password"
	// Registration order differs from access and ID order.
	type fixture struct{ email, username, fullName string }
	fixtures := []fixture{
		{"charlie@example.invalid", "zed", ""},
		{"alice@example.invalid", "alice", "Bravo Person"},
		{"bob@example.invalid", "bob", "alpha person"},
	}
	var registered []string
	for _, f := range fixtures {
		account, err := runtime.Accounts.CreateLocal(t.Context(), f.email, password)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Accounts.UpdateProfile(t.Context(), account.ID, f.username, f.fullName); err != nil {
			t.Fatal(err)
		}
		registered = append(registered, account.ID)
	}
	charlie, alice, bob := registered[0], registered[1], registered[2]
	if err := runtime.Accounts.SetAdministrator(t.Context(), bob, true); err != nil {
		t.Fatal(err)
	}
	adminEmails := []string{"charlie@example.invalid"}
	order := func(query accounts.DirectoryQuery) []accounts.RegisteredAccount {
		t.Helper()
		query.Limit = 50
		rows, more, err := runtime.Accounts.RegisteredAccounts(t.Context(), query, adminEmails)
		if err != nil || more {
			t.Fatalf("directory: more=%v err=%v", more, err)
		}
		return rows
	}
	expect := func(query accounts.DirectoryQuery, want ...string) {
		t.Helper()
		var got []string
		for _, row := range order(query) {
			got = append(got, row.ID)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%+v order = %v, want %v", query, got, want)
		}
	}
	expect(accounts.DirectoryQuery{Sort: accounts.SortByRegistered}, charlie, alice, bob)
	expect(accounts.DirectoryQuery{Sort: accounts.SortByRegistered, Descending: true}, bob, alice, charlie)
	ids := slices.Sorted(slices.Values(registered))
	expect(accounts.DirectoryQuery{Sort: accounts.SortByID}, ids...)
	// Stored and configured administrators come first; ID breaks the tie.
	if rows := order(accounts.DirectoryQuery{Sort: accounts.SortByAccess}); rows[2].ID != alice {
		t.Fatalf("access order = %v", rows)
	}
	if rows := order(accounts.DirectoryQuery{Sort: accounts.SortByAccess, Descending: true}); rows[0].ID != alice {
		t.Fatalf("descending access order = %v", rows)
	}
	// Rows contain masked hints only. Name uses the preferred username when
	// the account has no full name.
	hints := map[string]string{}
	for _, row := range order(accounts.DirectoryQuery{Sort: accounts.SortByRegistered}) {
		hints[row.ID] = row.EmailHint + " " + row.NameHint
	}
	if hints[charlie] != "c•••@e•••.invalid z•••" || hints[alice] != "a•••@e•••.invalid B•••" || hints[bob] != "b•••@e•••.invalid a•••" {
		t.Fatalf("hints = %v", hints)
	}
	// Pages slice the sorted directory.
	page, more, err := runtime.Accounts.RegisteredAccounts(t.Context(), accounts.DirectoryQuery{Sort: accounts.SortByRegistered, Offset: 1, Limit: 1}, adminEmails)
	if err != nil || !more || len(page) != 1 || page[0].ID != alice {
		t.Fatalf("sorted page: %v", err)
	}
	page, more, err = runtime.Accounts.RegisteredAccounts(t.Context(), accounts.DirectoryQuery{Sort: accounts.SortByRegistered, Offset: 3, Limit: 1}, adminEmails)
	if err != nil || more || len(page) != 0 {
		t.Fatalf("page past the end: %v", err)
	}
	for _, sort := range []accounts.DirectorySort{"password", "email", "name"} {
		if _, _, err := runtime.Accounts.RegisteredAccounts(t.Context(), accounts.DirectoryQuery{Sort: sort, Limit: 1}, adminEmails); err == nil {
			t.Fatalf("sort %q accepted", sort)
		}
	}

	token, _, err := runtime.Sessions.Create(t.Context(), charlie)
	if err != nil {
		t.Fatal(err)
	}
	h := web.Handler(web.Dependencies{Accounts: runtime.Accounts, Sessions: runtime.Sessions, Administration: config.AdministrationConfig{AdminEmails: adminEmails}})
	get := func(target string, want int) string {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.AddCookie(&http.Cookie{Name: "authling_session", Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s status = %d, want %d", target, w.Code, want)
		}
		return w.Body.String()
	}
	for _, target := range []string{"/admin?sort=password", "/admin?sort=email", "/admin?sort=name", "/admin?order=up", "/admin?page=0", "/admin?page=x", "/admin?page=1000001"} {
		get(target, http.StatusBadRequest)
	}
	body := get("/admin?sort=registered&order=asc", http.StatusOK)
	// The header shows the signed-in administrator's own email, so inspect
	// only the rows.
	rows := body[strings.Index(body, "<tbody>"):]
	if strings.Index(rows, "account-"+charlie) > strings.Index(rows, "account-"+bob) {
		t.Fatal("rendered directory ignores the sort")
	}
	for _, private := range []string{"@example.invalid", "Bravo Person", "alpha person", "zed"} {
		if strings.Contains(rows, private) {
			t.Fatalf("directory shows verbatim personal data")
		}
	}
	// The current column reverses its order; masked columns cannot sort.
	if !strings.Contains(body, `aria-sort="ascending"`) || !strings.Contains(body, `href="/admin"`) || strings.Contains(body, "sort=email") || strings.Contains(body, "sort=name") {
		t.Fatal("sort links are incorrect")
	}
	if strings.Contains(body, "Load more users") {
		t.Fatal("single page shows a load-more row")
	}

	// A full page ends with a row that loads the next page in the same order.
	for i := range 48 {
		if _, err := runtime.Accounts.CreateLocal(t.Context(), fmt.Sprintf("user%02d@example.invalid", i), password); err != nil {
			t.Fatal(err)
		}
	}
	body = get("/admin?sort=id", http.StatusOK)
	if strings.Count(body, `class="whitespace-nowrap"`) != 50 || !strings.Contains(body, `hx-get="/admin?page=2&amp;sort=id"`) || !strings.Contains(body, `href="/admin?page=2&amp;sort=id"`) || strings.Contains(body, "First page") {
		t.Fatal("first page does not offer the next page")
	}
	body = get("/admin?sort=id&page=2", http.StatusOK)
	if strings.Count(body, `class="whitespace-nowrap"`) != 1 || strings.Contains(body, "Load more users") || !strings.Contains(body, `href="/admin?sort=id"`) {
		t.Fatal("last page is incorrect")
	}
}
