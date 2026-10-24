# Botto

Botto is an example Chatto bot built with TypeScript, the Pi SDK, and
`@chatto/client`. It has no Runling dependency. Use it as a starting point for
your own bot. This directory is part of the root pnpm workspace and uses Apache-2.0.

## Run

Create a separate bot account and API key in Chatto. From the repository root:

```sh
mise x -- pnpm install
cp examples/botto/botto.example.toml examples/botto/botto.toml
```

Edit `botto.toml` and set `chatto.url` and `agent.model`. Then create
`examples/botto/.env` with only secrets:

```dotenv
BOTTO_CHATTO_API_KEY=replace-with-the-bot-api-key
ANTHROPIC_API_KEY=replace-with-the-provider-api-key
```

Use the provider environment variables for your chosen model. Pi can also use
its normal authentication store. Both `.env` and `botto.toml` are ignored by Git.

```sh
mise x -- pnpm --filter @chatto/example-botto start
```

From `examples/botto`, you can also run `pnpm start`. Mention the bot in a room,
send it a DM, or reply to one of its messages. Stop with Ctrl-C. Run only one
process per data directory and bot account.

## Configuration

Botto reads `botto.toml` from the working directory. Select another file with
`pnpm start --config /path/to/botto.toml`. Data paths are relative to the TOML
file. `.env` is still loaded from the working directory. Settings are read once;
restart after changing them. `pnpm dev` restarts after source changes.

| Setting                         | Default                  | Purpose                                                                       |
| ------------------------------- | ------------------------ | ----------------------------------------------------------------------------- |
| `chatto.url`                    | Required                 | Chatto server URL                                                             |
| `chatto.api_key`                | Required                 | API key; use an environment reference such as `${BOTTO_CHATTO_API_KEY}`       |
| `chatto.mcp`                    | `false`                  | Give Pi all tools advertised by the Chatto MCP endpoint                       |
| `memory.enabled`                | `false`                  | Enable local Means memory                                                     |
| `memory.command`                | `means`                  | Means executable name or absolute path                                        |
| `memory.path`                   | `memory`                 | Collection directory, relative to the TOML file                               |
| `web.enabled`                   | `false`                  | Give Pi web search and page fetching from Exa's hosted MCP server             |
| `web.exa_api_key`               | Unset                    | Optional Exa API key; use an environment reference such as `${EXA_API_KEY}`   |
| `chatto.allowed_user_id`        | Unset                    | Restrict requests to one Chatto user ID                                       |
| `agent.model`                   | Required                 | Pi model in `provider/model` format                                           |
| `agent.name`                    | `Botto`                  | Name given to the model and used to label its thread context                  |
| `agent.system_prompt`           | Helpful assistant prompt | Persona and response instructions; supports TOML multiline strings            |
| `runtime.data_directory`        | `.botto`                 | Private Pi transcript storage                                                 |
| `runtime.acknowledgement_emoji` | `eyes`                   | Reaction when a new conversation starts; empty string disables it             |
| `runtime.debounce_ms`           | `2000`                   | Quiet period for mentions, replies to Botto, and DMs, 2000–60000 milliseconds |
| `runtime.passive_debounce_ms`   | `5000`                   | Quiet period for other text in active room threads, 2000–600000 milliseconds  |
| `runtime.turn_timeout_ms`       | `120000`                 | Turn deadline, 1000–3600000 milliseconds                                      |
| `runtime.context_messages`      | `30`                     | Recent thread replies to include, 1–50                                        |
| `runtime.max_pending_messages`  | `100`                    | Accepted messages across queued and active batches, 1–10000                   |
| `runtime.max_threads`           | `8`                      | Maximum active or queued threads, 1–100                                       |

Unknown settings and invalid values fail at startup. Keep API key values in the
environment and refer to the Chatto key from `chatto.api_key`. The configured name does not rename the Chatto account;
change its visible name in Chatto. The host adds the configured name and fixed
capability instructions to the system prompt. Changing the prompt does not add tools.

