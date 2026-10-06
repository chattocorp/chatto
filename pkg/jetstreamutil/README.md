# JetStream Utilities

`hmans.de/chatto/pkg/jetstreamutil` provides small, application-neutral helpers
for NATS JetStream clients:

- key-value reads through the stream leader, so that a read sees every
  committed write;
- retried provisioning of streams, key-value buckets, and object stores; and
- consistent classification of optimistic-concurrency conflicts.

Applications own resource names, configurations, identity metadata, timeout
policy, and the decisions that they make from key-value state. The module
depends only on `nats.go`.

## Provision JetStream resources

`CreateJetStreamResourceWithRetry` runs a repeatable resource callback with a
bounded `JetStreamResourceRetryPolicy`. Applications choose `MaxAttempts`
(including the first call) and `RetryDelay`. After failed attempt N, the helper
waits for N times `RetryDelay`. The helper rejects invalid policy values.

The helper retries request deadline errors only while the parent context is
active. It also retries the specific JetStream store-creation and stream-name
conflict errors used during concurrent provisioning. Parent cancellation or
expiry stops the operation and takes precedence over the callback result.
Other errors and retry exhaustion remain fatal to the caller.

Callbacks must honor the context and be safe to repeat when the server has
already committed an operation but its response was lost. Use this helper for
streams, KV buckets, and Object Stores, not event writes. Applications retain
resource names, configuration, identity metadata, and timeout policy. With
nats.go, the default request timeout applies only when the supplied context
has no deadline; an expired parent deadline cannot be retried.

## Read key-value buckets consistently

JetStream serves `jetstream.KeyValue.Get` through DirectGet when a bucket
allows it, and buckets that nats.go creates allow it. Any replica can then
answer. A follower that lags behind a committed write returns an older
revision or no entry. A read that must see a preceding write, or that decides
an OCC update, a claim, or a revocation, then works from stale state.

`KeyValue` wraps a bucket handle and keeps its complete
`jetstream.KeyValue` interface. It supports only the default JetStream API:
`NewKeyValue` rejects a context with a domain or a different API prefix. `Get` and `GetRevision` read through the
stream leader and keep the semantics of the bucket's own methods. `Latest`
returns the newest entry for a key or wildcard filter, including delete,
purge, and expiry markers. `UpdateWithTTL` replaces a revision and sets a new
per-message TTL, which the bucket API supports only on `Create`.

`GetAnyReplica` is the bucket's own read, for hot paths. Any replica can
answer, so the result can be an older revision, or a miss for an entry that
the replica has not applied yet. Accept a result from `GetAnyReplica`, but
decide a negative result again with `Get`: a missing entry, or an entry that
causes a rejection. Skip that second read only where a wrong negative result
is harmless. Use `GetAnyReplica` only where an older revision that the caller
accepts cannot cause a wrong decision.

```go
bucket, err := js.CreateOrUpdateKeyValue(ctx, config)
if err != nil {
	return err
}
kv, err := jetstreamutil.NewKeyValue(js, bucket)
if err != nil {
	return err
}

entry, err := kv.Get(ctx, key) // sees every committed write

cached, err := kv.GetAnyReplica(ctx, key) // hot path; can be an older revision
if errors.Is(err, jetstream.ErrKeyNotFound) {
	cached, err = kv.Get(ctx, key) // decide the miss through the leader
}
```

Bind every bucket handle that the application reads through `NewKeyValue`, so
that code that receives a `jetstream.KeyValue` also gets the leader-routed
`Get`. Watchers, key listings, and history remain the bucket's own. The
bucket's `Create` also reads a delete marker through a direct get. It can thus
return `jetstream.ErrKeyExists` for a key that a lagging replica still shows.

## Classify OCC conflicts

`IsSequenceConflict` reports whether an error is a JetStream
optimistic-concurrency conflict: an expected-last-sequence mismatch from a
stream publish, or `jetstream.ErrKeyExists` or
`jetstream.ErrKeyRevisionMismatch` from a key-value write. JetStream uses a
different error code for streams with more than one replica; the function
recognizes both.

## Status

This module is an incubation surface shared by Chatto and Authling. Its API is
not yet covered by a stability promise, and releases remain pre-1.0.

## Development

Run the module tests independently:

```sh
mise test-jetstreamutil
```

From this directory, the equivalent standalone check is:

```sh
GOWORK=off go test ./...
```

## License

The module is licensed under [`Apache-2.0`](LICENSE). Its permissive license
does not imply API stability.
