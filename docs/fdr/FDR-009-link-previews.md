# FDR-009: Link Previews

**Status:** Active
**Last reviewed:** 2026-11-01

## Overview

When a message contains a URL, Chatto can attach a preview card with the page's title, description, site name, and image. Previews are fetched client-driven while the user is composing — the user sees the preview before sending and can dismiss it.

## Behavior

- Automatic URL detection requires `http://`, `https://`, or `www.`. The client uses HTTPS for `www.` URLs. Bare domains and filenames do not trigger preview requests. Explicit Markdown links to HTTP or HTTPS URLs can still trigger a preview.
- The composer fetches a link preview as soon as the user has typed a complete eligible URL.
- Only the first eligible URL in a message gets a preview. There is no multi-preview layout.
- Server, room, and user URLs recognised by the client's entity pills do not get a preview attachment. This includes other servers in the client catalogue. A later eligible web URL can get the preview instead. Unknown servers keep normal URL previews; the client does not contact them to discover whether they run Chatto.
- URLs inside code spans, code blocks, pre-formatted text, and blockquotes do not trigger link previews.
- An HTTP or HTTPS link that starts with an angle bracket, such as `<https://example.com>`, stays clickable and does not trigger a link preview. The closing angle bracket is optional. This behavior works in both message editors. A later eligible URL can get the preview instead.
- YouTube URLs get a specialized embed-ready card without scraping the page.
- Posted previews for supported Vimeo video URLs show the Vimeo player. This also applies to stored generic previews. Public video links, player links, channel links, group video links, and showcase video links are supported. Unlisted links keep their privacy hash. The player loads lazily with Vimeo's Do Not Track option and without automatic playback. Loading it connects the reader's browser to Vimeo and reveals the reader's IP address to Vimeo. Other Vimeo pages keep the normal link card.
- Supported public social-post URLs use a native Chatto card populated from provider data. The card can include the provider, author, post text, attached images, an embedded website card, and one quoted post with its own common media. Bluesky and Mastodon are supported providers. Mastodon content warnings and accepted quote posts use the same common fields; warned text and media stay concealed until the reader reveals them, and boosts show the original post without boost attribution. If structured post data is unavailable, the post falls back to a normal link preview.
- For video and animated-GIF attachments on a social post, the card shows the static thumbnail from the provider as an image. Chatto does not download or play the video. The card does not show audio attachments or video attachments that have no thumbnail.
- A preview shows up in the composer with a dismiss button. The composer shows a compact preview of one row: a thumbnail, the site, and the title. It does not load video players or show the full social-post card while the user types. A social post with a content warning shows no image or post text in this row. A message-link preview in the composer shows about one line of the message body and shows its attachments as one line of text. Dismissing the preview prevents it from being attached to the sent message, and the dismissal is remembered for that URL during the composition session.
- When the server returns a preview to the composer, it also returns a short-lived opaque preview token.
- When the message is sent, the client sends only the preview token. The server resolves the token to cached, server-fetched metadata and stores that metadata as part of the message body.
- Stored preview metadata is size-limited before storage: URL 2,048 bytes, title 300 bytes, description 1,000 bytes, image asset ID 15 bytes, site name 200 bytes, embed type 64 bytes, and embed ID 256 bytes. Structured social-post fields use the corresponding text, URL, and asset limits, carry at most four images per post, and allow only one quoted-post level.
- After posting, the message author can delete the preview from the message without deleting the message.

## Design Decisions

### 1. Preview fetching is client-driven, not server post-process

**Decision:** The composer queries for the preview during typing; the user explicitly accepts or dismisses before sending.
**Why:** Server-side preview generation after post is a worse user experience: previews appear seconds after the message, can't be dismissed before sending, and silently inflate every message with a URL. Client-driven puts control in the user's hands.
**Tradeoff:** Each compose session may make a preview query even if the user ends up not sending. Cost is small and capped (one URL per message).

### 2. One preview per message, first URL only

**Decision:** Only the first eligible URL in a message gets a preview card. Subsequent URLs render as plain links.
**Why:** Multi-preview layouts (Slack-style) blow up the message height and are usually visual clutter. One preview captures the most-likely-relevant link.
**Tradeoff:** Messages that genuinely need to highlight several links can't. Authors can split into multiple messages.

### 3. 24-hour positive cache, 1-hour negative cache

**Decision:** Successful previews cache for 24 hours; failed fetches cache as failures for 1 hour.
**Why:** Web pages change, so unlimited positive caching would mean stale OpenGraph data. A 24-hour TTL is the usual balance. Negative caching is shorter because transient outages shouldn't lock us out for a day; but some caching is needed to avoid hammering unreachable sites.
**Tradeoff:** A site that updates its OpenGraph metadata sees stale previews for up to a day.

