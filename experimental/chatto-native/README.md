# Chatto Native

An experimental Rust client built with [GPUI](https://gpui.rs/)
and the local [Chatto Rust SDK](../chatto-rs/README.md).

## Requirements

- macOS with Metal support. This is the only build target checked at this stage.
- Full Xcode, with its license accepted and Metal tools installed. Check the
  active developer directory with `xcode-select -p` and the Metal compiler with
  `xcrun --find metal`. Command Line Tools alone are not sufficient.
  If the compiler reports a missing Metal toolchain, run
  `xcodebuild -downloadComponent MetalToolchain`.
- [mise](https://mise.jdx.dev/).

## Development

Run these commands from this directory:

```sh
mise install rust
mise dev
```

Close the window or press Command-Q to stop the app. Change `src/main.rs`, then
run `mise dev` again to see the change.

## Connect and read

1. Enter the server origin, such as `https://chat.example.com`, in **Server URL**.
   The origin must have no path, query, or fragment. HTTP is
   accepted only for `localhost` and loopback IP addresses.
   Local development defaults to `127.0.0.1` with `CHATTO_DEV_CHATTO_PORT`, or
   port 4000 when that variable is absent. For a development URL that ends in
   `.localhost`, use `127.0.0.1` with the same port.
2. Enter your **Username** and **Password**. The password is masked. Fields
   support normal typing, paste, and Tab navigation. The server must allow
   direct password login. A server with only browser or provider login cannot
   use this sign-in method.
3. Press **Sign in**, or press Enter in the password field. The app opens a
   realtime connection and reads the server profile, room directory, sidebar
   groups, direct-message participants, and unread counts.
4. Select a room to read its latest 50 timeline events. Message text supports
   basic Markdown: headings, bold, italic, inline and fenced code, lists, task
   lists, and quotes. Link labels are shown, but links are not active yet.
   Tables, syntax highlighting, and full Markdown layout are not implemented.
   Embedded HTML stays literal text. Images show their alternate text and do
   not cause network requests.
5. Scroll near the top to load the preceding history page automatically, up to
   500 retained messages. The virtual list builds only visible rows and a small
   margin around them. Older pages keep the current message and pixel offset
   in place. New messages follow the bottom when you are already at the bottom.
   Use **Mark read** to mark the room through its latest root event as read.

Drag over message text to select it, double-click a word, or use Command-A to
select the message text. Press Command-C to copy the rendered selection.
Right-click text for **Copy**, **Select All**, **Copy message**, and
**Copy message ID**. **Copy message** copies the original Markdown source.
The author and timestamp also have the whole-message copy actions.
Message text stays read-only. Room and connection resets clear selection.
Selection in rows far outside the viewport can be discarded with the row cache.

The sidebar keeps server-defined group order and links. Other channels and
direct messages appear in separate sections. Groups can collapse. Unread rooms
are bold, and badges show important notification counts. WebSocket events update
sidebar resources and the active timeline. **Refresh** requests current data.
Connection status shows reconnect and synchronization progress. **Retry** starts
another connection after a terminal failure.

On disconnect or a permission change, the app clears visible server data before
it reads authorized resources again. Reconnect uses the last applied realtime
cursor. If the server cannot resume, the app loads current resources again.
Directory entries do not grant history access; the history API checks each room.

## Saved connection

The app saves the server origin, selected room, and collapsed groups in
`~/Library/Application Support/Chatto Native/settings.json`. The directory has
mode `0700`, and the file has mode `0600`. Set `CHATTO_NATIVE_CONFIG_DIR` to use
another settings directory. Only one app process can use a settings directory.

Access and refresh credentials are stored in macOS Keychain under service
`run.chatto.native.experimental.session`, with the settings directory as the
account key. Passwords and their input edit history are discarded when sign-in
starts. No message history or realtime cursor is saved. The app restores the
saved session at startup and renews access tokens. It saves each refresh identity
before the request, then saves replacement credentials before using them.

**Sign out** clears saved credentials and visible account data, cancels pending
reads, and asks the server to revoke the session. The server origin stays saved.
If Keychain deletion fails, the app reports the failure and retains the session
for a retry. If server revocation fails after local deletion, the app reports
that local sign-out succeeded but server revocation did not complete.

The app sends credentials and API requests only to the selected server. That
server can see the connection's IP address. The app does not fetch avatars or
attachments. Clicking a sidebar link opens the system browser, which contacts
the link destination. The app does not send messages yet.

Rust is pinned in `mise.toml` and `rust-toolchain.toml`. Keep these versions equal.
GPUI is pinned in `Cargo.toml`. Commit `Cargo.lock` to keep builds repeatable.
The first build downloads dependencies from crates.io and compiles GPUI.
Keep GPUI's `font-kit` feature enabled. Without it, the macOS backend measures
text but does not render glyphs.

## Build and check

```sh
mise check
mise test
mise build
```

The release binary is `target/release/chatto-native`. Run it directly to open
the app. App bundles, signing, and installers are not set up yet.

Run these checks locally. App tests cover sidebar ordering, private settings, exclusive access, and
credential-save failures during renewal. Headless UI tests check a 10,000-row
Markdown list and the real timeline's automatic history request. SDK tests use synthetic loopback HTTP
and WebSocket peers to check contracts, reconnect cursors, permission resets,
session termination, and cancellation of stale room reads.
The font smoke test starts the app with `--check-fonts` and checks proportional
system-font metrics, then exits without opening a window. This rejects GPUI's
dummy text backend, which reports the same width for all ASCII glyphs.
