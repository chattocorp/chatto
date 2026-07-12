# FDR-018: Account Lifecycle

**Status:** Active
**Last reviewed:** 2026-07-10

## Overview

This FDR covers the user account from registration through deletion: signup, email verification, account deletion, and the crypto-shredding model that makes deletion permanent and reliable. It does *not* cover authentication mechanics (login, sessions, tokens) — those live in FDR-023.

## Behavior

### Registration

- A user signs up with a login, email, and password. The login must pass uniqueness, format, and blocked-username checks; email uniqueness is enforced when the address is verified.
- After signup, an email is sent to the address with a six-digit verification code.
- Registration and verification codes are backed by `RUNTIME_STATE` HMAC-derived records with configurable per-key TTLs (default 15 minutes). Raw code values are never written to `EVT` or backup archives. If email delivery fails, the pending OTP is cancelled so the failed send does not consume resend throttle capacity.
- Until the email is verified, the account has limited capabilities (configurable per server) — typically read-only or some restricted set defined by what the `verified` role grants.
- Entering the verification code marks the email as verified. The user gains the `verified` implicit role and the full set of permissions that role grants.
- If the verified email matches an entry in `owners.emails` in the server config, the user is auto-assigned the `owner` role on verification.

### Email management

- A user can have multiple verified email addresses on file.
- Adding a new email triggers a verification mail to the new address; the email is added in pending state until the code is confirmed.
- A user can delete one of their verified emails as long as at least one verified email remains.
- Email verification code issuance is recorded in the EVT audit log with a hashed email, expiry, and safe request metadata; the raw code is not recorded.

### Account deletion

- The user requests deletion via Account Settings.
- A two-step confirmation flow asks the user to type a confirmation string before the deletion executes.
- Account deletion confirmation-token issuance is recorded in the EVT audit log with expiry and safe request metadata; the raw token is not recorded.
- The account deletion confirmation token itself lives in `RUNTIME_STATE` under an HMAC-derived key with a 15-minute per-key TTL.
- On deletion, the server: removes the user's profile data, deletes their avatar, shreds the user's app-owned DEK refs from `RUNTIME_STATE` and KMS wrapping-key refs from `ENCRYPTION_KEYS`, records `UserKeyShreddedEvent` on the user aggregate, records durable deletion facts for message-owned assets and derivatives, and revokes all their sessions and bearer tokens. An elected cleanup worker retries physical removal for current message-owned asset deletion facts.
- After deletion, all messages the user ever posted are tombstoned by projection before decryption and cryptographically unreadable. Timeline clients apply the normal deleted-message retention rule, so placeholders without current attachments, previews, reactions, or thread replies disappear after one hour.
- New durable user events store login, display name, and verified email as encrypted PII payloads. Projections decrypt them while the user's key exists and skip rebuilding them after crypto-shredding.
- The login is freed up for re-use.

## Design Decisions

### 1. Email verification gates non-trivial actions, not access itself

