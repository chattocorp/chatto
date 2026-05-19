# FDR-018: Account Lifecycle

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

This FDR covers the user account from registration through deletion: signup, email verification, account deletion, and the crypto-shredding model that makes deletion permanent and reliable. It does *not* cover authentication mechanics (login, sessions, tokens) — those live in FDR-023.

## Behavior

### Registration

- A user signs up with a login, email, and password. The login must pass uniqueness and format checks; emails are checked against the server's blocked-usernames list.
- After signup, an email is sent to the address with a verification link.
- Until the email is verified, the account has limited capabilities (configurable per server) — typically read-only or some restricted set defined by what the `verified` role grants.
- Clicking the verification link marks the email as verified. The user gains the `verified` implicit role and the full set of permissions that role grants.
- If the verified email matches an entry in `owners.emails` in the server config, the user is auto-assigned the `owner` role on verification.

### Email management

- A user can have multiple verified email addresses on file.
- Adding a new email triggers a verification mail to the new address; the email is added in pending state until the link is clicked.
- A user can delete one of their verified emails as long as at least one verified email remains.

### Account deletion

- The user requests deletion via Account Settings.
- A two-step confirmation flow asks the user to type a confirmation string before the deletion executes.
- On deletion, the server: removes the user's profile data, deletes their avatar, removes their per-user encryption key from the `ENCRYPTION_KEYS` KV bucket, and revokes all their sessions and bearer tokens.
- After deletion, all messages the user ever posted are cryptographically unreadable — the bytes are still on disk in JetStream, but without the key they decrypt to noise.
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

### 3. Two-step deletion confirmation

**Decision:** Account deletion requires the user to type a confirmation phrase, not just click a button. The `requestAccountDeletion` mutation sets up the flow; `deleteMyAccount` finalises it.
**Why:** Deletion is irreversible (the encryption key is destroyed; messages can't be recovered). A single misclick can't be allowed to trigger that. The phrase-typing step also defends against XSS triggering the mutation without the user's awareness — content scripts can't fill the phrase from the actual user.
**Tradeoff:** Slightly more UI work. Worth it for the irreversibility.

### 4. Crypto-shredding instead of message deletion

**Decision:** Account deletion destroys the user's encryption key. Encrypted message bodies stay on disk but become permanently unreadable.
**Why:** Scanning every JetStream stream and KV bucket for a user's messages would be slow, error-prone, and leave fragments in backups and replicas. Destroying one key destroys all their content atomically. Backups specifically exclude the encryption key bucket so that restoring a backup doesn't restore the ability to read deleted users' messages. See ADR-007.
**Tradeoff:** Encrypted-but-unreadable bytes linger forever. Storage cost is small (chat messages are tiny) and the privacy guarantee is stronger than "we tried to delete everything we could find".

### 5. Per-user keys, not shared keys

**Decision:** Each user has their own ChaCha20-Poly1305 encryption key.
**Why:** Shared keys would mean one user's deletion can't crypto-shred their messages without affecting others. Per-user keys make each deletion fully self-contained. See ADR-007.
**Tradeoff:** Every message-body decryption is a per-author KV lookup. The lookup is cheap (NATS KV is memory-cached) and dataloader batches help on bulk reads.

### 6. KMS service boundary, even though it's in-process

**Decision:** Encryption operations go through a KMS service interface (`encrypt`, `decrypt`, `deleteKey`) rather than direct key access. The default implementation runs in-process; the interface is designed for extraction to a standalone service.
**Why:** A clean service boundary is what makes future extraction to Vault / AWS KMS / HSM possible without rewriting business logic. See ADR-007.
**Tradeoff:** A tiny indirection layer for what's currently an in-process call. Negligible cost; future flexibility worth a lot.

### 7. Login is freed on deletion

**Decision:** After account deletion, the deleted user's login is available for re-use by a new signup.
**Why:** Holding usernames forever would gradually exhaust the namespace. Re-use is acceptable because the new owner gets a new identity (new user ID, new encryption key) — they don't inherit any of the previous user's data or messages.
**Tradeoff:** Old @mentions of the previous user may visually point at the new user once the login is reclaimed. The underlying mention link is to the user *id*, which is gone; the rendering falls back to plain text.

## Permissions

- Self: anyone authenticated can update their own profile (FDR-022), add or remove their own emails, and delete their own account.
- `user.delete-any` — admin permission to delete other users' accounts. Subject to outranking the target via `requireUserAdminTarget`.
- `user.delete-self` — gates own-account deletion. Granted to `everyone` by default; operators can revoke to lock down self-deletion.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-022 (User Profile), FDR-023 (Authentication & Sessions)

## Open Questions

- Should "delete" be replaceable with "anonymize" (keep messages, scrub identity)? Today the choice is delete-and-shred or nothing. Operators occasionally want a middle ground for moderation purposes.
