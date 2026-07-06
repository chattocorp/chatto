# ADR-039: Service Worker Virtual Asset URLs with Ticketed Fallback

**Date:** 2026-06-08

**Status:** Superseded by [ADR-047](ADR-047-direct-ticketed-asset-urls.md)

**Update (2026-07-05):** Chatto no longer uses Service Worker virtual asset
URLs. Browser media now uses direct ticketed asset URLs and foreground clients
refresh those URLs before expiry or after media load errors. This ADR remains as
historical context for the removed privacy-hardening design.

## Context

Chatto's browser client can connect to multiple registered servers. A page served by one Chatto server may render attachment media from another registered server. Native browser media elements such as `<img>` and `<video>` cannot reliably attach Chatto's registered-server bearer token to those cross-origin subresource requests, and SameSite cookie behavior makes relying on remote-server cookies brittle.

To make remote attachments render, Chatto introduced per-user asset access tickets on stable asset paths such as `/assets/files/{assetId}?access={ticket}` and `/assets/files/{assetId}/image/{width}x{height}/{fit}?access={ticket}`. The backend verifies the ticket signature, expiry, and current room membership on each fetch. That makes remote media work, but the URL is still a bearer capability until it expires or the signed user loses access. A user can copy the rendered asset URL out of the DOM and share it with someone else.

We want the normal web app to avoid putting bearer asset tickets in rendered markup, while preserving two properties:

1. Assets should still render in browsers or modes where Service Workers are unavailable, disabled, not yet controlling the page, or cleared by browser storage policy.
2. Protected asset bytes should not be reused from browser caches; ticket expiry, deletion, and room-membership revocation should be checked on the next fetch.

## Decision

In Service Worker-controlled browser sessions, the frontend renders stable asset URLs through a same-origin virtual namespace:

```text
/__chatto/assets/{serverId}/assets/files/{assetId}[...]
```

The virtual URL is not a bearer credential. It only resolves inside a Chatto client whose Service Worker has received the matching server registration and hidden ticketed target from the app. The frontend registers a hidden mapping from the virtual URL to the current ticketed target URL, and the worker resolves fetches by using that mapping or, for same-origin cookie sessions, by rebuilding the target from the registered server URL and asset path.

For full `GET` requests, the worker fetches the hidden ticketed target and adds `X-Chatto-Asset-Proxy: 1`. The backend treats that proxy header as a request to stream the asset through Chatto instead of redirecting originals to S3. Protected asset responses use `private, no-store`, and the worker does not cache response bodies; every protected asset load reaches the server so ticket expiry, deletion, and room-membership revocation are rechecked.

The restart and fallback paths are explicit and intentional:

- If a controlled page is still open but the browser has terminated and restarted the idle Service Worker, the worker asks open window clients to resend registered servers and the requested virtual target mapping before failing the fetch.
- If `navigator.serviceWorker.controller` is absent, asset URL helpers return the existing direct ticketed asset URL.
- Media `Range` requests are not cached by the asset proxy. The worker redirects them to the hidden target URL so browser media playback keeps current Range behavior until Chatto has deliberate Range streaming through the proxy path.
- If the browser clears Service Worker state or Cache Storage, the app resynchronizes registered servers after the worker controls the page again. Until then, direct ticketed URLs remain the compatibility path.

## Consequences

- **Copied DOM URLs are no longer access tickets in the main app path.** A copied `/__chatto/assets/...` URL is same-origin and only useful inside a controlled Chatto browser session with the matching registered server and hidden ticket mapping.
- **Ticketed URLs remain part of the architecture.** They are still emitted by projected API responses and still authorize non-Service-Worker clients, first-load/non-controlled pages, legacy clients, and Range redirects. Their TTL and membership checks remain important security controls.
- **Open pages are the recovery source for worker restarts.** Service Worker globals are volatile. The client keeps the virtual-target mappings it registered and answers worker resync requests so lazy-loaded assets can recover after an idle worker restart without persisting tickets to durable browser storage.
- **The fallback is less private but more compatible.** Browsers without working Service Workers keep rendering assets using the pre-existing ticketed URL behavior. This is acceptable because it is the old behavior, not a new breakage, and because ticket expiry plus membership checks still bound exposure.
- **Protected asset bodies are not cached by the worker.** Server-side image resize caching remains available for expensive transforms, but browser-visible protected responses must be revalidated through the server so authorization changes take effect on the next fetch.
- **API bearer tokens stay out of Service Worker asset state.** The worker does not receive registered-server API tokens and does not add `Authorization` to proxied asset requests. That keeps token exposure concentrated in the foreground API clients and avoids persisting token-derived asset responses under a token-agnostic cache key.
- **Safari, private browsing, and managed browsers may see less benefit.** Service Worker or Cache Storage behavior can be unavailable, ephemeral, or aggressively evicted. The feature must remain an enhancement with a reliable direct-URL fallback.
- **Range support remains deliberately conservative.** Redirecting Range requests avoids partially reimplementing media streaming in the Service Worker. A future backend proxy streaming design can replace this fallback when we need cacheable or non-ticketed Range playback.
- **Backend asset serving now has a proxy mode.** `X-Chatto-Asset-Proxy: 1` is a private contract between the browser Service Worker and Chatto servers. It must be allowed by CORS and included in `Vary` so proxy-streamed responses do not mix with presigned-redirect responses.
