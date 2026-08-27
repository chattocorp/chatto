# ADR-004: Provide OpenID Connect with CIMD-Native Client Discovery

**Status:** Accepted

**Date:** 2026-08-01

## Context

Authling exists to provide one stable identity to independently operated
applications. Ordinary OpenID Connect relying parties expect an issuer,
discovery metadata, configured client credentials, exact redirect URI checks,
and standard Authorization Code exchange. Chatto deployments also need to
start a relationship with a person's Authling without every server first
being registered by that Authling's operator.

OAuth Dynamic Client Registration would add a mutable registration API,
registration credentials, ownership, and garbage-collection policy. Client ID
Metadata Documents (CIMD) instead let a public client use the HTTPS URL of its
metadata document as its client identifier. The current specification is an
Internet-Draft and can still change.

OIDC state contains redirects, state and nonce values, account identifiers,
authorization codes, and bearer-token authority. It therefore needs stronger
storage and network-fetch boundaries than ordinary application data.

## Decision

One Authling deployment is one immutable OIDC issuer. The canonical
`http.public_url` becomes the issuer when storage is first initialized; a later
configuration mismatch fails startup. The Authling account ID is the public
OIDC `sub`.

The provider profile supports discovery, JWKS, Authorization Code, ID-token
issuance, bearer access tokens, and UserInfo. It requires `openid`, `code` as
the response type, and S256 PKCE for every client. ADR-007 limits the provider
to identity scopes; the initial profile accepts exactly `openid`.
Authorization codes are short-lived, single-use through JetStream optimistic
concurrency, and bound to the exact client and redirect URI. The first slice
does not support refresh tokens, implicit or hybrid flow, request objects,
dynamic registration, or additional identity claims.

Authling supports two client sources behind the same protocol boundary:

1. Operators may declare conventional public or `client_secret_basic` clients
   in `authling.toml`. Redirect URIs are exact values. HTTPS is required except
   for loopback HTTP in loopback development.
2. A public client may use an HTTPS Client ID Metadata Document URL as its
   client ID. Authling fetches that document without redirects, proxies, or
   remote image retrieval; bounds time, size, concurrency, and cache lifetime;
   rejects special-use network destinations before fetching and again when
   dialing; requires the document's `client_id` to exactly equal its URL; and
   accepts only the initial public Authorization Code profile.

Special-use CIMD destinations remain denied by default. An operator may
explicitly trust exact hostnames for controlled development environments.
Private-host and loopback-host trust are separate capabilities, and each
permits only its named address class. Link-local, multicast, and all other
special-use destinations stay blocked.

The implementation tracks
[draft-ietf-oauth-client-id-metadata-document-02](https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/02/).
Draft evolution must be treated as a compatibility and security review, not an
automatic dependency update.

The initial RS256 signing key is generated in Authling's key store. A key's
stable public-key fingerprint is its `kid`; private keys never enter events or
runtime state. The issuer and initial signing-key reference are established by
an EVT event. [ADR-008](ADR-008-automatic-oidc-signing-key-rotation.md)
supersedes the initial single-key lifecycle with automatic pre-publication,
activation, overlap, and retirement. Pending requests, authorization-code
mappings, and access-token records are authenticated-encrypted in the expiring
runtime-state bucket beneath HMAC-derived keys.

Authling records explicit consent as a durable, account-owned authorization
grant for one exact client ID and scope set. A covered later request may reuse
that grant; `prompt=consent` always asks again. A signed-out browser resumes the
request through its opaque server-side request ID after login. Client redirect
URIs never become general-purpose return parameters. Grant behavior and its
future extension boundary are recorded in
[FDR-010](../fdr/FDR-010-oidc-authorization-grants.md).

## Consequences

Standard OIDC libraries can use a conventionally configured Authling client,
while CIMD-aware Chatto components can establish a relationship without an
Authling-side registration write API. Both routes share token, consent,
redirect, PKCE, and subject semantics.

The immutable issuer makes URLs operationally significant. Moving a deployed
Authling to another public origin requires an explicit future migration rather
than silently changing token identity.

CIMD adds a tightly constrained outbound HTTPS fetcher and its associated
SSRF, DNS rebinding, availability, and draft-compatibility responsibilities.
Configured clients remain the fallback for consumers that do not use CIMD.

The token and claim profile remains deliberately small. Signing-key lifecycle
is defined separately by ADR-008.
