package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
)

const (
	defaultStreamMessageReadConcurrency = 16
	minimumCacheCleanupInterval         = 10 * time.Millisecond
	maximumCacheCleanupInterval         = time.Minute
)

// ErrInvalidStreamMessageReaderConfig marks invalid exact-read concurrency or
// cache lifetime settings.
var ErrInvalidStreamMessageReaderConfig = errors.New("invalid stream message reader config")

// ErrStreamMessageReaderAlreadyStarted is returned when Run is called more
// than once on the same reader. A reader owns one cache cleanup lifecycle.
var ErrStreamMessageReaderAlreadyStarted = errors.New("stream message reader already started")

type exactStreamMessageSource interface {
	GetMsg(context.Context, uint64, ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error)
}

// StreamMessageReaderConfig controls exact stream reads and the optional
// process-local message cache.
type StreamMessageReaderConfig struct {
	// CacheIdleTTL sets sliding idle expiry. Zero disables the cache.
	CacheIdleTTL time.Duration
	// MaxConcurrentReads limits broker reads across calls. Zero uses 16.
	MaxConcurrentReads int
	// Logger receives cache-miss, batch, expiry, and clearing diagnostics.
	Logger Logger
}

// StreamMessageReader loads opaque records at exact stream sequences. It can
// retain successful reads in a disposable process-local cache with sliding
// idle expiry. The cache is bound to the supplied stream handle, so sequence
// numbers from different streams never share a key space.
//
// Run should execute for the lifetime of a reader with caching enabled. Reads
// still reject expired entries when Run is not active, but idle entries are
// reclaimed in the background only while Run is active.
type StreamMessageReader struct {
	source        exactStreamMessageSource
	cacheIdleTTL  time.Duration
	readSemaphore chan struct{}
	cache         *ttlcache.Cache[uint64, EncodedSubjectRecord]
	logger        Logger

	invalidationMu  sync.Mutex
	cacheGeneration uint64
	runStarted      atomic.Bool
}

// NewStreamMessageReader binds exact reads and an optional cache to one
// JetStream stream.
func NewStreamMessageReader(stream jetstream.Stream, config StreamMessageReaderConfig) (*StreamMessageReader, error) {
	return newStreamMessageReader(stream, config)
}

func newStreamMessageReader(source exactStreamMessageSource, config StreamMessageReaderConfig) (*StreamMessageReader, error) {
	if source == nil {
		return nil, fmt.Errorf("%w: stream is unavailable", ErrInvalidStreamMessageReaderConfig)
	}
	if config.CacheIdleTTL < 0 {
		return nil, fmt.Errorf("%w: cache idle TTL must not be negative", ErrInvalidStreamMessageReaderConfig)
	}
	if config.MaxConcurrentReads < 0 {
		return nil, fmt.Errorf("%w: maximum concurrent reads must not be negative", ErrInvalidStreamMessageReaderConfig)
	}
	maxConcurrentReads := config.MaxConcurrentReads
	if maxConcurrentReads == 0 {
		maxConcurrentReads = defaultStreamMessageReadConcurrency
	}
	reader := &StreamMessageReader{
		source:        source,
		cacheIdleTTL:  config.CacheIdleTTL,
		readSemaphore: make(chan struct{}, maxConcurrentReads),
		logger:        normalizeLogger(config.Logger),
	}
	if config.CacheIdleTTL > 0 {
		reader.cache = ttlcache.New(
			ttlcache.WithTTL[uint64, EncodedSubjectRecord](config.CacheIdleTTL),
		)
	}
	return reader, nil
}

// Message loads one exact stream sequence. Successful cache hits extend the
// entry's idle lifetime. Returned payload bytes do not alias the cache or the
// NATS client response.
func (r *StreamMessageReader) Message(ctx context.Context, sequence uint64) (EncodedSubjectRecord, error) {
	startedAt := time.Now()
	record, cacheMiss, err := r.message(ctx, sequence)
	if cacheMiss && r.cache != nil {
		r.logger.Debug(
			"Stream message cache miss",
			"stream_sequence", sequence,
			"entries", r.cache.Len(),
			"duration", time.Since(startedAt),
		)
	}
	return record, err
}

