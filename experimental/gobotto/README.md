# Gobotto

Gobotto is an experimental Go bot for Chatto. It keeps one OpenRouter model
conversation per Chatto thread. A ping or DM starts a turn in that conversation.
The model posts a reply or reacts with an emoji. The Go client API is unstable.

## Run

Create a bot and API key in Chatto. Give the bot access to a test room and
permission to read messages, post thread replies, and add reactions.

Copy `.env.example` to `.env` in this directory and fill in these values.
The local `mise.toml` loads `.env` automatically. Git ignores `.env`.

| Variable | Value |
| --- | --- |
| `CHATTO_URL` | Server origin, such as `https://chat.example.com` |
| `CHATTO_API_KEY` | Bot API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |

From the repository root:

```sh
cd experimental/gobotto
cp .env.example .env
# Edit .env and gobotto.toml before starting the bot.
mise gobotto
```

The task compiles `bin/gobotto` and runs it. Git ignores the build output.

Run Gobotto in an interactive terminal. When the header shows `Connected`, send
the bot a DM or mention it in a room or thread it can read. Existing messages
do not start sessions.

## Configuration

Edit `gobotto.toml` to choose the conversation model and its instructions:

```toml
owner_user_ids = []

[conversation]
model = "z-ai/glm-5.3-flash"
system_prompt = """
Be friendly, direct, and concise. Match the conversation's language and tone.
Use an emoji reaction when a short acknowledgment is enough.
"""
```

`model` is a required OpenRouter model ID with tool support. `system_prompt` is
optional; an empty value uses only Gobotto's internal instructions. API keys stay
in `.env`. `OPENROUTER_MODEL` is no longer read: move your selection to
`conversation.model` if it differs from the included configuration.

Gobotto reads `gobotto.toml` from its working directory at startup. Set
`GOBOTTO_CONFIG` to select another file. Invalid TOML, unknown fields, and missing
model IDs stop startup before connecting to Chatto or OpenRouter.

Internal prompts teach the model about Chatto threads, authenticated identity,
supplied context, tools, and delivery receipts. Your prompt adds purpose, tone,
and preferences within those rules. It cannot add tools or change host-enforced
message routing. The prompts are sent to the model provider and stored in private
session snapshots; do not put credentials in them.

Restart after edits. Prompt changes apply to the next turn of both new and
restored sessions without discarding conversation history. They change the prompt
prefix and can reduce cache hits initially. Changing the model selects a separate
session storage namespace. The previous model's saved sessions remain on disk.

### Owners

Set the top-level `owner_user_ids` list to exact Chatto user IDs:

```toml
owner_user_ids = ["first-owner-user-id", "second-owner-user-id"]
```

Place this setting before any TOML section header. Omission or an empty list means
no owners. IDs are case-sensitive; handles and display names do not establish
ownership. Blank IDs and IDs with whitespace or control characters are rejected.
Restart to apply changes, including to restored conversations and queued input.

Ownership applies to each message author, never to a room, thread, or batch.
The host supplies each triggering message's author ID and owner status to the
model, even when that message already appears in thread context. An owner's
participation does not authorize other participants. The current system prompt's
owner list supersedes historical owner labels after a configuration change.
Message text, quoted instructions, and claims of ownership cannot change this list.
Other users can still converse with Gobotto. Reply and reaction targets do not change.

This setting supplies owner recognition to the model; it does not add privileged
tools. Future privileged actions must check the requesting user's identity in host
code. Owner IDs and per-message author metadata are sent to OpenRouter with model
context and stored in private session transcripts, not operational logs.

### Session titles

The included configuration uses a separate model for short topic titles:

```toml
[titles]
model = "openai/gpt-6-luna"
system_prompt = "Use a short, specific title in the conversation's language."
```

Remove `[titles]` to disable generation. The model makes one tool-free request
after the first successful turn, after reply or reaction delivery. Existing
sessions without titles receive one after their next successful turn. Titles
are saved in snapshots and shown in the session list. The lower pane keeps the
stable, copyable session ID; filenames do not change.

