# ADR-046: Typed Runtime Credentials

**Date:** 2026-06-30

## Context

Chatto currently authenticates runtime requests through two persisted credential
models:

- Bearer auth token records under `RUNTIME_STATE` `session.{hmac}` keys.
- HTTP-only browser cookie session records under `RUNTIME_STATE`
  `cookie_session.{userId}.{hmac}` keys.

That split was practical when bearer tokens were added for cross-origin and
multi-server clients, but the SSO account-creation and account-linking work made
the cost visible. Fresh-auth state, auth-generation checks, revocation, session
refresh, OAuth authorization continuation, and audit metadata all need to reason
about "the credential that authenticated this request". Keeping cookie sessions
and bearer tokens as separate runtime models makes it easy for one path to drift
from the other.

The frontend's multi-server model still depends on bearer credentials. A single
Chatto client can register multiple servers, store one opaque credential per
server, and send that credential to the selected server's ConnectRPC and realtime
endpoints. Cookies cannot replace that because browser cookies are origin-scoped
and are not a reliable transport for remote registered servers.

At the same time, same-origin browser sessions still benefit from HTTP-only,
SameSite cookies. Cookies reduce localStorage exposure for the server that is
serving the app and let OAuth/external-provider browser redirects resume through
ordinary browser navigation.

## Decision

Chatto will converge on one persisted runtime credential model with explicit
credential types. The stored runtime credential is the source of truth; bearer
headers and browser cookies are presentation mechanisms for that credential.

The credential types are:

- `first_party_session`: a user session issued by Chatto's own password,
  registration, bootstrap, or external-provider login flows. These credentials
  may be presented either as an opaque bearer token or through a same-origin
  HTTP-only cookie carrying an opaque credential handle.
- `oauth_access_token`: a delegated access token issued by Chatto's OAuth
  authorization-code exchange for a trusted client origin. These credentials may
  authenticate normal API and realtime requests, but they are not first-party
  sessions and cannot satisfy or acquire fresh-auth status.

Fresh-auth metadata, auth generation, source, request metadata, expiry, sliding
TTL behavior, validation, and revocation eligibility belong to the typed runtime
credential record. HTTP edge code may still extract credentials differently from
`Authorization` headers and signed browser cookies, but both presentations must
normalize to the same validated runtime-credential result before user context,
logout, audit, realtime subscription, CSRF binding, or session-termination
behavior is applied. Request context carries the presentation kind plus the
single opaque handle; it does not duplicate bearer-token and cookie-session
fields. Fresh credential checks must explicitly require a first-party runtime
credential. OAuth access tokens remain useful for multi-server clients, but they
must not authorize account-security operations such as adding a password or
linking/disconnecting sign-in methods.

The multi-server frontend keeps its per-server bearer-token registry. Each
registered server still has its own opaque bearer credential, scoped by the
client to that server ID/base URL. Same-origin cookie auth remains an optimization
and compatibility transport for the origin server only. The app must not rely on
cookies for remote registered servers.

Migration is phased:

1. Write explicit credential types on newly issued bearer-token records.
2. Update fresh-auth and runtime-credential helpers to reason from the typed
   credential, not from ad-hoc source-string checks.
3. Write browser cookie sessions as first-party `session.{hmac}` runtime
   credentials with `presentation = "cookie"` while continuing to validate
   legacy `cookie_session.*` records.
4. Store only the opaque runtime credential handle in newly written signed
   browser sessions. Keep dual-read support for legacy signed sessions that
   contain `user_id` plus `cookie_session_id`.
5. Keep cookie rotation, revocation, logout audit, live session termination, and
   auth-context injection on the shared credential path once the presentation
   channel has been checked.
6. Keep legacy record validation and cleanup until existing TTLs expire or a
   documented pre-1.0 compatibility cutoff removes them. The
   `cookie_session.*` keyspace is deprecated compatibility-only storage and must
   not receive new writes.

## Consequences

Fresh-auth and account-security code gets a single security invariant:
freshness is a property of first-party runtime credentials only. Delegated OAuth
access tokens can authenticate ordinary API calls without becoming equivalent to
the user's own browser session.

Runtime credential revocation becomes easier to reason about because password
changes, password resets, external-identity disconnects, and account deletion can
target one credential model instead of coordinating separate cookie-session and
bearer-token stores.

The OAuth/external-provider browser flow gets a cleaner continuation story:
creating or resuming a first-party session can use the same runtime credential
record whether the browser presents it via cookie or the SPA receives it as a
bearer credential.

The migration has compatibility cost. Deprecated `cookie_session.*` and untyped
`session.*` records must remain readable during the rollout, and user-wide
cleanup must scan both old and new keys until those records have expired or been
explicitly retired.

The multi-server frontend continues to carry bearer tokens in browser storage for
remote servers, so XSS prevention remains part of the client auth boundary. This
ADR simplifies server-side credential semantics; it does not remove the bearer
transport required by ADR-025.
