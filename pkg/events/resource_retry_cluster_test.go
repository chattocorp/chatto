package events_test

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/pkg/events"
)

func TestCreateJetStreamResourceWithRetryCluster(t *testing.T) {
	// JetStream requires a configured route even on the seed server.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	seedPort := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	route, err := url.Parse(fmt.Sprintf("nats://127.0.0.1:%d", seedPort))
	if err != nil {
		t.Fatal(err)
	}
	var servers []*server.Server
	for i := range 3 {
		opts := &server.Options{
			ServerName: fmt.Sprintf("resource-%d", i), Host: "127.0.0.1", Port: -1,
			JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
			Cluster: server.ClusterOpts{Name: "resources", Host: "127.0.0.1", Port: -1},
			Routes:  []*url.URL{route},
		}
		if i == 0 {
			opts.Cluster.Port = seedPort
		}
		s, err := server.NewServer(opts)
		if err != nil {
			t.Fatal(err)
		}
		s.ConfigureLogger()
		s.Start()
		t.Cleanup(func() { s.Shutdown(); s.WaitForShutdown() })
		if !s.ReadyForConnections(5 * time.Second) {
			t.Fatal("cluster server not ready")
		}
		servers = append(servers, s)
	}
	waitFor(t, 10*time.Second, func() bool {
		leader := false
		for _, s := range servers {
			if s.NumRoutes() < 2 || !s.JetStreamIsCurrent() {
				return false
			}
			leader = leader || (s.JetStreamIsLeader() && len(s.JetStreamClusterPeers()) == 3)
		}
		return leader
	})
	clients := make([]jetstream.JetStream, 2)
	for i := range clients {
		nc, err := nats.Connect(servers[i].ClientURL())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(nc.Close)
		clients[i], err = jetstream.New(nc)
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := jetstream.StreamConfig{
		Name: "RESOURCE", Subjects: []string{"resource.>"}, Replicas: 3,
		Storage: jetstream.FileStorage, Compression: jetstream.S2Compression,
		AllowMsgTTL: true, AllowAtomicPublish: true,
		Metadata: map[string]string{"application.identity": "opaque-incarnation", "other": "preserved"},
	}
	type result struct {
		stream   jetstream.Stream
		err      error
		attempts int
		created  time.Time
		metadata map[string]string
	}
	results := make(chan result, 2)
	created := make(chan struct{})
	firstCalls := make(chan struct{}, 2)
	retry := make(chan struct{})
	for i, js := range clients {
		go func() {
			var r result
			r.stream, r.err = events.CreateJetStreamResourceWithRetry(ctx, events.JetStreamResourceRetryPolicy{MaxAttempts: 3, RetryDelay: 25 * time.Millisecond}, func(ctx context.Context) (jetstream.Stream, error) {
				r.attempts++
				// Order the initial create and attach to avoid relying on the
				// upstream race. Both callers then retry concurrently.
				if i == 1 && r.attempts == 1 {
					select {
					case <-created:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				stream, err := js.CreateOrUpdateStream(ctx, cfg)
				if i == 0 && r.attempts == 1 {
					close(created)
				}
				if r.attempts == 1 {
					firstCalls <- struct{}{}
					if err != nil {
						return nil, fmt.Errorf("initial provisioning: %w", err)
					}
					r.created = stream.CachedInfo().Created
					r.metadata = maps.Clone(stream.CachedInfo().Config.Metadata)
					select {
					case <-retry:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					// The server committed the operation, but the caller did not
					// receive its result before the request deadline.
					return nil, context.DeadlineExceeded
				}
				return stream, err
			})
			results <- r
		}()
	}
	for range 2 {
		select {
		case <-firstCalls:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(retry)
	var incarnation time.Time
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		if r.attempts != 2 {
			t.Fatalf("attempts = %d, want 2", r.attempts)
		}
		info, err := r.stream.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if incarnation.IsZero() {
			incarnation = info.Created
		}
		if !info.Created.Equal(incarnation) || !info.Created.Equal(r.created) || !maps.Equal(info.Config.Metadata, r.metadata) {
			t.Fatalf("stream identity or metadata changed: %+v", info)
		}
		for key, value := range cfg.Metadata {
			if info.Config.Metadata[key] != value {
				t.Fatalf("metadata %s = %q, want %q", key, info.Config.Metadata[key], value)
			}
		}
		if info.Config.Replicas != 3 || info.Cluster == nil || len(info.Cluster.Replicas) != 2 {
			t.Fatalf("expected R3 stream: %+v", info)
		}
	}
}
