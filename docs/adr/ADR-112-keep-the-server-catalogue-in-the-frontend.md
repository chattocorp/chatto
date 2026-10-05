# ADR-112: Keep the Server Catalogue in the Frontend

**Date:** 2026-10-02

## Status

Accepted. Partially supersedes
[ADR-111](ADR-111-move-client-state-into-chatto-client.md). Phase 1 is
implemented. Phase 2 is planned. The server catalogue is the device-local
list of servers that [ADR-064](ADR-064-separate-server-catalog-and-sessions.md)
separates from sessions and
[ADR-074](ADR-074-keep-server-catalogue-device-local.md) keeps on the device.

## Context

ADR-111 moved the frontend's client state into `@chatto/client`. The move
included the complete `ServerRegistry`. The registry has two different jobs:

1. **The servers of a client.** It adds a server, creates its store, keeps its
   session, renews its bearer token, and handles a rejected session. Every
   host needs this to connect to a server. A bot uses it through
   `client.connect()`.
2. **The server catalogue of the bundled frontend.** It keeps the list of
   servers that the user added on this device, finds and registers the origin
   server, registers a server from the Server Directory without a session, and
   selects a server for navigation. This is the policy of one application.

The second job makes `@chatto/client` larger and harder to understand for
integrations, which only need to connect to a server. It also makes it easy to
add more frontend policy to the package, as the Server Directory join did.

## Decision

`@chatto/client` keeps the servers of a client. The bundled frontend keeps its
server catalogue. The frontend builds the catalogue on the public API of the
client.

### What stays in `@chatto/client`

- Server lifecycle: `addServer`, `removeServer`, `init`, `dispose`, `servers`,
  `getServer`, `getStore`, `tryGetStore`, and `watchStores`.
- Sessions: the session of each server, bearer renewal with its cross-tab
  coordination, the handling of a rejected session, `replaceServerAuthentication`,
  and `clearServerAuthentication`.
- Fixed tokens for `connect()`, and server ID allocation (`generateServerId`,
  now in `server/serverIds` with the process-wide ID claims).
- The origin cookie session: `originServer`, `isOriginServer`,
  `authenticateOriginCookie`, `clearOriginAuthentication`,
  `settleOriginUnauthenticated`, and `originSignInRequired`. Any browser host
  on a Chatto origin needs these. `authenticateOriginCookie` still registers
  the origin when a signed-in cookie session arrives before the origin is
  registered.
- Recovery of discovery and of the viewer (`needsRecovery`, `recoverServer`,
  and the runtime's recovery schedule). Bots use it to reconnect.
- The storage option for sessions (`memory` or `device`), because a
  third-party frontend can also keep sessions across page loads.
- `serverDisplayName`, which knows the client's default server name.

### What moves to the frontend

Phase 1 (implemented):

- Origin discovery: the frontend asks its origin for public server data and
  registers the origin as a server (`registerOriginServer` in
  `$lib/serverCatalogue`). The registry's `probeOrigin` and `originProbed` are
  removed.
- The Server Directory join: `addSignedOutServer` and `findServerByUrl`.
- Navigation policy: `firstAuthenticatedServerId`.
- URL helpers for the catalogue and its display: `canonicalServerOrigin` and
  `serverHost` (`$lib/serverUrl`).

Phase 2 (planned):

- The device-stored server list (the `instances` record and its migration).
  The registry then receives the list from its host through a storage adapter
  and keeps only the authentication records.
- Account policy: `resetToOrigin` and `removeAll`.
- Registration metadata (`updateRegistration`) and the composed
  `RegisteredServer` view.

## Consequences

- `@chatto/client` is smaller, and its registry has one job. Integrations do
  not see frontend policy.
- Frontend policy for servers has one place, `$lib/serverCatalogue`.
- Until phase 2 is done, the registry still writes the device-stored server
  list. The frontend's catalogue functions change it only through the public
  registry API.
- A third-party frontend that wants a catalogue writes its own, or copies the
  bundled frontend's. This is intended: catalogue policy differs between
  applications.
