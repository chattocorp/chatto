package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/chattocorp/chatto/authling/internal/accounts"
	"github.com/chattocorp/chatto/authling/internal/sessions"
	"github.com/chattocorp/chatto/authling/internal/web/ui"
)

// directoryPageSize is the number of users on one directory page.
const directoryPageSize = 50

// maxDirectoryPage bounds the page parameter so offsets cannot overflow.
const maxDirectoryPage = 1_000_000

// directoryView is the validated sort and page of the account directory. Its
// URL parameters contain no personal data: sort, order, and a page number.
type directoryView struct {
	sort       accounts.DirectorySort
	descending bool
	page       int
}

// defaultDirectoryView shows the newest registrations first.
var defaultDirectoryView = directoryView{sort: accounts.SortByRegistered, descending: true, page: 1}

// parseDirectoryView validates the sort, order, and page query parameters.
// Absent parameters use the default view; unknown values are rejected.
func parseDirectoryView(query url.Values) (directoryView, bool) {
	view := defaultDirectoryView
	if raw := query.Get("sort"); raw != "" {
		view.sort = accounts.DirectorySort(raw)
		if !view.sort.Valid() {
			return directoryView{}, false
		}
		view.descending = defaultDescending(view.sort)
	}
	switch query.Get("order") {
	case "":
	case "asc":
		view.descending = false
	case "desc":
		view.descending = true
	default:
		return directoryView{}, false
	}
	if raw := query.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 || page > maxDirectoryPage {
			return directoryView{}, false
		}
		view.page = page
	}
	return view, true
}

// defaultDescending puts the newest registrations first; text columns and
// access start in ascending order.
func defaultDescending(sort accounts.DirectorySort) bool {
	return sort == accounts.SortByRegistered
}

// query converts the view to an account query for its page.
func (v directoryView) query() accounts.DirectoryQuery {
	return accounts.DirectoryQuery{Sort: v.sort, Descending: v.descending, Offset: (v.page - 1) * directoryPageSize, Limit: directoryPageSize}
}

// url returns the directory URL for this view. It omits default values.
func (v directoryView) url() string {
	values := url.Values{}
	if v.sort != defaultDirectoryView.sort {
		values.Set("sort", string(v.sort))
	}
	if v.descending != defaultDescending(v.sort) {
		if v.descending {
			values.Set("order", "desc")
		} else {
			values.Set("order", "asc")
		}
	}
	if v.page > 1 {
		values.Set("page", strconv.Itoa(v.page))
	}
	if len(values) == 0 {
		return "/admin"
	}
	return "/admin?" + values.Encode()
}

// column returns a header that sorts by the given column from the first page.
// The current sort column reverses its order; another column uses its default.
func (v directoryView) column(label string, sort accounts.DirectorySort) ui.Column {
	target := directoryView{sort: sort, descending: defaultDescending(sort), page: 1}
	direction := ui.Unsorted
	if v.sort == sort {
		target.descending = !v.descending
		direction = ui.Ascending
		if v.descending {
			direction = ui.Descending
		}
	}
	return ui.Column{Label: label, SortURL: target.url(), Direction: direction}
}

// firstPageURL returns the first page in the same order, or "" on page one.
func (v directoryView) firstPageURL() string {
	if v.page == 1 {
		return ""
	}
	v.page = 1
	return v.url()
}

// nextPageURL returns the following page in the same order, or "" if none.
func (v directoryView) nextPageURL(more bool) string {
	if !more {
		return ""
	}
	v.page++
	return v.url()
}

// mountAdministration exposes only a read-only, authenticated account directory.
func mountAdministration(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login?return_to=%2Fadmin")
			return
		}
		if err != nil {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		access, err := deps.Accounts.Administrator(r.Context(), account.ID, deps.Administration.AdminEmails)
		if err != nil {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		if !access.Allowed() {
			http.Error(w, "administrator access required", http.StatusForbidden)
			return
		}
		view, ok := parseDirectoryView(r.URL.Query())
		if !ok {
			http.Error(w, "invalid directory query", http.StatusBadRequest)
			return
		}
		rows, more, err := deps.Accounts.RegisteredAccounts(r.Context(), view.query(), deps.Administration.AdminEmails)
		if err != nil {
			http.Error(w, "account directory unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		// A long directory read must not retain privileges revoked while it ran.
		access, err = deps.Accounts.Administrator(r.Context(), account.ID, deps.Administration.AdminEmails)
		if err != nil || !access.Allowed() {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		render(w, r, http.StatusOK, administrationPage(email, rows, access.Configured, view, more))
	})
}
