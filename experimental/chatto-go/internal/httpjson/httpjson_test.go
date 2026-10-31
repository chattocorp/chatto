package httpjson

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRejectRedirectAndPrivateErrors(t *testing.T) {
	var visited atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { visited.Store(true) }))
	defer target.Close()
	for _, status := range []int{307, 401, 500} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			_, _ = w.Write([]byte("private-message-secret"))
		}))
		var out any
		err := Post(context.Background(), s.URL, "key", struct{}{}, &out)
		s.Close()
		if err == nil || strings.Contains(err.Error(), "private-message-secret") || visited.Load() {
			t.Fatalf("status=%d err=%v", status, err)
		}
	}
}
