# FDR-009: Browser Session Management

**Status:** Experimental
**Last reviewed:** 2026-08-20

## Overview

A signed-in person can review the active Authling browser sessions for their
account, identify the current browser, sign out one other browser, or sign out
all other browsers. Authling deliberately does not collect browser names, IP
addresses, or locations to make the list more descriptive.

## Behavior

- The account page lists every currently valid first-party browser session for
  the signed-in account. The current browser appears first and is labelled
  explicitly.
- Each row shows when the session started, when it was last active, and its
  absolute expiry. Activity can be several minutes behind because session
  touches are deliberately rate-limited.
- A person can sign out one other browser or all other browsers. The current
  browser cannot be revoked through these controls; its existing Sign out
  action remains the explicit way to end it.
- A revoked browser fails its next authenticated request. Revoking Authling
  browser sessions does not revoke already issued OIDC tokens or sessions held
  by relying parties.
- Session-management forms require Authling's canonical browser origin and a
  valid current session. Missing, expired, foreign-account, forged, and already
  revoked session identifiers do not disclose another account's state.
- Session inventory survives Authling process restarts by rebuilding from
  authoritative runtime state. It is not a durable authentication history.

## Design Decisions

### 1. Materialize one process-wide account index

**Decision:** Each Authling process maintains one filtered watcher over browser
session records in `AUTHLING_RUNTIME_STATE`. It materializes account-to-session
and session-key reverse indexes in memory. The encrypted KV records remain the
only authoritative session state.

**Why:** Bearer-derived session keys are intentionally non-enumerable by
account. A single watcher makes account-scoped reads cheap without request-time
bucket scans, repeated decryption, per-request watchers, or a second persisted
source of truth. Replaying the latest KV value for each key reconstructs the
entire disposable index after restart.

**Tradeoff:** Every replica holds metadata for all live browser sessions. The
index is bounded by the 24-hour absolute session lifetime, and Authling must
replay it before becoming ready.

### 2. Keep public session IDs separate from bearers and storage keys

**Decision:** Account-page forms use an opaque `ses_` identifier derived with a
domain-separated keyed digest of the private session storage key. The browser
bearer and bearer-derived KV key never enter page content or form values.

**Why:** A session-management identifier should be safe to render and submit
without becoming a credential or exposing JetStream coordinates. Derivation
also gives existing runtime records an identifier without changing their
encrypted storage schema.

**Tradeoff:** The identifier is meaningful only to the Authling deployment that
derived it. That is intentional: sessions do not migrate independently between
issuers or key hierarchies.

### 3. Re-authorize every revocation against runtime state

**Decision:** The in-memory index selects a candidate, but revocation re-reads
and decrypts its authoritative KV record, confirms the signed-in account owns
it, and deletes it with optimistic concurrency control. The service waits for
the deletion revision to reach its local watcher before returning.

**Why:** A disposable index can briefly lag or contain a record that changed
after selection. Authoritative re-authorization prevents stale projection
state or a forged opaque ID from crossing an account boundary. OCC prevents a
concurrent activity touch from resurrecting a revoked session, while the
watcher wait makes the refreshed account page read its own write.

**Tradeoff:** Signing out all other browsers can partially progress if storage
fails mid-operation. Retrying is safe and converges on the desired result.

### 4. Minimize session metadata

**Decision:** The inventory exposes only lifecycle timestamps and whether a
session is current. Authling does not retain user agents, device labels, raw IP
addresses, inferred locations, or a durable login history for this feature.

**Why:** Device and network metadata is personal data, is often inaccurate,
and is not required for authoritative revocation. A deliberately modest list
provides the security control without expanding Authling's identity-data
footprint.

**Tradeoff:** People must distinguish other sessions by their timestamps. A
future device-description feature would require a separate privacy and
retention decision.

## Security and Failure Behavior

- The account boundary comes from the authenticated current session, never
  from a submitted account ID.
- Opaque session IDs are compared only within that account's inventory, then
  checked again against decrypted authoritative state.
- The current session is identified from its bearer-derived internal key and
  cannot be ended through a modified remote-revocation form.
- Malformed or undecryptable runtime records cannot authenticate and are
  omitted from the inventory. A watcher startup failure prevents readiness; a
  later watcher failure terminates the required runtime model.
- Session creation, activity touches, logout, and remote revocation wait for
  their own KV revisions when the inventory is running.
- No browser bearer, public session ID, internal KV key, account identifier,
  timestamp, IP address, or user agent is written to logs by this feature.

## Compatibility

The encrypted session-record format and cookie format are unchanged. Existing
sessions receive deterministic account-page identifiers when a new process
indexes them, and older Authling replicas continue validating the same records.
All replicas observe authoritative KV deletions even when they do not expose
the new account-page controls. No migration is required.

## Limitations

- Session rows do not identify a browser, device, IP address, or location.
- Authling does not provide durable login history or record remote-revocation
  audit events.
- OIDC token revocation and relying-party logout remain separate protocol
  features.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md),
  [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md),
  [ADR-003](../adr/ADR-003-server-rendered-templ-ui.md)
- **Features:** [FDR-003](FDR-003-local-login-and-browser-sessions.md),
  [FDR-006](FDR-006-password-reset.md),
  [FDR-007](FDR-007-verified-email-change.md),
  [FDR-008](FDR-008-signed-in-password-change.md)
