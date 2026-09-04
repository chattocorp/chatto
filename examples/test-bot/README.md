# Test bot

`test_bot` is a small, long-running integration example. It authenticates with
a bot API key, reads the current viewer and room directory with ConnectRPC, and
then listens to the protobuf realtime WebSocket. It logs event metadata but it
does not log message text, user names, or credentials.

The regular `mise dev` command starts this bot. When the local data directory is
empty, a bootstrap-tagged development server creates the account on the first
startup, makes Alice its owner, joins it to `general`, and writes its generated
API key to `cli/data/bootstrap/test_bot.key`. Release builds do not run this
bootstrap code.

To run the bot separately after you build it, set these variables:

- `CHATTO_TEST_BOT_SERVER_URL`: Chatto HTTP or HTTPS base URL.
- `CHATTO_TEST_BOT_API_KEY_FILE`: file that contains the bot API key.
- `CHATTO_TEST_BOT_STATE_FILE`: file that stores the opaque resume cursor and
  the bounded event deduplication window.

Then run `mise test-bot-build` and `pnpm --filter @chatto/test-bot start`.

The bot saves a cursor only after it handles all earlier frames. On reconnect,
it asks Chatto for the missed replayable events. If the cursor is absent or is
not usable, it starts at the current live boundary. This example intentionally
does not use a realtime snapshot because it reads its finite resource state
through ConnectRPC.
