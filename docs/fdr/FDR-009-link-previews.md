# FDR-009: Link Previews

**Status:** Active
**Last reviewed:** 2026-07-04

## Overview

When a message contains a URL, Chatto can attach a preview card with the page's title, description, site name, and image. Previews are fetched client-driven while the user is composing — the user sees the preview before sending and can dismiss it.

## Behavior

- The composer fetches a link preview as soon as the user has typed a complete URL.
- Only the first URL in a message gets a preview. There is no multi-preview layout.
- URLs inside code spans, code blocks, pre-formatted text, and blockquotes do not trigger link previews.
- YouTube URLs get a specialized embed-ready card without scraping the page.
- A preview shows up in the composer with a dismiss button. Dismissing the preview prevents it from being attached to the sent message, and the dismissal is remembered for that URL during the composition session.
- When the server returns a preview to the composer, it also returns a short-lived opaque preview token.
- When the message is sent, the client sends only the preview token. The server resolves the token to cached, server-fetched metadata and stores that metadata as part of the message body.
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

### 5. Preview images are downloaded, resized, and stored as persisted assets

**Decision:** Preview images are fetched once, resized to 1200×630 max, converted to WebP, and stored through the configured persisted asset backend (S3 when configured, otherwise NATS `SERVER_ASSETS`). Sent message bodies carry the preview image as `LinkPreview.image_asset` (`AssetRecord`); `image_asset_id` remains as a compatibility field for older stored previews.
**Why:** Hot-linking preview images from third-party sites means broken previews when those sites change URLs, plus a privacy leak (the third party sees each preview fetch). Storing locally fixes both.
**Tradeoff:** Per-server storage cost. Acceptable given the small fixed size cap and the fact that posted message previews should not lose images just because a cache expired.

### 6. Message posting uses server-issued preview tokens

**Decision:** `MessageService.FetchLinkPreview` returns display metadata plus a short-lived opaque token. `MessageService.CreateMessage` accepts only that token for link previews and never accepts client-provided title, description, image asset ID, site name, or embed metadata.
**Why:** The composer still needs preview metadata to let the author accept or dismiss the card, but trusting the same client to send final metadata would allow spoofed titles, descriptions, and image asset references.
**Tradeoff:** Posting a preview depends on the cached server preview and token still being valid. If either expires, the client must fetch the preview again before sending it.

### 7. Stored preview metadata is bounded

**Decision:** Preview metadata attached to a sent message is accepted only within generous per-field size limits.
**Why:** Even though metadata is server-fetched, it is persisted with the message body. Bounding it keeps a single message from carrying arbitrarily large URL metadata.
**Tradeoff:** A page with unusually large metadata requires the server fetch/cache layer to trim or omit the preview before sending.

## Permissions

- Any authenticated user can fetch a link preview.
- Only the message author can delete a preview from their message.

## Related

- **FDRs:** FDR-008 (File Attachments & Video Processing)
