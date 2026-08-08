# Chatto Desktop

This directory packages the official Chatto SvelteKit frontend as an
experimental desktop app. It uses
[Deno Desktop](https://docs.deno.com/runtime/desktop/) with the Chromium
Embedded Framework (CEF) backend, so WebRTC and rendering use a bundled Chromium
engine on every supported platform. The toolchain is pinned to Deno 2.9.5, the
current stable release; Deno Desktop remains explicitly experimental in that
release.

Chatto Desktop has its own pre-1.0 version and release-please component. Tags
use `chatto-desktop/vX.Y.Z`; desktop-only releases do not bump the Chatto server
version. Each tagged build still embeds the official frontend revision from the
same commit, whose version continues to govern client-server compatibility.

There is no desktop-specific frontend. The build embeds the unchanged static
artifacts from `apps/frontend/build` and serves them from Deno Desktop's private
loopback origin. Chatto's existing standalone-frontend behavior owns server
registration, authentication, routing, and local browser state.

CEF does not return a browser popup from `window.open()`. The official frontend
therefore detects a small binding installed by this shell and asks Deno Desktop
to create a native `BrowserWindow` for Chatto OAuth. The remote server still
owns the sign-in page, PKCE still protects the authorization code, and the
same-origin callback returns to the main client through `BroadcastChannel`.
Normal web builds continue to use the existing browser-popup path.

## Run it

From the repository root:

```sh
mise desktop-dev
```

The app opens Chatto's normal server registration screen. Servers and delegated
access tokens use the same browser storage as the standalone web frontend.

On macOS, the development task first builds `Chatto Desktop.app` and then makes
an APFS copy-on-write clone for Deno's private HMR host. Deno 2.9.5 otherwise
launches its generic cached `laufey.app`, whose `Info.plist` lacks the privacy
descriptions required by macOS. Requesting a microphone or camera from that
generic host makes macOS terminate the process instead of showing a permission
prompt. The development task verifies Chatto's media descriptions before it
starts HMR; the private clone also keeps Deno's HMR re-signing away from the
normal build output.

## Verify and build

```sh
mise test-desktop
mise desktop-build
```

`mise desktop-build` first runs Chatto's existing `build-frontend` task, then
embeds that output in the host-platform bundle beneath `apps/desktop/dist/`. CEF
alone adds roughly 150 MB. Deno downloads the matching prebuilt backend on the
first build; no C++ or platform CEF build is required.

CI checks and builds the application on macOS, Windows, and Linux. Desktop
release tags publish unsigned archives and SHA-256 checksums while the app is
experimental. These artifacts are suitable for testing, but operating systems
may warn about or block them until platform signing and notarisation are added.

Deno Desktop's automatic SvelteKit detection does not support
`@sveltejs/adapter-static`. `frontend_server.ts` therefore serves the static
artifacts with the existing `200.html` SPA fallback, while `main.ts` owns the
native window bindings. Neither contains application UI.

Deno 2.9.5 writes a configured macOS icon after its internal ad-hoc signing
pass. The build task therefore applies and verifies one final ad-hoc signature
on macOS. Remove that workaround when upgrading to a Deno release that seals the
icon itself, and replace it with the release signing/notarisation workflow
before distribution.

On first launch, the bundled Deno runtime writes its auto-update health marker
next to the runtime library inside the macOS app. That changes a sealed bundle
after signing. The build workaround removes a marker left by an earlier local
launch before re-signing, but it cannot prevent the installed app from creating
it again. Production macOS signing therefore remains blocked on an upstream fix
or a supported external location for Laufey's update state.

Homebrew distribution should use a Cask that installs the macOS application
archive, rather than a Formula. The current ad-hoc signature checks local bundle
integrity but is not a Developer ID signature and cannot satisfy Gatekeeper. An
application also cannot make itself trusted by signing after launch because
Gatekeeper evaluates it before it can run. Publish the Cask as a normal-user
installation path only after the release archive is Developer ID-signed and
notarised.

The current CEF backend also uses CEF's generic browser profile rather than an
app-specific path. On macOS the directory is:

```text
~/Library/Application Support/CEF/User Data
```

It contains Chromium-managed state such as cookies, registered servers, and
delegated access tokens; the shell writes no separate settings file. Deno
Desktop 2.9.5 does not expose a profile-path setting, so a release should wait
for that support or add an upstream-compatible isolation mechanism.

## Prototype boundaries

This scaffold proves the desktop runtime, WebRTC-compatible rendering path, and
native-window Chatto OAuth flow. It does not yet provide Developer ID-signed or
notarised packages, auto-update, OS deep-link handling, desktop-aware
external-link handling, end-to-end desktop tests, or a system-browser OAuth
handoff. Servers configured with an explicit restrictive
`webserver.allowed_origins` list may also reject the desktop app's random
loopback origin. Some identity providers reject authentication inside embedded
user agents, so a system-browser handoff is required before treating this as a
general release.
