//go:build test_endpoints

package http_server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchAndUploadAvatarFromURLRejectsOversizedResponse(t *testing.T) {
	avatarServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", int(oidcAvatarMaxBytes)+1)))
	}))
	defer avatarServer.Close()

	err := fetchAndUploadAvatarFromURL(context.Background(), avatarServer.URL, nil, "user-id")
	if err == nil {
		t.Fatal("expected oversized avatar response to fail")
	}
	if !strings.Contains(err.Error(), "avatar exceeds maximum size") {
		t.Fatalf("expected oversized avatar error, got %v", err)
	}
}
