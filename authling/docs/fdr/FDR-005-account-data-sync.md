# FDR-005: Account Data Synchronization

**Status:** Experimental
**Last reviewed:** 2026-08-02

## Overview

An authenticated Authling account has one small, durable TinyBase data space.
User devices can keep local state, work offline, and synchronize through
Authling without making one Chatto server the user's home server.

## Behavior

- A client connects to `GET /data/sync` with its Authling browser session.
- The WebSocket accepts only Authling's exact browser origin.
- The validated session selects the account. The client cannot supply an
  account ID or select another data space.
- Authling acts as the durable TinyBase peer. Devices do not need a direct
  connection to each other.
- TinyBase hybrid logical clock stamps resolve concurrent writes with
  last-writer-wins behavior. Deletion tombstones synchronize like other
  stamped changes.
- A device with existing local state can upload it after it connects. A new
  device can download state after Authling restarts.
- Connected devices on one Authling process receive live changes. JetStream
  optimistic concurrency protects writes from different Authling replicas.
- Every incoming and outgoing protocol operation revalidates the browser
  session. Logout or expiry closes access to the live data space.

## Storage and Data Protection

The complete stamped state is stored in the `AUTHLING_USER_DATA` JetStream KV
bucket. The KV key is a keyed digest and does not contain the account ID.

Each account data space uses a random `account-data` key. The key is wrapped by
the account user key and stored in `AUTHLING_KEYS`. The state is encrypted with
XChaCha20-Poly1305. Authenticated data binds the ciphertext to the opaque state
key, data-key reference, purpose, and envelope version.

An unreadable envelope, absent key, wrong key purpose, substituted ciphertext,
or storage failure is an error. Authling does not return an empty data space in
those cases.

## Protocol and Limits

The endpoint supports the TinyBase 9.3 synchronizer protocol through an
experimental three-item JSON envelope:

```text
[requestId, messageNumber, body]
```

The transport represents JavaScript `undefined` deletion values with the
private object `{"__authling_tinybase_undefined":true}`. This encoding is part
of the experimental transport, not an application document value.

Messages are limited to 288 KiB. Decrypted durable state is limited to 256
KiB. One Authling process accepts at most eight live connections for one
account and at most 64 pending peer requests. Binary frames, invalid message
shapes, clocks over five minutes in the future, and unsupported protocol
messages close the connection without changing durable state.

Authling currently sends complete table or value state when content hashes
differ. TinyBase's row and cell hash-tree optimization is not implemented.

## Limitations

- Only accounts with local protected credentials have the user key required
  for account data.
- There are no application namespaces, delegated OIDC data scopes, quotas,
  administration tools, or general document CRUD API.
- Live fanout is process-local. A connection on another Authling replica sees
  the durable winner when it next exchanges a protocol message or reconnects.
- TinyBase calls its synchronizer protocol experimental. Authling supports
  exactly TinyBase 9.3.0 and requires the pinned compatibility test for an
  upgrade.

## Related

- **ADR:** [ADR-005](../adr/ADR-005-tinybase-account-data-sync.md)
- **Data protection:** [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md)
