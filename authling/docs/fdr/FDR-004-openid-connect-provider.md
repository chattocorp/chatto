# FDR-004: OpenID Connect Provider

**Status:** Experimental
**Last reviewed:** 2026-09-15

## Overview

Authling acts as an OpenID Provider for conventional configured clients and
automatically discovered CIMD public clients. A person authenticates with
their Authling browser session, explicitly authorizes one request, and returns
to the relying party with an Authorization Code.

## Behavior

- Discovery is available at `/.well-known/openid-configuration`; public keys
  are published at the advertised JWKS endpoint.
- Authling advertises and accepts only Authorization Code. Every request
  requires exactly `openid` and S256 PKCE, including confidential clients.
- Redirect URI matching is exact. Authorization errors are sent to a client
  only after that client and redirect have been validated.
- A signed-out person is sent through local login and then resumes the pending
  consent decision. When consent is required, the screen identifies the
  signed-in account and client and lists the account ID, preferred username,
  and optional full name that it can share. It explains that access includes
  future profile changes.
- Allowing creates or renews a durable exact-client authorization grant and
  binds the request to the current account. Later requests covered by that
  grant skip the consent screen only when its disclosure version is current
  and the request does not use `prompt=consent`. Denying
  returns `access_denied` and the original state to the validated redirect URI.
- The authorization code expires with its ten-minute request, is bound to the
  client, redirect, and PKCE verifier, and succeeds in at most one concurrent
  exchange.
- Successful exchange returns a five-minute RS256 ID token and opaque bearer
  access token. The issuer is Authling's immutable public URL, `sub` is the
  Authling account ID, and local accounts also receive their non-empty durable
  `preferred_username` and `name` identity hints. UserInfo returns the same
  claims. Access-token state also binds the client and granted scopes.
- Protocol state and token records are encrypted at rest and stored under
  non-reversible runtime keys. Raw codes and tokens are not durable keys and
  are never logged.
- Browser-capable discovery, JWKS, token, and UserInfo endpoints allow
  credential-free CORS. Authorization and consent do not.

## Conventional Clients

An operator declares conventional clients with `[[oidc.clients]]`. An empty
secret creates a public client using token endpoint authentication method
`none`; a secret of at least 32 characters creates a `client_secret_basic`
client. Both still require PKCE.

## CIMD Clients

An unconfigured HTTPS URL client ID is resolved as a Client ID Metadata
Document. It must describe that exact client ID, public token authentication,
one or more safe redirect URIs, and no flow outside Authorization Code. Fetches
are HTTPS-only, do not follow redirects, reject special-use destinations,
ignore proxy configuration, and have strict concurrency, response-size,
timeout, and cache bounds. Invalid responses are never cached.

Each resolver permits eight active cache-miss lookups. Admission happens before
DNS validation and does not queue: a saturated resolver rejects another cache
miss immediately. Valid cache hits still work. One five-second deadline covers
DNS validation, the HTTP request, and body reading; an earlier caller deadline
or cancellation takes precedence. The dial-time destination check retains the
lookup context even when the HTTP transport detaches its dial context.

The cache holds at most 256 clients. Each lookup and insertion removes expired
entries. At capacity, insertion evicts the entry that expires first. Cache
lifetimes retain the existing one-minute default and five-minute maximum;
`no-store` and `no-cache` responses are not retained. Idle expired entries can
remain allocated until the next lookup, within the same entry cap. Each parsed
document is limited to 5 KiB. Returned clients do not expose mutable cache data.

These limits are per process and use no durable state or background worker.
A later request can retry after a slot is released; failures are not cached.
Ingress rate limits remain a separate control for aggregate request traffic.

Special-use destinations are rejected by default. Operators may explicitly
trust exact CIMD hostnames that resolve to private or loopback addresses in
controlled development environments. Private-host and loopback-host trust are
separate exceptions, and each admits only its named address class. Neither
permits link-local, multicast, or other special-use destinations.

## Request Admission

Before client lookup, a syntactically valid authorization request consumes one
of 1,000 shared admissions. Each admission restarts a ten-minute quiet window;
the exhausted counter expires ten minutes after the last admission. Rejected
requests do not extend it. This bounds new pending state without evicting
sessions or recovery records. It also bounds admissions for configured and
cached CIMD clients. The counter uses OCC in `AUTHLING_RUNTIME_STATE` and survives
process restart. Failed lookup or state creation does not refund admission.

