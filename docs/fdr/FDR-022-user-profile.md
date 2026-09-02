# FDR-022: User Profile

**Status:** Active
**Last reviewed:** 2026-09-02

## Overview

A user's profile carries the public identity they present to the rest of the server (login, display name, avatar, custom status, bio, shared time zone) plus server-synced User Preferences (timezone, time format). Human accounts support the complete profile. Bot accounts support self-service login, display-name, bio, and avatar changes (FDR-038). The login is throttled to discourage identity-confusion abuse, with an admin escape hatch for legitimate human-account needs. The profile does not contain App Preferences, such as appearance, thread presentation, language, editor, and send-key behavior. The app applies these choices to its registered servers.

## Behavior

- **Display name** — freely editable by a human or bot account. Shown in messages, member lists, mention autocomplete, etc.
- **Login (username)** — editable by a human or bot account with a 30-day cooldown between changes. A user with `user.manage-accounts` bypasses the cooldown for their own login. Logins start with a letter or number and cannot end with a period; periods remain valid within a login. Bot logins must end in `_bot`. Each successful change that does not use the bypass records a timestamp; subsequent changes within the window are rejected with a clear error message.
- **Case-only changes** (e.g., `alice` → `Alice`) bypass the cooldown.
- **Avatar** — human and bot users can upload an image. The server resizes it to 256×256 maximum and stores it as lossless WebP. The old avatar is deleted after the new avatar is committed. Users can also delete their avatar and use the initial-letter placeholder. A human with `user.manage-accounts` can manage another human's avatar. A bot owner, a human with `bot.manage`, or a human with `user.manage-accounts` can manage a bot's avatar.
- **Custom status** — human users can set an emoji plus short text. The emoji is shown next to their name; the text is shown alongside it where space allows and as hover/accessible text in compact places.
- **Custom status templates** — the web client offers preset statuses for lunch, holiday/vacation, and sick leave plus a custom mode. Presets store reserved text tokens in the same free-form status text field so each client can render the label in its active locale. Custom mode stores the user's literal text.
- **Custom status expiry** — users can optionally choose an expiry date and time. After that instant, projected reads and the web client hide the status automatically. Users can also clear it manually.
- **User Preferences** — human accounts currently support a time zone (IANA name, e.g., `Europe/Berlin`), a time format (browser default / 12-hour / 24-hour), and permission to share the time zone. The server stores these choices and syncs them across devices. The stored time zone is private by default. If a choice is not set, the frontend uses the browser time zone and locale time-format default. A compatible web client reports the device time zone to the server once when no explicit time zone is set. It does not overwrite an explicitly selected zone. The unified Settings sidebar puts these personal choices in the Your account group. Permission-gated Server configuration remains separate.
- **Bio** — human and bot accounts can set a self-authored Markdown bio of up to 1,000 characters through `MyAccountService.UpdateProfile`. The bio is shown on the Profile Card and in the Profile View. The client disables source HTML and sanitizes rendered output.
- **Public time zone** — a user must explicitly enable time-zone sharing. When sharing is enabled, public user reads include the stored IANA zone and clients show the current local time on profile surfaces. When sharing is disabled, public reads and realtime updates omit both values. The account can still use its stored time zone for private formatting.
- **Profile View** — “View profile” opens or returns to a one-to-one direct message with that user, then opens their complete public profile in that conversation's Room Sidebar. It stays available when the viewer can open an existing DM but cannot create a new one. If no such DM exists, the action is disabled. The Profile View shows the avatar, display name, login, custom status, bio, bot marker, and local time. The direct-message header has an information button that opens the same view again, including in a self-DM. Closing it returns to the prior room-extras panel, or hides the Room Sidebar when no panel was open. The same Profile View follows the responsive Room Sidebar layout when the viewport changes.
- **App Preferences** — users can select System, Light, or Dark appearance, overlay or side-by-side thread presentation, a language, a message editor, and send-key behavior. System appearance follows the browser or OS colour-scheme preference. Overlay thread presentation is the default. The app applies these choices to every registered server. The Application Header gear opens Appearance for the active authenticated server. The unified Settings sidebar puts Appearance, Language, and Composer in an App preferences group. If no authenticated server is available, the same pages use a separate App Preferences sidebar. App Preferences do not sync to another browser or device.
- **Profile Card** — opening a user's Profile Card as a popover or bottom sheet shows their public identity, bio snippet, live local time in their shared zone, and available message or moderation actions. A final “Copy User ID” action copies the stable user ID to the clipboard.
- **Admin overrides** — operators with the right permissions can update other human users' profiles, bypass the login cooldown, clear the cooldown so the user can change again before the 30 days expire, and manage an avatar.
- **Bot identity management** — an API-key-authenticated bot updates its own login, display name, and bio through `MyAccountService.UpdateProfile`. It manages its avatar through `UserService`. Human owners manage bot lifecycle, ownership, permissions, API keys, and avatars. A human with `bot.manage` or `user.manage-accounts` can also manage a bot's avatar. Bot custom-status and personal-settings management are not supported.

