# Botto

Botto is an example Chatto bot built with TypeScript, Runling, and
`@chatto/client`. Runling hosts one workflow run per Chatto conversation and
uses Pi for model interactions. Use it as a starting point for
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

From `examples/botto`, you can also run `pnpm start` or
`pnpm exec runling serve --host 127.0.0.1`. Runling loads `runling.config.ts`,
which registers Botto's Chatto source and root task. Mention the bot in a room,
send it a DM, or reply to one of its messages. Stop with Ctrl-C. Run only one
process per data directory and bot account.

Workspace startup loads Runling's TypeScript source and serves its console through
Vite. You do not need to compile Runling before `pnpm start`.

Open `http://127.0.0.1:5173` to inspect conversations in Runling. Each Chatto
thread has one run. Each accepted batch appears as a **Botto turn** child task.
The run waits between turns and keeps its identity after a graceful restart.
It stays open until an operator closes or cancels it in Runling. A later message
can start a new run for that thread, using the same saved agent transcript.
Use `pnpm start --port 5174` to select a different console port for another instance.
The console binds to loopback and has no authentication. Keep it local or put
an authenticated proxy in front of it.

The console shows turn timing, failures, tool activity, and model usage.
Botto omits message text, model replies, reasoning, and tool arguments from
Runling logs. Full model context remains in the private Pi transcripts.
The console's message composer cannot submit Chatto messages or grant authority.

## Configuration

Botto reads `botto.toml` from the working directory. Select another file with
`BOTTO_CONFIG=/path/to/botto.toml pnpm start`. Runling's `--config` flag selects
the TypeScript project configuration, not the Botto TOML file. Data paths are relative to the TOML
file. `.env` is still loaded from the working directory. Settings are read once;
restart after changing them. `pnpm dev` restarts the complete host after source
or configuration changes. Use this full restart for development rather than
`runling serve --watch`: Botto's task and source share process-local admission state.

### System prompts

Botto builds each agent's system prompt from two parts. Botto supplies the role,
Chatto behavior, and instructions for the tools available in that session.
`agent.system_prompt` adds the instance's identity, personality, and workflow
instructions. It does not replace Botto's instructions or enable tools.

Main agents and subagents both receive the instance instructions. Subagents keep
their assignment role and return results to the main agent. They receive web
instructions when web access is enabled, but no Chatto, memory, image, or
delegation tool instructions. Main agents receive the modules for their enabled
tools, plus conversation, formatting, and authorization rules.

Existing TOML files still work. You can remove copied Chatto, memory, and tool
rules from `agent.system_prompt`; Botto now supplies these rules. Keep instructions
specific to your bot. Restart Botto after changes. The separate title-description
task does not use the instance prompt.

### Agent skills

Put operator-maintained skills in `.agents/skills/` beside `botto.toml` (or the
file selected by `BOTTO_CONFIG`). Each skill uses a directory with a `SKILL.md`
file containing YAML `name` and `description` fields and Markdown instructions.
Chatto formatting guidance is part of Botto's internal prompt and needs no skill.

Main agents and subagents use Pi's skill catalog. They see names and descriptions
and can load instructions when needed. Only this directory is discovered;
repository and global skills are not added. Discovery runs when each session
opens. A missing directory means no skills. Invalid skills follow Pi's validation
rules and may be omitted from the catalog.

The `read` tool reads skills and supporting UTF-8 documents inside that directory.
Relative paths start at the skills directory; references inside a skill should
use the absolute path resolved from that skill's directory, as Pi instructs.
Reads reject paths and symlinks that resolve outside the skills directory,
non-regular files, binary content, and files over 256 KiB. Output is paginated
and limited to 20,000 characters of file text per call and per line, plus a
continuation marker when more lines remain.
Botto supplies no shell, execution, or file-writing tool. Skills that require
scripts cannot run them. Skill instructions do not grant additional tools.

Keep this directory under operator control and store only material that the bot
may read. Skill descriptions and loaded text are sent to the configured model
provider; loaded text can remain in private conversation transcripts. Update
skills between turns. Means-managed editable skills are not supported.

