# FDR-008: File Attachments & Video Processing

**Status:** Active
**Last reviewed:** 2026-06-05

## Overview

Users can attach files to messages — images, videos, documents — via drag-and-drop, paste, or file picker. Images are dimensioned and resizable on the fly via signed URLs. When video processing is enabled, videos and animated GIFs are transcoded into web-friendly quality variants.

## Behavior

- The composer accepts files via drag-and-drop, paste, and a file picker button.
- Draft attachments persist across room switches inside the same session.
- Default upload size limits: 25 MB for general files, 100 MB for videos when video processing is enabled.
- Video uploads require server-side video processing to be enabled. When it is disabled, the composer rejects `video/*` files immediately and the GraphQL mutation rejects them before storage.
- Images are inspected for dimensions at upload time and can be resized at render time via URL parameters (width, height, fit mode). GraphQL exposes transform parameters for attachments and user avatars; public server branding images expose canonical URLs only.
- When enabled, videos and animated GIFs are processed by the current server process after asset creation and message submission scheduling. This is best-effort and intentionally simple until a real durable worker queue exists.
- Processing status: durable COMPLETED / FAILED outcomes are stored as room events. There is no new runtime KV state for video progress; failed videos still show the original message, and the UI falls back to the original upload when it is available.
- A thumbnail is generated from an early video frame.
- Resized images can be cached as WebP with an auto-expiring cache.
- In Service Worker-controlled browser sessions, stable asset URLs are rendered as same-origin virtual URLs and proxied to the owning server with the user's registered server credentials. Successful full responses are cached privately in the browser; media `Range` requests bypass that cache.

## Design Decisions

### 1. Dual storage backends (NATS ObjectStore + S3)

**Decision:** Attachments can be stored in NATS ObjectStore (default, good for development and small deployments) or in an S3-compatible bucket (production-grade). The retrieval layer tries both, with the configured primary first.
**Why:** Self-hosters running a single binary shouldn't have to spin up S3 just to send a screenshot. Larger operators need durable, replicated object storage. Supporting both lets us serve both ends of the spectrum. See ADR-021.
**Tradeoff:** Migration between backends is operator-managed. The "try both" retrieval logic adds a tiny overhead but greatly improves the migration experience.

### 2. Video processing is in-process and best-effort

**Decision:** The current implementation asks the process-local video service to spawn a goroutine from the message command path after `AssetCreatedEvent` has been appended and `AssetProcessingStartedEvent` has been recorded. It does not publish a NATS processing request and does not create runtime KV progress/claim records.
**Why:** The previous transient pub/sub worker path added queue semantics without giving us durable delivery or a clean multi-process claim model. A direct call is easier to reason about and easier to replace later with a real durable queue.
**Tradeoff:** This is intentionally best-effort. If the process crashes mid-transcode, boot recovery scans the EVT projection and retries unmanifested video assets. Multi-process duplicate work is possible until a future durable worker design adds explicit claims.

### 3. Animated GIFs go through the video pipeline

**Decision:** When video processing is enabled, animated GIFs are detected at upload and routed to the video transcoder rather than served as raw images. When video processing is disabled, GIFs remain allowed as image uploads.
**Why:** Animated GIF files are typically much larger than equivalent MP4s, and they're inefficient to decode in browsers. Transcoding to MP4 produces smaller, smoother playback.
**Tradeoff:** A static thumbnail is shown until processing finishes, even for GIFs that would have rendered immediately as-is. Worth it for the playback experience and bandwidth savings.

### 4. Quality variants are selected per source

**Decision:** Transcoding produces multiple H.264 MP4 variants whose target resolutions are derived from the source resolution. A 1080p source might yield 720p and 480p; a 480p source skips the higher tiers.
**Why:** Producing tiers higher than the source is pointless (upscaling is lossy without benefit). Producing tiers near the source is bandwidth waste for the common case.
**Tradeoff:** No HLS / adaptive bitrate streaming yet — the frontend picks a variant based on viewport and connection at the time of play. Adaptive streaming is tracked separately in GitHub issue #668.

