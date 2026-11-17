package bleve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	"github.com/chattocorp/chatto/cli/internal/search"
	"github.com/chattocorp/chatto/cli/internal/testutil"
)

// commitGate blocks Bleve batch commits while it is closed.
type commitGate struct {
	mu      sync.Mutex
	blocked bool
	release chan struct{}
	entered chan struct{}
}

func newCommitGate() *commitGate {
	return &commitGate{entered: make(chan struct{}, 16)}
}

func (g *commitGate) block() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blocked = true
	g.release = make(chan struct{})
}

func (g *commitGate) unblock() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.blocked {
		g.blocked = false
		close(g.release)
	}
}

func (g *commitGate) wait() {
	g.mu.Lock()
	blocked, release := g.blocked, g.release
	g.mu.Unlock()
	if blocked {
		g.entered <- struct{}{}
		<-release
	}
}

type healthTestFixture struct {
	nc         *nats.Conn
	js         jetstream.JetStream
	gate       *commitGate
	projection atomic.Pointer[Projection]
	logs       *synchronizedBuffer
	done       chan error
	stop       context.CancelFunc
}

func startHealthTestUnit(t *testing.T, health *Health) *healthTestFixture {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVT", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
		Metadata: map[string]string{evtstream.IdentityMetadataKey: "evt-incarnation-v1:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"},
	})
	require.NoError(t, err)
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "ENCRYPTION_KEYS", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RUNTIME_STATE", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)

	f := &healthTestFixture{nc: nc, js: js, gate: newCommitGate(), logs: &synchronizedBuffer{}, done: make(chan error, 1)}
	logger := log.New(f.logs)
	logger.SetFormatter(log.JSONFormatter)
	unit := Unit{
		Health: health,
		newProjection: func(p *Projection) {
			f.projection.Store(p)
			p.commitBatch = func(batch *blevesearch.Batch) error {
				f.gate.wait()
				return p.index.Batch(batch)
			}
		},
		probeInterval:   20 * time.Millisecond,
		probeTimeout:    50 * time.Millisecond,
		shutdownTimeout: 300 * time.Millisecond,
	}
	unitContext, stop := context.WithCancel(context.Background())
	f.stop = stop
	go func() {
		f.done <- unit.Run(unitContext, runtimeunit.Env{
			Config: config.ChattoConfig{SearchProvider: config.SearchProviderConfig{Directory: t.TempDir() + "/index", Languages: []string{}}},
			NC:     nc, JS: js, Logger: logger, Version: "test",
		})
	}()
	t.Cleanup(func() {
		f.gate.unblock()
		stop()
		select {
		case <-f.done:
		case <-time.After(5 * time.Second):
			t.Error("search unit did not stop")
		}
		// After a shutdown timeout, the unit leaves the projection open.
		if p := f.projection.Load(); p != nil {
			_ = p.Close()
		}
	})
	return f
}

// publishRetraction appends one search-relevant fact, which makes the
// projector commit a live batch.
func (f *healthTestFixture) publishRetraction(t *testing.T, id string) {
	t.Helper()
	payload, err := proto.Marshal(&evtv1.Event{
		Id: id, ActorId: "U1", CreatedAt: timestamppb.Now(),
		Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: "R1", EventId: "M-" + id}},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = f.js.Publish(ctx, evtstream.RoomAggregate("R1").Subject(evtstream.EventMessageRetracted), payload)
	require.NoError(t, err)
}

func (f *healthTestFixture) queryAvailable(t *testing.T) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := search.NewClient(f.nc).Query(ctx, &searchv1.QueryRequest{
		RequiredTerms: []string{"anything"}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 1,
	})
	return err == nil
}

func (f *healthTestFixture) waitForCommitBlocked(t *testing.T) {
	t.Helper()
	select {
	case <-f.gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("index commit did not start")
	}
}

