# Gobotto

Gobotto is an experimental Go bot for Chatto. It keeps one OpenRouter model
conversation per Chatto thread. A ping or DM starts a turn in that conversation.
The model posts a reply or reacts with an emoji. The Go client API is unstable.

When a new session opens, Gobotto immediately attempts an 👀 reaction on the
triggering message. This acknowledgement does not wait for the model. A failed
acknowledgement does not stop the turn and is not retried. Later messages and
restored sessions do not repeat this acknowledgement.

While a turn reads context, runs the model, or uses tools, Gobotto sends a typing
indicator in that thread and refreshes it every two seconds. Refreshes stop
before reply or reaction delivery, and on failure or cancellation. Typing errors
do not stop a turn. Chatto clears the indicator on a message or after six seconds
without a refresh; reactions and failed turns can leave it visible until expiry.
Debounce waits, background investigations, and session-title generation do not
send typing indicators. The conversation agent shows typing again when it answers
a new message or reports a completed investigation.

## Read-only code subagents

The bundled `code_investigator` can investigate the Chatto checkout. Configure
workers in `gobotto.toml`; restart Gobotto to apply changes:

```toml
[subagents.code_investigator]
description = "Investigate Chatto code with file and line references."
dir = "../.."
# model = "provider/model" # Omission uses conversation.model.
request_timeout = "3m" # Per model HTTP request; omission uses 90s, 0s disables it.
request_retries = 1 # Retries of one failed model request; 0 disables recovery.
max_auto_resumes = 1 # Completion-triggered continuations per investigation; 0 disables them.
# agents_md = false # Disable automatic AGENTS.md loading; enabled by default.
# Optional execution limits; omitted or zero means unlimited.
# timeout = "10m"
# max_tool_calls = 100
# max_model_steps = 150
system_prompt = "Trace relevant code paths and report gaps in the evidence."
```

`dir` resolves relative to the configuration file. It is a required read boundary,
not just a working directory. Startup must be able to open it. File tools use
Go's `os.Root` to prevent access outside that directory, including through
symlinks. Absolute paths and parent traversal are rejected. Ordinary reads and
discovery also exclude symlinks, dot-prefixed paths, `node_modules`, `vendor`,
and `brains`. Use a dedicated source checkout without private data;
these exclusions are not a general secret detector or an operating-system sandbox.

Ask Gobotto a code question, such as “Investigate how Chatto resumes a realtime
connection, and cite the relevant code.” Gobotto can call `delegate` with a
configured worker name and a task. The child receives that task and its configured
instructions, not the parent's transcript, identity, or tools. It can list
directories, read numbered lines, and search for literal text. It cannot run
commands, write files, use memory or MCP tools, post to Chatto, or delegate again.
All conversation participants can request these read-only investigations. The
parent receives the report and decides what to post in the original thread.

Workers load `dir/AGENTS.md` at the start of each investigation. Successful file
tools load applicable `AGENTS.md` files from the root to the target directory.
Each file applies to its directory and descendants; more specific guidance takes
precedence in that scope. Unchanged guidance is sent once per investigation.
Later reads disclose changes or removal. A search does not load guidance for all
matched subtrees: workers must read relevant files before relying on snippets.
Instruction reads use the same boundary and exclusions as source reads. Gobotto
does not read instructions above `dir` or from the user's home directory.

Set `agents_md = false` for a worker to disable automatic loading. This does not
block explicit file reads. Instructions cannot grant tools, write access, or owner
authority. Linked documents are not loaded automatically. Instruction files must
fit within 128 KiB each and 256 KiB per disclosure; unavailable or oversized guidance
produces an explicit notice instead of silent truncation. Repository instructions
are sent to the worker's model provider, like source files. Skill discovery and
activation are not implemented.

New investigations start a fresh model conversation. `delegate` accepts a
`resume_id` to continue a retained investigation in the same Chatto conversation.
Resume requires the same configured worker, model, and `dir`; current host
instructions and permissions replace the earlier settings. Each attempt uses the
current execution limits. Execution limits are optional,
with no fixed upper caps. Set `timeout`, `max_tool_calls`, and `max_model_steps`
for a child; omitted or zero values mean unlimited. Negative values are invalid.
Model steps include rejected requests. A configured call or step limit allows
one final text-only completion to report progress. Children run in the background
and can run concurrently. `delegate` returns a start receipt immediately.
The parent's `conversation.max_delegations` optionally limits their count per
turn. Each child uses its configured timeout; the short conversation-turn timeout
does not stop background work. Access invalidation and forced shutdown cancel
children. Normal shutdown waits for active work and reports completed results,
without processing queued user messages. Child completions use provider
defaults for reasoning and output length, independent of conversation overrides.

