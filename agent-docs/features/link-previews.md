# Link Previews

## Overview

- When a message contains a URL, a preview card can be shown with the page's title, description, site name, and image.
- Only the first URL in a message gets a preview (max one per message).
- YouTube URLs are detected and rendered as embed-ready cards without fetching the page.

## Lifecycle

- Link previews are **client-driven**: the composer fetches the preview via a query while the user is typing, not on message post.
- The preview card appears in the composer with a dismiss button. Dismissed URLs won't re-preview.
- On send, the preview data is included in the message input. The server stores it as part of the message body.
- After posting, the message author can delete the preview from the message.

## Caching

- Successful previews are cached for 24 hours.
- Failed fetches are negatively cached for 1 hour (prevents hammering unreachable sites).
- Preview images are downloaded, resized to 1200×630 max, converted to WebP, and stored as server assets.

## Security

- All URL fetches go through an SSRF-safe HTTP client that blocks private/loopback IP ranges.
- IP validation happens at connection time (not pre-check) to prevent DNS rebinding attacks.
- Image URLs from signed asset paths use HMAC validation to prevent parameter tampering.

## Permissions

- Any authenticated user can fetch a link preview.
- Only the message author can delete a preview from their message.
