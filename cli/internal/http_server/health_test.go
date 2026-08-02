package http_server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func TestHealthEndpointsReflectNATSConnectionLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ns, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, NoSigs: true})
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("NATS server did not become ready")
	}
	nc, err := nats.Connect(ns.ClientURL(), nats.MaxReconnects(-1), nats.ReconnectWait(10*time.Millisecond))
	if err != nil {
		ns.Shutdown()
		t.Fatalf("connect to NATS: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	s := &HTTPServer{nc: nc, router: gin.New(), logger: log.WithPrefix("test.HTTP")}
	s.setupHealthRoutes()

	assertHealthResponse(t, s.router, "/healthz", http.StatusOK, "ok")
	assertHealthResponse(t, s.router, "/readyz", http.StatusOK, "ready")

	ns.Shutdown()
	ns.WaitForShutdown()
	waitForHealthTest(t, 5*time.Second, nc.IsReconnecting, "NATS client to reconnect")
	assertHealthResponse(t, s.router, "/healthz", http.StatusOK, "ok")
	assertHealthResponse(t, s.router, "/readyz", http.StatusServiceUnavailable, "not ready")

	nc.Close()
	assertHealthResponse(t, s.router, "/healthz", http.StatusServiceUnavailable, "not live")
	assertHealthResponse(t, s.router, "/readyz", http.StatusServiceUnavailable, "not ready")
}

func assertHealthResponse(t *testing.T, router http.Handler, path string, status int, state string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != status {
		t.Fatalf("GET %s status = %d, want %d", path, response.Code, status)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode GET %s response: %v", path, err)
	}
	if body["status"] != state {
		t.Fatalf("GET %s status body = %q, want %q", path, body["status"], state)
	}
}

func waitForHealthTest(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
