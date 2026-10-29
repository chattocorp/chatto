package connectapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"connectrpc.com/authn"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
)

func TestUploadChunkLimiterSlots(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		releaseAll := sync.OnceFunc(func() { close(release) })
		// Let blocked requests end when the test fails early.
		defer releaseAll()
		inside := map[string]int{}
		entered := make(chan string, 10)
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == apiv1connect.AssetUploadServiceUploadChunkProcedure {
				entered <- authn.GetInfo(r.Context()).(Caller).UserID
				<-release
			}
			w.WriteHeader(http.StatusOK)
		})
		limiter := newUploadChunkLimiter(next, HandlerOptions()...)
		serve := func(ctx context.Context, userID, path string) *httptest.ResponseRecorder {
			req := httptest.NewRequestWithContext(authn.SetInfo(ctx, Caller{UserID: userID}), http.MethodPost, path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			limiter.ServeHTTP(rec, req)
			return rec
		}
		statuses := make(chan int, 10)
		chunk := func(userID string) {
			go func() {
				statuses <- serve(t.Context(), userID, apiv1connect.AssetUploadServiceUploadChunkProcedure).Code
			}()
		}
		drain := func() {
			synctest.Wait()
			for {
				select {
				case userID := <-entered:
					inside[userID]++
				default:
					return
				}
			}
		}

		// One user gets at most maxConcurrentUploadChunksPerUser slots.
		for range maxConcurrentUploadChunksPerUser + 1 {
			chunk("a")
		}
		drain()
		if inside["a"] != maxConcurrentUploadChunksPerUser {
			t.Fatalf("user a has %d chunk requests inside, want %d", inside["a"], maxConcurrentUploadChunksPerUser)
		}

		// Other users get the remaining process slots, then wait.
		for _, userID := range []string{"b", "c", "d"} {
			chunk(userID)
			drain()
		}
		total := 0
		for _, n := range inside {
			total += n
		}
		if total != maxConcurrentUploadChunks || inside["d"] != 0 {
			t.Fatalf("chunk requests inside = %v, want %d in total and none for d", inside, maxConcurrentUploadChunks)
		}

		// Other procedures do not wait for a slot.
		if rec := serve(t.Context(), "a", apiv1connect.AssetUploadServiceCreateUploadProcedure); rec.Code != http.StatusOK {
			t.Fatalf("create status = %d", rec.Code)
		}

		// A chunk request that ends while it waits gets a Connect error.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		rec := serve(ctx, "e", apiv1connect.AssetUploadServiceUploadChunkProcedure)
		if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"unavailable"`) {
			t.Fatalf("waiting chunk response = %d %s, want a Connect unavailable error", rec.Code, rec.Body.String())
		}

		releaseAll()
		synctest.Wait()
		close(statuses)
		for status := range statuses {
			if status != http.StatusOK {
				t.Errorf("chunk status = %d, want %d", status, http.StatusOK)
			}
		}
		limiter.mu.Lock()
		defer limiter.mu.Unlock()
		if len(limiter.users) != 0 || len(limiter.slots) != 0 {
			t.Fatalf("limiter keeps %d user entries and %d process slots after all requests", len(limiter.users), len(limiter.slots))
		}
	})
}

func TestUploadChunkLimiterReadTimeout(t *testing.T) {
	t.Parallel()
	readErr := make(chan error, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		readErr <- err
	})
	limiter := newUploadChunkLimiter(next, HandlerOptions()...)
	limiter.readTimeout = 100 * time.Millisecond
	server := httptest.NewServer(limiter)
	defer server.Close()

	// The client sends the headers and part of the body, then stops.
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, err = io.WriteString(conn, "POST "+apiv1connect.AssetUploadServiceUploadChunkProcedure+" HTTP/1.1\r\n"+
		"Host: example.com\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-readErr:
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("body read error = %v, want a timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("body read did not time out")
	}
}

// TestUploadChunkLimiterDeadlineEndsWithBody makes sure that the read deadline
// does not cancel the request while the handler stores a complete chunk.
// net/http removes the deadline when the body is complete.
func TestUploadChunkLimiterDeadlineEndsWithBody(t *testing.T) {
	t.Parallel()
	ctxErr := make(chan error, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			ctxErr <- err
			return
		}
		// The handler works longer than the read timeout after the body is
		// complete, as a slow storage write can.
		time.Sleep(300 * time.Millisecond)
		ctxErr <- r.Context().Err()
	})
	limiter := newUploadChunkLimiter(next, HandlerOptions()...)
	limiter.readTimeout = 100 * time.Millisecond
	server := httptest.NewServer(limiter)
	defer server.Close()

	resp, err := http.Post(server.URL+apiv1connect.AssetUploadServiceUploadChunkProcedure, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if err := <-ctxErr; err != nil {
		t.Fatalf("request context error after the body was complete = %v, want nil", err)
	}
}
