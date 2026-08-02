# ADR-064: Separate the Frontend Server Catalogue from Device Sessions

**Date:** 2026-08-02

## Status

Accepted

## Context

ADR-025 introduced one `RegisteredServer` model for public server metadata,
device-local bearer credentials, cached user identity, and reauthentication
state. That model was sufficient while every visible server was also an active
local connection.

Authling account-data synchronization breaks that assumption. A server can be
known through the user's global server list while this device is signed out of
it. Public synchronization must add, update, and remove catalogue entries
without treating credentials as synchronized data. Authling connection state
is also independent from every Chatto server session.

Keeping these facts in one mutable object made components and synchronization
code coordinate persistence, authentication, retained server stores, and
navigation directly. It also made account switching dangerous: a server learned
only from one Authling account could be mistaken for local data and uploaded to
another account.

## Decision

The frontend has four explicit state boundaries:

1. The **server catalogue** owns known server IDs, immutable origins, names,
   icons, registration times, and local provenance.
2. **Server sessions** own device-local bearer tokens, cached local user
   summaries, and reauthentication state.
3. The **Authling session** owns the frontend's Authling grant, account ID,
   provider label, and connection status. Its account-data synchronizer reads
   and writes only the server catalogue.
4. The **client account coordinator** owns commands that cross these
   boundaries, such as current-server sign-out and all-server sign-out.

`ServerRegistry` composes catalogue entries and sessions with retained
per-server stores and connections. It can expose a composed compatibility view,
but metadata and authentication use separate typed mutation commands. A known
server does not imply an authenticated session.

The existing `chatto:instances` local-storage key and combined record shape
remain the persistence compatibility boundary. A small adapter splits records
between the runtime owners on load and combines them on save. This preserves
existing credentials across upgrades and permits rollback while the runtime
model improves.

Catalogue entries learned only from Authling retain local provenance. When the
frontend explicitly disconnects Authling or replaces a failed persisted grant,
it removes signed-out entries from the prior Authling account. An authenticated
entry is promoted to local provenance so disconnecting Authling does not sign
the user out of Chatto. Chatto bearer tokens never enter TinyBase or Authling.

Signing out of all servers revokes remote sessions best-effort, clears the
Authling grant and local synchronized cache, clears device-local Chatto
sessions, removes remote catalogue entries, and retains only the configured
origin entry in a signed-out state. Failure to contact Authling or a Chatto
server does not block local cleanup.

This decision supersedes the unified registration-and-session state portion of
ADR-025. ADR-025 continues to govern multi-server routing, origin detection,
per-server stores, authentication protocols, and Authling provider selection.

## Consequences

Public server-list synchronization cannot overwrite or copy local credentials.
Components can render known signed-out servers without inventing a partial
session. Sign-out and account switching have one reviewed orchestration
boundary.

The registry still provides a composed view during migration, and persistence
still uses a combined compatibility record. Maintainers must keep that adapter
free of synchronized secrets and use the explicit catalogue or session command
for mutations. Provenance is local client metadata and is not part of
Authling's synchronized server row.
