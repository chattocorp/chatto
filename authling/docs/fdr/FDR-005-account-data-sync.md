# FDR-005: Account Data Synchronization

**Status:** Experimental
**Last reviewed:** 2026-08-02

## Overview

An authenticated Authling account has one small, durable TinyBase data space.
User devices can keep local state, work offline, and synchronize through
Authling without making one Chatto server the user's home server.

## Behavior

- A client connects to `GET /data/sync` with its Authling browser session.
- The original WebSocket mode accepts only Authling's exact browser origin.
  Its validated session selects the account.
- An OIDC client can request `openid account_data`. After explicit consent, it
  can connect from its exact callback origin with the
  `authling.account-data.v1` WebSocket subprotocol and authenticate with its
  five-minute access token.
- Authling derives the account from the validated session or token. The client
  cannot supply an account ID or select another data space.
- Authling acts as the durable TinyBase peer. Devices do not need a direct
  connection to each other.
- Chatto's bundled frontend is the first external consumer. It stores public
  registered-server fields: stable local ID, immutable origin, display name,
  icon URL, and registration time. Chatto credentials and per-server user
  details remain local to each device.
- The frontend's own trusted bootstrap configuration selects the Authling
  issuer and frontend CIMD client. A connected Chatto server cannot redirect
  global account data to another issuer. Chatto server login uses a separate
  CIMD client and authorization.
- The Chatto frontend reads `sub` from Authling UserInfo and presents this as
  its global Authling session. It can show synchronized signed-out servers and
  starts a server's normal device-local OAuth sign-in when the user selects its
  sidebar icon. A server provider is preferred only when its advertised issuer
  exactly matches the frontend's trusted issuer.
- The Chatto frontend retains the five-minute account-data access token in
  browser-local storage across reloads, tabs, and browser restarts. It reuses
  the token after transport closure, but removes expired, malformed, or
  issuer/client-mismatched grants.
- The Chatto frontend persists its mergeable stamps locally. A new device
  downloads the server list, while an existing device can make offline changes
  without replacing deletion history with fresh timestamps on every reload.
- Chatto's all-server sign-out disconnects account-data synchronization and
  clears the frontend's Authling grant and local TinyBase cache before it
  clears the Chatto registry. It does not synchronize those local removals to
  Authling, so the durable server list remains available after a later Authling
  sign-in. Authling's own browser SSO session is separate until RP-initiated
  logout is implemented.
- TinyBase hybrid logical clock stamps resolve concurrent writes with
  last-writer-wins behavior. Deletion tombstones synchronize like other
  stamped changes.
- A device with existing local state can upload it after it connects. A new
  device can download state after Authling restarts.
- Connected devices on one Authling process receive live changes. JetStream
  optimistic concurrency protects writes from different Authling replicas.
- Every incoming and outgoing protocol operation revalidates its browser
  session or access token. Authling also checks an idle connection every 30
  seconds. Session logout, session expiry, or token expiry closes access.

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

After authentication, the endpoint supports the TinyBase 9.3 synchronizer
protocol through an experimental three-item JSON envelope:

```text
[requestId, messageNumber, body]
```

The transport represents a complete cell or value that is JavaScript
`undefined` with TinyBase's reserved `U+FFFC` string. The transport decodes
this marker only at protocol leaf positions. A `U+FFFC` string nested inside a
JSON object or array remains application data.

Messages are limited to 288 KiB. Each account has one process-local token
bucket shared by all its connections. It allows a burst of 32 messages and
refills at eight messages per second. The bucket and idle peer remain for five
minutes after the last connection closes, so reconnecting does not reset the
limit or force repeated cold storage loads. Decrypted durable state is limited
to 256 KiB. Every synchronization-boundary
content-hash request refreshes durable state. A second shared bucket allows
four such requests and refills at one per second. The peer checks JetStream at
most once per second during other ordinary message handling. A process-local
revision cache avoids repeated key resolution and decryption when JetStream
still has the same revision. OCC conflicts force an immediate refresh. One
Authling process accepts at most eight live connections for one account and at
most 256 live connections across all accounts. It keeps at most 128 decrypted
account spaces per process. At that limit it evicts the least recently used
idle space before loading another one. It rejects a new space when every
retained space is active. A cold load has a five-second deadline and does not
hold the process-wide hub lock, so slow storage for one account does not stop
other accounts from connecting. Each connection allows at most 64 pending peer
requests. Binary
frames, invalid message shapes, clocks over five minutes in the future,
rate-limit violations, and unsupported protocol messages close the connection
without changing durable state.

Token clients must first send an 8 KiB or smaller JSON authentication message.
Authling allows at most 64 pending token authentications per process and gives
each one two seconds. One direct network source can hold at most eight of those
slots. Authling applies this admission only after a successful WebSocket
upgrade. One source therefore cannot consume the full pool, and malformed HTTP
requests do not consume it. A configured trusted proxy must replace
`X-Forwarded-For` with one client address; only then does Authling use that
address as the source. Authling binds the token to the account, client, granted
scope, expiry, and exact callback origin. It revalidates this authority for
protocol messages and at least every 30 seconds while idle.

The encrypted payload records Authling state format version 1 and exact
TinyBase protocol version 9.3.0. Authling rejects unknown versions or invalid
stored clocks and values. It does not silently reset them.

Authling currently sends complete table or value state when content hashes
differ. TinyBase's row and cell hash-tree optimization is not implemented.

## Limitations

- Only accounts with local protected credentials have the user key required
  for account data.
- There are no application namespaces, quotas, administration tools, or
  general document CRUD API.
- Live fanout is process-local. A connection on another Authling replica sees
  the durable winner when it next exchanges a protocol message or reconnects.
- TinyBase calls its synchronizer protocol experimental. Authling supports
  exactly TinyBase 9.3.0 and requires the pinned compatibility test for an
  upgrade.

## Related

- **ADR:** [ADR-005](../adr/ADR-005-tinybase-account-data-sync.md)
- **Delegated access:** [ADR-006](../adr/ADR-006-oidc-authorized-account-data.md)
- **Data protection:** [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md)
