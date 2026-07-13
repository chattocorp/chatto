# FDR-009: Link Previews

**Status:** Active
**Last reviewed:** 2026-07-13

## Overview

When a message contains a URL, Chatto can attach a preview card with the page's title, description, site name, and image. If the first URL directly returns an image, Chatto can instead import that image as an ordinary room attachment while preserving the URL in the message. Detection is client-driven while the user is composing, so the user sees and can dismiss either result before sending.

## Behavior

- The composer fetches a link preview as soon as the user has typed a complete URL.
- Only the first URL in a message gets a preview. There is no multi-preview layout.
- URLs inside code spans, code blocks, pre-formatted text, and blockquotes do not trigger link previews.
- YouTube URLs get a specialized embed-ready card without scraping the page.
- Direct JPEG, PNG, GIF, and static WebP URLs up to 5 MB are imported as pending room attachments when the author has `message.attach`. Animated GIFs use the ordinary attachment video-processing path when it is enabled.
- Imported direct images use the same presentation, image viewer, attachment limits, room Files index, permissions, and deletion lifecycle as uploaded images. The source URL remains ordinary message text.
- Dismissing an imported image removes it from the draft. An imported image that is never claimed by a message expires through the existing pending-attachment cleanup.
- A preview shows up in the composer with a dismiss button. Dismissing the preview prevents it from being attached to the sent message, and the dismissal is remembered for that URL during the composition session.
- When the server returns an OpenGraph or specialized preview to the composer, it also returns a short-lived opaque preview token.
- When a generic preview is sent, the client sends only the preview token. When an imported image is sent, the client supplies its pending attachment asset ID through the ordinary attachment field.
- Stored preview metadata is size-limited before storage: URL 2,048 bytes, title 300 bytes, description 1,000 bytes, image asset ID 15 bytes, site name 200 bytes, embed type 64 bytes, and embed ID 256 bytes.
- After posting, the message author can delete the preview from the message without deleting the message.

## Design Decisions

### 1. Preview fetching is client-driven, not server post-process

**Decision:** The composer queries for the preview during typing; the user explicitly accepts or dismisses before sending.
**Why:** Server-side preview generation after post is a worse user experience: previews appear seconds after the message, can't be dismissed before sending, and silently inflate every message with a URL. Client-driven puts control in the user's hands.
**Tradeoff:** Each compose session may make a preview query even if the user ends up not sending. Cost is small and capped (one URL per message).

### 2. One preview per message, first URL only

**Decision:** Only the first URL in a message gets a preview card. Subsequent URLs render as plain links.
**Why:** Multi-preview layouts (Slack-style) blow up the message height and are usually visual clutter. One preview captures the most-likely-relevant link.
**Tradeoff:** Messages that genuinely need to highlight several links can't. Authors can split into multiple messages.

### 3. 24-hour positive cache, 1-hour negative cache

**Decision:** Successful previews cache for 24 hours; failed fetches cache as failures for 1 hour.
**Why:** Web pages change, so unlimited positive caching would mean stale OpenGraph data. A 24-hour TTL is the usual balance. Negative caching is shorter because transient outages shouldn't lock us out for a day; but some caching is needed to avoid hammering unreachable sites.
**Tradeoff:** A site that updates its OpenGraph metadata sees stale previews for up to a day.

### 4. SSRF-safe fetcher with connection-time IP validation

**Decision:** All URL fetches go through an HTTP client that blocks private/loopback IP ranges. The IP check happens at connection time, not pre-check, to prevent DNS rebinding.
**Why:** Without these protections, a maliciously crafted URL could make the server fetch internal services. A pre-fetch DNS lookup is bypassable via rebinding; connection-time enforcement is not.
**Tradeoff:** Some legitimate internal-network use cases (preview an intranet wiki page) don't work. Operators who need that can disable previews entirely.

### 5. Generic preview images are downloaded, resized, and stored as persisted assets

**Decision:** OpenGraph preview images are fetched once, resized to 1200×630 max, converted to WebP, and stored through the configured persisted asset backend (S3 when configured, otherwise NATS `SERVER_ASSETS`). Sent message bodies carry the preview image as `LinkPreview.image_asset` (`AssetRecord`); `image_asset_id` remains as a compatibility field for older stored previews.
**Why:** Hot-linking preview images from third-party sites means broken previews when those sites change URLs, plus a privacy leak (the third party sees each preview fetch). Storing locally fixes both.
**Tradeoff:** Per-server storage cost. Acceptable given the small fixed size cap and the fact that posted message previews should not lose images just because a cache expired.

### 6. Direct images enter the ordinary attachment lifecycle

**Decision:** When the first URL directly returns a supported image and the composer supplies a room, the server stages the fetched bytes as a pending room attachment. Posting claims it through the normal attachment path; abandoning it leaves it eligible for existing pending-asset expiry and cleanup. Direct-image results are not stored in the shared link-preview cache.
**Why:** A single attachment lifecycle consolidates sizing, authorization, storage, GIF processing, image viewing, Files indexing, deletion, and cleanup. It also bounds abandoned remote imports instead of creating a separate durable preview-asset surface.
**Tradeoff:** Direct image expansion requires `message.attach`, counts toward attachment limits, and makes the imported copy a room file. Fetching the same image in separate drafts may repeat work because direct results bypass the shared preview cache.

### 7. Message posting uses server-issued preview tokens

**Decision:** `MessageService.FetchLinkPreview` returns display metadata plus a short-lived opaque token. `MessageService.CreateMessage` accepts only that token for link previews and never accepts client-provided title, description, image asset ID, site name, or embed metadata.
**Why:** The composer still needs preview metadata to let the author accept or dismiss the card, but trusting the same client to send final metadata would allow spoofed titles, descriptions, and image asset references.
**Tradeoff:** Posting a preview depends on the cached server preview and token still being valid. If either expires, the client must fetch the preview again before sending it.

### 8. Stored preview metadata is bounded

**Decision:** Preview metadata attached to a sent message is accepted only within generous per-field size limits.
**Why:** Even though metadata is server-fetched, it is persisted with the message body. Bounding it keeps a single message from carrying arbitrarily large URL metadata.
**Tradeoff:** A page with unusually large metadata requires the server fetch/cache layer to trim or omit the preview before sending.

## Permissions

- Any authenticated user can fetch a link preview.
- Importing a direct image requires the ordinary posting permission for the target message plus `message.attach`.
- Only the message author can delete a preview from their message.

## Related

- **FDRs:** FDR-008 (File Attachments & Video Processing)
