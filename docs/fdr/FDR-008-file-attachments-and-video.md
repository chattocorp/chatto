# FDR-008: File Attachments & Video Processing

**Status:** Active
**Last reviewed:** 2026-07-19

## Overview

Users can attach files to messages — images, videos, documents — via drag-and-drop, paste, or file picker. Images are dimensioned and resizable on the fly via signed URLs. When video processing is enabled, videos and animated GIFs are transcoded into web-friendly quality variants.

## Behavior

- The composer accepts files via drag-and-drop, paste, and a file picker button when the viewer has `message.attach`.
- Draft attachments persist across room switches inside the same session.
- Message attachments are uploaded through `chatto.api.v1.AssetUploadService` before message creation. The browser sends bounded unary chunks with SHA-256 checksums, then calls `MessageService.CreateMessage` with completed attachment asset IDs.
- Default upload size limits: 25 MB for general files, 100 MB for videos when video processing is enabled.
- Video uploads require server-side video processing to be enabled. When it is disabled, the composer rejects `video/*` files immediately and the message-post API rejects them before storage.
- Images are inspected for dimensions at upload time and can be resized at render time via URL parameters (width, height, fit mode). Public attachment and avatar APIs expose transform parameters; public server branding images expose canonical URLs only.
- The room timeline loads attachment images within 960×400 bounds, while the lightbox loads a separate derivative within 2048×2048 bounds. The untouched upload remains available through Open original and file-download actions.
- When enabled, videos and animated GIFs are processed by the current server process after asset creation and message submission scheduling. This is best-effort and intentionally simple until a real durable worker queue exists.
- Processing status: durable STARTED / COMPLETED / FAILED outcomes are stored as asset aggregate events (`evt.asset.{assetId}.*`) and delivered through the normal live EVT subscription path after room-membership authorization. There is no separate `video_processed` live event or new runtime KV state for video progress; failed videos still show the original message, and the UI falls back to the original upload when it is available.
- Processed video dimensions are display dimensions used for layout, not necessarily raw encoded storage pixels. Non-square-pixel and rotated sources should render in their intended orientation and aspect ratio. The room timeline displays every posted video uncropped at its measured aspect ratio, including unusual near-square dimensions and converted animated GIF loops. The player canvas is bounded to the available timeline width and a maximum height; for ratios beyond 9:16 or 16:9, it uses letterboxing so playback controls remain usable without cropping the video.
- A thumbnail is generated from an early video frame using the same display dimensions, so non-square-pixel sources do not persist squished or pillarboxed poster images.
- Opaque static attachment derivatives use JPEG quality 75. Derivatives that require transparency or animation use lossless WebP, and resized results can be held in the auto-expiring server cache.
- Browser media uses direct signed asset URLs. Relative attachment URLs are resolved against the server that owns the message or room-file item, so remote-server images, audio, and video can load without cross-site cookies or bearer headers. Chatto-streamed NATS objects are full, non-seekable responses; S3-backed passive media redirects to object storage for byte-range delivery.
- Clients refresh expiring attachment URL fields through room-scoped `AssetService.GetAsset` / `BatchGetAssets`, or by refetching the relevant timeline or room attachment-list page. The timeline, previews, lightbox, downloads, and room-files surfaces refresh before expiry and retry after media load errors.
- Active document attachment types such as HTML, XHTML, SVG, and XML can still be uploaded and viewed inline, but original-file responses are delivered in a browser sandbox so uploaded scripts do not run as trusted Chatto application code.
- The room sidebar Files panel lists current accessible attachments from both root messages and thread replies, grouped by date as Today, Yesterday, This week, This month, then older calendar months. Rows show a thumbnail or file-type icon, filename, and upload time; selecting a root-message attachment jumps the room timeline to that message, while selecting a thread-reply attachment opens the thread pane and highlights the reply.
- Deleting a message-owned attachment durably revokes access first, then removes its source/derivative bytes and transform-cache entries. A single elected cleanup worker retries failed physical deletion after process restart or replica handover.

## Design Decisions

### 1. Attachment uploads use chunked ConnectRPC sessions

