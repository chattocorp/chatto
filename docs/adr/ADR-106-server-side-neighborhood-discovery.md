# ADR-106: Discover the Neighborhood on the Server

**Date:** 2026-09-25
**Status:** Accepted

## Context

FDR-042 kept the server passive. The server stored Neighbor origins, and the
browser contacted each advertised server to load its directory, profile, logo,
and banner. Each contacted server could see the user's IP address. The Server
Directory therefore asked for consent before its first discovery request. The
browser also had to schedule and limit a recursive crawl for each page session.

Each registered server already knows the user's IP address. When a registered
server contacts other servers for the user, no additional server sees that
address.

## Decision

Each Chatto server discovers its own Neighborhood in the background and
publishes the cached result through the public
`ServerDiscoveryService.ListNeighborhoodServers` RPC. A call to this RPC reads
only the cache. It never starts a remote request. A caller cannot supply a URL,
so the RPC is not a general proxy.

A discovery pass applies the FDR-042 rules on the server:

- It starts from the server's own Neighbors and lists each direct Neighbor
  whose public profile loads.
- It reads a direct Neighbor's directory. It expands that Neighbor only when
  the Neighbor advertises the server back.
- It lists a recommended server when that server advertises the recommending
  server back. It follows at most two mutual hops.
- It removes duplicate canonical origins and records every mutual recommender.

One pass permits at most 150 directory requests and 120 profile requests,
with six active requests and a ten-second timeout for each request. It reads
at most 100 origins from one remote directory and ignores advertised origins
longer than 300 bytes. It truncates the stored name, version, and description.

The server uses the link-preview HTTP client. That client checks each resolved
address when it connects and rejects loopback, private, and link-local
addresses. Discovery rejects redirects. It accepts a logo or banner only from
the advertised origin, only with a declared GIF, JPEG, PNG, or WebP media type,
and only up to 5 MiB. The server decodes the image with the normal bounded
image decoder and stores a re-encoded WebP copy.

The latest directory is one `cache_state.v1.NeighborhoodDirectory` value under
`neighborhood.directory` in `MEMORY_CACHE`. Image copies are content-addressed
objects in the `NEIGHBORHOOD_IMAGES` object store. The object store has a
seven-day TTL. A pass rewrites an image that it still uses when the image is
older than three days. Unused images expire without a cleanup pass. The public
`/assets/neighborhood/{sha256}` route serves only names from this store.

Every replica runs one worker that checks the directory every five seconds. A
check reads the directory and compares a hash of its Neighbor set with the
local Neighbor projection. A pass is due when the directory is missing or one
hour old. It is also due after ten minutes when a remote request failed in the
previous pass, and after ten seconds when the Neighbor set changed. Every
replica sees a Neighbor change through its projection, so no change signal is
necessary. After a failed check, the worker waits one minute, because a failed
write can follow a complete remote crawl.

As an interim design, the replicas do not coordinate their passes. When a pass
is due, each replica can run it and write an equivalent directory. A later
shared job queue can replace this worker.

## Consequences

- A client can show the Neighborhood without a consent step and without
  contacting unregistered servers. Joining a server still contacts that server
  because the user starts that action.
- The server now makes outbound requests to its Neighbors and their mutual
  recommendations. Operators who block outbound traffic get an empty or
  partial Neighborhood.
- Discovery rejects servers on private network addresses. A Neighborhood of
  internal servers is not available through this RPC.
- A remote server receives requests once per discovery pass from each
  recommending server, not once per user visit. A deployment with several
  replicas can send these requests once from each replica.
- Results can be up to one hour old. A Neighbor change appears within about
  fifteen seconds. A server that was unavailable during a pass can return after
  about ten minutes.
- A NATS restart removes the memory-backed directory. The next check starts a
  new pass. Backups exclude `NEIGHBORHOOD_IMAGES`.
- The bundled Server Directory merges the Neighborhoods of all registered
  servers and removes its browser crawl and consent prompt. It accepts cached
  images only from the registered server that supplied them.
- This supersedes FDR-042 Design Decision 12 and partially supersedes Design
  Decision 3. The Neighbor administration page still loads public profiles in
  the browser.
