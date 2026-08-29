# FDR-031: Client–Server Compatibility Discovery

**Status:** Experimental
**Last reviewed:** 2026-08-29

## Overview

The multi-server client compares each registered Chatto server's software
version with the releases that introduced the features it uses, shows the
server's current version, and warns when the client and server cannot provide
the expected experience. This gives people useful upgrade guidance while
Chatto's pre-1.0 API remains experimental.

## Behavior

- A registered server's context menu shows the software version reported by
  that server's latest discovery response.
- A registered server's context menu or touch sheet shows its configured host
  and provides a final action that copies that exact host, including a
  non-default port, to the clipboard.
- A warning marker appears when the server predates the oldest version
  supported by the current client. The 0.5 client classifies pre-0.5 servers as
  unsupported because they do not provide the required server-projection
  stream.
- Servers with non-standard or unparseable versions remain explicitly unknown.
- An unreachable server remains registered and is reported as unreachable
  rather than being assigned a healthy or compatible state.
- Third-party clients own and test their own minimum supported server release.
- The bundled client shows relative sidebar drag handles only when the server
  version supports relative room-group and sidebar-item moves. Other sidebar
  management actions continue to use the older management API when available.
- The `chatto.realtime.v1` protobuf namespace implements only behavioural
  protocol version 2 in 0.5. Servers reject version 0, version 1, and unknown
  handshakes.

## Design Decisions

### 1. The bundled client records minimum server versions per feature

**Decision:** Features that vary across releases use one internal table mapping
the feature to the first server version that supports it.
**Why:** The 0.5 release is a clean compatibility baseline, and exposing
implementation-level protocol flags would turn internal rollout details into a
public contract. An explicit table keeps version knowledge in one place.
**Tradeoff:** Forks and builds with non-standard version strings cannot declare
support independently; the client treats them conservatively as unknown or
unsupported for gated features.

### 2. The client owns compatibility policy

**Decision:** Unauthenticated discovery reports the server software version.
Each client owns its minimum supported server release and compares that policy
with the discovered version before connecting.
**Why:** Future clients know which older server contracts they still implement;
the server cannot predict the requirements of clients that do not exist yet.
This also avoids turning client release policy into public server metadata.
**Tradeoff:** A client update must keep its minimum-version table accurate and
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

**Decision:** Compatibility discovery informs feature gating and warnings but
does not turn the experimental `v1` packages into a stability guarantee.
**Why:** Chatto still needs room to reshape its public API in response to early
feedback. ADR-045 requires intentional review and migration guidance for
breaks without prematurely freezing the API.
**Tradeoff:** Integrators must still pin server versions and read release notes.

## Related

- **ADRs:** ADR-025 (multi-instance client architecture), ADR-042 (protobuf-first public API), ADR-045 (public API stability tiers), ADR-051 (server-scoped resumable client projection), ADR-067 (Electron desktop packaging)
- **FDRs:** FDR-017 (Room Groups & Sidebar Layout), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-034 (Chatto Desktop)