**Decision:** Public message attachment uploads use `AssetUploadService`: `CreateUpload`, `UploadChunk`, `GetUpload`, `CompleteUpload`, and `CancelUpload`. Chunks are bounded unary ConnectRPC requests instead of browser client-streaming RPCs. Each upload declares the final file size and lowercase SHA-256 digest, each chunk carries its offset and chunk SHA-256, and `CompleteUpload` verifies the assembled digest before creating the durable asset. `CreateMessage` accepts only completed, room-matching attachment asset IDs.
**Why:** Attachments should remain inside the protobuf/ConnectRPC API surface instead of introducing a second REST upload endpoint. Unary chunks work with the current browser Connect stack and give resumable progress through the committed offset.
**Tradeoff:** Clients must hash the full file before completion and issue several RPCs for larger files. Temporary chunks and open sessions need cleanup if the browser disappears before completion.

### 2. Dual storage backends (NATS ObjectStore + S3)

**Decision:** Attachments can be stored in NATS ObjectStore (default, good for development and small deployments) or in an S3-compatible bucket (production-grade). Each asset records its storage backend and logical key at upload time; S3 deployments may add a configurable object-key prefix that is applied only at the S3 client boundary.
**Why:** Self-hosters running a single binary shouldn't have to spin up S3 just to send a screenshot. Larger operators need durable, replicated object storage. Supporting both lets us serve both ends of the spectrum. See ADR-021.
**Tradeoff:** Migration between backends or S3 prefixes is operator-managed. Stored asset keys remain prefix-free so moving objects between S3 base paths does not require rewriting Chatto metadata. NATS ObjectStore does not expose offset-aware reads, so Chatto serves those videos as complete non-seekable responses; deployments that require reliable duration detection and seeking must use S3.

### 3. Video processing is in-process and best-effort

**Decision:** The current implementation asks the process-local video service to spawn a goroutine from the message command path after `AssetCreatedEvent` has been appended and `AssetProcessingStartedEvent` has been recorded. It does not publish a NATS processing request and does not create runtime KV progress/claim records.
**Why:** The previous transient pub/sub worker path added queue semantics without giving us durable delivery or a clean multi-process claim model. A direct call is easier to reason about and easier to replace later with a real durable queue.
**Tradeoff:** This is intentionally best-effort. If the process crashes mid-transcode, boot recovery scans the EVT projection and retries unmanifested video assets. Multi-process duplicate work is possible until a future durable worker design adds explicit claims.

### 4. Animated GIFs go through the video pipeline

**Decision:** When video processing is enabled, animated GIFs are detected at upload and routed to the video transcoder rather than served as raw images. When video processing is disabled, GIFs remain allowed as image uploads.
**Why:** Animated GIF files are typically much larger than equivalent MP4s, and they're inefficient to decode in browsers. Transcoding to MP4 produces smaller, smoother playback.
**Tradeoff:** A static thumbnail is shown until processing finishes, even for GIFs that would have rendered immediately as-is. Worth it for the playback experience and bandwidth savings.

### 5. Quality variants are selected per source

**Decision:** Transcoding produces multiple H.264 MP4 variants whose target resolutions are derived from the source display resolution. A 1080p source might yield 720p and 480p; a 480p source skips the higher tiers. Processing metadata records display dimensions so clients can reserve the correct frame for sources with non-square pixels or rotation metadata. Generated thumbnails are rendered at display dimensions with square pixels. The chat timeline fits the complete video without cropping. For media more extreme than 9:16 or 16:9, it clamps the player canvas to that range and letterboxes the media so the controls have usable space.
**Why:** Producing tiers higher than the source is pointless (upscaling is lossy without benefit). Producing tiers near the source is bandwidth waste for the common case.
**Tradeoff:** No HLS / adaptive bitrate streaming yet — the frontend picks a variant based on viewport and connection at the time of play. Historical processed-video manifests are not rewritten when display-dimension handling improves; clients can still correct the rendered frame after media metadata loads. Videos whose meaningful content occupies only part of the encoded canvas retain that empty space because the player does not guess which parts are safe to crop. Adaptive streaming is tracked separately in GitHub issue #668.

### 6. Attachments are declared content; derivative manifests are durable events