The same optional `timeout`, `max_tool_calls`, and `max_model_steps` settings
apply under `[conversation]`. Parent call/step budgets count the delegation as
one tool call; each child's internal work has its own configured budget. Restart
applies current limits to new and restored conversations. These settings govern
execution, not file/result sizes, retained context, or transport timeouts. Model
Parent model HTTP requests have a 90-second network timeout. Child requests use
`request_timeout`; the bundled investigator uses three minutes. Shift-Q cancels active work
even when all execution limits are disabled.

Reads accept UTF-8 regular files up to 1 MiB. Each read returns at most 200 lines
and 24 KB of text. Search scans at most 5,000 entries and approximately 8 MiB of
accepted file content, and returns at most 100 matches. Results report truncation
and skipped files. Search exclusions and limits can make findings incomplete.
The checkout is live, not a frozen snapshot; workers cannot verify Git status or
commit identity with these tools. No Git or shell commands run.

The turn inspector includes named child model and file-tool steps under the
delegation, with bounded details. Reported child tokens and cost contribute to
the parent turn and conversation totals. The parent session store retains
up to eight recent child conversations, including source reads and provider
metadata. Active children are never evicted, so they can exceed that retention
count. Results awaiting delivery and usage awaiting accounting are also protected.
Children checkpoint their private
ledger independently from the parent session before model requests and at the end
of each attempt. Reported usage is checkpointed in the child ledger first, then
applied once to the parent's totals. The child transcript size limit is 2 MiB of encoded JSON.
A restart marks unfinished children as partial; it never runs them automatically.
`investigation_status` lists retained investigations without starting work and can
return one report with its evidence. Resume IDs are internal, not user citations.

Completed investigations are code inspections, not runtime verification. Full
reports and excerpts include host-supplied verification limits: the worker did
not run tests, reproduce behavior, or inspect Git history. Excerpts also identify
report truncation. These limits apply to retained sessions too. Worker and parent
prompts require a distinction between source observations, inferred causes, and
checks still needed. These instructions guide model wording; they cannot verify
the model's conclusions.

Child model requests preserve safe failure codes and diagnostics, including HTTP
status, transport timeout, cancellation, and output limits. `request_retries`
defaults to one retry per failed request. Only transient transport failures and
HTTP 408, 429, and 5xx responses qualify. Retries use cancellable exponential
backoff and exactly the same transcript; they never execute tools again. Failed
requests without usage still might incur a charge. They are recorded as unreported,
and costs display as partial when some charges are unknown.

After an eligible partial result, a completion turn can use
`resume_investigation` to continue that same read-only child. The tool accepts
only eligible IDs from that completion event and an optional focused next check.
It cannot select another worker, directory, or replacement task. The persisted
`max_auto_resumes` allowance defaults to one per investigation; configuration sets
the allowance, and the run stores how many were used. Explicit user-turn resumes
remain available through `delegate`. Authentication, payment, cancellation,
context-limit, and storage failures do not authorize automatic continuation.

Each child retains up to 32 recent public updates, with a total text limit of
64 KiB. Individual updates are limited to 2,000 characters. This buffer includes
explicit interim assistant messages and bounded tool receipts, never private
reasoning. New conversation turns receive a small snapshot of these buffers.
Snapshots also report the host-observed phase and its start time, so waiting for
a model response is distinct from reading files or retrying a request.
The conversation agent can answer a status question while the child is still
running; the usual incoming-message debounce still applies. Detailed status reads
return the full bounded buffer without waiting for the child. A completed child
queues a host event for the conversation agent to report its findings. Host events
carry no user or owner authority and are not replayed automatically after restart.
They expose only reporting tools and the restricted recovery tool when eligible.

Failed or limited children return a partial result with the stop reason, the
latest explicit provisional finding when available, and up to eight recent tool
receipts. Evidence excerpts are bounded and marked when truncated. The parent can
answer from that evidence or continue the retained child with a focused question.
No extra model request is needed to construct a failure result. If no evidence was
collected, the result says that no diagnosis was produced. Invalidation prevents
stale result delivery. Retained findings remain available after an ordinary
interruption for a later authorized turn. A context-limit failure needs a narrower new investigation;
automatic compaction is not implemented.

