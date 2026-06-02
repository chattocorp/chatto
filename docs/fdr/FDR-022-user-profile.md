# FDR-022: User Profile

**Status:** Active
**Last reviewed:** 2026-06-02

## Overview

A user's profile carries the public identity they present to the rest of the server (login, display name, avatar) plus personal settings (timezone, time format). Most of the profile is self-editable; one field — the login — is throttled to discourage identity-confusion abuse, with an admin escape hatch for legitimate needs.

## Behavior

- **Display name** — freely editable by the user. Shown in messages, member lists, mention autocomplete, etc.
- **Login (username)** — editable by the user with a 30-day cooldown between changes. Each successful change records a timestamp; subsequent changes within the window are rejected with a clear error message.
- **Case-only changes** (e.g., `alice` → `Alice`) bypass the cooldown.
- **Avatar** — users upload an image; the server resizes to 256×256 max and stores it as lossless WebP. The old avatar is deleted after the new one is committed. Users can also delete their avatar (falling back to an initial-letter placeholder).
- **Settings** — currently timezone (IANA name, e.g., `Europe/Berlin`) and time format (12-hour / 24-hour). Stored server-side so they sync across devices. If not set, the frontend uses the browser timezone.
- **Admin overrides** — operators with the right permissions can update other users' profiles, bypass the login cooldown, clear the cooldown so the user can change again before the 30 days expire, and force-delete an avatar.

## Design Decisions

### 1. 30-day login change cooldown

**Decision:** A user can change their login only once every 30 days.
**Why:** Logins are the basis for `@mentions`, search results, and recognition across the server. Frequent changes are an impersonation/confusion risk — `@alice` today might be a different person tomorrow. A 30-day cooldown discourages rapid churn while still allowing occasional rename for legitimate reasons. Case-only changes are exempt because they don't change identity.
**Tradeoff:** A user who legitimately needs to change twice in 30 days (e.g., picked a typo'd name) is stuck. The admin clear-cooldown affordance handles those cases.

### 2. Login uniqueness is enforced with projection catch-up and OCC

**Decision:** Login changes wait for the user projection to catch up, check the decrypted login index, and append the login-change event with optimistic concurrency over the user subject family. If another writer wins first, the operation retries against the updated projection.
**Why:** User profile state now lives in the event-sourced user aggregate, and new durable login-change facts carry encrypted PII. Projection catch-up plus OCC keeps uniqueness race-safe without reintroducing a separate login KV as source of truth.
**Tradeoff:** The write path depends on projection readiness and may retry under contention. In exchange, the durable event stream remains append-only and the login index stays derived state.

### 3. Admin path doesn't advance the cooldown timestamp

**Decision:** When an admin changes a user's login, the user's cooldown clock isn't reset. The user can still wait out their own cooldown and change to a different login.
**Why:** The cooldown is about the *user's* identity stability, not the admin's. An admin-driven correction shouldn't reset the user's own quota.
**Tradeoff:** A user who keeps getting admin-renamed has a slightly confusing experience around when their next self-change is allowed. Acceptable; uncommon edge case.

### 4. Avatars are WebP-only, capped at 256×256

**Decision:** Uploaded avatars are resized to a 256×256 max box and re-encoded as lossless WebP. Original is discarded.
**Why:** Avatars render at small sizes everywhere — 256px is the largest the UI ever shows. Storing originals is waste. Lossless WebP is small and supports transparency. See FDR-008's notes on the WebP/JPEG split for transparency vs photographic content.
**Tradeoff:** A user uploading a high-resolution avatar can't ever get the original back. The 256×256 cap can't be inferred from the user's perspective unless documented.

### 5. Server-side settings, not browser-local

**Decision:** Timezone and time format live in the user's profile (in `User.settings`), synced server-side.
**Why:** A user signing in from a new browser shouldn't have to re-pick their preferences. Local storage works fine for one device; for multi-device users it's actively worse than server-side.
**Tradeoff:** Every settings change requires a mutation, but settings change rarely so the cost is negligible.

### 6. Browser timezone fallback when unset

**Decision:** If the user hasn't picked a timezone, the frontend uses the browser's `Intl.DateTimeFormat().resolvedOptions().timeZone`.
**Why:** Forcing every new user to pick a timezone at signup is friction. The browser usually knows.
**Tradeoff:** Travelers see times rendered in their travel timezone if they haven't explicitly set one. Most users either don't notice or prefer this.

### 7. Cross-user edits gated by `role.assign` + outranking

**Decision:** Admin updates to other users' profiles use the `requireUserAdminTarget` helper, which requires `role.assign` permission *and* that the actor outranks the target. Self-edits bypass both — they're an identity edit, not an authorization edit (see FDR-001).
**Why:** Without the outrank check, a moderator with `role.assign` could rename the server owner. The combination prevents that without inventing new permissions.
**Tradeoff:** Two-step authorization for one operation. The helper centralizes it.

## Permissions

- Self-edit (display name, avatar, settings, own login subject to cooldown) — no explicit permission; just authentication.
- Cross-user edit — `role.assign` + outrank target (via `requireUserAdminTarget`).
- Clear another user's login cooldown — same gate.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-021 (dual asset storage)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-008 (File Attachments & Video Processing), FDR-018 (Account Lifecycle)
