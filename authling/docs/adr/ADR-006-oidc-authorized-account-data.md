# ADR-006: Authorize Global Account Data Through OpenID Connect

**Status:** Accepted

**Date:** 2026-08-02

## Context

ADR-005 gives a signed-in Authling browser access to one global account data
space. A Chatto frontend can instead run on a Chatto server, as a standalone
web application, or in a future desktop or mobile application. It cannot rely
on an Authling cookie or Authling's origin.

OIDC already identifies the account and client and records explicit consent.
Sending an access token in a WebSocket URL would expose bearer material to
request logs and browser history. Sending it in a cookie would also depend on
cross-site cookie policy. The browser WebSocket API cannot set an
`Authorization` header.

Application-scoped data is future work. This decision concerns only the
existing global data space.

## Decision

Authling defines the `account_data` OAuth scope. It grants read and write
synchronization of the authenticated account's global data space. `openid`
remains mandatory. The consent screen names the private data access separately
from release of the stable account identifier.

An issued access-token record binds:

- the account ID;
- the OIDC client ID;
- the granted scopes;
- the expiry time; and
- the exact origin of the authorization callback.

The callback-origin binding limits browser use of a token to the origin that
received it. Authling derives the account only from the validated token. A
client cannot provide an account ID.

An OIDC-authorized client connects to `GET /data/sync` with the
`authling.account-data.v1` WebSocket subprotocol. It sends one bounded JSON
authentication message containing the bearer token before any TinyBase
message. Authling validates the token, scope, origin, account, and expiry, then
returns a `ready` message and starts the existing TinyBase transport. Tokens do
not enter URLs, cookies, WebSocket subprotocol values, or logs.

Authling permits HTTPS client origins and loopback HTTP origins for
development. It limits authentication messages to 8 KiB, allows at most 64
pending token authentications per process, and closes a connection that does
not authenticate within two seconds. A direct network source can hold at most
eight of those slots. Admission happens only after a successful WebSocket
upgrade, so one source cannot consume the full pool and malformed HTTP requests
do not consume it. When a configured trusted reverse proxy is the direct peer,
Authling uses its sanitized, single-address `X-Forwarded-For` value. It never
trusts that header from other peers or accepts an address chain. Authling
revalidates token authority during
message handling and at least every 30 seconds while idle.

The existing exact-origin browser-session mode remains available without the
new subprotocol. It retains its current wire behavior.

## Consequences

A frontend on another origin can synchronize global account data after one
ordinary OIDC flow and explicit user consent. The same TinyBase data and
encryption formats serve browser-session and access-token clients, so this
change needs no data migration.

Any OIDC client can request `account_data`. User consent is the current grant
boundary. Authling does not yet group clients into applications or provide
application-scoped data. Those features can later add narrower grants without
changing the meaning of this global scope.

The callback-origin binding is useful for browser clients. Native clients may
need a different proof or transport binding in the future. Refresh tokens are
not available, so a client must repeat authorization after the five-minute
access token expires.

## Related

- [ADR-004: Provide OpenID Connect with CIMD-Native Client Discovery](ADR-004-cimd-native-openid-provider.md)
- [ADR-005: Synchronize Account Data with a Durable TinyBase Peer](ADR-005-tinybase-account-data-sync.md)
