# FDR-042: Chatto Neighbors

**Status:** Experimental
**Last reviewed:** 2026-09-26

## Overview

Chatto Neighbors is a public directory of Chatto servers that one server
advertises. An administrator maintains the directory. A Neighbor is a
recommendation, not a trust or reciprocal relationship.

## Behavior

- An administrator can create, read, update, and delete Neighbors in Server
  Configuration.
- The administration form accepts a server hostname or URL. The client sends
  only the canonical HTTP or HTTPS origin for storage.
- Each Neighbor contains one canonical HTTP or HTTPS server origin. A server
  can advertise at most 100 Neighbors.
- A Neighbor origin cannot match the server's canonical `webserver.url` origin
  or an exact `webserver.allowed_origins` alias.
- The directory has no ordering contract.
- Any caller can list the advertised origins through the public discovery API.
- Each server discovers its Neighborhood in the background. The Neighborhood
  contains each direct Neighbor whose public profile loads. It also contains
  servers that a mutually advertising Neighbor recommends when that
  recommendation is also mutual, to at most two mutual hops.
- Any caller can list the cached Neighborhood through the public discovery
  API. The call does not contact other servers. Each result has the server
  origin, public name, version, description, logo, and banner. It also tells
  whether the called server advertises the result directly and which other
  Neighborhood servers mutually recommend it.
- The server refreshes the Neighborhood when the cached result is one hour old.
  After a failed remote request, it refreshes when the cached result is ten
  minutes old. A Neighbor change starts a refresh within about fifteen seconds.
  One pass permits at most 150 directory requests and 120 profile requests,
  with six active requests and a ten-second timeout for each request.
- Neighborhood discovery rejects redirects and servers on loopback, private,
  and link-local network addresses. It stores re-encoded copies of logos and
  banners and serves them from the called server.
- The Add Server action in the Server Gutter opens the Server Directory in a
  history-backed view that fills the app frame. The app header stays
  available. Escape, the close button, and the browser Back action close it.
  In a narrow space, the server cards are icon tiles in two columns. The
  `/chat/servers` route shows the same directory as a page.
  The standalone client uses this page before it registers a server.
- The Server Directory starts with a direct server-address lookup, followed
  by the recommendations.
- The Server Directory loads the cached Neighborhood of each server that is
  registered in the client. It contacts no other server to show results and
  does not ask for consent. A registered server already knows the user's
  network address.
- The client removes duplicate canonical origins but does not rank or sort the
  results. Each result identifies all sources whose recommendations are shown.
  A direct Neighbor of a registered server names that registered server as a
  source.
- The Server Directory shows its results as server profile cards in a grid. A
  card without a banner shows a gradient that the server name selects.
- The Neighbor administration page loads each advertised server's public name,
  description, logo, and banner directly. It keeps an advertised server visible
  so that an administrator can review or remove it. A failed request does not
  hide profiles that loaded successfully.
- A public profile card accepts a logo or banner only from one origin. On the
  Neighbor administration page, this is the advertised server. In the Server
  Directory, this is the registered server that supplied the cached copy. The
  client loads the image without credentials or referrer data, rejects
  redirects, and accepts only responses that declare a supported raster image
  media type and contain at most 5 MiB.
- The Server Directory sends one request to each registered server, with at
  most six active requests and a ten-second timeout for each request. A
  registered server that does not provide a Neighborhood contributes no
  results and does not count as a failure.
- Joining a server first loads its current public sign-in data from that
  server. The user starts this request with the join action. The sign-in
  window opens from that action. If the current version is not compatible or
  sign-in is not available, the client closes the window, stops the join, and
  shows the current action for that server.
- An advertised server that is already registered remains visible and is
  marked as joined. Opening, joining, or signing in to a server from the dialog
  replaces the dialog's history entry, so Back does not reopen the dialog.
- An unregistered server has a join action only when its discovered version is
  compatible with the client. When the version is incompatible or unknown,
  the client opens the server origin in a new tab. The server can then provide
  its own compatible client.
- A user can enter a server address directly when the wanted server is not in
  the directory.
- Neighbor administration does not contact a Neighbor. Background
  Neighborhood discovery reads public data from Neighbors and their mutual
  recommendations. It does not test compatibility, ownership, or consent.
