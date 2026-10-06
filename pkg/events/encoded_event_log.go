package events

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	mrand "math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// ErrConflict marks an optimistic-concurrency mismatch. Callers can use
// errors.Is without depending on NATS API error codes.
var ErrConflict = errors.New("optimistic concurrency sequence mismatch")

// ErrInvalidEncodedRecord marks a record without the stable identifier needed
// for JetStream message deduplication.
var ErrInvalidEncodedRecord = errors.New("invalid encoded event record")

// ErrMissingOCC is returned when a write has no optimistic concurrency guard:
// AppendAt with a zero Expectation, or an atomic batch without a guarded
// entry. There is no publish-without-OCC path through the event log.
var ErrMissingOCC = errors.New("missing optimistic concurrency guard")

// ErrDuplicateBatchMessageID reports that JetStream rejected an atomic batch
// because at least one record ID is still inside the stream's de-duplication
// window. Callers can safely fall back to idempotent single-record publishes.
var ErrDuplicateBatchMessageID = errors.New("atomic batch contains duplicate message id")

// ErrInvalidOCC is returned for a guard that JetStream cannot evaluate:
// a filter guard without a filter, or a stream guard on a batch entry other
// than the first. A stream guard belongs on the first batch entry because it
// fences the committed stream state that precedes the complete batch.
var ErrInvalidOCC = errors.New("invalid optimistic concurrency guard")

// NATS does not currently expose the atomic-batch duplicate-ID server code.
const jetStreamDuplicateBatchMessageIDErrorCode = 10201

// ErrInvalidSubjectReadLimit marks a page request that cannot provide a
// bounded result.
var ErrInvalidSubjectReadLimit = errors.New("invalid subject record read limit")

// StreamPosition identifies a committed stream sequence together with the
// subject or subject filter that made that sequence relevant to the caller.
type StreamPosition struct {
	SubjectFilter string
	Seq           uint64
}

// SubjectPosition returns a stream position for an exact subject or wildcard
// subject filter.
func SubjectPosition(subjectFilter string, seq uint64) StreamPosition {
	return StreamPosition{SubjectFilter: subjectFilter, Seq: seq}
}

// IsZero reports whether the position points at no stream message.
func (p StreamPosition) IsZero() bool {
	return p.Seq == 0
}

// EncodedRecord is one opaque durable event payload. ID becomes the NATS
// message ID, may appear in diagnostics, and therefore must be a stable,
// opaque, non-sensitive identifier across retries.
type EncodedRecord struct {
	ID   string
	Data []byte
	// TTL requests broker-side physical expiry for this record. Zero uses the
	// stream's retention policy. Applications remain responsible for their own
	// semantic expiry boundary.
	TTL time.Duration
}

// EncodedSubjectRecord preserves a durable subject alongside its opaque
// payload. Subject is caller-owned metadata and must be safe to log.
type EncodedSubjectRecord struct {
	Subject  string
	Sequence uint64
	ID       string
	Data     []byte
}

// EncodedBatchEntry is one record in an atomic publish batch. Expect is the
// entry's optional OCC guard. Only the first entry can carry a stream guard.
// At least one entry in a batch must carry a guard.
//
// JetStream evaluates every entry against committed state at batch acceptance.
// It does not advance an entry's expected sequence for earlier members of the
// same batch, so callers must avoid dependent same-subject OCC entries.
type EncodedBatchEntry struct {
	Subject string
	Record  EncodedRecord
	Expect  Expectation
}

// EncodedEventLog owns opaque-byte JetStream reads and OCC-only writes.
// Application adapters remain responsible for event validation, encoding, and
// subject policy.
type EncodedEventLog struct {
	js     jetstream.JetStream
	stream jetstream.Stream
	// streamMu prevents Stream.Info, which refreshes cached metadata in the
	// nats.go stream handle, from racing with operations that read that cache.
	streamMu sync.RWMutex
	logger   *slog.Logger
}

