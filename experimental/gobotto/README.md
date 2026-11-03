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

## Recurring tasks

Configured bot admins (`owner_user_ids`) can ask for recurring tasks, for
example: “Every six hours, check for free games and post useful findings here.”
`schedule_create` accepts a title, task, `destination` (`room` or `thread`), and
`schedule` string. Room delivery creates a top-level message in the current room;
thread delivery replies in the current thread. The saved destination is an object
with `kind`, `room_id`, and, for thread delivery, `thread_id`. The host supplies IDs.
Gobotto confirms the selected destination. To move delivery, request an update
from the new destination and explicitly select its room or thread.

Schedule examples:

- `0 9 * * 1-5`: weekdays at 09:00 UTC (five-field cron).
- `CRON_TZ=Europe/Berlin 0 9 * * 1-5`: weekdays at 09:00 Berlin time.
- `@daily`: midnight UTC.
- `@every 6h`: every six hours, starting six hours after creation.

Calendar schedules default to UTC. Timezone schedules follow the Go cron library's
daylight-saving rules: a missing local time is skipped; a repeated local time can
run twice. Elapsed intervals accept whole seconds from one minute to 30 days.
The first run is the next occurrence after creation.
`schedule_list` shows all schedules for this bot, across rooms and threads. Any
configured admin can manage any schedule from any conversation:

- `schedule_update` changes the supplied title, task, timing, or destination fields.
  Omitted fields and the enabled state stay unchanged. Managing a schedule from
  another conversation does not move its destination.
- `schedule_pause` disables future runs and revokes pending delivery.
- `schedule_resume` enables the schedule, with its next occurrence after now.
- `schedule_delete` revokes and removes the schedule.

These mutations require `id` and the current `revision` from the list. A stale
revision is rejected without changes. Each changed definition increments its
revision. Receipts include the resulting state and whether a run is still active.
An active run must finish before update or resume; pause and delete can revoke it.
Already completed or in-flight external effects cannot be undone. Gobotto must
not claim a schedule stopped until a pause or delete tool returns success.
Schedule data is loaded on demand through `schedule_list`. A clear
request such as “stop posting jokes” authorizes deletion of the matching schedule;
the admin does not need to name the scheduling tool. Temporary stops use pause.

Management tools are exposed only for entirely admin-authored input. Destination
IDs and authorizing admin come from host context, not tool arguments. Definitions
retain `created_by` and `created_from` as provenance. `authorized_by` records the
admin who approved the current revision. Creation context does not restrict management.
The built-in
`brains/schedules/` collection has a fixed definition and one document per task.
The document body holds the task as Markdown; frontmatter holds timing, destination,
revision, and provenance. Approval covers both metadata and task text.
Generic memory tools cannot access it, including through fixed binding aliases.
The separate `.runtime.json` ledger binds execution authority to structured
definition data, its revision, and the current server/bot identity. Markdown/JSON
formatting does not change approval. Manual task edits do not authorize execution;
invalid definitions are disabled and can be deleted by an admin. Do not edit files
while Gobotto runs. An exclusive
directory lock prevents multiple processes from executing the same schedules.
There can be at most 100 registered schedules.
Gobotto refreshes the code-owned `MEANS.md` schema at startup. It does not migrate
task documents. Older definitions without the required revision and destination
object are disabled; delete and recreate them through the management tools.

Due tasks use the same serial worker as ordinary conversations. A busy worker can
delay a run; it does not start overlapping executions or replay every elapsed
interval. The oldest due task runs first. Startup skips overdue runs and marks
previously running work interrupted.
Claims are synced before model work or delivery; interrupted or uncertain work is
never automatically replayed. Removing the authorizing admin revokes their schedules
when next checked. A current admin can explicitly resume or update a revoked schedule
to approve a new revision. Re-adding the former admin alone does not reactivate it.
Deletion revokes a claimed task that has not started; completed effects remain.

