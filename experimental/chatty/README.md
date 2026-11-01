# Chatty

An experimental Chatto terminal client. Chatty uses the standalone
[Go SDK](../chatto-go/README.md), Bubble Tea, Bubbles, Lip Gloss, and Glamour.
It runs as your human account, not a bot. It does not use an AI model.

## Run

From this directory:

```sh
mise chatty
```

Enter the server origin on the sign-in screen. You can also set `CHATTO_URL`,
or run `./bin/chatty --server https://chat.example.com` after building.
The module stays outside the root Go workspace. Its mise configuration sets
`GOWORK=off`. No `.env` file or API token is required.

## Sign in

- **Browser:** enter the server origin and press **Ctrl+B**. Complete the server's
  normal browser sign-in, including an external identity provider if configured.
  The server must enable `auth.loopback_client_enabled`. This existing client
  identity appears as **Unverified local app**; its renewable session ends at
  most 24 hours after sign-in. Chatty does not change the server setting.
- **Password:** enter your username and password, then press **Alt+Enter**. This
  requires the server to enable direct login. Passwords are masked and are not
  saved. The form clears the password when you submit it.

Browser login uses S256 PKCE, random state, and a callback bound to an ephemeral
port on `127.0.0.1`. The callback closes when login finishes, is cancelled, or
times out after five minutes. Press **Alt+Esc** to cancel a pending login.
Browser launching currently supports macOS (`open`) and Linux (`xdg-open`).

Session credentials are saved in `chatty/session.json` under Go's user config
directory. On macOS this is `~/Library/Application Support/chatty`; on Linux it
is normally `~/.config/chatty`. The directory has mode `0700` and session files
have mode `0600`. This is a private plaintext file, not an OS keychain.
Do not share it. Access-token renewal preserves a refresh request ID before
rotation, so an uncertain refresh can retry the same request.

**Ctrl+L** signs out and revokes the server session. If revocation fails, Chatty
keeps the local session and reports the failure. Quit with **Ctrl+D**.
Quitting preserves the session for the next launch. Plain letters never trigger
application shortcuts. On macOS, configure Option to send Alt/Esc in your terminal.
Chatty also saves the last successfully loaded room and reopens it on launch.
If that room is no longer available, Chatty opens the first accessible room.
Signing out clears the saved room.
Only one Chatty process can use the config directory. After a crash, remove
`running.lock` there only after you confirm that no Chatty process is running.

## Navigate and write

The left pane groups channels by the server's room groups, in the server's
group and room order. DMs have a separate **Direct messages** section. Channels
outside the returned groups appear under **Other rooms**; if group metadata is
unavailable, all channels appear under **Rooms**. Sidebar links are not shown yet.
Each section shows an unread marker when any child room has
server-reported unread attention. Click a section header, or select it and press
Enter, to collapse or expand it. Left/Right also collapse/expand sections.
Section states are saved with the session and restored across restarts. Signing
out clears them. Collapsing a section does not close its conversation or draft.

Rooms with unread attention have a bold dot. Notification counts appear at the
right edge: white for ordinary attention and orange for Important attention.
These count notification occurrences, not messages or rooms; counts above 99
show `99+`. Counts load on connection and update from realtime events. Reading
notifications in another client clears the badges here too. Servers without the
notification-count API keep unread dots but omit notification badges.
Click an orange badge to jump to the newest unread Important message notification
in that room. Click a white badge for the newest ordinary message notification.
Chatty loads history around the source message, selects it, and opens the thread
pane if the source is a thread reply. The lookup includes notification pages that
are not already loaded. Expired, read, and unknown notification kinds are skipped.
If no message target remains, Chatty reports this without navigating elsewhere.
Clicking a badge does not mark it read. Use **Alt+R** to load the latest history again.

The center pane shows the selected room. Open a message's thread to add a right
pane; in a narrow terminal, the thread replaces the center pane. Message bodies
use a compact IRC layout: time, a colored nickname, and message text on one row.
Wrapped lines align with the message body. Date separators mark each day.
Markdown rendering is retained. Selection uses a full-row background.
The bottom bars show message position, connection status, the input prompt,
and keyboard controls.

| Key                   | Action                                                                    |
| --------------------- | ------------------------------------------------------------------------- |
| Tab / Shift+Tab       | Select the next / previous visible pane                                   |
| Ctrl+N                | Select the next navigation pane or login field                            |
| Ctrl+B                | Hide or show the sidebar while signed in                                  |
| Up / Down             | Select a room or message                                                  |
| Enter                 | Send a nonempty draft; otherwise open the selected thread or sidebar item |
| Alt+Enter             | Send the draft to the focused conversation                                |
| Alt+V / Ctrl+O        | Open the selected message's first loaded image full-screen                |
| Esc                   | Close the thread, or select the sidebar                                   |
| Alt+PgUp / Alt+PgDown | Scroll the active timeline; load older messages at the top                |
| Alt+R                 | Return to the latest messages and refresh                                 |
| Ctrl+R                | Retry the exact retained unconfirmed send                                 |
| Alt+D                 | Discard the local unconfirmed send; does not delete a server message      |
| Ctrl+L                | Sign out                                                                  |
| Ctrl+D                | Quit                                                                      |

