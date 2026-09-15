package oidcprovider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func cimdTestResponse(request *http.Request, cacheControl string) *http.Response {
	document := fmt.Sprintf(`{"client_id":%q,"client_name":"Client","redirect_uris":["https://client.example/callback"],"token_endpoint_auth_method":"none"}`, request.URL.String())
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}, "Cache-Control": {cacheControl}}, Body: io.NopCloser(strings.NewReader(document)), Request: request}
}

func newCIMDTestResolver(t *testing.T, transport roundTripFunc) *CIMDResolver {
	t.Helper()
	resolver, err := NewCIMDResolver("https://auth.example", &http.Client{Transport: transport}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolver.validateDestination = func(context.Context, string) error { return nil }
	return resolver
}

func TestCIMDCacheCapacityExpiryAndIsolation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newCIMDTestResolver(t, func(request *http.Request) (*http.Response, error) {
			age := "max-age=60"
			if request.URL.Path == "/0" {
				age = "max-age=1"
			}
			return cimdTestResponse(request, age), nil
		})
		for i := range maxCIMDCacheEntries + 1 {
			if _, err := resolver.Resolve(t.Context(), fmt.Sprintf("https://client.example/%d", i)); err != nil {
				t.Fatal(err)
			}
			if len(resolver.cache) > maxCIMDCacheEntries {
				t.Fatal("cache exceeded capacity")
			}
		}
		if len(resolver.cache) != maxCIMDCacheEntries {
			t.Fatal("cache did not reach capacity")
		}
		if _, exists := resolver.cache["https://client.example/0"]; exists {
			t.Fatal("soonest-expiring entry was not evicted")
		}
		client, err := resolver.Resolve(t.Context(), "https://client.example/1")
		if err != nil {
			t.Fatal(err)
		}
		client.NameValue, client.Redirects[0] = "changed", "https://changed.example/"
		again, err := resolver.Resolve(t.Context(), "https://client.example/1")
		if err != nil || again.NameValue != "Client" || again.Redirects[0] != "https://client.example/callback" {
			t.Fatal("caller mutated cached client")
		}
		time.Sleep(time.Minute)
		if _, err := resolver.Resolve(t.Context(), "https://client.example/unrelated"); err != nil {
			t.Fatal(err)
		}
		if len(resolver.cache) != 1 {
			t.Fatalf("expired entries retained: %d", len(resolver.cache))
		}
	})
}

func TestCIMDAdmissionBoundsDNSAndRecovers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var fetches, validations atomic.Int32
		resolver := newCIMDTestResolver(t, func(request *http.Request) (*http.Response, error) {
			fetches.Add(1)
			return cimdTestResponse(request, "max-age=60"), nil
		})
		if _, err := resolver.Resolve(t.Context(), "https://client.example/cached"); err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{}, maxCIMDLookups)
		release := make(chan struct{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		resolver.validateDestination = func(ctx context.Context, _ string) error {
			validations.Add(1)
			entered <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		results := make(chan error, maxCIMDLookups)
		for i := range maxCIMDLookups {
			go func() {
				_, err := resolver.Resolve(ctx, fmt.Sprintf("https://client.example/active-%d", i))
				results <- err
			}()
		}
		for range maxCIMDLookups {
			<-entered
		}
		for range 32 {
			// The caller's deadline also prevents a broken admission gate from hanging the test.
			attempt, done := context.WithTimeout(ctx, time.Second)
			_, err := resolver.Resolve(attempt, "https://client.example/overload")
			done()
			if !errors.Is(err, errCIMDBusy) {
				t.Fatalf("saturated lookup = %v, want busy", err)
			}
		}
		if validations.Load() != maxCIMDLookups || fetches.Load() != 1 {
			t.Fatal("overload started DNS or HTTP work")
		}
		if _, err := resolver.Resolve(ctx, "https://client.example/cached"); err != nil {
			t.Fatalf("cache hit blocked by saturation: %v", err)
		}
		close(release)
		for range maxCIMDLookups {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		if len(resolver.slots) != 0 {
			t.Fatal("lookup slots leaked")
		}
		if _, err := resolver.Resolve(ctx, "https://client.example/recovered"); err != nil {
			t.Fatalf("resolver did not recover: %v", err)
		}
	})
}

type cimdWaitingBody struct {
	ctx    context.Context
	closed bool
}

func (b *cimdWaitingBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b *cimdWaitingBody) Close() error             { b.closed = true; return nil }

func TestCIMDDeadlineCoversDNSFetchAndBody(t *testing.T) {
	for _, stage := range []string{"DNS", "fetch after DNS", "body after DNS"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var body *cimdWaitingBody
				resolver := newCIMDTestResolver(t, func(request *http.Request) (*http.Response, error) {
					if stage == "DNS" {
						t.Fatal("fetch started after DNS deadline")
					}
					if stage == "fetch after DNS" {
						<-request.Context().Done()
						return nil, request.Context().Err()
					}
					response := cimdTestResponse(request, "max-age=60")
					body = &cimdWaitingBody{ctx: request.Context()}
					response.Body = body
					return response, nil
				})
				resolver.validateDestination = func(ctx context.Context, _ string) error {
					if stage == "DNS" {
						<-ctx.Done()
						return ctx.Err()
					}
					time.Sleep(4 * time.Second)
					return nil
				}
				started := time.Now()
				if _, err := resolver.Resolve(t.Context(), "https://client.example/slow"); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("deadline error = %v", err)
				}
				if elapsed := time.Since(started); elapsed != cimdLookupTimeout {
					t.Fatalf("lookup took %v, want %v total", elapsed, cimdLookupTimeout)
				}
				if len(resolver.slots) != 0 || len(resolver.cache) != 0 {
					t.Fatal("timeout retained admission or cached a result")
				}
				if body != nil && !body.closed {
					t.Fatal("timed-out body was not closed")
				}
			})
		})
	}
}

