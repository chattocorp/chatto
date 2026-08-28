# FDR-008: Signed-in Password Change

**Status:** Experimental
**Last reviewed:** 2026-08-20

## Overview

A signed-in person with a local Authling account can replace its password after
re-entering the current password. The account ID, verified email address, and
OpenID Connect `sub` remain unchanged.

## Behavior

- The account page links to password change. The form requires the current
  password, a distinct replacement password under the active password policy,
  and matching confirmation.
- Incorrect current passwords share Authling's distributed attempt limits and
  bounded password-verification capacity. Operational failures do not consume
  the guessing budget.
- A successful change records one durable, PII-free password-change fact bound
  to the exact credential that was reauthenticated. Concurrent password or
  email changes make an older reauthentication stale.
- Completion advances the account authentication version, invalidating every
  older Authling browser session across replicas and restarts. The completing
  browser receives a fresh session and remains signed in.
- The old password stops authenticating immediately after commit; the new
  password authenticates the same account.
- Already issued OIDC ID and access tokens are not revoked. They expire after
  five minutes, and relying-party sessions remain under relying-party control.

## Design Decisions

### 1. Require the current password

**Decision:** An active browser session alone cannot authorize password
rotation. The person must re-enter the current local password.

**Why:** A stolen unlocked browser should not be sufficient to establish a new
long-term credential.

**Tradeoff:** A signed-in person who no longer knows the password must use the
verified-email password-reset flow.

### 2. Record only the successful mutation durably

**Decision:** The successful `PasswordChangedEvent` identifies a signed-in
ceremony and the prior credential event. Failed guesses and form attempts are
not permanent events; current-password failures use bounded runtime state for
guessing controls.

**Why:** The committed credential mutation is security-relevant audit history.
Persisting attacker-controlled attempts would create an event-log growth vector
without improving account recovery.

**Tradeoff:** Authling does not yet expose a durable user-visible history of
failed reauthentication attempts.

### 3. Bind authorization to one credential generation

**Decision:** The command captures both the account and email-registry
boundaries, waits for the projection through them, then uses account-subject
OCC. It succeeds only while the credential whose password was checked remains
current. Unrelated audit-only events may advance the account tail and are
retried without weakening the credential precondition. The validated
replacement travels inside the transient command target so completion cannot
substitute another password.

**Why:** A password proven before another credential mutation must not
authorize a later overwrite. Waiting through the registry side of an atomic
email-change batch prevents password change from observing its staged account
event as though the older credential were still safe to mutate. The event
correlation makes historical replay validate the same relationship.

**Tradeoff:** A concurrent password or email change forces the person to sign
in again before retrying.

### 4. Invalidate older Authling sessions

**Decision:** Password change advances the durable authentication version.
Every older Authling browser session becomes invalid, while the completing
browser receives a replacement session bound to the exact new generation.

**Why:** Credential rotation is frequently a response to suspected compromise.
Generation checks revoke sessions across replicas and restarts without relying
on the enumerable session inventory introduced by FDR-009.

**Tradeoff:** Other trusted Authling browser sessions cannot be preserved
selectively in this initial feature.

### 5. Preserve external identity continuity

**Decision:** Password change preserves the account ID, verified email claim,
OIDC `sub`, already issued OIDC tokens, and relying-party sessions.

**Why:** The local password authenticates an existing identity; changing it is
not a new identity or a protocol-wide logout operation.

**Tradeoff:** A person cannot use password change to immediately revoke tokens
or sessions already held by relying parties. Token revocation and RP-initiated
logout remain separate protocol work.

## Security and Failure Behavior

- Passwords appear only in bounded same-origin POST bodies and transient
  in-process command data. They never enter URLs, logs, event payloads,
  subjects, or runtime keys.
- The durable event contains only opaque account, key, and event references,
  the ceremony kind, and the encrypted replacement verifier.
- Wrong-current-password, throttled, stale-credential, key-vault, projection,
  and storage paths fail closed. Infrastructure errors do not masquerade as a
  credential mismatch or consume the guessing budget.
- A failure after the event commits cannot restore the old password. If the
  completing browser cannot receive a replacement session, it must sign in
  with the new password.
- The POST endpoint requires Authling's canonical browser origin and uses a
  bounded request body.

## Compatibility

The persisted protobuf change is additive. Older readers already understand
`PasswordChangedEvent` and ignore its new prior-credential and ceremony fields;
new readers continue to accept historical events that leave them unspecified.
No event migration is required, and mixed-version readers materialize the same
credential and authentication-version change.

## Limitations

- Authling does not provide MFA recovery or user-visible authentication
  history.
- Existing OIDC tokens and relying-party sessions are not revoked.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md),
  [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md),
  [ADR-003](../adr/ADR-003-server-rendered-templ-ui.md)
- **FDRs:** [FDR-003](FDR-003-local-login-and-browser-sessions.md),
  [FDR-006](FDR-006-password-reset.md),
  [FDR-007](FDR-007-verified-email-change.md),
  [FDR-009](FDR-009-browser-session-management.md)