The request includes up to 6,000 characters of message excerpts, excludes system
instructions and model reasoning, and allows up to 64 output tokens. It uses the
same OpenRouter key but the configured title model and its provider. Reported
title tokens and cost are included in session totals. Titles are private display
data and are not added to operational logs or sent to Chatto.

The title request has a ten-second timeout and finishes before the next model
turn starts. Failure keeps the session ID as its label. Each session attempts
generation only once, including across restarts; changing the title model does
not regenerate existing titles. Startup alone does not call the title model.

## Terminal interface

The top pane lists thread sessions, newest first, in aligned columns for status,
turn count, tokens, cost, and start time. Narrow terminals hide secondary columns.
A `+` after cost means the total is partial. Further messages in that thread reuse
the same row. The default top pane uses at most eight rows; dragging the divider
overrides this limit and preserves your chosen split during terminal resizes.
The default view has sessions across the top, a turn list at the lower left,
and details for the selected turn at the lower right. Click a turn to inspect it.
The lower toolbar switches between the inspector and a separate operational log.
New sessions do not change your selection. Logs follow new entries until you scroll up.
The selected session has a full-row background. Click a session to select it,
click either pane to focus it, and use the mouse wheel over a pane to navigate.
Drag either border between the panes to resize them. Each pane keeps at least
two content rows; the chosen split is retained when the terminal resizes.

| Key | Action |
| --- | --- |
| Up/Down or `k`/`j` | Select a session or turn, or scroll the focused detail/log view |
| Tab or Shift-Tab | Cycle focus through sessions, turns, and details |
| `i` / `l` | Show the inspector / operational log |
| `[` / `]` | Select the previous / next turn |
| Enter | Expand or collapse message and tool details |
| `c` / `y` | Copy the selected turn's response / input messages |
| Page Up/Page Down | Scroll details or logs |
| End or `G` | Select the latest turn, or follow the latest log |
| Home or `g` | Jump to the first retained log when the log pane has focus |
| `q` or Ctrl-C | Quit, cancel active work, and close the connection |

The view resizes with the terminal and needs at least 36 columns and 12 rows.
It retains up to 100 sessions and 200 operational log entries per session.
Logs and inspection records survive restarts. Each session keeps up to 50 turn
records, trimming older records when their encoded size exceeds 2 MiB; the newest
turn is always kept. Inputs are bounded to 10,000 characters per message, and tool
details to 4,000 characters. Truncation is marked in the inspector.
The log title shows cumulative tokens (input plus output) and OpenRouter account
cost. Cached input is part of input, not an additional count. Cost comes from
the response's `usage.cost` field. Missing values show as `n/a`; a cost total
with some missing charges shows as `partial`. Totals cover reported requests only.
Logs show turn starts, reply/reaction delivery, omitted history, and failures.
They do not contain message bodies, user profiles, credentials, or raw model
output. The TUI adds no external connections.

The private inspector shows triggering messages first, then the reply or reaction
and its delivery status, an execution timeline, and token and cost figures.
Completed turns are labeled `Replied` or `Reacted` in the turn list. Expand a turn
to see tool arguments and results. Title-generation usage appears as its own step.
Each model step shows the serving provider when reported. Expanded details show
the OpenRouter request ID for comparison with its activity page. Missing cache
counts show as `cache n/a`; incomplete totals show as `partial`. Zero cache counts
in older snapshots also show as `n/a`, because those snapshots did not distinguish
a reported zero from an absent count.
Running operations update their elapsed time. The turn list and details scroll
independently. Model reasoning and credentials are not shown.

Diagnostic notes identify extra thread refreshes, multiple model calls for a
reaction, and cache-hit drops of at least 20 percentage points. These are evidence
to inspect, not proof of a fault. An uncertain delivery remains explicitly marked
and is never retried automatically. Older snapshots do not contain turn records;
their next turn begins the inspection history.

## Behavior and limits

- The bot authenticates its realtime subscription with the bot API key. It
  accepts server-verified direct, role, `@here`, and `@all` mentions that include
  the bot. Plain text that looks like a mention does not count. DMs, including
  replies in DM threads, start sessions without a mention. Ordinary channel
  replies require a mention to open a session. Once a session exists, every new
  message in its thread can start a turn. Echoes and self-messages are ignored.