// NewEncodedEventLog binds opaque event-log mechanics to one JetStream stream.
func NewEncodedEventLog(js jetstream.JetStream, stream jetstream.Stream, logger *slog.Logger) *EncodedEventLog {
	if js == nil || stream == nil {
		panic("events: encoded event log requires a JetStream context and a stream")
	}
	return &EncodedEventLog{js: js, stream: stream, logger: normalizeLogger(logger)}
}

// LastStreamSeq returns the current last sequence of the bound stream. Unlike
// the message count, this remains a valid OCC token when messages have been
// deleted or expired.
func (l *EncodedEventLog) LastStreamSeq(ctx context.Context) (uint64, error) {
	l.streamMu.Lock()
	defer l.streamMu.Unlock()
	info, err := l.stream.Info(ctx)
	if err != nil {
		return 0, err
	}
	return info.State.LastSeq, nil
}

const maxAppendRetries = 5

// maxByteBoundedPageFetch caps the records that one fetch of a byte-bounded
// SubjectRecordsAfterPage call transfers.
const maxByteBoundedPageFetch = 64

// Append publishes a record using the current tail of subject as its OCC
// token. Conflicts are returned so state-replacement callers can re-read and
// re-compose before retrying.
func (l *EncodedEventLog) Append(ctx context.Context, subject string, record EncodedRecord) (uint64, error) {
	if err := validateEncodedRecord(record); err != nil {
		return 0, err
	}
	expectedSeq, err := l.lastSubjectSeq(ctx, subject)
	if err != nil {
		return 0, err
	}
	sequence, _, err := l.publish(ctx, subject, record, ExpectSubjectSeq(expectedSeq))
	return sequence, err
}

// AppendEventually retries OCC conflicts with the exact same opaque record.
// Application adapters decide which event semantics make that retry safe.
func (l *EncodedEventLog) AppendEventually(ctx context.Context, subject string, record EncodedRecord) (uint64, error) {
	if err := validateEncodedRecord(record); err != nil {
		return 0, err
	}

	var lastErr error
	for attempt := 1; attempt <= maxAppendRetries; attempt++ {
		expectedSeq, err := l.lastSubjectSeq(ctx, subject)
		if err != nil {
			return 0, err
		}
		seq, _, err := l.publish(ctx, subject, record, ExpectSubjectSeq(expectedSeq))
		if err == nil {
			return seq, nil
		}
		if !errors.Is(err, ErrConflict) {
			return 0, err
		}

		lastErr = err
		if attempt == maxAppendRetries {
			break
		}
		l.logger.Debug("OCC conflict, retrying",
			"subject", subject,
			"expected_seq", expectedSeq,
			"attempt", attempt,
			"max_attempts", maxAppendRetries)
		if err := waitBeforeConflictRetry(ctx, attempt); err != nil {
			return 0, err
		}
	}
	return 0, fmt.Errorf("append after %d attempts: %w", maxAppendRetries, lastErr)
}

// waitBeforeConflictRetry waits before OCC conflict retry attempt+1. The delay
// doubles from 1ms per attempt, plus up to 5ms of jitter so that contending
// writers do not retry in lockstep.
func waitBeforeConflictRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(1<<(attempt-1))*time.Millisecond + mrand.N(5*time.Millisecond))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// AppendAt publishes a record to subject with the caller's OCC guard. A zero
// Expectation returns ErrMissingOCC.
func (l *EncodedEventLog) AppendAt(ctx context.Context, subject string, record EncodedRecord, expect Expectation) (uint64, error) {
	if err := validateEncodedRecord(record); err != nil {
		return 0, err
	}
	if err := expect.validate(); err != nil {
		return 0, err
	}
	sequence, _, err := l.publish(ctx, subject, record, expect)
	return sequence, err
}

// publish writes one record with its guard. It also reports whether JetStream
// acknowledged the record ID as a duplicate.
func (l *EncodedEventLog) publish(ctx context.Context, subject string, record EncodedRecord, expect Expectation) (uint64, bool, error) {
	publishOpts := append(expect.publishOpts(), jetstream.WithMsgID(record.ID))
	if record.TTL > 0 {
		publishOpts = append(publishOpts, jetstream.WithMsgTTL(record.TTL))
	}
	ack, err := l.js.Publish(ctx, subject, record.Data, publishOpts...)
	if err == nil {
		return ack.Sequence, ack.Duplicate, nil
	}
	if isSequenceConflict(err) {
		return 0, false, expect.conflict(subject)
	}
	return 0, false, fmt.Errorf("publish: %w", err)
}

