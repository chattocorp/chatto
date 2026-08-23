# ADR-080: Explicit Expiry Markers for Mutable Runtime Credentials

**Date:** 2026-08-23

**Status:** Accepted

**Partially supersedes:** [ADR-036](ADR-036-runtime-state-kv-boundary.md) and
[ADR-079](ADR-079-renewable-bearer-sessions.md) for cookie-session and
renewable-session expiry storage.

## Context

JetStream applies a per-message TTL when a process publishes a KV revision.
The KV update API does not extend or preserve that TTL. Chatto worked around
this rule by publishing mutable KV revisions to the bucket's internal subject
with a new `Nats-TTL` header.

That workaround coupled authentication code to JetStream's KV stream layout.
Cookie validation also wrote a new revision on every request to keep a sliding
TTL. A validation read could therefore fail because an unrelated storage write
failed. The high write rate also made session behavior harder to reason about.

Chatto needs mutable session authorities for fresh-auth changes and bearer
rotation. It also needs fixed security expiry even when cleanup is late or all
Chatto processes are stopped.

## Decision

Cookie-session records and renewable bearer-session authorities store an
explicit absolute expiry in their JSON value. Validation checks that value.
Physical retention is not an authorization decision.

Each mutable record has a separate immutable expiry marker:

- `expiry.session.{hmac}` expires with its cookie-session record.
- `expiry.renewable_session.{hmac}` expires with its renewable-session record.

Chatto creates a marker once with a JetStream per-message TTL. It does not
update the marker. The marker value contains the same absolute expiry for
diagnostics. The mutable credential record uses the public KV create and update
APIs. Its initial revision can carry the same TTL as a compatibility cleanup
fallback. Later CAS updates do not try to preserve, extend, or replace that
message TTL; the immutable marker remains intact.

Each Chatto replica watches the marker keyspaces. When JetStream expires a
marker, the replica deletes the related mutable record. Deletes are
idempotent, so more than one replica can process the same marker. On startup,
each replica scans the two mutable keyspaces. It deletes explicitly expired
records and creates a missing marker for each live record. This reconciliation
repairs a process stop, a deployment gap, or a record that an older binary
created.

Cookie validation is read-only except for compatibility migration or invalid
record cleanup. A cookie session has the fixed `auth.token_ttl` lifetime that
starts when Chatto issues its handle. In the final quarter of that lifetime,
the HTTP edge creates a new session and browser handle before it revokes the
old record. This rotation gives active browsers a new fixed window without a
write on every request.

Renewable bearer authorities keep their original absolute expiry during every
refresh rotation and fresh-auth update. Their KV revision remains the OCC
boundary. Immutable bearer access-token verifier records still use a direct
per-message TTL because Chatto never updates them.

The bundled frontend uses the HttpOnly cookie for its origin server. During
migration, it tries the cookie before a stored origin bearer credential. It
removes that bearer credential only after cookie authentication succeeds. A
remote server still uses its renewable bearer session. The frontend warns the
user seven days before a renewable bearer session reaches its fixed maximum,
but it never starts OAuth without a user action.

## Consequences

- Session validation no longer depends on a KV write.
- Authentication code no longer publishes directly to the internal KV
  subject to maintain mutable session TTLs.
- Security expiry remains correct when marker cleanup is delayed.
- Cookie-session and renewable-session storage use ordinary KV semantics and
  revision OCC.
- Initial records retain native TTL cleanup for version skew. A later mutation
  can remove that TTL, so the separate marker remains required.
- `RUNTIME_STATE` stores one small marker for each live mutable human session.
- Startup work scans the two bounded session keyspaces. It does not gate
  authorization because validation checks explicit expiry.
- A browser that is inactive for the complete cookie lifetime must sign in
  again. An active browser rotates in the final quarter and should not see an
  unexpected sign-out.
- A bearer session still has a fixed maximum lifetime. The warning makes that
  deliberate reconnect visible before service stops.

## Alternatives considered

- **Publish mutable values with `Nats-TTL`:** This keeps one key per record but
  depends on the bucket's internal subject and JetStream header behavior.
- **Use one global bucket TTL:** Different runtime values need different
  lifetimes, and updates would still change their expiry semantics.
- **Create one KV bucket per lifetime:** This avoids per-message TTL but adds
  many durable resources and cannot express arbitrary configured lifetimes.
- **Run only a periodic expiry scan:** This is simple, but cleanup latency and
  scan load increase together. Immutable markers provide the normal event and
  the startup scan repairs missed work.
- **Keep sliding cookies:** This writes on every authenticated request and
  makes a successful validation depend on storage mutation.

## Related

- [ADR-017](ADR-017-cookie-sessions-for-websocket-auth.md)
- [ADR-025](ADR-025-multi-instance-client-architecture.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [ADR-079](ADR-079-renewable-bearer-sessions.md)
- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md)
