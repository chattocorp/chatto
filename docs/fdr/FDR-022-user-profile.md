# FDR-022: User Profile

**Status:** Active
**Last reviewed:** 2026-07-15

## Overview

A user's profile carries the public identity they present to the rest of the server (login, display name, avatar, custom status) plus server-synced personal settings (timezone, time format). Most of the profile is self-editable; one field — the login — is throttled to discourage identity-confusion abuse, with an admin escape hatch for legitimate needs. Browser-local display preferences, such as theme, live outside the profile.

## Behavior

- **Display name** — freely editable by the user. Shown in messages, member lists, mention autocomplete, etc.
- **Login (username)** — editable by the user with a 30-day cooldown between changes. Logins start with a letter or number and cannot end with a period; periods remain valid within a login. Each successful change records a timestamp; subsequent changes within the window are rejected with a clear error message.
- **Case-only changes** (e.g., `alice` → `Alice`) bypass the cooldown.
- **Avatar** — users upload an image; the server resizes to 256×256 max and stores it as lossless WebP. The old avatar is deleted after the new one is committed. Users can also delete their avatar (falling back to an initial-letter placeholder).
- **Custom status** — users can set an emoji plus short text. The emoji is shown next to their name; the text is shown alongside it where space allows and as hover/accessible text in compact places.
- **Custom status templates** — the web client offers preset statuses for lunch, holiday/vacation, and sick leave plus a custom mode. Presets store reserved text tokens in the same free-form status text field so each client can render the label in its active locale. Custom mode stores the user's literal text.
- **Custom status expiry** — users can optionally choose an expiry date and time. After that instant, projected reads and the web client hide the status automatically. Users can also clear it manually.
- **Settings** — currently timezone (IANA name, e.g., `Europe/Berlin`) and time format (browser default / 12-hour / 24-hour). Stored server-side so they sync across devices. If not set, the frontend uses the browser timezone and locale time-format default.
- **Display theme** — users can choose System, Light, or Dark. System follows the browser or OS color-scheme preference. The choice is browser-local and applies immediately on that device.
- **Admin overrides** — operators with the right permissions can update other users' profiles, bypass the login cooldown, clear the cooldown so the user can change again before the 30 days expire, and force-delete an avatar.

## Design Decisions

### 1. 30-day login change cooldown

**Decision:** A user can change their login only once every 30 days.
**Why:** Logins are the basis for `@mentions`, search results, and recognition across the server. Frequent changes are an impersonation/confusion risk — `@alice` today might be a different person tomorrow. A 30-day cooldown discourages rapid churn while still allowing occasional rename for legitimate reasons. Case-only changes are exempt because they don't change identity.
**Tradeoff:** A user who legitimately needs to change twice in 30 days (e.g., picked a typo'd name) is stuck. The admin clear-cooldown affordance handles those cases.

### 2. Login uniqueness is enforced with projection catch-up and OCC

**Decision:** Login changes wait for the user projection to catch up, check its derived normalised login-digest index, and append the encrypted login-change event with optimistic concurrency over the user subject family. If another writer wins first, the operation retries against the updated projection. Projection apply decrypts the login transiently to update the digest index; user reads decrypt it again only while hydrating the response.
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

**Decision:** Timezone and time format live in the user's profile (in `User.settings`), synced server-side. Display theme is browser-local.
**Why:** A user signing in from a new browser shouldn't have to re-pick their preferences. Local storage works fine for one device; for multi-device users it's actively worse than server-side.
**Tradeoff:** Every timezone or time-format change requires a mutation, but settings change rarely so the cost is negligible. Theme can differ per browser, which is appropriate for device-specific light/dark preferences but means it does not sync across devices.

### 6. Browser timezone fallback when unset

**Decision:** If the user hasn't picked a timezone, the frontend uses the browser's `Intl.DateTimeFormat().resolvedOptions().timeZone`.
**Why:** Forcing every new user to pick a timezone at signup is friction. The browser usually knows.
**Tradeoff:** Travelers see times rendered in their travel timezone if they haven't explicitly set one. Most users either don't notice or prefer this.

### 7. Cross-user edits gated by `user.manage-accounts`

**Decision:** Admin updates to other users' profiles require `user.manage-accounts` for cross-user edits. Self-edits bypass that permission because they're privilege-neutral identity edits.
**Why:** Chatto's simplified RBAC model is permission-based for everyone except effective owners, who are protected by the owner override rather than target-rank gates.
**Tradeoff:** A user with `user.manage-accounts` can edit any target user's profile.

### 8. Custom status is durable profile metadata, not presence

**Decision:** Custom statuses are stored as user-aggregate EVT facts (`custom_status_set` / `custom_status_cleared`) and projected into `User.customStatus`. The status is independent of online/away/DND presence and does not affect notification routing.
**Why:** The product meaning is user-authored profile context ("working on X", "back after lunch"), not a current connection-state hint. Persisting it in EVT makes it replayable, backup-safe, and consistent across replicas and devices while keeping presence ephemeral.
**Tradeoff:** An expired status remains in historical EVT facts. Projections and clients hide it after `expiresAt`; clearing is a separate explicit fact rather than a background rewrite or KV delete.

### 9. Custom status writes use the protobuf-first API

**Decision:** The web client writes custom status through `MyAccountService` on the ConnectRPC `/api/connect` surface. Projected profile reads and realtime profile-change signals are also consumed through the ConnectRPC/realtime surface.
**Why:** Keeping profile writes, projected reads, and live refetch signals on the protobuf-first path avoids transport drift and keeps profile behavior aligned with the rest of the public API migration.
**Tradeoff:** Clients need to combine request/response profile APIs with the app-session realtime stream rather than relying on one subscription protocol for both.

### 10. Status templates are client-side reserved text tokens

**Decision:** Built-in templates use the same persisted `CustomUserStatus` shape as custom statuses. The emoji is stored normally, while the text field stores a reserved token such as `chatto:status:out_for_lunch`. Clients that understand the token render a localized label; unknown/custom text is rendered literally.
**Why:** This keeps the durable EVT model simple and preserves the "any emoji plus any text" API while allowing built-in statuses to be localized for each viewer.
**Tradeoff:** Older clients that do not know the reserved tokens may display the raw token. This is acceptable during early development and avoids a protobuf shape change solely for UI presets.

## Permissions

- Self-edit (display name, avatar, custom status, settings, own login subject to cooldown) — no explicit permission; just authentication.
- Cross-user edit — `user.manage-accounts`.
- Clear another user's login cooldown — same gate.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-021 (dual asset storage), ADR-043 (client-shell internationalization)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-008 (File Attachments & Video Processing), FDR-011 (User Presence), FDR-018 (Account Lifecycle)
