# ADR-067: Package Chatto Desktop with Electron

**Date:** 2026-08-08

**Status:** Partially superseded

**Partially superseded by:** [ADR-072](ADR-072-optional-host-capabilities-in-the-shared-frontend.md),
which permits narrow optional preload capabilities while retaining the renderer
sandbox and context isolation.

## Context

ADR-063 selected Deno Desktop and CEF for Chatto's experimental desktop client.
That prototype validated reuse of the official frontend and a bundled Chromium
runtime, but Deno Desktop 2.9.5 does not expose two controls needed for a usable
application: a stable renderer origin and an app-specific browser profile.
Random loopback ports give browser-managed state a different origin on each
run, while the generic CEF profile can collide with unrelated applications.
The runtime also writes update state inside the macOS application bundle after
signing. These are release blockers rather than polish items.

Electron is larger and gives Chatto an Electron-specific host to maintain, but
it provides mature application sessions, custom protocol handling, window
controls, permission handlers, and cross-platform packaging. Its normal
browser popup behavior also removes the CEF-only OAuth bridge from the shared
frontend.

## Decision

Chatto will package the experimental desktop client with a pinned stable
Electron release. This decision supersedes ADR-063.

The application continues to embed the unmodified static artifacts produced by
the official SvelteKit frontend. Electron's default persistent session owns the
renderer's browser storage in the operating system's app-specific user-data
directory. Renderer Node.js integration is disabled, context isolation is
enabled, and the renderer is sandboxed.

The Electron host registers the standard, secure custom origin
`chatto://desktop` and serves embedded frontend resources without opening a TCP
listener. HTTP and HTTPS are not intercepted and use Chromium networking
normally. The fixed origin provides a stable secure-context namespace for
local storage, IndexedDB, service workers, OAuth callbacks, and related browser
state. Chatto servers trust the exact official callback
`chatto://desktop/servers/callback`; no other path or custom-scheme host gains
OAuth redirect trust. The fixed `chatto://desktop` client identity and callback
need no server configuration; API and realtime access use bearer credentials.

OAuth keeps the frontend's ordinary `window.open()` flow, PKCE, consent,
callback validation, `BroadcastChannel` response, token exchange, and bearer
storage. The host allows the blank popup created synchronously by the frontend,
then permits HTTP or HTTPS navigation inside that child while denying nested
windows. Main-window navigation away from the app origin is denied and normal
web links open in the system browser.

The host grants camera, microphone, and notification permission only to the
fixed app origin. Screen capture requires a user gesture and an explicit native
source choice. No preload bridge or Chatto domain API is exposed to renderer
code.

Chatto Desktop remains an independently versioned product artifact. Release
Please owns `apps/desktop/CHANGELOG.md` and `apps/desktop/package.json`; tags use
`chatto-desktop/v{version}`. CI checks and packages host-platform bundles on
macOS, Windows, and Linux. Trusted code signing, notarisation, installers,
auto-update, and clean-machine WebRTC verification remain release-hardening
work.

## Consequences

Registrations, delegated credentials, and other browser state now have a stable
origin and an application-owned profile across launches. The desktop host is
smaller conceptually because it no longer owns a private HTTP server or OAuth
binding, while the frontend remains shared with browser and PWA deployments.

Electron significantly increases download size and adds an urgent browser
security-update cadence. Chatto must update Electron promptly, audit the main
process as a privileged boundary, and verify media behavior on each supported
platform.

The fixed origin becomes a durable client contract. Changing it would strand
browser-managed state and require a migration. It has no TCP port and cannot
conflict with another local service. Because ordinary HTTPS is not intercepted,
remote redirects and browser networking retain their native semantics.

Desktop clients using the custom callback require Chatto 0.5 or newer;
older servers reject its non-HTTPS redirect URI. Identity providers that reject
embedded user agents still require a future system-browser OAuth handoff.
