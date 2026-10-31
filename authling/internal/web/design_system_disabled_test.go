//go:build !bootstrap

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProductionHasNoDesignSystemRoute(t *testing.T) {
	r := httptest.NewRecorder()
	Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/dev/ui", nil))
	if r.Code != http.StatusNotFound {
		t.Fatalf("production preview status = %d, want 404", r.Code)
	}
}
