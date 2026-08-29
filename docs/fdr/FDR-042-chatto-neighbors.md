# FDR-042: Chatto Neighbors

**Status:** Experimental
**Last reviewed:** 2026-08-29

## Overview

Chatto Neighbors is a public directory of Chatto servers that one server
advertises. An administrator maintains the directory. A Neighbor is a
recommendation, not a trust or reciprocal relationship.

## Behavior

- An administrator can create, read, update, and delete Neighbors in Server
  Configuration.
- Each Neighbor contains one canonical HTTP or HTTPS server origin. A server
  can advertise at most 100 Neighbors.
- The directory has no ordering contract.
- Any caller can list the advertised origins through the public discovery API.
- The advertising server does not contact a Neighbor. It does not test
  reachability, compatibility, ownership, or consent.
- `server.manage-neighbors` controls administrative access. The permission is
  independently grantable. An effective `server.manage` allow includes it
  through explicit permission metadata.
- Each administrative mutation writes a durable `EVT` fact. A resource
  revision prevents stale changes to the same Neighbor. Aggregate optimistic
  concurrency control preserves canonical-origin uniqueness across replicas.

## Design Decisions

### 1. A Neighbor is an individual resource

**Decision:** Each Neighbor has a stable ID, a canonical origin, and an opaque
revision. The administrative API provides list, get, create, update, and delete
operations.

**Why:** Individual operations match administrator intent and do not replace
unrelated directory state.

**Tradeoff:** The server maintains resource IDs and revisions in addition to
origins.

### 2. The directory is unilateral

**Decision:** A server can advertise another server without a reciprocal
confirmation.

**Why:** The first iteration is a small directory feature. It does not require
server-to-server communication, queues, or a request lifecycle.

**Tradeoff:** An advertised server has not confirmed the relationship.
Administrators and clients must not present a Neighbor as trusted or mutual.

### 3. The server stores origins but does not fetch them

**Decision:** The server validates and stores canonical origins. It does not
request discovery data, images, or health information from a Neighbor.

**Why:** Passive storage keeps writes deterministic and avoids remote effects
inside the configuration operation.

**Tradeoff:** The directory can contain an offline server or a server that is
not Chatto. A client can discover this when a user later chooses that server.

### 4. Permission inclusion is explicit

**Decision:** `server.manage` explicitly includes
`server.manage-neighbors`. Permission punctuation has no authority semantics.

**Why:** Existing server managers retain access, while an operator can delegate
only Neighbor management.

**Tradeoff:** A narrow deny cannot remove Neighbor management from an effective
`server.manage` allow.

### 5. Compatibility follows the server release boundary

**Decision:** The APIs are additive. A new client treats `Unimplemented` from
an older server as an empty Neighbor directory. Discovery does not expose a
feature flag.

**Why:** The bundled client owns minimum server versions. Method-level
capability flags would duplicate that policy.

**Tradeoff:** A client that supports several server releases must handle the
missing method explicitly.

## Non-goals

- Reciprocal requests, approval, rejection, or revocation
- Server-to-server authentication or trust
- Recursive Neighbor discovery
- Remote-server moderation or blocking
- Server-side reachability or compatibility checks

## Related

- **ADRs:** ADR-033, ADR-034, ADR-040, ADR-044, ADR-045
- **FDRs:** FDR-001 (Roles & Permissions), FDR-020 (Server Branding &
  Configuration), FDR-031 (Client–Server Compatibility Discovery)
- **Issue:** [#1669](https://github.com/chattocorp/chatto/issues/1669)