### 5. Attachments are declared content; derivative manifests are durable events

**Decision:** `AssetCreatedEvent` records each uploaded or generated binary as a first-class `Asset`. `Asset` carries inline storage and flat media metadata such as dimensions, duration, and bitrate; ownership context (`message`, `derivative`, `user_avatar`) lives on `AssetCreatedEvent`. Processing outcome events reference asset IDs instead of embedding derivative asset metadata. Message posting imperatively invokes process-local video processing for newly uploaded video/animated-GIF assets after their asset creation events are appended; boot recovery derives any missed work from the EVT projection and uses the same local processor path. After transcoding succeeds, the original upload is retained as source content, and generated thumbnails/MP4 variants are appended as derivative `AssetCreatedEvent`s whose owner points at the original asset. Durable failed/unavailable outcomes are recorded with `AssetProcessingFailedEvent.failure_code`; GraphQL maps the source-missing enum to the stable `original_missing` reason for clients.
**Why:** Attachments and video derivatives are content metadata, not runtime state. Making derivatives normal assets gives projections a single asset graph (`message -> original asset -> derivative assets`) and lets future uploads exist outside messages without a parallel asset model. Keeping the original allows future re-encoding, and storing processing outcomes in EVT lets processed playback survive projection rebuilds and storage-boundary cleanup.
**Tradeoff:** Retaining originals costs more storage than the old replace-after-transcode behavior. Processing execution is still operational, not durable; a crash between the durable asset event and a completed processing outcome is repaired by boot recovery rather than by treating `AssetCreatedEvent` as a live subscriber trigger. Legacy processed videos may still have missing originals because the old pipeline deleted them; migration backfills asset creation events from message attachments and imports existing variant manifests so they remain playable.

### 6. Attachment URLs are per-user signed capabilities

**Decision:** GraphQL exposes attachment media as stable asset paths plus per-user access tickets: `/assets/files/{assetId}?access={ticket}` for originals and `/assets/files/{assetId}/image/{width}x{height}/{fit}?access={ticket}` for image derivatives. `Attachment.assetUrl` / `thumbnailAssetUrl`, video thumbnail URLs, and variant URLs also expose the ticket expiry so the client can refresh before or after a lazy-load miss. Every fetch verifies the signed user is still a member of the asset's room.
**Why:** Cross-origin `<img>` tags (used when the SPA loads attachments from a *remote* registered server) can't carry session cookies (SameSite) or Authorization headers. A signed per-user access ticket lets browsers load remote attachments directly, while the room-membership check still auto-revokes access on kick/leave.
**Tradeoff:** The access ticket is still a bearer capability — anyone holding it can fetch until the expiry passes or the signed user loses room membership. `core.AssetAccessTicketTTL` is currently **1 hour** so normal rendering, lazy loading, media startup, and lightbox use are reliable; clients refresh URL fields via GraphQL when tickets approach expiry or a media load fails. In Service Worker-controlled sessions, the DOM uses non-portable same-origin virtual URLs under `/__chatto/assets/{serverId}/...`; the worker keeps the ticketed target URL out of markup, fetches through the registered server credentials when possible, and caches full successful responses in a private per-browser cache. This removes the "lazy copy URL grants access" problem for the main web app, while keeping ticketed URLs as fallback for non-Service-Worker clients and for media `Range` redirects. Legacy `/assets/attachments/{signedLocator}` URLs remain supported for compatibility/internal fallback and use the shorter 5-minute locator TTL. Rotating `[core.assets].signing_secret` invalidates all outstanding tickets and legacy locators at once.

## Permissions

No separate upload permission. Posting an attachment requires room membership and the relevant message-posting permission (`message.post` or `message.post-in-thread`).

## Related

- **ADRs:** ADR-021 (dual asset storage), ADR-023 (HMAC-signed image transform URLs), ADR-032 (self-describing signed attachment URLs), ADR-036 (runtime state in `RUNTIME_STATE`), ADR-039 (Service Worker virtual asset URLs with ticketed fallback)
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion)
