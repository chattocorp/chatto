# ADR-079: Renewable Bearer Sessions with Rotating Refresh Credentials

**Date:** 2026-08-22

**Status:** Accepted

**Partially supersedes:** [ADR-024](ADR-024-opaque-bearer-tokens-for-cross-origin-auth.md) and [ADR-036](ADR-036-runtime-state-kv-boundary.md) for human bearer-credential lifetime and renewal. Their opaque-token and runtime-state decisions remain current.

## Context

Chatto's human bearer credentials originally used a sliding `RUNTIME_STATE`
TTL. Every successful API request renewed the same opaque credential, so an
active credential had no maximum lifetime. A copied bearer token could
therefore remain useful indefinitely if either its legitimate owner or an
attacker kept using it.

Giving that token a fixed expiry without a replacement path would turn normal
session maintenance into an interactive authentication event. In particular,
the bundled multi-server client would have to reopen the remote server's OAuth
authorization flow, take the user away from their current work, and repeat
that interruption independently for every connected server. Non-browser
clients would have no equivalent automatic path.

The replacement must work with multiple Chatto replicas, survive a lost token
response, limit replay, preserve the frontend's current route and realtime
projection, and avoid storing raw credentials. Cookie sessions and bot API
keys have different presentation and lifecycle models and do not need this
renewal protocol.

## Decision

Human bearer authentication uses a **renewable session** consisting of:

- a short-lived opaque access token for ConnectRPC, HTTP, and realtime traffic;
- a rotating opaque refresh credential normally used at `/oauth/token` and
  also presented by the bundled client at `/auth/logout` for stable-session
  revocation; and
- one stable latest-value renewable-session record in `RUNTIME_STATE`.

This applies to first-party bearer sessions issued by password, registration,
and explicit external-identity account-creation flows, and to delegated bearer
sessions issued by OAuth Authorization Code with PKCE. OAuth sessions remain
bound to their validated client ID. Bot API keys remain non-expiring durable
bot credentials, and same-origin cookie sessions retain their existing
rotation and sliding runtime-record behavior.

The default access-token lifetime is 15 minutes and is configurable with
`auth.access_token_ttl` / `CHATTO_AUTH_ACCESS_TOKEN_TTL`. The renewable session
has a non-renewable maximum lifetime of 90 days, configured by the existing
`auth.token_ttl` / `CHATTO_AUTH_TOKEN_TTL`. An access token issued near the end
of a session is clamped to the remaining session lifetime.

### Runtime-state representation

`RUNTIME_STATE` stores two related records:

- `renewable_session.{hmac}` is the stable authority for one renewable
  session. Its JSON value contains the user, optional OAuth client ID,
  credential kind and source, safe audit request metadata, creation and
  absolute expiry times, user auth generation, current rotation generation,
  previous refresh request ID and rotation time, and authoritative fresh-auth
  metadata.
  Its per-key TTL is always the remaining absolute lifetime and is never
  extended.
- `session.{hmac}` is one short-lived access-token verifier record. It includes
  its fixed expiry, renewable-session ID, access generation, user auth
  generation, and the established typed-credential metadata. Fresh-auth fields
  are copied at issuance, but validation resolves their current values from the
  stable session so re-verification remains correct across concurrent rotation.
  Validation does not renew the access record's TTL.

The stable session ID is opaque and HMAC-keyed before it enters
`RUNTIME_STATE`. Access tokens and refresh credentials are deterministic,
purpose-separated HMAC credentials derived from that ID and the rotation
generation. The raw values are returned to the client but are never stored.
As with Chatto's other runtime credentials, restoring `RUNTIME_STATE` preserves
sessions only when `[core].secret_key` is also preserved.

Every access-token validation checks both its own fixed expiry and the stable
renewable-session authority. Deleting that authority rejects every access
generation on its next API request or realtime connection, even if individual
verifier records remain until TTL cleanup. A realtime socket that already
completed authentication retains its authorized context only until that access
token's fixed expiry, unless a separate logout or account-lifecycle signal ends
it sooner. Password and account lifecycle changes still use the durable user
auth generation as their primary revocation fence. Blocking an OAuth client
also invalidates the stable sessions issued to that client.

### Rotation, concurrency, and replay

Refresh uses the public `/oauth/token` endpoint with
`grant_type=refresh_token`. The request includes the refresh credential, the
OAuth `client_id` when the session is delegated, and a client-generated
`refresh_request_id`. The endpoint accepts the existing JSON and standard form
encodings, returns the rotated access and refresh credentials with both
remaining lifetimes, and marks responses non-cacheable.

The client must persist the request ID before sending the request and keep it
until a valid response has been persisted. The server rotates as follows:

1. Authenticate the HMAC refresh credential, stable session, client binding,
   absolute expiry, OAuth-client policy, and user auth generation.
2. Increment the generation and record the request ID and rotation time by
   updating the stable key with its exact JetStream KV revision and remaining
   absolute TTL.
3. Create the deterministic access-token verifier for the committed generation.
4. Return the deterministic credential pair.

The KV revision is the cross-replica serialization boundary. A conflicting
replica reloads the stable record and evaluates the newly committed state; no
process-local lock is required for correctness.

If the response is lost after the stable update, presenting the immediately
previous refresh generation with the same request ID during the new access
token's useful lifetime recreates or returns the exact same deterministic
credential pair. This also recovers a process failure between committing the
stable rotation and creating its access record.