### 4. SSRF-safe fetcher with connection-time IP validation

**Decision:** Page and image fetches block private, loopback, shared-address, and
special-use destinations. The policy includes `100.64.0.0/10`, documentation,
benchmark, multicast, and reserved ranges. Standard NAT64 ranges
(`64:ff9b::/96` and `64:ff9b:1::/48`), Teredo, and 6to4 are blocked in full.
IPv4-mapped IPv6 addresses use the IPv4 policy. Each new connection checks all
DNS answers and rejects the destination if any answer is blocked. Connections
use the checked IP directly. Redirects use the same checks.

**Why:** A URL must not let a user make the server contact an internal service.
Connection-time checks and direct IP dialing prevent a second DNS lookup from
changing the destination. Translation ranges are blocked because an IPv6
address in those ranges can lead to a private IPv4 destination.

**Tradeoff:** Intranet previews and previews reached only through standard
NAT64 translation do not work. Deployment-specific translation routes still
need network egress controls. Existing cached previews and posted cards keep
their normal lifetimes; this policy controls new network connections.

### 5. Preview images are downloaded, resized, and stored as persisted assets

**Decision:** Preview images are fetched once, resized to 1200×630 max, converted to WebP, and stored through the configured persisted asset backend (S3 when configured, otherwise NATS `SERVER_ASSETS`). Sent message bodies carry the preview image as `LinkPreview.image_asset` (`AssetRecord`); `image_asset_id` remains as a compatibility field for older stored previews.
**Why:** Hot-linking preview images from third-party sites means broken previews when those sites change URLs, plus a privacy leak (the third party sees each preview fetch). Storing locally fixes both.
**Tradeoff:** Per-server storage cost. Acceptable given the small fixed size cap and the fact that posted message previews should not lose images just because a cache expired.

### 6. Message posting uses server-issued preview tokens

**Decision:** `MessageService.FetchLinkPreview` returns display metadata plus a short-lived opaque token by default. Display-only callers set `omit_preview_token` to avoid creating an unused composer token. `MessageService.CreateMessage` accepts only that token for link previews and never accepts client-provided title, description, image asset ID, site name, or embed metadata.
**Why:** The composer still needs preview metadata to let the author accept or dismiss the card, but trusting the same client to send final metadata would allow spoofed titles, descriptions, and image asset references.
**Tradeoff:** Posting a preview depends on the cached server preview and token still being valid. If either expires, the client must fetch the preview again before sending it.

### 7. Stored preview metadata is bounded

**Decision:** Preview metadata attached to a sent message is accepted only within generous per-field size limits.
**Why:** Even though metadata is server-fetched, it is persisted with the message body. Bounding it keeps a single message from carrying arbitrarily large URL metadata.
**Tradeoff:** A page with unusually large metadata requires the server fetch/cache layer to trim or omit the preview before sending.

### 8. Social posts use bounded, provider-neutral snapshots

**Decision:** Provider adapters resolve recognized public social posts into one bounded snapshot containing common presentation data. A snapshot may include one quoted post, but quotes within that post are omitted. Chatto persists the snapshot and its images, then renders it with native card components. Bluesky and Mastodon use the same snapshot and native card; the existing OpenGraph card remains the fallback. Mastodon normally uses origin-bound oEmbed discovery. Federated proxy permalinks that lack oEmbed metadata instead require origin-bound public instance metadata before Chatto trusts the server's status API.
**Why:** A provider-neutral snapshot keeps durable message data and the public API independent from any one social network. Native cards stay visually consistent with the timeline, avoid loading a full third-party website inside a message, and prevent client-side provider requests when reading history.
**Tradeoff:** Chatto deliberately implements only a common subset of social-post presentation. Provider-specific features require explicit additive support, changes made to the original post are not reflected after the snapshot is stored, the current snapshot does not identify who boosted or reposted a post, and a video thumbnail looks the same as a photo because the snapshot does not record the media type.

### 9. The composer shows a compact preview

**Decision:** The composer shows a compact link preview of one row. Posted messages show the full card.

**Rationale:** On a phone, the software keyboard leaves little height for the page. A full preview card with a large image can fill this height and push the text field out of view. The compact preview keeps the text field visible. It also prevents a connection to a third-party player while the user types.

## Permissions

- Any authenticated user can fetch a link preview.
- Only the message author can delete a preview from their message.

## Related

- **FDRs:** FDR-008 (File Attachments & Video Processing)
