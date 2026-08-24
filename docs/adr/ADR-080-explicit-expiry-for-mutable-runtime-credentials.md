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

A cookie session has a fixed window. In the final quarter of that window, the
HTTP edge creates a new session and cookie before it revokes the old session.
This rotation gives an active browser a new fixed window without a write on
each request.

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

- [ADR-017](ADR-017-cookie-sessions-for-websocket-auth.md)
- [ADR-025](ADR-025-multi-instance-client-architecture.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [ADR-079](ADR-079-renewable-bearer-sessions.md)
- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md)
