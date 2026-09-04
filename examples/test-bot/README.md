# Test bot

`test_bot` is a small, long-running integration example. It authenticates with
a bot API key, reads the current viewer and room directory with ConnectRPC, and
then listens to the protobuf realtime WebSocket. It uses the Pi SDK to generate
replies. The bot replies only to messages that contain a direct `@test_bot`
mention. A mention in a root message starts a thread for the reply. Role,
`@here`, and `@all` mentions do not trigger the bot. The bot logs event
metadata, but it does not log message text, user names, prompts, replies, or
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
  bounded event deduplication and pending-reply data.
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

For each direct mention, the bot first creates an italic `Thinking…` reply. It
then reads up to 40 current messages from the public thread API. This context
includes messages that do not mention the bot and messages from other users.
The bot sends their text to the selected AI provider and replaces the thinking
reply with the completed answer. It replaces Chatto user IDs with prompt-local
labels such as `Person 1`, and it does not send profile names. It always
includes the realtime source message, even when a resource read has not caught
up.

The Pi agent has one local extension named `web_fetch`. The model can use it to
fetch text from public HTTP and HTTPS URLs when current information helps with a
reply. Each request has a 30-second time limit and returns at most 100 KB. The
extension checks each redirect and blocks local, private, reserved, and
authenticated URL destinations. It treats web content as untrusted data and
asks the model to cite the source URL. It cannot read files, run commands, or
call Chatto by itself. A fetch sends the requested URL to the remote web server
from the machine that runs TestBot. The system prompt asks for concise,
professional answers. It also requires the bot to consult and cite
`https://docs.chatto.run/` when it answers questions about Chatto.

Then run `mise test-bot-build` and `pnpm --filter @chatto/test-bot start`.

The bot handles realtime events at least once. It saves the thinking reply ID
before it calls the AI provider. A retry then updates the existing reply instead
of intentionally creating another one. It saves an event as processed only
after the final edit succeeds. A failure can repeat the model request or final
edit. A process failure between creating the thinking reply and saving its ID
can still cause a duplicate reply because `CreateMessage` has no idempotency
key. Production bots need an application-specific idempotency strategy.

The bot saves a cursor only after it handles all earlier frames. On reconnect,
it asks Chatto for the missed replayable events. If the cursor is absent or is
not usable, it starts at the current live boundary. This example intentionally
does not use a realtime snapshot because it reads its finite resource state
through ConnectRPC.
