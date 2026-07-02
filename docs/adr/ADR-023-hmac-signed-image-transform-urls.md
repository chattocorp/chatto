# ADR-023: HMAC-Signed Image Transform URLs

**Date:** 2026-03-01

**Update:** This ADR now describes the low-level HMAC transform-signing
primitive, not the whole current browser-facing asset URL model. Attachment
URLs later moved to stable `/assets/files/{assetId}` paths with signed per-user
access tickets (ADR-032, ADR-039, FDR-008).

## Context

Chatto serves user-uploaded images (avatars, attachments) and supports on-demand resizing and cropping. The transform parameters (width, height, crop mode) are specified in the URL. Without protection, any client could request arbitrarily large or unusual transformations, consuming server CPU and memory.

The options are:

- **Open transform parameters**: Simple but vulnerable to abuse. A malicious client could request thousands of unique sizes to exhaust resources.
- **Pre-generated thumbnails**: Generate a fixed set of sizes at upload time. Safe but inflexible — adding a new size requires batch-processing all existing assets.
- **Signed transform URLs**: The server generates URLs with HMAC signatures. Only URLs signed by the server are accepted, preventing parameter tampering.

## Decision

Use HMAC-SHA256-signed URL path components for image transform requests. The signed path encodes:

1. The resource identifiers that bind the signature to the transformed object
   (for example, a stable asset ID or server-asset resource namespace)
2. The transform parameters as base64-encoded JSON
3. An HMAC-SHA256 signature (truncated to 32 hex characters) computed over the combined path

The signature is appended after a dot separator in the URL path. The server verifies the signature before performing any transform work. Transform results can optionally be cached in a NATS `ASSET_CACHE` object store.

## Consequences

- **Abuse prevention**: Clients cannot craft arbitrary transform requests. Only server-generated URLs are accepted. The parameter space is controlled by the application, not the client.
- **Resource-specific signatures**: The HMAC covers the resource identity, so a signature for asset A cannot be reused for asset B. Changing any parameter invalidates the signature.
- **No token storage**: Unlike API keys or session-based authorization, HMAC signatures are stateless. The server only needs the signing secret to verify any URL.
- **Separate signing secret**: The image transform signing key is independent of the session cookie signing key. Compromising one doesn't compromise the other.
- **Cache-friendly**: Signed transform components are deterministic - the same resource binding and parameters produce the same signature. Browser-facing attachment URLs may still carry expiring access tickets, so those full URLs are not stable CDN keys.
- **URL opacity**: The base64-encoded parameters make URLs opaque to humans. Debugging requires decoding the parameter blob, but this is a minor inconvenience.
