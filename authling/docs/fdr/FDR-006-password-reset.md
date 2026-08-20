# FDR-006: Password Reset

**Status:** Experimental
**Last reviewed:** 2026-08-14

## Overview

Authling lets a person who controls a local account's verified email address
replace its password through a short-lived email-code flow. The account ID,
email claim, and OpenID Connect `sub` remain unchanged.

## Behavior

- The sign-in page links to password reset. A person submits an email address,
  enters the six-digit code from the email, and chooses a new password under
  the same policy used by signup.
- Every syntactically valid email address follows the same flow and email
  delivery path whether or not it currently identifies an account. Browser
  copy does not disclose account existence.
- After rate limits accept a request for an existing account, Authling commits
  a `PasswordResetRequestedEvent` before creating the flow or sending email.
  The event contains only the account ID and current credential event ID; its
  envelope ID is the opaque recovery request identifier. Requests for absent
  accounts cannot produce an account-scoped event.
- A flow and its code expire after 15 minutes. Five incorrect code attempts
  exhaust the challenge. Delivery is limited per normalized address and
  globally, and SMTP and password-completion concurrency are bounded.
- A flow is bound to the exact password credential current when its code was
  issued. A concurrent password reset that commits first makes every older
  flow stale; a stale flow cannot overwrite the newer password.
- Completing a reset appends a durable `PasswordChangedEvent` linked to its
  `PasswordResetRequestedEvent`. It preserves the account's stable ID and
  credential key hierarchy while replacing the encrypted Argon2id verifier.
- A successful reset advances the account's durable authentication version.
  Authling browser sessions created under an older version stop validating,
  including after a process restart. The completing browser receives a new
  session and continues to its account or its interrupted OIDC consent request.
- Previously issued OIDC ID and access tokens are not revoked. They expire
  after five minutes, and relying-party sessions remain under the relying
  party's control.

## Design Decisions

### 1. Separate auditable facts from recovery secrets

**Decision:** An accepted request for an existing account and its resulting
password change are durable events. The request event is mandatory before flow
creation or email delivery, and the change references its request event ID.
One-time-code digests and attempt state remain authenticated-encrypted runtime
records under non-reversible keys. The submitted email is not retained in the
event or flow after delivery.

**Why:** The fact that recovery was initiated is a security-relevant audit
signal. The code, bearer, and attempt mechanics are expiring coordination that
must not become permanent identity history.

**Tradeoff:** A request event says that Authling accepted the request, not that
SMTP delivery succeeded. Incorrect-code attempts are not durable audit events;
recording attacker-controlled attempts permanently would create an event-log
growth vector. Losing runtime state cancels the in-progress flow, which the
person must restart.

### 2. Do not reveal whether an address is registered

**Decision:** Authling sends the same reset message for claimed and unclaimed
valid addresses, presents the same code form, and reveals no account state
until after control of the submitted mailbox has been demonstrated.

**Why:** Different responses or an omitted email would turn recovery into an
account-enumeration endpoint.

**Tradeoff:** An unclaimed address can receive an unsolicited but harmless
reset code, and its verified flow cannot complete.

### 3. Bind completion to the observed credential

**Decision:** The encrypted flow records the account, credential event, and
request event IDs observed at start. Completion compares the credential
binding with the current projection and appends against the observed
account-subject tail using OCC. Audit-only reset requests advance that tail but
do not stale a flow whose credential remains current.

**Why:** Two independently verified recovery flows must never overwrite each
other according to completion timing after one has already changed the
credential.

**Tradeoff:** A person must restart an older flow after any intervening
password change.

### 4. Invalidate Authling sessions by durable generation

**Decision:** Every password change advances an account authentication version.
Browser sessions record the version at issuance and fail closed when it no
longer matches.

**Why:** Reset is frequently a response to suspected credential compromise.
Generation checks revoke every pre-reset Authling browser session without a
session index or process-local coordination and remain effective after restart.

**Tradeoff:** There is no selective preservation of another browser session.
The completing browser is deliberately reauthenticated with a new session.

### 5. Reuse the credential data key

**Decision:** `PasswordChangedEvent` encrypts its verifier with the local
credential's existing data key and new event-specific AAD.

**Why:** Authling cannot destroy the prior key while historical account events
still require it during replay. Adding a new key would not erase the older
verifier and would increase key-management complexity without improving the
current erasure guarantee.

**Tradeoff:** Historical password verifiers remain decryptable to a live
Authling process with key-store access until erasure-aware replay and key
retirement are implemented.

## Security and Failure Behavior

- Flow tokens and OTPs never appear in URLs or logs. The browser carries the
  opaque flow token only in form fields.
- Request audit events contain no email, OTP, flow token, IP address, user
  agent, or other recovery material. A successful change carries only the
  opaque request-event reference needed to correlate the audit trail.
- Flow state and password verifiers are authenticated-encrypted with distinct,
  versioned AAD domains. Ciphertext cannot be moved between creation and change
  events or between credentials without failing authentication.
- Wrong, repeated, expired, absent-account, and stale-flow completion paths are
  bounded and fail closed. Storage, projection, key-vault, or email failures do
  not masquerade as success.
- All reset POSTs require Authling's canonical browser origin and have bounded
  request bodies.

## Limitations

- Authling does not currently support signed-in password change, email-address
  change, MFA recovery, recovery codes, administrator recovery, or user-visible
  authentication history.
- Password reset does not revoke already issued OIDC tokens or sessions held by
  relying parties. Token revocation and RP-initiated logout are separate future
  protocol work.
- The built-in password blocklist is intentionally small and is not yet a
  maintained compromised-password corpus.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md),
  [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md),
  [ADR-003](../adr/ADR-003-server-rendered-templ-ui.md)
- **Features:** [FDR-002](FDR-002-verified-email-signup.md),
  [FDR-003](FDR-003-local-login-and-browser-sessions.md),
  [FDR-004](FDR-004-openid-connect-provider.md)
