# Chatto GPUix

A native Chatto client built with
[gpuix-svelte](https://github.com/khromov/gpuix-svelte) and the workspace
`@chatto/client`. Svelte components render through GPUI, without a browser.

The SDK owns authentication renewal, requests, room stores, paging, and realtime
WebSocket delivery. The app adds a native sign-in flow, a grouped sidebar with
channels and direct messages, Markdown, image attachments, link and social-post
preview cards, a virtual timeline, older-page loading
near the top, and a message composer. It also includes an offline demo.

## Run

Use a Mac with Apple Silicon and GPU support. This is the only platform checked
here. From this directory:

```sh
mise setup
mise dev
```

`mise` provides Node, pnpm, and Bun. The GPUI module comes prebuilt from npm, so
this app needs no local Rust or Metal shader build. `mise dev` opens a native
window and reloads changed Svelte components. Close the window or press
Control-C in the terminal to stop it. Hot reload closes the old SDK client.

Enter the server origin and username, then select **Sign in**. macOS asks for
the password in a system dialog with a hidden input. GPUix 0.4.1 does not expose
a masked input field. The server must allow direct password login. HTTPS is
required except for loopback development servers. Browser and provider login
are not implemented.

The server origin, username, and selected room are saved in
`~/Library/Application Support/Chatto GPUix/settings.json`, with mode `0600`
inside a `0700` directory. Set `CHATTO_GPUIX_CONFIG_DIR` to use another directory.
Passwords are discarded after sign-in. **Sessions stay in memory in this
version**, so restarting the app requires sign-in. The SDK renews access tokens
while the app is open. **Sign out** clears the local connection first, then
asks the server to revoke the session.

Image attachments and video posters load near the visible part of the timeline.
The app uses the SDK to refresh expired attachment URLs. Image frames keep a
fixed height while bytes load. PNG, JPEG, GIF, and WebP previews have a 12 MB
limit. Failed reads show a retry button. Other files have an **Open attachment**
action, which opens the signed file URL with the macOS default application.

Link and social-post cards use the server's preview metadata. Social content
warnings conceal text and images until you select **Show content**. YouTube and
Vimeo links open in the browser; embedded video playback is not implemented.
Images written directly in message Markdown remain text labels; HTML remains
code. Avatars on the connected server load near the viewport. Avatars on other
origins keep their initials, without a request.

The configured server receives requests, credentials, and the connection's IP
address. Images on that origin load automatically. Images on other origins,
including external asset storage, require **Load image** first. The button names
the host that receives your IP address. Image reads send no cookies, bearer
tokens, or referrer and do not follow redirects. Opening a link or attachment
uses your browser or default application and its own network settings.
Installing dependencies contacts package registries and tool download hosts.

## Native design components

The native app follows the web client's dark Chatto palette and compact layout.
`src/ui/theme.ts` defines semantic colors, fonts, radii, and themes for native
inputs and Markdown. These are local tokens with the web theme's names; the app
does not import browser CSS or web components.

`src/ui/` contains the native theme boundary, line icons, avatars, channel rows,
collapsible sidebar sections, message frames, preview surfaces, and composer.
`ActionButton.svelte` supplies default, primary, quiet, disabled, and icon-only
controls. The connected client, offline demo, and gallery reuse these parts.
Conversation search filters the loaded room directory; it does not search
message history.

Run the component gallery without a server:

```sh
mise gallery
```

You can also open **Component gallery** from the sign-in screen or the account
panel. The gallery has local navigation and a local composer. It sends no chat
messages. Its explicit **Open link in browser** action opens the sample link.

IBM Plex Sans Regular and SemiBold and IBM Plex Mono Regular are bundled under
the SIL Open Font License 1.1. `assets/plex.json` stores the unmodified TTF bytes
as base64 so the standalone executable needs no font files beside it. The app
registers these fonts before text layout. See `assets/plex.json.license` for the
upstream revision and `assets/OFL.txt` for the license.

## Check and build

```sh
mise check
mise test
mise build
```

Checks cover TypeScript and every native Svelte component. Headless tests use
GPUI hit testing and cover the demo, sign-in cancellation, late credential
delivery, Markdown normalization, image loading and cancellation, external-image
consent, retry, size limits, social content warnings, and gallery navigation,
section disclosure, buttons, and composer input. An optional live check uses the disposable
local development server and its bootstrap test accounts:

```sh
CHATTO_GPUIX_TEST_ORIGIN=http://127.0.0.1:4000 pnpm test:live
```

Run that check only against a disposable loopback server. It sends a synthetic
message and checks the directory, realtime rendering, paging, and sign-out.
Set `CHATTO_GPUIX_TEST_MEDIA=1` to also upload a synthetic image and check its
realtime delivery and native rendering. For this option, the server public URL
must match the test origin, so signed image URLs use that origin. Start the
root development server with `CHATTO_DEV_CHATTO_URL_OVERRIDE` set to the selected
loopback origin. Set `CHATTO_GPUIX_TEST_LONG_HISTORY=1`
to add 550 synthetic messages and check paging past 500 events. Both options
write test data to that disposable server.

On macOS, `mise build` creates `dist/Chatto GPUix.app`, which includes the Bun
runtime and GPUI module. Open it from Finder. This is an unsigned local build.
Signing, notarization, and releases are not configured. Run checks and builds
locally; this experiment has no CI workflow.

## Dependency boundary

This app now uses the root pnpm workspace so it can consume `@chatto/client`
and generated protocol types directly. gpuix-svelte keeps its custom Svelte
build nested inside its own package. The `tsconfig.json` paths and GPUix loader
keep the app and SDK Svelte adapter on that runtime. The web frontend retains
its normal Svelte runtime.

gpuix-svelte is pinned to 0.4.1. Development uses its Node loader plus `tsx` to
compile workspace TypeScript imports. Standalone builds use Bun. Do not add a
separate Svelte runtime dependency to this app. Native elements, events, and
CSS support differ from browser Svelte; read the installed types before adding
UI behavior. Keep Chatto protocol and server-state logic in `@chatto/client`.
