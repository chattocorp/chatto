package events

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	validKeyValueKey    = regexp.MustCompile(`^[-/_=\.a-zA-Z0-9]+$`)
	validKeyValueFilter = regexp.MustCompile(`^[-/_=\.a-zA-Z0-9*]*[>]?$`)
)

// KeyValue is a jetstream.KeyValue whose Get, GetRevision, and Latest observe
// every committed write.
//
// JetStream serves KeyValue.Get through DirectGet when the bucket allows it,
// which is the default for buckets that nats.go creates. Any replica can then
// answer, and a follower can lag behind a write that already committed: it
// returns an older revision or no entry. Read-your-writes, OCC updates, claims,
// and revocations then decide from stale state.
//
// KeyValue routes Get and GetRevision through the stream leader. Every other
// method is the bound bucket's, including watchers, key listings, and history.
// The bucket's Create also checks for a delete marker through DirectGet, so it
// can report jetstream.ErrKeyExists for a key that a lagging replica still
// shows. GetAnyReplica is the opt-in fast read for hot paths.
//
// The bucket must be a plain bucket on the default JetStream API: not a
// mirror, not in another domain, and not reached through a subject transform
// or API prefix.
type KeyValue struct {
	jetstream.KeyValue
	js jetstream.JetStream
	// leader sends STREAM.MSG.GET management requests, which the stream
	// leader answers. The newer jetstream API chooses DirectGet from the
	// stream configuration and has no option to avoid it.
	leader  nats.JetStreamContext
	stream  string
	subject string
}

// NewKeyValue binds leader-routed reads to bucket. It rejects a JetStream
// context with a domain or a non-default API prefix.
func NewKeyValue(js jetstream.JetStream, bucket jetstream.KeyValue) (*KeyValue, error) {
	if js == nil || bucket == nil {
		return nil, errors.New("key-value bucket and JetStream context are required")
	}
	opts := js.Options()
	if opts.Domain != "" || (opts.APIPrefix != "" && strings.TrimSuffix(opts.APIPrefix, ".")+"." != jetstream.DefaultAPIPrefix) {
		return nil, errors.New("key-value reads support only the default JetStream API")
	}
	leader, err := js.Conn().JetStream(nats.MaxWait(opts.DefaultTimeout))
	if err != nil {
		return nil, fmt.Errorf("bind leader reads for bucket %q: %w", bucket.Bucket(), err)
	}
	return &KeyValue{
		KeyValue: bucket,
		js:       js,
		leader:   leader,
		stream:   "KV_" + bucket.Bucket(),
		subject:  "$KV." + bucket.Bucket() + ".",
	}, nil
}

// Get returns the latest committed entry for key through the stream leader.
// Like jetstream.KeyValue.Get, it returns jetstream.ErrKeyNotFound for a
// missing, deleted, purged, or expired key.
func (kv *KeyValue) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if !keyValueKeyValid(key, validKeyValueKey) {
		return nil, jetstream.ErrInvalidKey
	}
	return putEntry(kv.Latest(ctx, key))
}

// GetRevision returns the entry for key at revision through the stream leader.
// It returns jetstream.ErrKeyNotFound when that revision belongs to another
// key or removes this one. Revision 0 means the latest revision, as in nats.go.
func (kv *KeyValue) GetRevision(ctx context.Context, key string, revision uint64) (jetstream.KeyValueEntry, error) {
	if revision == 0 {
		return kv.Get(ctx, key)
	}
	if !keyValueKeyValid(key, validKeyValueKey) {
		return nil, jetstream.ErrInvalidKey
	}
	message, err := kv.leader.GetMsg(kv.stream, revision, nats.Context(ctx))
	if err != nil {
		return nil, keyValueReadError(err)
	}
	if message.Subject != kv.subject+key {
		return nil, jetstream.ErrKeyNotFound
	}
	return putEntry(kv.entry(message), nil)
}

// Latest returns the newest entry whose key matches filter, read through the
// stream leader. filter can contain the wildcards `*` and `>`. Unlike Get,
// Latest also returns a delete, purge, or expiry marker; its Operation is
// jetstream.KeyValueDelete or jetstream.KeyValuePurge. It returns
// jetstream.ErrKeyNotFound only when no message matches.
func (kv *KeyValue) Latest(ctx context.Context, filter string) (jetstream.KeyValueEntry, error) {
	if !keyValueKeyValid(filter, validKeyValueFilter) {
		return nil, jetstream.ErrInvalidKey
	}
	message, err := kv.leader.GetLastMsg(kv.stream, kv.subject+filter, nats.Context(ctx))
	if err != nil {
		return nil, keyValueReadError(err)
	}
	return kv.entry(message), nil
}

