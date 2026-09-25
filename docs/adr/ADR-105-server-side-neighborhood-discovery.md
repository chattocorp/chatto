# ADR-105: Discover the Neighborhood on the Server

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

Every replica checks the directory once per minute. A pass is due when the
directory is missing or one hour old. It is also due after ten minutes when a
remote request failed in the previous pass, and after two minutes when the
Neighbor set changed. The `neighborhood-discovery` lease in `MEMORY_CACHE`
lets one replica run a pass at a time. The shared directory age sets the
cluster-wide rate. The replica checks again after it gets the lease.

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
  recommending server, not once per user visit.
- Results can be up to one hour old. A Neighbor change appears after about two
  minutes. A server that was unavailable during a pass can return after about
  ten minutes.
- A NATS restart removes the memory-backed directory. The next check starts a
  new pass. Backups exclude `NEIGHBORHOOD_IMAGES`.
- This supersedes FDR-042 Design Decision 3 for Neighborhood discovery. The
  Neighbor administration page still loads public profiles in the browser.
