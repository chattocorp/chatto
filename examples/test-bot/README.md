# Test bot

`test_bot` is a small, long-running integration example. It authenticates with
a bot API key, reads the current viewer and room directory with ConnectRPC, and
then listens to the protobuf realtime WebSocket. It replies in the same thread
when a user directly mentions `@test_bot`. A mention in a root message starts a
thread for the reply. Role, `@here`, and `@all` mentions do not cause a reply.
The bot logs event metadata but it does not log message text, user names, or
credentials.

The regular `mise dev` command starts this bot. When the local data directory is
empty, a bootstrap-tagged development server creates the account on the first
startup, makes Alice its owner, joins it to `general`, and writes its generated
API key to `cli/data/bootstrap/test_bot.key`. Release builds do not run this
bootstrap code.

The bootstrap grants `room.join`, `room.list`, `message.read`, and
`message.post-in-thread`. It does not grant `message.post`, so the bot cannot
create root messages. A room-level permission denial can still prevent a reply.

To run the bot separately after you build it, set these variables:

- `CHATTO_TEST_BOT_SERVER_URL`: Chatto HTTP or HTTPS base URL.
- `CHATTO_TEST_BOT_API_KEY_FILE`: file that contains the bot API key.
- `CHATTO_TEST_BOT_STATE_FILE`: file that stores the opaque resume cursor and
  the bounded event deduplication window.

Then run `mise test-bot-build` and `pnpm --filter @chatto/test-bot start`.

The bot handles realtime events at least once. It saves an event as processed
after the reply request succeeds. A connection failure after the server creates
the reply but before the client receives the response can cause a duplicate
reply. Production bots need an application-specific idempotency strategy.

The bot saves a cursor only after it handles all earlier frames. On reconnect,
it asks Chatto for the missed replayable events. If the cursor is absent or is
not usable, it starts at the current live boundary. This example intentionally
does not use a realtime snapshot because it reads its finite resource state
through ConnectRPC.
