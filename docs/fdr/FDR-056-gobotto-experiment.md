# FDR-056: Gobotto Experiment

**Status:** Experimental
**Last reviewed:** 2026-10-30

## Overview

Gobotto is a Go bot prototype under `experimental/gobotto/`. It explores a Go
client for Chatto and an agent loop that uses OpenRouter. It is separate from
ChattoBot and does not replace the supported TypeScript client.

## Behavior

- The bot connects with a bot API key. A server-verified direct, role, or broadcast
  mention that includes the bot starts a session. DMs and replies in DM threads
  also start sessions without a mention. Ordinary channel replies require a
  mention to open a session. Self-messages and channel echoes are ignored.
- When a new session opens, the host attempts an `eyes` reaction on the opening
  message before the debounce completes. This best-effort acknowledgement runs
  independently of the model and does not block intake or replies. The session
  is saved before the attempt. Failed attempts are not retried; restored
  sessions and later messages do not repeat the acknowledgement.
- A session belongs to one Chatto thread and retains its model transcript across
  turns. Its first turn reads recent thread context. Later turns append realtime
  input. Every new message in an existing session's thread starts or resets the
  debounce, even without a mention or after restoring the session. A top-level
  message, including a DM, is its own thread root.
- A triggering message starts a configurable debounce, defaulting to four seconds.
  `conversation.debounce` accepts a positive duration such as `4s` or `750ms`.
  Each later message
  resets the timer. Busy-time windows elapse during the active
  turn. One model turn runs at a time, so execution can wait longer under load.
- The model replies in the same thread or reacts to the latest triggering message
  in the batch. Its exact assistant/tool transcript and delivery receipts remain
  available on later turns. Provider session IDs remain stable for ordinary turns.
- During context reads, model calls, and tool work, the host refreshes the active
  thread's typing indicator every two seconds. Refreshes run independently of
  model work and stop before delivery or when the turn fails or is cancelled.
  Typing errors do not fail turns. Debounce waits and title generation do not
  show typing. Chatto clears indicators on a matching message or after six
  seconds without a refresh; there is no explicit stop request.
- Operator configuration can expose allowlisted tools from Streamable HTTP MCP
  servers. The Exa configuration enables keyless search and page fetching; an
  optional environment-sourced API-key header enables authenticated use.
  Tools are namespaced by server, sorted, and discovered once at startup.
  The model receives a compact name list plus discovery and call functions.
  Full remote schemas enter history on demand and are reused on later turns.
  First use requires an additional model step; unused schemas consume no
  conversation history. Chatto and memory tools stay directly available.
  Reconnects verify the same definitions and never replay failed calls.
  Results are bounded, marked as untrusted, and included in normal tool records.
  Remote errors are reported as failed tool events. All participants can use
  configured tools. Remote prompts, OAuth, subprocess servers, and interactive
  requests are outside this implementation. MCP servers receive tool arguments
  and the bot's outbound IP; results enter model context and persisted sessions.
- Turns run in batch-deadline order and have time and call limits. A failed turn
  does not stop intake. Realtime disconnects reconnect with backoff. An atomic
  inbox stores accepted messages and their replay cursor together. Pending batches
  survive restart and wait for replay to finish before starting a turn. Heartbeats
  refresh the cursor. Started batches are never automatically retried after a
  crash because delivery may already have occurred. If the cursor cannot resume
  (including its 15-minute expiry), the UI reports a gap and retained threads
  refresh before reuse. Messages missed beyond replay do not trigger new turns.
- The terminal has two stacked panes: a selectable session list above the
  selected session's operational log or turn inspector. The inspector divides
  the lower area into a mouse-selectable turn list and stats/log panes beside it.
  The inspector is the default view. Keyboard shortcuts `l` and `i` switch to
  the operational log and back; there is no mode toolbar.
  Operators can switch panes, scroll independently, and follow new
  logs. The selected row has a full-width background. Mouse clicks select sessions
  and focus panes; the wheel navigates the pane under the pointer. Operators can
  drag the divider to resize the panes. New sessions preserve the current selection. Up to 100 sessions and
  200 operational log entries per session are retained and persisted. Logs omit private conversation
  content. Each row groups turns for a thread. The log pane title shows cumulative
  reported tokens and account cost. Routine request and usage events do not add
  log entries. Missing cost is shown as unknown or partial, not zero.
  Normal quit waits for the active turn and displays a waiting message. No new
  turns start during shutdown; pending inbox messages remain for restart.
  Shift-Q cancels active work immediately. Both paths close connections and
  save session state before returning.
