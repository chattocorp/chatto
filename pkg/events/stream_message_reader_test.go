package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type exactMessageSourceStub struct {
	mu      sync.Mutex
	reads   map[uint64]int
	msgs    map[uint64]*jetstream.RawStreamMsg
	err     error
	started chan struct{}
	release chan struct{}
	active  int
	maximum int
}

func (s *exactMessageSourceStub) GetMsg(ctx context.Context, sequence uint64, _ ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	s.mu.Lock()
	s.reads[sequence]++
	s.active++
	s.maximum = max(s.maximum, s.active)
	message := s.msgs[sequence]
	err := s.err
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.active--
		s.mu.Unlock()
	}()
	if s.started != nil {
		s.started <- struct{}{}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, jetstream.ErrMsgNotFound
	}
	return message, nil
}

func newTestStreamMessageReader(t *testing.T, source exactStreamMessageSource, config StreamMessageReaderConfig) *StreamMessageReader {
	t.Helper()
	reader, err := newStreamMessageReader(source, config)
	if err != nil {
		t.Fatalf("newStreamMessageReader: %v", err)
	}
	return reader
}

func TestStreamMessageReaderCachesClonedRecords(t *testing.T) {
	header := nats.Header{}
	header.Set(nats.MsgIdHdr, "event-7")
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			7: {Subject: "evt.room.room.message_posted", Sequence: 7, Header: header, Data: []byte("payload")},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: time.Minute})

	first, err := reader.Message(context.Background(), 7)
	if err != nil {
		t.Fatalf("Message first read: %v", err)
	}
	first.Subject = "changed"
	first.Data[0] = 'X'
	second, err := reader.Message(context.Background(), 7)
	if err != nil {
		t.Fatalf("Message cached read: %v", err)
	}
	if second.Subject != "evt.room.room.message_posted" || string(second.Data) != "payload" || second.ID != "event-7" {
		t.Fatalf("cached record = %#v, want unchanged broker record", second)
	}
	if source.reads[7] != 1 {
		t.Fatalf("source reads = %d, want 1", source.reads[7])
	}
}

func TestStreamMessageReaderUsesSlidingIdleExpiry(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			4: {Subject: "evt.room.room.user_joined", Sequence: 4, Data: []byte("four")},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: 200 * time.Millisecond})

	if _, err := reader.Message(context.Background(), 4); err != nil {
		t.Fatalf("Message initial read: %v", err)
	}
	first := reader.cache.Get(4, ttlcache.WithDisableTouchOnHit[uint64, EncodedSubjectRecord]())
	if first == nil {
		t.Fatal("cache entry after initial read = nil")
	}
	firstExpiry := first.ExpiresAt()
	time.Sleep(50 * time.Millisecond)
	if _, err := reader.Message(context.Background(), 4); err != nil {
		t.Fatalf("Message touched read: %v", err)
	}
	second := reader.cache.Get(4, ttlcache.WithDisableTouchOnHit[uint64, EncodedSubjectRecord]())
	if second == nil || !second.ExpiresAt().After(firstExpiry) {
		t.Fatalf("expiration after cache hit = %v, want after %v", second, firstExpiry)
	}
	if source.reads[4] != 1 {
		t.Fatalf("source reads after active use = %d, want 1", source.reads[4])
	}
	time.Sleep(time.Until(second.ExpiresAt()) + 10*time.Millisecond)
	if _, err := reader.Message(context.Background(), 4); err != nil {
		t.Fatalf("Message after idle expiry: %v", err)
	}
	if source.reads[4] != 2 {
		t.Fatalf("source reads after idle expiry = %d, want 2", source.reads[4])
	}
}