func (r *StreamMessageReader) message(ctx context.Context, sequence uint64) (EncodedSubjectRecord, bool, error) {
	if r == nil || r.source == nil {
		return EncodedSubjectRecord{}, false, fmt.Errorf("stream message reader is unavailable")
	}
	if sequence == 0 {
		return EncodedSubjectRecord{}, false, fmt.Errorf("stream sequence must be positive")
	}
	if record, ok := r.cached(sequence); ok {
		return record, false, nil
	}

	select {
	case r.readSemaphore <- struct{}{}:
		defer func() { <-r.readSemaphore }()
	case <-ctx.Done():
		return EncodedSubjectRecord{}, false, ctx.Err()
	}

	// A read that waited for capacity can reuse a record loaded meanwhile. The
	// miss and cache-generation capture are atomic with cache invalidation.
	record, ok, cacheGeneration := r.cachedOrBeginRead(sequence)
	if ok {
		return record, false, nil
	}
	message, err := r.source.GetMsg(ctx, sequence)
	if err != nil {
		return EncodedSubjectRecord{}, true, fmt.Errorf("read stream sequence %d: %w", sequence, err)
	}
	if message == nil {
		return EncodedSubjectRecord{}, true, fmt.Errorf("read stream sequence %d: broker returned no record", sequence)
	}
	if message.Sequence != sequence {
		return EncodedSubjectRecord{}, true, fmt.Errorf(
			"read stream sequence %d: broker returned sequence %d",
			sequence,
			message.Sequence,
		)
	}
	record = EncodedSubjectRecord{
		Subject:  message.Subject,
		Sequence: message.Sequence,
		Data:     message.Data,
	}
	if message.Header != nil {
		record.ID = message.Header.Get(nats.MsgIdHdr)
	}
	r.storeIfCurrent(record, cacheGeneration)
	return cloneEncodedSubjectRecord(record), true, nil
}

// Messages loads exact stream sequences with bounded process-wide
// concurrency. It reads duplicate sequences once and preserves caller order
// and duplicate positions in the result.
func (r *StreamMessageReader) Messages(ctx context.Context, sequences []uint64) ([]EncodedSubjectRecord, error) {
	if r == nil || r.source == nil {
		return nil, fmt.Errorf("stream message reader is unavailable")
	}
	if len(sequences) == 0 {
		return nil, nil
	}

	unique := make([]uint64, 0, len(sequences))
	uniqueIndex := make(map[uint64]int, len(sequences))
	for _, sequence := range sequences {
		if sequence == 0 {
			return nil, fmt.Errorf("stream sequence must be positive")
		}
		if _, exists := uniqueIndex[sequence]; exists {
			continue
		}
		uniqueIndex[sequence] = len(unique)
		unique = append(unique, sequence)
	}

	loaded := make([]EncodedSubjectRecord, len(unique))
	cacheMisses := make([]bool, len(unique))
	startedAt := time.Now()
	reads, readCtx := errgroup.WithContext(ctx)
	reads.SetLimit(cap(r.readSemaphore))
	for index, sequence := range unique {
		reads.Go(func() error {
			record, cacheMiss, err := r.message(readCtx, sequence)
			if err != nil {
				return err
			}
			loaded[index] = record
			cacheMisses[index] = cacheMiss
			return nil
		})
	}
	if err := reads.Wait(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.cache != nil {
		missCount := 0
		for _, cacheMiss := range cacheMisses {
			if cacheMiss {
				missCount++
			}
		}
		r.logger.Debug(
			"Stream message cache batch read",
			"requested", len(sequences),
			"unique", len(unique),
			"hits", len(unique)-missCount,
			"misses", missCount,
			"entries", r.cache.Len(),
			"duration", time.Since(startedAt),
		)
	}

	result := make([]EncodedSubjectRecord, len(sequences))
	for i, sequence := range sequences {
		result[i] = cloneEncodedSubjectRecord(loaded[uniqueIndex[sequence]])
	}
	return result, nil
}

// Forget removes exact sequences from the local cache. It has no effect on
// the durable stream.
func (r *StreamMessageReader) Forget(sequences ...uint64) {
	if r == nil || r.cache == nil || len(sequences) == 0 {
		return
	}
	r.invalidationMu.Lock()
	defer r.invalidationMu.Unlock()
	r.cacheGeneration++
	for _, sequence := range sequences {
		r.cache.Delete(sequence)
	}
}

// Clear removes all records from the local cache. It has no effect on the
// durable stream.
func (r *StreamMessageReader) Clear() {
	if r == nil || r.cache == nil {
		return
	}
	r.invalidationMu.Lock()
	entries := r.cache.Len()
	r.cacheGeneration++
	r.cache.DeleteAll()
	r.invalidationMu.Unlock()
	if entries > 0 {
		r.logger.Debug("Stream message cache cleared", "entries", entries)
	}
}

// Run reclaims entries whose idle lifetime has elapsed. It returns ctx.Err
// after cancellation and clears the cache before it returns. Run is
// single-use; construct a new reader for a new lifecycle.
func (r *StreamMessageReader) Run(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("stream message reader is unavailable")
	}
	if !r.runStarted.CompareAndSwap(false, true) {
		return ErrStreamMessageReaderAlreadyStarted
	}
	if r.cache == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	interval := max(r.cacheIdleTTL/2, minimumCacheCleanupInterval)
	interval = min(interval, maximumCacheCleanupInterval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer r.Clear()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.deleteExpired()
		}
	}
}