- `server.manage-neighbors` controls administrative access. The permission is
  independently grantable. An effective `server.manage` allow includes it
  through explicit permission metadata.
- Each administrative mutation writes a durable `EVT` fact. A resource
  revision prevents stale changes to the same Neighbor. Aggregate optimistic
  concurrency control preserves canonical-origin uniqueness across replicas.

## Design Decisions

### 1. A Neighbor is an individual resource

**Decision:** Each Neighbor has a stable ID, a canonical origin, and an opaque
revision. The administrative API provides list, get, create, update, and
delete operations.

**Why:** Individual operations match administrator intent and do not replace
unrelated directory state.

**Tradeoff:** The server maintains resource IDs and revisions in addition to
origins.

### 2. Direct recommendations remain unilateral

**Decision:** The Server Directory shows a direct recommendation from a
registered server without reciprocal confirmation. Neighborhood discovery
expands that remote server only when it observes that both public directories
advertise each other.

**Why:** Direct recommendations keep the directory useful for old servers and
for servers that do not advertise their recommenders back. Observed
mutuality prevents one unilateral recommendation from amplifying the recursive
crawl.

**Tradeoff:** A direct result can remain one-sided. Two matching public
responses are not durable consent or an authenticated relationship. The
observation can change between requests.

### 3. The server stays passive and the client loads public profiles

**Status:** Partially superseded by ADR-106 and Design Decision 14. The server
now contacts Neighbors for background Neighborhood discovery, and the Server
Directory reads the cached result. Neighbor administration writes remain
passive. The Neighbor administration page still loads public profiles in the
browser.

**Decision:** The server validates and stores canonical origins. It does not
request discovery data, images, or health information from a Neighbor. The
client requests public discovery data directly from advertised origins when it
displays the Neighbor administration page. The Server Directory makes these
requests only after the user gives consent or when the device has saved consent.

**Why:** Passive storage keeps writes deterministic and avoids remote effects
inside the configuration operation.

**Tradeoff:** Opening the Neighbor administration page sends browser requests
to advertised servers. The Server Directory sends them after consent. It omits
an offline or invalid server. The administration page shows that server without
a public profile so that an administrator can remove it.

### 4. Permission inclusion is explicit

**Decision:** `server.manage` explicitly includes
`server.manage-neighbors`. Permission punctuation has no authority semantics.

**Why:** Existing server managers retain access, while an operator can delegate
only Neighbor management.

**Tradeoff:** A narrow deny cannot remove Neighbor management from an effective
`server.manage` allow.

### 5. Compatibility follows the server release boundary

**Decision:** A new client treats `Unimplemented` from an older server as an
empty Neighbor directory. Public discovery returns only canonical origins and
does not expose a feature flag.

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
identifies each source server whose recommendation is shown.

**Why:** A user can see where a recommendation comes from without seeing
duplicate server cards.

**Tradeoff:** A source name can come from the device-local catalogue or from a
cached Neighborhood profile. A cached name can be up to one hour old.

### 8. Recommendations contain no operator-written text

**Decision:** A Neighbor contains no operator-written text. Current APIs,
projections, snapshots, and clients use only the Neighbor origin, ID, and
revision. Legacy testimonial facts remain decodable for `EVT` replay. Their
text has no projected effect, but each fact advances the Neighbor revision.

**Why:** Public operator-written text creates an abuse and moderation surface
that is not necessary for server discovery.

**Tradeoff:** An operator cannot publish an explanation with a recommendation.
Historical text remains in `EVT`, backups, and old snapshot generations until
their normal retention policies remove it.

### 9. A server cannot advertise itself

**Decision:** Create and update operations reject an origin that identifies
the server. This set includes the canonical `webserver.url` origin and each
exact non-wildcard `webserver.allowed_origins` entry.

**Why:** A self-reference does not recommend another server. Reverse-proxy
aliases must not make the same server appear as a separate Neighbor.

**Tradeoff:** A configuration change can make an existing Neighbor identify
the server. Chatto keeps that historical Neighbor so an administrator can
remove it or change it to an external origin.

### 10. Incompatible servers use their own client

**Decision:** The Server Directory does not add an unregistered server when
the discovered version is below the client's minimum supported version or is
unknown. It opens the canonical server origin in a new tab. Registered servers
keep their open or sign-in action.

