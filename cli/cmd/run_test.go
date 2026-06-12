package cmd

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/testutil"
)

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
		closeNATSConnection(nc)
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