// AppendBatch atomically publishes encoded records. Either all records land
// adjacently in stream order or none do.
func (l *EncodedEventLog) AppendBatch(ctx context.Context, entries []EncodedBatchEntry) ([]uint64, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	guards := 0
	for i, entry := range entries {
		if err := validateEncodedRecord(entry.Record); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
		if entry.Expect.isZero() {
			continue
		}
		if err := entry.Expect.validate(); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
		if entry.Expect.stream && i != 0 {
			return nil, fmt.Errorf("batch entry %d: %w: stream-tail guard must be on first entry", i, ErrInvalidOCC)
		}
		guards += entry.Expect.guards()
	}
	if guards == 0 {
		return nil, ErrMissingOCC
	}

	batchID, err := newBatchID()
	if err != nil {
		return nil, fmt.Errorf("generate batch id: %w", err)
	}
	for i, entry := range entries[:len(entries)-1] {
		if _, err := l.publishBatchEntry(ctx, entry, entryConflict(entry), batchID, uint64(i+1), false); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
	}

	commitEntry := entries[len(entries)-1]
	commitSeq, err := l.publishBatchEntry(ctx, commitEntry, batchConflict(entries, guards), batchID, uint64(len(entries)), true)
	if err != nil {
		return nil, fmt.Errorf("batch commit: %w", err)
	}
	seqs := make([]uint64, len(entries))
	for i := range entries {
		seqs[i] = commitSeq - uint64(len(entries)-1-i)
	}
	return seqs, nil
}

func (l *EncodedEventLog) publishBatchEntry(
	ctx context.Context,
	entry EncodedBatchEntry,
	conflict error,
	batchID string,
	batchSeq uint64,
	commit bool,
) (uint64, error) {
	msg := buildEncodedBatchMsg(entry, batchID, batchSeq, commit)
	resp, err := l.js.Conn().RequestMsgWithContext(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("publish: %w", err)
	}
	return decodeBatchAck(resp, conflict)
}

// entryConflict is the error for a conflict that JetStream reports for one
// staged entry. An entry without a guard can still fail on the batch.
func entryConflict(entry EncodedBatchEntry) error {
	if entry.Expect.isZero() {
		return errBatchGuardsConflict()
	}
	return entry.Expect.conflict(entry.Subject)
}

// errBatchGuardsConflict is the conflict error when JetStream does not tell
// which guard of the batch failed.
func errBatchGuardsConflict() error {
	return fmt.Errorf("atomic batch OCC guards: %w", ErrConflict)
}

// batchConflict is the error for a conflict that JetStream reports at commit.
// guards counts every guard in the batch. The error names the guard when the
// batch has exactly one.
func batchConflict(entries []EncodedBatchEntry, guards int) error {
	if guards == 1 {
		for _, entry := range entries {
			if !entry.Expect.isZero() {
				return entry.Expect.conflict(entry.Subject)
			}
		}
	}
	return errBatchGuardsConflict()
}