- History or access invalidations cancel affected work and discard its context.
  Message edits and retractions retain their target IDs and event reasons through
  the Go SDK. Gobotto matches them against conversation roots and retained, queued,
  or active message IDs. Unrelated changes in the same room do not cancel model
  requests. Missing targets and context reads in progress remain conservative.
  Cancellation causes appear in the turn inspector and session log.
  The next message in that thread loads a fresh authorized snapshot. Retention and
  input buffers are bounded; automatic summarization remains open work.
  Cache hits depend on the provider, not just the bot.
- Versioned, atomic JSON snapshots retain each session's model history, message
  IDs, turn counts, usage totals, and provider session ID. Storage is local,
  owner-readable, and separate for each server, bot identity, and model. A process
  lock prevents concurrent writers. Startup loads sessions without replaying
  actions from started batches. Interrupted delivery is uncertain. The durable
  inbox restores pending input and deduplicates recently completed events.
  Before a resumed turn, a thread read checks access and refreshes context;
  changed or missing known messages reset model history. Corrupt snapshots stop
  startup without replacement. Eviction removes the snapshot.
- Structured private turn records persist separately from operational logs. They
  contain input batches, model and tool steps, durations, outputs, delivery state,
  and reported usage. They omit model reasoning. Up to 50 records are retained,
  trimming older records above 2 MiB while keeping the newest turn. The inspector
  supports expansion, copying inputs/responses, and diagnostics for extra reads,
  multi-call reactions, and cache-hit drops. Old snapshots gain records on their
  next turn. Interrupted records never cause action replay.
- Each session has a persistent readable ID shared by its TUI labels, snapshot
  filename, and `HumanID` field. This lets operators find the model transcript
  for evaluation. Context resets retain the ID; older snapshots acquire one on load.
- Gobotto reads its authenticated profile from `ViewerService/GetViewer` at
  startup. Model instructions identify the bot by that account's ID, login, and
  display name, including after restoring a session. Profile text is data, not
  instructions. Operational logs omit it. Restart refreshes profile changes.
- `gobotto.toml` configures `conversation.model` and `conversation.system_prompt`.
  The model ID is required; an empty operator prompt is allowed. Credentials stay
  in environment variables. `GOBOTTO_CONFIG` can select another file. Invalid or
  unknown settings stop startup. Embedded internal prompts define Chatto behavior,
  identity, tool use, and delivery rules; operator instructions add preferences.
  Restart applies current prompts to new and restored sessions. A model change
  selects its own existing storage namespace. `OPENROUTER_MODEL` is no longer read.
- Optional `[titles]` settings select a separate model and title preferences.
  After a successful delivery, an untitled session makes one tool-free
  title request. Titles persist and replace IDs in list labels; IDs remain
  copyable in the lower pane and continue to identify files. Title usage is
  included in session totals. Failure keeps the ID label and is not retried.
  A title request receives message excerpts, not system prompts or reasoning.
  It sends no output-token cap; provider defaults apply. Displayed titles remain
  bounded. Each conversation batch also carries a current UTC timestamp, without
  changing the stable system prefix. User-facing references use verified handles
  when available; raw user IDs remain internal. Host tool rejections are recorded
  as rejected, distinct from execution failures and successful completion.
- The model provider receives the bot's profile, triggering text, and recent thread
  context, including messages from other participants. Operators must obtain
  participant agreement before using private conversations.

## Design Decisions

1. **Keep the prototype in its own Go module.**
   **Why:** It can evolve without changing server dependencies or ChattoBot.
   Chatto connection code lives in a separate `experimental/chatto-go/` module
   so it can become an official Go SDK. Gobotto depends only on its public API.
   **Tradeoff:** Both modules need checks outside the root Go workspace.
2. **Use server-verified realtime mentions.**
   **Why:** Ping routing follows Chatto's resolved recipients, including role
   and broadcast recipients, without guessing from message text.
   **Tradeoff:** The experiment needs a protobuf WebSocket client and a durable
   inbox. Recovery is bounded by the server's cursor lifetime; it cannot guarantee
   a response to every offline message or safely repeat an interrupted delivery.
