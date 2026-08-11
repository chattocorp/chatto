package events

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand"
	"strconv"
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

// ErrMissingOCC is returned when an atomic batch contains no optimistic
// concurrency guard. Every batch needs at least one guard so there is no
// accidental publish-without-OCC path through the event log.
var ErrMissingOCC = errors.New("missing optimistic concurrency guard")

// ErrInvalidBatchOCC is returned when an atomic batch places a stream-tail
// guard where JetStream cannot evaluate it. Stream-tail OCC belongs on the
// first batch entry because it fences the committed stream state that precedes
// the complete batch.
var ErrInvalidBatchOCC = errors.New("invalid optimistic concurrency guard placement")

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
}

// EncodedSubjectRecord preserves a durable subject alongside its opaque
// payload. Subject is caller-owned metadata and must be safe to log.
type EncodedSubjectRecord struct {
	Subject  string
	Sequence uint64
	ID       string
	Data     []byte
}

// EncodedBatchEntry is one record in an atomic publish batch. Each entry may
// carry per-subject or wildcard-filter OCC. The first entry may instead (or
// additionally) carry whole-stream OCC. At least one entry in a batch must
// carry an OCC guard.
//
// JetStream evaluates every entry against committed state at batch acceptance.
// It does not advance an entry's expected sequence for earlier members of the
// same batch, so callers must avoid dependent same-subject OCC entries.
type EncodedBatchEntry struct {
	Subject           string
	Record            EncodedRecord
	ExpectedSeq       uint64
	FilterSubject     string
	HasOCC            bool
	ExpectedStreamSeq uint64
	HasStreamOCC      bool
}

// EncodedEventLog owns opaque-byte JetStream reads and OCC-only writes.
// Application adapters remain responsible for event validation, encoding, and
// subject policy.
type EncodedEventLog struct {
	js     jetstream.JetStream
	stream jetstream.Stream
	logger Logger
}

// NewEncodedEventLog binds opaque event-log mechanics to one JetStream stream.
func NewEncodedEventLog(js jetstream.JetStream, stream jetstream.Stream, logger Logger) *EncodedEventLog {
	return &EncodedEventLog{js: js, stream: stream, logger: normalizeLogger(logger)}
}

// StreamUsage returns the current message and byte totals for the bound stream.
func (l *EncodedEventLog) StreamUsage(ctx context.Context) (messages, bytes uint64, err error) {
	info, err := l.stream.Info(ctx)
	if err != nil {
		return 0, 0, err
	}
	return info.State.Msgs, info.State.Bytes, nil
}

// LastStreamSeq returns the current last sequence of the bound stream. Unlike
// the message count, this remains a valid OCC token when messages have been
// deleted or expired.
func (l *EncodedEventLog) LastStreamSeq(ctx context.Context) (uint64, error) {
	info, err := l.stream.Info(ctx)
	if err != nil {
		return 0, err
	}
	return info.State.LastSeq, nil
}

const maxAppendRetries = 5

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
	return l.publishAt(ctx, subject, record, expectedSeq, "")
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
		seq, err := l.publishAt(ctx, subject, record, expectedSeq, "")
		if err == nil {
			return seq, nil
		}
		if !errors.Is(err, ErrConflict) {
			return 0, err
		}

		if l.logger != nil {
			l.logger.Debug("OCC conflict, retrying",
				"subject", subject,
				"expected_seq", expectedSeq,
				"attempt", attempt,
				"max_attempts", maxAppendRetries)
		}
		lastErr = err

		baseDelay := time.Duration(1<<(attempt-1)) * time.Millisecond
		jitter := time.Duration(mrand.Int63n(int64(5 * time.Millisecond)))
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(baseDelay + jitter):
		}
	}
	return 0, fmt.Errorf("append after %d attempts: %w", maxAppendRetries, lastErr)
}

// AppendAt publishes a record with a caller-supplied expected last sequence
// for subject.
func (l *EncodedEventLog) AppendAt(
	ctx context.Context,
	subject string,
	record EncodedRecord,
	expectedSeq uint64,
) (uint64, error) {
	if err := validateEncodedRecord(record); err != nil {
		return 0, err
	}
	return l.publishAt(ctx, subject, record, expectedSeq, "")
}

// AppendAtFilter publishes a record to subject with OCC against the current
// tail of a possibly wildcarded filter.
func (l *EncodedEventLog) AppendAtFilter(
	ctx context.Context,
	subject string,
	record EncodedRecord,
	filter string,
	expectedFilterSeq uint64,
) (uint64, error) {
	if err := validateEncodedRecord(record); err != nil {
		return 0, err
	}
	return l.publishAt(ctx, subject, record, expectedFilterSeq, filter)
}