func TestStreamMessageReaderForgetAndClear(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			1: {Subject: "evt.one", Sequence: 1, Data: []byte("one")},
			2: {Subject: "evt.two", Sequence: 2, Data: []byte("two")},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: time.Minute})
	if _, err := reader.Messages(context.Background(), []uint64{1, 2}); err != nil {
		t.Fatalf("Messages initial read: %v", err)
	}
	reader.Forget(1)
	if _, err := reader.Messages(context.Background(), []uint64{1, 2}); err != nil {
		t.Fatalf("Messages after Forget: %v", err)
	}
	if source.reads[1] != 2 || source.reads[2] != 1 {
		t.Fatalf("source reads after Forget = %v, want map[1:2 2:1]", source.reads)
	}
	reader.Clear()
	if _, err := reader.Messages(context.Background(), []uint64{1, 2}); err != nil {
		t.Fatalf("Messages after Clear: %v", err)
	}
	if source.reads[1] != 3 || source.reads[2] != 2 {
		t.Fatalf("source reads after Clear = %v, want map[1:3 2:2]", source.reads)
	}
}

func TestStreamMessageReaderInvalidationPreventsInflightReadFromRefillingCache(t *testing.T) {
	for name, invalidate := range map[string]func(*StreamMessageReader){
		"forget": func(reader *StreamMessageReader) { reader.Forget(1) },
		"clear":  func(reader *StreamMessageReader) { reader.Clear() },
	} {
		t.Run(name, func(t *testing.T) {
			source := &exactMessageSourceStub{
				reads:   make(map[uint64]int),
				msgs:    map[uint64]*jetstream.RawStreamMsg{1: {Subject: "evt.one", Sequence: 1, Data: []byte("one")}},
				started: make(chan struct{}, 1),
				release: make(chan struct{}),
			}
			reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: time.Minute})
			done := make(chan error, 1)
			go func() {
				_, err := reader.Message(context.Background(), 1)
				done <- err
			}()
			<-source.started
			invalidate(reader)
			close(source.release)
			if err := <-done; err != nil {
				t.Fatalf("Message: %v", err)
			}
			if _, err := reader.Message(context.Background(), 1); err != nil {
				t.Fatalf("Message after invalidation: %v", err)
			}
			if source.reads[1] != 2 {
				t.Fatalf("source reads = %d, want 2", source.reads[1])
			}
		})
	}
}

func TestStreamMessageReaderMessagesDeduplicateAndPreserveOrder(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			4: {Subject: "evt.four", Sequence: 4, Data: []byte("four")},
			9: {Subject: "evt.nine", Sequence: 9, Data: []byte("nine")},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{})

	records, err := reader.Messages(context.Background(), []uint64{9, 4, 9})
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if got := []string{string(records[0].Data), string(records[1].Data), string(records[2].Data)}; got[0] != "nine" || got[1] != "four" || got[2] != "nine" {
		t.Fatalf("record order = %v, want [nine four nine]", got)
	}
	if source.reads[9] != 1 || source.reads[4] != 1 {
		t.Fatalf("source reads = %v, want one read per unique sequence", source.reads)
	}
}

func TestStreamMessageReaderMessagesHonorsCancellationWithCachedRecords(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			1: {Subject: "evt.one", Sequence: 1, Data: []byte("one")},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: time.Minute})
	if _, err := reader.Message(context.Background(), 1); err != nil {
		t.Fatalf("Message: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Messages(ctx, []uint64{1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Messages error = %v, want context.Canceled", err)
	}
}

