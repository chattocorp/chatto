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
  bounded event deduplication data.
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

For each direct mention, the bot immediately publishes a live-only typing
indicator in the thread. It refreshes the indicator every two seconds while Pi
works. Receiving clients remove an idle indicator after six seconds. The bot
then posts one final, durable reply.

The bot reads a window of up to 40 messages around the source message from the
public thread API. It excludes messages that came after the source message. The
context includes messages that do not mention the bot and messages from other
users. If the anchored resource read has not caught up, the bot uses the
realtime source message by itself and can still answer.

The bot reconstructs the thread as structured user and assistant turns. It uses
a stable, hashed session ID for each thread. It also replaces Chatto user IDs
with stable, hashed labels that apply only to that thread. It does not send
profile names. These stable inputs let an AI provider use prompt caching when
the provider supports it. Chatto remains the source of truth, so the bot can
reconstruct the conversation after a restart. The context has limits of 40
messages, 4,000 characters per message, and 32,000 characters in total.

Each mention gets an independent Pi session and an immutable thread snapshot.
The bot can run up to eight reply jobs at the same time, including jobs for the
same thread. Concurrent jobs do not include answers that are still in progress.
When a job is complete, the bot posts the answer and stops refreshing its typing
indicator.

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

The bot handles realtime events at least once. It saves an event as processed
only after the final reply succeeds. A failure can repeat the model request.
A process failure between creating the final reply and saving the source event
as processed can cause a duplicate reply because `CreateMessage` has no
idempotency key. Production bots need an application-specific idempotency
strategy. Typing indicators are transient and are not part of replay.

The bot saves a cursor only after it handles all earlier frames. On reconnect,
it asks Chatto for the missed replayable events. If the cursor is absent or is
not usable, it starts at the current live boundary. This example intentionally
does not use a realtime snapshot because it reads its finite resource state
through ConnectRPC. Reply jobs can finish in any order, but the bot saves their
event IDs and cursors in realtime delivery order. It does not save a cursor past
an unfinished reply.
