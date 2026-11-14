# ADR-010: Issue Resource-Bound JWT Access Tokens and Rotating Refresh Tokens

**Status:** Accepted

**Date:** 2026-11-13

## Context

A relying party can forward an access token to a separate API. That API needs
to verify the issuer, signature, audience, subject, and expiry without a call
to Authling. Opaque five-minute tokens cannot meet this requirement. A client
also needs a way to renew access without repeating browser authorization.

## Decision

Issue RS256 JWT access tokens under RFC 9068, with `typ: at+jwt`, through the
existing signing-key lifecycle and JWKS. Keep access tokens valid for five
minutes. Keep ID-token audiences separate: they identify the client.

Accept one optional RFC 8707 `resource` identifier at authorization. Configured
clients have exact resource allowlists. Unregistered CIMD clients can use only
resources that the operator explicitly marks as allowing CIMD. Metadata cannot
grant this permission. No resource URL is fetched. Resource selection is bound
to the consent grant, authorization request, code, and refresh family. Token
requests may repeat that selection or omit it; they cannot change it. Requests
for multiple resources return `invalid_target`.

Every access token includes Authling's UserInfo endpoint as an audience. An
explicit resource adds that API as a second audience. Without a resource,
UserInfo is the only audience. This preserves OIDC UserInfo use for clients
that also call an API. APIs must check their own audience and must not accept a
UserInfo-only token. Identity scopes release identity information; they do not
assign application roles or permissions.

`offline_access` is consented once per grant. A `prompt=none` request is
auto-approved when the grant already covers its scopes for that resource.
It is available to configured confidential clients and public clients, including
CIMD clients whose metadata declares `refresh_token`. Configured clients can
disable renewal with `allow_refresh_tokens = false`. Public clients retain mandatory S256 PKCE. A refresh family lasts
30 days at most and expires after seven days without successful use. Both
limits are configurable. Refreshes rotate credentials with one OCC write.
For a reuse grace period after a rotation (10 seconds by default, configurable from 0 to 60 seconds), the previous credential
can repeat that rotation once. The repeat returns a new access token and the
current refresh credential again; an OCC write records that the repeat is used.
Any other authenticated use of an older credential is reuse and revokes the
whole family. Clients must store the
new token before the next use.

Store each family in one encrypted runtime record. A domain-separated keyed
authenticator over a random family identifier and generation proves that a
submitted old credential was issued by Authling. This avoids an ever-growing
history of used credentials. Store no raw refresh token. Retain the record
until its absolute expiry, including after revocation, to recognize reuse.
Refreshes check the authoritative grant generation, current scopes, resource
policy, and active account. Grant revocation ends refresh authority. Existing
short-lived signed tokens and relying-party sessions retain their own expiry.

Consent disclosure version 3 adds the resource and offline-access permissions.
The resource is stored inside encrypted grant metadata. Historical disclosure
versions 1 and 2 still replay but cannot authorize new reuse. No protobuf tags
change. Upgrade all replicas together. Older binaries reject version-3 grant
history, so rollback after recording such a grant is not supported.

## Consequences

Resource servers can validate access tokens locally through discovery and JWKS.
They must require `at+jwt`, RS256, the trusted issuer, their exact audience, a
valid expiry, and the expected subject. JWT payloads are readable by their
holders. The email scope exposes the verified email to the client and selected
API. The client makes the API request; Authling makes no new outbound request.
Existing CIMD metadata fetches retain their established privacy boundary.
Refreshes do not fetch CIMD metadata; they use a cached document when one is
present. Startup rejects resource configuration that could exceed the
encrypted grant metadata capacity of one client.

JWT claims describe issuance time. UserInfo still returns current authorized
account information and checks active account state. UserInfo accepts old
opaque access tokens until their existing five-minute expiry; new exchanges
issue only JWTs. Local API verification cannot observe account deletion, email
changes, grant revocation, or family reuse before token expiry.

An uncertain storage write can require new browser authorization. So can a
lost successful refresh response that the client does not retry within the
grace period. Concurrent refreshes of one credential converge on
one replacement: after an OCC conflict, the loser reads the family again and
gets the winner's credential as a repeat. A third conflict revokes the family.
The grace period lets an attacker with a stolen previous credential get the
current credential within the grace period of a rotation, but only once. The client's
own retry, or the attacker's attempt after the client's, is then a second
repeat and revokes the family. This exposure is accepted: without it, a lost
response or two tabs that refresh together end the session.
No access token is returned after a failed rotation. A revocation concurrent with issuance can overlap an in-flight
response; every later refresh checks the durable grant again.

Revocation and introspection endpoints, RP-initiated logout, and first-party
consent bypass remain separate work. None is required for this design.

Additional consent merges scopes separately for each resource in encrypted
grant metadata. It preserves existing families without transferring permissions
between resources. Password events supply a durable account version bound into
each family; password reset and change invalidate all earlier families while
email change does not. Grants remain available for silent authorization.

All registered resource identifiers are reserved against CIMD resolution, even
when a URI has a non-root path. This prevents a resource from also being a client
audience. Configuration rejects equality with the issuer or configured clients.

Create the access-token record before consuming the code or refresh credential.
A failed write leaves the credential retryable; an orphan record has no bearer
and expires normally. Scope narrowing affects only the response. Replacement
refresh tokens retain their original scope, also when the narrowed scope omits
`offline_access`, as RFC 6749 section 6 and RFC 8707 section 2.2 require.
UserInfo permits 60 seconds of future `iat` or `nbf` skew but does not extend
expiry.

All users must consent once after this upgrade. Rollback is unsupported after
the first version-3 grant is recorded.
