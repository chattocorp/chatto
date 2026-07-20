package cmd

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/internal/testutil"
)

type failingRuntimeUnit struct{}

func (failingRuntimeUnit) Name() string { return "failing" }
func (failingRuntimeUnit) Run(context.Context, runtimeunit.Env) error {
	return errors.New("unit failed")
}

func TestEffectiveLogFormat(t *testing.T) {
	tests := []struct {
		name             string
		configuredFormat string
		outputIsTerminal bool
		want             string
	}{
		{name: "auto uses text on terminal", configuredFormat: "auto", outputIsTerminal: true, want: "text"},
		{name: "auto uses json off terminal", configuredFormat: "auto", outputIsTerminal: false, want: "json"},
		{name: "empty defaults to auto text on terminal", configuredFormat: "", outputIsTerminal: true, want: "text"},
		{name: "empty defaults to auto json off terminal", configuredFormat: "", outputIsTerminal: false, want: "json"},
		{name: "explicit text", configuredFormat: "text", outputIsTerminal: false, want: "text"},
		{name: "explicit json", configuredFormat: "json", outputIsTerminal: true, want: "json"},
		{name: "explicit logfmt", configuredFormat: "logfmt", outputIsTerminal: true, want: "logfmt"},
		{name: "case insensitive", configuredFormat: "JSON", outputIsTerminal: false, want: "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveLogFormat(tt.configuredFormat, tt.outputIsTerminal); got != tt.want {
				t.Fatalf("effectiveLogFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldPrintBannerOnlyForTextLogs(t *testing.T) {
	if !shouldPrintBanner("text", false) {
		t.Fatal("expected text logs to print banner")
	}
	if shouldPrintBanner("json", true) {
		t.Fatal("expected json logs to suppress banner")
	}
	if shouldPrintBanner("auto", false) {
		t.Fatal("expected auto logs off terminal to suppress banner")
	}
}

func TestOptionalRuntimeUnitFailureDoesNotStopServer(t *testing.T) {
	err := runOptionalRuntimeUnit(context.Background(), runtimeunit.Env{
		Logger: log.New(io.Discard),
	}, failingRuntimeUnit{})
	if err != nil {
		t.Fatalf("runOptionalRuntimeUnit() = %v, want nil", err)
	}
}

func TestCloseNATSConnectionWaitsForDrainToComplete(t *testing.T) {
	ns, _ := testutil.StartNATS(t)

	nc, err := nats.Connect(
		nats.DefaultURL,
		nats.InProcessServer(ns),
		nats.DrainTimeout(200*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("connect to nats: %v", err)
	}
	t.Cleanup(nc.Close)

	callbackStarted := make(chan struct{})
	unblockCallback := make(chan struct{})

	_, err = nc.Subscribe("drain.wait", func(*nats.Msg) {
		close(callbackStarted)
		<-unblockCallback
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush subscription: %v", err)
	}
	if err := nc.Publish("drain.wait", []byte("pending")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription callback to start")
	}

	drainReturned := make(chan struct{})
	go func() {
		runtimeunit.CloseNATSConnection(nc)
		close(drainReturned)
	}()

	select {
	case <-drainReturned:
		t.Fatal("closeNATSConnection returned before NATS drain completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(unblockCallback)

	select {
	case <-drainReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for closeNATSConnection to return")
	}
	if !nc.IsClosed() {
		t.Fatal("expected NATS connection to be closed after drain")
	}
}
