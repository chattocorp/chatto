# TinyBase durable peer proof

This proof tests one design question: can a Go Authling service act as the
durable peer for TinyBase stores on user devices?

The answer is yes for the tested TinyBase version and data types.

## Tested design

- Each device owns a TinyBase `MergeableStore`.
- A custom TinyBase synchronizer sends protocol messages to a Go peer.
- The Go peer merges TinyBase hybrid logical clock (HLC) stamps.
- The peer saves the full stamped state through a small storage interface.
- A new peer process can load that state and continue synchronization.

The compatibility test pins TinyBase 9.3.0. It proves these cases:

1. Device A writes data.
2. The Go peer stops and restarts.
3. Device B receives the data from the restarted peer.
4. Device A receives a later write from device B.
5. Both devices edit one value while device A is offline.
6. The devices converge after device A reconnects.
7. A deletion tombstone reaches the other device.

Run the proof from the Authling directory:

```sh
mise test-sync-poc
```

## Deliberate limits

The original newline-delimited JSON transport remains private to the pinned
compatibility test. [FDR-005](../fdr/FDR-005-account-data-sync.md) applies the
proved merge behavior to Authling's authenticated WebSocket and encrypted
JetStream storage. Application namespaces, delegated access, and quotas remain
outside that first feature slice.

The peer sends the complete stamped table or value state when hashes differ.
This is valid protocol behavior, but it does not use TinyBase's row and cell
hash-tree optimization. It is suitable for a small proof, not for an unbounded
production data space.

TinyBase describes this synchronization protocol as experimental. Authling
must pin a supported TinyBase version and keep a cross-language compatibility
test. A TinyBase upgrade must pass that test before Authling can support it.

## Production follow-up

The implemented account data space binds the connection to the authenticated
account, adds input limits, encrypted JetStream persistence, and OCC merge
retries. Later work must add application access, quotas, cross-replica live
fanout, and a stable public compatibility boundary.
