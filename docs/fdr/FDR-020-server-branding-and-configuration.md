# FDR-020: Server Branding & Configuration

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Operators can customize how their Chatto server presents itself. The server's name, description, welcome message, logo, banner, and message-of-the-day are all editable from the admin UI and visible to members and visitors. A small number of operational knobs (blocked usernames, etc.) live in the same config surface.

## Behavior

- **Server name** — appears in page titles, the chat header, and OG metadata. Defaults to "Chatto".
- **Description** — used in OG metadata for link previews when sharing the server URL.
- **Welcome message** — shown on the login page. Markdown is supported.
- **MOTD (message of the day)** — appears in a banner across the top of the chat surface for all members. Broadcasts to live clients when changed.
- **Logo** — shown in the chat header, login page, and OG image fallback. Uploaded as an image; resized variants served via signed URLs.
- **Banner** — shown on the login page and in OG previews. Same upload/serve pipeline as the logo.
- **Blocked usernames** — newline-separated list checked at signup. Matches are rejected before account creation.

## Design Decisions

### 1. One config surface, with nil-preserve semantics

**Decision:** `updateServer` accepts every config field as nullable. A nil input for a field leaves the existing value untouched; only fields the caller explicitly sets get changed.
**Why:** Partial-update semantics let UI forms send only changed fields without GET-then-PUT round-trips and without overwriting other fields with whatever defaults the form thinks they should be. It also makes API clients (CLI tools, scripts) safer.
**Tradeoff:** Two ways to "clear" a string field: empty-string vs unset. The API treats empty string as a clear and nil as "leave alone". Documented; consistent across all string fields.

### 2. Config changes broadcast as public live events

**Decision:** Changes publish to `live.server.config.*` and are delivered to every authenticated user.
**Why:** Server name, MOTD, logo — these are visible everywhere in the UI. Without live delivery, every member would see stale branding until refresh. The events have no privacy concern (everyone sees branding equally) so a broad broadcast is correct. See ADR-012.
**Tradeoff:** Every connected client gets every config change event, including ones for fields they may not render. Volume is low (operators don't tweak branding constantly) so this is fine.

### 3. Logo and banner have their own upload mutations

**Decision:** `uploadServerLogo` and `uploadServerBanner` are separate from `updateServer`. They accept multipart uploads, process the image, store it as an asset, and write the asset URL back into the server config.
**Why:** Image upload is a different shape from string config — multipart bodies, content-type validation, asset storage. Keeping them in their own mutations means `updateServer` stays simple and the upload path can reuse the standard asset-processing pipeline (FDR-008).
**Tradeoff:** The admin UI needs separate forms / flows for branding text vs branding images. In practice the UX is clearer this way (the image upload is its own focused interaction).

### 4. Markdown in the welcome message, plain text in MOTD

**Decision:** The login welcome message supports markdown; the MOTD is plain text.
**Why:** The login page has room for formatted content (a paragraph, a link, a bit of structure). The MOTD is a one-line banner where formatting would add visual noise. Different surfaces, different needs.
**Tradeoff:** Operators may expect MOTD to support links. If demand emerges, a future tweak could allow a single link.

### 5. Blocked usernames as a config field, not a separate management surface

**Decision:** The blocked-usernames list is one field on the server config, edited as a newline-separated text area.
**Why:** Few operators will blocklist many usernames. A separate "blocked usernames" admin page with add/remove operations would be overkill for the volume. A text area is the smallest viable UI.
**Tradeoff:** Large lists could become awkward to edit. None of the live deployments have lists big enough for this to matter.

### 6. Edit window is a constant exposed via GraphQL, not a config field

**Decision:** `Server.messageEditWindowSeconds` is queryable but read-only. The value comes from a Go constant (`core.MessageEditWindow = 3 * time.Hour`); the GraphQL schema doesn't include it in `UpdateServerConfigInput`.
**Why:** The frontend needs to know the window to render countdown timers and disable the edit affordance at the right moment, so exposing it via GraphQL is necessary. But making it operator-tunable opens space for inconsistent UX across servers without clear benefit — and the value isn't sensitive enough to need server-by-server control.
**Tradeoff:** Operators who want a different window have to recompile. If demand emerges this can be promoted to a config field cheaply.

## Permissions

- `server.manage` — gates every server-config mutation (`updateServer`, `uploadServerLogo`, `uploadServerBanner`).

## Related

- **ADRs:** ADR-012 (two-tier real-time events)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-004 (Message Editing & Deletion), FDR-008 (File Attachments & Video Processing), FDR-021 (Admin Dashboard & System Monitoring)