String settings can refer to environment variables with `${VARIABLE}`:

```toml
[chatto]
url = "${CHATTO_URL}"
api_key = "${BOTTO_CHATTO_API_KEY}"

[agent]
name = "${BOT_NAME}"
model = "${MODEL_PROVIDER}/${MODEL_ID}"
system_prompt = """
Answer questions about ${PROJECT_NAME}.
"""
```

Variables come from the process environment, including `.env`. Expansion happens
after TOML parsing, so quotes and newlines in values cannot change the TOML
structure. Values are expanded once; references inside an environment value are
not expanded again. A missing variable fails startup with its name and setting,
without printing its value. Empty results still fail non-empty string validation.
Use `$$` for a literal dollar sign, or `$${VARIABLE}` to keep `${VARIABLE}` as
literal text. Variable names use letters, digits, and underscores, and cannot
start with a digit. Shell commands and default-value expressions are not evaluated.
Numeric settings still require TOML integers, not quoted variable references.
`chatto.api_key` must be set in TOML; there is no implicit environment fallback.
Its reference can use any environment variable name. Provider API keys continue
to use Pi's dedicated environment variables or authentication store.

If the model is missing from Pi's local catalog, Botto refreshes the selected
provider's catalog once. Pi's `PI_OFFLINE` environment switch disables this
refresh. The initial Chatto connection has a 30-second deadline.

## Message flow

Botto answers text DMs, verified mentions, and verified replies to its messages.
It also answers ordinary text in threads where it has queued or running work,
or a saved Pi transcript. Saved transcripts keep threads active across restarts;
this does not make other threads in the room active. Messages from other bots
and from users outside `chatto.allowed_user_id`, when set, are ignored.

Ordinary messages in active room threads wait for 5 seconds of quiet by default
(`runtime.passive_debounce_ms`). A mention, verified reply to Botto, or DM
selects `runtime.debounce_ms` (2 seconds by default) for the whole pending batch.
Both settings require at least 2 seconds. Existing TOML values remain in use;
change them to use the new defaults. Values below 2000 fail validation.
Later ordinary messages keep the addressed delay until the batch starts. Each new
message or typing signal in that thread resets the selected quiet period;
duplicate messages do not. Botto ignores its own typing and typing from users
outside `chatto.allowed_user_id`, when set. Typing never starts a turn or interrupts
one already running. Continued typing can keep a queued batch waiting.
After the batch starts, the next batch selects its delay independently. Top-level
DM messages also wait, but each still opens a separate reply thread. Room-level
typing delays queued root messages in that room, not replies in other threads.

Turns in one thread run in order. Messages received during a turn form the next
batch; its timer can elapse while the current turn runs. The model receives the
batch and recent thread context. Botto gets the thread's live timeline from
`@chatto/client`. The client reads the latest page once, when the thread first
becomes active, and keeps it current through realtime delivery. This includes
edits, deletions, and the reload after a reconnect. Botto selects the root and
up to `runtime.context_messages` recent text replies from that timeline. It
has no separate thread cache or event reducer.
Botto retains the timeline while it reads it. If a read fails, Botto reads
again once; if that also fails, the turn stops instead of using incomplete
context. The client keeps the most recently used thread timelines and
disposes older ones; a later turn in such a thread reads it again.
Only the accepted batch and model responses
are saved as conversation messages. The recent thread snapshot is added to the
current model request without saving it again in the transcript. This preserves
ordinary human replies as context without accumulating repeated snapshots.
Before each model request, Botto merges snapshot messages into retained batches
by message ID. This supplies edited bodies without repeating the previous body.
Snapshot copies of Botto replies are omitted when their text already occurs in
a retained assistant response; human messages with the same text are kept.
This check uses Pi's current request context, so messages removed by compaction
can be supplied by the snapshot again. Compaction summaries can still describe
the same events. These request changes do not rewrite the saved transcript.
When Botto first joins a conversation, it adds an 👀 reaction to the first message
in that initial batch. Later turns do not get this automatic reaction. An existing
Pi transcript also suppresses the acknowledgement after a restart. Set
`runtime.acknowledgement_emoji` to another emoji shortcode or `""` to disable it.
Reaction failures do not stop the answer. Botto leaves this reaction in place as
an acknowledgement, not a completion indicator.