| Setting                         | Default                  | Purpose                                                                        |
| ------------------------------- | ------------------------ | ------------------------------------------------------------------------------ |
| `chatto.url`                    | Required                 | Chatto server URL                                                              |
| `chatto.api_key`                | Required                 | API key; use an environment reference such as `${BOTTO_CHATTO_API_KEY}`        |
| `chatto.mcp`                    | `false`                  | Give Pi all tools advertised by the Chatto MCP endpoint                        |
| `chatto.report_presence`        | `true`                   | Show the bot account as online while Botto runs                                |
| `chatto.respond_to`             | See below                | Which messages address Botto                                                   |
| `memory.enabled`                | `false`                  | Enable local Means memory                                                      |
| `memory.command`                | `means`                  | Means executable name or absolute path                                         |
| `memory.path`                   | `memory`                 | Collection directory, relative to the TOML file                                |
| `web.enabled`                   | `false`                  | Give Pi web search and page fetching from Exa's hosted MCP server              |
| `web.exa_api_key`               | Unset                    | Optional Exa API key; use an environment reference such as `${EXA_API_KEY}`    |
| `chatto.allowed_user_id`        | Unset                    | Restrict requests to one Chatto user ID                                        |
| `agent.model`                   | Required                 | Pi model in `provider/model` format                                            |
| `agent.reasoning_level`         | Pi/session default       | Reasoning effort: `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max` |
| `agent.name`                    | `Botto`                  | Name given to the model and used to label its thread context                   |
| `agent.system_prompt`           | Helpful assistant prompt | Persona and response instructions; supports TOML multiline strings             |
| `runtime.data_directory`        | `.botto`                 | Private transcripts, delivery ledger, and Runling history                      |
| `runtime.acknowledgement_emoji` | `eyes`                   | Reaction when a new conversation starts; empty string disables it              |
| `runtime.debounce_ms`           | `2000`                   | Quiet period for mentions, replies to Botto, and DMs, 2000–60000 milliseconds  |
| `runtime.passive_debounce_ms`   | `5000`                   | Quiet period for other text in active room threads, 2000–600000 milliseconds   |
| `runtime.turn_timeout_ms`       | `900000`                 | Turn deadline (15-minute default), 1000–3600000 milliseconds                   |
| `runtime.context_messages`      | `30`                     | Recent thread replies to include, 1–50                                         |
| `runtime.max_pending_messages`  | `100`                    | Accepted messages across queued and active batches, 1–10000                    |
| `runtime.max_threads`           | `8`                      | Maximum active or queued threads, 1–100                                        |

`agent.reasoning_level` applies to the root agent and delegated agents. Set it to
`low` to request less reasoning effort. An explicit value overrides the level in
saved sessions. If omitted, Pi uses its defaults or the saved session level.
Pi adjusts unsupported levels to the model's supported levels.

`chatto.respond_to` lists the reasons that address a message to Botto:
`direct_message`, `mention` (by name), `role_mention`, `here_mention`,
`all_mention`, and `reply`. The default is
`["direct_message", "mention", "reply"]`, so Botto ignores role, `@here`, and
`@all` mentions. While Botto shows as online, every `@here` message mentions
it, so enable `here_mention` only if Botto must answer those messages. After
Botto stops, Chatto shows it as offline within 60 seconds.

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

When a turn opens its agent, Runling checks Pi's local model catalog. If the
model is missing, it refreshes that provider's catalog with a five-second
deadline. `PI_OFFLINE` disables this refresh. The initial Chatto connection
has a 30-second deadline.

## Message flow

Botto answers text DMs, verified mentions, and verified replies to its messages.
It also considers ordinary text and image messages in threads where it has queued or running work,
or a saved Pi transcript. Saved transcripts keep threads active across restarts;
this does not make other threads in the room active. Messages from other bots
and from users outside `chatto.allowed_user_id`, when set, are ignored.
Every accepted DM message is addressed to Botto, including follow-ups and replies
to the other user's earlier messages. The passive-chatter default does not apply
to DMs; Botto should respond with text or an appropriate emoji reaction.
In a shared room thread, an explicit reply to another user's message does not start a turn,
even if it mentions Botto. This rule also applies to recovered notifications.
Accepted messages include the reply target's ID and author ID in the model input.
Each message also includes its verified addressing reasons. Unmentioned shared
thread activity first goes to a tool-free OpenRouter chat model with
recent thread text and attachment types. DMs, mentions, and verified replies to
Botto bypass it, including batches that mix addressed and unmentioned messages.
The classifier returns validated JSON: a reply, no response, or a reaction with
an emoji shortcode it chooses for the last message in the batch. Only a reply opens the main agent.
Ignored batches and classifier reactions do not open memory, load images, name
the conversation, show typing, or generate text. A classifier failure fails the
turn before effects and leaves the batch available for delivery retry. A failed
reaction is not retried because its outcome can be uncertain.