Read-only file calls in a child can share one model response. The host executes
them sequentially, enforces the call budget for each operation, and records skipped
calls. Mixed or unknown-tool batches are rejected before execution. Parent tools
still require one call per response.

The conversation agent can call `post_update` to announce substantial work or
report a meaningful finding without ending its turn. A host announcement precedes
code investigation if the model has not already posted one. The parent can end
its turn with `defer_response` while children continue. It must not poll or wait
for them in a model loop. Progress posts are limited to once per minute within a
turn and use the same thread, edit checks, and cancellation rules as final replies.
Uncertain progress delivery is not retried. Background children do not keep typing active.

If the model attaches a written answer to a reaction call, Gobotto posts the text
and records that the reaction was skipped. Reaction receipts explicitly state that
no accompanying text was sent. Older ambiguous reaction receipts are clarified
when a session loads.

Investigation tasks, repository instructions, and retrieved source are sent to
the configured OpenRouter model/provider. Child transcripts and bounded inspection
excerpts are retained in private local storage. No additional MCP connection is needed for these built-in
read tools.

## Web access and MCP

The checked-in configuration enables Exa search and page fetching through its
hosted MCP server. No key is required for its rate-limited free access. To use
authenticated access, set `EXA_API_KEY` in `.env`; this can incur Exa charges.

```toml
[mcp.servers.exa]
url = "https://mcp.exa.ai/mcp?tools=web_search_exa,web_fetch_exa"
tools = ["web_search_exa", "web_fetch_exa"]

[mcp.servers.exa.optional_headers_from_env]
x-api-key = "EXA_API_KEY"
```

