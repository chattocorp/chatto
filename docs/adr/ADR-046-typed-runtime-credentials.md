# ADR-046: Typed Runtime Credentials

**Date:** 2026-06-30

**Updated:** 2026-08-26

**Status:** Partially superseded

**Partially superseded by:** [ADR-079](ADR-079-renewable-bearer-sessions.md)
for bearer renewal and
[ADR-081](ADR-081-explicit-expiry-for-mutable-runtime-credentials.md)
for cookie-session expiry storage and origin-client preference.

## Context

Chatto historically authenticated runtime requests through two persisted
credential models:

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

Fresh-auth metadata, auth generation, source, request metadata, explicit
expiry, validation, and revocation eligibility belong to the typed runtime
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

The multi-server frontend keeps bearer credentials for remote servers. Each
remote server has its own opaque credential, scoped by the client to that
server ID and base URL. The origin server uses same-origin cookie auth. The app
must not rely on cookies for remote registered servers.

The migration completed at the 0.5 compatibility boundary:

1. Write explicit credential types on newly issued bearer-token records.
2. Update fresh-auth and runtime-credential helpers to reason from the typed
   credential, not from ad-hoc source-string checks.
3. Write browser cookie sessions as first-party `session.{hmac}` runtime
   credentials with `presentation = "cookie"`.
4. Store only the opaque runtime credential handle in SCS-managed
   `chatto_auth_<slot>` cookies. Retain the signed and optionally encrypted
   `chatto_session` cookie only for short-lived browser-flow state. Retired
   signed-session fields such as `user_id` and `cookie_session_id` are never
   accepted as authentication inputs. During 0.5 only, a dedicated same-origin
   migration route can read the immediately previous typed
   `runtime_credential_id` field and move its existing handle to SCS.
5. Keep cookie renewal, revocation, logout audit, live session termination, and
   auth-context injection on the shared credential path once the presentation
   channel has been checked.
6. Stop reading, refreshing, revoking, or scanning legacy `cookie_session.*`
   records in 0.5. Any remaining records are inert and disappear through their
   existing TTL. The 0.5 bridge does not read this older storage shape.

## Consequences

Fresh-auth and account-security code gets a single security invariant:
freshness is a property of first-party runtime credentials only. Delegated OAuth
access tokens can authenticate ordinary API calls without becoming equivalent to
the user's own browser session.

Runtime credential revocation becomes easier to reason about because password
changes, password resets, external-identity disconnects, and account deletion can
target one credential model instead of coordinating separate cookie-session and
bearer-token stores.

The OAuth and external-provider browser flow gets a cleaner continuation
story. Creating or resuming a first-party session uses the same typed runtime
credential model. The origin browser presents a cookie credential, while a
programmatic client can use the bearer presentation.

The 0.5 cutoff keeps active typed browser sessions from 0.4 through a one-time
automatic migration. Browsers that carry the older `cookie_session.*` shape
must sign in again. After migration, every active
browser session follows typed validation and explicit revocation rules.
Cookie records use explicit expiry plus stable-handle renewal, while bearer
access records use fixed expiry and a stable `renewable_session.*` authority.
User-wide cleanup covers both
`session.*` records and renewable-session authorities.

The multi-server frontend continues to carry bearer tokens in browser storage for
remote servers, so XSS prevention remains part of the client auth boundary. This
ADR simplifies server-side credential semantics; it does not remove the bearer
transport required by ADR-025.
