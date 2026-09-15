# FDR-013: Account Deletion

**Status:** Experimental
**Last reviewed:** 2026-09-15

## Overview

A signed-in user can permanently close a local Authling account. Authling ends
access first, then destroys the account keys in the background. This makes the
account's encrypted history unreadable in live storage.

## Behavior

- The account page links to a separate deletion page. The page requires the
  current password and explicit confirmation. Password checks use the existing
  login attempt limits.
- Accepted deletion ends access from all Authling browser sessions. Pending
  recovery, identity changes, consent, code exchange, and UserInfo cannot grant
  access to the deleted account.
- Background key erasure resumes after failures and restarts. The result page
  confirms that account access has ended; it does not claim that background
  work has already finished.
- The email address becomes available for signup. A new account gets a new
  account ID and OIDC `sub`; it cannot recover the deleted account or its grants.
- Deletion does not close accounts or sessions in relying-party apps. It does
  not remove data already sent to those apps. An app can still accept an
  already-issued ID token until that token expires.
- Opaque IDs, event types, timestamps, key references, client digests, scopes,
  and encrypted event payloads remain in history. The account's email,
  password verifier, profile, and client display metadata become unreadable
  after live key erasure.
- Encrypted short-lived verification records expire separately, normally
  within 15 minutes of their creation. Browser-session records expire within
  24 hours; access is denied immediately after the deletion boundary. OIDC
  runtime records also keep their existing bounded lifetimes.
- Backup copies and external key exports follow the operator's retention
  policy. Restoring an old data-and-key backup can restore a deleted account.
  Live deletion cannot remove those external copies.

## Design Decisions

### 1. Deny access before destroying keys

**Decision:** Make account closure durable before background key erasure.
**Why:** A crash must not leave unreadable history without a deletion record.
This follows [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md).
**Tradeoff:** Key destruction can remain pending during a storage outage.

### 2. Release the email, preserve the deleted identity

**Decision:** Release the address with account closure and never reuse the
account ID.
**Why:** A later signup must not inherit another identity's app access.
**Tradeoff:** Users must manage old app accounts directly with those apps.

### 3. State the erasure boundary

**Decision:** Promise live account-key erasure and immediate Authling denial.
Describe independent workflow, relying-party, and backup retention explicitly.
**Why:** Authling cannot remove data or keys held outside this boundary.
**Tradeoff:** Operators still need a key-backup retirement policy before launch.

## Gates

- An active local-account session, a current password proof, and explicit
  confirmation are required. Cross-origin submissions are rejected.
- All running replicas must support account erasure before it is enabled by
  this release. Older binaries cannot replay the new event variants. Authling
  has no deployed installation to migrate; no compatibility path is added.

## Related

- **ADR:** [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md)
- **FDRs:** [Browser sessions](FDR-003-local-login-and-browser-sessions.md),
  [Authorization grants](FDR-010-oidc-authorization-grants.md),
  [Account profile](FDR-011-account-profile.md)
