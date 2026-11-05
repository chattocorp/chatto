# Chatto Rust SDK experiment

This crate is a small asynchronous client for Chatto's public Connect JSON API.
Its Cargo package name is `chatto`. Its API is incomplete, unstable, and not
published. [Chatto Native](../chatto-native/README.md) uses it through a local
Cargo dependency. The crate has no GPUI dependency.

## API and boundaries

`Client::new(origin, token)` accepts an HTTPS server origin and a bearer
credential. HTTP is accepted only for `localhost` and loopback IP addresses.
Paths, user information, queries, and fragments are rejected. The credential
can be an API key or a human access token.

The asynchronous methods require a Tokio runtime:

- `Client::login_password(origin, username, password)` signs in through
  `/auth/login` on servers that allow direct password login. It does not retain
  the password. The returned `HumanSession` provides an authenticated `client()`,
  access/session expiry estimates, explicit refresh methods, and `logout()` to
  revoke the session.
- `server_profile()`, `room_groups()`, `profiles()`, and `notification_counts()`
  read sidebar resources. `mark_room_read()` is an explicit read command.
- `viewer()` reads the authenticated account's public profile.
- `rooms(offset, limit)` reads a live directory page, with a limit up to 100.
  Follow `page.has_more`. Offsets can shift; deduplicate room IDs when merging.
- `room_events(room_id, options)` reads raw timeline events and included
  resources. Unknown event kinds and resource fields remain available as JSON.
- `timeline(room_id, options)` projects non-deleted message posts and included
  author profiles. It omits lifecycle events and retains self-posts and echoes.
  Unmodeled message fields remain in `Message::metadata` as JSON. `body: None`
  means unavailable content; `Some("")` means a present empty body.

`TimelineOptions` accepts a limit up to 500 and either `before` or `after`.
Zero uses the server default. Keep cursors opaque and use them only for the
same account and room. The message projection keeps chronological server order.

Each request has a 30-second timeout and a four MiB response limit. Requests
never follow redirects or retry. Drop a request future to cancel it. `Client`
is immutable and cheaply cloned; create a new client when credentials change.
Errors and client debug output omit URLs, credentials, bodies, and user data.
Resource types deliberately do not implement `Debug`.

`examples/read.rs` reads the viewer, one directory page, and the first room's
latest message page. Set `CHATTO_ORIGIN` and `CHATTO_TOKEN` in the environment,
then run `mise x -- cargo run --locked --example read`. The example prints
only counts. Keep tokens out of shell history and command arguments.

`examples/password_login.rs` uses `CHATTO_ORIGIN`, `CHATTO_USERNAME`, and
`CHATTO_PASSWORD`. Run `mise x -- cargo run --locked --example password_login`.
It checks login and reads, then revokes the session. It prints only counts and
operation status. Keep credentials out of shell history and command arguments.

## Realtime and saved sessions

`realtime::LiveServer` owns one optional WebSocket connection and an in-memory
projection. Its `updates()` watch receiver supplies immutable snapshots with
connection status, a complete directory, and one active room timeline.
`select()`, `older()`, and `refresh()` send bounded commands. The caller sets a
history retention cap between 50 and 500 messages. Dropping the server cancels
its connection, reader, and pending resource requests.

The connection uses protocol v4 and `LIVE_ONLY`. After the caught-up boundary,
it loads resources through Connect with `Chatto-Realtime-Minimum-Cursor`.
Events invalidate resources rather than supply message previews. Reconnects use
only applied cursors. An invalid resume cursor causes a fresh resource load.
Disconnects, permission changes, and unknown durable events clear stale data.
Late reads from another room cannot restore its history. Cursorless typing and
presence events are ignored. Heartbeats refresh time-dependent unread counts.

The host supplies a `ClientProvider`. Every returned client must use the same
server and account. The callback must renew credentials when its argument is
true and should renew before local expiry. It must serialize refresh requests.
For human sessions, save `credentials()` in a secure store. To renew:

1. Call `prepare_refresh()` and save those credentials before any request.
2. Call `refresh()`. On an uncertain failure, keep the prepared credentials and
   reuse their request identity on the next attempt, including after restart.
3. Save the replacement credentials before returning its `client()`.

`HumanSession::restore()` reconstructs a saved session. The SDK does not write
files or access Keychain. Credential types do not implement `Debug`; never log
their serialized contents. The host controls local deletion and remote logout.

`examples/live.rs` observes a room for 20 seconds and prints counts only. Set
`CHATTO_ORIGIN`, `CHATTO_TOKEN`, and optional `CHATTO_ROOM`, then run
`mise x -- cargo run --locked --example live`. Its fixed-token provider cannot
renew tokens. The native host supplies the complete saved-session provider.

The host owns presentation, clipboard access, secure storage, and the lifetime
of each `LiveServer`. The SDK has no browser login or disk persistence.
Directory metadata does not grant history access; the server checks each read.

## Development

Run these commands from this directory:

```sh
mise install rust
mise check
mise test
mise build
```

Keep the Rust versions in `mise.toml` and `rust-toolchain.toml` equal. Commit
`Cargo.lock`. HTTP and WebSocket tests use synthetic data on loopback and do not need a Chatto
server. Run these checks locally.
