# Chatto Desktop

This directory packages the official Chatto SvelteKit frontend as an
experimental Electron application. Chatto Desktop has its own pre-1.0 version
and release-please component; tags use `chatto-desktop/vX.Y.Z`, independently of
the Chatto server version.

There is no desktop-specific frontend. Electron embeds the unchanged static
artifacts from `apps/frontend/build` and exposes them to its renderer at the
fixed, privileged origin `chatto://desktop`. The stable secure origin and
Electron's app-specific persistent session give local storage, IndexedDB,
service workers, registered servers, and delegated access tokens the same
durable namespace on every launch. Electron does not intercept ordinary HTTP
or HTTPS traffic, so requests to Chatto servers use Chromium's normal network
stack.

The official desktop origin is accepted by Chatto's OAuth redirect policy. The
desktop shell uses Electron's ordinary popup support for the same PKCE and
`BroadcastChannel` flow as a browser. Servers with an explicit restrictive
`webserver.allowed_origins` list must include `chatto://desktop` for API
requests from the desktop app.

## Run it

From the repository root:

```sh
mise desktop-dev
```

The task builds the shared frontend and opens it in Electron. Electron stores
the default session beneath the platform's application-data directory for
`Chatto Desktop`; the shell writes no separate credentials or settings file.

## Verify and build

```sh
mise test-desktop
mise desktop-build
```

The build task first produces the frontend, then packages the host-platform
bundle beneath `apps/desktop/dist/`. CI checks and packages macOS, Windows, and
Linux bundles. The current release archives are unsigned experimental builds,
so operating systems may warn about or block them until production signing and
macOS notarisation are added.

Electron handles camera, microphone, and notification permission requests only
for the fixed app origin. Screen sharing presents a native source picker.
Navigation outside the app is restricted to OAuth popup windows or opened in
the system browser; renderer Node.js integration is disabled and the renderer
is sandboxed.

## Prototype boundaries

This scaffold does not yet provide production signing/notarisation, auto-update,
OS deep links, installers, or end-to-end desktop tests. Some identity providers
reject authentication inside embedded user agents, so a system-browser OAuth
handoff may still be required before treating this as a general release.
