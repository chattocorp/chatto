# FDR-034: Chatto Desktop

**Status:** Experimental
**Last reviewed:** 2026-10-28

## Overview

Chatto Desktop packages the official multi-server Chatto frontend as an
Electron application. It gives desktop users a consistent bundled Chromium
runtime without creating a second frontend or changing how people register and
use Chatto servers. The application remains experimental while distribution,
system-browser authentication, and clean-machine media behavior are hardened.

## Behavior

- Mouse back and forward buttons navigate the main window's history. On macOS,
  this includes navigation gestures sent by mouse software such as Options+.
  Keyboard navigation uses `Alt+Left` and `Alt+Right` on Windows/Linux, or
  `Cmd+[` and `Cmd+]` on macOS. Navigation stops at either end of the history.
- The application is named **Chatto Desktop** and presents the same interface,
  routes, translations, and client-server compatibility behavior as the
  official standalone frontend.
- People can register and switch between multiple Chatto servers through the
  existing client registry. Registrations, credentials, and browser-managed
  preferences persist across application launches in an app-specific profile.
- On launch, restored authenticated servers reconcile in the background so the
  Server Directory shows saved servers without selecting one. Desktop starts
  on this directory, with direct address lookup available. It does not register
  its application origin as a Chatto server. See FDR-042.
- A signed-out server shows a password form in the main window when the server
  permits password login. Desktop sends the password only to the selected
  server and stores its renewable session. It does not store the password.
  Configured provider buttons appear above the password form. Selecting one
  starts the server's PKCE-protected OAuth flow with that provider selected.
  The provider still owns its login page in a separate window. Servers that
  do not support the provider hint show their normal login page instead.
  **Continue in the browser** opens the existing PKCE-protected OAuth window
  for external providers, account creation, and password recovery. Browser
  deployments continue to use the server-owned OAuth pages for remote login.
- The sign-in screen reuses the web authentication layout and server branding.
  When direct registration is enabled, **Create account** opens the server's
  registration page with a pending PKCE authorization. The person enters the
  emailed verification code and account details in that window. After any
  required consent, the window closes and Desktop opens the new account.
  Servers that do not support the signup hint show their login page instead.
  Switching between registration, login, and SSO keeps the authorization
  continuation. Registration and invitation rules remain server-owned.
  **Remove server** remains in the server context menu, outside the sign-in form.
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
- macOS, Windows, and Linux bundles are built in CI. Release macOS artifacts
  are Developer ID signed and notarised. Release Windows executables and
  libraries are signed and timestamped through Microsoft Artifact Signing
  before CI packages them. The main application must report ChattoCorp's stable
  publisher identity; bundled third-party libraries may retain their original
  valid publisher. Local, pull-request, and Linux artifacts are not trusted
  release builds.
- Chatto Desktop shares the server and frontend version, release commit,
  changelog, and `v{version}` tag. Each release embeds the frontend from that
  commit and displays the shared version. Manual verification builds display
  a development version and abbreviated commit SHA.
- Desktop downloads are attached to the Chatto release. Signing failures can
  delay those downloads, but do not block server publication. Matching desktop
  and server versions are not required; existing compatibility rules apply.
- macOS users can download a signed, notarised disk image and drag the app to
  Applications. The ZIP remains available. To update, users quit the app and
  replace it with the newer version. Both downloads use the same profile;
  replacement preserves registrations, sessions, and preferences. Removing the
  app does not erase the profile or sign out registered servers.
- Windows users can install through a self-contained installer without
  administrator rights. It adds a Start menu shortcut and a Windows Settings
  uninstall entry. Running a newer installer updates the application while
  preserving the existing profile. The installer and uninstaller are signed
  with ChattoCorp's publisher identity. The ZIP remains available.
- The installed Windows application uses the same profile as the ZIP version.
  Uninstall removes application files and shortcuts but preserves registrations,
  credentials, and preferences. Users must close the ZIP application before
  switching to the installed application. Old extracted folders are not removed.
  Updates remain manual.
- The application requires a network connection. It does not provide an
  offline Chatto experience.
- Local macOS development bundles can use the separate **Chatto Desktop Dev**
  identity, helper identity, and app-data directory. This keeps development
  permissions and sessions separate from the official app. Ad-hoc signed
  rebuilds can still require a new permission grant; a stable signing
  certificate preserves the code identity across rebuilds.
- The Diagnostics menu can copy a bounded, in-memory native screen-share
  performance report. It includes codec, encoder, resolution, frame rate,
  bitrate, and network statistics without source names, account identifiers,
  addresses, or credentials. Reports are not sent automatically.
