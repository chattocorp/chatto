# ADR-128: Serve Remote Images Through a Signed Server Proxy

**Date:** 2026-11-23
**Status:** Accepted

## Context

Chatto shows images that other hosts supply: the avatar of a sign-in provider
on the account creation page (FDR-023), and the logos and banners of
Neighborhood servers (ADR-106). Markdown images in messages (#3494) will add
more. When a browser loads such an image directly, the remote host sees the
user's IP address.

Before this decision, three features each fetched remote images with their own
code: link previews, Neighborhood discovery, and the provider avatar import.
Each one had its own limits and checks. Neighborhood discovery also kept its
own content-addressed copies in `ASSET_CACHE`, rewrote them to keep them alive,
and served them from `/assets/neighborhood/{sha256}`. The account creation page
used a separate route that the pending flow token authorized.

Chatto already has an image transform pipeline for attachments and server
assets. It resizes an image to a requested size and fit, caches the result in
`ASSET_CACHE`, shares one transform between concurrent requests, keeps
animated GIFs animated, and answers conditional requests with ETags.

## Decision

The server fetches, resizes, caches, and serves remote images for the
browser. The browser never contacts the remote host.

**One fetch.** The `core/remoteimage` package fetches every remote image with
the SSRF-safe client. The client checks each resolved address when it
connects, also after a redirect. The fetch accepts only absolute HTTP(S) URLs
without credentials, a `200 OK` response, a declared type that is missing,
`image/*`, or `application/octet-stream`, and at most 5 MiB by default. Its
errors carry only a category, such as a timeout, a network error, or the reason for a rejection. Transport errors can name the source URL, a redirect location, host names, and IP addresses, so their text never reaches logs. Link previews, the remote image route,
and the provider avatar import use it.

**Presets.** A preset names a fixed size and fit: `avatar` fits 256×256 and
`banner` fits 1200×630, both with `contain`. The presets are a table in
`core/remoteimage`; a new preset is one entry. The set is closed, so the
processing and cache space that one image can cause stay bounded. Arbitrary
sizes are not allowed.

**Signed URLs.** The server serves a remote image at
`/assets/remote/{preset}/{signature}?url={source}`. The signature is an
HMAC-SHA256 of the preset and the source URL with
`core.assets.signing_secret` and a fixed domain prefix. The server signs only
the combinations of preset and source URL that it decided to show, so the
route is not an open proxy, and a client cannot request another preset with a
signed URL. The URL does not depend on the user or the time, so browsers and
shared caches can keep the image, and any replica can serve it. Responses are
public and cacheable for one day, and their ETag names the content. The image
at a remote URL can change, so responses are not immutable, and a browser
that revalidates gets the new image.

**Transform pipeline.** The route is one more source of the image transform
pipeline. The pipeline decodes the fetched bytes with the bounded raster
decoder and re-encodes them, so it never serves the bytes that the remote host
sent, and it rejects SVG and other formats without a registered decoder. It
caches each derivative under
`remote_image.{sha256(source)}-{generation}.{paramsHash}` in `ASSET_CACHE`,
whose seven-day TTL removes unused copies. Remote derivatives include animated
GIFs, which the pipeline does not cache for stored assets.

**Refresh.** A cache generation lasts one day. Each source starts its
generations at an offset derived from its hash, so refreshes spread over the
day. The first request in a new generation fetches the source again, so a
changed image shows within a day. A refresh writes a new object. The
pipeline writes a remote derivative only when its key does not exist yet,
because reading an object store object while it is replaced can stall for
seconds and fail. Two writers that miss the same key at the same moment can
still both write it, so a read of a remote derivative has a two-second
deadline and counts as a miss when it passes. When the fetch fails, the route
serves the copy of the previous generation.

**Failures.** After a fetch fails, or the fetched bytes are not a decodable raster image, the server stores an empty marker under
`remote_image_failure.{hmac(source)}` in `RUNTIME_STATE` with a five-minute
TTL. While the marker exists, every replica rejects the source without a
remote request, so a broken or slow host does not cause one request for each
client request. A failed image, including one that the pipeline cannot
transform, returns 404, which clients can cache for five
minutes. The server logs it only at debug level.

**Consumers.**

- The account creation page receives a signed `avatar` URL for the provider
  avatar. Account creation fetches the avatar again and stores it as a
  managed avatar asset.
- Neighborhood discovery resolves each logo and banner URL against the server
  origin and stores the absolute URL. `ListNeighborhoodServers` returns signed
  `avatar` and `banner` URLs. The route fetches an image when a client first
  shows it. The `/assets/neighborhood` route and the `neighborhood_image.*`
  copies are removed.
- Link previews use the shared fetch, but keep storing their image as a server
  asset of the message. A preview image belongs to the message and must stay
  available as long as the message exists.

## Consequences

- Remote hosts see the server's address, not the user's.
- The signed URL contains the source URL. Anyone who has the signed URL can
  load that image in its preset; the image is public on the remote host
  anyway. A client that needs another size needs a URL that the server signed
  for that preset.
- Each preset of an image fetches the source once per generation. The server
  does not cache source images.
- After a failure is recorded, all replicas skip the source for five minutes.
  Concurrent first requests on several replicas can each fetch before the
  marker exists; the marker bounds repeated failures, not the first one. A
  source whose header decodes but whose image data is corrupt is not marked,
  so it is fetched again on each request. A source that fails for more than one
  generation stops showing its old copy.
- Rate limiting the route is still open. Markdown images (#3494) let members
  choose source URLs, so that work must add it.
- The Neighborhood directory no longer checks images during discovery. A
  broken logo or banner reaches the client as a 404, and the client shows its
  fallback.
- Neighborhood images now follow redirects. The SSRF-safe client checks every
  address that a redirect reaches.
- Rolling upgrades: older replicas still write `object_name` into Neighborhood
  records and return `/assets/neighborhood/` URLs, which newer replicas do not
  serve. Missing images during mixed-version operation are acceptable
  (ADR-106). Newer replicas resolve the source URL of older records, so those
  images work at once. Old `neighborhood_image.*` objects expire with the TTL.
- A new consumer, such as Markdown images, only needs a preset and a call to
  `RemoteImagePath`.