Human reactions added to Botto's messages also enter the classifier, including
in DMs. The classifier sees the emoji, its target message, and recent thread
context. It chooses a reply or silence; it does not react to Botto's own message.
Appreciation usually needs no reply, while confusion or an answer to a question
can warrant follow-through. Botto ignores its own reactions, other bots,
reaction removals, and reactions to other people's messages. The user allowlist
and passive debounce apply. Accepted reactions are saved and deduplicated by
event ID. Reactions missed while disconnected are not recovered from notification
history; queued reactions survive a restart.

The classifier should ignore neutral acknowledgements after a completed answer,
such as "mh" or "ok", and conversation between other people. Thanks and clear
appreciation directed at Botto should receive an emoji reaction without a textual
reply. The model chooses the emoji. A request after Botto
offers help, an answer to its question, or a nudge after an unanswered request
should receive a reply without another mention. These are model decisions,
not guaranteed rules.

Set `OPENROUTER_API_KEY` in the environment for passive participation. All
classification requests use OpenRouter; no direct provider API
is used. OpenRouter and its selected provider receive the supplied thread text
and attachment types; OpenRouter also receives the host's IP address. No memory
documents or image bytes are included. Runling records the decision,
duration, and reported token usage and cost without logging the full model response or reasoning.
Each successful classification also adds a concise participation log entry with
the action, a short plain-text explanation, the selected emoji if any, and elapsed time.
The classifier is instructed to explain the conversational situation without quotes,
personal details, or credentials. This restriction depends on model compliance.
A missing or empty explanation is logged as "Reason not supplied" and does not
invalidate the decision. Emoji selection remains open-ended.

```toml
[participation]
model = "z-ai/glm-5.3-flash" # Default; OpenRouter model ID without openrouter/
```

The classifier uses low reasoning and no saved session. Use an ordinary chat
model that can return JSON; Decisions API models are not supported here.
Botto reads targets outside the recent context window separately. If a target is
missing or access is denied, its author is `null`.

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
up to `runtime.context_messages` recent replies, including image-only messages, from that timeline. It
has no separate thread cache or event reducer.
While Chatto reconnects, a turn waits for the connection, until the turn
deadline. If a read fails, Botto reads again once. If the client disposes the
timeline while Botto waits for it, Botto gets the timeline again once. If the
second attempt also fails, the turn stops instead of using incomplete context.
The client keeps the 20 most recently used thread timelines, or
`runtime.max_threads` when that is higher, and disposes older ones. A later
turn in such a thread reads it again. The client can dispose a timeline that a
turn waits for only when many other threads start a read during that wait;
Botto then gets the timeline again once.
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
When Botto first joins an addressed conversation, it adds an 👀 reaction to the first
addressed message in that batch. Passive batches do not get this automatic
reaction. Later turns do not get this automatic reaction. An existing
Pi transcript also suppresses the acknowledgement after a restart. Set
`runtime.acknowledgement_emoji` to another emoji shortcode or `""` to disable it.
Reaction failures do not stop the answer. Botto leaves this reaction in place as
an acknowledgement, not a completion indicator.

The model can also use `add_reaction` for agreement, appreciation, or humor. This
native Pi tool uses the Chatto client and works without Chatto MCP. It can react
only to messages in the current batch or supplied thread context. The default
prompt tells the model not to duplicate the automatic acknowledgement reaction.

Botto shows typing for the whole main-agent turn. Addressed batches start typing
immediately; passive batches start typing after the classifier selects reply.
Botto stops typing and waits for a pending refresh before it posts the final
answer. Interim text before tool calls is posted during the turn. Conversation
title generation does not keep typing active.
Typing stops when the turn ends, including on failure. It posts completed parent assistant messages in
the thread, referencing the last message in the batch. These include interim
updates and the final answer. After choosing a textual reply, the model should
send a brief update before any tool call, including memory reads. Answers without tools, reaction-only
turns, and silent turns do not need an update. A successful turn can return no
text; provider errors and cancellation still fail the turn. Tools stay available
throughout the turn. The model decides when to send text; the prompt does not
guarantee an early update.
Updates are agent-written; there is no timed or fixed host message. They arrive
when the model completes a text message, not token by token. Child-agent text,
reasoning, and tool results are not forwarded directly.
Interim posts are serialized. The final answer is posted once, after the model
finishes and typing refreshes stop. A failed post stops model work without retrying an
uncertain write. An update already delivered remains visible if later work fails.
The live Pi session closes after each turn; its transcript stays on disk.
On the next message in that thread, including after a restart, Botto reopens the
same transcript. It does not preload all transcripts at process startup.

