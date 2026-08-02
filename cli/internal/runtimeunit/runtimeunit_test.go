package runtimeunit

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/pkg/natsauth"
)

type testUnit struct{ name string }

func (u testUnit) Name() string                 { return u.name }
func (testUnit) Run(context.Context, Env) error { return nil }

func TestRequireStandaloneNATSClientURL(t *testing.T) {
	t.Run("allows configured client URL", func(t *testing.T) {
		cfg := config.ChattoConfig{}
		cfg.NATS.Client.URL = "nats://127.0.0.1:4222"

		if err := RequireStandaloneNATSClientURL(cfg, "exporter"); err != nil {
			t.Fatalf("expected configured NATS client URL to be accepted: %v", err)
		}
	})

	t.Run("explains standalone requirement", func(t *testing.T) {
		err := RequireStandaloneNATSClientURL(config.ChattoConfig{}, "exporter")
		if err == nil {
			t.Fatal("expected missing NATS client URL to fail")
		}

		msg := err.Error()
		for _, want := range []string{"exporter", "[nats.client]", "nats.embedded.port"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("expected error %q to mention %q", msg, want)
			}
		}
	})
}

func TestConnectToNATSRecoversAfterTemporaryAuthenticationFailure(t *testing.T) {
	const token = "correct-token"
	ns := startRuntimeUnitTestNATS(t, -1, token)
	port := ns.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.ChattoConfig{}
	cfg.NATS.Client.URL = fmt.Sprintf("nats://127.0.0.1:%d", port)
	cfg.NATS.Client.AuthMethod = natsauth.AuthToken
	cfg.NATS.Client.Token = token
	nc, err := ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		stopRuntimeUnitTestNATS(ns)
		t.Fatalf("ConnectToNATS: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		stopRuntimeUnitTestNATS(ns)
	})

	stopRuntimeUnitTestNATS(ns)
	ns = startRuntimeUnitTestNATS(t, port, "temporarily-wrong-token")
	waitForRuntimeUnitTest(t, 5*time.Second, nc.IsReconnecting, "client to enter reconnecting state")

	// nats.go normally gives up after receiving the same authentication error
	// twice. Keep the bad server up long enough to cross that threshold.
	time.Sleep(500 * time.Millisecond)
	if nc.IsClosed() {
		t.Fatal("NATS client permanently closed after temporary authentication failures")
	}

	stopRuntimeUnitTestNATS(ns)
	ns = startRuntimeUnitTestNATS(t, port, token)
	waitForRuntimeUnitTest(t, 5*time.Second, nc.IsConnected, "client to reconnect after authentication repair")
}

func TestConnectToNATSUnavailableServerStopsWithContext(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	cfg := config.ChattoConfig{}
	cfg.NATS.Client.URL = "nats://" + address
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := ConnectToNATS(ctx, cfg, nil); err == nil {
		t.Fatal("ConnectToNATS succeeded while server remained unavailable")
	}
}

func startRuntimeUnitTestNATS(t *testing.T, port int, token string) *server.Server {
	t.Helper()
	ns, err := server.NewServer(&server.Options{
		Host:          "127.0.0.1",
		Port:          port,
		Authorization: token,
		NoSigs:        true,
	})
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("NATS server did not become ready")
	}
	return ns
}

func stopRuntimeUnitTestNATS(ns *server.Server) {
	if ns == nil {
		return
	}
	ns.Shutdown()
	ns.WaitForShutdown()
}

func waitForRuntimeUnitTest(t *testing.T, timeout time.Duration, condition func() bool, description string) {
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

func TestRegistrationEnabled(t *testing.T) {
	cfg := config.ChattoConfig{}
	registration := Registration{
		Unit: testUnit{name: "test"},
		StartWithRun: func(config.ChattoConfig) bool {
			return true
		},
	}
	if !registration.Enabled(cfg) {
		t.Fatal("enabled registration reported disabled")
	}
	if (Registration{}).Enabled(cfg) {
		t.Fatal("incomplete registration reported enabled")
	}
}

func TestValidateRegistrations(t *testing.T) {
	enabled := func(config.ChattoConfig) bool { return true }
	tests := []struct {
		name          string
		registrations []Registration
		wantError     string
	}{
		{name: "valid", registrations: []Registration{{Unit: testUnit{name: "one"}, StartWithRun: enabled}}},
		{name: "nil unit", registrations: []Registration{{StartWithRun: enabled}}, wantError: "has no unit"},
		{name: "empty name", registrations: []Registration{{Unit: testUnit{}, StartWithRun: enabled}}, wantError: "empty name"},
		{name: "nil predicate", registrations: []Registration{{Unit: testUnit{name: "one"}}}, wantError: "no chatto run predicate"},
		{name: "duplicate", registrations: []Registration{{Unit: testUnit{name: "one"}, StartWithRun: enabled}, {Unit: testUnit{name: "one"}, StartWithRun: enabled}}, wantError: "registered more than once"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRegistrations(test.registrations)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("ValidateRegistrations: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("ValidateRegistrations error = %v, want %q", err, test.wantError)
			}
		})
	}
}