Each execution starts a fresh model context containing the saved task, destination,
and up to three confirmed prior outputs, each limited to 500 characters. The model
can search for `read_destination` to read recent room or thread messages when needed.
It can use ordinary memory and configured external tools, but has
no schedule management, app lifecycle, delegation, reaction, or progress tools.
Destination access is checked before model work and again before delivery. The host
posts the final answer only to the saved destination; `NO_UPDATE` suppresses an empty
report. Each run is limited to five minutes, 30 tool calls, and 40 model steps,
or lower configured conversation limits. Failures still leave any earlier tool
effects in place. Subsequent scheduled occurrences are independent executions.

The TUI keeps a separate “Scheduled:” session per schedule with retained turn
inspection; scheduled history never becomes a normal conversation's model context.
Deleting a schedule keeps its inspector history. Definitions, task text, tool
receipts, and thread context are private runtime data. The configured model
provider receives execution context, and configured MCP servers receive requested
tool arguments and the bot's outbound IP, as with interactive turns. No new
external service is required for scheduling.

## Web access and MCP

Each turn includes a local `tool_search` capability and a reminder to search
before work that needs tools or a claim that a capability is unavailable.
Search matches names and descriptions by keyword. A nonempty query exposes up to
eight matching tool schemas for the rest of the turn. An empty query lists concise
entries without exposing all schemas; search specific names next. Initial requests
include only core conversation tools and search. Schema exposure resets each turn
and does not change execution authority. Results include configured MCP capabilities and their discovery
route. Search does not contact external services or grant permissions.
The catalog is rebuilt after host permission checks on each turn. Input that is
not entirely owner-authored excludes app lifecycle tools; a later owner-only
input can expose them again. Completion notifications retain their restricted
reporting and recovery tools. Simple conversation, reactions, and silence do not
need a search. These instructions guide the model; host checks enforce access.

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
The host refreshes triggering authors' profiles each turn, but sends only changed
profiles. Failed lookups clear stale names. Authority metadata is always supplied.
The internal prompt directs the model to use verified `@usernames` in replies,
keep raw user IDs internal, and avoid inventing handles when none are available.
This does not add a user-profile lookup tool.

### Conversation journals and context rollover

Gobotto maintains the protected `brains/conversations/` means collection. Each
room and thread has a separate document. Thread turns update only that thread's
journal. A triggering top-level user message is also recorded in its room journal,
but Gobotto's thread reply is not copied there. Scheduled top-level posts update
the destination room journal. Unaddressed room traffic is not recorded.

The host checks current Chatto access before journal reads. `conversation_recall`
can read or search only the current scope: there is no room, thread, path,
or collection selector. Thread recall does not read its room or sibling threads.
Configured admins do not bypass this boundary. Generic memory tools, app lifecycle
tools, and fixed collection aliases cannot access the collection. The collection
is locked to one server/bot identity and one running process.

After a turn, an isolated tool-free model request writes a short note about useful
facts, decisions, corrections, or explicit outstanding requests. The request keeps
each message's author and identifies the bot separately; mentions identify
addressees, not authors. Gobotto appends the note as a Markdown bullet with
`means append`. Greetings, repetition, skipped reactions, and casual offers need
no note. An empty notes array still checkpoints the turn, so filler cannot block
the next useful entry. Notes cover useful changes across the supplied evidence;
there is no required one-to-one match between notes and turns. The document body is one notebook;
there is no separate running summary or JSON event log. Raw tool receipts stay in
the existing private session records. Notes never grant authority, prove delivery,
or authorize replay of an old action.

