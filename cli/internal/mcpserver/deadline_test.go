package mcpserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// deadlineTestHandler uses the production Gin wrapper and a short parent deadline
// so stalled I/O tests do not have to wait for the 15-second MCP timeout.
func deadlineTestHandler(next http.Handler) http.Handler {
	router := gin.New()
	bounded := withRequestDeadline(next)
	router.Any("/*path", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthy" {
			ctx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
			defer cancel()
			r = r.WithContext(ctx)
		}
		bounded.ServeHTTP(w, r)
	})))
	return router
}

func TestMCPRequestDeadlineInterruptsHTTP1IO(t *testing.T) {
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			result := make(chan error, 1)
			server := httptest.NewServer(deadlineTestHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if operation == "read" {
					_, err := io.Copy(io.Discard, r.Body)
					result <- err
					return
				}
				// Exceed the socket buffers while the client does not read. Reuse
				// one chunk to keep the test's memory bounded.
				chunk := make([]byte, 64*1024)
				for range 1024 {
					if _, err := w.Write(chunk); err != nil {
						result <- err
						return
					}
				}
				result <- nil
			})))
			defer server.Close()
			conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.(*net.TCPConn).SetReadBuffer(1024); err != nil {
				t.Fatal(err)
			}
			bodyLength, body := 0, ""
			if operation == "read" {
				bodyLength, body = 10, "x"
			}
			if _, err := fmt.Fprintf(conn, "POST /mcp HTTP/1.1\r\nHost: test\r\nContent-Length: %d\r\n\r\n%s", bodyLength, body); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				var timeout net.Error
				if !errors.As(err, &timeout) || !timeout.Timeout() {
					t.Fatalf("I/O error = %v, want timeout", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("stalled I/O did not stop at the shorter parent deadline")
			}
		})
	}
}

func TestMCPRequestDeadlineClearsHTTP1KeepAlive(t *testing.T) {
	server := httptest.NewServer(deadlineTestHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})))
	defer server.Close()
	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	for request := range 2 {
		if request == 1 {
			// Reuse the actual connection after the first request's deadline.
			// An HTTP client pool could hide a stale deadline by reconnecting.
			time.Sleep(200 * time.Millisecond)
		}
		if _, err := io.WriteString(conn, "GET /mcp HTTP/1.1\r\nHost: test\r\n\r\n"); err != nil {
			t.Fatal(err)
		}
		response, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("request %d: %v", request, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || string(body) != "ok" {
			t.Fatalf("request %d: status %d, body %q, error %v", request, response.StatusCode, body, err)
		}
	}
}

func TestMCPRequestDeadlineHTTP2StreamIsolation(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	server := httptest.NewUnstartedServer(deadlineTestHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthy" {
			select {
			case <-finished:
				_, _ = io.WriteString(w, "ok")
			case <-r.Context().Done():
			}
			return
		}
		defer close(finished)
		var first [1]byte
		if _, err := io.ReadFull(r.Body, first[:]); err != nil {
			result <- err
			return
		}
		close(started)
		_, err := io.Copy(io.Discard, r.Body)
		result <- err
	})))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	bodyReader, bodyWriter := io.Pipe()
	defer bodyReader.Close()
	defer bodyWriter.Close()
	slowConn := make(chan net.Conn, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	slowCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		slowConn <- info.Conn
	}})
	request, err := http.NewRequestWithContext(slowCtx, http.MethodPost, server.URL+"/mcp", bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	request.ContentLength = 10
	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		response, err := client.Do(request)
		if err == nil {
			response.Body.Close()
		}
	}()
	defer func() {
		cancel()
		bodyWriter.Close()
		<-slowDone
	}()
	if _, err := io.WriteString(bodyWriter, "x"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("HTTP/2 handler did not read the first body byte")
	}
	var healthyConn net.Conn
	healthyCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		healthyConn = info.Conn
	}})
	healthy, err := http.NewRequestWithContext(healthyCtx, http.MethodGet, server.URL+"/healthy", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(healthy)
	if err != nil {
		t.Fatalf("healthy stream: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.ProtoMajor != 2 || response.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("healthy stream: protocol %s, status %d, body %q, error %v", response.Proto, response.StatusCode, body, err)
	}
	if healthyConn != <-slowConn {
		t.Fatal("healthy stream used a different connection")
	}
	// The read deadline and write deadline can race. net/http can return
	// either a read timeout or a stream reset when the write timer fires first.
	if err := <-result; err == nil {
		t.Fatal("stalled HTTP/2 read completed without an error")
	}
}

func TestMCPRequestDeadlineWithoutIOControl(t *testing.T) {
	var requestContext context.Context
	before := time.Now()
	withRequestDeadline(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requestContext = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}")))
	deadline, ok := requestContext.Deadline()
	if !ok || deadline.Before(before.Add(requestTimeout)) || deadline.After(time.Now().Add(requestTimeout)) {
		t.Fatalf("context deadline = %v, want the MCP request timeout", deadline)
	}
	if !errors.Is(requestContext.Err(), context.Canceled) {
		t.Fatalf("context after handler exit = %v, want cancellation", requestContext.Err())
	}
}