Remove the server section to disable it. Restart after configuration changes.
An unset or empty optional environment variable omits the header. Credentials
stay in HTTP headers; do not put keys in the endpoint URL or tool arguments.
The allowlist stays the same when a key is added, so Exa's research agent is not
enabled. See [Exa's MCP documentation](https://exa.ai/docs/get-started/exa-mcp).

Additional servers use the same configuration shape. Gobotto supports MCP
Streamable HTTP over HTTPS, with HTTP allowed for loopback development servers.
Each server needs an explicit nonempty tool list. Optional credential headers
are `Authorization`, `x-api-key`, or `api-key`, sourced from environment variables.
OAuth, local subprocess servers, remote prompts, sampling, and interactive
elicitation are not supported. Configured tools are available to all conversation
participants; configure only capabilities that they are permitted to use.

Gobotto discovers remote tools at startup and assigns names such as
`mcp_exa__web_search_exa`. The model receives only a compact name list and two
functions: `mcp_discover` returns one tool's description and argument schema;
`mcp_call` executes it. Chatto and memory tools remain directly available.
Schemas are loaded into conversation history only when needed and can be reused
across turns and restarts when their revision matches the current name list.
A changed revision tells the model to discover that tool again.
First use adds a local discovery step and another
model request. This reduces the always-present MCP tool payload, especially in
conversations that do not use web access. Once all schemas have been requested,
they remain in history; this does not guarantee lower total tokens for every run.
Definitions and ordering stay fixed until restart.
Startup fails if a server cannot connect or an allowed tool is missing. After
a transport failure, the next explicit call reconnects and verifies that tool
definitions have not changed. The failed call is not retried automatically.

Remote discovery and tool calls have 30-second timeouts. Responses are limited to 4 MiB
on the wire and 24,000 bytes of extracted content per tool result. Results mark
truncation and omitted non-text blocks. Text, embedded text resources, and links
are supported; structured JSON is used when no text is available. Remote tool
errors remain actionable results, marked as failures in the turn log. Local
schema discovery does not contact the server and returns one startup definition,
bounded to 32,000 bytes before the discovery-result envelope.

Search queries and requested URLs go to Exa, which also sees the bot server's
outbound IP address. Exa fetches requested pages on the bot's behalf. Returned
content goes to the configured model through OpenRouter and is retained in the
local session record. Other MCP servers receive their tool arguments and the
bot's outbound IP. Gobotto does not send the conversation transcript to MCP
servers. Internal instructions tell the model to minimize arguments, treat web
content as untrusted data, and cite source links. These instructions are not an
access-control boundary; operators must choose trusted servers and allowed tools.
Exa charges are separate from the OpenRouter cost shown in the stats panel.

## Run

Create a bot and API key in Chatto. Give the bot access to a test room and
permission to read messages, post thread replies, and add reactions.

Copy `.env.example` to `.env` in this directory and fill in these values.
The local `mise.toml` loads `.env` automatically. Git ignores `.env`.

| Variable             | Value                                             |
| -------------------- | ------------------------------------------------- |
| `CHATTO_URL`         | Server origin, such as `https://chat.example.com` |
| `CHATTO_API_KEY`     | Bot API key                                       |
| `OPENROUTER_API_KEY` | OpenRouter API key                                |

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

`conversation.max_output_tokens` optionally sets a positive per-completion output
budget. By default, Gobotto omits `max_tokens` and applies no output cap of its own;
provider defaults and model limits still apply. Choose a value supported by the model if you set a cap. Provider-counted reasoning
also consumes this budget. Longer brain definitions can require more output than
ordinary replies. If the provider reports an output-limit stop, the inspector
shows the cause and setting to change. Gobotto records usage but does not execute
partial tool calls, deliver partial replies, or retry automatically.

Model failures retain safe diagnostics in turn records and operational logs:
HTTP status codes, connection failures, timeouts, invalid JSON, and provider error
codes. Raw provider error bodies, request URLs, and credentials are not logged.
Response-read failures also record HTTP status and protocol, bytes read, declared
content length, and compression state. Error classification distinguishes
truncated bodies, HTTP/2 resets and GOAWAY codes, timeouts, connection resets,
and compression failures. These records help diagnose transport failures without
retaining response content. Parent conversation requests are not retried
automatically. Child requests use the recovery policy described above; a retried
request can incur an additional provider charge.
Older records with only `Model request failed` cannot recover the original cause.

Optional reasoning settings apply to conversation completions:

```toml
[conversation.reasoning]
effort = "medium"
# Alternatively, replace effort with max_tokens = 2048.
# enabled = true
# exclude = false
```

Omit the section to preserve provider defaults. `effort` accepts `none`, `minimal`,
`low`, `medium`, `high`, `xhigh`, or `max`; the selected model may support only a
subset. Use either `effort` or a positive `max_tokens` reasoning budget, not both.
If you set an output cap, the reasoning budget must be smaller than
`conversation.max_output_tokens` to leave room for tool arguments and replies.
`enabled = false` requests reasoning
off where supported. Gobotto rejects contradictory settings before connecting.
`exclude = true` hides returned reasoning; it does not disable reasoning or save
its tokens. These controls do not change the separate title model's configuration.
See [OpenRouter's reasoning controls](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens)
for model-specific support.

The inspector shows reported reasoning tokens separately as part of output, not
an additional charge or token total. Session totals retain them across restarts.
Missing telemetry appears as `reasoning n/a`; mixed reporting appears as partial.
Old snapshots do not contain reasoning counts, so their totals remain partial
when new counts arrive. Token counts are separate from the text of model reasoning.

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

### Conversation context

Each message batch includes the current UTC timestamp for calendar questions.
The internal prompt directs the model to use verified `@usernames` in replies,
keep raw user IDs internal, and avoid inventing handles when none are available.
This does not add a user-profile lookup tool.

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

Brain creation and archival require a triggering batch in which every message
author is an owner. Document use follows each brain's prompt guidance. Owner IDs and per-message author metadata are sent to OpenRouter with model
context and stored in private session transcripts, not operational logs.

### Means collections

Configure a directory where Gobotto can manage brains:

```toml
[memory]
directory = "./brains"
```

Owners can ask Gobotto to create a brain, for example: “Create a project notebook
that anyone can read and contribute to.” The `memory_brain_create` tool accepts a
name, title, description, JSON Schema, and Markdown instructions. Gobotto builds
the `MEANS.md` frontmatter; means validates the schema. Invalid arguments can be
corrected in the same turn when the tool reports `retryable: true` and
`mutated: false`. After a CLI failure, Gobotto permits correction only if it can
remove the empty reservation directory, proving no collection files were created.
Partial writes remain uncertain and block retries.
`memory_brain_archive` moves a brain into `.archive/` inside the directory;
it removes the brain from discovery but retains its files for manual recovery.
Only owner-only triggering batches receive these lifecycle tools. Each brain can
have one successful lifecycle change per turn. Uncertain mutations are not retried.

Gobotto discovers immediate child directories with a regular `MEANS.md`, excluding
hidden directories and symlinks. Names follow the same rules as fixed bindings.
The current catalog is included in every turn, including new and restored
sessions. Creation and archival become visible without a restart. Full guidance
is retrieved through `memory_info`; the host requires this before document access.

Each brain's Markdown guidance defines who may read or write, when to remember
information, and how to use it. Document tools are available to non-owners too.
The agent applies these rules using trusted author IDs and owner flags for each
message. These prompt rules are behavioral guidance, not data isolation. Catalogs
are shared across sessions, and retrieved content remains in saved transcripts.
Do not use brains for secrets or private user data.

Bind existing means collections in `gobotto.toml`:

```toml
[[memory.collections]]
name = "preferences"
path = "./memory/preferences"
writable = true

[[memory.collections]]
name = "reference"
path = "./memory/reference"
```

Initialize each collection with means before you start Gobotto. Each path must
contain `MEANS.md`. Relative paths resolve from the configuration file's directory.
Names must be unique and use lowercase letters, digits, underscores, or hyphens,
starting with a letter (maximum 64 characters). Restart to apply configuration
changes. The `means` executable must be on `PATH` when collections are configured.
Without collections or a managed directory, Gobotto does not require means.

Collection metadata comes from `MEANS.md` through `means info`. The catalog reads
each collection's current title and description. The collection-info tool returns
its complete Markdown instructions and schema as well. Keep this content in the
collection; `gobotto.toml` contains only the binding name, path, and write access.

Fixed bindings cannot be archived through tools. `writable` defaults to `false`
and remains a hard write limit for these bindings. Within that limit, their
Markdown guidance governs document use, just as for managed brains. Fixed and
managed collection names must not collide.

The agent can discover collections, inspect their schemas and instructions, search,
and read documents. Authorized turns can create and update documents. Before a
write, the agent must inspect the collection. Updates require the hash from a
document read in the same turn; means rejects conflicting changes. Update bodies
replace the full body, while application data follows means' field-patch rules.
The agent cannot supply collection paths, change existing schemas, delete
individual documents, or run shell commands. Creation and archival are limited to
the managed directory.

Search uses full-text matching and returns up to 20 documents. Each command has a
20-second timeout, a 64 KiB output limit, and a 32 KiB tool-argument limit. Large
results fail instead of being added to the model context. Calls execute one at a
time. A repeated identical successful write in the same turn returns its receipt.
After an uncertain write, further writes in that turn are blocked.

Memory tool results appear in the turn inspector and private session transcript.
Completed receipts are saved before the next model request. A later model or
Chatto failure does not undo a memory write. Started turns are not replayed after
a crash. Verify the collection before requesting another write after an uncertain
result. Removing a collection from configuration does not remove its previously
retrieved content from saved conversations.

Retrieved content is sent to OpenRouter as model context. Means runs with the
local means configuration and inherited environment. If means embeddings are
enabled, writes can also send document content to its configured embedding
provider and incur charges. Gobotto itself requests lexical search only. Disable
embeddings in means if you want collection operations to remain local.

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
instructions and model reasoning, and sends no output-token cap. Provider defaults
and model limits apply. Displayed titles remain limited to 60 characters. It uses the
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
and stats above a larger log for the selected turn at the lower right.
Turn rows show total input/output tokens and reported cost, including child calls.
Narrow panes hide tokens first, then cost. A `+` marks an incomplete total; `—`
means unknown. Drag either facing border or the gap between the turn list and
the details to resize them. The chosen ratio survives terminal resizes.
The turn log shows newest events first, one row per event, including incoming messages and outgoing
replies or reactions. Provider names and timings stay on the event row. Long
rows do not wrap; use Left/Right in the focused log to scroll horizontally.
Click a turn to inspect it. Use `l` for the separate operational log and `i`
to return; there is no mode toolbar. Operational logs follow new entries until
you scroll up.
The selected session has a full-row background. Click a session to select it,
click either pane to focus it, and use the mouse wheel over a pane to navigate.
Drag either border between the panes to resize them. Each pane keeps at least
two content rows; the chosen split is retained when the terminal resizes.

| Key                | Action                                                          |
| ------------------ | --------------------------------------------------------------- |
| Up/Down or `k`/`j` | Select a session or turn, or scroll the focused detail/log view |
| Tab or Shift-Tab   | Cycle focus through sessions, turns, and details                |
| `i` / `l`          | Show the inspector / operational log                            |
| `[` / `]`          | Select the previous / next turn                                 |
| Enter              | Expand or collapse message and tool details                     |
| `c` / `y`          | Copy the selected turn's response / input messages              |
| Page Up/Page Down  | Scroll details or logs                                          |
| End or `G`         | Select the latest turn, or follow the latest log                |
| Home or `g`        | Jump to the first retained log when the log pane has focus      |
| `q` or Ctrl-C      | Finish active work and report child results, then quit          |
| Shift-Q            | Cancel active work and quit immediately                         |

The view resizes with the terminal and needs at least 36 columns and 12 rows.
Normal quit keeps the UI open with a waiting message while active turns and
children finish. Completion reports can run during shutdown; queued user turns
do not start. Queued messages remain in the
persistent inbox for restart. Press Shift-Q to cancel instead; Gobotto still
closes connections and saves session state before it exits.
It retains up to 100 sessions and 200 operational log entries per session.
Newly added sessions become selected and visible in the top pane. The lower
panes and copyable session ID switch with that selection. Updates to existing
sessions preserve your selection.
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
- Each new message in an existing session's thread starts or resets a configurable
  debounce, including messages without a mention and messages from other
  participants. This also applies to restored sessions. Set `debounce = "4s"`
  under `[conversation]` to choose the quiet period. Omission defaults to four
  seconds; values must be positive durations, such as `750ms` or `6s`.
  Messages received during a model
  turn queue for the next turn. Their window elapses while the current turn runs.
- One model turn runs at a time across the bot. Ready batches run in deadline
  order; a busy model can delay execution beyond the configured collection window.
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
- The model cannot choose another Chatto destination. External tools are limited
  to configured capabilities; shell and arbitrary file access are not supplied.
- Turn deadlines, tool calls, model steps, and delegation counts are configurable
  in `gobotto.toml`; omitted or zero execution limits are unlimited. Batched
  calls are rejected before execution with explicit feedback and do not consume
  the execution allowance. Repeated rejections still consume model work steps.
  At a configured call/step limit, one final text-only completion reports
  completed and unfinished work from tool receipts.
  If the model attempts another tool during wrap-up, the host delivers a fixed
  partial-progress notice without executing it. Model output-token caps follow
  the conversation configuration; no cap is supplied by default.
- A failed turn is logged without private data; the bot continues listening.
  There are no automatic retries. A failed write can have reached Chatto.
- A disconnected realtime socket reconnects with exponential backoff and jitter.
  The UI reports disconnection and recovery. Gobotto saves the replay cursor and
  incoming messages together, then resumes from that cursor after reconnect or
  restart. Queued turns wait until replay reaches the live boundary and retain
  the configured debounce. Heartbeats refresh the saved cursor during idle time.
  Authentication and protocol rejection stop intake instead of retrying forever.
- Chatto cursors expire after 15 minutes. If replay is unavailable, Gobotto reports
  a gap and refreshes retained thread context before its next use. Messages missed
  in that gap do not automatically trigger turns. First startup without a cursor
  starts at the live boundary. Recovery does not scan all old threads for work.
- Message edits/retractions and room-access changes invalidate affected context.
  Before invalidating for an edit, Gobotto re-reads the thread and compares the
  affected message's model-visible fields. An unchanged message, such as one
  whose link preview was removed, leaves the active turn and transcript intact.
  Delivery waits for this check. Missing messages, changed content, or a failed
  verification still invalidate context; the check has a five-second timeout.
  Edits and retractions are matched to the thread root, retained context, queued
  messages, and active batch. Changes to unrelated messages in the same room do
  not cancel an active model request. Missing targets and context reads still in
  progress use conservative room-wide handling. Cancellation records identify
  whether the trigger was an edit, retraction, room change, or access change.
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
Gobotto supplies an explicit text-message projection to the model. SDK attachment
metadata, signed asset URLs, and creation timestamps are excluded from this
projection and the durable intake inbox. Edit checks compare the same text and
identity fields, so display metadata changes do not reset model context. URLs
that users write in message text remain part of that text.

Chatto and OpenRouter see the bot host's network IP address. OpenRouter routes
requests to model providers under its routing and data policies. The bot does
not send a participant's network IP address. The selected model incurs charges
on the configured OpenRouter account. No model call occurs before new intake.

The client requires HTTPS except for a loopback Chatto server. HTTP redirects
are rejected. Logs contain fixed status and error descriptions, not message
text, credentials, or provider response bodies.

## Development

Read [AGENTS.md](AGENTS.md) for development boundaries and runtime invariants.
Gobotto does not use FDRs; keep its documentation in this directory.

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