Note requests use the conversation model, appear as **Journal note** in the
inspector, and count toward reported usage. Each changed journal needs at most one
request per update; a top-level request can update both room and thread journals.
These auxiliary requests are separate from the action loop's tool and step budgets.
Requests have a 30-second timeout and at most 4,096 output tokens, or the lower
configured output limit. Each note is limited to 600 bytes and an appended batch
to 2,400 bytes. Routine notes retain the request topic and outcome, not detailed
weather reports or illustration descriptions. Fenced code and artifact payloads
are omitted from note input; prose and tool excerpts are capped at 2,000 characters
each with explicit omission markers. The original session text is unchanged.
Only when the notebook
approaches 16,000 bytes does the model condense it to at most 8,000 bytes, using
`means update`. A malformed or failed note leaves the original transcript intact.

At a turn boundary, Gobotto can replace a transcript of at least 256 KiB with the
scoped notebook and the most recent complete turn, including exact tool exchanges.
Current system instructions and permissions are supplied again. This byte limit
is a conservative trigger, not a model-specific token count. Rollover requires a
matching durable journal checkpoint, no active child work, no pending delivery,
and no recorded uncertain delivery or failed effectful turn. Recall returns notes,
not old tool receipts; verify old outcomes with current tools when necessary.
Work already performed is never replayed by the host. Child investigations keep
their existing context-limit behavior.

Journal failures, oversized entries, or a recent turn that is too large prevent
rollover. The existing 1 MiB transcript guard still applies and can require a new
thread. Transient turn evidence is limited to 64 KiB per turn and model input to
128 KiB. Pending evidence stays in memory; the original session transcript supports
recovery after restart. A retry replaces the previous pending transcript snapshot
instead of accumulating overlapping copies, while preserving uncertain effects.
The inspector distinguishes model failures, timeouts, invalid JSON, missing fields,
note limits, stale checkpoints, and storage failures without logging response text.
Frontmatter keeps a fixed-size latest checkpoint and an
uncertainty flag. Hash checks and a temporary mutation marker make interrupted
appends recoverable without adding the note twice. Failed means commands block
journal writes until restart reconciles that marker. Recall returns the bounded
Markdown body, or matching lines for a search query.

Access or history invalidations invalidate the affected room's journals, including
thread journals outside the loaded session cache. Generation checks prevent a
concurrent old turn or summary from restoring them. A restored session rechecks
access and discards context whose journal generation changed. Offline history
changes detected by the existing thread refresh also invalidate journal context.
As with the existing transcript, offline edits outside the fetched history window
cannot all be verified from a bounded refresh.

Journal files are private local data. The configured model provider receives the
notebook and supplied turn excerpts; no additional service is contacted. Journal
storage requires a means CLI with `append` and uses an isolated configuration with
embeddings disabled. The documents use Markdown and JSON frontmatter. Old journals
remain readable and change to the notebook format on their next successful update.
Retained transcripts contribute scoped excerpts within the same bounds.

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

### Memory and apps

Gobotto has a built-in `memory` brain for general knowledge and knowledge about
individual Chatto users, plus protected `schedules` and `conversations` brains for
recurring tasks and scoped journals. Dynamic brains are **apps**, such as a task tracker or
project notebook. App formatting, workflow, and default rules belong in that
app's instructions. They do not belong in personal memory.

The storage directory defaults to `./brains`, relative to `gobotto.toml`:

```toml
[memory]
directory = "./brains"
```

Gobotto initializes built-in memory in `memory/` and discovers dynamic apps under
`apps/` within this directory. For example, the default paths are
`brains/memory/` and `brains/apps/chatto_todos/`. Its built-in memory
schema and guidance are defined in code. Documents have a `kind` of `general_fact`
or `user_fact`, a discovery `summary`, and a Markdown body for details. User facts
also require the exact Chatto `user_id`. The name `memory` is reserved. Agents
can write knowledge documents, but cannot archive this brain or change its
definition. Startup rejects a modified definition instead of replacing it.

