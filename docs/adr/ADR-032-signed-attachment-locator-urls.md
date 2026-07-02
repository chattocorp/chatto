# ADR-032: Self-Describing Signed Attachment URLs

**Date:** 2026-05-23

**Status:** Superseded

**Update (2026-05-24):** The locator payload is now extended with the
calling user's ID (`u`) and a Unix-second expiry (`e`), both signed.
The HTTP handler no longer reads the session cookie — the signed
claims *are* the authorization (verified signature + non-expired +
signed user is still a room member). This unblocks cross-origin `<img>`
loads from remote-server SPAs, where neither cookies nor bearer
headers reach the asset endpoint. The "HMAC isn't the access control"
property called out in the original write-up is reversed by this
update; the signed locator is the capability now.

This was a known stopgap rather than a clean cross-origin auth design:
a leaked URL granted access for the full TTL, so locator URLs were kept to
**5 minutes** — short enough that URLs effectively only worked while a page
was being rendered. Considered and rejected at the time:
two scoped session cookies (third-party-cookie blocking risk), a
service worker that proxies remote-server fetches with the bearer
token (more upfront work but ages best — likely the long-term
direction), and a same-origin asset proxy (incompatible with the
"standalone frontend with no backing server" deployment shape, and
poor fit for GDPR data-residency expectations).

**Update (2026-06-08):** The Service Worker direction is now adopted
for stable `/assets/files/...` URLs in [ADR-039](ADR-039-service-worker-virtual-asset-urls.md).
The browser app rewrites those stable asset URLs to same-origin virtual
URLs once a Service Worker controls the page, while keeping direct
ticketed URLs as the explicit fallback for non-controlled clients,
and media Range redirects.

**Update (2026-07-02):** The signed locator route was removed before 0.4.0.
Protected attachments now use stable `/assets/files/{assetId}` paths with
`AssetAccessTicket` authorization for originals, image derivatives, thumbnails,
and video derivatives. Chatto streams protected bytes by default and only
redirects heavy passive originals to short-lived S3 URLs after authorizing the
stable asset request.

See [FDR-008](../fdr/FDR-008-file-attachments-and-video.md) and
[ADR-039](ADR-039-service-worker-virtual-asset-urls.md) for the current flow
and trade-offs.

## Context

At the time of this decision, attachment metadata (room ID, storage location, filename, dimensions) for a posted file lived embedded inside its owning `MessageBody` proto in `SERVER_BODIES`. The asset HTTP handler at `/assets/attachments/...` needed three things on every request:

1. **The room ID**, to authorize the caller against room membership.
2. **A pointer to the source-of-truth proto** (which body the attachment belongs to, or which projected video manifest event it came from in the case of a transcoded variant or thumbnail).
3. **The attachment ID**, to pick the right attachment out of that source proto.

