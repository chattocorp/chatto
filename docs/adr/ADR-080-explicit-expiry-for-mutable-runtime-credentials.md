# ADR-080: Explicit Expiry for Mutable Runtime Credentials

**Date:** 2026-08-24

**Status:** Accepted

**Partially supersedes:** [ADR-036](ADR-036-runtime-state-kv-boundary.md) and
[ADR-079](ADR-079-renewable-bearer-sessions.md) for cookie-session and
renewable-session expiry storage.

## Context

Chatto stores mutable cookie sessions and renewable bearer-session authorities
in JetStream KV. Each record needs two related controls:

- an absolute security expiry that validation can check; and
- physical cleanup after that expiry.

JetStream KV supports a per-key TTL when a process creates a key. Its `Update`
operation does not accept a new per-key TTL. A normal KV update therefore
replaces the current revision without the earlier revision's per-message TTL.

Chatto previously published mutable revisions to the KV stream subject with a
new `Nats-TTL` value. This operation is a documented JetStream publish with an
expected last subject sequence and a per-message TTL, but it bypasses the
higher-level KV update API. The old cookie validator used this operation on
every request with the complete configured lifetime. That behavior made the
session lifetime slide indefinitely and made a read depend on a storage write.

The replacement must keep active users signed in, reject expired credentials,
remove expired data, and serialize changes across replicas. It must not need a
process-local scheduler or periodic scan for authorization correctness.

## Decision

Each mutable session record stores an explicit `ExpiresAt` value. Validation
checks this value before it accepts the credential. JetStream retention is not
an authorization decision.

Chatto creates the record with `KeyTTL` set to its remaining explicit
lifetime. Cookie validation and bearer access-token validation do not update
the record or its TTL. Validation deletes a malformed or explicitly expired
record as best-effort cleanup.

When Chatto changes a mutable record, it publishes one new revision with:

- the documented KV stream subject for the `RUNTIME_STATE` key;
- the current KV revision as the expected last sequence for that subject; and
- a per-message TTL equal to `ExpiresAt - now`.

One core helper owns this low-level operation. Session call sites use a second
helper that accepts the absolute expiry and calculates the remaining TTL. The
helper rejects an expiry that is not in the future. No session call site can
set a new complete lifetime unless it also changes `ExpiresAt` as part of the
same revision.

The expected sequence is the optimistic-concurrency boundary. If two replicas
change the same record, one publish succeeds and the other reloads the current
revision or returns a conflict according to the operation. No process-local
lock is required.

A cookie session has one stable opaque handle. In the final quarter of the
current window, the core advances `ExpiresAt` on the same record and publishes
the revision with the remaining TTL. The expected KV revision serializes
concurrent renewal across replicas. Logout deletes the stable key without a
revision condition. This delete fences a renewal that races logout: either the
renewal publishes first and logout deletes it, or the delete makes the renewal
fail its expected-revision check.

After valid cookie authentication, the HTTP edge signs the same handle again
with the record's remaining lifetime. This operation does not change the KV
record outside the final-quarter renewal. It lets the next response repair a
browser cookie when an earlier `Set-Cookie` response was lost. A response that
sets an authentication cookie uses `Cache-Control: private, no-store`.
Content-hashed public frontend assets do not authenticate the cookie and do
not set authentication or CSRF cookies.

A cookie-authenticated realtime connection has a deadline at the start of the
final quarter. At that deadline, the server cancels authorized work and asks
the client to reconnect. The replacement WebSocket upgrade renews the stable
record and includes the signed cookie in the `101` response. This renewal does
not require user action.

A bearer refresh updates its stable renewable-session record. An ordinary
refresh keeps the current `ExpiresAt` value and publishes with the remaining
TTL. A refresh in the final quarter advances `ExpiresAt` to one complete
configured lifetime from that refresh and publishes with the new remaining
TTL. Fresh-auth changes use the current `ExpiresAt` value.

Immutable bearer access-token records use `KeyTTL` when Chatto creates them.
Chatto does not update those records.

The bundled frontend uses the secure HttpOnly cookie for its origin server. It
refreshes remote renewable bearer sessions in the background. Neither path
requires a user to extend a session manually.

## Consequences

- Session validation is read-only in the normal valid case.
- One key stores each cookie session or renewable bearer authority.
- Cookie renewal keeps the same opaque browser handle. Concurrent requests do
  not fork replacement handles, and logout fences concurrent renewal.
- JetStream removes the current mutable revision after its explicit lifetime.
- There is no expiry-marker keyspace, watcher, scheduler, startup scan, or
  cross-replica cleanup orchestration.
- Explicit expiry remains correct if physical cleanup is late.
- Every mutable revision keeps a per-message TTL. A rollback does not leave a
  current session revision without physical expiry.
- The low-level helper depends on JetStream's documented KV subject layout,
  expected-last-subject-sequence option, and per-message TTL option. Tests
  inspect the stored revision to verify that the TTL header is present.
- An inactive browser or bearer client must sign in after one complete session
  window. Normal activity renews the session automatically in the final
  quarter.
- Public immutable frontend assets remain safe for shared caching because they
  do not carry authentication cookies.

## Alternatives considered

- **Use a separate TTL marker for each mutable record:** This uses only the KV
  API, but it adds a second key, a watcher on every replica, marker-replacement
  races, and startup reconciliation. The extra machinery does not improve the
  security decision because validation must still check `ExpiresAt`.
- **Run a periodic expiry scan:** This avoids the low-level publish, but cleanup
  latency and scan load increase together. Every replica also needs ownership
  or duplicate-work rules.
- **Use one global bucket TTL:** Runtime-state records have different
  lifetimes. A bucket-wide TTL cannot represent them.
- **Create one bucket for each lifetime:** This adds durable resources and
  cannot represent arbitrary configured lifetimes.
- **Keep sliding validation writes:** This can keep a copied credential alive
  indefinitely and makes validation depend on a successful write.

## Related

- [ADR-017](ADR-017-cookie-session-auth-for-websocket.md)
- [ADR-025](ADR-025-multi-instance-client-architecture.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [ADR-079](ADR-079-renewable-bearer-sessions.md)
- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md)