When a thread is first loaded or refreshed, Gobotto supplies a knowledge index
with document IDs, titles, and summaries: up to 20 general facts and 20 facts
for each of up to eight distinct participants. The index is limited to 16 KiB
and reports omitted entries and document diagnostics. It contains no document
bodies. Ordinary turns retain the snapshot; the agent can search for newer or
omitted knowledge through the memory tools. The index goes to the model provider
and remains in the private session transcript. Per-user attribution is not a
privacy boundary; this is shared memory.

Existing knowledge collections are not migrated or deleted automatically. Move
their relevant facts into built-in memory explicitly if they should appear in
the index. Other existing collections remain app brains. A collection named
`memory` that is not the built-in brain must be renamed before startup.
For the previous layout, move `brains/.memory/` to `brains/memory/` and move
dynamic app directories into `brains/apps/`. Startup rejects the old layout
instead of creating empty replacement memory or ignoring old apps. Tool names
remain unchanged; catalog entries distinguish `kind: memory` from `kind: app`.

Owners can ask Gobotto to create an app, for example: “Create a project notebook
that anyone can read and contribute to.” The `memory_brain_create` tool accepts a
name, title, description, JSON Schema, and Markdown instructions. Gobotto builds
the `MEANS.md` frontmatter; means validates the schema. Invalid arguments can be
corrected in the same turn when the tool reports `retryable: true` and
`mutated: false`. After a CLI failure, Gobotto permits correction only if it can
remove the empty reservation directory, proving no collection files were created.
Partial writes remain uncertain and block retries.
`memory_brain_archive` moves an app into `apps/.archive/` inside the brains directory;
it removes the brain from discovery but retains its files for manual recovery.
Only owner-only triggering batches receive these lifecycle tools. Each brain can
have one successful lifecycle change per turn. Uncertain mutations are not retried.

Owners can ask Gobotto to change an app brain's schema, for example: “Add
HIGHEST to the allowed priorities.” `memory_brain_schema_set` installs a complete
replacement schema after a `memory_info` read in the same turn. This requires a
means version with `schema set` support (verified with 0.4.2). Means validates the
schema and existing documents before installation; Gobotto does not expose the
force option. Documents, title, description, and Markdown guidance are preserved.
Existing document values are not migrated. Fixed collection schemas cannot be
changed through this tool. After success, Gobotto must read `memory_info` again
before document operations. The current schema defines allowed values if the
unchanged guidance still lists older values; audience rules remain in effect.
CLI failures block further writes in that turn because their outcome can be
uncertain. Inspect the current schema before a new request.

Owners can change an app's behavior with `memory_brain_instructions_set`, for
example: “Always show these tasks as a table with priority and who added them.”
The tool requires the definition hash from `memory_info` in the current turn and
a complete, nonempty Markdown body. Gobotto calls `means instructions set` with
JSON on stdin. Means rejects stale hashes and preserves frontmatter, permissions,
and documents. Gobotto requires another `memory_info` read after success. The
same owner-only, fixed-binding, built-in memory, and uncertain-write restrictions
apply. Use a means binary with `instructions set` and a hash in `info --json`;
0.4.2 does not provide this command. Check which binary is first on `PATH` if
multiple versions are installed.

Gobotto discovers immediate child directories of `apps/` with a regular `MEANS.md`, excluding
hidden directories and symlinks. Names follow the same rules as fixed bindings.
The catalog is checked each turn and supplied when it changes, or when a session
starts or is restored. Creation and archival become visible without a restart. Full guidance
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
changes. The `means` executable must be on `PATH`; built-in memory requires it
even when no app collections are configured.

Collection metadata comes from `MEANS.md` through `means info`. The catalog reads
each collection's current title and description. The collection-info tool returns
its complete Markdown instructions and schema as well. Keep this content in the
collection; `gobotto.toml` contains only the binding name, path, and write access.

Fixed bindings cannot be archived through tools. `writable` defaults to `false`
and remains a hard write limit for these bindings. Within that limit, their
Markdown guidance governs document use, just as for managed brains. Fixed and
managed collection names must not collide.