The previous design (introduced in #575) answered all three by maintaining a second copy of the `Attachment` proto as a standalone record at `attachment.{roomId}.{attachmentId}` in `SERVER_BODIES`. The HTTP handler took the attachment ID from the URL, did a wildcard filter lookup on the records bucket, and read everything else off the matched record.

That design solved the original problem — the prior `roomID == "" → allow` shortcut on the S3 fast path was a real auth bypass — but it introduced a duplication: every `Attachment` proto was now persisted twice (once inside its body, once standalone). Two copies is a drift surface for any future mutation, and a write-amplification cost at upload time.

The options for fixing the duplication:

- **Standalone record is the source of truth.** Body holds attachment IDs only; the record holds the full proto. Inverts the relationship; loses the "a message edit re-renders its attachments naturally" invariant; requires N record fetches per message render.
- **Embedded copy is the source of truth, with a thin index.** Body owns the proto; the standalone bucket reduces to a pointer (e.g., body key + attachment ID). HTTP handler does index → body fetch → attachment scan. Still two writes per upload (binary + index), still a second KV namespace to maintain.
- **Encode the lookup pointer into the URL itself.** Eliminate the index entirely. URL carries the room ID, source-of-truth pointer (body key or video-origin attachment ID), and attachment ID; signed so attackers can't craft URLs for arbitrary IDs.

## Decision

Adopt the third option. **Attachment URLs are self-describing signed locators.** No separate index bucket; the URL itself carries everything the HTTP handler needs to authorize and serve. This decision has since been superseded by stable asset access tickets.

### URL shape

Original:

```
/assets/attachments/{base64payload}.{hexHMAC}
```

Transformed (image resize):

```
/assets/attachments/{base64payload}.{hexHMAC}/t/{base64params}.{hexHMAC}
```

`base64payload` decodes to JSON `{r, b?, v?, a}`:

- `r` — room ID (for the authz check)
- `b` — message body key `{userId}.{bodyId}` (set for body-embedded attachments; exactly one of `b` or `v` is set)
- `v` — video-origin attachment ID (set for variants and thumbnails; the processed video manifest is keyed by this ID)
- `a` — the attachment ID itself

`hexHMAC` is the first 16 bytes (32 hex chars) of HMAC-SHA256 of the base64 payload using `[core.assets].signing_secret` — the same secret as the transform-URL signing in [ADR-023](ADR-023-hmac-signed-image-transform-urls.md). The transform component, when present, retains its existing signing scheme with the locator string as its first resource ID.

### HTTP handler flow

1. Parse the path segment, verify the locator signature, decode the payload. Invalid signature → 403.
2. Look up the session cookie. No session → 401.
3. Resolve the room kind from `r` and verify `RoomMembershipExists(kind, userID, r)`. Not a member → 403.
4. Dispatch by source: `FindBodyAttachment(b, a)` for body attachments, `FindVideoOriginAttachment(v, a)` for variants/thumbnails. Missing → 404.
5. Read `Storage` off the returned proto and serve (presigned S3 redirect or NATS stream).

No standalone-record bucket is consulted.

### Source-of-truth changes

- **Body attachments**: `AssetCreatedEvent` is the durable asset creation event for the asset ID, owner, storage, and media metadata. Message-owned assets use the `message` owner branch. User avatars use the `user_avatar` owner branch. Future room-level uploads and server media should add their own owner branches when they start emitting asset events, without introducing another asset model. Message-owned assets are also embedded as attachments in `MessageBody.Attachments` so message rendering and signed URL generation have the back-pointer at hand. `PostMessage` emits asset creation events for new attachments, asks the process-local video service to spawn processing for assets selected by the command path, and leaves boot recovery to retry unmanifested video assets from the EVT projection.
- **Video variants and thumbnails**: derivatives are first-class `AssetCreatedEvent`s with a `derivative` owner pointing at the original asset. `AssetProcessingSucceededEvent.video` references those derivative asset IDs (`thumbnail_asset_id`, `AssetVideoVariant.asset_id`) instead of embedding full asset metadata.

## Consequences

- **One source of truth per attachment.** No write-amplification, no drift surface. A message-body mutation (rare today, but a possibility tomorrow) updates the only copy.
- **No second KV namespace.** `SERVER_BODIES` reverts to holding just bodies. Existing `attachment.*` records are historical dead weight; current boot no longer reads them as a migration source.
- **Authorization surface changed by the ticket update.** For the 2026-05-24 ticket-bearing locator flow, the signed URL was a short-lived capability: a leaked URL could be used until it expired or the signed user lost room membership. Stable `/assets/files/...` URLs use the same ticket capability model, but ADR-039 hides those tickets behind Service Worker virtual URLs in controlled browser sessions.
- **Forgery prevention.** The previous URL shape (`/assets/attachments/{id}`) let attackers probe arbitrary attachment IDs to enumerate the space. The locator URL requires a valid HMAC; only IDs the server has issued URLs for can be tested.
- **Longer URLs.** ~150 chars vs ~30. Irrelevant for our use case (URLs aren't human-typed and aren't shared as bare share-links outside the app).
- **URLs weren't individually revocable.** Rotating `[core.assets].signing_secret` invalidated *all* URLs at once. Ticketed URL leaks were bounded by expiry and the signed user's current room membership. If we ever want share links with single-use semantics, we'd extend the payload or add server-side revocation state; none of that is needed today.
- **Expiry became signed into issued browser URLs.** The original design did not include an expiry claim, but the 2026-05-24 update added one because signed browser asset URLs became standalone capabilities. Stable `/assets/files/...` tickets use the same bounded capability model.
- **Operational: secret rotation invalidates in-flight URLs.** Currently-loaded pages reload their asset URLs through projected timeline/attachment responses after a signing-secret rotation, so the impact is bounded to "until next refresh."
- **Cleaner internal API.** `GetAttachmentReader(*Attachment)` and `DeleteAttachmentFromStorage(*Attachment)` take the proto directly; the previous `(spaceID, attachmentID)` shape and the multi-layout S3 key probing are gone. Reads `Storage.S3.Key` straight off the proto.

## Related

- [ADR-023](ADR-023-hmac-signed-image-transform-urls.md) — HMAC-signed image transform URLs (same primitive, layered with this one for transforms)
- [ADR-021](ADR-021-dual-asset-storage.md) — Dual asset storage (now reads `Storage` directly instead of probing)
- [ADR-039](ADR-039-service-worker-virtual-asset-urls.md) — Service Worker virtual asset URLs with ticketed fallback
- [FDR-008](../fdr/FDR-008-file-attachments-and-video.md) — File attachments & video processing
