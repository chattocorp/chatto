# FDR-002: Verified Email Signup

**Status:** Experimental
**Last reviewed:** 2026-08-21

## Overview

Authling lets a person create a local email-and-password account only after
proving control of the email address with a short-lived one-time code. The
initial flow is server-rendered and requires no JavaScript. It creates the
durable account and starts the browser session defined by FDR-003.

## Behavior

1. The person submits an email address at `/signup`.
2. Authling normalizes it by trimming surrounding whitespace and lowercasing
   it. Authling does not apply provider-specific dot or plus-address rules.
3. Authling emails a random six-digit code. The browser receives only an opaque
   flow token; the address and code never enter a URL.
4. The flow expires after 15 minutes. At most ten codes may be delivered to an
   address and 1,000 globally in that window, no more than eight deliveries run
   concurrently per process, five wrong codes exhaust an individual flow, and
   at most five password-completion attempts may perform expensive verifier
   work. At most four password completions run concurrently per process.
5. A correct code moves the same expiring flow to a verified state. It still
   does not create an account.
6. The person enters and confirms a password whose minimum length is configured
   by the operator and whose maximum is 1,024 UTF-8 bytes. Authling requires
   both entries to match before attempting account creation. The default
   minimum is ten Unicode characters; operators may select a value from eight
   through 128. Authling rejects exact, case-insensitive matches from a small
   built-in list of commonly chosen passwords. It does not reject a longer
   password merely because that password contains a listed value.
   Authling uses Argon2id with a random salt and stores only the verifier in its
   own encrypted credential field.
7. Authling atomically creates the per-account history and claims the
   normalized address through a separate registry event. The flow is consumed
   after both facts become visible in the serving projection.
   Account creation also derives a preferred username from the normalized
   email local part, removes characters unsuitable for a portable username,
   and falls back to `user` when fewer than two characters remain. The value
   is an identity hint, not a login credential or an Authling-wide unique name.
8. Authling creates a fresh browser session and takes the person to the signed-in
   account page. If session storage is unavailable, the account remains created
   and the person can sign in later.

Requests for an already claimed address follow the same throttling, SMTP, and
verification path as available addresses so status and timing do not disclose
the claim. The final account command still rejects the duplicate. Invalid,
expired, exhausted, and duplicate flows use non-enumerating browser errors.
SMTP delivery failure removes the new flow and rolls back its delivery
reservation.

## Security and storage

- Raw OTP codes are never persisted. Lookup and comparison values use HMAC with
  a deployment workflow key.
- Pending addresses and OTP state are authenticated-encrypted in the
  `AUTHLING_RUNTIME_STATE` KV bucket and expire with the flow.
- Durable normalized email and password verifier values are encrypted as
  separately authenticated fields with a random credential data key. The
  preferred username is protected by the same key hierarchy as a distinct
  authenticated field. The
  projection decrypts only the email field needed for its lookup index. The
  data key is wrapped by a per-account user key;
  both key records live in the separate `AUTHLING_KEYS` bucket.
- The event contains only opaque key references, envelope metadata, nonce, and
  ciphertext. Authenticated data binds the ciphertext to its event, account,
  credential purpose, version, and data-key reference.
- A PII-free account-registry event subject serializes email claims with
  optimistic concurrency control while the same atomic batch establishes the
  per-account aggregate. The projection decrypts active email fields during
  replay and retains only keyed email digests for equality lookup.
- Cross-origin form submissions are rejected. Opaque flow tokens additionally
  bind the verification and completion forms to server-side state.
- SMTP transport encryption is mandatory by default. Opportunistic TLS is an
  explicit development-only choice used by the checked-in Mailpit config.

## Design decisions

### Create nothing durable before verification

Pre-verification activity is expiring workflow state, not an incomplete
account. This prevents unverified addresses from acquiring durable identity
state and keeps abandoned signups out of the event log.

### Serialize claims without durable email indexes

The registry subject is a contention point for signup, but signup is low
volume and correctness matters more than parallel write throughput. It avoids
putting email-derived digests in event subjects or durable indexes, preserving
the data-protection and cryptographic-erasure contract in ADR-002.

### Treat verified signup as an authentication ceremony

Proof of the emailed code and selection of the new password establish the same
browser session as a later password login. FDR-003 owns cookie, expiry,
revocation, and logout behavior; durable account creation remains complete even
if the temporary session cannot be stored.

### Keep the minimum length configurable

The default ten-character minimum keeps initial self-hosted deployments
approachable, while operators with different assurance requirements can raise
it. Authling enforces a lower bound of eight so configuration cannot
accidentally remove meaningful password-length protection. The byte ceiling
remains fixed to bound verifier resource use.

### Reject common passwords without composition rules

Authling rejects exact matches from a small built-in baseline list instead of
requiring particular character classes. This catches predictable choices such
as common numeric sequences and `Password123` without encouraging equally
predictable variations or rejecting ordinary passphrases. Case-insensitive
comparison prevents capitalization alone from bypassing the list; login and
all other password verification remain case-sensitive.

## Limitations

- The built-in common-password list is only a baseline. It does not yet use a
  comprehensive, maintained compromised-password corpus or have an operator
  update and extension story. The feature remains Experimental until those
  controls exist.
- Key provisioning writes a durable operation marker before key material and
  removes it after the referencing event commits. Normal failures compensate
  immediately. A crash orphan remains discoverable but is deliberately not
  deleted by a time-based heuristic; a future event-backed cleanup worker must
  prove that no publication can still reference it. Cryptographic erasure also
  requires erasure-aware replay before any account key is destroyed.
- There is no resend button; submitting the email form again starts a separate
  code flow within the shared delivery limit.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md),
  [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md),
  [ADR-003](../adr/ADR-003-server-rendered-templ-ui.md)
- **Features:** [FDR-001](FDR-001-standalone-account-runtime.md),
  [FDR-003](FDR-003-local-login-and-browser-sessions.md)
- **Security baseline:** [NIST SP 800-63B](https://pages.nist.gov/800-63-4/sp800-63b.html),
  [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html),
  [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html),
  and [OWASP CSRF Prevention](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
