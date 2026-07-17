# FDR-031: Client–Server Compatibility Discovery

**Status:** Experimental
**Last reviewed:** 2026-07-17

## Overview

The multi-server client discovers which protocol contracts each registered
Chatto server supports, shows the server's current software version, and warns
when the client and server cannot provide the expected experience. This gives
people useful upgrade guidance while Chatto's pre-1.0 API remains experimental.

## Behavior

- A registered server's context menu shows the software version reported by
  that server's latest discovery response.
- A warning marker appears when required protocol support is missing, the
  server predates the oldest version supported by the current client, the server
  requires a newer bundled web client, or recommended realtime support is
  unavailable. The 0.5 client classifies pre-0.5 servers as unsupported because
  they do not provide the required server-projection stream.
- Missing recommended support degrades only the affected experience. The
  client does not reject a whole server merely because an optional capability
  is unavailable.
- Servers that predate compatibility discovery are classified from their
  software version when possible and otherwise remain explicitly unknown.
- An unreachable server remains registered and is reported as unreachable
  rather than being assigned a healthy or compatible state.
- Third-party clients can use the public discovery response to inspect protocol
  capability keys. The minimum web-client version applies only to Chatto's
  bundled web client.
- The `chatto.realtime.v1` protobuf namespace implements only behavioural
  protocol version 2 in 0.5. Servers reject version 0, version 1, and unknown
  handshakes; clients must discover `chatto.realtime.projection.v1` first.

## Design Decisions

### 1. Capabilities decide behaviour; versions explain legacy compatibility

**Decision:** Clients prefer stable protocol capability keys and use software
versions only when a server does not provide compatibility metadata.
**Why:** Individual capabilities can evolve independently, while a single
software-version comparison cannot explain which operation is available. A
version fallback still gives the 0.5 client a useful answer for older servers.
**Tradeoff:** Capability keys become public contracts and need deliberate
naming and maintenance.

### 2. Compatibility metadata is public discovery data

**Decision:** Protocol capabilities and an optional minimum bundled-client
version are returned with unauthenticated server discovery.
**Why:** An instance-agnostic client must decide whether it can authenticate and
render a server before it has a normal session. This follows ADR-025 and keeps
the decision independent of user permissions.
**Tradeoff:** The metadata is publicly visible, like the existing server
software version, and contributes to server fingerprinting.

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

- **ADRs:** ADR-025 (multi-instance client architecture), ADR-042 (protobuf-first public API), ADR-045 (public API stability tiers), ADR-051 (server-scoped resumable client projection)
- **FDRs:** FDR-023 (Authentication & Sessions), FDR-027 (PWA Shell & Service Worker)