Exhaustion returns HTTP 429 with `Retry-After: 600`; unavailable or malformed
admission state returns HTTP 503. Neither response logs request metadata or
redirects to an unvalidated client. Ingress limits remain necessary for total
HTTP traffic. Admission does not apply to consent, code exchange, or UserInfo,
so existing flows can finish while new requests are limited.

Browser preflight at the token endpoint returns CORS headers before POST
validation. Actual token requests still require the supported form encoding,
parameters, and PKCE proof.

## Authentication Freshness

Authorization accepts `prompt=login`, `prompt=consent`, and their combination.
`prompt=login` and `max_age=0` require a successful authentication ceremony
that starts after the authorization request was created. A positive `max_age`
sets the maximum elapsed seconds since authentication. Invalid, duplicate,
negative, and overflowing values are rejected.

`prompt=none` permits no interactive page. An active session and a current
covering grant can authorize silently. An absent or stale session returns
`login_required`; a missing or revoked grant returns `consent_required` to the
validated redirect URI with the original state. Combining `none` with another
prompt is invalid. The encrypted request stores this constraint across restart.

Both automatic grant reuse and explicit approval check freshness. If the
session is too old, the browser returns to login with the same pending request.
A failed login does not change authentication time. The checks use full timestamp
precision so a session from earlier in the same second cannot satisfy forced
login. Positive age limits are checked again when consent is submitted.

ID tokens include `auth_time`, in Unix seconds, from the authenticated browser
session. Consent, profile updates, and session activity do not advance this
value. Email-change session replacement preserves it. Successful login, signup,
password reset, and signed-in password change record the start of their
successful credential ceremony as a conservative authentication time.

Authentication time and request constraints are encrypted runtime state and
survive a restart. They do not add durable domain events. This implements the
freshness parameters in [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html#AuthRequest).

## Security and Failure Behavior

- An issuer mismatch or signing-key mismatch prevents readiness.
- Duplicate security-sensitive authorization parameters, missing or weak
  PKCE, unsupported scopes and response modes, and request objects fail closed.
- Consent and login POSTs require Authling's exact browser origin. Pending IDs
  are resolved server-side and cannot carry an arbitrary return URL. Login and
  recovery forms permit only the validated client redirect origin in addition
  to Authling in their Content Security Policy `form-action` directive, so
  automatic consent reuse can complete the browser redirect chain.
- A storage conflict during approval or code claim fails the operation instead
  of creating two grants.
- Grant creation and revocation use account-subject OCC and wait for the grant
  projection. Revocation controls future consent reuse, not already issued
  short-lived tokens or relying-party sessions.
- Failure responses do not reveal client secrets, codes, tokens, account IDs,
  email addresses, or complete request URLs.

## Limitations

- Only local password authentication and the `pwd` authentication-method
  reference exist.
- Refresh tokens, token revocation, RP-initiated logout, further identity
  scopes and claims beyond `preferred_username` and `name`, relying-party
  grouping, and official conformance-suite automation are not implemented.
- CIMD remains an Internet-Draft. Authling implements the reviewed draft-02
  profile and may need an explicit migration as the document evolves.

## Related

- **ADR:** [ADR-004](../adr/ADR-004-cimd-native-openid-provider.md)
- **Product boundary:** [ADR-007](../adr/ADR-007-limit-authling-to-identity-provider.md)
- **Features:** [FDR-003](FDR-003-local-login-and-browser-sessions.md)
- **Authorization grants:** [FDR-010](FDR-010-oidc-authorization-grants.md)
- **Profiles:** [FDR-011](FDR-011-account-profile.md)
- **Signing-key rotation:** [FDR-012](FDR-012-automatic-oidc-signing-key-rotation.md)

## Upgrade Behavior

These changes add no durable event variants. Historical pending requests omit
`silent` and retain their interactive behavior. New runtime counters expire on
their own; no data migration is required. Update all replicas before relying on
the shared limits or silent-request behavior: older replicas do not enforce
these controls.