- At startup, Gobotto reads its authenticated Chatto profile through the API.
  The model uses that account's login and display name as its identity; Gobotto
  is the software name. Restored sessions receive the current profile before
  their next model turn. Restart the bot after changing its Chatto profile.
- Each room/thread pair has one retained session for the connected bot identity.
  A top-level message is its own thread root, including in DMs. Replies in that
  thread reuse its session. Separate top-level messages remain separate threads.
- Each new message in an existing session's thread starts or resets a two-second
  debounce, including messages without a mention and messages from other
  participants. This also applies to restored sessions. A batch waits for two
  seconds of quiet. Messages received during a model
  turn queue for the next turn. Their window elapses while the current turn runs.
- One model turn runs at a time across the bot. Ready batches run in deadline
  order; a busy model can delay execution beyond the two-second collection window.
  The realtime reader keeps receiving messages during model work.
- Before a thread's first model request, the session reads its root and up to
  50 recent replies. Older history can be omitted. Incoming messages already in
  that snapshot are not inserted twice. Later turns append new messages to the
  existing model transcript. The model can explicitly refresh context with
  `read_thread`; normal follow-up turns do not fetch the thread again.
- A final text answer is posted in the ping's thread. Alternatively, the model
  calls `react_to_ping` with an emoji name such as `thumbsup` or `heart`. This adds
  a reaction to the latest triggering message in the batch and ends the turn
  without a text reply. Text replies use that same message for reply attribution.
- Assistant messages, reasoning metadata, tool calls, tool results, and delivery
  receipts stay in the transcript. The bot's own realtime posts are not inserted
  again. A failed tool turn does not leave an incomplete tool exchange.
- The model cannot choose another destination or use shell, file, or web tools.
- Each turn has a three-minute timeout, at most eight model requests, and at
  most eight tool calls. Each model response has a 2,048-token output limit.
- A failed turn is logged without private data; the bot continues listening.
  There are no automatic retries. A failed write can have reached Chatto.
- A disconnected realtime socket reconnects with exponential backoff and jitter.
  The UI reports disconnection and recovery. Gobotto saves the replay cursor and
  incoming messages together, then resumes from that cursor after reconnect or
  restart. Queued turns wait until replay reaches the live boundary and retain
  the two-second debounce. Heartbeats refresh the saved cursor during idle time.
  Authentication and protocol rejection stop intake instead of retrying forever.
- Chatto cursors expire after 15 minutes. If replay is unavailable, Gobotto reports
  a gap and refreshes retained thread context before its next use. Messages missed
  in that gap do not automatically trigger turns. First startup without a cursor
  starts at the live boundary. Recovery does not scan all old threads for work.
- Message edits/retractions and room-access changes invalidate affected context.
  Account deletion and viewer-permission changes invalidate all sessions. Active
  affected turns are cancelled and their queued input is discarded. The next
  message in that thread loads a fresh authorized snapshot. These events do not start
  a model turn. A write already sent before invalidation cannot be undone.
- At capacity, the least recently used idle session is evicted. It reloads from
  Chatto if addressed again. Each thread can buffer 128 incoming messages; an
  overflow stops intake instead of silently dropping context. A transcript over
  1 MiB or more than 4,096 remembered message IDs pauses that conversation until
  it is reset or evicted. Start a new thread to continue. Automatic summarization
  is not implemented yet. Eviction also removes the stored snapshot.
- Attachments and streamed answers are not implemented.

## Session storage

Gobotto writes one versioned JSON snapshot per conversation under
`.gobotto/sessions/`, relative to the working directory. Set `GOBOTTO_SESSION_DIR`
to use another location. The default directory is ignored by Git. Files contain
private conversation text and model responses, but no API keys. Directories use
owner-only access and snapshots use mode `0600`. Storage is not encrypted.

Each session has a readable ID, such as `calm-otter-012345abcdef`. The session
list and log pane show this ID. Its snapshot is stored at
`.gobotto/sessions/<namespace>/<human-id>.json`; the `HumanID` field identifies
the session and `History.messages` contains its model transcript for evaluation.
IDs remain stable across restarts and context resets. Existing snapshots receive
an ID and a matching filename automatically when loaded.
Click the session ID in the lower pane's title to copy it to the system clipboard.
The footer confirms the copy or reports that the clipboard is unavailable.