func (l *EncodedEventLog) publishAt(
	ctx context.Context,
	subject string,
	record EncodedRecord,
	expectedSeq uint64,
	filter string,
) (uint64, error) {
	var opt jetstream.PublishOpt
	if filter == "" {
		opt = jetstream.WithExpectLastSequencePerSubject(expectedSeq)
	} else {
		opt = jetstream.WithExpectLastSequenceForSubject(expectedSeq, filter)
	}
	ack, err := l.js.Publish(ctx, subject, record.Data, opt, jetstream.WithMsgID(record.ID))
	if err == nil {
		return ack.Sequence, nil
	}

	target := subject
	if filter != "" {
		target = "filter " + filter
	}
	if conflictErr := sequenceConflictError(err, target, expectedSeq); conflictErr != nil {
		return 0, conflictErr
	}
	return 0, fmt.Errorf("publish: %w", err)
}

func (l *EncodedEventLog) publishAtStreamTail(
	ctx context.Context,
	subject string,
	record EncodedRecord,
	expectedStreamSeq uint64,
) (uint64, error) {
	if err := validateEncodedRecord(record); err != nil {
		return 0, err
	}
	ack, err := l.js.Publish(
		ctx,
		subject,
		record.Data,
		jetstream.WithExpectLastSequence(expectedStreamSeq),
		jetstream.WithMsgID(record.ID),
	)
	if err == nil {
		return ack.Sequence, nil
	}
	if conflictErr := sequenceConflictError(err, "stream", expectedStreamSeq); conflictErr != nil {
		return 0, conflictErr
	}
	return 0, fmt.Errorf("publish: %w", err)
}

// AppendBatch atomically publishes encoded records. Either all records land
// adjacently in stream order or none do.
func (l *EncodedEventLog) AppendBatch(ctx context.Context, entries []EncodedBatchEntry) ([]uint64, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	hasOCC := false
	for i, entry := range entries {
		if err := validateEncodedRecord(entry.Record); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
		if entry.HasStreamOCC && i != 0 {
			return nil, fmt.Errorf("batch entry %d: %w: stream-tail guard must be on first entry", i, ErrInvalidBatchOCC)
		}
		hasOCC = hasOCC || entry.HasOCC || entry.HasStreamOCC
	}
	if !hasOCC {
		return nil, ErrMissingOCC
	}

	batchID, err := newBatchID()
	if err != nil {
		return nil, fmt.Errorf("generate batch id: %w", err)
	}
	for i, entry := range entries[:len(entries)-1] {
		if _, err := l.publishBatchEntry(ctx, entry, conflictExpectationForEntry(entry), batchID, uint64(i+1), false); err != nil {
			return nil, fmt.Errorf("batch entry %d: %w", i, err)
		}
	}

	commitEntry := entries[len(entries)-1]
	commitSeq, err := l.publishBatchEntry(ctx, commitEntry, batchConflictExpectation(entries), batchID, uint64(len(entries)), true)
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
	conflict conflictExpectation,
	batchID string,
	batchSeq uint64,
	commit bool,
) (uint64, error) {
	msg := buildEncodedBatchMsg(entry, batchID, batchSeq, commit)
	resp, err := l.js.Conn().RequestMsgWithContext(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("publish: %w", err)
	}
	return decodeBatchAckWithExpectation(resp, conflict)
}

type conflictExpectation struct {
	target      string
	expectedSeq uint64
	exact       bool
}

func conflictExpectationForEntry(entry EncodedBatchEntry) conflictExpectation {
	if entry.HasOCC && entry.HasStreamOCC {
		return conflictExpectation{target: "batch entry OCC guards"}
	}
	if entry.HasStreamOCC {
		return conflictExpectation{target: "stream", expectedSeq: entry.ExpectedStreamSeq, exact: true}
	}
	if entry.FilterSubject != "" {
		return conflictExpectation{target: "filter " + entry.FilterSubject, expectedSeq: entry.ExpectedSeq, exact: true}
	}
	return conflictExpectation{target: entry.Subject, expectedSeq: entry.ExpectedSeq, exact: true}
}

