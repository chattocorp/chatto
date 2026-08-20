# FDR-007: Verified Email Change

**Status:** Experimental
**Last reviewed:** 2026-08-20

## Overview

A signed-in person with a local Authling account can replace its verified email
address after re-entering the current password and proving control of the new
mailbox. The account ID and OpenID Connect `sub` remain unchanged.

## Behavior

- The account page links to email change. Starting the flow requires a valid
  Authling browser session, the current local password, and a syntactically
  valid address different from the current normalized address. Password checks
  share Authling's distributed attempt limits and bounded Argon2 capacity.
- Authling records a PII-free `EmailChangeRequestedEvent` after successful
  reauthentication and before it creates the flow or sends email. The event
  identifies only the account and credential version that was reauthenticated.
- Authling sends a six-digit code to the requested address. The encrypted flow
  and code expire after 15 minutes, five incorrect attempts exhaust the code,
  and delivery and completion work are bounded.
- Multiple flows may coexist for one credential. The first committed identity
  mutation makes every other flow for that older credential stale.
- A claimed address follows the same code-delivery and verification path as an
  available address. Completion returns the same expired-or-unavailable result
  used for a stale flow and does not reveal the conflicting account.
- Completion appends an encrypted `EmailChangedEvent` to the account and a
  PII-free `EmailClaimedEvent` to the global registry in one atomic batch. OCC
  guards both the account and registry tails so only one concurrent claimant
  can win.
- The old address remains the active login until the atomic completion commits.
  Afterward, only the new normalized address authenticates the account.
- Completion advances the account authentication version, invalidating every
  older Authling browser session across replicas and restarts. The completing
  browser receives a fresh session.
- Authling attempts a security notification to the old address after commit.
  Notification failure does not roll back the identity change and is shown to
  the completing browser.
- Already issued OIDC ID and access tokens are not revoked. They expire after
  five minutes, and relying-party sessions remain under relying-party control.

## Design Decisions

### 1. Require both reauthentication and new-mailbox control

**Decision:** A valid browser session is insufficient by itself. Starting the
flow verifies the current local password, and completion requires the emailed
one-time code.

**Why:** Email is both the login identifier and recovery channel. Requiring two
independent proofs reduces the impact of a stolen unlocked browser or a leaked
password alone.

**Tradeoff:** A person who has lost access to both the current password and the
signed-in session cannot use this flow. Password reset remains separate and
delivers only to the currently verified address.

### 2. Keep addresses encrypted and claims PII-free

**Decision:** The request event contains no address. `EmailChangedEvent`
contains the replacement address only as authenticated ciphertext under the
existing credential data key. Its authenticated metadata binds the account,
key hierarchy, accepted request, and prior credential. The adjacent registry
event contains opaque account and credential-event IDs only.

**Why:** Durable audit and uniqueness do not require exposing login identifiers
in stream subjects or plaintext event payloads.

**Tradeoff:** Historical encrypted addresses remain decryptable to a live
Authling process with key-store access until erasure-aware replay and key
retirement are implemented.

### 3. Claim and activate atomically

**Decision:** Completion writes the account change and registry claim as one
adjacent atomic batch guarded by both account-subject and registry-subject OCC.
The projector stages the encrypted account event and activates it only when it
applies the correlated registry event.

**Why:** The new address must never become active without its uniqueness claim,
and two replicas or accounts must not claim the same normalized address.

**Tradeoff:** The global registry remains a serialization point for email
claims. Email changes are low-volume security operations, so clarity and
correctness outweigh claim-write throughput.

### 4. Bind the flow to the reauthenticated credential

**Decision:** The flow records the credential event current during password
verification. The model retains a bounded set of accepted email-change request
IDs so replay can validate the request and credential correlation. A password
or email change makes every flow for the older credential stale; unrelated
audit-only events may advance the tail without invalidating a flow.

**Why:** An old password must not authorize an email change after a concurrent
credential mutation, and competing verified flows must not overwrite or bypass
the request correlation replay validates.

**Tradeoff:** The replay model retains up to 4,096 request correlations per
account. This exceeds the number allowed by the global 15-minute delivery
budget while preventing abandoned audit requests from growing memory without
bound.

### 5. Revoke browser sessions by durable generation

**Decision:** A committed email change advances the same durable account
authentication version used by password reset. Every older Authling session
fails validation, while the completing browser receives a new session.

**Why:** Changing the login and recovery address is a takeover-sensitive
operation. Durable generation checks work across replicas and process restarts
without a process-local session index.

**Tradeoff:** Authling cannot selectively preserve other browser sessions in
this initial flow.

### 6. Notify the previous address after commit

**Decision:** Authling attempts a plain security notification to the old
address only after the new claim commits. Delivery failure is reported to the
completing browser but does not undo the committed change.

**Why:** The previous mailbox owner should receive a takeover signal, while an
SMTP failure cannot safely reverse a durable identity mutation after its new
claim may already be observed.

**Tradeoff:** Notifications are not yet durable effects. Completion recovery
may retry a notification after an ambiguous crash, so the old mailbox can
receive a duplicate. Recovery is browser-driven, so a crash can still lose the
best-effort effect when no completion retry reaches the restarted process.
Durable operational effects remain future work.

## Security and Failure Behavior

- Flow bearers and OTPs appear only in POST form fields and encrypted runtime
  state, never in URLs, logs, event payloads, subjects, or runtime keys.
- Raw old and new addresses are retained only in authenticated-encrypted event
  or runtime records and otherwise exist transiently at in-process command and
  email-delivery boundaries.
- Claimed, stale, replayed, wrong-code, expired, and cross-account flow use fail
  closed without disclosing another account.
- A failed verification, failed delivery, or abandoned flow leaves the old
  address authoritative. A failure after the atomic completion cannot restore
  the old address.
- Completion attempts acquire a 45-second OCC-backed lease. Lease acquisition,
  identity mutation, notification, and cleanup use explicit deadlines whose
  combined maximum remains below that lifetime, so another replica cannot
  overlap a live owner while a crash remains retryable. Recovery and the
  replacement browser session are both bound to the exact authentication
  generation produced by the email change; a later password or email mutation
  invalidates them.
- All POST endpoints require Authling's canonical browser origin and use
  bounded request bodies.

## Compatibility

The protobuf additions are additive persisted fields and event variants, and
historical events replay without migration. Older Authling binaries reject the
new event variants rather than ignoring identity changes. During the current
experimental phase, operators running multiple Authling application replicas
must upgrade or stop every old replica before enabling traffic that can perform
email change; mixed old/new application replicas are not supported for this
event rollout.

## Limitations

- Authling does not yet provide selective session preservation, signed-in
  password change, MFA recovery, administrator recovery, or a user-visible
  authentication history.
- The security notice to the old address is best-effort and has no durable
  retry worker.
- Existing OIDC tokens and relying-party sessions are not revoked.

## Related

- [FDR-002: Verified Email Signup](FDR-002-verified-email-signup.md)
- [FDR-003: Local Login and Browser Sessions](FDR-003-local-login-and-browser-sessions.md)
- [FDR-006: Password Reset](FDR-006-password-reset.md)
- [Runtime architecture inventory](../architecture/INDEX.md)