The agent can discover collections, inspect their schemas and instructions, list, search,
and read documents. Authorized turns can create and update documents. Before a
write, the agent must inspect the collection. Updates require the hash from a
document read in the same turn; means rejects conflicting changes. Update bodies
replace the full body, while application data follows means' field-patch rules.
The agent cannot supply collection paths, delete individual documents, or run
shell commands. App creation, schema changes, and archival are limited to the
managed directory. App instruction updates use the same boundary.

`memory_list` enumerates documents in filename order. It requires a means version
with `list --offset` support. Start with `{"collection":"steam_deals"}`; the
default offset is 0 and the default limit is 20. Set `limit` to 1–50 to change
the page size; unbounded listing is not exposed. Results include document
metadata, diagnostics, `has_more`, and `next_offset` when more records remain.
Pass that offset with the same collection and filters until `has_more` is false.
Listing accepts the data and datetime filters described below, but no text query.
Read `memory_info` in the current turn first; listing does not replace
`memory_read` or its hash requirement for updates.

Pages read current files, not a snapshot. Inserts, deletes, renames, and changes
to filter fields can shift later pages. Finish listing before bulk edits. If the
collection changes during traversal, restart and deduplicate by document ID;
do not treat the result as a complete snapshot. Diagnostics can report invalid
or omitted documents even on the final page. Each request runs a local means
scan; offset pagination bounds model output, not the cost of scanning files.

Search supports full-text queries and structured filters, and returns up to 20
documents. `memory_search` accepts an optional `filters` object keyed by fields
directly under document `data`. All conditions use AND; an empty `query` searches
by filters alone. JSON value types are preserved. Object filters match supplied
keys recursively, arrays match exactly, and absent fields do not match.

For example, recall for one user uses
`{"collection":"memory","query":"","filters":{"kind":"user_fact","user_id":"<user ID>"}}`.
An app query can use `{"status":"open","priority":"highest"}` as its filters.
Text queries require all words to match; structured field values do not need to
appear in document titles or bodies.

Optional `after`, `before`, `on_or_after`, and `on_or_before` objects map data
fields to RFC 3339 timestamps with timezones. The first two comparisons are
exclusive; the latter two are inclusive. They combine with text and data filters
using AND. Date-only values, such as a task's `added` date, are not datetime
filters; use an equality filter for an exact date. Inspect `has_more` before
treating a result as complete. Filtering uses the local means CLI and does not
enable semantic search or contact an embedding provider.

Each command has a
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
are saved in snapshots and shown in the session list. The session summary keeps the
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

The overview shows a session list and a runtime dashboard. Six aligned metrics
span wide terminals and wrap into two rows on narrow terminals. Muted labels
and bold values separate counts from their explanations. The dashboard shows process
uptime, active and queued sessions, pending messages, and totals for retained
sessions, turns, tokens, and cost. These totals include restored session history;
they are not limited to the current process run. The overview has no inspector.

Press Enter on a session to open its session view. Clicking a session only
selects it; Right does not open it. Wide session rows include an aligned turn
count column. The session view has a summary dashboard above one full-width
turn table. It reuses the overview's metric layout for session state, turns,
pending messages, input and output tokens, and reported cost.
Opening a stored session loads its newest 20 retained turns asynchronously.
The table shows a loading indicator, or a safe error with `r` to retry. Use `p`
to load the next older page. Live turn updates take precedence over older disk
results. Only one inspection read runs at a time; moving to another session
does not let an earlier response change your selection.
All turns start expanded on each visit, and new turns appear expanded. Manual
fold choices survive updates during the visit. Only turns with child rows show
an expansion caret. Input messages, execution steps, outputs, reactions, and
failures appear as children of their turn. Child operations appear as named steps.
Sessions and turns appear newest first; steps stay in execution order.
Escape returns from the turn tree to the overview and restores its selection.