func batchConflictExpectation(entries []EncodedBatchEntry) conflictExpectation {
	var (
		expectation conflictExpectation
		guards      int
	)
	for _, entry := range entries {
		if entry.HasOCC {
			guards++
			expectation = conflictExpectationForEntry(entry)
		}
		if entry.HasStreamOCC {
			guards++
			expectation = conflictExpectationForEntry(entry)
		}
	}
	if guards == 1 {
		return expectation
	}
	return conflictExpectation{target: "atomic batch OCC guards"}
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

func decodeBatchAck(resp *nats.Msg, entry EncodedBatchEntry) (uint64, error) {
	return decodeBatchAckWithExpectation(resp, conflictExpectationForEntry(entry))
}

func decodeBatchAckWithExpectation(resp *nats.Msg, conflict conflictExpectation) (uint64, error) {
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
		if isSequenceConflict(apiErr) {
			if conflict.exact {
				return 0, fmt.Errorf("%s at expected seq %d: %w", conflict.target, conflict.expectedSeq, ErrConflict)
			}
			return 0, fmt.Errorf("%s: %w", conflict.target, ErrConflict)
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
	if entry.HasOCC {
		hdr.Set("Nats-Expected-Last-Subject-Sequence", strconv.FormatUint(entry.ExpectedSeq, 10))
		if entry.FilterSubject != "" {
			hdr.Set("Nats-Expected-Last-Subject-Sequence-Subject", entry.FilterSubject)
		}
	}
	if entry.HasStreamOCC {
		hdr.Set(jetstream.ExpectedLastSeqHeader, strconv.FormatUint(entry.ExpectedStreamSeq, 10))
	}
	hdr.Set(jetstream.MsgIDHeader, entry.Record.ID)
	return &nats.Msg{Subject: entry.Subject, Header: hdr, Data: entry.Record.Data}
}

func sequenceConflictError(err error, target string, expectedSeq uint64) error {
	if !isSequenceConflict(err) {
		return nil
	}
	return fmt.Errorf("%s at expected seq %d: %w", target, expectedSeq, ErrConflict)
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
// also bounded by maxBytes. More indicates that the page's point-in-time
// result had additional records. Callers can pass LastSequence to the next
// request without exposing JetStream consumer coordinates.
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
	consumer, err := l.stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    []string{subject},
		DeliverPolicy:     deliverPolicy,
		OptStartSeq:       startSeq,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return SubjectRecordPage{}, err
	}
	defer l.stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return SubjectRecordPage{}, err
	}
	target := min(uint64(maxRecords), info.NumPending)
	page := SubjectRecordPage{Records: make([]EncodedSubjectRecord, 0, target)}
	var bytesRead int
	for uint64(len(page.Records)) < target {
		batchSize := int(target - uint64(len(page.Records)))
		if maxBytes > 0 {
			// Fetch one message at a time so a message that would exceed the
			// byte window causes an explicit error instead of being silently
			// omitted from a page.
			batchSize = 1
		}
		msgs, err := consumer.Fetch(batchSize, jetstream.FetchMaxWait(10*time.Second))
		if err != nil {
			if errors.Is(err, jetstream.ErrNoMessages) {
				break
			}
			return SubjectRecordPage{}, err
		}

		fetched := 0
		for msg := range msgs.Messages() {
			fetched++
			data := msg.Data()
			if maxBytes > 0 && (len(data) > maxBytes || bytesRead+len(data) > maxBytes) {
				return SubjectRecordPage{}, fmt.Errorf("%w: page payload exceeds %d bytes", ErrInvalidSubjectReadLimit, maxBytes)
			}
			meta, err := msg.Metadata()
			if err != nil {
				return SubjectRecordPage{}, fmt.Errorf("message metadata: %w", err)
			}
			sequence := meta.Sequence.Stream
			page.LastSequence = sequence
			page.Records = append(page.Records, EncodedSubjectRecord{
				Subject:  msg.Subject(),
				Sequence: sequence,
				ID:       msg.Headers().Get(jetstream.MsgIDHeader),
				Data:     bytes.Clone(data),
			})
			bytesRead += len(data)
		}
		if fetched == 0 {
			break
		}
	}
	page.More = info.NumPending > uint64(len(page.Records))
	return page, nil
}

// SubjectRecordsAfter returns all opaque records matching subject with stream
// sequence greater than afterSeq, plus the last matching sequence. New callers
// that need a bounded allocation should use SubjectRecordsAfterPage instead.
func (l *EncodedEventLog) SubjectRecordsAfter(
	ctx context.Context,
	subject string,
	afterSeq uint64,
) ([]EncodedSubjectRecord, uint64, error) {
	var records []EncodedSubjectRecord
	var lastSeq uint64
	for {
		page, err := l.SubjectRecordsAfterPage(ctx, subject, afterSeq, 500, 0)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, page.Records...)
		if page.LastSequence > 0 {
			lastSeq = page.LastSequence
		}
		if !page.More || len(page.Records) == 0 {
			break
		}
		afterSeq = page.LastSequence
	}
	return records, lastSeq, nil
}

func (l *EncodedEventLog) lastSubjectSeq(ctx context.Context, subject string) (uint64, error) {
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
	return nil
}
