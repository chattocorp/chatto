# FDR-020: Server Branding & Configuration

**Status:** Active
**Last reviewed:** 2026-06-06

## Overview

Operators can customize how their Chatto server presents itself. The server's name, description, welcome message, logo, banner, and message-of-the-day are all editable from the admin UI and visible to members and visitors. A small number of operational knobs (blocked usernames, etc.) live in the same config surface.

## Behavior

- **Server name** — appears in page titles, the chat header, and OG metadata. Defaults to "Chatto".
- **Description** — used in OG metadata for link previews when sharing the server URL.
- **Welcome message** — shown on the login page. Markdown is supported.
- **MOTD (message of the day)** — appears in a banner across the top of the chat surface for all members. Broadcasts to live clients when changed.
- **Logo** — shown in the chat header, login page, and OG image fallback. Uploaded as an image; the public GraphQL profile exposes its canonical URL without transform arguments.
- **Banner** — shown on the login page and in OG previews. Same upload/serve pipeline as the logo.
- **Blocked usernames** — newline-separated list checked at signup. Matches are rejected before account creation.
- Text configuration is bounded before storage: server name 80 bytes, description 500 bytes, MOTD 1,000 bytes, welcome message 10,000 bytes, blocked-usernames field 10,000 bytes, and each blocked username no longer than a normal username.

## Design Decisions

### 1. Presentation config uses one partial-update surface

**Decision:** `updateServerConfig` accepts the presentation text fields as nullable values. A nil input for a field leaves the existing value untouched; only fields the caller explicitly sets get changed.
**Why:** Partial-update semantics let UI forms send only changed fields without GET-then-PUT round-trips and without overwriting other fields with whatever defaults the form thinks they should be. It also makes API clients (CLI tools, scripts) safer.
**Tradeoff:** Two ways to "clear" a string field: empty-string vs unset. The API treats empty string as a clear and nil as "leave alone". Documented; consistent across all string fields.

### 2. Public profile/config changes broadcast as one live event

**Decision:** Public server profile/config changes publish one transient `ServerUpdatedEvent` on `live.sync.config.server_updated` and are delivered to every authenticated user. Clients treat the event as a refetch signal for `Server.profile` and related authenticated settings they display.
**Why:** Server name, MOTD, logo, banner, description, and welcome copy are visible across the UI. One event keeps profile/config live behavior clear and avoids duplicate broadcasts from text updates.
**Tradeoff:** Every connected client gets every public profile/config change event, including fields they may not render. Volume is low (operators don't tweak branding constantly) so this is fine.

### 3. Logo and banner have their own upload mutations

**Decision:** `uploadServerLogo` and `uploadServerBanner` are separate from `updateServerConfig`. They accept multipart uploads, process the image, store the binary in the asset store, and update the server's logo or banner pointer through the same event-sourced configuration flow as the rest of the server config.
**Why:** Image upload is a different shape from string config — multipart bodies, content-type validation, asset storage. Keeping it in its own mutations means `updateServerConfig` stays simple, while storing the current pointer in configuration events keeps branding restorable from the event log instead of depending on legacy KV state.
**Tradeoff:** The admin UI needs separate forms / flows for branding text vs branding images. In practice the UX is clearer this way (the image upload is its own focused interaction).

### 4. Branding asset bytes stay outside EVT

**Decision:** Server logo and banner binaries stay in the object store; configuration events only carry the current storage pointer and set/clear history.
**Why:** The event log should record durable domain state and audit history, not inline image bytes. This matches the broader asset model while still making the user-visible branding selection replayable from EVT.
**Tradeoff:** Backup/restore still needs the object-store bucket alongside EVT. A replay can reconstruct which logo or banner is current, but not the image bytes unless the asset store is preserved too.

### 5. Markdown in the welcome message, plain text in MOTD

**Decision:** The login welcome message supports markdown; the MOTD is plain text.
**Why:** The login page has room for formatted content (a paragraph, a link, a bit of structure). The MOTD is a one-line banner where formatting would add visual noise. Different surfaces, different needs.
**Tradeoff:** Operators may expect MOTD to support links. If demand emerges, a future tweak could allow a single link.

### 6. Blocked usernames as a dedicated security mutation

**Decision:** The blocked-usernames list is stored with server config, but edited through `admin.updateBlockedUsernames` rather than the general `updateServerConfig` mutation.
**Why:** The data is part of runtime config, but the workflow is a security/admin control rather than presentation copy. A dedicated mutation keeps the API boundary clear without introducing a full CRUD surface for a small newline-separated list. Blocked-username edits do not publish a member-visible live event; admins receive the updated value from the mutation response and future admin reads.
**Tradeoff:** Operators still edit the list as a text area, so very large lists could become awkward. None of the live deployments have lists big enough for this to matter.

### 7. Runtime text config has fixed size caps

**Decision:** Runtime-editable server text fields have fixed server-side size limits rather than operator-tunable limits.
**Why:** These values are public presentation/configuration text, not bulk content. Fixed limits keep event payloads and admin forms bounded while preserving enough room for normal operator usage.
**Tradeoff:** Operators who want unusually large welcome copy or blocklists have to shorten the content instead of raising a config value.

### 8. Edit window is a constant exposed via GraphQL, not a config field

**Decision:** `Server.messageEditWindowSeconds` is queryable but read-only. The value comes from a Go constant (`core.MessageEditWindow = 3 * time.Hour`); the GraphQL schema doesn't include it in `UpdateServerConfigInput`.
**Why:** The frontend needs to know the window to render countdown timers and disable the edit affordance at the right moment, so exposing it via GraphQL is necessary. But making it operator-tunable opens space for inconsistent UX across servers without clear benefit — and the value isn't sensitive enough to need server-by-server control.
**Tradeoff:** Operators who want a different window have to recompile. If demand emerges this can be promoted to a config field cheaply.

## Permissions

- `server.manage` — gates presentation, branding, and security-list mutations (`updateServerConfig`, `admin.updateBlockedUsernames`, `uploadServerLogo`, `uploadServerBanner`, `deleteServerLogo`, `deleteServerBanner`).

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-033 (event-sourced state with projections), ADR-035 (per-aggregate phased migration)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-004 (Message Editing & Deletion), FDR-008 (File Attachments & Video Processing), FDR-021 (Admin Dashboard & System Monitoring)