The input is always active. Type or paste to write while you navigate messages.
Enter sends a nonempty draft to the room when the room or sidebar has focus,
or to the thread when the thread has focus. Focus changes preserve separate room
and thread drafts. With an empty input, Enter opens a thread or
activates the selected sidebar item. Alt+Enter also sends. Left/Right edits the text
cursor except when the sidebar is selected, where it folds sections.
Click a room or message to select it. The mouse wheel scrolls
the timeline or sidebar under the pointer. Drafts are
separate per room/thread and stay in memory.
Drag a vertical separator to resize the sidebar or thread pane. Widths stay
within the available terminal space. **Ctrl+B** hides or shows the sidebar without
losing its width or section states. Tab skips the sidebar while it is hidden.
Empty panes show a loading indicator while
their data is requested. Scroll upward at the top of a timeline to load the
previous page. Moving the cursor to the first loaded message with Up also
loads older messages. Both actions use the same history-loading path in room
and thread panes. New pages preserve your position and selected message.
Each conversation pane follows incoming messages while it is at the bottom.
Scrolling back holds your reading position; scrolling to the bottom resumes
following. Sending a message returns its destination pane to the bottom.
Room and thread panes follow independently, including when image previews load.
An unconfirmed send is saved with its destination, body, and idempotency key;
Chatty never retries it automatically. A manual retry uses exactly that payload
within the server's idempotency window. Check history before retrying an old
attempt: the server's deduplication window is finite.

The SDK owns the server connection and shared stores. Startup loads the realtime
snapshot; message histories load on demand. Live changes update those stores,
with targeted resource reads when an event only identifies the changed resource.
Chatty does not poll the directory. Reopening a loaded room reuses its history.
Reconnect resumes from the last applied cursor when possible; a replacement
snapshot clears old histories when replay is unavailable. Access invalidations
clear private state immediately. Scrollback preserves the reading position while
live events update the store. Use **Alt+R** to return to the latest stored messages.

## Images

Ghostty and Kitty show attached images as inline thumbnails without cropping.
Click a thumbnail to open that image in a full-screen viewer. **Alt+V** or
**Ctrl+O** opens the selected message's first loaded image. Press **Esc**, either
shortcut, or click to close the viewer and return to the same scroll position.
Use Ctrl+O if macOS handles Option+V as a character instead of sending Alt+V.
The viewer does not change your draft.
It first shows the cached thumbnail, then downloads the original image and
replaces the preview when ready. The footer shows loading progress, image
dimensions, or a download failure. Closing the viewer cancels the download and
releases the original; the timeline thumbnail stays cached.

The images move with message text when you scroll or resize panes. Layout uses
the terminal's reported cell dimensions, with a fallback when no report arrives.
PNG, JPEG,
GIF (first frame), and WebP can be decoded. Other formats show the filename.
Failed previews also keep the filename visible.

Detection uses the terminal environment. Run `./bin/chatty --images kitty` to
enable Kitty graphics explicitly, or `--images off` to disable image downloads.
Automatic detection leaves tmux and screen in text mode; passthrough is not
implemented. Other terminals show attachment filenames.

Chatty downloads previews only for visible messages, with two requests at a time.
The cache holds up to 16 images in memory and terminal storage. It is cleared on
room changes, access invalidation, sign-out, and exit. Downloads are limited to
8 MiB, decoded images to 16 million pixels, and terminal images to 768 pixels
on their longest side. The full-screen viewer downloads originals up to 32 MiB
and decodes up to 32 million pixels. Its terminal copy preserves source resolution
up to 4096 pixels on the longest side; larger images are scaled down. Only one
original is retained at a time. Images are not saved to disk.

## Scope and data flows

This first prototype supports one account/server at a time, existing rooms and
DMs, text messages, thread replies, and older history. It does not yet create
rooms or DMs, mark messages read, edit/delete messages, add reactions, upload
attachments, display presence/typing, or provide offline history. Attachments
show filenames and supported image previews. The realtime snapshot supplies the visible rooms.

Only the selected Chatto server receives API requests and message text. Browser
login can also contact identity providers configured by that server; those
services see your browser connection. Direct login sends your password only to
the selected server. HTTPS is required except on loopback. Markdown rendering
does not download images or fetch linked pages. Attachment previews use fresh,
authorized asset URLs from the selected server. Downloads stay on that origin,
carry no bearer token, and do not follow redirects. No third-party image host
is contacted. No model provider receives data.
Timeline history is not saved; the local session file contains credentials,
the last room ID, sidebar section states, and at most one unconfirmed outgoing message.

## Development

```sh
mise check
CGO_ENABLED=0 mise x -- go build -o bin/chatty .
```

Run `mise check` in `../chatto-go` after SDK changes, and Gobotto's tests when
shared connection behavior changes. Tests use synthetic HTTP servers and TUI
models; a passing test does not prove login works against a deployed server.