Snapshots include model history, message IDs, turn counts, reported token and cost
totals, and the OpenRouter session ID. Server URL, bot identity, and model select
separate storage namespaces. A process lock prevents concurrent writers. Writes
replace snapshots atomically. Invalid snapshots stop startup without overwriting
the files; restore a backup or move the affected directory before restarting.

Gobotto saves before a turn, after reported usage, before delivery, after a turn,
and on clean shutdown. An interrupted turn is never retried automatically; an
interrupted delivery is marked uncertain. Turn inspection records and operational
logs are stored alongside the transcript. The namespace's private `.realtime`
file stores the cursor, queued input, started batches, and recent completed IDs.
The inbox is limited to 4,096 entries and 16 MiB; overflow stops intake before
advancing the cursor. It retains up to 4,096 completed IDs to suppress duplicates.

Pending batches survive restart. Before model work starts, Gobotto marks the
batch as started on disk. Started work is not retried after a crash, even when
the crash happened before delivery; the UI reports these interrupted messages.
This avoids duplicate posts but does not guarantee an answer to every message.
Completed turns remove their input from the inbox. A crash before a usage
checkpoint can lose the latest reported charge.

Before the first resumed model turn, Gobotto reads the thread to check access and
refresh context. Changed or missing known messages reset the model context. This
also applies when older messages fall outside the API's recent-history window.
Otherwise, the retained model history is preserved and current context is appended.

## Prompt caching

The system instructions, tool definitions, and prior transcript remain stable
between ordinary turns. New input is appended at the end. Each thread gets an
opaque OpenRouter `session_id` for provider stickiness; it contains no Chatto IDs.
Claude models also receive the top-level ephemeral `cache_control` hint. Other
models use their provider's implicit caching behavior.

Chat Completions still receives the retained transcript on each request. Stable
prefixes allow provider cache reuse but do not guarantee a hit. Model thresholds,
cache expiry, provider routing, and context resets affect savings. See
[OpenRouter prompt caching](https://openrouter.ai/docs/guides/best-practices/prompt-caching).

## Data sent to external systems

Chatto receives the bot credential, timeline requests, replies, and reactions. OpenRouter
and its selected model provider receive the triggering message, instructions,
tool results, and model conversation, including the bot's authenticated account
ID, login, and display name. The profile is also stored in the private session
snapshot, but is excluded from operational logs. A thread read can include messages from
other participants and their Chatto user and message IDs. Do not use private
conversations unless participants agree to this use.

Chatto and OpenRouter see the bot host's network IP address. OpenRouter routes
requests to model providers under its routing and data policies. The bot does
not send a participant's network IP address. The selected model incurs charges
on the configured OpenRouter account. No model call occurs before new intake.

The client requires HTTPS except for a loopback Chatto server. HTTP redirects
are rejected. Logs contain fixed status and error descriptions, not message
text, credentials, or provider response bodies.

## Development

```sh
GOWORK=off mise x -- go test -race ./...
GOWORK=off mise x -- go vet ./...
```

The standalone [Chatto Go SDK](../chatto-go/README.md) owns all Chatto transport,
authentication, protocol types, and messaging operations. Gobotto imports its
public `chatto` package through a local module replacement. It has no access to
the SDK's private wire types. `agent/` owns the bounded loop, OpenRouter HTTP
transport, and model adapter. `internal/bot/` owns thread sessions, message
batching, and output delivery. `main.go` configures the worker and terminal.
`internal/tui/` owns the Bubble Tea model, Bubbles list and viewport, and Lip Gloss
layout. The worker sends operational events to the UI; only the UI goroutine
changes session history or selection. Exiting waits for the worker to stop.
The module stays outside the root Go workspace so experiments do not change
server dependency resolution. Run its checks explicitly.

Run the SDK's checks and regenerate its protocol types from its own directory:

```sh
cd ../chatto-go
mise check
mise codegen
```

The model transport uses
[OpenRouter Chat Completions](https://openrouter.ai/docs/api/api-reference/chat/send-chat-completion-request).
Tests use local HTTP/WebSocket fixtures and a fake model; they incur no model charges.
