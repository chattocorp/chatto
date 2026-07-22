package runtimeunit

import (
	"context"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
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
