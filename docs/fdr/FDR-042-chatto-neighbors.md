# FDR-042: Chatto Neighbors

**Status:** Experimental
**Last reviewed:** 2026-08-30

## Overview

Chatto Neighbors is a public directory of Chatto servers that one server
advertises. An administrator maintains the directory. A Neighbor is a
recommendation, not a trust or reciprocal relationship.

## Behavior

- An administrator can create, read, update, and delete Neighbors in Server
  Configuration.
- The administration form accepts a server hostname or URL. The client sends
  only the canonical HTTP or HTTPS origin for storage.
- Each Neighbor contains one canonical HTTP or HTTPS server origin and can
  contain one public testimonial. The testimonial contains at most 500 Unicode
  characters. A server can advertise at most 100 Neighbors.
- A Neighbor origin cannot match the server's canonical `webserver.url` origin
  or an exact `webserver.allowed_origins` alias.
- The directory has no ordering contract.
- Any caller can list the advertised origins through the public discovery API.
- The Server Directory page combines the direct Neighbors from all servers
  registered in the client. It removes duplicate canonical origins but does
  not rank or sort the results. Each result identifies the registered servers
  that recommend it. It preserves each source's testimonial.
- The Server Directory uses a tapestry layout. It shows each testimonial in a
  review card below the server profile. The review card shows the device-local
  name and icon of the registered server that supplied it.
- The Server Directory and Neighbor administration page load each advertised
  server's public name, description, logo, and banner. The Server Directory
  omits a server when its public profile does not load. The administration page
  keeps that server visible so that an administrator can review or remove it.
  A failed request does not hide profiles that loaded successfully.
- An advertised server that is already registered remains visible and is
  marked as joined.
- An unregistered server has a join action only when its discovered version is
  compatible with the client. When the version is incompatible or unknown,
  the client opens the server origin in a new tab. The server can then provide
  its own compatible client.
- A user can enter a server address directly when the wanted server is not in
  the directory.
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

**Decision:** Each Neighbor has a stable ID, a canonical origin, an optional
testimonial, and an opaque revision. The administrative API provides list,
get, create, update, and delete operations.

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

### 3. The server stays passive and the client loads public profiles

**Decision:** The server validates and stores canonical origins. It does not
request discovery data, images, or health information from a Neighbor. The
client requests public discovery data directly from advertised origins when it
displays the Server Directory or the Neighbor administration page.

**Why:** Passive storage keeps writes deterministic and avoids remote effects
inside the configuration operation.

**Tradeoff:** Opening the Server Directory or Neighbor administration page sends
browser requests to advertised servers. The Server Directory omits an offline
or invalid server. The administration page shows it without a public profile
so that an administrator can remove it.

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

### 6. Joined servers remain visible

**Decision:** The Server Directory does not remove an advertised origin when
that server is already in the device-local server catalogue. It marks the
server as joined and offers the applicable open or sign-in action.

**Why:** The complete directory shows the recommendation network without
making entries disappear after a user joins them.

**Tradeoff:** The directory includes entries that do not offer a new server to
join.

### 7. Deduplication preserves recommendation sources

**Decision:** One server appears once in the Server Directory. The result also
identifies each registered server that advertises it.

**Why:** A user can see where a recommendation comes from without seeing
duplicate server cards.

**Tradeoff:** The source names depend on the server catalogue on the user's
device.

### 8. A testimonial belongs to one recommendation

**Decision:** A Neighbor can contain one optional testimonial. The server trims
outer white space and limits the text to 500 Unicode characters. Clients can
render paragraphs, emphasis, strong emphasis, and inline code. They do not
render links, headings, lists, tables, images, or source HTML. An empty
testimonial clears the value. When one update changes the origin and
testimonial, the server writes both facts in one atomic `EVT` batch.

**Why:** A testimonial explains why one server recommends another server. It
must stay associated with that directed recommendation when clients combine
results from several sources.

**Tradeoff:** Different sources can publish different testimonials for the
same target server. The directory must preserve the source of each text.

### 9. Structured discovery extends the origin list

**Decision:** Public discovery returns structured Neighbor values with an
origin and optional testimonial. It also returns the existing origin list.
New clients prefer the structured values and use the origin list as a fallback.

**Why:** Old clients can continue to read the origin list. New clients can show
testimonials when the server supplies them.

**Tradeoff:** The server sends the origins in two fields during the
compatibility period.

### 10. A server cannot advertise itself

**Decision:** Create and update operations reject an origin that identifies
the server. This set includes the canonical `webserver.url` origin and each
exact non-wildcard `webserver.allowed_origins` entry.

**Why:** A self-reference does not recommend another server. Reverse-proxy
aliases must not make the same server appear as a separate Neighbor.

**Tradeoff:** A configuration change can make an existing Neighbor identify
the server. Chatto keeps that historical Neighbor so an administrator can
remove it or change it to an external origin.

### 11. Incompatible servers use their own client

**Decision:** The Server Directory does not add an unregistered server when
the discovered version is below the client's minimum supported version or is
unknown. It opens the canonical server origin in a new tab. Registered servers
keep their open or sign-in action.

**Why:** The remote server can provide a client that matches its release. The
current client must not start a server registration flow that it cannot
support.

**Tradeoff:** A server with a missing or non-standard version cannot use the
direct join flow, even when it might work with the client.

## Non-goals

- Reciprocal requests, approval, rejection, or revocation
- Server-to-server authentication or trust
- Recursive Neighbor discovery
- Directory ranking or sorting
- Remote-server moderation or blocking
- Server-side reachability or compatibility checks

## Related

- **ADRs:** ADR-033, ADR-034, ADR-040, ADR-044, ADR-045
- **FDRs:** FDR-001 (Roles & Permissions), FDR-020 (Server Branding &
  Configuration), FDR-031 (Client–Server Compatibility Discovery)
- **Issues:** [#1669](https://github.com/chattocorp/chatto/issues/1669),
  [#2208](https://github.com/chattocorp/chatto/issues/2208)
