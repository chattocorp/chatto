package connectapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
)

func TestLimitUploadChunks(t *testing.T) {
	t.Parallel()
	const limit = 2
	var inside atomic.Int32
	entered := make(chan struct{}, limit+1)
	release := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == apiv1connect.AssetUploadServiceUploadChunkProcedure {
			inside.Add(1)
			entered <- struct{}{}
			<-release
			inside.Add(-1)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := limitUploadChunks(next, limit, HandlerOptions()...)
	serve := func(ctx context.Context, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	var wg sync.WaitGroup
	for range limit + 1 {
		wg.Go(func() {
			if rec := serve(t.Context(), apiv1connect.AssetUploadServiceUploadChunkProcedure); rec.Code != http.StatusOK {
				t.Errorf("chunk status = %d", rec.Code)
			}
		})
	}
	for range limit {
		<-entered
	}
	select {
	case <-entered:
		t.Fatal("a chunk request entered while all slots were in use")
	case <-time.After(100 * time.Millisecond):
	}
	if inside.Load() != limit {
		t.Fatalf("%d chunk requests inside, want %d", inside.Load(), limit)
	}

	// Other procedures do not wait for a slot.
	if rec := serve(t.Context(), apiv1connect.AssetUploadServiceCreateUploadProcedure); rec.Code != http.StatusOK {
		t.Fatalf("create status = %d", rec.Code)
	}

	// A chunk request that ends while it waits gets a Connect error.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	rec := serve(ctx, apiv1connect.AssetUploadServiceUploadChunkProcedure)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"unavailable"`) {
		t.Fatalf("waiting chunk response = %d %s, want a Connect unavailable error", rec.Code, rec.Body.String())
	}

	close(release)
	wg.Wait()
	if inside.Load() != 0 {
		t.Fatalf("%d chunk requests still inside", inside.Load())
	}
}