**Why:** The remote server can provide a client that matches its release. The
current client must not start a server registration flow that it cannot
support.

**Tradeoff:** A server with a missing or non-standard version cannot use the
direct join flow, even when it might work with the client.

### 11. Recursive discovery has a per-pass budget

**Decision:** Neighborhood discovery lists direct recommendations and follows
at most two verified mutual hops. One pass permits at most 150 directory
requests and 120 profile requests, with six active requests and a ten-second
timeout for each request. It reads at most 100 origins from one remote
directory.

**Why:** Recursive discovery can find servers beyond a direct recommendation,
but one malicious or cyclic directory must not start unbounded work. Fixed
limits make the maximum request effect testable.

**Tradeoff:** A pass can stop before it explores every recommendation. Result
order reflects discovery order and the budget, not quality.

### 12. Server Directory discovery requires consent

**Status:** Superseded by Design Decision 14 and ADR-106. The Server Directory
contacts only registered servers, so it no longer asks for consent. A saved
consent value from an older client has no effect.

**Decision:** The Server Directory does not contact advertised servers until
the user selects **Discover servers**. Before that action, the client explains
that each contacted server can see the user's IP address and technical
connection details. The client saves consent in device-local storage. Direct
server lookup remains available without Server Directory consent because the
user supplies its server address in an explicit action. The Neighbor
administration page does not use this prompt because administrators add the
remote origins and open the page to inspect and manage them.

**Why:** Opening a page must not expose the user's network information to a
set of remote systems without a clear choice. Device-local consent avoids a
repeated prompt after the user understands and enables discovery. The
administrator workflow already makes the remote systems and the purpose of the
connections clear.

**Tradeoff:** A user must take one extra action before first use on each device
or browser profile. Clearing local data makes the client ask again. An
administrator does not get a separate connection prompt on the management page.

### 13. Public profile images use a restricted source

**Decision:** A public profile card accepts an image only from one expected
origin: the advertised server on the Neighbor administration page, or the
registered server that supplied the cached copy in the Server Directory. The
request sends no credentials or referrer data, does not follow redirects, and
accepts a limited set of declared raster image media types. It rejects an image
response after its body exceeds 5 MiB.

**Why:** A profile must not make the client contact an unrelated image host or
send reusable user credentials. Rejecting redirects prevents hidden network
hops. The media-type and size limits bound untrusted image handling.

**Tradeoff:** Images on a content delivery network, redirected images, and SVG
images do not display. A remote image response must permit the browser's
cross-origin request.

### 14. The server discovers and caches the Neighborhood

**Decision:** Each server runs Neighborhood discovery in the background with
the mutual-hop rules and fixed request limits. It stores the latest result in
`MEMORY_CACHE` and image copies in `NEIGHBORHOOD_IMAGES`. The public
`ListNeighborhoodServers` RPC returns only the cached result. The Server
Directory merges the cached results of all registered servers and does not ask
for consent. See ADR-106.

**Why:** A registered server already knows the user's IP address. When it
contacts other servers, those servers do not see the user's address. A client
can then show the Neighborhood without a consent step. One cached pass also
replaces a separate crawl for each user visit.

**Tradeoff:** The server makes outbound requests and cannot reach servers on
private network addresses. Results can be up to one hour old.

## Permissions

- `server.manage-neighbors` permits Neighbor administration. A human session
  must also have privileged mode active.
- An effective `server.manage` allow includes `server.manage-neighbors`.

## Non-goals

- Authenticated reciprocal requests, approval, rejection, or revocation
- Server-to-server authentication or trust
- Unbounded recursive Neighbor discovery
- Directory ranking or sorting
- Remote-server moderation or blocking
- Server-side compatibility checks

## Related

- **ADRs:** ADR-033, ADR-034, ADR-040, ADR-044, ADR-045, ADR-106
- **FDRs:** FDR-001 (Roles & Permissions), FDR-020 (Server Branding &
  Configuration), FDR-031 (Client–Server Compatibility Discovery)
- **Issues:** [#1669](https://github.com/chattocorp/chatto/issues/1669),
  [#2208](https://github.com/chattocorp/chatto/issues/2208),
  [#2207](https://github.com/chattocorp/chatto/issues/2207),
  [#2206](https://github.com/chattocorp/chatto/issues/2206)