The session list places a busy spinner before the title, with two blank cells
in that position when idle. It has no status column. The session table has a fixed
header with Type, Description / metadata, Runtime, Status, Tokens, and Cost.
All event rows use the same columns; unavailable values stay blank. The type
column retains the turn tree and tool names. Inputs use blue, outputs use green,
and failures use red. Narrow terminals omit runtime, then usage, then status
columns to keep descriptions readable. Step descriptions include provider, request ID,
tool details, cache and reasoning counts when available. Model rows put input,
output, cache, and reasoning counts before request IDs. Turn totals include
child and title calls. A `+` marks
an incomplete total; `—` means unknown. A completed model step does not mean
that a message was delivered. Turns show `Replied` or `Reacted` only after the
host confirms delivery.

Busy sessions show a small animated Braille spinner before their title; running
turns show it beside their status.
Active states use teal, completed operations use green, queued or partial states
use amber, and failed or uncertain operations use red. Session names use blue
and turn names use lavender.
The `›` and `⌄` chevrons indicate collapsed and expanded nodes.
Long descriptions end with an ellipsis and do not wrap. Use `[` and `]` to move
through description text while event types and telemetry stay fixed; `c` copies
the selected event's full recorded text.
Click a row to select it, or a turn's chevron to fold it. The mouse wheel
navigates the table. `l` switches the same panel to session operational logs.
These logs lack turn associations and are not assigned to turns by the UI.

| Key                | Action                                                 |
| ------------------ | ------------------------------------------------------ |
| Up/Down or `k`/`j` | Select a visible row                                   |
| Right / Left       | Open or enter a node / close it or select its parent   |
| Space              | Open or close the selected node                        |
| Enter              | Open a session, fold a turn, or inspect an event       |
| `i`                | Inspect the selected session, turn, or event           |
| Escape             | Return to the overview                                 |
| `[` / `]`          | Pan event text left / right                            |
| `l`                | Switch between the turn table and operational logs     |
| `f`                | Toggle following new session, turn, and step activity  |
| `c`                | Copy the selected event, turn response, or session ID  |
| `y`                | Copy the selected turn's input messages                |
| `p` / `r`          | Load older turns / retry or refresh stored history     |
| Page Up/Page Down  | Move one page in the focused view                      |
| Home / End         | Move to the start / end of the focused view            |
| `q` or Ctrl-C      | Finish active work and report child results, then quit |
| Shift-Q            | Cancel active work and quit immediately                |

The private detail view supports Tab/Shift-Tab to switch sections, arrow keys or
the mouse wheel to scroll, Page Up/Down and Home/End to navigate, `c` to copy the
current section, and Escape to return. It shows a fixed snapshot; close and reopen
it to refresh running work. A model completion has sections for its messages
(including system instructions and injected JSON context), available tool schemas,
request settings, and returned message. Other events show their recorded details.
The completion details include a byte breakdown of compact inspection JSON:
system instructions, tool schemas, latest input, other history, tool results, and
settings/framing. These are not token counts or wire sizes; images and provider
reasoning are excluded. The breakdown needs a complete live request capture.
Older records explicitly show when a request was not captured.

New ordinary and scheduled model completions capture the request after provider
tools and per-call controls are applied. These snapshots are live-only: they are
not written to disk and never enter model context or operational logs. Restarting
or reloading history loses them; the existing transcript and event records remain.
Image URLs/pixels and
provider reasoning text are omitted; reasoning configuration remains visible.
Capture is limited to 512 KiB per request/response and 4 MiB per turn, with explicit
truncation markers. Older live captures are discarded when their total exceeds
4 MiB per session, while their event records remain. Auxiliary title requests and delegated child requests are not
captured by this view.