func (r *StreamMessageReader) deleteExpired() {
	r.invalidationMu.Lock()
	before := r.cache.Metrics().Evictions
	r.cache.DeleteExpired()
	expired := r.cache.Metrics().Evictions - before
	entries := r.cache.Len()
	r.invalidationMu.Unlock()
	if expired > 0 {
		r.logger.Debug(
			"Stream message cache entries expired",
			"count", expired,
			"entries", entries,
		)
	}
}

func (r *StreamMessageReader) cached(sequence uint64) (EncodedSubjectRecord, bool) {
	if r.cache == nil {
		return EncodedSubjectRecord{}, false
	}
	item := r.cache.Get(sequence)
	if item == nil {
		return EncodedSubjectRecord{}, false
	}
	return cloneEncodedSubjectRecord(item.Value()), true
}

func (r *StreamMessageReader) cachedOrBeginRead(sequence uint64) (EncodedSubjectRecord, bool, uint64) {
	if r.cache == nil {
		return EncodedSubjectRecord{}, false, 0
	}
	r.invalidationMu.Lock()
	defer r.invalidationMu.Unlock()
	if item := r.cache.Get(sequence); item != nil {
		return cloneEncodedSubjectRecord(item.Value()), true, 0
	}
	return EncodedSubjectRecord{}, false, r.cacheGeneration
}

// storeIfCurrent does not let an in-flight read refill the cache after Forget
// or Clear. Invalidation is rare, so it can discard unrelated concurrent fills.
func (r *StreamMessageReader) storeIfCurrent(record EncodedSubjectRecord, cacheGeneration uint64) {
	if r.cache == nil {
		return
	}
	r.invalidationMu.Lock()
	defer r.invalidationMu.Unlock()
	if cacheGeneration != r.cacheGeneration {
		return
	}
	r.cache.Set(record.Sequence, cloneEncodedSubjectRecord(record), ttlcache.DefaultTTL)
}

func cloneEncodedSubjectRecord(record EncodedSubjectRecord) EncodedSubjectRecord {
	record.Data = cloneBytes(record.Data)
	return record
}

func cloneBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	return append([]byte(nil), data...)
}