// GetAnyReplica is the bucket's own Get. When the bucket allows direct gets,
// any replica can answer, so the result can be an older revision, or a miss
// for an entry that a lagging replica has not applied yet. Use it on hot paths
// to accept a result quickly, and decide a negative result again with Get.
func (kv *KeyValue) GetAnyReplica(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	return kv.KeyValue.Get(ctx, key)
}

// UpdateWithTTL replaces key if its latest revision is revision, and sets a
// new per-message TTL. jetstream.KeyValue supports a TTL on Create only. The
// bucket must allow per-message TTLs. It returns the new revision, or an error
// that matches jetstream.ErrKeyRevisionMismatch on a revision conflict.
func (kv *KeyValue) UpdateWithTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error) {
	if !keyValueKeyValid(key, validKeyValueKey) {
		return 0, jetstream.ErrInvalidKey
	}
	if ttl <= 0 {
		return 0, errors.New("key-value TTL must be positive")
	}
	message := nats.NewMsg(kv.subject + key)
	message.Data = value
	ack, err := kv.js.PublishMsg(ctx, message,
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(ttl),
	)
	if err != nil {
		// Report a revision conflict as jetstream.KeyValue.Update does.
		if isWrongLastSequence(err) {
			return 0, fmt.Errorf("%w: %w", err, jetstream.ErrKeyRevisionMismatch)
		}
		return 0, err
	}
	return ack.Sequence, nil
}

func (kv *KeyValue) entry(message *nats.RawStreamMsg) keyValueEntry {
	return keyValueEntry{
		bucket:    kv.Bucket(),
		key:       strings.TrimPrefix(message.Subject, kv.subject),
		value:     message.Data,
		revision:  message.Sequence,
		created:   message.Time,
		operation: keyValueOperation(message.Header),
	}
}

// putEntry turns a removal marker into jetstream.ErrKeyNotFound, as
// jetstream.KeyValue.Get does.
func putEntry(entry jetstream.KeyValueEntry, err error) (jetstream.KeyValueEntry, error) {
	if err != nil {
		return nil, err
	}
	if entry.Operation() != jetstream.KeyValuePut {
		return nil, jetstream.ErrKeyNotFound
	}
	return entry, nil
}

// keyValueOperation decodes a KV operation header or a server marker the same
// way nats.go does.
func keyValueOperation(header nats.Header) jetstream.KeyValueOp {
	switch header.Get("KV-Operation") {
	case "DEL":
		return jetstream.KeyValueDelete
	case "PURGE":
		return jetstream.KeyValuePurge
	}
	switch header.Get(jetstream.MarkerReasonHeader) {
	case "MaxAge", "Purge":
		return jetstream.KeyValuePurge
	case "Remove":
		return jetstream.KeyValueDelete
	}
	return jetstream.KeyValuePut
}

// keyValueReadError maps legacy JetStream errors to the errors that
// jetstream.KeyValue returns.
func keyValueReadError(err error) error {
	switch {
	case errors.Is(err, nats.ErrMsgNotFound):
		return jetstream.ErrKeyNotFound
	case errors.Is(err, nats.ErrStreamNotFound):
		return fmt.Errorf("%w: %w", jetstream.ErrBucketNotFound, err)
	}
	return err
}

func keyValueKeyValid(key string, pattern *regexp.Regexp) bool {
	if key == "" || key[0] == '.' || key[len(key)-1] == '.' || strings.Contains(key, "..") {
		return false
	}
	return pattern.MatchString(key)
}

type keyValueEntry struct {
	bucket    string
	key       string
	value     []byte
	revision  uint64
	created   time.Time
	operation jetstream.KeyValueOp
}

func (e keyValueEntry) Bucket() string                  { return e.bucket }
func (e keyValueEntry) Key() string                     { return e.key }
func (e keyValueEntry) Value() []byte                   { return e.value }
func (e keyValueEntry) Revision() uint64                { return e.revision }
func (e keyValueEntry) Created() time.Time              { return e.created }
func (e keyValueEntry) Delta() uint64                   { return 0 }
func (e keyValueEntry) Operation() jetstream.KeyValueOp { return e.operation }
