# FDR-034: Chatto Desktop

**Status:** Experimental
**Last reviewed:** 2026-08-02

## Overview

Chatto Desktop packages the official multi-server Chatto frontend as a native
desktop application. It gives desktop users a bundled Chromium runtime for
consistent WebRTC behavior without creating a second frontend or changing how
people register and use Chatto servers. The application is experimental while
its embedded-browser storage, authentication, media, and distribution
boundaries are being hardened.

## Behavior

- The application is named **Chatto Desktop** and presents the same interface,
  routes, translations, and client-server compatibility behavior as the
  official standalone frontend.
- People can register and switch between multiple Chatto servers through the
  existing client registry.
- Connecting a server starts Chatto's PKCE-protected OAuth flow in a separate
  desktop window. The remote server continues to own the visible sign-in and
  consent pages.
- Voice calls, camera video, screen sharing, and media-device selection use the
  embedded Chromium WebRTC implementation and the operating system's media
  permission prompts.
- macOS, Windows, and Linux bundles are built in CI. Experimental release
  artifacts are unsigned until platform signing and notarisation are added.
- Chatto Desktop has an independent version and changelog. Its release tags use
  `chatto-desktop/v{version}` and do not change the Chatto server version.
- The application requires a network connection. It does not provide an
  offline Chatto experience.
- Until Deno Desktop provides a stable renderer origin and isolated browser
  profile, browser-managed registrations and credentials are not guaranteed to
  remain reachable across application launches. This prevents treating the
  current build as a supported general release.
- Identity providers that reject embedded user agents are not supported until
  Chatto Desktop gains a system-browser authorization handoff.
- Stock CEF builds do not include every patented media codec. Chatto's H.264/AAC
  video renditions therefore require a codec strategy before desktop video
  playback can be considered complete.

## Design Decisions

### 1. Reuse the official frontend build

**Decision:** Chatto Desktop embeds the static artifacts produced by the
official frontend build and does not maintain desktop-specific application UI.
**Why:** One frontend keeps behavior, accessibility, translations, protocol
support, and security fixes aligned across browser and desktop deployments.
This follows ADR-063.
**Tradeoff:** Desktop-only capabilities need narrow host bindings or shared
frontend abstractions instead of a separately optimized desktop interface.

### 2. Use CEF for the renderer

**Decision:** The desktop shell uses Deno Desktop's Chromium Embedded Framework
backend rather than the operating system webview.
**Why:** Chatto's voice and video calls depend on WebRTC APIs whose availability
and behavior vary in system webviews. A bundled Chromium engine gives the
application a known rendering and WebRTC baseline. See ADR-063.
**Tradeoff:** CEF substantially increases artifact size, has its own security
update cadence, and omits proprietary codecs from stock builds.

### 3. Preserve the multi-server client model

**Decision:** The desktop application runs the existing standalone frontend
against a private local origin; server discovery, registration, bearer tokens,
and compatibility policy remain owned by the multi-server client.
**Why:** Desktop is another distribution of the same Chatto client, not a new
server relationship or API tier. Reusing ADR-025 keeps remote-server behavior
consistent.
**Tradeoff:** Browser origin and profile behavior become part of the desktop
security and durability boundary. Deno Desktop's current random loopback origin
is not sufficient for a supported release.

### 4. Use a narrow native OAuth-window bridge

**Decision:** The shared frontend can ask the desktop host to create, navigate,
inspect, and close a native authorization window. Browser deployments keep the
normal `window.open()` path, and both paths use the same PKCE and callback
handling.
**Why:** CEF does not return a usable popup window for the existing browser
flow. A small lifecycle binding restores the expected separate-window behavior
without moving credentials or authorization decisions into the native shell.
**Tradeoff:** The desktop host must validate renderer-provided navigation and
secure the binding as carefully as any privileged browser integration. Some
external providers still require a system-browser flow.

### 5. Release the desktop shell independently

**Decision:** Chatto Desktop uses an independent pre-1.0 release stream,
changelog, tag namespace, and artifacts while continuing to bundle a specific
official frontend revision.
**Why:** Desktop packaging, platform fixes, and runtime upgrades have a cadence
different from Chatto server releases. Independent versions make that cadence
visible without falsely implying a server protocol change.
**Tradeoff:** Every desktop release must record which frontend revision it
contains, and compatibility decisions must distinguish the desktop shell
version from the bundled Chatto client version.

### 6. Build all supported host bundles before signing them

**Decision:** CI checks and builds macOS, Windows, and Linux bundles. Early
artifacts may be published unsigned, but signed/notarised distribution remains
a separate release-hardening milestone.
**Why:** Cross-platform builds catch packaging drift early and let contributors
exercise the application before signing credentials and platform release
infrastructure are available.
**Tradeoff:** Operating systems may warn about or block unsigned artifacts.
Unsigned CI success is not evidence that a package is ready for normal users.

## Related

- **ADRs:** ADR-024 (opaque bearer tokens for cross-origin auth), ADR-025 (multi-server client architecture), ADR-043 (client-shell internationalization), ADR-063 (Deno Desktop and CEF packaging)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-016 (Voice Calls), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-031 (Client–Server Compatibility Discovery)

## Open Questions

- Whether to wait for upstream stable-origin and profile support or maintain a
  small Deno/Laufey fork.
- Whether desktop video should use a codec-enabled CEF build or an additional
  open-codec Chatto rendition.
- How system-browser OAuth callbacks and normal external links should return to
  or focus the application on every platform.
- Which platform and architecture becomes the first signed release target.