// A blocked index commit blocks queries on the projection lock. The provider
// then leaves the shared query queue, keeps at most one probe waiting,
// resumes serving after the commit returns, and stops within its shutdown
// budget when the commit blocks again.
func TestUnitAlignsServingWithQueryProbeAndBoundsShutdown(t *testing.T) {
	for _, test := range []struct {
		name   string
		health *Health
	}{
		{"embedded", nil},
		{"standalone", NewHealth()},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := startHealthTestUnit(t, test.health)
			require.Eventually(t, func() bool { return f.queryAvailable(t) }, 5*time.Second, 10*time.Millisecond)
			if test.health != nil {
				require.True(t, test.health.Ready())
				require.True(t, test.health.Live())
			}
			require.Contains(t, f.logs.String(), `"stage":"service_ready"`)

			f.gate.block()
			f.publishRetraction(t, "E1")
			f.waitForCommitBlocked(t)
			// Wait for the log, not a query: a query sent before the provider
			// leaves the queue would also wait for the lock.
			require.Eventually(t, func() bool {
				return strings.Contains(f.logs.String(), `"stage":"query_probe_failed"`)
			}, 5*time.Second, 10*time.Millisecond)
			require.False(t, f.queryAvailable(t))
			if test.health != nil {
				require.False(t, test.health.Ready())
				require.True(t, test.health.Live(), "a blocked commit must not fail liveness")
			}
			// The status falls back to the startup endpoint and reports the probe.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			status, err := search.NewClient(f.nc).GetStatus(ctx)
			cancel()
			require.NoError(t, err)
			require.Equal(t, searchv1.ProviderState_PROVIDER_STATE_DEGRADED, status.GetState())
			require.NotNil(t, status.GetQueryProbe().GetInFlightSince())

			// Many probe intervals later, only one probe waits for the lock.
			projection := f.projection.Load()
			deadline := time.Now().Add(20 * 20 * time.Millisecond)
			for time.Now().Before(deadline) {
				require.LessOrEqual(t, projection.diagnostics.Snapshot(time.Now()).QueriesWaiting, int64(1))
				time.Sleep(10 * time.Millisecond)
			}
			diagnostics := projection.diagnostics.Snapshot(time.Now())
			require.Equal(t, indexWriteStageCommit, diagnostics.WriteStage)
			require.EqualValues(t, 1, diagnostics.QueriesWaiting)

			f.gate.unblock()
			require.Eventually(t, func() bool { return f.queryAvailable(t) }, 5*time.Second, 10*time.Millisecond)
			require.Contains(t, f.logs.String(), `"stage":"query_probe_recovered"`)
			require.Equal(t, 1, strings.Count(f.logs.String(), `"stage":"service_ready"`))

			f.gate.block()
			f.publishRetraction(t, "E2")
			f.waitForCommitBlocked(t)
			started := time.Now()
			f.stop()
			select {
			case err := <-f.done:
				require.ErrorIs(t, err, errSearchShutdownTimeout)
				f.done <- err // Let cleanup observe the stopped unit.
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not finish while a commit blocked")
			}
			require.Less(t, time.Since(started), 3*time.Second)
			require.Contains(t, f.logs.String(), `"stage":"shutdown_timeout"`)
		})
	}
}

// Diagnostics read no projection lock, so they answer during a blocked commit.
func TestHealthHandlerDuringBlockedCommit(t *testing.T) {
	health := NewHealth()
	f := startHealthTestUnit(t, health)
	require.Eventually(t, health.Ready, 5*time.Second, 10*time.Millisecond)
	f.gate.block()
	f.publishRetraction(t, "E1")
	f.waitForCommitBlocked(t)
	require.Eventually(t, func() bool { return !health.Ready() }, 5*time.Second, 10*time.Millisecond)

	handler := health.Handler(true)
	get := func(path, remote string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = remote
		recorder := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			handler.ServeHTTP(recorder, request)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("GET %s blocked", path)
		}
		return recorder
	}
	require.Equal(t, http.StatusOK, get("/livez", "10.0.0.5:1234").Code)
	require.Equal(t, http.StatusServiceUnavailable, get("/readyz", "10.0.0.5:1234").Code)

	metrics := get("/metrics", "10.0.0.5:1234")
	require.Equal(t, http.StatusOK, metrics.Code)
	body := metrics.Body.String()
	require.Contains(t, body, "chatto_search_provider_serving 0")
	require.Contains(t, body, `chatto_search_query_probe_runs_total{result="succeeded"}`)
	require.Contains(t, body, "chatto_search_index_write_age_seconds")

	require.Equal(t, http.StatusNotFound, get("/debug/search", "10.0.0.5:1234").Code)
	require.Equal(t, http.StatusNotFound, get("/debug/pprof/goroutine?debug=2", "10.0.0.5:1234").Code)
	goroutines := get("/debug/pprof/goroutine?debug=2", "127.0.0.1:1234")
	require.Equal(t, http.StatusOK, goroutines.Code)
	require.Contains(t, goroutines.Body.String(), "commitGate")

	response := get("/debug/search", "[::1]:1234")
	require.Equal(t, http.StatusOK, response.Code)
	var diagnostics healthDiagnostics
	require.NoError(t, json.NewDecoder(response.Body).Decode(&diagnostics))
	require.True(t, diagnostics.Live)
	require.False(t, diagnostics.Ready)
	require.False(t, diagnostics.Serving)
	require.NotNil(t, diagnostics.Index)
	require.Equal(t, indexWriteStageCommit, diagnostics.Index.WriteStage)
	require.NotNil(t, diagnostics.QueryProbe)
	require.NotNil(t, diagnostics.QueryProbe.InFlightSince)
	require.False(t, diagnostics.QueryProbe.Healthy)
	require.True(t, diagnostics.Projector.StartupComplete)
}

func TestHealthHandlerWithoutDiagnostics(t *testing.T) {
	handler := NewHealth().Handler(false)
	for path, code := range map[string]int{
		"/livez":                 http.StatusOK,
		"/readyz":                http.StatusServiceUnavailable,
		"/metrics":               http.StatusOK,
		"/debug/search":          http.StatusNotFound,
		"/debug/pprof/":          http.StatusNotFound,
		"/debug/pprof/goroutine": http.StatusNotFound,
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = "127.0.0.1:1234"
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		require.Equal(t, code, recorder.Code, path)
		_, _ = io.Copy(io.Discard, recorder.Body)
	}
}