- Development and release builds default to tuned H.264 with one layer and
  resolution priority. Tall and square sources scale to fit the negotiated
  decoder's frame-size limit while retaining the full image. Automatic mode
  uses hardware VBR on macOS 26 or later.
  Older systems, or hardware that rejects VBR setup, use the tuned average-rate
  mode. This preserves the H.264 receiver contract, including tested Firefox
  viewing, without increasing the 8 Mbps video maximum. Diagnostics retain
  all explicit comparisons and a default-mode selection for the next share.
  HEVC remains opt-in. The publisher does not send a parallel backup codec.
  An explicit server rejection of HEVC retries stock H.264. If a viewer cannot
  receive HEVC, the sender must select H.264 and restart.
- The Diagnostics menu also offers a single-layer H.264 comparison at up to
  8 Mbps of video. It removes lower-resolution layers without changing the
  capture settings or frame-rate limit. This mode prioritizes resolution and
  permits frame rate to decrease under pressure. All viewers receive the same
  encoded resolution. Reports identify the selected layer mode and include an outgoing
  bandwidth estimate when available. Samples identify the resolution preference.
  Regular codec options restore simulcast and frame-rate priority.
- A separate **Test Tuned H.264** option uses a custom hardware VideoToolbox
  encoder with High profile, one layer, and resolution priority. Its average
  target is 90% of WebRTC's current video allowance. Its five-second data limit
  follows that allowance, capped at 8 Mbps; zero pauses encoding. Shorter bursts
  can exceed this limit, as with the SDK encoder. Audio and transport overhead
  are additional. This experiment tests the High-profile and rate-control
  headroom gains from the local synthetic benchmark in a live call. It requires
  a compatible High-profile receiver and does not fall back to Baseline or
  software encoding. Reports include applied bitrate,
  hardware status, actual SPS profile, frame counts, and VideoToolbox errors.
- The default VBR mode and **Test Tuned H.264 VBR** option use macOS 26 variable-bitrate
  controls. Its target is the smaller of 6 Mbps or 90% of WebRTC's allowance.
  Its VBV maximum follows that allowance up to 8 Mbps. A 250 ms VBV buffer
  with zero initial delay replaces the average-rate mode's five-second data
  limit. The mode preserves single-layer publication and resolution priority.
  Explicit VBR requires hardware support and does not silently fall back.
  Automatic mode retries a failed VBR setup once with a fresh average-rate
  session. The fallback remains active for that encoder. Local tests found
  better synthetic motion quality with additional encoder delay. Manual
  Firefox and production tests showed smooth playback. These tests do not
  cover all receiver hardware or impaired networks. Diagnostics identify
  requested and actual modes, fallback status, accepted target and maximum,
  buffer duration, and input queue drops.
- The native publisher uses LiveKit Swift 2.18.0. The SDK seeds screen-share
  bandwidth estimation at 90% of the active layers' configured bitrate limits.
  For the single-layer comparison, this is a 7.2 Mbps startup hint, not a
  guaranteed send rate. Congestion control remains active. Reports include the
  SDK version and sample every second for the first 20 seconds, then every
  five seconds. When available, they include capture, encoded, and sent frame
  rates, packet send delay, and the target-byte rate for encoded frames over
  the same interval as the actual bitrate. This target-byte rate is not a
  network bandwidth estimate. Missing counters remain absent.
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