func TestStreamMessageReaderBoundsConcurrentReadsAcrossCalls(t *testing.T) {
	const limit = 3
	source := &exactMessageSourceStub{
		reads:   make(map[uint64]int),
		msgs:    make(map[uint64]*jetstream.RawStreamMsg),
		started: make(chan struct{}, limit+1),
		release: make(chan struct{}),
	}
	for sequence := uint64(1); sequence <= limit*2; sequence++ {
		source.msgs[sequence] = &jetstream.RawStreamMsg{Subject: "evt.test", Sequence: sequence}
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{MaxConcurrentReads: limit})
	done := make(chan error, limit*2)
	for sequence := uint64(1); sequence <= limit*2; sequence++ {
		go func() {
			_, err := reader.Message(context.Background(), sequence)
			done <- err
		}()
	}
	for range limit {
		<-source.started
	}
	select {
	case <-source.started:
		t.Fatal("more than the configured number of reads started concurrently")
	default:
	}
	close(source.release)
	for range limit * 2 {
		if err := <-done; err != nil {
			t.Fatalf("Message: %v", err)
		}
	}
	source.mu.Lock()
	maximum := source.maximum
	source.mu.Unlock()
	if maximum != limit {
		t.Fatalf("maximum concurrent reads = %d, want %d", maximum, limit)
	}
}

func TestStreamMessageReaderDoesNotCacheFailures(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs:  make(map[uint64]*jetstream.RawStreamMsg),
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: time.Minute})
	for range 2 {
		if _, err := reader.Message(context.Background(), 8); !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Fatalf("Message error = %v, want jetstream.ErrMsgNotFound", err)
		}
	}
	if source.reads[8] != 2 {
		t.Fatalf("source reads = %d, want 2", source.reads[8])
	}
}

func TestStreamMessageReaderRejectsUnexpectedSequence(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			7: {Subject: "evt.test", Sequence: 8},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: time.Minute})
	if _, err := reader.Message(context.Background(), 7); err == nil {
		t.Fatal("Message error = nil, want unexpected sequence error")
	}
}

func TestStreamMessageReaderConfigValidation(t *testing.T) {
	source := &exactMessageSourceStub{}
	for _, config := range []StreamMessageReaderConfig{
		{CacheIdleTTL: -time.Second},
		{MaxConcurrentReads: -1},
	} {
		if _, err := newStreamMessageReader(source, config); !errors.Is(err, ErrInvalidStreamMessageReaderConfig) {
			t.Fatalf("newStreamMessageReader(%+v) error = %v, want ErrInvalidStreamMessageReaderConfig", config, err)
		}
	}
}

func TestStreamMessageReaderRunRemovesExpiredEntriesAndClearsOnShutdown(t *testing.T) {
	source := &exactMessageSourceStub{
		reads: make(map[uint64]int),
		msgs: map[uint64]*jetstream.RawStreamMsg{
			1: {Subject: "evt.one", Sequence: 1},
			2: {Subject: "evt.two", Sequence: 2},
		},
	}
	reader := newTestStreamMessageReader(t, source, StreamMessageReaderConfig{CacheIdleTTL: 20 * time.Millisecond})
	expired := make(chan uint64, 2)
	unsubscribe := reader.cache.OnEviction(func(_ context.Context, reason ttlcache.EvictionReason, item *ttlcache.Item[uint64, EncodedSubjectRecord]) {
		if reason == ttlcache.EvictionReasonExpired {
			expired <- item.Key()
		}
	})
	defer unsubscribe()
	if _, err := reader.Messages(context.Background(), []uint64{1, 2}); err != nil {
		t.Fatalf("Messages: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- reader.Run(ctx) }()

	for range 2 {
		select {
		case <-expired:
		case <-time.After(time.Second):
			t.Fatal("expired cache entry was not reclaimed")
		}
	}
	if _, err := reader.Message(context.Background(), 1); err != nil {
		t.Fatalf("Message after sweep: %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	remaining := reader.cache.Len()
	if remaining != 0 {
		t.Fatalf("cache entries after shutdown = %d, want 0", remaining)
	}
}

func TestStreamMessageReaderRunIsSingleUse(t *testing.T) {
	reader := newTestStreamMessageReader(t, &exactMessageSourceStub{}, StreamMessageReaderConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := reader.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if err := reader.Run(context.Background()); !errors.Is(err, ErrStreamMessageReaderAlreadyStarted) {
		t.Fatalf("repeated Run error = %v, want ErrStreamMessageReaderAlreadyStarted", err)
	}
}