## Design Decisions

### 1. 30-day login change cooldown

**Decision:** A user can change their login only once every 30 days. A user with `user.manage-accounts` can bypass this limit for their own login. A bypassed change does not clear or advance the existing cooldown timestamp.
**Why:** Logins are the basis for `@mentions`, search results, and recognition across the server. Frequent changes are an impersonation/confusion risk — `@alice` today might be a different person tomorrow. A 30-day cooldown discourages rapid churn while still allowing occasional rename for legitimate reasons. Case-only changes are exempt because they don't change identity.
**Tradeoff:** A user who legitimately needs to change twice in 30 days (e.g., picked a typo'd name) is stuck. The admin clear-cooldown affordance handles those cases.

### 2. Login uniqueness is enforced with projection catch-up and OCC

**Decision:** Login changes wait for the user projection to catch up, check its derived normalised login-digest index, and append the encrypted login-change event with optimistic concurrency over the user subject family. If another writer wins first, the operation retries against the updated projection. Projection apply decrypts the login transiently to update the digest index; user reads decrypt it again only while hydrating the response.
**Why:** User profile state now lives in the event-sourced user aggregate, and new durable login-change facts carry encrypted PII. Projection catch-up plus OCC keeps uniqueness race-safe without reintroducing a separate login KV as source of truth.
**Tradeoff:** The write path depends on projection readiness and may retry under contention. In exchange, the durable event stream remains append-only and the login index stays derived state.

### 3. Privileged changes do not advance the cooldown timestamp

**Decision:** A login change does not reset the cooldown clock when an account manager changes another user's login or bypasses their own cooldown. The previous self-service cooldown timestamp stays in effect.
**Why:** A privileged correction does not use the user's normal login-change allowance.
**Tradeoff:** A user can see a cooldown that started before a privileged login change. This case is uncommon.

### 4. Avatars are WebP-only, capped at 256×256

**Decision:** Uploaded avatars are resized to a 256×256 max box and re-encoded as lossless WebP. Original is discarded.
**Why:** Avatars render at small sizes everywhere — 256px is the largest the UI ever shows. Storing originals is waste. Lossless WebP is small and supports transparency. See FDR-008's notes on the WebP/JPEG split for transparency vs photographic content.
**Tradeoff:** A user uploading a high-resolution avatar can't ever get the original back. The 256×256 cap can't be inferred from the user's perspective unless documented.

### 5. User Preferences and App Preferences have different scopes

**Decision:** Timezone and time format are User Preferences in the user's profile (`User.settings`). The server syncs these choices. Appearance, thread presentation, language, editor, and send-key behavior are App Preferences. The app stores these choices and applies them to its registered servers. Authenticated users manage both scopes in one Settings sidebar, with separate groups that identify the scope.
**Why:** The server can use a person's timezone for server features. For example, the server can warn you when it is late for a person you want to message. The app controls presentation and input choices. These choices must stay the same when you move between its registered servers.
**Tradeoff:** Each timezone or time-format change requires a mutation. These settings change rarely, so the cost is small. App Preferences can be different in each browser or device and do not follow the user. The selected server remains in the Settings header so the account scope stays clear beside the app-wide group.

### 6. Browser timezone fallback when unset

**Decision:** If the user hasn't picked a timezone, the frontend uses the browser's `Intl.DateTimeFormat().resolvedOptions().timeZone`.
**Why:** Forcing every new user to pick a timezone at signup is friction. The browser usually knows.
**Tradeoff:** Travelers see times rendered in their travel timezone if they haven't explicitly set one. Most users either don't notice or prefer this.

### 7. Cross-user edits gated by `user.manage-accounts`

**Decision:** Admin updates to other human users' profiles require `user.manage-accounts` for cross-user edits. Human and bot self-edits use `MyAccountService.UpdateProfile` and bypass that permission because they are privilege-neutral identity edits. Avatar upload and deletion use the target-aware `UserService` methods. A cross-user human target requires `user.manage-accounts`. A cross-user bot target permits its owner, `user.manage-accounts`, or `bot.manage`. A bot cannot target another account. Bot lifecycle and credential management remain separate owner-authorized operations in `BotService`.
**Why:** Chatto's simplified RBAC model is permission-based for everyone except effective owners, who are protected by the owner override rather than target-rank gates.
**Tradeoff:** Avatar authority differs from authority for other profile fields. Clients must use the target-aware avatar methods and must not infer authority from access to other profile operations.

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

### 11. Bio is encrypted PII with change-detection

**Decision:** Bio edits validate a 1,000-character cap, compare against the projected value, and append a durable encrypted `bio_changed` fact only when the value actually changes. Clearing stores an empty payload. Projected state keeps only ciphertext; reads decrypt transiently like display names. Clients render bios as Markdown through the same audited HTML boundary as message Markdown. Source HTML stays disabled.
**Why:** Free-form user text is PII under Chatto's crypto-shredding model, so it follows the display-name pattern. Change detection prevents idle autosaves or repeated submits from appending meaningless facts to EVT. An unchanged update returns success without a write.
**Tradeoff:** Unchanged-write no-ops mean clients cannot distinguish "saved" from "nothing changed"; both return the current profile, which is what UIs need.

### 12. One stored time zone has independent sharing permission

**Decision:** There is one stored time zone per account and a separate sharing setting. The time zone formats the user's own timestamps and stays private by default. The server populates the public `User.timezone` only when the user enables sharing. Clients then show the IANA name and the derived local time. Historical accounts and new accounts start with sharing disabled. New servers always return the sharing setting in `UserSettings`. Its field presence tells a client that the privacy control is supported.
**Why:** One stored value prevents drift between formatting and profile data. A separate permission lets a user keep the formatting benefit without disclosing location-related data. The local time stays correct across daylight saving time changes.
**Tradeoff:** Older servers do not support the permission and can publish a stored time zone. A compatible client does not report the browser time zone to such a server and warns the user. During an upgrade, all replicas must run the new version before the private behavior is reliable.

### 13. The Profile View belongs to one-to-one direct messages

**Decision:** The Profile View is a transient view in the Room Sidebar of that user's one-to-one direct message. “View profile” opens that direct message and its Profile View. It can reuse an existing DM without permission to create new DMs, but it is disabled when no reusable DM exists. The direct-message header has an information button for the same view. Channel-room information will use that information-button pattern later.
**Why:** A profile is context for a person-to-person conversation, not a room-extras category. The message timeline stays visible beside the profile and the header has one stable place to reopen it. Reusing a DM does not create a new conversation. The pattern also gives channel rooms a clear future home for room information without making two unrelated sidebar modes look alike.
**Tradeoff:** Viewing a profile can navigate away from the current room and can create a direct message when permission allows it. A direct profile URL is not available. Closing the profile preserves the selected sidebar panel in the direct message.

### 14. Avatar mutations use one target-aware user API

**Decision:** `UserService.UploadAvatar` and `UserService.DeleteAvatar` each take a target user ID. The core checks the caller, the target account kind, bot ownership, and current permissions at the domain boundary. The durable fact records the authenticated caller as its actor. The write validates stable request-time authorization inputs and uses optimistic concurrency control for the target user aggregate. The response waits for the user projection and then publishes the existing profile-update snapshot. Delete is idempotent.
**Why:** One command path gives human and bot avatars the same validation, storage, projection, cleanup, and realtime behavior. Stable authorization input validation prevents a torn permission or ownership decision. Target-user OCC protects account and avatar state.
**Tradeoff:** This is an intentional pre-1.0 API break. Clients that used `MyAccountService.UploadAvatar` or `MyAccountService.DeleteAvatar` must move to `UserService` and send the authenticated user's ID when they manage their own avatar.

## Permissions

- Human or bot self-edit of an avatar — no explicit permission; only authentication.
- Human self-edit of display name, custom status, settings, and own login subject to cooldown — no explicit permission; only authentication. `user.manage-accounts` bypasses the holder's own login cooldown.
- Cross-human-user edit — `user.manage-accounts`.
- Clear another user's login cooldown — same gate.
- Bot avatar edit by another human — bot ownership, `user.manage-accounts`, or `bot.manage`.
- Bot login and display-name edit — the authenticated bot through `MyAccountService.UpdateProfile`; bot custom-status and personal-settings edits are not supported.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-021 (dual asset storage), ADR-065 (runtime JSON client internationalization), ADR-087 (request-time authorization with aggregate OCC)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-008 (File Attachments & Video Processing), FDR-011 (User Presence), FDR-018 (Account Lifecycle), FDR-038 (Bot Accounts)
