# ADR-071: Identify Open OAuth Clients through CIMD

**Date:** 2026-08-11

**Status:** Accepted

## Context

Chatto's multi-server frontend uses OAuth Authorization Code with PKCE to add a
server without asking users to enter that server's credentials into another
server's frontend. The original flow authorized callbacks through
`webserver.oauth_redirect_origins` and `webserver.allowed_origins`. Operators
therefore had to coordinate each frontend with every server it might connect
to, or set both OAuth callback and credentialed CORS policy to `*`.

An unrestricted callback wildcard makes the client ecosystem open, but gives
the server no stable client identity, no exact callback registration, and no
durable key on which to base consent, audit, revocation, or administrative
policy. An operator-maintained registration table would restore those
properties at the cost of recreating the coordination problem.

Client ID Metadata Documents (CIMD) let a client use an HTTPS metadata URL as
its OAuth `client_id`. The document repeats that identifier and publishes the
client's exact redirect URIs and display metadata. A client can therefore
identify itself without prior registration at each Chatto server.

## Decision

Chatto's OAuth client ecosystem is open: any compatible public client may ask a
user to authorize access. Browser clients identify themselves with a CIMD URL.
Chatto Desktop uses the fixed built-in client identifier `chatto://desktop` and
an exact built-in callback. Future native applications may use HTTPS-hosted
CIMD metadata with native application callback schemes.

The authorization server retrieves a CIMD document itself and validates that:

- the client identifier is an HTTPS URL with a non-root path, or an HTTP
  loopback URL when the Chatto server itself is in loopback development;
- the document's `client_id` exactly equals the URL that served it;
- the client is public (`token_endpoint_auth_method` is `none`) and supports
  Authorization Code;
- the requested callback exactly equals one declared redirect URI; and
- web callbacks use HTTPS, while native private-use schemes require
  `application_type = "native"`.

CIMD retrieval is an unauthenticated network boundary. Chatto disables proxy
inheritance and redirects, limits concurrency, the complete resolution/fetch
time and body size, requires a JSON media type, rejects special-use destination addresses, pins
the validated destination through dialing to resist DNS rebinding, and caches
only valid metadata for at most five minutes in a bounded cache. The retrieval
concurrency limit covers destination resolution as well as HTTP. Loopback destinations are
available only for a loopback development server.

The validated authorization request is stored behind an opaque, HMAC-keyed,
single-use `RUNTIME_STATE` handle; the signed browser session carries only that
small handle. This keeps large valid metadata out of the browser cookie and lets
any replica continue the flow. The `client_id` is then carried through the
single-use authorization code and the resulting opaque OAuth access-token
record. Token exchange requires the same client identifier, callback, and PKCE
verifier used by authorization. A mismatch consumes the code and fails closed.
Consent is remembered by user plus stable client identifier. The consent page
shows the validated client name and identifier origin, rather than attributing
the request to a potentially unrelated callback service. Audit facts retain the
client identifier, validated client URI origin, and canonical callback origin
or native callback scheme observed at the time of the decision; client URI
paths and queries are not persisted.

CIMD identifies a client; it does not endorse it. Users still see and approve
the client application before a token is issued. A future administrative policy
may trust or block known clients without changing this open registration model.
Metadata probes alone do not create durable known-client state.

CIMD can also publish `jwks` or `jwks_uri`, but this decision does not add
client-authentication proofs, service credentials, scopes, token exchange, or
client-credentials grants. Browser and native clients remain public clients
using PKCE. Server-to-server push relay is outside this decision.

The 0.5 client and server move to this contract together. Compatibility with
pre-0.5 origin-only OAuth clients is not a design requirement; the temporary
origin allow-list path is removed once the bundled frontend sends `client_id`.

## Consequences

Any software capable of publishing a valid CIMD document can act as a Chatto
client, subject to user consent. This is intentionally equivalent to `*` in
who may ask, while providing exact callback binding and a stable identity for
audit and policy.

Operators no longer need to register every remote frontend on every server.
The authorization server makes a bounded outbound HTTPS request to client
metadata, so private-network-only metadata requires loopback development or a
future explicit private-destination trust mechanism rather than implicit SSRF
reachability.

Changing a client's metadata affects new authorization attempts after the
short cache expires. An authorization already in progress remains bound to its
validated metadata snapshot; token exchange remains bound to the client ID and
exact callback embedded in its one-time code.

Native applications can participate without browser CORS, and Chatto Desktop
continues to work without hosting a metadata document. The fixed Desktop
registration is part of the server release rather than an operator setting.

The persisted user-event changes are additive. Existing origin-keyed consent
facts remain replayable, but 0.5 clients create client-ID-keyed facts and access
tokens record their issuing client.

## Related

- [ADR-024](ADR-024-opaque-bearer-tokens-for-cross-origin-auth.md)
- [ADR-025](ADR-025-multi-instance-client-architecture.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [ADR-067](ADR-067-electron-desktop-client.md)
- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md)