The view resizes with the terminal and needs at least 36 columns and 12 rows.
Normal quit keeps the UI open with a waiting message while active turns and
children finish. Completion reports can run during shutdown; queued user turns
do not start. Queued messages remain in the
persistent inbox for restart. Press Shift-Q to cancel instead; Gobotto still
closes connections and saves session state before it exits.
It retains up to 100 sessions and 200 operational log entries per session.
The UI caches inspection data for three sessions. Reopening an uncached session
reads its records again; the overview keeps only its summary.
During startup, selection stays on the first, newest session as the list fills.
After you navigate or open a session, new sessions and updates preserve your
selection. Follow mode selects incoming
session activity in the overview. In a session view, it follows only that
session's turns and steps. It does not switch sessions. Manual navigation stops
following. If retention removes a selected turn, selection moves to a retained
row. If the open session is evicted, the UI returns to the overview.
Logs and inspection records survive restarts. Each session keeps up to 50 turn
records, trimming older records when their encoded size exceeds 2 MiB; the newest
turn is always kept. Inputs are bounded to 10,000 characters per message, and tool
details to 4,000 characters. Truncation remains marked in the recorded text.
The session dashboard shows cumulative tokens and OpenRouter account
cost. Cached input is part of input, not an additional count. Cost comes from
the response's `usage.cost` field. Missing values show as `n/a`; a cost total
with some missing charges shows as `partial`. Totals cover reported requests only.
Logs show turn starts, reply/reaction delivery, omitted history, and failures.
They do not contain message bodies, user profiles, credentials, or raw model
output. The TUI adds no external connections.

The private turn table shows triggering messages, execution steps, and outputs
with their delivery status. Title-generation usage appears as its own step.
Each model step shows the serving provider when reported. Event text includes
the OpenRouter request ID for comparison with its activity page. Missing cache
counts show as `cache n/a`; incomplete totals show as `partial`. Zero cache counts
in older snapshots also show as `n/a`, because those snapshots did not distinguish
a reported zero from an absent count.
Running operations update their elapsed time. Model reasoning and credentials
are not shown. An uncertain delivery remains explicitly marked
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
- Each turn resolves the triggering authors' public usernames and display names.
  Initial thread loads and refreshes after restart also resolve authors from the loaded messages.
  These profiles are separate from message text and do not change ownership or
  message-edit fingerprints. Lookup failures leave names unavailable and do not
  stop the reply. The prompt requires verified `@username` handles for mentions;
  display names help identify people but do not prove an external identity.
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
- The model can call `stay_silent` with `{}` to end a turn without a reply or
  reaction. The inspector records `silent`, and the transcript keeps a tool
  receipt. This is distinct from waiting for background work or a failed request.
  Earlier tool effects and progress messages remain in effect. Written text
  takes precedence if the model incorrectly combines it with a silence call.
  Participation guidance favors silence for conversation between other people
  and respects requests to speak less until invited back. Mentions, direct
  questions, and valid moves in an activity can still need a response.
  This decision uses a model request; it does not reduce inference costs or
  disable the existing typing indicator and first-session acknowledgment.
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
- Image attachment inspection is supported as described below. Sending
  attachments, reading other file types, and streamed answers are not implemented.

## Image inspection

Gobotto can inspect PNG, JPEG, WebP, and GIF attachments with `inspect_image`.
The conversation model and its provider must support image input. Gobotto uses
the configured conversation model; it does not select a separate vision model.
The model receives stable image references in message context and can request
pixels when an answer needs them. Attaching an image does not download it by itself.

Each inspection checks that the attachment still belongs to an accessible message
in the current thread page. Older messages outside that page cannot be inspected.
The SDK obtains fresh access from Chatto and downloads from the Chatto server
origin without forwarding bearer credentials or following redirects. Download
URLs do not reach the model or session storage. No arbitrary URLs are accepted.

Downloads are limited to 4 MiB per image and decoded images to 16 million pixels.
Gobotto converts the first frame to JPEG, removes file metadata, and resizes to
at most 1600 pixels per side. A turn can inspect up to four images, each with at
most 2 MiB of normalized data. Small text and animation details can be lost.
Unsupported, removed, oversized, or unreadable attachments return an explicit
tool rejection. A provider that does not accept images can fail the model request.

