# Test bot

`test_bot` is a small, long-running integration example. It authenticates with
a bot API key, reads the current viewer and room directory with ConnectRPC, and
then listens to the protobuf realtime WebSocket. It uses the Pi SDK to generate
replies. A direct `@test_bot` mention enrolls the bot in that thread. The bot
then replies to later human messages in the same thread without another
mention. A mention in a root message starts a thread for the reply. Role,
`@here`, and `@all` mentions do not enroll the bot. The bot logs event metadata,
but it does not log message text, user names, prompts, replies, or credentials.

Chatto automatically follows a thread for a directly mentioned account. The
bot uses this public follow state as its durable conversation list. It loads
the list at startup and applies realtime follow-state changes. This lets the bot
continue a conversation after a restart. Unfollow a thread as `test_bot` to
stop automatic replies in that thread.

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
  the matching followed-thread state and bounded event deduplication window.
- `CHATTO_TEST_BOT_AI_PROVIDER`: Pi provider ID. The default is `faux`.
- `CHATTO_TEST_BOT_AI_MODEL`: Pi model ID. This value is required unless the
  provider is `faux`.

Pi reads provider credentials from the standard provider environment. For
example, the Anthropic provider reads `ANTHROPIC_API_KEY`, and the OpenAI
provider reads `OPENAI_API_KEY`. TestBot does not select a real provider from an
ambient credential. You must set both TestBot AI variables to enable paid model
requests. Local development defaults to Pi's no-cost faux provider.

For example:

```sh
export CHATTO_TEST_BOT_AI_PROVIDER=anthropic
export CHATTO_TEST_BOT_AI_MODEL=claude-haiku-4-5
export ANTHROPIC_API_KEY=your-key
```

For each reply, the bot reads up to 40 current messages from the public thread
API. It sends their text to the selected AI provider. It replaces Chatto user
IDs with prompt-local labels such as `Person 1`, and it does not send profile
names. It always includes the realtime source message, even when a resource
read has not caught up. The Pi agent has no tools. It cannot read files, run
commands, or call Chatto by itself.

Then run `mise test-bot-build` and `pnpm --filter @chatto/test-bot start`.

The bot handles realtime events at least once. It saves an event as processed
after both the model request and reply request succeed. A connection failure
after the AI provider charges for a request, or after the server creates the
reply, can repeat work. It can also cause a duplicate reply. Production bots
need an application-specific idempotency strategy.

The bot saves a cursor only after it handles all earlier frames. On reconnect,
it asks Chatto for the missed replayable events. If the cursor is absent or is
not usable, it starts at the current live boundary. This example intentionally
does not use a realtime snapshot because it reads its finite resource state
through ConnectRPC.
