# ADR-005: Synchronize Account Data with a Durable TinyBase Peer

**Status:** Accepted

**Date:** 2026-08-02

## Context

Authling must store small user-controlled data that is independent of every
Chatto server. The first examples are a user's registered servers and client
preferences. The same data must work in a server-hosted frontend, a standalone
web client, and future desktop or mobile clients. Devices can be offline and
must converge when they reconnect.

TinyBase provides a local `MergeableStore`, hybrid logical clock stamps, and a
custom synchronization transport. A cross-language proof shows that a Go peer
can implement the TinyBase 9.3 protocol, retain deletion tombstones, merge
offline changes, and restore durable state after a restart.

TinyBase describes its synchronizer protocol as experimental. The proof also
uses a complete-state diff path. It is correct for small data spaces but is not
an unbounded document database design.

## Decision

Authling will provide one small account-owned TinyBase data space as its first
user-data feature.

### Client and protocol boundary

Clients use a TinyBase `MergeableStore` and connect to an authenticated
Authling WebSocket endpoint. Authling is the durable, always-online peer. Two
user devices do not need a direct connection.

Authling pins one supported TinyBase version. A cross-language compatibility
test is the protocol contract. A TinyBase upgrade requires explicit review and
must pass restart, offline, conflict, deletion, and invalid-message tests.

The first endpoint uses the TinyBase protocol through an Authling-owned wire
envelope. The endpoint is experimental. Authling does not promise that this
wire format is a permanent public API.

### Ownership and authorization

The data space belongs to one Authling account. The browser session authorizes
the WebSocket upgrade. The account ID comes only from the validated server-side
session. A client cannot select or name another account's data space.

The initial data space is global to the account. Application namespaces and
delegated OIDC access are outside this slice. They must be added before an
external application can receive access to application-scoped data. The
storage and authorization model must keep the global space distinct from later
application spaces. A validated server-side grant, not client input, must
select any later application namespace.

### Persistence and encryption

The complete stamped TinyBase state is stored in a dedicated Authling
JetStream KV bucket. Its storage key is a keyed digest of the account ID and
does not expose the public account ID.

Each data space uses a random data key wrapped by the account's user key. The
wrapped key record declares the `account-data` purpose. The encrypted state
envelope contains only an opaque data-key reference, nonce, ciphertext, and
format version. Authenticated data binds the ciphertext to its storage key,
data-key reference, purpose, and envelope version.

The opaque key reference and versioned envelope must permit more than one data
key generation. Authling may later offer opt-in, configurable user DEK
rotation without changing the TinyBase data model. Rotation policy,
re-encryption, retirement, recovery, and backup behavior require a separate
decision before implementation.

JetStream KV revision checks are the cross-replica write boundary. On a
conflict, a writer reloads the winner, merges the incoming TinyBase changes,
and retries. Process-local locks and WebSocket hubs are only optimizations.

### Bounds and failure behavior

The endpoint limits message size, state size, connection count per account and
per process, retained decrypted account spaces, and pending protocol requests.
It evicts the least recently used idle space under process-wide pressure. A
cold account load has a deadline and runs outside the global hub lock. It
accepts only the supported message shapes. Malformed input closes the
connection without changing durable state.

Missing sessions, missing accounts, missing keys, decryption failures, and
storage failures fail closed. Authling never substitutes an empty data space
for unreadable active state.

## Consequences

Users get local-first data that remains independent of Chatto servers. A
device can start with local data, reconnect after being offline, and converge
through Authling.

The first slice is intentionally small. Complete-state diffs make cost
proportional to the complete data space. Limits are therefore part of the
correctness and abuse boundary. Row and cell hash-tree optimization can be
added later without changing stored TinyBase stamps.

The design depends on an experimental upstream protocol. Pinning and contract
tests reduce upgrade risk but do not remove it. A future stable Authling API can
replace or version the wire boundary while retaining the account data model.

## Related

- [ADR-002: Protect User Data with Hierarchical Keys and Cryptographic Erasure](ADR-002-hierarchical-keys-and-cryptographic-erasure.md)
- [TinyBase durable peer proof](../experiments/tinybase-durable-peer.md)