**Decision:** `AssetCreatedEvent` records each uploaded or generated binary as a first-class `Asset` on `evt.asset.{assetId}.asset_created`. `Asset` carries inline storage and flat media metadata such as dimensions, duration, and bitrate; room scope and ownership context (`message`, `derivative`, `user_avatar`) live on `AssetCreatedEvent`. Uploaded pending attachment assets also record SHA-256, uploader, pending-attachment expiry, and video-processing hints. Processing outcome events reference asset IDs instead of embedding derivative asset metadata, and are appended to the same asset aggregate. Message posting imperatively invokes process-local video processing for newly uploaded video/animated-GIF assets after their asset creation events are appended; boot recovery derives any missed work from the asset and room projections and uses the same local processor path. After transcoding succeeds, the original upload is retained as source content, and generated thumbnails/MP4 variants are appended as derivative `AssetCreatedEvent`s whose owner points at the original asset. Durable failed/unavailable outcomes are recorded with `AssetProcessingFailedEvent.failure_code` and are mapped to stable client-facing failure reasons. Beta 0.1.0 histories that already wrote asset lifecycle facts under `evt.room.{roomId}.asset_*` remain readable through the asset projection's legacy subscription lanes.
**Why:** Attachments and video derivatives are content metadata, not runtime state. Making assets their own aggregates gives projections a single asset graph (`message -> original asset -> derivative assets`), keeps binary lifecycle facts out of the room aggregate, and lets future uploads exist outside messages without a parallel asset model. Keeping the original allows future re-encoding, and storing processing outcomes in EVT lets processed playback survive projection rebuilds and storage-boundary cleanup.
**Tradeoff:** Retaining originals costs more storage than the old replace-after-transcode behavior. Processing execution is still operational, not durable; a crash between the durable asset event and a completed processing outcome is repaired by boot recovery rather than by treating `AssetCreatedEvent` as a live subscriber trigger. Moving new writes from room aggregates to asset aggregates means older beta binaries must not be rolled back after new asset-subject writes have occurred; compatibility is maintained by this and later versions reading both subject shapes, not by rewriting history.

### 7. Attachment URLs are per-user signed capabilities

**Decision:** Public attachment APIs expose attachment media as stable asset paths plus per-user access tickets: `/assets/files/{assetId}?access={ticket}` for originals and `/assets/files/{assetId}/image/{width}x{height}/{fit}?access={ticket}` for image derivatives. Attachment, thumbnail, video thumbnail, and variant URLs also expose the ticket expiry so the client can refresh before or after a lazy-load miss. Every fetch verifies the signed user is still a member of the asset's room.
**Why:** Cross-origin `<img>` tags (used when the SPA loads attachments from a _remote_ registered server) can't carry session cookies (SameSite) or Authorization headers. A signed per-user access ticket lets browsers load remote attachments directly, while the room-membership check still auto-revokes access on kick/leave.
**Tradeoff:** The access ticket is a bearer capability — anyone holding it can fetch until the expiry passes or the signed user loses room membership. `core.AssetAccessTicketTTL` is currently **24 hours** so normal rendering, lazy loading, deferred media startup, and lightbox use are reliable across long-lived room views; clients still refresh URL fields through projected timeline, message, or attachment-list reads when tickets approach expiry or a media load fails. Protected asset responses use `private, no-store`, so browser-visible protected bytes are not reused as authorization state. Chatto streams protected bytes by default, with short-lived S3 redirects reserved for heavy passive originals such as video, audio, and large files. Rotating `[core.assets].signing_secret` invalidates all outstanding access tickets.

### 8. Active document attachments render in a browser sandbox

**Decision:** Original attachment responses for active document formats (HTML, XHTML, SVG, XML, and XML-derived media types) include a CSP sandbox and `nosniff`. S3-backed attachments of those types stream through Chatto instead of redirecting directly to a presigned object URL, so the same response policy applies.
**Why:** Some teams need to share these file types inline, but uploaded active content must not become trusted Chatto application code. A sandbox without same-origin privileges preserves the viewing use case while preventing the easiest same-origin stored-XSS path.
**Tradeoff:** Scripts, forms, top-level navigation, and same-origin APIs are restricted inside uploaded active documents. S3 deployments also lose the zero-copy redirect fast path for those active document types, while heavy passive originals can still use a short-lived S3 redirect after Chatto authorizes the request.