The model can also use `add_reaction` for agreement, appreciation, or humor. This
native Pi tool uses the Chatto client and works without Chatto MCP. It can react
only to messages in the current batch or supplied thread context. The default
prompt tells the model not to duplicate the automatic acknowledgement.

Botto shows a typing indicator and posts the
final answer in the thread, referencing the last message in the batch.
The live Pi session closes after each turn; its transcript stays on disk.
On the next message in that thread, including after a restart, Botto reopens the
same transcript. It does not preload all transcripts at process startup.

Messages that exceed the in-memory queue or thread limit wait in the delivery file.
Ordinary room posts and reactions do not start turns. Botto always provides the reaction tool.
It loads only host-selected extensions, not local
Pi extensions, skills, prompt templates, or project instructions.

## Optional Chatto MCP tools

Set `mcp = true` in the `[chatto]` table to give Pi every tool advertised by
the configured Chatto server's `/mcp` endpoint. The server must have MCP enabled.
Botto uses `chatto.api_key` for authentication. There is no tool allowlist;
this includes write tools such as posting messages and joining or leaving rooms,
subject to the bot account's permissions. `chatto.allowed_user_id` limits who
can trigger a turn; it does not limit which rooms the tools can access.

Pi's native MCP extension connects the configured servers, discovers their tools,
and exposes them directly as `mcp__chatto__<tool>`. Built-in file and shell tools
stay disabled. Botto does not load ambient Pi MCP configuration.
Each turn closes its connections when it ends, including on cancellation.
Pi waits up to 10 seconds for tools before the first model request. If a server
fails or is still connecting, Pi can continue with the available tools; its
request context includes server status. Botto does not retry tool calls.
Normal thread context and final replies still use `@chatto/client`.
The model is told that its final reply is posted automatically, so it should use
the posting tool only for additional messages required by the task.

## Optional web access

Set `enabled = true` in the `[web]` table to connect Exa's hosted MCP server at
`https://mcp.exa.ai/mcp`. Pi then exposes `mcp__exa__web_search_exa` and
`mcp__exa__web_fetch_exa`, which search the web and read pages as Markdown.
This uses Pi's native MCP extension, as described above, and adds no dependency.

```toml
[web]
enabled = true
exa_api_key = "${EXA_API_KEY}"
```

`exa_api_key` is optional. Without a key, Exa limits calls by the bot host's IP
address and returns an error when the limit is reached. One research question
can use several calls, so set a key for regular use. Create a key in the Exa
dashboard; its free tier includes monthly credit. Botto sends the key in the
`x-api-key` header, not in the URL.

## Data and recovery

### Local memory

Install the `means` CLI and enable local memory in TOML:

```toml
[memory]
enabled = true
command = "means"
path = "memory"
```

Run `means --help` and `means agents` for CLI setup and usage. The executable must
be available to the Botto process; `command` can be an absolute path. This feature
is independent of `chatto.mcp` and needs no additional network service.

`memory.path` selects one Means collection. It defaults to `memory`, relative to
the TOML file, and also accepts an absolute path or `${VARIABLE}`. The example's
`memory/` directory is the collection: Git tracks `MEANS.md`, which contains the
schema in YAML frontmatter and the workflow in its Markdown body. Git ignores
private memory documents and Means indexes. To use another directory, copy
`MEANS.md` and `.gitignore` there first. Botto requires `MEANS.md`; it does not
copy templates at runtime.

