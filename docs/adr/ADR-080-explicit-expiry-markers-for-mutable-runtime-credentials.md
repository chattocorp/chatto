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
rotation. It also needs explicit security expiry even when cleanup is late or
all Chatto processes are stopped.

## Decision

Cookie-session records and renewable bearer-session authorities store an
explicit expiry in their JSON value. Validation checks that value. Physical
retention is not an authorization decision.

Each mutable record has a separate expiry marker:

- `expiry.session.{hmac}` has the fixed expiry of its cookie-session record.
- `expiry.renewable_session.{hmac}` has the current window expiry of its
  renewable-session record.

Chatto creates a marker with a JetStream per-message TTL. The marker value
contains the explicit expiry for diagnostics. The TTL includes one second of
cleanup delay so JetStream cannot delete an otherwise valid record just before
the process clock reaches the security deadline. The mutable credential record
uses the public KV create and update APIs. Its initial revision also carries
the security lifetime as a cleanup fallback. Later CAS updates do not try to
preserve that message TTL.

A cookie marker remains fixed because cookie renewal creates a new session
record and handle. A bearer refresh in the final quarter advances the renewable
session window. Chatto then deletes and recreates that session's marker through
the public KV API. It does not publish to the KV bucket's internal subject or
try to update a JetStream message TTL.

Each Chatto replica watches the marker keyspaces. When JetStream deletes a
marker, the replica reads the related mutable record. It deletes an explicitly
expired record or creates a marker for a live record. This check makes marker
replacement safe across replicas. On startup, each replica scans the two
mutable keyspaces. It deletes explicitly expired records and creates a missing
marker for each live record. Chatto creates the marker before the related
authority, so a process failure cannot leave a new authority without its
cleanup signal.

Cookie validation is read-only except for compatibility migration or invalid
record cleanup. A cookie session has the fixed `auth.token_ttl` lifetime that
starts when Chatto issues its handle. In the final quarter of that lifetime,
the HTTP edge creates a new session and browser handle before it revokes the
old record. This rotation gives active browsers a new fixed window without a
write on every request.

Renewable bearer authorities keep their current expiry during ordinary refresh
rotation and fresh-auth updates. A refresh in the final quarter advances the
window and replaces the marker. Their KV revision remains the OCC boundary.
Immutable bearer access-token verifier records still use a direct per-message
TTL because Chatto never updates them.

The bundled frontend uses the HttpOnly cookie for its origin server. It holds
the bearer returned by direct authentication in memory while it makes a
cookie-only viewer request. Successful cookie authentication discards that
bearer and removes any older stored origin bearer. If the cookie request is
unauthenticated, the client verifies and persists the returned bearer as an
origin fallback. A remote server still uses its renewable bearer session. The
frontend renews that session in the background and never starts OAuth without
a user action.

## Consequences

- Session validation no longer depends on a KV write.
- Authentication code no longer publishes directly to the internal KV
  subject to maintain mutable session TTLs.
- Security expiry remains correct when marker cleanup is delayed.
- Cookie-session and renewable-session storage use ordinary KV semantics and
  revision OCC.
- Initial records retain native TTL cleanup as defense in depth. A later
  mutation can remove that TTL, so the separate marker remains required.
- `RUNTIME_STATE` stores one small marker for each live mutable human session.
- Startup work scans the two bounded session keyspaces. It does not gate
  authorization because validation checks explicit expiry.
- A browser that is inactive for the complete cookie lifetime must sign in
  again. An active browser rotates in the final quarter and should not see an
  unexpected sign-out.
- An active bearer session advances its window during normal background
  refresh. An inactive session expires after one complete window.

## Alternatives considered

- **Publish mutable values with `Nats-TTL`:** This keeps one key per record but
  depends on the bucket's internal subject and JetStream header behavior.
- **Use one global bucket TTL:** Different runtime values need different
  lifetimes, and updates would still change their expiry semantics.
- **Create one KV bucket per lifetime:** This avoids per-message TTL but adds
  many durable resources and cannot express arbitrary configured lifetimes.
- **Run only a periodic expiry scan:** This is simple, but cleanup latency and
  scan load increase together. TTL markers provide the normal event and
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