### 9. Room Files panel is a read projection, not durable attachment state

**Decision:** `Room.attachments` exposes a paginated list of current message attachments for a room. The read walks the visible room timeline projection, folds current message bodies, includes thread replies, preserves attachment order within each message, and sorts by newest message first.
**Why:** Files should disappear from the sidebar when their message body is retracted or the attachment is removed. Deriving the list from the existing room/message projections keeps the panel consistent with the timeline without adding duplicate durable state.
**Tradeoff:** There is no search or media filtering in this iteration. Clients page through the current list and refresh it after attachment-affecting live events.

### 10. Displayed images use bounded derivatives

**Decision:** Timeline images fit within 960×400 bounds and lightbox images fit within 2048×2048 bounds. Opaque static derivatives use JPEG quality 75, while transparency and animation continue to use lossless WebP. Original uploads remain unchanged and available separately.
**Why:** Timeline frames are much smaller than typical camera and screenshot uploads, and even full-screen viewing rarely benefits from transferring the source resolution. Separate display sizes reduce bandwidth without sacrificing the original file-sharing behavior.
**Tradeoff:** Opaque displayed images are lossy and capped in resolution. Transparent and animated images may see smaller savings because preserving their behavior requires lossless encoding.

### 11. Message-owned asset deletion is replayable

**Decision:** Request paths still attempt immediate NATS/S3 and transform-cache deletion, while the holder of the `asset_cleanup` lease incrementally consumes canonical `AssetDeletedEvent` facts and retries each idempotent cleanup independently. The asset ID locates the same aggregate's durable `AssetCreatedEvent`, which supplies storage metadata even after the in-memory projection drops it. Beta room-scoped histories without a canonical asset creation aggregate are skipped rather than probing guessed object keys.
**Why:** A committed deletion must remain recoverable when immediate storage cleanup fails, the process exits, or another replica committed the event. Resolving the immutable creation fact preserves that guarantee without duplicating storage metadata in the deletion event or depending on a mutable projection.
**Tradeoff:** Each cleanup requires an aggregate-history lookup, and a fresh worker replays prior deletion facts idempotently. Beta room-scoped events cannot gain the same guarantee without a migration or unsafe backend-key inference, and server branding/avatar cleanup remains outside this message-owned worker.

### 12. Cleanup health is shared and owner-visible

**Decision:** The elected cleanup worker publishes a privacy-safe heartbeat to `MEMORY_CACHE`. Owner-only admin diagnostics compare that record with the current lease and latest asset-deletion sequence, then expose initializing, healthy, retrying, stalled, or inactive state together with pending retry count/age and pass timestamps. The Server Admin System tab renders this status without exposing asset IDs, filenames, storage keys, raw errors, or a reclaimed-byte estimate.
**Why:** Automatic retry is only operationally useful when self-hosters can tell whether it completed, is catching up, or stopped reporting. Shared runtime state makes the result consistent when an admin request reaches a non-holder replica.
**Tradeoff:** The heartbeat is intentionally volatile and is reconstructed after loss. Counts describe queued retry work, not historical deletion totals, and idempotent cleanup cannot reliably attribute reclaimed bytes.

## Permissions

Posting an attachment requires room membership, the relevant message-posting permission (`message.post` or `message.post-in-thread`), and `message.attach`. The `message.attach` permission is configurable at server, group, and room scope and only gates message attachments; server branding uploads, user avatars, link previews, and attachment deletion use their existing checks.

Fresh servers seed `message.attach` for `everyone` so new deployments keep uploads enabled by default. Existing servers are not automatically backfilled after upgrade; operators should grant `message.attach` manually or through their chosen RBAC maintenance flow if existing rooms should keep allowing uploads.

## Related

- **ADRs:** ADR-021 (dual asset storage), ADR-023 (HMAC-signed image transform URLs), ADR-032 (self-describing signed attachment URLs), ADR-036 (runtime state in `RUNTIME_STATE`), ADR-041 (runtime units for optional processes), ADR-047 (direct ticketed asset URLs)
- **FDRs:** FDR-002 (Replies & Threads), FDR-004 (Message Editing & Deletion)
