# ADR-051: Home Server for Portable Client Sync

**Date:** 2026-07-15

## Context

Chatto's multi-server client can run from a generic web host, desktop shell, or
mobile shell. Preferences such as language, timezone, and time format belong to
the person using that client rather than to whichever server happens to serve
the frontend. The client also needs a portable list of the servers the person
has joined. Browser storage provides an offline cache but cannot restore this
state on another device.

Asking for a separate sync-service URL would expose infrastructure choices in
ordinary onboarding. Writing personal settings to `EVT` would turn
latest-value private state into permanent domain history. A new JetStream KV
bucket would add operational cost for two small documents per user.

## Decision

A user may designate one normally registered Chatto server as their **home
server**. A server is eligible only when the user is authenticated and public
discovery advertises client-sync support. A fresh client automatically chooses
the sole eligible server; with multiple eligible servers, the user chooses
explicitly. With none, settings remain device-local. Home is visible in the
server gutter and cannot be removed until the user moves home.

Client sync is operator opt-in and defaults to disabled. Enabling
`[client_sync].enabled` (or `CHATTO_CLIENT_SYNC_ENABLED`) both advertises the
capability through `ServerDiscoveryService.GetServer` and permits authenticated
calls to the service. Disabled servers return `UNIMPLEMENTED` defensively.

The home server exposes an authenticated `ClientSyncService`. It stores two
no-TTL protobuf documents and a retained deletion marker in the existing
backed-up `RUNTIME_STATE` bucket:

- `client_sync.{userId}.preferences`
- `client_sync.{userId}.servers`
- `client_sync.{userId}.deleted`
- `client_sync.{userId}.deletion_pending`

The preferences document contains portable language, timezone, and time-format
choices. The server-directory document contains public server metadata and the
home-server ID. Passwords, sessions, bearer tokens, and other credentials never
enter client sync; restored server entries require authentication on the new
device. Theme and notification-sound shaping remain device-local even though
their controls live in the global settings screen.

Documents are intentionally coarse storage units. Public mutations remain
fine-grained: preference updates use field masks and server-directory entries
use resource operations. The owning core service reads the current protobuf,
applies the requested mutation, and writes with a JetStream KV revision. It
retries revision conflicts so replicas and concurrent clients cannot silently
replace each other's changes. A directory is limited to 100 servers to bound
the size and rewrite cost of its single KV document.

Persistence and public transport contracts use dedicated packages instead of
expanding the event or general API packages:

- `chatto.clientsync.v1` for stored protobuf messages
- `chatto.clientsync.api.v1` for public ConnectRPC resources and services

The existing `chatto.api.v1.MyAccountService.UpdateSettings` timezone and time
format surface remains available for mixed-version clients. New clients adopt
those values when a home server has no personal preferences yet, then use the
client-sync API.

The portable identity of a known server is its canonical URL origin, not a
client-generated sidebar ID. A client may refresh the name and icon for an
already registered origin, but synced metadata must never replace that local
origin or move its credentials. Local IDs remain device-local implementation
details.

Each client records the last successfully synchronized directory as a local
baseline scoped to the home origin and authenticated user ID. Personal caches
use the same scope. A home origin is bound to that account until the person
signs out of all servers, preventing another account on a shared device from
receiving the previous account's cached preferences or directory. The first
sync with a home server unions both directories, while a later absence relative
to that baseline is treated as a deletion.

Moving home unions the destination directory before writing the current
preferences and home marker. It also updates the former home's directory to
point at the new home, allowing other devices still anchored there to follow
the move. If the former home is temporarily unavailable, the client retains a
device-local pending move and retries while it still has authenticated access.
Pending moves retain and verify the former home account ID, and newer moves
collapse older redirect chains so retries cannot create cycles.
This avoids destructive first-sync and home-move overwrites without
adding tombstones to the first protocol slice. Within each portable directory,
canonical server origins are unique.

## Consequences

People get a natural sync location by choosing from servers they already use,
and standalone clients can restore preferences and their server list without a
vendor account. A server backup includes this client sync because it includes
`RUNTIME_STATE`.

The home server becomes a small availability dependency for sync. Clients must
continue using their local cache while it is offline. Older servers and servers
whose operators do not opt in simply remain ineligible for home selection.
Moving home copies current client state to the new server; automated
server-to-server transfer and identity federation remain future work.

The client-local directory baseline means deletion convergence depends on a
device having completed an earlier sync. Durable cross-device tombstones may be
added later if real-world conflict behaviour requires them.

Server URLs can reveal community membership. Operators already control
`RUNTIME_STATE`, which contains other private authenticated runtime records;
access to this API is strictly limited to the authenticated owner and code must
not log directory contents. Account deletion retains a marker before purging
both plaintext client-sync records. An exclusive, timestamped preparation
marker blocks mutations before the account event; after commit, the command
creates the retained deletion fence and uses the preparation as its retry
marker. Recovery starts only after projections catch up, completes confirmed
deletions, and revision-purges active-account preparations stale for one hour.
Failed commands roll back only their own preparation revision with a bounded,
non-cancelled context. Deletion therefore wins over in-flight requests and
transient failures without letting cold or pre-commit recovery erase active
data or repeatedly traversing every historical committed fence.