**Decision:** Unverified users can sign in and see basic surfaces, but the meaningful permissions come from the `verified` implicit role. Operators decide what that role grants.
**Why:** Hard-blocking unverified users from logging in is annoying — common operational issues (typo'd email, lost verification mail) become lockouts. Permission-gating instead lets operators decide what "verified" means while keeping the user reachable.
**Tradeoff:** Operators have to actively configure the `verified` role for it to be meaningful. The default grants ensure the role does something out of the box.

### 2. Multiple verified emails per user

**Decision:** A user can attach multiple verified email addresses. Any of them count for owner-email matching, password resets, and identity correlation.
**Why:** People have work and personal addresses, change jobs, or have an alias. Single-email accounts force needless friction during transitions. Multiple-emails also helps the `owners.emails` config — operators can list either an old or new email and the right user gets owner status.
**Tradeoff:** The data model and resolvers have to handle a list, not a scalar. Minor extra complexity in exchange for real flexibility.

### 2a. Workflow tokens in runtime state

**Decision:** Registration and email-verification codes, registration completion tokens, password-reset tokens, and account-deletion confirmation tokens are stored in `RUNTIME_STATE` under HMAC-derived keys with per-key TTLs. The HMAC input is scoped by workflow and keyed by `[core].secret_key`.
**Why:** These values are raw credentials or credential-adjacent workflow state. They need restart and restore survival, but they are not reconstructable account history and should not become event-log or backup secrets. The audit value is captured separately in safe EVT facts.
**Tradeoff:** Operators must keep `[core].secret_key` stable across restores if pending account workflows should continue working. Changing it intentionally invalidates outstanding registration, email-verification, password-reset, and account-deletion credentials.

### 3. Two-step deletion confirmation

**Decision:** Account deletion requires the user to type a confirmation phrase, not just click a button. The `requestAccountDeletion` mutation sets up the flow; `deleteMyAccount` finalises it.
**Why:** Deletion is irreversible (the encryption key is destroyed; messages can't be recovered). A single misclick can't be allowed to trigger that. The phrase-typing step also defends against XSS triggering the mutation without the user's awareness — content scripts can't fill the phrase from the actual user.
**Tradeoff:** Slightly more UI work. Worth it for the irreversibility.

### 4. Crypto-shredding instead of message deletion

**Decision:** Account deletion shreds the app-owned DEK refs and KMS wrapping-key refs that protect the user's purpose-scoped DEKs and appends a durable `UserKeyShreddedEvent`. Encrypted message bodies and durable user PII stay on disk but become permanently unreadable; projections use the shred event to tombstone authored messages before attempting decryption. Message-owned assets, including derivative children such as thumbnails and video variants, receive `AssetDeletedEvent` and have their backing bytes removed.
**Why:** Scanning every JetStream stream and KV bucket for a user's messages would be slow, error-prone, and leave fragments in backups and replicas. Destroying the content-key records and their wrapping keys destroys all text content atomically, while the shred event gives projections and cleanup code a deterministic audit signal. Backups specifically exclude the encryption key bucket so that restoring a backup doesn't restore the ability to read deleted users' messages. See ADR-007.
**Tradeoff:** Encrypted-but-unreadable message bytes linger forever. Storage cost is small for text; binary assets are explicitly deleted because signed URLs could otherwise keep serving blobs until expiry.

### 5. Per-user KEKs, not shared keys

**Decision:** Each user has their own KEK, addressed through an opaque KMS key ref. New messages use a purpose-scoped message-body DEK epoch stored under an opaque app-owned content-key ref and wrapped by that key ref; durable user PII uses a separate user-PII DEK epoch. Legacy messages encrypted directly with the per-user key remain readable.
**Why:** Shared keys would mean one user's deletion can't crypto-shred their messages without affecting others. Per-user KEKs make each deletion fully self-contained, while opaque key refs, content-key refs, and purpose-scoped DEK epochs keep message and user events compact and map cleanly to local DEK storage plus external KMS unwrap flows. See ADR-007.
**Tradeoff:** Message-body and user-PII decryption have to resolve and unwrap the relevant DEK epoch. The built-in KMS path is cheap and local today; an external KMS may need caching policy and latency budgets.

### 6. Durable user PII is encrypted, not indexed in EVT

**Decision:** New durable user events encrypt login, display name, and verified email fields with the user's active user-PII DEK epoch. The projection decrypts these values and derives in-memory login/email indexes.
**Why:** Immutable event logs are the wrong long-term home for plaintext PII. Keeping the encrypted payload in EVT preserves replayability without a separate PII KV store, and deletion destroys the key needed to rebuild the data. See ADR-007.
**Tradeoff:** Projections need access to key-unwrapping during replay. If a user's key is gone, cold replay intentionally cannot rebuild their profile PII or uniqueness indexes.

### 7. KMS service boundary, even though it's in-process

**Decision:** KEK creation, DEK wrapping/unwrapping, and KEK shredding go through a KMS service interface (`createKey`, `wrapContentKey`, `unwrapContentKey`, `shredKey`) using opaque key refs rather than direct KEK access or user IDs. DEK record create/load/shred stays in application-owned `RUNTIME_STATE` storage.
**Why:** A clean KMS boundary is what makes future extraction to Vault / AWS KMS / HSM possible without turning the external KMS into Chatto's DEK registry. Keeping wrapped DEKs in `RUNTIME_STATE` also preserves them in normal data backups without backing up the KEKs needed to unwrap them. See ADR-007.
**Tradeoff:** A tiny indirection layer for what's currently an in-process call. Legacy direct-key body decrypt still has a local raw-KEK compatibility path until old bodies age out.

### 8. Login is freed on deletion

**Decision:** After account deletion, the deleted user's login is available for re-use by a new signup.
**Why:** Holding usernames forever would gradually exhaust the namespace. Re-use is acceptable because the new owner gets a new identity (new user ID, new encryption key) — they don't inherit any of the previous user's data or messages.
**Tradeoff:** Old @mentions of the previous user may visually point at the new user once the login is reclaimed. The underlying mention link is to the user *id*, which is gone; the rendering falls back to plain text.

## Permissions

- Self: anyone authenticated can update their own profile (FDR-022), add or remove their own emails, and delete their own account.
- `user.delete-any` — admin permission to delete other users' accounts.
- `user.delete-self` — gates own-account deletion. Granted to `everyone` by default; operators can revoke to lock down self-deletion.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-022 (User Profile), FDR-023 (Authentication & Sessions)

## Open Questions

- Should "delete" be replaceable with "anonymize" (keep messages, scrub identity)? Today the choice is delete-and-shred or nothing. Operators occasionally want a middle ground for moderation purposes.
