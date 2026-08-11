# FDR-034: Chatto Desktop

**Status:** Experimental
**Last reviewed:** 2026-08-08

## Overview

Chatto Desktop packages the official multi-server Chatto frontend as an
Electron application. It gives desktop users a consistent bundled Chromium
runtime without creating a second frontend or changing how people register and
use Chatto servers. The application remains experimental while distribution,
system-browser authentication, and clean-machine media behavior are hardened.

## Behavior

- The application is named **Chatto Desktop** and presents the same interface,
  routes, translations, and client-server compatibility behavior as the
  official standalone frontend.
- People can register and switch between multiple Chatto servers through the
  existing client registry. Registrations, credentials, and browser-managed
  preferences persist across application launches in an app-specific profile.
- Connecting a server starts Chatto's PKCE-protected OAuth flow in a separate
  Electron window. The remote server continues to own the visible sign-in and
  consent pages.
- Voice calls, camera video, screen sharing, and media-device selection use
  Electron's bundled Chromium WebRTC implementation. Camera and microphone
  requests use operating-system permission prompts; screen sharing requires an
  explicit native source choice.
- macOS, Windows, and Linux bundles are built in CI. Experimental release
  artifacts are unsigned until platform signing and notarisation are added.
- Chatto Desktop has an independent version and changelog. Its release tags use
  `chatto-desktop/v{version}` and do not change the Chatto server version.
- The application requires a network connection. It does not provide an
  offline Chatto experience.
- Servers recognize the built-in `chatto://desktop` OAuth client and its exact
  callback. Desktop API and realtime access use bearer credentials and require
  no origin configuration.
- Identity providers that reject embedded user agents are not supported until
  Chatto Desktop gains a system-browser authorization handoff.

## Design Decisions

### 1. Reuse the official frontend build

**Decision:** Chatto Desktop embeds the static artifacts produced by the
official frontend build and does not maintain desktop-specific application UI.
**Why:** One frontend keeps behavior, accessibility, translations, protocol
support, and security fixes aligned across browser and desktop deployments.
This follows ADR-067.
**Tradeoff:** Desktop-only capabilities need narrow host integration instead of
a separately optimized desktop interface.

### 2. Use Electron's bundled Chromium renderer

**Decision:** The desktop shell uses a pinned stable Electron release rather
than Deno Desktop, a system webview, or a custom CEF host.
**Why:** Electron supplies a consistent WebRTC-capable renderer together with
the persistent-session, protocol, permission, and packaging controls missing
from the Deno prototype. See ADR-067.
**Tradeoff:** Electron substantially increases artifact size and adds an
embedded-browser security update obligation.

### 3. Give the renderer a stable local origin

**Decision:** Electron registers the standard, secure custom origin
`chatto://desktop` and serves bundled frontend files there without binding a
local TCP port. The default persistent session stores Chromium state in the
application's user-data directory. HTTP and HTTPS retain Chromium's normal
network behavior.
**Why:** A stable secure origin keeps local storage, IndexedDB, service workers,
OAuth callbacks, and registered servers reachable on every launch. The
dedicated scheme cannot collide with a local service and avoids intercepting
remote server navigation. Chatto servers trust only its exact callback path.
**Tradeoff:** The exact origin becomes a compatibility boundary. Desktop
clients using this identity require the corresponding Chatto 0.5 server
behavior.

### 4. Keep the browser OAuth-window flow

**Decision:** Both browser and Electron deployments use the frontend's ordinary
`window.open()` authorization flow and the same PKCE and callback handling.
**Why:** Electron implements browser popup windows, so the desktop host no
longer needs the privileged CEF bridge introduced by the Deno prototype.
**Tradeoff:** Some providers still require a system-browser flow, and the host
must tightly constrain popup and navigation behavior.

### 5. Release the desktop shell independently

**Decision:** Chatto Desktop uses an independent pre-1.0 release stream,
changelog, tag namespace, and artifacts while continuing to bundle a specific
official frontend revision.
**Why:** Desktop packaging, platform fixes, and runtime upgrades have a cadence
different from Chatto server releases.
**Tradeoff:** Compatibility diagnostics must distinguish the desktop shell
version from the bundled Chatto client version.

### 6. Build all supported host bundles before signing them

**Decision:** CI checks and builds macOS, Windows, and Linux bundles. Early
artifacts may be published unsigned, but trusted signing and notarisation remain
a separate release-hardening milestone.
**Why:** Cross-platform builds catch packaging drift and let contributors test
the application before release credentials are available.
**Tradeoff:** Operating systems may warn about or block unsigned artifacts, and
CI assembly alone does not prove clean-machine WebRTC behavior.

## Related

- **ADRs:** ADR-024 (opaque bearer tokens for cross-origin auth), ADR-025 (multi-server client architecture), ADR-064 (separate frontend server catalogue and sessions), ADR-065 (runtime JSON client internationalization), ADR-067 (Electron desktop packaging)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-016 (Voice Calls), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-031 (Client–Server Compatibility Discovery)

## Open Questions

- How system-browser OAuth callbacks and normal external links should return to
  or focus the application on every platform.
- Which platform and architecture becomes the first signed release target.
- Which installer and automatic-update strategy should follow the first
  downloadable archives.
- Whether Electron's shipped codec set covers every media artifact Chatto
  currently generates on every supported platform.