To migrate an existing collection with `means.schema.json`, stop Botto and move
the schema into the `schema` field of the YAML frontmatter in `MEANS.md`. Use the
example's `memory/MEANS.md` as a template, and preserve any custom schema rules
and workflow guidance. Remove `means.schema.json`, then run
`means doctor --path <memory.path>` before restarting Botto. Keep the memory
documents and their IDs unchanged.

Each memory can have its own Markdown document. Frontmatter `data.scope` selects
global or user memory; `data.user_id` associates user memories with a stable
Chatto user ID. Each scope can have zero or more documents. Botto searches these
fields for global memory and users who send a message in an accepted batch.
It does not create empty documents. Documents are reused across threads and
restarts. Use a separate `memory.path` for each server and bot account; Botto no
longer adds an identity hash to this path.

Existing documents with several memories remain valid. No data migration is
required. Update the workflow in your collection's `MEANS.md` and any custom
`agent.system_prompt` that requires one document per scope. Preserve custom schema
rules and other workflow guidance.

To keep memory from the previous hashed layout, stop Botto and move the document
Markdown files from `.botto/memory/<server-and-bot-hash>/` into `memory/`. Preserve
any edits to the old `MEANS.md` and schema. Means rebuilds its indexes. Alternatively,
set `memory.path` to the old collection directory. Pi session storage is unchanged.

Pi receives lists of matching document IDs per scope and user, the collection
workflow, and the full Means tool catalog with a `mcp__memory__` prefix. The default
system prompt tells it to read global and participant memory before answering,
create separate documents for new memories, and update existing related facts.
Memory updates are model-driven. If you supply a custom `agent.system_prompt`, include
your desired memory behavior there. Edit the collection's `MEANS.md` to change
its workflow; new turns read it again through MCP.

Botto uses a short-lived Means subprocess to find document IDs. Pi then opens
its own Means connection for model tool calls and closes it when the turn ends.
Botto serializes Means calls within its process. It uses
Pi's sequential tool mode when memory is enabled, so a batch of memory calls
cannot block itself while waiting for the shared lock. Updates use
Means content hashes to detect stale reads. Continue to run only one Botto process
per data directory and memory collection; do not edit memory concurrently through other processes.

Memory contains private Markdown. User documents organize facts; they are not
access-control boundaries. The model can access the complete collection. The
workflow tells it to keep private user facts out of global memory and other users'
replies, and to never store credentials. Memory read through tools is sent to the
configured model provider and can also remain in Pi transcripts. Clearing a memory
document does not erase older transcripts. Missing Means or invalid collection
data during memory discovery fails the turn. Later connection failures follow
Pi's MCP behavior described above.

Botto connects to the configured Chatto server and model provider. The provider
receives addressed messages, recent thread text, author IDs, and saved context.
With MCP enabled, the provider also receives tool definitions and tool results,
which can include messages from other rooms the bot can access. Tool calls and
results are saved in the Pi transcript. MCP uses the configured Chatto server;
it adds no separate external service.
With web access enabled, Botto connects to Exa. Exa receives the search queries
and URLs that the model selects from the conversation, and fetches those pages
from its own servers. The target sites do not see the bot host. The model
provider receives the search and page results, and Pi transcripts keep them.
Each connected service can observe the bot host's IP address. Chat participants' browsers
do not connect to the model provider through this integration.

Transcript file names bind conversations to the server origin, bot account,
room, and thread. The data directory contains private conversation data. New
files use private permissions. There is no automatic retention limit; stop
Botto before deleting transcripts.

Botto saves accepted messages in a private `delivery-*.json` file in the same
data directory. Keep this directory on persistent storage. Run only one Botto
process for each server/account pair. Queued messages survive a restart,
including messages still in their quiet period and messages that exceed the
in-memory limits. Completed message IDs prevent duplicate turns after recovery.

