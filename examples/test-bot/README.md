# Test bot

`test_bot` is a small, long-running integration example. It authenticates with
a bot API key, reads the current viewer and room directory with ConnectRPC, and
then listens to the protobuf realtime WebSocket. It uses the Pi SDK to generate
replies. In a channel, the bot replies only to messages that contain a direct
`@test_bot` mention. A mention in a root message starts a thread for the reply.
Role, `@here`, and `@all` mentions do not trigger the bot. In a direct message
(DM), each human message triggers the bot without a mention. A root DM message
starts a thread for the reply. A later message in that thread continues the
same conversation. The bot logs event metadata, but it does not log message
text, user names, prompts, replies, or credentials.

The regular `mise dev` command starts this bot. When the local data directory is
empty, a bootstrap-tagged development server creates the account on the first
startup, makes Alice its owner, joins it to `general`, and writes its generated
API key to `cli/data/bootstrap/test_bot.key`. Release builds do not run this
bootstrap code.

The bootstrap grants `room.join`, `room.list`, `message.read`, and
`message.post-in-thread`. A human must start a DM and include TestBot. TestBot
does not need `message.post` because its channel and DM answers are thread
replies. A narrower permission decision can still prevent a reply.

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

For each direct channel mention or human DM message, the bot immediately
publishes a live-only typing indicator. It uses a thread indicator in a channel
and a room indicator in a DM. It refreshes the indicator every two seconds
while Pi works. Receiving clients remove an idle indicator after six seconds.
The bot then posts one final, durable reply.

The bot reads a window of up to 40 messages around the source message through
the public thread API. It excludes messages that came after the source message.
The context includes messages that do not mention the bot and messages from
other users. If the anchored resource read has not caught up, the bot uses the
realtime source message by itself and can still answer.

The bot reconstructs each channel or DM thread as structured user and assistant
turns. It uses a stable, hashed session ID for each thread. It also
replaces Chatto user IDs with stable, hashed labels that apply only to that
conversation. It does not send profile names. These stable inputs let an AI
provider use prompt caching when the provider supports it. Chatto remains the
source of truth, so the bot can reconstruct the conversation after a restart.
The context has limits of 40 messages, 4,000 characters per message, and 32,000
characters in total.

Each thread has at most one active reply job. The bot waits 400 ms
before it starts the job so that it can combine a short message burst. An
unmentioned channel message can extend a pending reply, but it cannot start a
reply by itself. If a new message arrives while Pi works, the bot stops that
model call and starts again with a new immutable snapshot. This prevents
out-of-order or duplicate answers in one conversation. The bot can run jobs for
up to eight different conversations at the same time. When a job is complete,
the bot posts the answer and stops refreshing its typing indicator.

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

The bot can receive the same realtime event more than once. It saves an event as processed
only after the final reply succeeds. A failure can repeat the model request.
A process failure between creating the final reply and saving the source event
as processed can cause a duplicate reply because `CreateMessage` has no
idempotency key. Production bots need an application-specific idempotency
strategy. Typing indicators are transient and are not part of replay.

The bot saves a cursor only after it handles all earlier frames. On reconnect,
it asks Chatto for the missed replayable events. If the cursor is absent or is
not usable, it starts at the current live boundary. It logs the actual
`caught_up` recovery result. A failed resume that uses live-only fallback also
logs `recovery_gap`; the bot does not claim to have handled missed triggers. This example intentionally
does not use a realtime snapshot because it reads its finite resource state
through ConnectRPC. Jobs for different conversations can finish in any order,
but the bot saves their event IDs and cursors in realtime delivery order. All
source messages in a combined burst wait for the final reply. The bot does not
save a cursor past an unfinished reply.

Posted-message events provide the room kind, thread root, and structured
mentions. The bot does not load the room directory to classify messages. User
lookups, thread reads, typing, and reply posts caused by an event send
`Chatto-Realtime-Minimum-Cursor` with that event’s cursor and a 10-second
request deadline. A replica that is behind waits for its content view to reach
the boundary. The header does not wait for asynchronous notification work.
The cursor must still be within its 15-minute lifetime when the RPC starts.

Message metadata and server-enforced idempotency remain future work. Metadata
can help find a previous reply, but a separate lookup and post cannot prevent
two workers from posting concurrently. This example makes no exactly-once
claim.