Messages that exceed the in-memory queue or thread limit wait in the delivery file.
Unaddressed room posts outside active threads do not start turns. Reactions to
Botto's messages use the classifier described above. Botto provides the reaction tool.
It loads only host-selected extensions and the configured instance's skill
directory, not ambient Pi extensions, prompt templates, or project instructions.

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
Normal thread context and assistant messages still use `@chatto/client`.
The model is told that its text messages are posted automatically, so it should use
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

Set `chatto.admin_user_ids = ["stable-user-id"]` to identify users who may request
changes to Botto's overall behavior. The default is an
empty list. Use stable Chatto user IDs, not display names, and restart Botto
after changing the list. `chatto.allowed_user_id` filters incoming messages;
it does not grant admin authority.

Global memory holds shared facts and general knowledge without a `user_id`.
Any participant can contribute this knowledge. The subject of a fact determines
its scope, not the person who supplied it. Personal memory holds facts and
preferences about the user identified by `user_id`. Botto can remember useful
facts without an explicit request; remembered knowledge does not grant authority
or set behavior rules.

Botto's system prompt supplies the configured admins. Each message in the input
prompt includes its verified author ID. The instructions tell Botto to attribute
each request to that author: users may change
their own personal facts and preferences, but even admins may not change another
user's memory. Chat claims and stored memories cannot grant authority. Mixed
batches still produce one answer; another participant's presence grants no rights.
These are model instructions, not tool-level access controls. Memory tools retain
access to the whole collection, so incorrect model behavior can violate the rules.

Existing documents with several memories remain valid. No data migration is
required. Update the workflow in your collection's `MEANS.md` and any custom
`agent.system_prompt` that requires one document per scope. Preserve custom schema
rules and other workflow guidance.

To keep memory from the previous hashed layout, stop Botto and move the document
Markdown files from `.botto/memory/<server-and-bot-hash>/` into `memory/`. Preserve
any edits to the old `MEANS.md` and schema. Means rebuilds its indexes. Alternatively,
set `memory.path` to the old collection directory. Pi session storage is unchanged.

Pi receives matching document IDs per scope and user in the request-only input
context, plus the full Means tool catalog with a `mcp__memory__` prefix. Means
serves the collection schema and workflow through its `info` tool; Botto does not
copy that workflow into its system prompt. The default
system prompt tells it to read relevant global and participant memory when a task
needs prior decisions, preferences, or facts, and to skip memory lookups for
self-contained questions. It also tells it to create separate documents for new
memories and update existing related facts. Read-before-update checks still apply.
Memory updates are model-driven. If you supply a custom `agent.system_prompt`, include
your desired memory behavior there. Edit the collection's `MEANS.md` to change
its workflow; Botto can read the current version through MCP `info`.

Batch authors and memory document IDs do not change the system prompt. They are
input data for the current turn. Room and thread IDs remain in the system prompt
when Chatto MCP is enabled; they stay fixed within a conversation. Memory document
IDs are refreshed for each turn and are not saved as additional transcript messages.

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
receives addressed messages, recent thread text, author IDs, saved context, and
selected image attachments. Image-only DMs and replies in active threads can
trigger a turn under the same participation rules as text.

Botto reads up to four images per turn, prioritizing the current batch, then
recent thread images. In model requests, each image stays with its original
message batch when that batch is retained. Images from additional thread context
have a separate context label and their source message and attachment IDs.
Chatto resizes each image to fit within 2048 pixels on each
side. Each download is limited to 4 MiB and 30 seconds, within the turn deadline.
The client fetches images only from the configured Chatto origin, with current
room authorization and without following redirects. No image URL or access ticket
is sent to the model provider. PNG, JPEG, WebP, and GIF responses are supported;
other file types are not loaded. Botto checks the active model's input types in Pi's
model metadata. It downloads images only when that metadata includes image input;
no separate Botto capability setting is needed. Downloads are reused within a turn.
The model receives a note when an image is unavailable, unsupported, or omitted.

