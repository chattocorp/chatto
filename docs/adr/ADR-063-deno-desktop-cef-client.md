# ADR-063: Package Chatto Desktop with Deno Desktop and CEF

**Date:** 2026-08-02

## Context

Chatto's official frontend already operates as a static multi-server client,
but browser installation does not provide a consistent desktop WebRTC runtime.
System webviews differ by platform and have previously lacked or varied in
camera, microphone, screen-sharing, and media behavior. Chatto needs a desktop
distribution without creating a second frontend that would drift from the
official client.

Electron provides a mature Chromium application platform but adds a Node.js and
Electron-specific application boundary. Native-webview shells are smaller but
retain the WebRTC differences that motivate this work. Building and maintaining
a custom CEF host would provide control at the cost of owning a substantial
native integration. Deno Desktop provides a TypeScript host API, prebuilt CEF
backends, cross-platform packaging, and a small binding surface, but it is new
and still has important upstream gaps.

The desktop shell also has release concerns that do not align with the Chatto
server. CEF and Deno upgrades, platform packaging, signing, and desktop-only
fixes may need releases even when the public Chatto API and server are
unchanged.

## Decision

Chatto will incubate an experimental desktop application under `apps/desktop`
using a pinned stable Deno version and Deno Desktop's CEF backend.

The application embeds the unmodified static artifacts from the official
SvelteKit frontend described by ADR-018. A small Deno entrypoint serves those
artifacts from the private desktop HTTP listener and adopts the startup
`BrowserWindow`. It does not implement application UI, Chatto domain behavior,
or a backend.

Desktop-specific frontend integration must remain optional and narrow. The
initial integration abstracts the authorization-window lifecycle: ordinary web
deployments use `window.open()`, while the Deno host exposes methods to create,
navigate, inspect, and close a native CEF window. The host accepts only
credential-free HTTP or HTTPS navigation. Chatto's existing authorization-code
flow, PKCE verifier, consent, callback validation, token exchange, and bearer
storage remain authoritative under ADR-024 and ADR-025.

The compiled host receives only the runtime permissions required to listen on
loopback and read the embedded frontend build. Operating-system media access is
declared through platform package metadata. Development tooling may use broader
permissions only for building and preparing a private HMR host.

Chatto Desktop is an independently versioned Chatto product artifact. Release
Please owns `apps/desktop/CHANGELOG.md` and `apps/desktop/deno.json`; release
tags use `chatto-desktop/v{version}`. Desktop paths are excluded from the root
Chatto server release component. A desktop release records and bundles the
official frontend revision at its tagged commit, but its version is not used as
the server/client protocol compatibility version.

CI runs the desktop tests and builds unsigned host bundles on macOS, Windows,
and Linux. Signing, notarisation, trusted installer formats, and Homebrew Cask
publication may be added without changing the runtime choice. An ad-hoc or
self-signature is useful for local bundle integrity but is not treated as a
trusted distribution signature.

The application remains experimental until at least these boundaries have
supported solutions:

- a stable renderer origin and app-specific CEF profile for durable,
  non-colliding browser storage;
- a desktop-origin/CORS contract for restrictively configured Chatto servers;
- a system-browser authorization option for providers that reject embedded
  user agents;
- a media strategy for Chatto's H.264/AAC video output with stock CEF;
- package version metadata, signed/notarised distribution, legal notices, and
  clean-machine WebRTC verification; and
- Laufey runtime update state that does not mutate a sealed application bundle.

## Consequences

Chatto gains one official frontend across server-hosted, standalone, PWA, and
desktop distributions. Frontend fixes and translations automatically reach the
desktop build, and CEF supplies a predictable WebRTC implementation.

The shell stays small and auditable because it owns packaging, local static
serving, and native integration only. It does not become another place to
implement Chatto product behavior.

CEF makes bundles significantly larger and adds an embedded-browser security
update obligation. A current Deno release does not by itself prove that the
bundled CEF revision satisfies Chatto's security or codec needs.

Independent versioning prevents desktop packaging fixes from bumping the
server, but release notes and diagnostics must show both the desktop shell
version and the bundled Chatto frontend/server compatibility version where
users need to distinguish them.

Cross-platform CI can prove that unsigned bundles assemble. It cannot prove
Gatekeeper, SmartScreen, Linux desktop integration, media permissions, or real
WebRTC behavior on clean user machines. Those remain explicit release gates.
