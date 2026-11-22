package events

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// newClusterResetHarness starts a three-node cluster, an R3 stream, and a
// harness whose client can reconnect to any node.
func newClusterResetHarness(t *testing.T) (*resetHarness, *testCluster) {
	t.Helper()
	cluster := startTestCluster(t)
	conn, err := nats.Connect(cluster.urls(), nats.MaxReconnects(-1), nats.ReconnectWait(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	js := must(jetstream.New(conn))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	// The meta leader can exist before it knows all peers, so retry until
	// the R3 stream can be placed.
	var stream jetstream.Stream
	for {
		stream, err = js.CreateStream(ctx, jetstream.StreamConfig{
			Name:     "EVENTS_RESET",
			Subjects: []string{"evt.reset.>"},
			Replicas: 3,
			Storage:  jetstream.FileStorage,
		})
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("create R3 stream: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	return newResetHarnessOn(t, ctx, conn, js, stream), cluster
}

// projectorConsumer returns the name of the projector's ordered consumer and
// the node that hosts it. A reset creates a consumer with a new name.
func (h *resetHarness) projectorConsumer() (name, host string) {
	h.t.Helper()
	for info := range h.stream.ListConsumers(h.ctx).Info() {
		if info.Cluster != nil {
			return info.Name, info.Cluster.Leader
		}
	}
	h.t.Fatal("projector consumer not found")
	return "", ""
}

// TestProjectorClusterServerChange runs the main reset regression on a
// three-node cluster with an R3 stream. While consumed event s is in its
// decoder, the node that hosts the ordered consumer stops, or the stream
// leader steps down. Stopping the consumer's node makes the ordered consumer
// reset on another node; NATS places the consumer, so the stopped node can
// also be the client's node, and then the client reconnects too. A stream
// leader step-down keeps the R1 consumer, so that case checks only that a
// leader change keeps the order. The test logs which case ran.
func TestProjectorClusterServerChange(t *testing.T) {
	for _, change := range []string{"consumer host shutdown", "stream leader stepdown"} {
		t.Run(change, func(t *testing.T) {
			h, cluster := newClusterResetHarness(t)
			h.start(h.projection, true)
			first := h.append(resetConsumedSubject, "first")
			h.waitFor(first)
			<-h.projection.applied
			consumer, host := h.projectorConsumer()
			client := h.conn.ConnectedServerName()
			held := h.holdDecode("s")
			s := h.append(resetConsumedSubject, "s")
			receive(t, held.entered, "hold s")
			switch change {
			case "consumer host shutdown":
				cluster.server(host).Shutdown()
			case "stream leader stepdown":
				if _, err := h.conn.Request("$JS.API.STREAM.LEADER.STEPDOWN.EVENTS_RESET", nil, 5*time.Second); err != nil {
					t.Fatal(err)
				}
			}
			changed := time.Now()
			next := h.appendRetrying(resetConsumedSubject, "next")
			t.Logf("appended next %v after the change", time.Since(changed).Round(time.Millisecond))
			waitS := waitAsync(func() error { return h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, s)) })
			h.expectHeld("s is held", waitS)
			held.open()
			if err := <-waitS; err != nil {
				t.Fatal(err)
			}
			h.waitFor(next)
			t.Logf("applied next %v after the change", time.Since(changed).Round(time.Millisecond))
			later := h.append(resetConsumedSubject, "later")
			h.waitFor(later)
			h.expectApplied(first, s, next, later)
			nowConsumer, nowHost := h.projectorConsumer()
			if change == "consumer host shutdown" && nowConsumer == consumer {
				t.Fatalf("consumer %s was not replaced after its node stopped", consumer)
			}
			t.Logf("client on %s, client reconnects %d, consumer %s on %s, now %s on %s", client, h.conn.Stats().Reconnects, consumer, host, nowConsumer, nowHost)
		})
	}
}

// appendRetrying appends event and tries again while the cluster recovers
// from the server change. The message ID makes JetStream store the event
// once, also when an earlier attempt was stored without an answer.
func (h *resetHarness) appendRetrying(subject, event string) uint64 {
	h.t.Helper()
	for {
		attempt, cancel := context.WithTimeout(h.ctx, 2*time.Second)
		seq, err := h.log.AppendEventually(attempt, subject, EncodedRecord{ID: event, Data: []byte(event)})
		cancel()
		if err == nil {
			return seq
		}
		if h.ctx.Err() != nil {
			h.t.Fatalf("append %s: %v", event, err)
		}
	}
}
