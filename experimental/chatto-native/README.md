# Chatto Native

An experimental macOS Chatto client built with [GPUI Kit](https://gpui-kit.com/)
and the local [Chatto Rust SDK](../chatto-rs/README.md).

## Requirements

- macOS 15 or later with Metal support. This is the only checked platform.
- Full Xcode, with its license accepted and Metal tools installed. Check the
  active developer directory with `xcode-select -p` and the Metal compiler with
  `xcrun --find metal`. If necessary, run
  `xcodebuild -downloadComponent MetalToolchain`.
- [mise](https://mise.jdx.dev/).

Run these commands from this directory:

```sh
mise install rust
mise dev
```

Close the window or press Command-Q to stop the app. Terminal interrupts also
use the normal shutdown path. Run `mise dev` again after a source change.

## Servers and browser sign-in

1. Enter a server origin, such as `https://chat.example.com`, then select
   **Continue in browser**. HTTP is accepted only on loopback and localhost.
   Paths, query strings, fragments, and user information are not accepted.
2. Use the server's normal login and consent pages in your system browser.
   This includes the identity providers configured on that server. The native
   app does not receive your password.
3. Return to the app. Access and refresh tokens are saved in macOS Keychain.
   The app restores the saved servers and renews expired access tokens on launch.
4. Select **Add server** in the server gutter to connect another server.
   Every server has its own session, room selection, and collapsed groups.
   Background realtime connections keep directory and unread data current.

The gutter shows server logos when available, important unread counts, and a
sign-in indicator when a session ends. **Settings and servers** lets you change
server order, open a server, or sign out and remove it. Removing one server does
not remove the other sessions. Local credentials are removed before revocation.
The app reports when it cannot revoke a session on an offline server.

This experiment uses the built-in `chatto://desktop` OAuth identity with S256
PKCE, random transaction state, and exact callback validation. A temporary
macOS helper receives `chatto://desktop/servers/callback?mode=popup`. It forwards
the callback in an HTTP body to a bounded loopback listener. Auth codes and
verifiers are not saved or put in command arguments. The helper restores the
previous URL handler after completion, cancellation, timeout, or normal shutdown.
Run only one browser sign-in flow among the desktop experiments and Electron
app at a time, because they use the same URL scheme.

## Interface

`src/ui.rs` defines the Chatto color, typography, size, and radius tokens, plus
shared app frames, pane headers, navigation rows, avatars, image frames, and icon
buttons. `src/chat_shell.rs` composes the titlebar, server sidebar, account menu,
and People pane. The interface uses bundled
IBM Plex Sans and IBM Plex Mono. It does not download fonts. Pane headers use
one height and spacing contract. The minimum window is 900 × 600 logical pixels.

- Command-K focuses the conversation filter. It searches visible room and link
  names on the active server, not message history.
- Command-Plus or Command-Equals increases UI scale by 10 percentage points.
  Command-Minus decreases it. Command-0 resets it to 100%. Control also works.
- Scale is saved with the catalogue and limited to 50–200%. Text, controls,
  spacing, and panes scale together. Reflow keeps the current message anchor.

At 100%, body text is 16 pixels, secondary text is 14, captions are 12, pane
titles are 16, the sign-in title is 24, badges are 10, and code is 14. Pane
headers are 56 pixels high, server tiles are 48, and message avatars are 44.

The sidebar has **Overview** and **Search** controls. The account menu contains
connection refresh, settings, and sign-out. The conversation header can open
**People**, which shows the authorized server directory. This is not a room
membership or presence list. On narrow windows, it replaces the conversation
until you close it.

## Messages and media

Select a room to read its latest 50 timeline events. Scroll near the top to load
an earlier page, up to 500 retained messages. The virtual list builds visible
rows and a small margin. Earlier pages preserve the message and pixel offset.
New messages follow the bottom when you are already there. **Mark read** marks
the active room through its latest root event.

GPUI Kit renders headings, paragraphs, lists, task lists, quotes, tables, links,
and code blocks. HTML stays literal text. Syntax highlighting is not configured.
Select text and press Command-C, or right-click for **Copy**, **Select All**,
**Copy message**, and **Copy message ID**. Copy message keeps the original
Markdown. Selection and decoded images are cleared when the server or room's
authorization generation changes.

The app displays image attachments, inline Markdown images, server/profile
images, link cards, and bounded social-post previews. PNG, JPEG, WebP, and the
first GIF frame are supported. It shows up to eight attachments or inline images
per message and up to four images in each social post. One quoted post can be
shown. Content warnings keep a social post collapsed. SVG and video/audio
playback are not supported. Links and full attachments open in the system browser.

Visible previews renew expired attachment URLs through the room's authorized
asset API. Signed-URL changes do not discard an already decoded image. Raster
reads have a 12 MiB transfer limit, an eight-megapixel decode limit, four concurrent
reads, and a 48 MiB decoded-cache budget. Display previews are reduced to 800 × 600;
profile images are reduced to 128 × 128. Image bytes are not saved to disk.

The app connects to each signed-in server, which can see the connection's IP
address. Same-origin images load automatically. Foreign message images require
**Load image from [host]**, so that connection and its IP disclosure are explicit.
Foreign profile images keep initials. Raster requests send no cookies, bearer
credentials, or referrers, and do not follow redirects. Opening a link or
attachment in the browser contacts its destination. Browser sign-in can also
contact the identity provider selected on the server.

The app does not send messages or support threads yet.

## Saved state

The default directory is `~/Library/Application Support/Chatto Native`.
Set `CHATTO_NATIVE_CONFIG_DIR` to use another directory.

- `catalogue.json` stores server origins and names, their order, the active
  server, and UI scale. It contains no credentials.
- `servers/<id>/settings.json` stores a server's selected room and collapsed
  groups. Access/refresh tokens, OAuth client identity, expiry times, and the
  prepared refresh request ID stay in the server's Keychain item.
- Existing single-server settings and Keychain credentials are adopted in place.
  Migration does not require another login or copy tokens to a JSON file.

Directories have mode `0700`; settings files have mode `0600`. Writes use an
atomic rename and directory sync. Catalogue and server leases prevent concurrent
processes from rotating the same refresh token. The next refresh request identity
is saved before the request; replacement credentials are saved before use.
Normal shutdown waits for credential persistence and URL-handler cleanup.
No message history or realtime cursor is saved.

## Offline preview and local checks

`mise preview` opens synthetic conversations without settings, Keychain, or a
network connection. Set `CHATTO_NATIVE_PREVIEW_SCALE=1.5` to preview at 150%.
Set `CHATTO_NATIVE_PREVIEW_PEOPLE=1` to include the People pane.
For a native screenshot, set `CHATTO_NATIVE_PREVIEW_CAPTURE` to an output PNG
path, then run:

```sh
mise x -- cargo run --locked --features preview-capture -- --preview
```

The app captures the offline fixture with its renderer, then exits.

```sh
mise check
mise test
mise build
```

Checks cover separate session restoration, legacy adoption, server switching,
private-view resets, actual macOS callback delivery and handler cleanup, secure
image transport and URL renewal, Markdown selection, zoom, and virtual history.
SDK checks use synthetic HTTP/WebSocket peers for protocol and reconnect behavior.
The font smoke test starts the binary on its main thread and checks bundled
IBM Plex Sans metrics.

Rust is pinned in `mise.toml` and `rust-toolchain.toml`; keep them equal. GPUI Kit
is pinned in `Cargo.toml` and supplies the matching runtime, Base, Component, and
icons. The dependency is named `gpui` for GPUI macro compatibility. Commit
`Cargo.lock`. The default renderer is used; GPUI Fast is not enabled.

The release binary is `target/release/chatto-native`. App packaging, signing,
and installers are not set up. Run the checks locally; no experiment CI job is
required.
