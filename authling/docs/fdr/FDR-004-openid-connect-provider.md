# FDR-004: OpenID Connect Provider

**Status:** Experimental
**Last reviewed:** 2026-08-21

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
  signed-in account and client and explains that the stable account identifier
  will be shared.
- Allowing creates or renews a durable exact-client authorization grant and
  binds the request to the current account. Later requests covered by that
  grant skip the consent screen unless they use `prompt=consent`. Denying
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

Special-use destinations are rejected by default. Operators may explicitly
trust exact CIMD hostnames that resolve to private or loopback addresses in
controlled development environments. Private-host and loopback-host trust are
separate exceptions, and each admits only its named address class. Neither
permits link-local, multicast, or other special-use destinations.

## Security and Failure Behavior

- An issuer mismatch or signing-key mismatch prevents readiness.
- Duplicate security-sensitive authorization parameters, missing or weak
  PKCE, unsupported scopes and response modes, request objects, and
  `prompt=none` fail closed.
- Consent and login POSTs require Authling's exact browser origin. Pending IDs
  are resolved server-side and cannot carry an arbitrary return URL.
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
  grouping, key rotation, and official
  conformance-suite automation are not implemented.
- CIMD remains an Internet-Draft. Authling implements the reviewed draft-02
  profile and may need an explicit migration as the document evolves.

## Related

- **ADR:** [ADR-004](../adr/ADR-004-cimd-native-openid-provider.md)
- **Product boundary:** [ADR-007](../adr/ADR-007-limit-authling-to-identity-provider.md)
- **Features:** [FDR-003](FDR-003-local-login-and-browser-sessions.md)
- **Authorization grants:** [FDR-010](FDR-010-oidc-authorization-grants.md)
- **Profiles:** [FDR-011](FDR-011-account-profile.md)
