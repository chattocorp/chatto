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

The bundled frontend publishes its CIMD document for the canonical
`webserver.url` origin and each exact `webserver.allowed_origins` entry. The
request host must match one of these configured origins. The document uses the
matched origin for its client ID, client URI, and callback. A wildcard or an
unknown request host does not publish a frontend client identity. Alias entries
must be origins without paths, queries, or fragments. Two configured origins
must not use different schemes with the same request host because a
TLS-terminating proxy does not provide a trusted request scheme.

The authorization server retrieves a CIMD document itself and validates that:

- the client identifier is an HTTPS URL with a non-root path, or an HTTP
  loopback or concrete `.localhost` URL when the Chatto server itself uses a
  local development URL;
- the document's `client_id` exactly equals the URL that served it;
- the client is public (`token_endpoint_auth_method` is `none`), supports
  Authorization Code, and declares no grant outside Authorization Code and the
  rotating refresh grant added by ADR-079;
- the requested callback exactly equals one declared redirect URI, except that
  a literal `127.0.0.1` or `[::1]` callback can use a different port;
- callback registrations cannot contain a wildcard hostname; and
- web callbacks use HTTPS, while native private-use schemes and remote HTTP
  loopback callbacks require `application_type = "native"`.

A native client can register an HTTP callback on `127.0.0.1`, `[::1]`,
`localhost`, or one concrete hostname below `.localhost`, even when Chatto is
not on a local host. Named localhost callbacks keep an exact port. Literal IP
callbacks can select an available runtime port as required by RFC 8252.
The authorization code and token exchange remain bound to the callback that
the client selected for that authorization.

CIMD retrieval is an unauthenticated network boundary. Chatto disables proxy
inheritance and redirects, limits concurrency, the complete resolution and
fetch time, and the body size. It requires a JSON media type, rejects
special-use destination addresses, pins
the validated destination through dialing to resist DNS rebinding, and caches
only valid metadata for at most five minutes in a bounded cache. The retrieval
concurrency limit covers destination resolution as well as HTTP. Loopback
destinations are available only when the Chatto server itself uses a loopback
or concrete `.localhost` development URL. A remote server does not retrieve
CIMD from a local address.

The validated authorization request is stored behind an opaque, HMAC-keyed,
single-use `RUNTIME_STATE` handle; the signed browser session carries only that
small handle. This keeps large valid metadata out of the browser cookie and lets
any replica continue the flow. The `client_id` is then carried through the
single-use authorization code and the resulting opaque OAuth access-token
record. Token exchange requires the same client identifier, callback, and PKCE
verifier used by authorization. A mismatch consumes the code and fails closed.
Consent is remembered by user plus stable client identifier for non-local
callbacks. A local callback requires a new decision for each authorization.
The consent page shows the validated client name and identifier origin, rather
than attributing the request to a potentially unrelated callback service. It
also shows the exact local callback origin and warns the user to continue only
when they started the request. Audit facts retain the client identifier,
validated client URI origin, and canonical callback origin or native callback
scheme observed at the time of the decision; client URI paths and queries are
not persisted.

CIMD identifies a client; it does not endorse it. Users still see and approve
the client application before a token is issued. After a user successfully
authorizes a client, Chatto records the client's validated identity, display
metadata, callback origins, first and latest authorization times, and distinct
authorized-user count. Metadata probes alone do not create durable known-client
state.

Administrators can leave a recorded client at the default policy, label it
trusted, or block it. Trust is an administrative annotation and never bypasses
user consent. Blocking rejects new authorization attempts, authorization-code
issuance, code exchange, and access-token use; changing a client to blocked also
scans and revokes its renewable OAuth sessions, which invalidates every access
generation. Policy changes are durable EVT facts, and each replica enforces
them from a cold-replayed projection.

CIMD can also publish `jwks` or `jwks_uri`, but this decision does not add
client-authentication proofs, service credentials, scopes, token exchange, or
client-credentials grants. Browser and native clients remain public clients
using PKCE. Server-to-server push relay is outside this decision.

The 0.5 client and server move to this contract together. Compatibility with
pre-0.5 origin-only OAuth clients is not a design requirement. The bundled
frontend sends `client_id`, and the temporary origin allow-list path is removed.

## Consequences

Any software capable of publishing a valid CIMD document can act as a Chatto
client, subject to user consent. This is intentionally equivalent to `*` in
who may ask, while providing exact callback binding and a stable identity for
audit and policy.

The administration surface shows only clients that have completed at least one
user-approved authorization. It is therefore an inventory of successful access,
not a registry of every metadata document that anyone has asked the server to
retrieve.

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
Native development tools can use a local callback with a remote Chatto server,
but the user must approve each local handoff. A local process can claim the
callback before the intended client, so public metadata and PKCE do not make a
local callback equivalent to an HTTPS callback controlled by the client.

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
- [ADR-079](ADR-079-renewable-bearer-sessions.md)
- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md)
