# FDR-034: Chatto Desktop

**Status:** Experimental
**Last reviewed:** 2026-08-13

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
- On launch, restored authenticated servers reconcile in the background so the
  welcome screen can show their availability without selecting one. Desktop
  remains on the welcome screen until the person chooses a server.
- Connecting a server starts Chatto's PKCE-protected OAuth flow in a separate
  Electron window. The remote server continues to own the visible sign-in and
  consent pages.
- Voice calls, camera video, screen sharing, and media-device selection use
  Electron's bundled Chromium WebRTC implementation. Camera and microphone
  requests use operating-system permission prompts; screen sharing requires an
  explicit native source choice.
- Every macOS build embeds a native screen-capture provider. The shared
  screen-share control opens a Chatto-owned picker of ordinary visible windows
  and complete displays, with static in-memory previews. Window capture
  publishes video and owning-application audio to the existing LiveKit call;
  display capture is video-only to prevent remote call playback from being
  captured and echoed. The native publisher supplies several video qualities
  and stops encoding qualities that no receiver consumes. Native and browser
  sharing replace one another, while camera and microphone can remain enabled.
  Acknowledged lifecycle control keeps the UI in a stopping state until the
  helper disconnects its LiveKit companion and exits. Hosts without this
  capability keep the browser's ordinary source chooser.
- macOS, Windows, and Linux bundles are built in CI. Experimental macOS
  artifacts are ad-hoc signed, while the other platform artifacts remain
  unsigned until trusted platform signing and macOS notarisation are added.
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
network behavior. The custom shell origin is frontend-only and is never probed
as a Chatto HTTP backend; discovery, viewer, and realtime requests target only
registered HTTP or HTTPS server origins.
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

### 7. Expose desktop-only capabilities through narrow optional bridges

**Decision:** The desktop host exposes a `screenShare` capability only when a
native provider is present, and gives the shared frontend temporary opaque
window/display sources, static preview bytes, and metadata needed for explicit
user choice. A focused frontend adapter feature-detects and validates this
capability; the same control uses the browser implementation when it is absent.
Source enumeration requires user activation, is serialized and cancellable,
and produces short-lived, single-use random offers rather than native source
coordinates. Window offers are bound to the enumerated application, and the
host excludes its own windows so remote call playback cannot be republished as
application audio.
The host does not expose general process, filesystem, or operating-system
access to the renderer. This follows ADR-072.
**Why:** The shared frontend needs to own the visible call interaction without
weakening Electron's sandbox or taking a dependency on macOS-specific source
identifiers. The same product-level boundary can be implemented independently
by future Windows and feasible Linux providers.
**Tradeoff:** Each desktop-only capability needs a deliberately designed bridge,
validation on both sides, bounded data transfer, and platform-specific
availability handling. Preview images are point-in-time hints rather than live
streams, and may be stale when selected.

### 8. Publish native screen-share media through a companion LiveKit connection

**Decision:** Platform helpers own capture, WebRTC encoding, E2EE, and LiveKit
publication for native screen-share media. The desktop bridge carries only
temporary source descriptions and previews, a fresh URL/token/key credential,
stop commands, and lifecycle status; captured video and audio do not pass
through Chromium or Electron IPC.
The helper uses a separate opaque LiveKit identity whose metadata names the
owning user. The shared frontend merges its tracks into that user's logical
participant and suppresses all local companion audio playback to avoid
feedback. Window capture can publish isolated owning-application audio. Display
capture remains video-only because its system audio would include Chatto's own
remote call playback before the frontend could suppress it.
**Why:** A second native LiveKit connection cannot reuse the member's identity
without disconnecting the existing voice client. A separately authorised
companion connection avoids the prototype's native encode, browser decode,
canvas capture, and WebRTC re-encode while keeping the product-level source and
participant model shared across platform providers.
**Tradeoff:** Chatto needs an auxiliary publisher-token RPC, companion-aware
webhook and reconciliation filtering, and frontend participant merging. The
native helper also becomes responsible for matching the primary call's E2EE
and publication policy. Its aspect-ratio-preserving H.264 publication includes
1920-, 1280-, and 640-pixel maximum-edge quality classes at 60, 60, and 30 fps,
and dynacast pauses layers that no subscriber requests. Simultaneously active
qualities increase native encoding and upload cost. These are explicit
control-plane costs in exchange for a materially shorter and more efficient
media path that can adapt to each receiver.

## Related

- **ADRs:** ADR-024 (opaque bearer tokens for cross-origin auth), ADR-025 (multi-server client architecture), ADR-064 (separate frontend server catalogue and sessions), ADR-065 (runtime JSON client internationalization), ADR-067 (Electron desktop packaging), ADR-072 (optional host capabilities)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-016 (Voice Calls), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-031 (Client–Server Compatibility Discovery)

## Open Questions

- How system-browser OAuth callbacks and normal external links should return to
  or focus the application on every platform.
- Which platform and architecture becomes the first signed release target.
- Which installer and automatic-update strategy should follow the first
  downloadable archives.
- Whether Electron's shipped codec set covers every media artifact Chatto
  currently generates on every supported platform.
- Which higher native screen-share quality profiles should be offered once representative
  game footage and supported Mac hardware have been benchmarked.
