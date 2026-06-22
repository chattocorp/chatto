package exporter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const readTimeout = 5 * time.Second

type Options struct {
	Config  config.ExporterConfig
	Core    config.CoreConfig
	NC      *nats.Conn
	JS      jetstream.JetStream
	Version string
	Logger  *log.Logger
}

type Server struct {
	cfg     config.ExporterConfig
	coreCfg config.CoreConfig
	nc      *nats.Conn
	version string
	logger  *log.Logger

	js       jetstream.JetStream
	evt      jetstream.Stream
	memoryKV jetstream.KeyValue
	stats    *evtStats
	s3       *s3Scanner

	replayTarget uint64
	replayFailed atomic.Bool
}

func New(opts Options) (*Server, error) {
	if opts.NC == nil {
		return nil, fmt.Errorf("nats connection is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.WithPrefix("exporter")
	}
	s3, err := newS3Scanner(opts.Core.Assets, opts.Config.S3TimeoutOrDefault())
	if err != nil {
		return nil, err
	}
	version := opts.Version
	if version == "" {
		version = "unknown"
	}
	return &Server{
		cfg:     opts.Config,
		coreCfg: opts.Core,
		nc:      opts.NC,
		js:      opts.JS,
		version: version,
		logger:  logger,
		stats:   newEVTStats(),
		s3:      s3,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	if err := s.initResources(ctx); err != nil {
		return err
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(newCollector(s))

	mux := http.NewServeMux()
	mux.Handle(s.cfg.PathOrDefault(), promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	httpServer := &http.Server{
		Addr:              net.JoinHostPort(s.cfg.BindAddressOrDefault(), fmt.Sprint(s.cfg.PortOrDefault())),
		Handler:           mux,
		ReadHeaderTimeout: readTimeout,
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.runEVTReplay(gctx)
	})
	if s.s3 != nil {
		g.Go(func() error {
			s.s3.run(gctx, s.cfg.S3RefreshIntervalOrDefault())
			return nil
		})
	}
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		s.logger.Info("Starting Chatto exporter", "url", s.url())
		err := httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (s *Server) initResources(ctx context.Context) error {
	js := s.js
	if js == nil {
		var err error
		js, err = jetstream.New(s.nc, jetstream.WithDefaultTimeout(30*time.Second))
		if err != nil {
			return fmt.Errorf("create JetStream context: %w", err)
		}
	}
	evt, err := js.Stream(ctx, "EVT")
	if err != nil {
		return fmt.Errorf("open EVT stream: %w", err)
	}
	memoryKV, err := js.KeyValue(ctx, "MEMORY_CACHE")
	if err != nil {
		return fmt.Errorf("open MEMORY_CACHE KV bucket: %w", err)
	}
	info, err := evt.Info(ctx)
	if err != nil {
		return fmt.Errorf("read EVT stream info: %w", err)
	}
	s.js = js
	s.evt = evt
	s.memoryKV = memoryKV
	s.replayTarget = info.State.LastSeq
	if s.replayTarget == 0 {
		s.stats.markReplayComplete()
	}
	return nil
}

func (s *Server) runEVTReplay(ctx context.Context) error {
	consumer, err := s.evt.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    []string{"evt.>"},
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		s.replayFailed.Store(true)
		return fmt.Errorf("create EVT ordered consumer: %w", err)
	}

	failed := make(chan error, 1)
	consume, err := consumer.Consume(func(msg jetstream.Msg) {
		if err := s.handleEVTMessage(msg); err != nil {
			select {
			case failed <- err:
			default:
			}
		}
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		s.logger.Warn("EVT exporter consumer error (auto-recovering)", "error", err)
	}))
	if err != nil {
		s.replayFailed.Store(true)
		return fmt.Errorf("start EVT consume: %w", err)
	}
	defer consume.Stop()

	select {
	case <-ctx.Done():
		return nil
	case err := <-failed:
		s.replayFailed.Store(true)
		return err
	}
}

func (s *Server) handleEVTMessage(msg jetstream.Msg) error {
	meta, err := msg.Metadata()
	if err != nil {
		return fmt.Errorf("EVT message metadata: %w", err)
	}
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data(), &event); err != nil {
		return fmt.Errorf("unmarshal EVT event at seq %d: %w", meta.Sequence.Stream, err)
	}
	s.stats.apply(&event, meta.Sequence.Stream)
	if meta.Sequence.Stream >= s.replayTarget {
		s.stats.markReplayComplete()
	}
	return nil
}

func (s *Server) presenceSnapshot(ctx context.Context) (map[string]int, error) {
	if s.memoryKV == nil {
		return nil, fmt.Errorf("MEMORY_CACHE KV bucket is not open")
	}
	lister, err := s.memoryKV.ListKeysFiltered(ctx, "presence.>")
	if err != nil {
		return nil, err
	}

	counts := map[string]int{
		"online":         0,
		"away":           0,
		"do_not_disturb": 0,
	}

	keys, err := uniqueListedKeys(lister)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		entry, err := s.memoryKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, err
		}
		var presence corev1.UserPresence
		if err := proto.Unmarshal(entry.Value(), &presence); err != nil {
			return nil, err
		}
		counts[presenceStatusLabel(presence.GetStatus())]++
	}
	return counts, nil
}

func uniqueListedKeys(lister jetstream.KeyLister) ([]string, error) {
	seen := make(map[string]struct{})
	keys := make([]string, 0)
	for key := range lister.Keys() {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if err := lister.Stop(); err != nil {
		return nil, err
	}
	return keys, nil
}

func presenceStatusLabel(status corev1.UserPresenceStatus) string {
	switch status {
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY:
		return "away"
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_DO_NOT_DISTURB:
		return "do_not_disturb"
	default:
		return "online"
	}
}

func (s *Server) url() string {
	return "http://" + net.JoinHostPort(s.cfg.BindAddressOrDefault(), fmt.Sprint(s.cfg.PortOrDefault())) + s.cfg.PathOrDefault()
}
