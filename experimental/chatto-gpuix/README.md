# Chatto GPUix

A native Chatto client built with
[gpuix-svelte](https://github.com/khromov/gpuix-svelte) and the workspace
`@chatto/client`. Svelte components render through GPUI, without a browser.

The SDK owns authentication renewal, requests, room stores, paging, and realtime
WebSocket delivery. The app adds a native sign-in flow, a grouped sidebar with
channels and direct messages, Markdown, image attachments, link and social-post
preview cards, a virtual timeline, older-page loading
near the top, a message composer, and threads. It also includes an offline demo.

## Run

Use a Mac with Apple Silicon and GPU support. This is the only platform checked
here. From this directory:

```sh
mise setup
mise dev
```

`mise` provides Node, pnpm, and Bun. The GPUI module comes prebuilt from npm, so
this app needs no local Rust or Metal shader build. `mise dev` opens a native
window and reloads changed Svelte components. The window starts at 1100 × 740
logical pixels and has a native minimum size of 900 × 600. Development,
packaged builds, and the component gallery share `src/window.ts`. Restart the
app after changing these native window options. Close the window or press
Control-C in the terminal to stop it. Hot reload closes the old SDK client.

Enter the server origin and select **Continue in browser**. The server opens
its normal login and consent pages in your default browser. This supports the
server's password and identity-provider options. The app receives a one-use
code on a temporary `127.0.0.1` listener and exchanges it with PKCE. HTTPS is
required except for loopback development servers.

For this experiment, browser sign-in uses the built-in `chatto://desktop`
client identity. No public metadata deployment is needed. A temporary macOS
helper app receives `chatto://desktop/servers/callback?mode=popup` and passes
the URL to the running app over loopback. Codes stay in memory. The helper
claims the `chatto` URL scheme during sign-in and restores the previous handler
when the attempt ends. Avoid starting a separate Electron sign-in during that
attempt. The server's consent page identifies this experiment as **Chatto Desktop**.

To test a separate public identity, set `CHATTO_GPUIX_OAUTH_CLIENT_ID` to a hosted
client metadata URL. The local test fixture is
`tests/oauth-client.json`. It declares a native public
client with `http://127.0.0.1/servers/callback?mode=popup` as its callback; the
port is temporary. Its `client_id` must match its URL and `client_uri` must use
the same origin. No client secret is used.

The alternate **Use username and password** action uses a macOS system dialog
with a hidden password input. It requires a server that permits direct password
login. Passwords are discarded after sign-in.

The server gutter switches between saved servers and shows unread counts.
The plus button adds a server. Open **Settings and servers** to change their
order or remove a server. Each server keeps its room selection. All authenticated servers use
the SDK's realtime connections. **Sign out** clears only the selected server's
local credentials, then asks the server to revoke that session. The server
stays in the gutter for sign-in. Removal also deletes its local registration.

The app saves server registrations, access and refresh credentials, refresh
request IDs, active server, room selections, gutter order, and UI scale in
`~/Library/Application Support/Chatto GPUix/state.enc`. AES-256-GCM encrypts
this file with a random key held in macOS Keychain. Atomic writes use mode
`0600` inside a `0700` directory. Set `CHATTO_GPUIX_CONFIG_DIR` to use another
directory; its Keychain key is bound to the canonical directory path. Do not
move the file alone. A missing key or damaged file stops restoration without
overwriting it. Unlock Keychain or close another app instance, then select
**Retry opening saved sessions** when necessary.

Only one process can use a state directory. The SDK saves each refresh request
ID before sending it and saves the rotated credentials before using them.
This permits a retry after a lost response or restart. Message history and
composer drafts are not saved. The former `settings.json` supplies only legacy
sign-in defaults; it contains no reusable session.

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

## Threads

Select **Reply in thread** below a channel message to open its thread. Messages
with replies show a reply count instead. The thread panel displays the root,
replies, Markdown, and attachments with the same virtual timeline as channels.
It loads older replies near the top and receives updates through the SDK's
existing realtime connection. Send a reply with the thread's composer.

The app checks the root message's server-resolved reply authority and thread
slow-mode deadline before sending. Replies stay out of the channel timeline;
the root's reply count updates there. Wide windows show the thread beside the
channel. Narrow windows and larger UI scales use the conversation area for
the thread; **Back to channel** closes it. The channel draft is retained.
Changing rooms or servers closes the thread and clears its draft. Thread
selection and drafts are not saved across restarts. A followed-threads inbox
and thread-follow controls are not implemented.

To run the native thread smoke test against a disposable local server:

```sh
CHATTO_GPUIX_TEST_ORIGIN=http://127.0.0.1:4000 pnpm test:threads-live
```

This check posts synthetic root messages and replies. It checks opening,
sending, realtime replies and counts, separate channel and thread timelines,
scaling, draft retention, and room-switch cleanup. Local headless tests also
check reply restrictions, slow mode, and send failures.

## Native design components

The native app follows the web client's dark Chatto palette and compact layout.
`src/ui/theme.ts` defines semantic colors, fonts, radii, and themes for native
inputs and Markdown. These are local tokens with the web theme's names; the app
does not import browser CSS or web components.

`src/ui/` contains the native theme boundary, line icons, avatars, channel rows,
collapsible sidebar sections, message frames, preview surfaces, and composer.
`PaneHeader.svelte` owns the header row for server, channel, and thread panes:
height, padding, typography, divider, text truncation, and the actions slot.
Its named theme tokens scale from a 50-pixel height, 16-pixel horizontal padding,
and 10-pixel gap. It cannot shrink when a sidebar has many rows. The connected
app, demo, and gallery use this same component.

`IconButton.svelte` provides quiet icon actions with a standard hitbox and a
required accessible label. Header close/back actions, gutter actions, and
sign-out use it. `ActionButton.svelte` supplies default, primary, quiet,
disabled, and icon-only controls. The connected client, offline demo, and gallery reuse these parts.
Conversation search filters the loaded room directory; it does not search
message history.

### Typography and UI scaling

`src/ui/theme.ts` owns the typography sizes. Body text, controls, and inputs
use 14 logical pixels; secondary text uses 12; badges use 10; headings use
16, 20, or 24. Native Markdown has explicit text and line-height metrics.
IBM Plex Sans is the UI and prose family. Code uses IBM Plex Mono. The app
registers the bundled font files before layout, with no font download.

Open **Settings and servers** with the bottom gutter button, then use **UI
scale**. Enter any percentage from 50 to 200 and press Enter or select **Apply**.
**Smaller** and **Larger** change the scale by 10 percentage points. **Reset**
returns to 100%. The window shortcuts are Command-Plus, Command-Minus, and
Command-0 on macOS, or Ctrl with those keys on other platforms. The app saves
the scale in encrypted local state and restores it on restart. Older state
files start at 100%.

Scaling changes text, spacing, controls, icons, avatars, sidebar widths, image
frames, and native Markdown metrics together. The virtual timeline rebuilds
its row measurements and restores the visible message anchor. Composer drafts
stay in the same connected view. The operating system owns the window title
bar and password dialog; these use its scale settings.

`src/ui/scale.svelte.ts` owns the reactive scale factor. CSS lengths use the
`--size-*` variables from `src/ui/theme.ts`. Numeric icon and avatar dimensions
use `scaled(base)`. Keep base lengths immutable and scale them once. New UI
must use these paths so its geometry follows zoom. Native Markdown resolves
its metrics at the selected scale. Percentage dimensions remain relative to
the available window.

GPUix does not expose a whole-content zoom setting. Its `rem` conversion is
fixed at 16 logical pixels, and CSS transforms do not supply UI scaling. This
app uses live size tokens and native layout instead. See the
[upstream styling documentation](https://github.com/khromov/gpuix-svelte#styling).

Run the component gallery without a server:

```sh
mise gallery
```

You can also open **Component gallery** from the sign-in screen. The gallery has local navigation and a local composer. It sends no chat
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

To check browser sign-in and restoration against two disposable servers:

```sh
CHATTO_GPUIX_TEST_ORIGIN=http://127.0.0.1:4000 \
CHATTO_GPUIX_TEST_SECOND_ORIGIN=http://127.0.0.1:4001 pnpm test:browser-live
```

This check creates temporary encrypted state and serves local client metadata.
Open the authorization URL from `.context/native-browser-url.json` in a browser
and complete login and consent with the first server's bootstrap test account.
The check then tests credential rotation, a second session, restart, room and
server order restoration, gutter switching, sign-out, and removal. It removes
its temporary state when it ends. Host tests also check encryption, exclusive
access, damaged files, callback validation, and cancellation.

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
