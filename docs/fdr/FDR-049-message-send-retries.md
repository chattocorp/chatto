# FDR-049: Message Send Retries

**Status:** Experimental
**Last reviewed:** 2026-10-08

## Overview

Message send retries let clients recover from a timeout, disconnect, or lost
response without posting the same intended message twice.

## Behavior

- A caller can supply a UUID for one logical send. The key is optional and
  belongs to the authenticated account.
- Within a fixed 30-minute window, the same key and arguments return the same
  message ID, including concurrent attempts on different replicas.
- Different arguments with the same key fail without creating another message.
- A retry does not extend the window. After expiry, the key can be used again.
- Current read access controls a repeated result. Changes to write permissions
  do not make a completed post occur again. Edits and retractions retain its ID.
- The shared client offers a prepared send that keeps the key and uploaded
  attachment IDs. The bundled composer reuses it when an unchanged failed draft
  is sent again. A successful send releases it. When a retry fails because its
  window expired or connection data was reset, the composer releases it and
  preserves the draft. The composer
  asks the user to check whether the message arrived. Another Send click starts
  a new send; the composer does not post again automatically.
- Prepared sends stay in memory. They do not survive a client restart. A client
  stops retries at its 30-minute deadline and asks the caller to check the result.
- MCP tells a host when it can retry the same request. Without a key, an
  uncertain post still requires a result check.

## Design Decisions

### 1. Share the canonical message operation

**Decision:** ConnectRPC, MCP, and clients use the same retry contract.
**Why:** [ADR-115](../adr/ADR-115-message-post-idempotency.md) resolves concurrency
and recovery once, without adding caller-controlled data to EVT.
**Tradeoff:** All serving replicas must support the contract before clients can
rely on it.

### 2. Use a bounded collision window

**Decision:** Keep one send identity for 30 minutes without refreshing it.
**Why:** This covers interactive retries and bounds storage growth.
**Tradeoff:** A client must check an old uncertain send instead of retrying it
after the window. This is not a permanent external message identifier.

## Gates

- Existing message-post authorization controls a new write.
- Current room membership and message-read authorization control a retry result.
- The MCP message-write scope is required on each post call.

## Related

- **ADRs:** [ADR-115](../adr/ADR-115-message-post-idempotency.md),
  [ADR-107](../adr/ADR-107-keep-chat-data-out-of-device-storage.md).
- **FDRs:** [FDR-043](FDR-043-model-context-protocol-integration.md).
