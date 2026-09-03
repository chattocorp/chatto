package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultStreamMessageReadConcurrency = 16
	maximumCacheSweepInterval           = time.Minute
)

// ErrInvalidStreamMessageReaderConfig marks invalid exact-read concurrency or
// cache lifetime settings.
var ErrInvalidStreamMessageReaderConfig = errors.New("invalid stream message reader config")

type exactStreamMessageSource interface {
	GetMsg(context.Context, uint64, ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error)
}

// StreamMessageReaderConfig controls exact stream reads and the optional
// process-local message cache. A zero CacheIdleTTL disables the cache. A zero
// MaxConcurrentReads uses a default of 16.
type StreamMessageReaderConfig struct {
	CacheIdleTTL       time.Duration
	MaxConcurrentReads int
}

type cachedStreamMessage struct {
	record    EncodedSubjectRecord
	expiresAt time.Time
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
	now           func() time.Time

	cacheMu     sync.Mutex
	cache       map[uint64]cachedStreamMessage
	inflight    map[uint64]int
	invalidated map[uint64]struct{}
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
		now:           time.Now,
	}
	if config.CacheIdleTTL > 0 {
		reader.cache = make(map[uint64]cachedStreamMessage)
		reader.inflight = make(map[uint64]int)
		reader.invalidated = make(map[uint64]struct{})
	}
	return reader, nil
}

// Message loads one exact stream sequence. Successful cache hits extend the
// entry's idle lifetime. Returned payload bytes do not alias the cache or the
// NATS client response.
func (r *StreamMessageReader) Message(ctx context.Context, sequence uint64) (EncodedSubjectRecord, error) {
	if r == nil || r.source == nil {
		return EncodedSubjectRecord{}, fmt.Errorf("stream message reader is unavailable")
	}
	if sequence == 0 {
		return EncodedSubjectRecord{}, fmt.Errorf("stream sequence must be positive")
	}
	if record, ok := r.cached(sequence); ok {
		return record, nil
	}

	select {
	case r.readSemaphore <- struct{}{}:
		defer func() { <-r.readSemaphore }()
	case <-ctx.Done():
		return EncodedSubjectRecord{}, ctx.Err()
	}

	// A read that waited for capacity can reuse a record loaded meanwhile. The
	// miss and in-flight registration are atomic with cache invalidation.
	if record, ok := r.cachedOrBeginRead(sequence); ok {
		return record, nil
	}
	message, err := r.source.GetMsg(ctx, sequence)
	if err != nil {
		r.finishRead(sequence, EncodedSubjectRecord{}, false)
		return EncodedSubjectRecord{}, fmt.Errorf("read stream sequence %d: %w", sequence, err)
	}
	if message == nil {
		r.finishRead(sequence, EncodedSubjectRecord{}, false)
		return EncodedSubjectRecord{}, fmt.Errorf("read stream sequence %d: broker returned no record", sequence)
	}
	if message.Sequence != sequence {
		r.finishRead(sequence, EncodedSubjectRecord{}, false)
		return EncodedSubjectRecord{}, fmt.Errorf(
			"read stream sequence %d: broker returned sequence %d",
			sequence,
			message.Sequence,
		)
	}
	record := EncodedSubjectRecord{
		Subject:  message.Subject,
		Sequence: message.Sequence,
		Data:     message.Data,
	}
	if message.Header != nil {
		record.ID = message.Header.Get(nats.MsgIdHdr)
	}
	r.finishRead(sequence, record, true)
	return cloneEncodedSubjectRecord(record), nil
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

	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan uint64, len(unique))
	for _, sequence := range unique {
		jobs <- sequence
	}
	close(jobs)

	loaded := make([]EncodedSubjectRecord, len(unique))
	var firstErr error
	var errOnce sync.Once
	workerCount := min(cap(r.readSemaphore), len(unique))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for sequence := range jobs {
				if readCtx.Err() != nil {
					return
				}
				record, err := r.Message(readCtx, sequence)
				if err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				loaded[uniqueIndex[sequence]] = record
			}
		}()
	}
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
	if r == nil || r.cache == nil {
		return
	}
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	for _, sequence := range sequences {
		delete(r.cache, sequence)
		if r.inflight[sequence] > 0 {
			r.invalidated[sequence] = struct{}{}
		}
	}
}

// Clear removes all records from the local cache. It has no effect on the
// durable stream.
func (r *StreamMessageReader) Clear() {
	if r == nil || r.cache == nil {
		return
	}
	r.cacheMu.Lock()
	clear(r.cache)
	for sequence := range r.inflight {
		r.invalidated[sequence] = struct{}{}
	}
	r.cacheMu.Unlock()
}

// Run reclaims entries whose idle lifetime has elapsed. It returns ctx.Err
// after cancellation and clears the cache before it returns.
func (r *StreamMessageReader) Run(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("stream message reader is unavailable")
	}
	if r.cacheIdleTTL == 0 {
		<-ctx.Done()
		return ctx.Err()
	}
	interval := min(r.cacheIdleTTL/2, maximumCacheSweepInterval)
	if interval <= 0 {
		interval = r.cacheIdleTTL
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer r.Clear()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			r.removeExpired(now)
		}
	}
}

func (r *StreamMessageReader) cached(sequence uint64) (EncodedSubjectRecord, bool) {
	if r.cache == nil {
		return EncodedSubjectRecord{}, false
	}
	now := r.now()
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	entry, ok := r.cache[sequence]
	if !ok {
		return EncodedSubjectRecord{}, false
	}
	if !now.Before(entry.expiresAt) {
		delete(r.cache, sequence)
		return EncodedSubjectRecord{}, false
	}
	entry.expiresAt = now.Add(r.cacheIdleTTL)
	r.cache[sequence] = entry
	return cloneEncodedSubjectRecord(entry.record), true
}

func (r *StreamMessageReader) cachedOrBeginRead(sequence uint64) (EncodedSubjectRecord, bool) {
	if r.cache == nil {
		return EncodedSubjectRecord{}, false
	}
	now := r.now()
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	if entry, ok := r.cache[sequence]; ok {
		if now.Before(entry.expiresAt) {
			entry.expiresAt = now.Add(r.cacheIdleTTL)
			r.cache[sequence] = entry
			return cloneEncodedSubjectRecord(entry.record), true
		}
		delete(r.cache, sequence)
	}
	r.inflight[sequence]++
	return EncodedSubjectRecord{}, false
}

func (r *StreamMessageReader) finishRead(sequence uint64, record EncodedSubjectRecord, cacheRecord bool) {
	if r.cache == nil {
		return
	}
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.inflight[sequence]--
	if r.inflight[sequence] == 0 {
		delete(r.inflight, sequence)
	}
	_, invalidated := r.invalidated[sequence]
	if invalidated && r.inflight[sequence] == 0 {
		delete(r.invalidated, sequence)
	}
	if cacheRecord && !invalidated {
		r.cache[record.Sequence] = cachedStreamMessage{
			record:    cloneEncodedSubjectRecord(record),
			expiresAt: r.now().Add(r.cacheIdleTTL),
		}
	}
}

func (r *StreamMessageReader) removeExpired(now time.Time) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	for sequence, entry := range r.cache {
		if !now.Before(entry.expiresAt) {
			delete(r.cache, sequence)
		}
	}
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