On startup, after reconnect, and every 30 seconds, Botto checks retained Chatto
notifications for missed DMs, mentions, replies, and activity in threads where
it participates. It reads current message content and checks the author before
accepting recovered work. It does not dismiss notifications. Keep the relevant
notification classes enabled for the bot account; muted classes and ordinary
thread activity without a notification cannot be recovered this way. Chatto
retains message notifications for up to 90 days. The first run of this version
starts recovery at its startup time; it does not answer historical notifications.
Later runs use the saved boundary. Removing the delivery file loses this boundary
and duplicate detection. Recovery scans all retained notifications since that
boundary, so accounts with many notifications require more requests.

Connection startup waits through temporary outages. Failed message verification
and read-only turn preparation are retried. Before the first model or reaction
operation, Botto saves the batch as `running`. A failed or interrupted running
batch stays in the delivery file for operator review and is not repeated
automatically. Check the Chatto thread and Pi transcript before deciding whether
to repeat that work. A saved model reply does not prove that Chatto received it;
model tools can also have completed an external action before interruption.
Botto does not retry uncertain posts or tool calls. Failed turns produce a local
log entry, not a chat reply. These changes add no external service connections.

## Logs and development

Timestamped terminal logs show connection changes, accepted messages, debounce
delays, batch sizes, context reads, model turns, reply posts, and shutdown.
MCP tool calls log one completion or failure with `tool`, `threadId`, and
`durationMs`. This includes Chatto tools, Exa tools, Means tools, and host-owned memory
document discovery and creation. Arguments, results, and raw errors are omitted.
Terminals that support color show colored severity labels and dimmed metadata.
Set `NO_COLOR` to disable colors. Other terminals use plain text. Redirected
output uses one JSON object per line, with `timestamp`, `level`, `message`, and
metric fields. Botto checks each output stream separately: informational logs
use stdout; warnings and errors use stderr.
Message and turn logs include the Chatto thread root ID as `threadId`; top-level
messages use their own ID. Process-local turn numbers identify concurrent work. Stage logs include durations
and safe error hints. Logs omit message text, user identities, configured names,
credentials, URLs, and raw external errors. Botto does not create log files.
Each model turn logs `Pi session created` or `Pi session resumed`, with the
persisted Pi `sessionId` and `historyMessages` count loaded before the new prompt.
The session ID stays the same when Botto reopens a thread after restart.
Each turn also logs `Pi token usage`: `inputTokens` (uncached input), `outputTokens`,
`cacheReadTokens`, `cacheWriteTokens`, and `estimatedCostUsd`. These are changes in
Pi's session totals during this turn, including recorded compaction and failed
work; restored history is not counted again. Cost uses Pi's model pricing and is
an estimate, not the provider invoice. Missing provider usage can appear as zero.
Cache reads show actual reported reuse; a resumed session alone does not prove
a cache hit.

```sh
mise x -- pnpm --filter @chatto/example-botto check
mise x -- pnpm --filter @chatto/example-botto test
```

`src/main.ts` owns connectivity and shutdown. `src/messages.ts` selects addressed
messages and ordinary text in active threads. `src/inbox.ts` owns batching and
ordering. `src/turn.ts` connects a batch to a model turn and reply. `src/agent.ts`
owns Pi sessions; `src/thread-context.ts` connects Pi's request hook to the pure
merge function in `src/context-messages.ts`. `src/context.ts` selects text from
the shared client timeline in the same message format used by that merge.
`src/settings.ts` validates TOML.
`src/mcp.ts` lists the Chatto, Exa, and memory MCP servers, configures Pi's native
MCP extension, and observes tool outcomes.
`src/memory.ts` finds memory documents and owns the shared memory lock.
Most tests use a mocked Pi boundary. A restoration test opens a real Pi transcript
in a fresh process and checks its session ID and model context. Tests do not
contact a model provider. Native MCP tests use real Pi sessions and in-memory
protocol transports. The `test-botto` GitHub Actions job runs the type check
and tests when Botto, its client dependencies, or shared CI configuration change.
Its tests run without network connectivity, including child processes. No model
credentials are required or supplied.