type pubAckEnvelope struct {
	Error *struct {
		Code        int    `json:"code"`
		ErrCode     uint16 `json:"err_code"`
		Description string `json:"description"`
	} `json:"error,omitempty"`
	Stream    string `json:"stream,omitempty"`
	Sequence  uint64 `json:"seq,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// decodeBatchAck returns the stored sequence from a batch acknowledgement, or
// conflict when JetStream reports an OCC mismatch.
func decodeBatchAck(resp *nats.Msg, conflict error) (uint64, error) {
	if len(resp.Data) == 0 {
		return 0, nil
	}
	var env pubAckEnvelope
	if err := json.Unmarshal(resp.Data, &env); err != nil {
		return 0, fmt.Errorf("decode ack: %w", err)
	}
	if env.Error != nil {
		apiErr := &jetstream.APIError{
			Code:        env.Error.Code,
			ErrorCode:   jetstream.ErrorCode(env.Error.ErrCode),
			Description: env.Error.Description,
		}
		if apiErr.ErrorCode == jetStreamDuplicateBatchMessageIDErrorCode {
			return 0, fmt.Errorf("%w: %s", ErrDuplicateBatchMessageID, env.Error.Description)
		}
		if isSequenceConflict(apiErr) {
			return 0, conflict
		}
		return 0, fmt.Errorf("server: %s (err_code=%d)", env.Error.Description, env.Error.ErrCode)
	}
	return env.Sequence, nil
}

func buildEncodedBatchMsg(
	entry EncodedBatchEntry,
	batchID string,
	batchSeq uint64,
	commit bool,
) *nats.Msg {
	hdr := nats.Header{}
	hdr.Set("Nats-Batch-Id", batchID)
	hdr.Set("Nats-Batch-Sequence", strconv.FormatUint(batchSeq, 10))
	if commit {
		hdr.Set("Nats-Batch-Commit", "1")
	}
	entry.Expect.setHeaders(hdr)
	hdr.Set(jetstream.MsgIDHeader, entry.Record.ID)
	if entry.Record.TTL > 0 {
		hdr.Set("Nats-TTL", entry.Record.TTL.String())
	}
	return &nats.Msg{Subject: entry.Subject, Header: hdr, Data: entry.Record.Data}
}

func newBatchID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// LastSubjectSeq returns the current last stream sequence for an exact subject
// or wildcard subject filter.
func (l *EncodedEventLog) LastSubjectSeq(ctx context.Context, subjectOrFilter string) (uint64, error) {
	pos, err := l.LastSubjectPosition(ctx, subjectOrFilter)
	return pos.Seq, err
}

// LastSubjectPosition returns the current position for an exact subject or
// wildcard subject filter.
func (l *EncodedEventLog) LastSubjectPosition(
	ctx context.Context,
	subjectOrFilter string,
) (StreamPosition, error) {
	seq, err := l.lastSubjectSeq(ctx, subjectOrFilter)
	if err != nil {
		return StreamPosition{}, err
	}
	return SubjectPosition(subjectOrFilter, seq), nil
}

// SubjectRecordPage is one bounded page of opaque subject records.
// LastSequence is suitable as the afterSeq cursor for the next page.
type SubjectRecordPage struct {
	Records      []EncodedSubjectRecord
	LastSequence uint64
	More         bool
}

// SubjectRecordsAfterPage returns at most maxRecords matching opaque records
// after afterSeq. When maxBytes is positive, the returned payload bytes are
// also bounded by maxBytes: the page ends before the first record that does
// not fit, and More reports the rest. When the first record of a page has a
// payload larger than maxBytes, the call returns ErrInvalidSubjectReadLimit.
// More indicates that the page's point-in-time result had additional records.
// Callers can pass LastSequence to the next request without exposing
// JetStream consumer coordinates.
func (l *EncodedEventLog) SubjectRecordsAfterPage(
	ctx context.Context,
	subject string,
	afterSeq uint64,
	maxRecords int,
	maxBytes int,
) (SubjectRecordPage, error) {
	if maxRecords <= 0 || maxBytes < 0 {
		return SubjectRecordPage{}, ErrInvalidSubjectReadLimit
	}
	deliverPolicy := jetstream.DeliverAllPolicy
	var startSeq uint64
	if afterSeq > 0 {
		deliverPolicy = jetstream.DeliverByStartSequencePolicy
		startSeq = afterSeq + 1
	}
	l.streamMu.RLock()
	consumer, err := l.stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    []string{subject},
		DeliverPolicy:     deliverPolicy,
		OptStartSeq:       startSeq,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	l.streamMu.RUnlock()
	if err != nil {
		return SubjectRecordPage{}, err
	}
	defer func() {
		l.streamMu.RLock()
		defer l.streamMu.RUnlock()
		_ = l.stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)
	}()

	info, err := consumer.Info(ctx)
	if err != nil {
		return SubjectRecordPage{}, err
	}
	target := min(uint64(maxRecords), info.NumPending)
	page := SubjectRecordPage{Records: make([]EncodedSubjectRecord, 0, target)}
	var bytesRead, largest, previousBatch int
	for uint64(len(page.Records)) < target && (maxBytes == 0 || bytesRead < maxBytes) {
		// A fetch asks for no more records than were pending when the page
		// started, so it normally returns at once. Records deleted since then
		// make it wait for FetchMaxWait.
		remaining := int(target - uint64(len(page.Records)))
		batchSize := remaining
		if maxBytes > 0 {
			// Size each fetch from the largest payload seen so far. The
			// first fetch reads one record because no size is known yet.
			// Later fetches at most double and never exceed
			// maxByteBoundedPageFetch records, so records larger than the
			// ones seen so far can make one fetch transfer at most
			// maxByteBoundedPageFetch payloads beyond the remaining budget.
			batchSize = 1
			if largest > 0 {
				batchSize = min(max((maxBytes-bytesRead)/largest, 1), 2*previousBatch, maxByteBoundedPageFetch, remaining)
			}
			previousBatch = batchSize
		}
		msgs, err := consumer.Fetch(batchSize, jetstream.FetchMaxWait(10*time.Second))
		if err != nil {
			return SubjectRecordPage{}, err
		}
		fetched, full, err := appendPageRecords(&page, msgs, maxBytes, &bytesRead, &largest)
		if err != nil {
			return SubjectRecordPage{}, err
		}
		if full || fetched == 0 {
			break
		}
	}
	page.More = info.NumPending > uint64(len(page.Records))
	return page, nil
}

// appendPageRecords appends fetched records to page until the next payload
// would exceed maxBytes. It returns the number of fetched messages and whether
// the page is full. The consumer has already delivered every fetched message,
// so the page must end at the first record that it cannot hold; the next page
// reads that record again. A payload larger than maxBytes is an error when it
// is the first record of the page, because no page can contain it. largest
// tracks the largest payload size seen, counting an empty payload as one byte.
func appendPageRecords(page *SubjectRecordPage, msgs jetstream.MessageBatch, maxBytes int, bytesRead, largest *int) (int, bool, error) {
	fetched := 0
	full := false
	for msg := range msgs.Messages() {
		fetched++
		if full {
			// Drain the batch so that its subscription ends.
			continue
		}
		data := msg.Data()
		if maxBytes > 0 && len(data) > maxBytes && len(page.Records) == 0 {
			return fetched, true, fmt.Errorf("%w: record payload of %d bytes exceeds %d", ErrInvalidSubjectReadLimit, len(data), maxBytes)
		}
		*largest = max(*largest, len(data), 1)
		if maxBytes > 0 && *bytesRead+len(data) > maxBytes {
			full = true
			continue
		}
		meta, err := msg.Metadata()
		if err != nil {
			return fetched, true, fmt.Errorf("message metadata: %w", err)
		}
		sequence := meta.Sequence.Stream
		page.LastSequence = sequence
		page.Records = append(page.Records, EncodedSubjectRecord{
			Subject:  msg.Subject(),
			Sequence: sequence,
			ID:       msg.Headers().Get(jetstream.MsgIDHeader),
			Data:     bytes.Clone(data),
		})
		*bytesRead += len(data)
	}
	if err := msgs.Error(); err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return fetched, true, err
	}
	return fetched, full, nil
}

func (l *EncodedEventLog) lastSubjectSeq(ctx context.Context, subject string) (uint64, error) {
	l.streamMu.RLock()
	defer l.streamMu.RUnlock()
	msg, err := l.stream.GetLastMsgForSubject(ctx, subject)
	if err == nil {
		return msg.Sequence, nil
	}
	if errors.Is(err, jetstream.ErrMsgNotFound) {
		return 0, nil
	}
	return 0, fmt.Errorf("last msg for subject %q: %w", subject, err)
}

func validateEncodedRecord(record EncodedRecord) error {
	if record.ID == "" {
		return fmt.Errorf("%w: record id is empty", ErrInvalidEncodedRecord)
	}
	if record.TTL < 0 {
		return fmt.Errorf("%w: record ttl is negative", ErrInvalidEncodedRecord)
	}
	return nil
}
