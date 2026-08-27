package http_server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserSessionCookieNamesAreBoundedAndStrict(t *testing.T) {
	seen := make(map[string]struct{})
	for range 32 {
		name, err := newBrowserSessionCookieName()
		if err != nil {
			t.Fatalf("newBrowserSessionCookieName: %v", err)
		}
		if !isBrowserSessionCookieName(name) {
			t.Fatalf("generated cookie name %q was rejected", name)
		}
		if _, ok := seen[name]; ok {
			t.Fatalf("generated duplicate cookie name %q", name)
		}
		seen[name] = struct{}{}
	}

	for _, name := range []string{
		"chatto_auth_short",
		"chatto_auth_000000000000000000000!",
		"chatto_auth_00000000000000000000000",
		"chatto_auth_attacker.example",
	} {
		if isBrowserSessionCookieName(name) {
			t.Fatalf("malformed cookie name %q was accepted", name)
		}
	}
}

func TestBrowserSessionCookieParsingRejectsAmplification(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://chatto.example.test/", nil)
	for index := 0; index < browserSessionCookieLimit+1; index++ {
		name, err := newBrowserSessionCookieName()
		if err != nil {
			t.Fatalf("newBrowserSessionCookieName: %v", err)
		}
		request.AddCookie(&http.Cookie{Name: name, Value: string(rune('a' + index))})
	}
	if _, err := browserSessionCookies(request); err == nil {
		t.Fatalf("browserSessionCookies accepted more than %d cookie slots", browserSessionCookieLimit)
	}
}

func TestBrowserSessionCookieParsingDeduplicatesOneHandleAcrossSlots(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://chatto.example.test/", nil)
	for range browserSessionCookieLimit + 1 {
		name, err := newBrowserSessionCookieName()
		if err != nil {
			t.Fatalf("newBrowserSessionCookieName: %v", err)
		}
		request.AddCookie(&http.Cookie{Name: name, Value: "same-handle"})
	}
	cookies, err := browserSessionCookies(request)
	if err != nil {
		t.Fatalf("browserSessionCookies: %v", err)
	}
	if len(cookies) != 1 || cookies[0].token != "same-handle" {
		t.Fatalf("deduplicated cookies = %#v, want one same-handle entry", cookies)
	}
}