Normalized pixels go to OpenRouter and the selected model provider as base64
image input, using the existing model connection and its outbound IP. Pixels
are present for the rest of that turn, including later model steps, and can add
usage charges. Signed URLs and pixels are not saved in Gobotto's transcript or
inspection log. Image references, tool receipts, and the model's descriptions
are saved. On a later turn, the model must inspect again to see the pixels.
Attachment changes participate in edit invalidation; URL refreshes do not.
See [OpenRouter image input documentation](https://openrouter.ai/docs/guides/overview/multimodal/image-understanding).

## Session storage

Gobotto writes one versioned JSON snapshot per conversation under
`.gobotto/sessions/`, relative to the working directory. Set `GOBOTTO_SESSION_DIR`
to use another location. The default directory is ignored by Git. Files contain
private conversation text and model responses, but no API keys. Directories use
owner-only access and snapshots use mode `0600`. Storage is not encrypted.

Each session has a readable ID, such as `calm-otter-012345abcdef`. The session
list shows this ID when no title is available. The session summary title
always shows the ID. Its snapshot is stored at
`.gobotto/sessions/<namespace>/v2/<human-id>.json`; the `HumanID` field identifies
the session and `History.messages` contains its model transcript for evaluation.
IDs remain stable across restarts and context resets. The indexed storage
generation starts fresh in `v2/`. Existing files and the old realtime inbox in
the parent directory remain unchanged and are not loaded or replayed. There is
no automatic migration. The namespace lock still prevents an older process
from using that namespace at the same time.
Click the session ID in the summary title, or select its session and press `c`,
to copy it to the system clipboard.
The footer confirms the copy or reports that the clipboard is unavailable.

Snapshots include model history, message IDs, turn counts, reported token and cost
totals, and the OpenRouter session ID. Server URL, bot identity, and model select
separate storage namespaces. A process lock prevents concurrent writers. Writes
replace snapshots atomically. Each file starts with a compact `Index`, followed
by `Records` and `Logs`, before the model transcript. The overview reads only
the index prefix. Inspection pages read only records and logs. These fields
share one atomic replacement, so the summary cannot refer to a different
snapshot generation. Invalid index data stops startup; invalid transcript data
fails when that conversation is loaded. Neither case overwrites the bad file.
A process identifier distinguishes live checkpoints from unfinished records
left by an earlier process. Loading a page never resumes interrupted work.

The runtime loads a full transcript only when its thread receives work. It
retains at most eight idle hydrated conversations. Active turns, queued input,
and pending child results can keep additional conversations loaded until they
are safe to unload. Unloading writes a checkpoint and keeps the session in the
overview; it does not remove its file. The existing 100-session retention limit
still applies. Loading after eviction rechecks thread access and history.

Gobotto saves loaded conversations before a turn, after reported usage, before
delivery, after a turn, and on clean shutdown. Unopened histories are not rewritten.
An interrupted turn is never retried automatically; an
interrupted delivery is marked uncertain. Turn inspection records and operational
logs are stored alongside the transcript. The `v2/` directory's private `.realtime`
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
other participants and their Chatto user and message IDs. Participant usernames
and display names are also sent to the model provider and retained in private
session transcripts, but are excluded from operational logs. Profile lookups
contact the configured Chatto server only. Do not use private
conversations unless participants agree to this use.
Gobotto supplies an explicit message projection to the model. Supported image
asset IDs and content types are included; filenames, signed asset URLs, and
creation timestamps are excluded from this projection and the durable intake
inbox. Edit checks compare text, identity, and image references, so display
metadata changes do not reset model context. URLs
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
`internal/tui/` owns the Bubble Tea model, visible tree rows, Bubbles spinner,
and shared Lip Gloss metric and table layouts. The worker sends operational events to the UI; only the UI goroutine
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
