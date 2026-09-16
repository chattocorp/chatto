package web

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/oidcprovider"
)

func TestSiteSettingsAreLocalToEachHandler(t *testing.T) {
	first := Handler(Dependencies{Site: config.SiteConfig{Name: "chatto.id", Description: "Your account for Chatto."}})
	second := Handler(Dependencies{Site: config.SiteConfig{Name: "Other accounts"}})
	for _, test := range []struct {
		handler http.Handler
		name    string
	}{{first, "chatto.id"}, {second, "Other accounts"}, {first, "chatto.id"}} {
		for _, path := range []string{"/", "/login", "/signup", "/password-reset"} {
			response := httptest.NewRecorder()
			test.handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.name) {
				t.Fatalf("%s did not use site name", path)
			}
			if strings.Contains(response.Body.String(), "Authling") {
				t.Fatalf("%s exposes software branding", path)
			}
		}
	}
	handler := Handler(Dependencies{PublicURL: "https://accounts.example.com:8443"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://accounts.example.com:8443/", nil))
	if !strings.Contains(response.Body.String(), "Welcome to accounts.example.com") {
		t.Fatal("missing configured hostname fallback")
	}
}

func TestSiteCopyIsEscapedAcrossBrowserPages(t *testing.T) {
	site := config.SiteConfig{Name: `Accounts <script>alert(1)</script>`, Description: `Description " onload="alert(1)`}
	ctx := context.WithValue(t.Context(), siteContextKey{}, site)
	for name, page := range map[string]templ.Component{
		"home": homePage(), "login": loginPage("", "", ""), "signup": signupPage("", ""),
		"password reset": newPasswordPage("", "", 10, ""),
		"email change":   emailChangePage("", ""), "email confirmation": emailChangeConfirmPage("", "", ""),
		"password change": passwordChangePage("", 10, ""),
		"account":         accountPage("acc_test", "", nil, nil, false, false, false, false, false, false, false, false, false),
		"consent":         consentPage(oidcprovider.ConsentRequest{ClientName: "Example app"}, "", "acc_test", accounts.Profile{}),
		"delete":          deleteAccountPage(""), "deleted": accountDeletedPage(),
	} {
		t.Run(name, func(t *testing.T) {
			var body strings.Builder
			if err := page.Render(ctx, &body); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(body.String(), "<script>") || strings.Contains(body.String(), `content="Description "`) {
				t.Fatal("site text escaped its HTML context")
			}
			if !strings.Contains(body.String(), html.EscapeString(site.Name)) {
				t.Fatal("missing escaped site name")
			}
			if strings.Contains(body.String(), "Authling") {
				t.Fatal("software name remains in browser copy")
			}
			if name == "consent" && !strings.Contains(body.String(), "Example app") {
				t.Fatal("branding replaced the connected app name")
			}
		})
	}
}