When an overview is too small to read, the root agent can call `view_image`
directly with an attachment ID from the tool's allowed choices. Botto resolves
the owning message from the thread context. An optional `crop` has
`x`, `y`, `width`, and `height` values from 0 to 1. Coordinates refer to the full
original image after orientation correction, with the origin at the top left.
For example, `{ "x": 0.5, "y": 0, "width": 0.5, "height": 1 }` selects the right
half. Omitting `crop` selects the full image. Crops always use the original,
not the resized overview or a previous crop. Animated images use the first frame.

The tool rechecks attachment membership and room access. It permits four attempts
per turn, in addition to its automatic overviews. Each original is limited to
16 MiB and 40 million decoded pixels. Processing has a ten-second timeout; the
download and processing also share the turn deadline. Output is PNG, at most
2048 pixels on each side and 4 MiB, without enlarging small images. The tool
rejects calls for models whose Pi metadata does not include image input.
After the fourth crop attempt, the crop tool is removed. Receipts
show the remaining crop allowance; excess calls already queued in the same batch
return a no-crops-remaining notice instead of starting more downloads.

Crop bytes stay in memory for the turn. Tool arguments and text receipts enter
the private transcript; crop bytes do not.
Later turns can request the image again if it is still in context and accessible.
This tool contacts only the configured Chatto server; the configured model provider
receives the selected pixels.

Image bytes are added to the current model request. They are not stored in Pi
transcripts, delivery files, or Runling journals. Later turns read images again
from current thread context; images outside that window are not retained. Model
responses can describe images, and those responses remain in the transcript.

### Subagents

The root agent can call `delegate` with a self-contained `task` and optional text
`context`. Each call starts an ordinary Runling child task and a fresh agent session
using Botto's configured model. The root waits for the result and decides how to
use it in its answer. Delegation is optional; image inspection stays with the root.

Children receive only the supplied task and context. They do not inherit chat
history, images, memory, or the root's tools. When web access is enabled, children
can use the same Exa search and fetch tools. They cannot post messages, change
memory, execute commands, or delegate further. Their only local file tool is the
restricted skill reader described above. Ambient Pi resources are disabled.
The task and context go to the configured model provider; web queries
and fetch requests go to Exa when those tools are used.

Each child shares the turn deadline and cancellation. Cancelling its tool call
also cancels the child and closes its session. There is no separate child timeout,
background execution, automatic retry, or saved child transcript. Concurrent calls
use separate sessions. Results are limited to 16,000 characters, with a notice
when truncated, and are treated as untrusted evidence. Task arguments and results
remain in the root's private transcript. Child text is not posted directly to Chatto.

Runling's existing `taskTool` and agent-session APIs provide child execution,
activity, and combined usage and cost. Journal entries omit task/context and reply
content. Delegation adds model calls and can increase latency and cost.

### External services and storage

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

Runling stores journals, snapshots, and server diagnostics in the `runling/`
subdirectory of `runtime.data_directory`. Keep it on persistent storage with
the existing transcripts and delivery ledger. The project configuration sets
`RUNLING_DATA_DIRECTORY` to this directory unless you set it explicitly.
Separate instances must use separate data directories.
Existing transcript filenames and delivery files need no
migration.

Each message batch runs as one finite root execution. After the answer and task
cleanup, the root returns. A later batch reactivates the retained workflow ID;
there is no idle conversation task to interrupt on shutdown. The inbox serializes
batches for each thread.

Runling saves workflow state after execution, every 60 seconds, and on graceful shutdown.
After a crash, a newly created run can be absent from the snapshot; later input
then starts a new run for the same Chatto thread. The delivery ledger remains
the authority for queued, completed, and uncertain work. A recovered workflow
returns without repeating its original input. This also closes old waiting roots
on the first restart after upgrade. The source admits later work through reactivation.
Runling keeps open runs and removes closed runs after its 30-day retention
period. Botto's existing transcript files are separate and have no automatic
retention limit. A failed turn ends its execution; a later batch can reactivate
the same retained run. After retention removes a run, a new batch starts a new ID.
After restart, Runling's console shows the latest execution attempt's activity
and total token usage and cost across all attempts. Earlier attempts remain in
the run journal; the agent continues from its saved transcript.