Any other presentation of an older generation is refresh-credential reuse.
The server durably records bearer revocation before deleting the stable
renewable-session key, invalidating the winner and every outstanding access
token for that session. Two concurrent refresh attempts with different
request IDs therefore produce at most one temporary winner and then revoke the
session when the loser observes reuse. Clients must serialize refreshes; the
bundled frontend coalesces same-tab work and uses a per-server Web Lock so tabs
on the same browser profile adopt one persisted rotation. Server-side OCC and
reuse detection remain authoritative for clients without Web Locks.

Rotating the refresh credential does not immediately revoke the previous
short-lived access token. Its fixed expiry bounds that overlap. Reuse detection,
explicit logout, user auth-generation changes, and OAuth-client blocking revoke
the stable authority, rejecting all overlap at its next validation. Established
realtime sockets retain the same fixed-expiry bound described above.

### Client behavior

The bundled frontend persists the access token, refresh credential, both
expiry instants, OAuth client ID when applicable, and any in-flight refresh
request ID in an independently keyed device-local per-server authentication
record. Combined catalogue compatibility writes merge those authoritative
fields instead of copying one tab's in-memory credential snapshot.

It refreshes shortly before access expiry, stops scheduling once access expiry
reaches the renewable session's absolute expiry, and also retries one unary
ConnectRPC request after an `Unauthenticated` response when forced renewal
succeeds. Transient network and server failures keep the credentials and
request ID for retry. An `invalid_grant` response is permanent: the frontend
marks only that server as requiring authentication, keeps the user's current
route and other connected servers intact, and exposes the existing explicit
reconnect action. It never starts OAuth automatically.

Realtime sockets are authenticated for the lifetime of the presented access
token. At expiry the server cancels authorized work, sends a reconnecting
`authentication_required` close when possible, and closes the socket. The
frontend rotates once, reconnects the same per-server event bus with the new
access token, and supplies its in-memory opaque resume cursor and retained-room
set. The projection and route are not recreated.

Logout presents the refresh credential when available so the server revokes
the whole renewable session, rather than only one access-token verifier.
Programmatic clients and integrations use the same refresh grant and must
implement request-ID persistence and refresh serialization.

### Compatibility and deployment

This is an intentional 0.5 authentication boundary. Bearer records created by
pre-renewal servers do not identify a renewable session and are rejected;
affected users sign in once to obtain a credential pair. There is no legacy
bearer fallback and no capability-negotiated dual mode.

Upgrade all serving replicas and bundled clients together. An older replica
would treat a new short-lived access record as a sliding bearer record and
could extend it beyond its intended expiry. Rolling back to a pre-renewal
binary after issuing new credentials has the same problem; revoke the affected
bearer sessions or require users to sign in again before such a rollback.
Current typed cookie sessions are independent and are not invalidated by this
bearer-record migration.

The additive fields on `chatto.auth.v1.CreateExternalIdentityAccountResponse`
remain wire-compatible, but clients participating in human bearer auth must
consume the complete pair and expiry values. Generated clients and public
reference documentation ship with the server change. The pull request is
labelled `api-breaking-change`, and the 0.5 release notes call out the one-time
sign-in and coordinated-upgrade requirement.

## Consequences

- A copied access token is useful for at most its short fixed lifetime unless
  the attacker also obtains the rotating refresh credential.
- Active users renew without navigation, OAuth popups, or losing their current
  server route and realtime projection.
- Refresh reuse turns suspicious concurrency into whole-session revocation for
  subsequent requests and reconnects. Benign lost responses remain recoverable
  only through the exact persisted request ID; an established realtime socket
  cannot outlive the access token that authenticated it.
- Correctness across replicas comes from one JetStream KV revision boundary;
  browser locks improve contention but are not a security primitive.
- `RUNTIME_STATE` gains one stable record per renewable bearer session plus
  short-lived verifier records for recent access generations. Raw credentials
  remain absent from storage, backups, logs, and audit events.
- The frontend stores a longer-lived refresh credential in device-local
  browser storage. XSS on that client origin can therefore steal the complete
  renewable session, not only one short-lived access token. Avoiding that risk
  would require a different trusted-client or cookie boundary that cannot
  serve Chatto's cross-origin multi-server frontend.
- Cookie sessions and bot API keys keep their distinct lifecycle models; the
  refresh grant is not a universal credential protocol.
- Pre-renewal bearer sessions are deliberately not migrated, simplifying the
  runtime state and avoiding an indefinitely supported insecure fallback.

## Alternatives considered

- **Keep sliding opaque bearer tokens:** simplest operationally, but leaves no
  upper bound on a stolen active token.
- **Give bearer tokens an absolute expiry and reopen OAuth:** limits theft but
  interrupts normal work, repeats per server, and gives non-browser clients no
  pragmatic renewal path.
- **Use self-contained JWT access and refresh tokens:** still requires mutable
  server state for single-use rotation, reuse detection, OAuth-client policy,
  and immediate user revocation, while adding key-rotation and clock concerns.
- **Store random raw refresh tokens:** makes exact rotation straightforward but
  turns `RUNTIME_STATE` and its backups into a store of directly redeemable
  credentials.
- **Accept the previous refresh token for a grace period without a request
  ID:** hides lost responses but also makes intentional replay
  indistinguishable from recovery and weakens reuse detection.

## Related

- [ADR-024](ADR-024-opaque-bearer-tokens-for-cross-origin-auth.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [ADR-051](ADR-051-server-scoped-resumable-client-projection.md)
- [ADR-071](ADR-071-cimd-identified-open-oauth-clients.md)
- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md)
