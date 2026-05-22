package http_server

import "testing"

func TestNormalizeLoopbackHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://localhost:5173", "http://127.0.0.1:5173"},
		{"http://localhost", "http://127.0.0.1"},
		{"http://localhost:5173/auth/atproto/callback", "http://127.0.0.1:5173/auth/atproto/callback"},
		{"http://127.0.0.1:5173", "http://127.0.0.1:5173"},
		{"https://chatto.example.com", "https://chatto.example.com"},
		{"https://chatto.example.com/auth/atproto/callback", "https://chatto.example.com/auth/atproto/callback"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeLoopbackHost(tc.in); got != tc.want {
				t.Errorf("normalizeLoopbackHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsLocalhostURL(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"http://localhost:8080", true},
		{"https://localhost", true},
		{"http://127.0.0.1:3000", true},
		{"http://[::1]:8080", true},
		{"https://chatto.example.com", false},
		{"https://localhost.example.com", false},
		{"", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := isLocalhostURL(tc.in); got != tc.want {
				t.Errorf("isLocalhostURL(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