## Logs and development

Botto names an untitled run with Runling's `createDescription()` task, alongside
the main reply. The task receives up to 8,000 characters of current message text,
without author IDs, memory, history, or attachments. It is instructed to omit
greetings and to exclude personal details and secrets. Its result sets the run
title; the main agent has no title tool. A saved title stays unchanged across
turns and restarts. It does not rename the Chatto thread or post a message.

Set `titles.model` to use a small model with your configured provider credentials.
It defaults to `agent.model`; `titles.reasoning_level` defaults to `off`. The selected
provider receives the text and the connection's IP address. This adds one model
request per naming attempt, with usage and cost recorded under the run. No tools
or external resources are available to the description task. The reply can be
posted while naming runs, but the turn joins naming before it ends. Naming has a
30-second deadline, bounded further by the turn deadline. A failure or empty
description leaves the run untitled and does not prevent the reply; a later turn
can try again. Model output can be imperfect. There is no automatic renaming.

Runling owns all log output. Connection and intake messages use a `[source:chatto]`
prefix. Work inside a conversation uses its run and task context and appears in
the console and run journal. Context reads, model turns, reply posts, and memory
operations are timed Runling steps. Runling reports model tool activity; Botto
does not add a second MCP completion log. Arguments, results, and raw errors are
omitted. `NO_COLOR` disables terminal colors. Terminal output remains readable
text when redirected; use Runling's JSON log files for structured records.
Set `RUNLING_LOG_LEVEL=debug` to include source and workflow debug logs and successful HTTP requests.
Message and turn logs include the Chatto thread root ID as `threadId`; top-level
messages use their own ID. Runling task references identify concurrent work. Steps include durations
and safe error hints. Logs omit message text, user identities, configured names,
credentials, URLs, and raw external errors. Runling writes its own server
diagnostics and run journals under the data directory. It records model usage
for each interaction and accumulates it within the conversation run. Restored
transcript history is not counted as new model usage. Cost uses model pricing
and is an estimate, not the provider invoice. Missing provider usage can appear
as zero. A resumed session alone does not prove a cache hit.

```sh
mise x -- pnpm --filter @chatto/example-botto check
mise x -- pnpm --filter @chatto/example-botto test
```

`runling.config.ts` reads settings once and registers the source and
recoverable conversation task. The Runling CLI owns serving, signal handling, and shutdown.
`src/source.ts` owns connectivity and the durable message ledger.
`src/conversation.ts` defines an ordinary Runling root task for each Chatto
thread. Its source routing starts or reactivates that root for one batch and waits
for its execution and cleanup. Routing reads Runling's retained source-owned runs
directly. Only a private, process-local ticket admits work; console messages and
recovery cannot replay callbacks.
Runling's batch queue owns ordering, capacity, and timers. `src/threads.ts`
keeps Botto's routing keys and transcript identities consistent.
`src/messages.ts` selects addressed
messages and ordinary text or images in active threads. `src/inbox.ts` supplies typing and
quiet-period policy. `src/turn.ts` connects a batch to a model turn and reply. `src/agent.ts`
configures Runling agent sessions; `src/thread-context.ts` connects Pi's request hook to the pure
merge function in `src/context-messages.ts`. `src/context.ts` selects text from
the shared client timeline in the same message format used by that merge.
`src/settings.ts` validates TOML.
`src/mcp.ts` lists the Chatto, Exa, and memory MCP servers, configures Pi's native
MCP extension, and releases shared memory locks. Runling observes tool activity.
`src/steps.ts` keeps external errors private at the Runling step boundary.
`src/memory.ts` finds memory documents and owns the shared memory lock.
Agent tests use a mocked Runling boundary. Botto's conversation tests use Runling's real
persistent host to check routing, restart, failure, and shutdown. A usage test runs
Botto through real Runling and Pi sessions with a synthetic provider stream. It
checks journal costs and confirms that reopening a transcript does not count old
tokens again or expose prompt and reply text. A restoration test opens a real Pi transcript
in a fresh process and checks its session ID and model context. Tests do not
contact a model provider. Native MCP tests use real Pi sessions and in-memory
protocol transports. The `test-botto` GitHub Actions job runs the type check
and tests when Botto, Runling, its client dependencies, or shared CI configuration change.
Its tests run without network connectivity, including child processes. No model
credentials are required or supplied.