YouTube player frames loaded directly by the local frontend receive the fixed
app identity `https://run.chatto.desktop/` as their HTTP referrer. YouTube
requires this identity because the custom origin does not supply an HTTP
referrer. The value contains no server address, room path, or user identifier.
The player still connects directly to YouTube, which receives the user's IP
address and requested video ID. This exception applies only to the existing
`youtube-nocookie.com` embed endpoint; other requests keep normal browser
referrer handling. See [YouTube's client identity requirements](https://developers.google.com/youtube/terms/required-minimum-functionality#api-client-identity-and-credentials).

**Why:** A stable secure origin keeps local storage, IndexedDB, service workers,
OAuth callbacks, and registered servers reachable on every launch. The
dedicated scheme cannot collide with a local service and avoids intercepting
remote server navigation. Chatto servers trust only its exact callback path.
**Tradeoff:** The exact origin becomes a compatibility boundary. Desktop
clients using this identity require the corresponding Chatto 0.5 server
behavior.

### 4. Inline password login with server-owned OAuth as an alternative

**Decision:** Desktop offers password login inside the selected server's view
through a narrow host capability (ADR-072). It uses the server's existing
programmatic password sessions, including renewal and revocation. These
sessions have no OAuth client grant or client-consent step. Server password
policy still applies. The server cannot identify an official Desktop binary
from this request. The web frontend does not offer direct remote login.
**Why:** Users can sign in without leaving the main app, while each server
keeps an independent session. A late response cannot replace a session after
the user leaves the form, removes the server, or replaces its session.
**Tradeoff:** Users trust the installed app with their server password.
External provider buttons send only the configured provider ID to the Chatto
server, which owns provider selection and account mapping. Registration and
recovery also use the server-owned OAuth window and its existing PKCE and
callback handling. Selecting a provider contacts that provider and reveals
the user's IP address to it. No provider connection starts from displaying
the buttons alone. Some providers
require a future system-browser flow, and the host
must tightly constrain popup and navigation behavior. The host opens a new
window only on the frontend's `/servers/authorize` launch page and sends all
other window requests to the system browser.

### 5. Synchronize Desktop and Chatto releases

**Decision:** Desktop and Chatto share one version, release commit, changelog,
and GitHub release. Every release builds Desktop from the same source and
uses the shared version for its frontend. See ADR-118.
**Why:** Users can compare desktop and web frontend versions directly, and
maintainers prepare one release instead of two.
**Tradeoff:** Each release requires desktop builds and signing approvals, and
desktop-only fixes also advance the Chatto version. Desktop uploads can arrive
after server publication so signing failures do not block security fixes.

### 6. Build all supported host bundles and sign trusted releases

**Decision:** CI checks and builds macOS, Windows, and Linux bundles. The
protected release workflow Developer ID signs and notarises macOS, and uses
Microsoft Artifact Signing with GitHub OpenID Connect to sign Windows PE files.
It validates macOS acceptance, every Windows signature and timestamp, and the
main application's publisher subject before packaging. Bundled third-party
libraries may retain their original valid publisher. Linux remains unsigned
until its update-capable package and trust model are selected.
**Why:** Cross-platform builds catch packaging drift and let contributors test
the application before release credentials are available. Managed Windows
signing keeps the private key out of GitHub while a protected environment and
least-privilege Azure role restrict which revisions can request signatures.
Windows and macOS use separate protected environments so neither platform's
runner can access the other platform's signing credentials.
**Tradeoff:** Ordinary CI artifacts remain unsigned, Windows signing depends on
Azure availability and organization validation, and CI signature checks do not
replace clean-machine installation or WebRTC verification.

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
and publication policy. Its default H.264 publication preserves aspect ratio
within a 1920-pixel maximum edge at up to 60 fps. One layer reduces encoding
and upload cost but gives all viewers the same encoded resolution. The
Diagnostics menu retains stock H.264 and HEVC simulcast comparisons with
1920-, 1280-, and 640-pixel quality classes at 60, 60, and 30 fps. Simultaneously
active layers increase encoding and upload cost. The native media path avoids
the prototype's extra decode and re-encode.

### 9. Keep background notification transport host-owned

**Decision:** Browser Web Push subscriptions remain a browser/PWA transport and
are not generalized into a cross-platform notification endpoint. A future
Desktop background-notification implementation will use a narrow host
capability and a Desktop-owned device registration or background connection,
while reusing Chatto's persistent notification facts, per-server routing, and
click targets above that transport boundary.
**Why:** Electron does not provide a portable Web Push path that can wake an
application after its process has exited. Keeping the current API honest about
browser subscriptions avoids persisting speculative Desktop fields or making
Desktop depend on service-worker scope and VAPID semantics. It also leaves each
platform free to choose a reliable native wake mechanism without changing the
shared notification behavior.
**Tradeoff:** PWA and Desktop delivery registrations will be separate and may
need a small server-side fan-out abstraction when Desktop background delivery
is implemented. That work is deferred; current Desktop notifications still
depend on the renderer being alive.

### 10. Use a drag-to-Applications disk image on macOS

**Decision:** Provide a signed and notarised DMG alongside the ZIP download.
**Why:** The app is self-contained. A disk image gives users a familiar way to
install it without installation scripts or additional system services.
**Tradeoff:** Updates require users to quit and replace the app manually.

## Related

The iOS shell shares the frontend but uses native system authentication. See
[FDR-048](FDR-048-chatto-mobile.md) and
[ADR-099](../adr/ADR-099-capacitor-mobile-client.md).

- **ADRs:** ADR-024 (opaque bearer tokens for cross-origin auth), ADR-025 (multi-server client architecture), ADR-064 (separate frontend server catalogue and sessions), ADR-065 (runtime JSON client internationalization), ADR-067 (Electron desktop packaging), ADR-072 (optional host capabilities), ADR-074 (device-local server catalogue), ADR-118 (synchronized releases)
- **FDRs:** FDR-008 (File Attachments & Video Processing), FDR-013 (Web Push Notifications), FDR-016 (Voice Calls), FDR-023 (Authentication & Sessions), FDR-027 (PWA & Service Worker), FDR-031 (Client–Server Compatibility Discovery)

## Open Questions

- How system-browser OAuth callbacks and normal external links should return to
  or focus the application on every platform.
- Which automatic-update strategy and Linux installers should follow the
  Windows installer, macOS disk image, and downloadable archives.
- Whether Electron's shipped codec set covers every media artifact Chatto
  currently generates on every supported platform.
- Which higher native screen-share quality profiles should be offered once representative
  game footage and supported Mac hardware have been benchmarked.