3. **Bind actions to the ping and read context first.**
   **Why:** The model has thread context before it chooses a reply or reaction.
   It cannot redirect that output to another conversation.
   **Tradeoff:** The bot cannot perform broader agent tasks yet. Long threads
   provide only the root and recent replies.
4. **Use Bubble Tea for the terminal interface.**
   **Why:** Operators can inspect one session while other activity arrives.
   One UI event loop owns selection, layout, and bounded session history.
   **Tradeoff:** Gobotto requires an interactive terminal. Operational logs and
   private inspection records are separate persisted views of execution.
5. **Retain an append-only conversation between ordinary turns.**
   **Why:** Stable prefixes can reuse provider caches, and new input does not
   require fetching the same Chatto history again. Two-second batches combine
   rapid messages into one response opportunity.
   **Tradeoff:** Transcripts consume memory and still travel with each completion
   request. History invalidations and eviction deliberately discard cached state.

## Permissions and Configuration

Conversation completions have an optional output budget and optional
OpenRouter reasoning controls. Omitted controls preserve provider defaults.
Gobotto sends no output cap unless the operator configures one.
Reasoning effort and explicit reasoning budgets are alternatives; the latter
must leave room in any configured total output budget. Reported reasoning tokens are stored
and displayed as a subset of output. Absent counts remain unknown, including in
legacy snapshots; mixed reporting is marked partial. Output-limit stops never
execute partial tool calls and have a specific inspector diagnostic.

The top-level `owner_user_ids` configuration lists exact, case-sensitive Chatto
user IDs. No configured IDs means no owners. Owner recognition uses the
server-supplied message author, not names or conversation text. Each triggering
message carries its own author and owner status; mixed-author batches and shared
threads never inherit ownership from one participant. On restart, the current
owner list replaces the system instructions in restored sessions before use.
Historical labels do not grant authority. Ordinary conversation remains available
to non-owners. Privileged tools must enforce requester identity in host code.

Configured `[[memory.collections]]` entries bind existing means collections by
name and path, with an optional `writable` flag. Titles, descriptions, and complete
Markdown instructions come from each collection's `MEANS.md` through `means info`.
The catalog exposes titles and descriptions; the info tool also exposes the
Markdown instructions and schema. Fixed bindings have a hard `writable` limit,
disabled by default. Document tools are available to all participants; the agent
applies each collection's prompt-defined audience and workflow to the trusted
author metadata. These rules are behavioral guidance, not data isolation.

The optional `memory.directory` contains managed brains. Immediate child
collections are discovered on demand; hidden directories and symlinks are excluded.
Every turn includes a fresh catalog, including new and restored sessions. Owners
can request creation with structured title, description, schema, and Markdown
instructions, or archival into `.archive/`. Go generates the means frontmatter.
Validation failures allow corrected input in the same turn. After a CLI failure,
correction is permitted only when the empty reservation can be removed, proving
that no collection files were created. Partial writes remain uncertain.
Only entirely owner-authored batches receive lifecycle tools. Fixed bindings cannot
be archived. Creation never overwrites an existing directory, and archival retains
files for manual recovery. Prompt text cannot change lifecycle authorization.

Means runs as a bounded JSON subprocess without a shell. Tools expose collection
discovery, schema inspection, lexical search, document reads, creation, and
hash-checked updates. Paths come only from fixed bindings or validated child names
under the configured directory. Agents cannot modify existing schemas or delete
individual documents. A guidance read precedes document access;
updates require a document hash read in the current turn. Successful write receipts
remain in the transcript and are saved before another model call. An uncertain
write blocks further writes in that turn. These rules limit duplicate mutations;
they do not make means writes and Chatto output one transaction.

Retrieved content is sent to OpenRouter and retained in private transcripts.
Removing a binding does not erase previous context. Means can additionally send
content to an embedding provider if the operator enables embeddings in means.
Preconfigured subagents remain outside this feature.

The bot needs a Chatto API key, room read access, and permission to post thread
replies and add reactions. Verified pings and DMs control intake. OpenRouter credentials
and an explicit model ID are required. See the
[setup guide](../../experimental/gobotto/README.md).

## Open Questions

- Should cursor expiry trigger a bounded history scan for missed messages?
- How should long conversations be summarized before their context budget is reached?
- When should the Go client become a separately supported SDK?
- Which additional tools should the experiment support?
