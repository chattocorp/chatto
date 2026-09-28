# FDR-031: Client–Server Compatibility Discovery

**Status:** Experimental
**Last reviewed:** 2026-09-28

## Overview

The multi-server client compares each registered Chatto server's software
version with the oldest release that the client supports, shows the server's
current version, and warns when the client and server cannot provide the
expected experience. This gives people useful upgrade guidance while
Chatto's pre-1.0 API remains experimental.

## Behavior

- A registered server's context menu shows the software version reported by
  that server's latest discovery response.
- A registered server's context menu or touch sheet shows its configured host
  and provides a final action that copies that exact host, including a
  non-default port, to the clipboard.
- A warning marker appears when the server predates the oldest version
  supported by the current client. The 0.5 client supports `0.5.0-beta.9` and
  newer. It classifies older servers as unsupported and does not open their
  realtime stream.
- Servers with non-standard or unparseable versions remain explicitly unknown.
- An unreachable server remains registered and is reported as unreachable
  rather than being assigned a healthy or compatible state.
- The server context menu and touch sheet explain each active sign-in,
  connection, and compatibility warning. When an unreachable status and a lost
  connection describe the same failure, they appear as one warning.
- Third-party clients own and test their own minimum supported server release.
- The `chatto.realtime.v1` protobuf namespace uses behavioral protocol version
  4 for the public event stream. The alpha server rejects older and unknown
  handshakes.

## Design Decisions

### 1. The bundled client has one minimum server version

**Decision:** The bundled client records one minimum supported server release.
Every feature that the client uses exists in that release, so the client does
not gate individual features by server version. When the client starts to use
a feature of a newer server, the minimum increases.
**Why:** Per-feature version gates make each screen keep a second code path for
servers that are already unsupported. Exposing implementation-level protocol
flags would also turn internal rollout details into a public contract. One
minimum keeps version knowledge in one place.
**Tradeoff:** A client cannot use a newer feature on new servers while it still
supports older servers. Forks and builds with non-standard version strings
cannot declare support independently; the client treats them as unknown and
does not open their realtime stream.

### 2. The client owns compatibility policy

**Decision:** Unauthenticated discovery reports the server software version.
Each client owns its minimum supported server release and compares that policy
with the discovered version before connecting.
**Why:** Future clients know which older server contracts they still implement;
the server cannot predict the requirements of clients that do not exist yet.
This also avoids turning client release policy into public server metadata.
**Tradeoff:** A client update must keep its minimum version accurate and
cannot rely on a server to reject it on the client's behalf.

### 3. Registration data does not cache compatibility conclusions

**Decision:** The client keeps version and compatibility results in live
per-server state and refreshes them from discovery instead of persisting them
with the registered server and its credentials.
**Why:** Persisted compatibility information would become stale across server
and client upgrades. The registry should retain connection identity, while the
server state owns current discovery facts.
**Tradeoff:** Compatibility is unknown until discovery completes after the
client starts.

### 4. Pre-1.0 compatibility remains advisory

**Decision:** Compatibility discovery informs connection decisions and warnings but
does not turn the experimental `v1` packages into a stability guarantee.
**Why:** Chatto still needs room to reshape its public API in response to early
feedback. ADR-045 requires intentional review and migration guidance for
breaks without prematurely freezing the API.
**Tradeoff:** Integrators must still pin server versions and read release notes.

## Related

- **ADRs:** ADR-025 (multi-instance client architecture), ADR-042 (protobuf-first public API), ADR-045 (public API stability tiers), ADR-067 (Electron desktop packaging), ADR-091 (semantic realtime events)
- **FDRs:** FDR-017 (Room Groups & Sidebar Layout), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-034 (Chatto Desktop), FDR-045 (Realtime Event Stream)
