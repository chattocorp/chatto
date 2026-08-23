# FDR-022: User Profile

**Status:** Active
**Last reviewed:** 2026-08-23

## Overview

A human user's profile carries the public identity they present to the rest of the server (login, display name, avatar, custom status) plus server-synced User Preferences (timezone, time format). Most of the profile is self-editable; one field — the login — is throttled to discourage identity-confusion abuse, with an admin escape hatch for legitimate needs. The profile does not contain App Preferences, such as appearance, language, editor, and send-key behavior. The app applies these choices to its registered servers. Bot accounts expose the same public identity shape but currently support only managed login and display-name edits (FDR-038).

## Behavior

- **Display name** — freely editable by a human user. Shown in messages, member lists, mention autocomplete, etc.
- **Login (username)** — editable by a human user with a 30-day cooldown between changes. Logins start with a letter or number and cannot end with a period; periods remain valid within a login. Each successful change records a timestamp; subsequent changes within the window are rejected with a clear error message.
- **Case-only changes** (e.g., `alice` → `Alice`) bypass the cooldown.
- **Avatar** — human users upload an image; the server resizes to 256×256 max and stores it as lossless WebP. The old avatar is deleted after the new one is committed. Users can also delete their avatar (falling back to an initial-letter placeholder).
- **Custom status** — human users can set an emoji plus short text. The emoji is shown next to their name; the text is shown alongside it where space allows and as hover/accessible text in compact places.
- **Custom status templates** — the web client offers preset statuses for lunch, holiday/vacation, and sick leave plus a custom mode. Presets store reserved text tokens in the same free-form status text field so each client can render the label in its active locale. Custom mode stores the user's literal text.
- **Custom status expiry** — users can optionally choose an expiry date and time. After that instant, projected reads and the web client hide the status automatically. Users can also clear it manually.
- **User Preferences** — human accounts currently support timezone (IANA name, e.g., `Europe/Berlin`) and time format (browser default / 12-hour / 24-hour). The server stores these choices and syncs them across devices. If a choice is not set, the frontend uses the browser timezone and locale time-format default. The unified Settings sidebar shows these personal choices with permission-gated Server Configuration.
- **App Preferences** — users can select System, Light, or Dark appearance, a language, a message editor, and send-key behavior. System appearance follows the browser or OS color-scheme preference. The app applies these choices to every registered server. Open them from the Application Header. They do not sync to another browser or device. The App Preferences sidebar contains Appearance, Language, and Composer pages.
- **Profile menu** — opening a user's profile popup or touch sheet shows their public identity and any available message or moderation actions. A final “Copy User ID” action copies the stable user ID to the clipboard.
- **Admin overrides** — operators with the right permissions can update other human users' profiles, bypass the login cooldown, clear the cooldown so the user can change again before the 30 days expire, and force-delete an avatar.
- **Bot identity management** — a bot owner or human user with `bot.manage` can update a bot's login and display name through `BotService`. Bot API keys cannot edit identity, and bot avatar, custom-status, and personal-settings management are not supported in this slice.

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
**Why:** The cooldown is about the _user's_ identity stability, not the admin's. An admin-driven correction shouldn't reset the user's own quota.
**Tradeoff:** A user who keeps getting admin-renamed has a slightly confusing experience around when their next self-change is allowed. Acceptable; uncommon edge case.

### 4. Avatars are WebP-only, capped at 256×256

**Decision:** Uploaded avatars are resized to a 256×256 max box and re-encoded as lossless WebP. Original is discarded.
**Why:** Avatars render at small sizes everywhere — 256px is the largest the UI ever shows. Storing originals is waste. Lossless WebP is small and supports transparency. See FDR-008's notes on the WebP/JPEG split for transparency vs photographic content.
**Tradeoff:** A user uploading a high-resolution avatar can't ever get the original back. The 256×256 cap can't be inferred from the user's perspective unless documented.

### 5. User Preferences and App Preferences have different scopes

