# FDR-048: Chatto Mobile

**Status:** Experimental
**Last reviewed:** 2026-09-25

## Overview

Chatto Mobile is an experimental iOS application that bundles the shared Chatto
client. It connects to independent servers without running a local backend.

## Behavior

- The app presents the shared multi-server interface and preferences.
- Native iOS defaults to Flat surface depth. A saved depth choice takes
  precedence. Browser and PWA defaults are unchanged.
- The web page paints the top iOS safe area with the client frame colour and
  the bottom safe area with the inner background colour. Native content
  insets are disabled; the shared layout reserves the top inset and
  the iOS shell adds bottom padding for the home indicator.
- The mobile sidebar columns and backdrop start below the toolbar and top
  safe area, and end above the home-indicator safe area.
- The iOS launch screen shows the muted CHATTO wordmark with system light or
  dark colours. The shell keeps this view visible until the bundled HTML has
  had a rendering opportunity, then fades into the web loading screen without
  a minimum display time. Reduced Motion disables the fade. The saved client
  theme takes effect when the web content loads.
- The iOS shell fixes the page scale at 1 to prevent gesture and input-focus
  zoom from cropping the interface. Page zoom is unavailable in this shell.
- The iOS keyboard hides the form-navigation accessory bar. UIKit's keyboard
  layout guide positions the webview above the keyboard during its movement;
  the plugin's delayed frame resizing is disabled. The shared client adapts to
  the smaller viewport so the composer remains visible.
- Native iOS resizing keeps the app and room headers visible. The native
  window follows the web background colour, including behind the keyboard's
  rounded corners. The Safari keyboard workaround does not run in the iOS app.
- Server sign-in opens the system authentication session. The selected server
  owns sign-in and consent. Cancellation leaves the client in place.
- Server registrations and sessions use persistent application webview storage.
- The shared client keeps no chat data in webview storage. It loads chat data
  from the server after it verifies the session. It does not provide offline
  reading or message sending.
- Sign-in requires HTTPS and a server with the mobile client registration.
- Registered servers recover from failed startup discovery or session loading
  through the shared retry loop. Returning to the foreground triggers an
  immediate retry. Network failure does not start a new sign-in flow.
- This first build has no native push or background-call support. Continuing
  calls with the phone locked remains a requirement for a later stage.
- Distribution is through local Xcode builds and manual TestFlight uploads.
  Public App Store distribution and release automation are deferred.

## Design Decisions

### 1. Share the frontend and add narrow native capabilities

**Decision:** Bundle the existing frontend with Capacitor, starting with iOS.
Use native system authentication as the first host capability.
Keep the iOS Flat surface default separate from native capability detection;
it changes presentation and does not grant access to a native operation.
**Why:** This retains the shared interface and session behaviour while allowing
later native media support. See ADR-072 and ADR-099.
**Tradeoff:** WebKit behaviour still applies to the shared interface. Native
integration requires device verification in addition to browser tests.

### 2. Keep the first implementation small

**Decision:** Retain webview session storage. Defer native push, native call
ownership, a native offline store, and public App Store distribution.
**Why:** First establish a working shell and authentication boundary.
**Tradeoff:** This is a development prototype, not a complete mobile release.

## Related

- **ADRs:** ADR-072, ADR-099, ADR-107
- **FDRs:** FDR-016, FDR-023, FDR-034

## Open Questions

- Native media ownership for locked-phone calls, audio routing, and interruptions.
- Credential storage and recovery after OS process termination.
- Native push delivery for independent servers and its privacy boundary.
- Android packaging and mobile release automation.