func TestCIMDConcurrentCacheFillsStayBounded(t *testing.T) {
	resolver := newCIMDTestResolver(t, func(request *http.Request) (*http.Response, error) {
		return cimdTestResponse(request, "max-age=60"), nil
	})
	var workers sync.WaitGroup
	for worker := range maxCIMDLookups {
		workers.Go(func() {
			for i := range maxCIMDCacheEntries {
				if _, err := resolver.Resolve(t.Context(), fmt.Sprintf("https://client.example/%d/%d", worker, i)); err != nil {
					t.Error(err)
					return
				}
				resolver.mu.Lock()
				size := len(resolver.cache)
				resolver.mu.Unlock()
				if size > maxCIMDCacheEntries {
					t.Error("concurrent cache fill exceeded capacity")
					return
				}
			}
		})
	}
	workers.Wait()
}

func TestCIMDDialRetainsLookupCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if deadline {
				time.Sleep(time.Second)
				synctest.Wait()
			} else {
				cancel()
			}
			// This is how net/http passes request values to a detached dial.
			detached := context.WithoutCancel(context.WithValue(ctx, cimdLookupContextKey{}, ctx))
			transport := cimdTransport(false, nil, nil)
			defer transport.CloseIdleConnections()
			// An invalid address would fail later; cancellation must win before parsing/DNS.
			if _, err := transport.DialContext(detached, "tcp", "invalid-address"); !errors.Is(err, ctx.Err()) {
				t.Fatalf("dial lost lookup cancellation: %v", err)
			}
		})
	}
}

func TestCIMDCallerCancellationAndFailureReleaseAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resolver := newCIMDTestResolver(t, func(request *http.Request) (*http.Response, error) { return cimdTestResponse(request, "no-store"), nil })
		resolver.validateDestination = func(ctx context.Context, _ string) error { <-ctx.Done(); return ctx.Err() }
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		started := time.Now()
		if _, err := resolver.Resolve(ctx, "https://client.example/slow"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("caller deadline = %v", err)
		}
		if time.Since(started) != time.Second || len(resolver.slots) != 0 {
			t.Fatal("caller deadline did not release lookup promptly")
		}
		resolver.validateDestination = func(context.Context, string) error { return fmt.Errorf("destination rejected") }
		for range maxCIMDLookups + 1 {
			if _, err := resolver.Resolve(t.Context(), "https://client.example/rejected"); err == nil || errors.Is(err, errCIMDBusy) {
				t.Fatalf("validation failure leaked admission: %v", err)
			}
		}
		resolver.validateDestination = func(context.Context, string) error { return nil }
		for range maxCIMDLookups + 1 {
			if _, err := resolver.Resolve(t.Context(), "https://client.example/recovered"); err != nil {
				t.Fatal(err)
			}
		}
		if len(resolver.cache) != 0 || len(resolver.slots) != 0 {
			t.Fatal("no-store response retained cache or admission")
		}
	})
}

func TestCIMDHTTPSBodyCancellationReleasesSlot(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/slow" {
			_, _ = io.WriteString(w, "{")
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(stopped)
			return
		}
		_, _ = fmt.Fprintf(w, `{"client_id":%q,"redirect_uris":["https://client.example/callback"],"token_endpoint_auth_method":"none"}`, "https://"+r.Host+r.URL.Path)
	}))
	defer server.Close()
	transport := cimdTransport(true, nil, nil)
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	defer transport.CloseIdleConnections()
	resolver, err := NewCIMDResolver("http://localhost", &http.Client{Transport: transport}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := resolver.Resolve(ctx, server.URL+"/slow"); result <- err }()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("slow response did not start: %v", err)
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("body cancellation = %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not observe cancellation")
	}
	if len(resolver.slots) != 0 || len(resolver.cache) != 0 {
		t.Fatal("canceled HTTP body retained slot or cache entry")
	}
	if _, err := resolver.Resolve(t.Context(), server.URL+"/recovered"); err != nil {
		t.Fatalf("HTTPS lookup did not recover: %v", err)
	}
}