**Decision:** Timezone and time format are User Preferences in the user's profile (`User.settings`). The server syncs these choices. Appearance, language, editor, and send-key behavior are App Preferences. The app stores these choices and applies them to its registered servers.
**Why:** The server can use a person's timezone for server features. For example, the server can warn you when it is late for a person you want to message. The app controls presentation and input choices. These choices must stay the same when you move between its registered servers.
**Tradeoff:** Each timezone or time-format change requires a mutation. These settings change rarely, so the cost is small. App Preferences can be different in each browser or device and do not follow the user.

### 6. Browser timezone fallback when unset

**Decision:** If the user hasn't picked a timezone, the frontend uses the browser's `Intl.DateTimeFormat().resolvedOptions().timeZone`.
**Why:** Forcing every new user to pick a timezone at signup is friction. The browser usually knows.
**Tradeoff:** Travelers see times rendered in their travel timezone if they haven't explicitly set one. Most users either don't notice or prefer this.

### 7. Cross-user edits gated by `user.manage-accounts`

**Decision:** Admin updates to other human users' profiles require `user.manage-accounts` for cross-user edits. Human self-edits bypass that permission because they're privilege-neutral identity edits. Bot login and display-name changes instead follow BotService ownership or `bot.manage` authorization and remain bounded to those two fields.
**Why:** Chatto's simplified RBAC model is permission-based for everyone except effective owners, who are protected by the owner override rather than target-rank gates.
**Tradeoff:** A user with `user.manage-accounts` can edit any target human user's profile, while bot identity management has a separate, deliberately narrower authority path.

### 8. Custom status is durable profile metadata, not presence

**Decision:** Custom statuses are stored as user-aggregate EVT facts (`custom_status_set` / `custom_status_cleared`) and projected into `User.customStatus`. The status is independent of online/away/DND presence and does not affect notification routing.
**Why:** The product meaning is user-authored profile context ("working on X", "back after lunch"), not a current connection-state hint. Persisting it in EVT makes it replayable, backup-safe, and consistent across replicas and devices while keeping presence ephemeral.
**Tradeoff:** An expired status remains in historical EVT facts. Projections and clients hide it after `expiresAt`; clearing is a separate explicit fact rather than a background rewrite or KV delete.

### 9. Custom status writes use the protobuf-first API

**Decision:** The web client writes custom status through `MyAccountService` on the ConnectRPC `/api/connect` surface. Resource reads remain available through ConnectRPC, while realtime profile changes arrive as authoritative client-projection operations.
**Why:** Keeping profile writes, resource reads, and projection updates on protobuf-first public surfaces avoids transport drift and keeps profile behavior aligned with the rest of the public API migration without requiring live refetches.
**Tradeoff:** Clients need to combine request/response profile APIs with the app-session realtime stream rather than relying on one subscription protocol for both.

### 10. Status templates are client-side reserved text tokens

**Decision:** Built-in templates use the same persisted `CustomUserStatus` shape as custom statuses. The emoji is stored normally, while the text field stores a reserved token such as `chatto:status:out_for_lunch`. Clients that understand the token render a localized label; unknown/custom text is rendered literally.
**Why:** This keeps the durable EVT model simple and preserves the "any emoji plus any text" API while allowing built-in statuses to be localized for each viewer.
**Tradeoff:** Older clients that do not know the reserved tokens may display the raw token. This is acceptable during early development and avoids a protobuf shape change solely for UI presets.

## Permissions

- Human self-edit (display name, avatar, custom status, settings, own login subject to cooldown) — no explicit permission; just authentication.
- Cross-human-user edit — `user.manage-accounts`.
- Clear another user's login cooldown — same gate.
- Bot login and display-name edit — bot ownership or `bot.manage`; bot avatar, custom-status, and personal-settings edits are not supported.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-021 (dual asset storage), ADR-065 (runtime JSON client internationalization)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-008 (File Attachments & Video Processing), FDR-011 (User Presence), FDR-018 (Account Lifecycle), FDR-038 (Bot Accounts)
