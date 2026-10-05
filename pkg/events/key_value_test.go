package events_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	. "hmans.de/chatto/pkg/events"
)

// laggingBucket answers Get as a DirectGet served by a follower that has not
// applied later writes: a key in stale returns its older entry, or
// jetstream.ErrKeyNotFound when that entry is nil.
type laggingBucket struct {
	jetstream.KeyValue
	stale map[string]jetstream.KeyValueEntry
}

func (b laggingBucket) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if entry, ok := b.stale[key]; ok {
		if entry == nil {
			return nil, jetstream.ErrKeyNotFound
		}
		return entry, nil
	}
	return b.KeyValue.Get(ctx, key)
}

func setupTestKeyValue(t *testing.T) (jetstream.JetStream, jetstream.KeyValue) {
	t.Helper()
	js, err := jetstream.New(startTestNATS(t))
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	bucket, err := js.CreateKeyValue(testContext(t), jetstream.KeyValueConfig{
		Bucket:         "KV_TEST",
		History:        1,
		LimitMarkerTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("create test bucket: %v", err)
	}
	return js, bucket
}

func newTestKeyValue(t *testing.T, js jetstream.JetStream, bucket jetstream.KeyValue) *KeyValue {
	t.Helper()
	kv, err := NewKeyValue(js, bucket)
	if err != nil {
		t.Fatalf("NewKeyValue: %v", err)
	}
	return kv
}

func TestKeyValueReadsIgnoreLaggingReplica(t *testing.T) {
	ctx := testContext(t)
	js, bucket := setupTestKeyValue(t)
	if _, err := bucket.Put(ctx, "updated", []byte("old")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	stale, err := bucket.Get(ctx, "updated")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	revision, err := bucket.Put(ctx, "updated", []byte("new"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := bucket.Put(ctx, "created", []byte("fresh")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	kv := newTestKeyValue(t, js, laggingBucket{
		KeyValue: bucket,
		stale:    map[string]jetstream.KeyValueEntry{"updated": stale, "created": nil},
	})

	entry, err := kv.Get(ctx, "updated")
	if err != nil || string(entry.Value()) != "new" || entry.Revision() != revision {
		t.Fatalf("Get = %v, %v; want the committed revision %d", entry, err, revision)
	}
	if entry.Key() != "updated" || entry.Bucket() != "KV_TEST" || entry.Operation() != jetstream.KeyValuePut {
		t.Fatalf("Get entry = key %q, bucket %q, operation %v", entry.Key(), entry.Bucket(), entry.Operation())
	}
	if entry, err := kv.GetRevision(ctx, "updated", revision); err != nil || string(entry.Value()) != "new" {
		t.Fatalf("GetRevision = %v, %v", entry, err)
	}

	if entry, err := kv.Get(ctx, "created"); err != nil || string(entry.Value()) != "fresh" {
		t.Fatalf("Get of an entry that the replica misses = %v, %v", entry, err)
	}

	// GetAnyReplica is the replica's answer; callers decide negatives with Get.
	if entry, err := kv.GetAnyReplica(ctx, "updated"); err != nil || string(entry.Value()) != "old" {
		t.Fatalf("GetAnyReplica on an older revision = %v, %v", entry, err)
	}
	if _, err := kv.GetAnyReplica(ctx, "created"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("GetAnyReplica on a lagging miss = %v, want ErrKeyNotFound", err)
	}
}

func TestKeyValueGetUsesLeaderReadsOnTheWire(t *testing.T) {
	ctx := testContext(t)
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "KV_WIRE_TEST"})
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	status, err := bucket.Status(ctx)
	if err != nil {
		t.Fatalf("bucket status: %v", err)
	}
	if !status.(*jetstream.KeyValueBucketStatus).StreamInfo().Config.AllowDirect {
		t.Fatal("test bucket does not allow direct gets")
	}
	if _, err := bucket.Put(ctx, "key", []byte("value")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	kv := newTestKeyValue(t, js, bucket)

	leaderReads := make(chan *nats.Msg, 8)
	directReads := make(chan *nats.Msg, 8)
	for subject, reads := range map[string]chan *nats.Msg{
		"$JS.API.STREAM.MSG.GET.KV_KV_WIRE_TEST": leaderReads,
		"$JS.API.DIRECT.GET.KV_KV_WIRE_TEST.>":   directReads,
		"$JS.API.DIRECT.GET.KV_KV_WIRE_TEST":     directReads,
	} {
		subscription, err := connection.ChanSubscribe(subject, reads)
		if err != nil {
			t.Fatalf("subscribe to %s: %v", subject, err)
		}
		t.Cleanup(func() { _ = subscription.Unsubscribe() })
	}
	if err := connection.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	expectRead := func(name string, want, other chan *nats.Msg) {
		t.Helper()
		select {
		case <-want:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s sent no request on the expected subject", name)
		}
		select {
		case message := <-other:
			t.Fatalf("%s also sent a request to %s", name, message.Subject)
		case <-time.After(50 * time.Millisecond):
		}
	}
	if _, err := kv.Get(ctx, "key"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	expectRead("Get", leaderReads, directReads)
	if _, err := kv.GetAnyReplica(ctx, "key"); err != nil {
		t.Fatalf("GetAnyReplica: %v", err)
	}
	expectRead("GetAnyReplica", directReads, leaderReads)
}

func TestKeyValueGetTreatsRemovalsAsMissing(t *testing.T) {
	ctx := testContext(t)
	js, bucket := setupTestKeyValue(t)
	kv := newTestKeyValue(t, js, bucket)
	for _, key := range []string{"deleted", "purged", "other"} {
		if _, err := bucket.Put(ctx, key, []byte(key)); err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}
	if err := bucket.Delete(ctx, "deleted"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := bucket.Purge(ctx, "purged"); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	for _, key := range []string{"deleted", "purged", "missing"} {
		if _, err := kv.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) {
			t.Fatalf("Get %s = %v, want ErrKeyNotFound", key, err)
		}
	}
	other, err := kv.Get(ctx, "other")
	if err != nil {
		t.Fatalf("Get other: %v", err)
	}
	if _, err := kv.GetRevision(ctx, "deleted", other.Revision()); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("GetRevision of another key's revision = %v, want ErrKeyNotFound", err)
	}
	if latest, err := kv.GetRevision(ctx, "other", 0); err != nil || latest.Revision() != other.Revision() {
		t.Fatalf("GetRevision 0 = %v, %v; want the latest revision", latest, err)
	}
	if _, err := kv.Get(ctx, "wild.*"); !errors.Is(err, jetstream.ErrInvalidKey) {
		t.Fatalf("Get with a wildcard = %v, want ErrInvalidKey", err)
	}

	if err := js.DeleteKeyValue(ctx, bucket.Bucket()); err != nil {
		t.Fatalf("DeleteKeyValue: %v", err)
	}
	if _, err := kv.Get(ctx, "other"); !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("Get from a deleted bucket = %v, want ErrBucketNotFound", err)
	}
}

func TestKeyValueLatestReturnsMarkersAndMatchesFilters(t *testing.T) {
	ctx := testContext(t)
	js, bucket := setupTestKeyValue(t)
	kv := newTestKeyValue(t, js, bucket)
	if _, err := bucket.Put(ctx, "presence.a", []byte("a")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := bucket.Put(ctx, "presence.b", []byte("b")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := bucket.Delete(ctx, "presence.a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	latest, err := kv.Latest(ctx, "presence.>")
	if err != nil {
		t.Fatalf("Latest wildcard: %v", err)
	}
	if latest.Key() != "presence.a" || latest.Operation() != jetstream.KeyValueDelete {
		t.Fatalf("Latest wildcard = key %q, operation %v; want the delete marker", latest.Key(), latest.Operation())
	}
	if entry, err := kv.Latest(ctx, "presence.b"); err != nil || entry.Operation() != jetstream.KeyValuePut {
		t.Fatalf("Latest key = %v, %v", entry, err)
	}
	if _, err := kv.Latest(ctx, "absent.>"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("Latest without a match = %v, want ErrKeyNotFound", err)
	}
}

func TestKeyValueUpdateWithTTLChecksRevision(t *testing.T) {
	ctx := testContext(t)
	js, err := jetstream.New(startTestNATS(t))
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "KV_TTL_TEST",
		LimitMarkerTTL: time.Minute,
	})
	if err != nil {
		t.Fatalf("create TTL bucket: %v", err)
	}
	kv := newTestKeyValue(t, js, bucket)
	created, err := kv.Create(ctx, "record", []byte("one"), jetstream.KeyTTL(time.Hour))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := kv.UpdateWithTTL(ctx, "record", []byte("two"), created, time.Hour)
	if err != nil {
		t.Fatalf("UpdateWithTTL: %v", err)
	}
	entry, err := kv.Get(ctx, "record")
	if err != nil || string(entry.Value()) != "two" || entry.Revision() != updated {
		t.Fatalf("Get after UpdateWithTTL = %v, %v", entry, err)
	}
	stream, err := js.Stream(ctx, "KV_KV_TTL_TEST")
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	stored, err := stream.GetMsg(ctx, updated)
	if err != nil {
		t.Fatalf("GetMsg: %v", err)
	}
	if ttl := stored.Header.Get(jetstream.MsgTTLHeader); ttl != time.Hour.String() {
		t.Fatalf("stored TTL header = %q, want %q", ttl, time.Hour.String())
	}
	_, err = kv.UpdateWithTTL(ctx, "record", []byte("stale"), created, time.Hour)
	if !errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
		t.Fatalf("UpdateWithTTL with a stale revision = %v, want ErrKeyRevisionMismatch", err)
	}
	if _, err := kv.UpdateWithTTL(ctx, "record", []byte("x"), updated, 0); err == nil {
		t.Fatal("UpdateWithTTL without a TTL succeeded")
	}
}

func TestKeyValueGetTreatsExpiryMarkersAsMissing(t *testing.T) {
	ctx := testContext(t)
	js, bucket := setupTestKeyValue(t)
	kv := newTestKeyValue(t, js, bucket)
	if _, err := bucket.Create(ctx, "expiring", []byte("soon"), jetstream.KeyTTL(time.Second)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		latest, err := kv.Latest(ctx, "expiring")
		if err == nil && latest.Operation() == jetstream.KeyValuePurge {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expiry marker did not appear: %v, %v", latest, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := kv.Get(ctx, "expiring"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("Get after expiry = %v, want ErrKeyNotFound", err)
	}
}

func TestNewKeyValueAcceptsOnlyTheDefaultAPI(t *testing.T) {
	ctx := testContext(t)
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	bucket, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "KV_API_TEST"})
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	defaultAPI, err := jetstream.NewWithAPIPrefix(connection, "$JS.API")
	if err != nil {
		t.Fatalf("create default-prefix context: %v", err)
	}
	if _, err := NewKeyValue(defaultAPI, bucket); err != nil {
		t.Fatalf("NewKeyValue with the default prefix: %v", err)
	}
	domain, err := jetstream.NewWithDomain(connection, "hub")
	if err != nil {
		t.Fatalf("create domain context: %v", err)
	}
	if _, err := NewKeyValue(domain, bucket); err == nil {
		t.Fatal("NewKeyValue accepted a JetStream domain")
	}
	prefixed, err := jetstream.NewWithAPIPrefix(connection, "tenant.API")
	if err != nil {
		t.Fatalf("create prefixed context: %v", err)
	}
	if _, err := NewKeyValue(prefixed, bucket); err == nil {
		t.Fatal("NewKeyValue accepted a non-default API prefix")
	}
}
