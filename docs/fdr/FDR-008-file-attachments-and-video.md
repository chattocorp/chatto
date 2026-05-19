# FDR-008: File Attachments & Video Processing

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Users can attach files to messages — images, videos, documents — via drag-and-drop, paste, or file picker. Images are dimensioned and resizable on the fly via signed URLs. Videos and animated GIFs are transcoded asynchronously into web-friendly quality variants.

## Behavior

- The composer accepts files via drag-and-drop, paste, and a file picker button.
- Draft attachments persist across room switches inside the same session.
- Default upload size limits: 25 MB for general files, 100 MB for videos when video processing is enabled.
- Images are inspected for dimensions at upload time and can be resized at render time via URL parameters (width, height, fit mode).
- Videos and animated GIFs are processed in the background. The message posts immediately; the video player appears once processing completes.
- Processing status: PENDING → PROCESSING → COMPLETED (or FAILED). Failed videos still show the original message; the UI surfaces an error state for the attachment.
- A thumbnail is generated from an early video frame.
- Resized images can be cached as WebP with an auto-expiring cache.

## Design Decisions

### 1. Dual storage backends (NATS ObjectStore + S3)

**Decision:** Attachments can be stored in NATS ObjectStore (default, good for development and small deployments) or in an S3-compatible bucket (production-grade). The retrieval layer tries both, with the configured primary first.
**Why:** Self-hosters running a single binary shouldn't have to spin up S3 just to send a screenshot. Larger operators need durable, replicated object storage. Supporting both lets us serve both ends of the spectrum. See ADR-021.
**Tradeoff:** Migration between backends is operator-managed. The "try both" retrieval logic adds a tiny overhead but greatly improves the migration experience.

### 2. Video processing is asynchronous and best-effort

**Decision:** A video upload doesn't block the message post. The message is posted immediately; transcoding happens in the background; a completion event refreshes the frontend.
**Why:** Synchronous transcoding would tie up an HTTP request for tens of seconds to minutes. Users expect messages to send instantly. The cost of "the video appears a moment later" is much lower than "your chat froze for a minute".
**Tradeoff:** Users see a "processing" placeholder until transcoding finishes. Failures surface as an error state on the attachment, not as a failed message.

### 3. Animated GIFs go through the video pipeline

**Decision:** Animated GIFs are detected at upload and routed to the video transcoder rather than served as raw images.
**Why:** Animated GIF files are typically much larger than equivalent MP4s, and they're inefficient to decode in browsers. Transcoding to MP4 produces smaller, smoother playback.
**Tradeoff:** A static thumbnail is shown until processing finishes, even for GIFs that would have rendered immediately as-is. Worth it for the playback experience and bandwidth savings.

### 4. Quality variants are selected per source

**Decision:** Transcoding produces multiple H.264 MP4 variants whose target resolutions are derived from the source resolution. A 1080p source might yield 720p and 480p; a 480p source skips the higher tiers.
**Why:** Producing tiers higher than the source is pointless (upscaling is lossy without benefit). Producing tiers near the source is bandwidth waste for the common case.
**Tradeoff:** No HLS / adaptive bitrate streaming — the frontend picks a variant based on viewport and connection at the time of play. Acceptable for chat-context videos.

### 5. Originals are replaced by variants after transcoding

**Decision:** After transcoding succeeds, the original upload is deleted and only the encoded variants are kept.
**Why:** Originals are uncompressed-ish source files that nobody needs to download. Keeping them around would double or triple storage cost for no benefit.
**Tradeoff:** Lossy. If we ever wanted to re-transcode with new settings, we couldn't recover the original. Considered acceptable given the cost ratio.

### 6. Image transform URLs are HMAC-signed

**Decision:** Resize requests (`?w=...&h=...`) on image URLs include an HMAC signature; unsigned or tampered requests are rejected.
**Why:** Without signing, anyone could trigger arbitrarily many resize operations on attached images — a cheap denial-of-service vector. Signing means only URLs the server itself generated work. See ADR-023.
**Tradeoff:** URLs are longer. Sharing a raw image URL outside the app means losing the resize capability — but this is what we want.

## Permissions

No separate upload permission. Posting an attachment requires room membership and the relevant message-posting permission (`message.post` or `message.post-in-thread`).

## Related

- **ADRs:** ADR-021 (dual asset storage), ADR-023 (HMAC-signed image transform URLs)
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion)
