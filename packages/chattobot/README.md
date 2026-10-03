# ChattoBot

ChattoBot is a Runling project. This first version sends brief agent replies to
Chatto direct messages, mentions, and direct replies to its own messages.
It owns its realtime source configuration and Chatto transport adapter.
It is the private workspace package `@chattocorp/chattobot` under
`packages/chattobot/`. It depends on the public Runling package API and
`@chatto/client`. The client owns API requests, the realtime transport, and
the server's projection; it is the same client that the Chatto frontend uses.
It also supplies identity, addressing, thread history, conversation keys, and
acceptance tracking. ChattoBot owns Runling routing, inboxes, cancellation,
and conversation lifetime. See the
[client guide](../chatto-client/README.md).

## Run locally

Use `pnpm start` to load the bot once without automatic reload. Use `pnpm dev`
for development; it passes `--watch` to `runling serve`. Both start the realtime
source and console. Without `--watch`, restart to load code or configuration changes.

1. Run `mise setup-frontend` from the monorepo root.
2. Copy `.env.example` to `.env` in this directory. Set `CHATTO_URL` to your Chatto
   server and `CHATTO_API_KEY` to the bot's API key. The bot needs permission to
   read messages and use `message.post-in-thread` (Post in threads) in the Direct messages
   scope. The bot must also be a member of the DM room. In the current Chatto
   implementation, `message.read-interactions` checks for authorship or an
   explicit mention even in DMs; membership alone does not pass that check.
   This can suppress ordinary DM deliveries. Broad `message.read` access works
   around that server behavior, but is not a ChattoBot requirement.
3. Set `OPENROUTER_API_KEY` in `.env`; no Pi login is required. The key is
   available to all agents that use an OpenRouter model. An existing Pi login
   is also supported. The default model is
   `openrouter/google/gemma-4-26b-a4b-it`. Set `CHATTO_AGENT_MODEL` to select
   another configured model.
   To use GLM 5.3 Flash for investigation and implementation, add:

   ```dotenv
   OPENROUTER_API_KEY=your-openrouter-api-key
   CHATTO_INVESTIGATION_MODEL=openrouter/z-ai/glm-5.3-flash
   CHATTO_IMPLEMENTATION_MODEL=openrouter/z-ai/glm-5.3-flash
   ```

   Each agent also has a reasoning effort setting: `off`, `minimal`, `low`,
   `medium`, `high`, `xhigh`, or `max`. Higher effort follows instructions more
   reliably, but costs more and answers more slowly. An unknown value stops the
   bot at startup with a configuration error.

   | Setting                          | Agents                      | Default  |
   | -------------------------------- | --------------------------- | -------- |
   | `CHATTO_AGENT_THINKING`          | Supervisor and web research | `medium` |
   | `CHATTO_INVESTIGATION_THINKING`  | Source investigation        | `medium` |
   | `CHATTO_IMPLEMENTATION_THINKING` | Implementation worker       | `medium` |
   | `CHATTO_CLASSIFIER_THINKING`     | Authorization classifier    | `low`    |

   `runling serve` loads `.env` from its working directory at startup. Restart
   the bot after changes. Existing shell environment variables take precedence
   over values in `.env`. OpenRouter and its selected model provider receive
   the prompts and source excerpts sent to the model.

4. From the repository root, run:

   ```sh
   mise chattobot
   ```

   This command installs packages, builds Runling and the Chatto client
   packages, and starts the bot. Use `mise dev-chattobot` to also reload
   code and configuration when files change.

5. Disable any old webhook for this bot. ChattoBot opens an outbound WebSocket
   to the configured server. No public URL, tunnel, or webhook is needed.
6. Send the bot a DM or mention it in a channel message. Replies appear in
   the triggering message's thread in both DMs and channels. For channels,
   the bot needs permission to read the thread and `message.post-in-thread`.

The server loads `.env` from this project directory. Restart it after changing
credentials. The API key authenticates the realtime connection. Empty settings
count as unset. The bot checks its settings before it connects. If a required
setting is missing or has an invalid format, the terminal shows a `ChattoBot
configuration error` that names the setting. The bot then stays stopped until you
correct the setting and restart. Model names, the source directory, and Git refs
are checked when they are used.
The web console is available at `http://localhost:5173`.

To restrict the bot to one user, set `CHATTO_ALLOWED_USER_ID` in `.env` to that
user's exact Chatto user ID, not their display name, and restart the bot.
When unset or empty, all users can address the bot. Messages from other users
are ignored before routing, including DMs, mentions, follow-ups, and `/cancel`.
They do not start runs or trigger reactions. This filters incoming requests;
thread history loaded for an allowed request can still include other participants.

At the start of a conversation, the host reads the thread root and the newest
100 replies. On later turns, it reads only the messages that arrived after its
previous read. It does not see edits or deletions of messages that it already
read. Each message carries its author's display name and login from the thread
page, so the supervisor can tell people apart and address them. The bot reads no
other profile fields. The model provider receives these names in the prompts,
and they can appear in run journals and the detailed server log when the model
repeats them.

The host reads the thread before each turn, but the prompt carries only the
conversation: `message` (the message that the supervisor answers, its author,
and whether the author is a maintainer), the message that opened the thread,
messages addressed to the bot (`toYou: true`), and the bot's own replies
(`from: "you"`). Other messages are only counted in `unreadThreadMessages`. The
supervisor reads them with the `readThread` tool when the message that it answers
refers to them, for example “what do you think?”. The tool returns the newest
messages with their authors and a note that messages to other people are context
only. A self-contained question needs no thread read. A notification turn has
`notification` instead of `message`.

Messages also carry the metadata of their attachments: file name, type, image
size, and description. The supervisor looks at an attachment with the
`viewAttachment` tool when it matters for the answer, for example a screenshot
of a bug. The tool accepts only attachments of messages in the conversation's
thread, and at most 30 views for each message to the bot. It returns PNG,
JPEG, GIF, and WebP images that Chatto resized to at most 1600 pixels on each
side, and text files of up to 100 KB, such as logs. It does not read other
files. The bot downloads an attachment from the configured Chatto server only
when the supervisor views it. Then the model provider receives the content, as
it receives message text. The bot does not log attachment content.
A model without image input receives a placeholder instead of the image.

A message is addressed to the bot when the bot accepted it as a direct message,
a mention, or a reply to one of its messages, from an allowed user. Only
addressed messages count as requests: the supervisor takes instructions and its
reply language only from them, and only addressed messages from maintainers
reach the authorization checks for implementation and GitHub changes. The bot
remembers addressed messages in process memory for 24 hours; after a restart,
older messages count as context. The bot's system prompt does not include its
working directory.

### Maintainers

Source investigation, implementation, and changes on GitHub are available only
to maintainers. Anyone who can address the bot can ask it to read GitHub. Set `CHATTO_MAINTAINER_USER_IDS` to their exact Chatto user IDs,
separated by commas. The bot does not start if one of these capabilities is
configured without maintainers.

Several people can write in one conversation, so the bot checks permission for
each request, not for the conversation. When the agent calls `investigateChatto`,
`implementChatto`, `askImplementation`, `task_send`, or `ghWrite`, the host checks the author
of the latest human message that the bot received in the conversation. If that
author is not a maintainer, or the turn started from a task notification, the tool
does not run. In a user turn, the bot then posts one fixed message that a
maintainer must ask. There is one exception: after a saved implementation
plan's completion notification, the supervisor can call `implementChatto` with
that plan's `investigationId` once on a notification turn. This requires that a
maintainer was the latest person who wrote to the bot when the notification
arrived; the next message from a person ends this exception. Only maintainers
can start investigations, and the authorization check, which receives the
plan's goal, must find a maintainer's request to implement. A maintainer who
asks for a pull request therefore does not have to ask again after the plan. Other users can still ask questions, and the
bot can answer from the documentation and web research.

This gives a simple approval flow: a user reports a problem in a thread, and a
maintainer replies in the same thread to ask the bot to investigate or implement.
The check uses the message the bot received last, not the message that the model
read last, so a maintainer's message that arrives during a turn can already allow
a request from that turn. Anyone in the thread can use `/cancel` or ask the bot to
cancel a task. This can stop work but cannot start or publish anything.

The development and start commands build workspace dependencies before they
start Runling's public CLI. Runling watches this package's configuration and
workflow files during development. From the repository root,
`mise x -- pnpm --dir packages/chattobot dev` starts the same server.
The package is private and has no npm release workflow.

## Conversation behavior

The bot adds an eyes reaction (`👀`) to the message that starts a conversation,
before loading context or engaging the agent. Follow-up messages in the active
thread do not get this automatic reaction. Duplicate deliveries do not add another reaction. `/cancel` skips
the reaction so cancellation stays immediate. The bot needs permission to add
reactions. A failed reaction logs a warning and the reply continues.

The supervisor can use `reactToMessage` to add an emoji reaction to the current
message addressed to the bot. For a simple thanks or acknowledgement, it can
send a reaction without a text reply, including after web research. The model
ends simple acknowledgement turns without text. A reaction does not suppress
an answer to a question. Questions and task results still need a
text reply. The tool permits one call per user message and no calls from
background notifications. It uses the same Chatto client and reaction permission.

The supervisor uses `acknowledgeRequest` to send a brief text reply before
research or other longer work. The host blocks reference loading, web research,
and GitHub reads until the acknowledgement has been posted. For an initiating root message, it sends this
reply first to open the thread. The model writes the acknowledgement in the
user's language. It then continues with the request. Simple follow-up messages
can still receive only a reaction.
Automatic eyes reactions run inside the conversation task, with a ten-second timeout and
cancellation support. Follow-up messages pass through one queue to Pi without
another automatic reaction request. Event acceptance does not wait
for the reaction request.

Replies in the same thread use the same agent and conversation history.
Each new root message starts a separate conversation, including in DMs. Messages sent while
the agent works wait until the current reply is posted. Each queued message starts a new turn with its own reply target and requester. Typing indicators stop while it
waits for another message. Rapid follow-ups wait for the active answer and each starts a new turn in the same run. Send `/cancel`
in the DM thread or mention the bot with
`/cancel` in the thread to stop the conversation.
Duplicate deliveries do not start another run.

A conversation ends after 15 minutes without a new message. Live conversation
state and completed plans are held in memory and are lost on process restart.
Config reload preserves active
conversations for the same source name, server, and bot identity. Existing
conversations keep their original code, server, and credentials; new conversations
use updated code and connection settings.
A mention or a direct reply to a bot-authored message can start a new conversation
in an existing thread. In channels, either mention the bot or use Chatto's reply
action on one of its messages. A thread reply without either does not address
the bot. Everyone who addresses the bot in a thread shares the thread's
conversation. The allowed-user setting still applies.

The bot sets `inReplyTo` to the human message being processed when it can identify
that message. This also applies to tool announcements and split replies. Background
notifications without a direct human target remain untagged. Incoming reply targets
are checked through the configured server's message API, so replies to older bot
messages also work after restart. If the target cannot be verified, an unmentioned
message is ignored. Lookup failures produce a safe warning; a mention or DM still works.

Before each agent turn, the host reads the thread through
`ThreadService/GetThreadEvents`: on the first turn the root and the newest 100
replies, later only the messages after its previous read. This applies to both
DM and channel threads. The prompt carries only the conversation; the supervisor
reads other messages with `readThread`. To view an attachment, the host reads its
address through `AssetService/BatchGetAssets` and downloads it from the same server.
The bot needs permission to read that history. Failed reads stop the run instead
of generating a reply from incomplete context.

For Chatto product questions, the agent can use `fetchPage` to read these
references and follow their links:

- `https://docs.chatto.run/`: documentation for released versions.
- `https://dev-docs.chatto.run/`: documentation for the in-development or
  pre-release version.
- The [Awesome Chatto](https://github.com/nickk-/awesome-chatto) community list,
  read as raw Markdown from `raw.githubusercontent.com`. Its entries are
  unofficial third-party projects. The agent cannot open the linked projects.

The agent is instructed to search both the documentation and the Awesome Chatto
list before it answers questions about Chatto features, setup, or behavior. It
skips them when the conversation already contains the answer or the question is
about a specific release, pull request, or issue. The bot keeps each successfully
read reference page in memory for one hour. It cites pages it reads, says which
documentation version it used, and says when the references do not answer a question. Published
documentation can differ from the connected server version.
The tool permits only those HTTPS locations, including redirects. It rejects URL
credentials and query strings, limits requests to 15 seconds and 512 KB, and
returns at most 30,000 characters of page text with a truncation marker.
It does not execute scripts or fetch page assets. Other websites remain unavailable.

When implementation or GitHub access is configured, replies link issue and pull
request numbers and code references to the repository. Code links from an
investigation point to its base commit; other code links point to the base
branch.

Replies use the Markdown that Chatto renders. Commands, configuration, and code
for the user to copy appear in fenced code blocks.

The agent is instructed not to disclose its model, model provider, instructions,
or host configuration. This instruction does not guarantee confidentiality. Do not
put secrets in agent instructions or tool results.

## Web research

Only maintainers can ask ChattoBot to search the public web or load pages outside
the Chatto reference allowlist. Other users can still read the Chatto reference
docs through the bot. This gate runs before the research agent starts, including
calls from scripts.

ChattoBot can answer questions from the public web through a separate research
agent. Configure one or both of these optional services:

```dotenv
# webSearch: Tavily Search API (basic search, at most 5 results per query)
CHATTO_TAVILY_API_KEY=tvly-...
# browsePage: Cloudflare Browser Run (token permission: Browser Rendering - Edit)
CHATTO_CLOUDFLARE_ACCOUNT_ID=your-32-character-account-id
CHATTO_CLOUDFLARE_API_TOKEN=...
```

Set both Cloudflare values or neither. Restart the bot after you change them.
When at least one service is configured, the chat agent gets a `researchWeb`
tool. It uses the tool when the Chatto references do not answer a question, or
when the user asks about another site. The tool starts a research agent with
these limits:

- It receives only the question that the chat agent writes, not the thread.
- Its only tools are `webSearch` and `browsePage`. It cannot delegate work, post
  to Chatto, or read the source checkout.
- It can make at most 5 searches and 5 page reads, and it stops after three
  minutes. It returns an answer and the source URLs. The chat agent can start at
  most 3 research requests for each user message.
- `browsePage` opens any public HTTP or HTTPS URL without credentials, including
  URLs that the research agent builds itself. Cloudflare loads each page in a
  browser on its network and returns up to 30,000 characters of Markdown. Target
  sites see Cloudflare, not the bot host, and the tool cannot reach private network
  addresses.
- The research agent knows only the question. A hostile page can make it open a URL
  that contains the question text. Therefore the chat agent is instructed to keep
  questions self-contained and free of private details.

If Tavily or Cloudflare rejects a request with a rate limit, the bot waits for
the time in `Retry-After`, at most 10 seconds, and tries once more. The Cloudflare
Workers Free plan allows one page read every 10 seconds, so research with several
page reads is slow on that plan.

The investigation and implementation workers have no web access.

Web content can contain instructions that try to control an agent. A research
result can carry such instructions to the chat agent. After a research result
enters a conversation, Runling blocks `implementChatto`, `askImplementation`,
`task_send` and `decideApproval` for the rest of that conversation. The bot then posts a fixed message
that asks the user to start a new thread. Read-only investigation and
`task_cancel` remain available.

The supervisor can cancel a task with `task_cancel` only in a turn that a
person's message started. Task notifications do not authorize cancellation, in
the same way that they do not authorize new work.

A later conversation in the same thread reads the thread again,
including bot replies that used research results. Runling does not track that
text as untrusted. Start a new thread, not only a new conversation, for work that
must not see earlier research. An investigation plan made after research can
contain injected instructions, but plans stay in their conversation, so they
cannot reach implementation. See Runling's
[ADR-005](../runling/docs/adr/ADR-005-untrusted-context.md).

Tavily receives each search query and the host's IP address. Cloudflare receives
each page URL and the host's IP address. The chat agent is instructed not to put
personal data, secrets, or private conversation details in research questions;
this instruction is not a guarantee. Both services charge for use: Tavily per
search credit, and Browser Run by browser time.

## Source investigation

To let the bot check bug reports and feature requests against source code, set:

```dotenv
CHATTO_SOURCE_DIRECTORY=/absolute/path/to/chatto
CHATTO_SOURCE_REF=origin/main
CHATTO_INVESTIGATION_MODEL=openai-codex/gpt-5.6-sol
```

The directory must be a local Git checkout. The ref must exist locally; the bot
does not fetch updates for investigations. If omitted or empty, the ref defaults to
`HEAD`. When implementation is enabled, investigations use the implementation base
branch's remote-tracking ref instead, for example `refs/remotes/origin/main`, so
plans and changes start from the same branch. Uncommitted changes
in the supplied checkout are not included. Configure the selected model's
credentials in Pi or the host environment, then restart ChattoBot and start a new
conversation. Without `CHATTO_SOURCE_DIRECTORY`, the investigation tool is absent. The
configured directory must exist. A missing directory stops the investigation
with a safe explanation in `failureSummary`; the supervisor can report the
cause. Subprocess errors do not expose command output.

The chat agent can call `investigateChatto` with a question and relevant context.
It supplies a brief announcement in the language of the message that it answers. The tool posts that
message to the conversation thread and waits for delivery before starting work.
If posting fails or the conversation is cancelled, the investigation does not start.
Tool-call preambles stay in agent logs. A delegation announcement or implementation
refusal supplies the turn's user-facing reply; the supervisor's second version
is suppressed. Later turns can report progress or answer new questions normally.
A task notification cannot start investigation. It can start implementation
only for a saved plan, as described above. A refusal
states whether work is active or was already attempted for the same request. The
bot posts refusals once per user turn, and the rest of its reply still posts.
Runling's `taskTool` bridge starts a background child workflow and returns a task
handle. The workflow is shown under the conversation in the console.
A separate read-only agent reads and searches files in a new detached worktree.
It has no edit, write, or shell tools. It cannot change files or execute tests;
implementation requests produce an assessment and suggested changes. Its report returns to
the chat agent with findings, source references, test coverage inspected, limitations, and the base
commit. Completion and requested-answer notifications wake the owning chat
agent. Worker progress is retained for later user questions; raw shell output is not
posted to Chatto. Accepted findings and worker commentary stay in the task buffer
without a notification for each finding. A stopped implementation reaches
the owner as its completion notification, and the owner reports it. A newer host phase
supersedes older progress. Provider retries and blockers remain visible in the
console. Each incoming prompt
includes task status and the age of the latest finding, so the owner can distinguish
old findings from current activity. No status polling tool is exposed.
Each delegated task retains a bounded local buffer of worker commentary and
findings. The owner receives a fresh snapshot on every user message and task
notification. JSON task results are decoded into structured values in this
snapshot; plain-text or truncated results remain text. Retained Runling records
are unchanged. Workflow state records the current phase; implementation also
records completed and pending checks. Commentary is historical context, not
proof of current activity, and does not itself trigger a reply. The buffer is
process-local and does not survive a restart.

Both direct investigation calls and the agent tool default to `purpose: assessment`.
To check whether a bug fix or feature is possible, the supervisor selects
`purpose: feasibility`. The investigator must then return a verdict through
`reportFeasibility`: `already_supported`, `feasible`, `feasible_with_caveats`,
`needs_product_decision`, or `not_feasible`. The verdict also names the estimated
size, the affected areas, compatibility risks, other risks, and open decisions.
The size is an estimate from reading the source, not a commitment. A feasibility
investigation can also return a plan; the conversation keeps it like other plans.
A missing verdict gets one corrective turn, then a blocked `missing_feasibility`
result. Each purpose is a list of deliverables in `workflows/deliverables.ts`.
For a feature or bug plan, the supervisor explicitly selects `purpose: implementation` and
returns a typed plan through `prepareImplementationPlan`. The plan contains a
goal, base commit, file-level steps, acceptance criteria, proposed checks, and
open questions. An assessment-only source question can omit a plan. A missing
required plan gets one corrective turn, then a blocked `missing_plan` result.
The conversation retains successful plans by investigation task ID. On a later
implementation request, the supervisor passes `investigationId`; host code
copies the original plan into the implementation input. Unknown or unfinished
plan IDs are rejected before work starts. Plans are not reconstructed from chat
prose. Plans live only in the active conversation.

After a process restart, old runs remain history only. A new addressed message
starts a new run with a fresh agent and the current thread as context. It does
not restore plans, inboxes, or in-flight work. A new request to continue an
unfinished implementation can reuse that conversation's retained worktree.
The host checks its branch and base commit, then reruns setup and final checks.
It does not resume a result with uncertain publication; inspect GitHub first.
Worktrees created before this recovery metadata was added need manual review.
Old experimental checkpoint files are ignored; no workflow is resumed at startup.

The implementer verifies the plan against its current base commit, preserves
acceptance criteria, and reports necessary deviations. Open questions are not
automatic design requirements. A direct implementation request can still start
without an investigation. A plan does not grant permission to implement or
replace the host's validation commands.

Observed tool activity replaces older progress prose. Tool failures appear in
snapshots with safe error categories and no raw error output. One failed lookup
does not wake the owner. Two failures of the same operation without a success
send a notification; permission failures notify immediately.
An investigation stops after three failures of the same
operation category without success; its artifacts remain available. Provider
recovery updates status silently and does not announce investigation progress.
Routine tool activity updates snapshots without waking the chat agent. User
messages and background notifications have separate, host-supplied origins.
Every prompt includes the last eight messages addressed to the bot
(`recentMessagesToYou`) as a language anchor. The supervisor writes in English,
unless the message that it answers is in another language or asks for one. For a
short message and on notification turns, it uses the language of the anchor.
Other thread messages and its own earlier replies do not choose the language.
Without the English default, the model sometimes switched to an unrelated
language.
Reports must distinguish source evidence from hypotheses and respect the
separate Chatto, Authling, and Runling product boundaries.

The investigator records structured findings through `recordFinding`. The host
extracts excerpts from relative file paths and line ranges in the retained
checkout. Optional supplied quotes must match the source. A citation covers at
most 40 lines and 3,000 characters, and an investigation records at most 12
findings. The host rejects an invalid citation with the reason, such as a
missing file or a range that is too long, so the investigator can correct it.
The finding limit is a normal answer that asks the investigator to finish, not a
tool failure. Only accepted findings reach the owner. Each accepted finding
reaches the owner at once with its checked excerpts, as task output. The final
result carries the claims and their locations without the excerpts, which keeps
it well below the task result limit. Source checks do not establish that a claim follows from the excerpt.
A completed investigation without accepted findings gets one
corrective turn in the same session and checkout. If it still supplies no findings,
the result is blocked with `missing_evidence`. `missing_outcome` identifies a
missing final report; `provider_error` identifies a provider failure. Deliberate
blocked reports and provider failures do not start another repair. The owner is
instructed to report the reason and stopped state without automatically starting
a new investigation. Findings identify observations or hypotheses, proposed changes, and
known limitations. A checked citation proves where the excerpt occurs, not that
the conclusion is correct. The report marks reproduction, test execution, and
applied changes as false. The owner is instructed to keep source citations and
the test limitation in its reply. Proposed changes still need human review.

Each investigation has a ten-minute deadline. `/cancel` cancels the conversation
and its child investigations. Follow-up messages start new supervisor turns, which can
forward relevant clarifications with `task_send`. The worker consumes these as
steering when possible and reports any clarification it could not consume.
The owning conversation remains open while background work is active. Typing
indicators reflect the chat agent's activity rather than the entire investigation.
Task handles are private to one conversation and limited to 32 per conversation.

Worktrees and artifacts remain under this package's `.runling/investigations/`,
including after failure or cancellation. Each directory contains `metadata.json`,
`worktree/`, and, after agent cleanup, `changes.patch`. Read-only investigations
leave the patch empty. Older investigations can contain changes. These are host-local paths, not
public download links. Review them before use. To release a retained worktree,
run `git -C /path/to/chatto worktree remove --force /path/to/worktree` after saving
any wanted files. Then delete its artifact directory. Retention has no automatic
expiry; the operator must manage disk use.

The agent's tools provide read access with the bot host's permissions.
A worktree is not a filesystem sandbox. Use an isolated
host for untrusted users and keep production credentials off that host. Agent
instructions prohibit publishing, pushing, and changing the original checkout,
but those instructions are not an operating-system access control.
The configured investigation model provider receives the supplied report context,
source excerpts, and tool results. Use a checkout whose contents may be sent to that provider.

## Implementation and pull requests

To let ChattoBot implement a requested fix or feature and publish a GitHub PR,
configure these values in addition to the Chatto credentials:

```dotenv
CHATTO_SOURCE_DIRECTORY=/absolute/path/to/chatto
CHATTO_IMPLEMENTATION_REPOSITORY=owner/repo
CHATTO_SOURCE_REF=origin/main
CHATTO_IMPLEMENTATION_MODEL=openai-codex/gpt-5.6-sol
```

`CHATTO_IMPLEMENTATION_REPOSITORY` enables this capability. Without it, the bot
can only investigate and propose changes. Implementation uses `CHATTO_SOURCE_REF`
as its base branch, or `main` when it is unset or empty. It accepts `main`, `origin/main`, and
`refs/remotes/origin/main`. When implementation is enabled, `CHATTO_SOURCE_REF`
must name a branch that exists on origin, rather than a tag or commit.
The model shown above is the default.
The source checkout's `origin` fetch and push URLs must both
match that repository on `github.com`. Fork publication and other Git hosts are
not supported. Git must have an author name and email configured. Install `gh`,
authenticate it for GitHub, and give the host permission to push branches and
create PRs in the configured repository. Restart the bot and start a new
conversation after changing these settings.

Ask the bot to implement a specific change, for example: “Implement the fix we
discussed and open a PR.” The owner announces the task, then starts a separate
implementation worker. Questions and investigation requests do not authorize
implementation. Before `implementChatto` runs, a separate authorization check
reads the ten newest messages from maintainers in the thread and the requested
change, and the bot's latest posted message. It does not see the supervisor's
conversation, other people's messages, or tool results. When it does not find a clear request to implement the change, the
call does not run and the supervisor asks the maintainer to confirm. See
[Authorization checks](#authorization-checks). When GitHub access is configured,
the supervisor can pass `issueNumber`. The host then reads the issue with a
read-only token and gives its title and body to the worker as untrusted
reference text. The pull request closes the issue on merge. The investigator stays read-only. Follow-up messages can steer
the implementation through the same `task_send` channel. Before publication, if
the worker does not consume a forwarded clarification, publication stops. After
publication, the worker waits for CI, and a forwarded message starts its next
turn. Use `/cancel` to stop the whole flow, including while CI runs.

Only the supervisor talks to people in the thread, in its own words.
The implementation task reports only to the supervisor task (Runling ADR-006),
as task notices that wake the supervisor model:

- **Milestones.** Each core stage is a notice whose `data.milestone` names it
  and whose `data` holds its facts: `validating` (the change is ready and host
  checks run), `published` (with `prUrl`), `ci_failed` (with the failed check
  names, the attempt, and its limit), `ci_rerun_pending`, `ci_rerunning`,
  `ci_fix_pushed`, `change_pushed`, and `messages_handled`, which carries the
  worker's answer to messages that arrived while CI ran. The notice text describes the stage for
  the model, not for the user. Add later stages, such as planning or review, as
  new milestones.
- **Progress.** While the worker works, it reports short updates with
  `reportProgress`: its approach, test results, and what it does next. The host
  redacts them like other worker text and sends at most one per minute. An
  earlier update waits until the minute ends, and a newer one replaces it. After
  eight minutes without an update, the host sends the number of changed files
  and test runs so far. The supervisor passes on only what is new, in one short
  sentence.
- **The result.** When the task finishes, the supervisor writes the final
  message from the result: the CI outcome, the PR URL, a summary of the change,
  and its notes.

Notices are not coalesced, so no milestone is lost. The host writes no message
text. It only guards facts: when the supervisor's reply to a new PR or a final
result leaves out the PR URL, the host appends the URL, and when the supervisor
says nothing, the host posts the URL alone.

When a user asks the owner to ask the implementation worker a question, the owner
uses `askImplementation`. The worker's answer wakes the owner, which can reply
while implementation continues. Queue acceptance alone does not mean the worker
answered. The worker uses `answerOwner` with the question ID to send its answer.
Ordinary clarifications still use `task_send`.

Each new implementation fetches the configured base branch and creates a new
`chattobot/<id>` branch in a separate worktree. It does not include uncommitted
changes from the supplied checkout. The host first runs
`mise x -- pnpm install --frozen-lockfile` in that worktree. Then it builds the
workspace packages that the frontend imports, such as the generated API types,
so that focused frontend tests can load. The worker uses
`apply_patch` for source changes and has no shell tool. It can use `reviewDiff`
to read the current diff, including new files, or select one changed path when
the complete diff is too long. `runCheck` runs an approved typecheck, lint, or
build check. `runFocusedTests` runs selected existing frontend test or spec
files in one Vitest project, and `runGoTests` runs the tests of selected Go
packages of the `cli` module, after it copies the legal files that `cmd` embeds.
`runGoTests` rejects `./...`, the complete module. Tests of other areas, such as
the workspace packages, run only in CI. The worker can save brief
handoff notes for a later attempt.
Patch and check failures return bounded diagnostics to the worker. A failed
patch also shows the current lines around its first failed hunk. Worker checks
are recorded separately from the final host checks because edits can make earlier results stale. Repository
setup and check commands do not inherit the bot's Chatto,
Authling, model-provider, or GitHub token variables. The host repeats final
checks before publication.

The host permits the implementation worker to read, edit in its worktree, and
run the existing approved checks. These tools do not yet request an owner grant. The worker's PR proposal waits
for a supervisor decision. Publication, later PR updates, and CI reruns also
wait for approval. Each decision applies to one exact proposal or source
revision. If the source or queued request changes while publication waits, the
host stops it. Existing path, repository, command, and validation checks still
apply.

Approval requests wake the supervisor without posting internal approval messages
to the thread. The supervisor can approve steps already covered by the user's
request. Only it asks the user when more authority is necessary. Pending requests
expire after five minutes and are denied on cancellation or restart. A resumed
implementation must request approval again. This uses
[Runling owner approvals](../runling/docs/approvals.md); it adds no external
service or provider.

An implementation that stops before publication can continue. This includes a
cancellation, a blocked worker, and a restart of the bot. The worker keeps its
conversation in `worker-session.jsonl` in the artifact folder. Each state update
names the artifact, so the supervisor still knows it after a cancellation. Each
supervisor prompt also lists up to three unfinished implementations of the
thread (`resumableImplementations`), so a new conversation after `/cancel` or a
restart can offer to continue too. The supervisor says that the work is kept
and offers to continue it. When the user agrees, it resumes the exact
`implementation-<id>` artifact. The host verifies that the artifact belongs to
the same thread and reuses its branch and worktree. With a saved conversation,
the worker continues it with its full context: it checks the diff, because its
last step may not have finished, and carries on. Without one, as for older
artifacts, the worker receives the original request and the saved handoff, and
must check the handoff against the retained diff. In both cases it also receives
the new instructions from each resume request, which the artifact keeps (at
most five); where they differ, the latest instructions apply. Artifacts created
before conversations were shared by thread belong to their original author's
conversation key and cannot be resumed through the bot.

After the worker reports its edits, the host prepares the tree as the
repository expects. It regenerates protobuf code with `mise run codegen-proto`
when files under `proto/` changed, and formats the changed files with Prettier
and gofmt. Then it runs typecheck and lint for the affected area:
`check:frontend` and `lint:frontend` for changes limited to `apps/frontend/`,
and the root `check` and `lint` scripts otherwise. Commands run through
`mise x -- pnpm run`. Protobuf changes also run `mise run lint-proto`, and
changes to Go source or module files also run `mise run lint-cli`. These checks
are fast and do not fail intermittently. The host does not run test suites: CI
runs them on the pull request, and the worker runs only the tests that its change
affects. Some local browser tests fail intermittently, so a local test failure
would often block a correct change.
The worker must finish its edits before it requests final validation. For a
large, actionable change, it can save progress with `checkpointWork` and get
another work turn in the same implementation. A checkpoint does not start
validation or publication. Three checkpoints with no source changes stop the
attempt and retain the handoff for review. The worker writes
the content that the change needs, including copy and translations. A missing
reviewer or approval does not stop it; it lists review needs in the PR notes,
unless the user requires that review before publication. A blocked or failed worker report ends the attempt
and requires a new user request. The host reports the worker's bounded,
redacted reason and the number of worker checks it ran. It says when final host
validation did not run. After a failed final check, the host sends bounded
diagnostic output to the same worker, with at most two repair turns.
The final result also retains failed-check diagnostics
for supervisor questions, with known host credentials, URLs, email addresses,
and IPv4 addresses removed. These private diagnostics are not operational logs
and must not be copied verbatim into chat or PR descriptions.
Every check must pass on the final Git tree. If a
check changes source files, validation must run again. Setup and each check
have a ten-minute limit. The complete implementation has no fixed deadline;
use `/cancel` to stop it.
The Runling console Log view and server terminal show implementation stages,
check starts and results, and repair attempts. If an agent produces no events
for two minutes, the console shows how long the run has been quiet. The server
terminal reports that the agent remains active without claiming progress.
The owner cannot start a replacement implementation from a task notification.
After a failed attempt, ask explicitly to continue the retained worktree or
start another implementation.
If implementation stops, the owner receives the result and reports it, and the
host retains any worktree for review. The bot waits for user direction before it
starts another attempt.
Unexpected worker or host errors also produce a stopped result when the
conversation remains available.
The host checks for an empty diff, protected instruction or environment files,
changed Git history, and whitespace errors before publication. Passing commands
does not establish that test coverage is sufficient; review is still required.

The host commits the changes with a Conventional Commit title, pushes only the
new branch, and creates a ready-for-review PR. Its body describes what changed,
why, verification, and limitations. It does not merge or deploy. The host reads
back the PR URL, branch, base branch, state, and commit before reporting success.

Then the host follows CI on the PR. It polls GitHub checks every 30 seconds and
stops at the first failed or cancelled check. It gives the failed check names
and the end of up to three failed GitHub Actions job logs, redacted like other
diagnostics, to the same worker. The worker either fixes the failure or, when
the failure is unrelated to its change, calls `rerunFailedChecks`. The host
validates a fix with typecheck and lint, commits it, and pushes it to the same
branch. GitHub reruns jobs only in finished workflow runs. For a rerun, the host
waits until the runs finish, reruns their failed jobs, and reads CI again.
While it waits, a new failure still goes to the worker at once. The worker gets at most three CI failures
(`MAX_CI_REPAIRS`). A message that arrives while CI runs interrupts the wait and
goes to the worker's next turn; its validated edits are also pushed.
The host posts the final CI result when checks pass, still fail after three
repair attempts, are all skipped, stay pending for 30 minutes, or cannot be
read. It also reports when the worker cannot produce a fix or the push fails.
Before each result, it verifies that the PR still points to ChattoBot's last
commit. If someone else pushes to the branch, it stops following CI. CI failure
does not undo the published PR.
If a publication response is lost, it checks for the existing PR rather than
creating another one. An unverified result is reported as uncertain and is not
automatically retried.

`/cancel` stops the conversation and its implementation worker. Cancellation
cannot undo a push or PR that GitHub already accepted. Retained artifacts under
`.runling/implementations/` include the worktree, patch, PR description when
prepared, and publication metadata. Check that metadata and GitHub before
retrying an interrupted publication. Remove retained worktrees with
`git worktree remove` when no longer needed, then remove their local branches
and artifact directories. There is no automatic cleanup or restart recovery.
Stopping the bot interrupts a running implementation, so avoid restarts, including
reloads in `mise dev-chattobot`, while one runs. An implementation keeps running
until CI on its PR finishes. After a restart, ask the bot to continue a stopped
artifact that has no PR. A restart after publication stops CI repair; the PR
stays open.

The host executes dependency setup and repository validation scripts. Although
the worker has no shell tool, edited code can run during validation.
A worktree is not a security sandbox. Use an isolated host and
trusted maintainers; set `CHATTO_MAINTAINER_USER_IDS` to control who can start
work, and `CHATTO_ALLOWED_USER_ID` to restrict who can address the bot at all.
An editor or other tool that finds the retained worktrees can run Git in them
and briefly hold their index lock. The host retries a Git command that fails
for that reason. When a host command fails anyway, the stopped result names the
stage and the command, for example `git add failed (exit 128)`, without its
output. Worker and model-provider errors stay generic, because they can contain
private details.

The worker's saved conversation holds its complete model context, including
source excerpts and the request. It stays in the private artifact folder.
Do not place production credentials on that host. The model provider receives
relevant request context, thread messages with their authors' display names and
logins, source content, and check output. GitHub receives the
host's network address, Git credentials, commits, PR content, and CI log and rerun
requests; public
repositories make the published changes and PR notes public. Commands for
dependency installation or verification can contact package registries and
other services used by the checkout. Package registries receive the host's
network address and requested package names.

## GitHub access

ChattoBot can read GitHub and, when a maintainer asks, change anything in the
repository that its GitHub App may change. It uses a GitHub App, not a personal
token. Actions show as the App's bot account, for example `chatto-bot[bot]`.

1. Create a GitHub App in the organization settings. Turn off its webhook and
   user authorization. Give it the repository permissions that ChattoBot may
   use. These permissions are the limit of what it can do on GitHub. For issue
   and CI work: Issues (read and write), Pull requests (read and write), Actions
   (read and write), Checks (read), Commit statuses (read), and Metadata (read).
   Add Contents (write) only if the bot may merge pull requests or change
   repository files. Actions (write) lets it rerun and start workflows, so
   protect release workflows with GitHub environments that need a reviewer.
2. Generate a private key and store the `.pem` file on the bot host with mode
   `0600`. Install the App on the repository only.
3. Configure the bot:

   ```dotenv
   CHATTO_GITHUB_APP_CLIENT_ID=Iv23li...
   CHATTO_GITHUB_APP_PRIVATE_KEY_FILE=/absolute/path/to/app.private-key.pem
   # Defaults to CHATTO_IMPLEMENTATION_REPOSITORY.
   CHATTO_GITHUB_REPOSITORY=owner/repo
   ```

4. Install `gh` on the bot host and restart the bot.

The client ID is not secret; the private key is. The host uses the key to get
installation tokens, which expire after one hour. Each token can reach only the
configured repository. Reads use a read-only token; changes use a token with the
installation's permissions. The host
reuses a token until shortly before it expires. The key never reaches `gh` or a
model.

The supervisor has two tools:

- `gh` runs read-only commands, such as `issue list` or `view`, `pr checks` or
  `diff`, `run view`, `search issues`, and `gh api` GET requests and GraphQL
  queries. Anyone who can address the bot can ask for them, for example to find
  an issue or to list the issues of a milestone. With a private repository,
  this lets those people read it through the bot. It always uses a read-only token. A command that the host classifies
  wrongly as a read therefore fails at GitHub and changes nothing.
- `ghWrite`, which only maintainers can use, runs changes: for example, it
  files, updates, comments on, labels, closes, or transfers issues, comments on,
  reviews, or merges pull requests, manages labels, releases, secrets, and
  variables, reruns or starts workflows, and makes `gh api` writes. When a
  maintainer asked for the change, it runs at once. The authorization
  classifier decides this from the maintainers' messages to the bot; the
  maintainer does not have to approve the exact text that the bot writes.
  Otherwise nothing runs, and the bot asks whether to make the change. The
  App's permissions decide what succeeds.

The host protects only the bot host and keeps commands on github.com. It allows a
fixed list of subcommands for each command group (see `github/policy.ts`). The
list leaves out gh aliases, programs and credentials (`alias`, `extension`,
`auth`, `config`), and subcommands that read or write local files or Git state,
such as `release create`, `release upload`, `run download`, `pr checkout`,
`pr create`, and `repo clone`. The host also rejects file input (`--body-file`,
`--notes-file`, `--env-file`, `--input`, `-F key=@file`, `--attach`), browsers
and editors,
`--repo`, `--hostname`, `--`, absolute API URLs, jq expressions that read the
environment, and any argument that names another host: a URL, an scp-style
`user@host:path`, or `HOST/OWNER/REPO`. Put text with links to other sites in
the body. `gh` gets no standard input.

`gh` runs without a shell in a new temporary directory. Its environment has only
`PATH`, `TMPDIR`, the repository, and settings that turn off prompts, pagers,
update checks, and telemetry. The token is in a configuration file in that
directory, not in the environment. `gh` cannot see the host's `gh` login, other
credentials, or the private key. The host also rejects combined short flags,
such as `-cq`, so that each flag and value is checked. The supervisor receives
only the URLs from the output of a write. Output is limited to about 32,000
characters. Tokens and email addresses are removed from it.

### Requests and offers

Every change needs a maintainer's request. The authorization classifier checks
each `ghWrite` call against the maintainers' messages to the bot. It also
receives the changes that the bot made in this conversation, and the bot's
latest message as posted to the thread. A short agreement, such as “yes”,
counts only for a change that this message names. So these all run a change:

- a direct request, such as “make a GH issue for this” or “how about filing an
  issue?”;
- agreement to an offer, such as “yes please” after the bot asked whether it
  should file an issue;
- details or corrections sent right after the bot changed an item, which it adds
  to that item, for example as a comment.

When the messages only discuss a problem, nothing runs, and the bot asks in its
own words whether it should make the change. A refusal, such as “not yet”,
prevents the change. A reply to the bot's question counts in any language or
tone, for example “Oui” or “I do!”. When a maintainer asks for a tone, such as
humor, the bot writes in it, unless the text would insult or harass a person.
There is no approval command and no posted command line.
An explicit confirmation, for example a direct message to the requesting
maintainer, is a possible later addition.
The host gives the supervisor only the URLs from a change's output, and adds
them to the reply when the supervisor leaves them out.

GitHub output is untrusted: anyone can write issue bodies and CI logs. After `gh`
returns output in a conversation, `implementChatto`, `askImplementation`,
`task_send` and `decideApproval` are blocked in it, as after web research. `ghWrite` stays
available, because every change needs a maintainer's request. To implement an
issue, ask for it in a new thread; the supervisor passes `issueNumber` to
`implementChatto` without reading the issue itself.

### Authorization checks

The authorization classifier is a separate model call from Runling (Runling
[ADR-007](../runling/docs/adr/ADR-007-authorization-classifier.md)). It checks
GitHub changes and `implementChatto` calls. It receives the action,
maintainers' messages to the bot, and host-chosen context: the changes that ran
in this conversation, saved plan goals, and the bot's latest posted message,
which people answer. That message is model text, so a short agreement counts
only for a change that it names. The classifier never receives the supervisor's
conversation or tool results. Errors,
timeouts, and unclear answers count as “not authorized”. The run log records the
category of each decision (`allow`, `deny`, or `unclear`), never the messages or
the classifier's reason. It uses
`CHATTO_CLASSIFIER_MODEL`, or the supervisor model when that setting is unset.
The classifier's model provider receives the action text and the maintainers'
messages.

## Codemode

The supervisor, the research agent, the investigator, and the implementation
worker have Runling's `codemode` tool (Runling FDR-008). The model can write a
JavaScript script that calls the agent's own tools, for example to read several
files or issues in parallel and keep only the relevant lines. Only the script's
output enters the conversation. Every call from a script passes the same checks
as a direct call: the authorization checks, the research limit, and the block
after untrusted content. Tools that only maintainers can start, and
`task_cancel`, run only as direct calls, because a script could wait until a
maintainer writes. Scripts cannot call `viewAttachment`, because they receive
only text, and the model must see the image itself. Supervisor scripts stop
after four minutes, worker scripts after twenty, and other scripts after ten.
A supervisor script can make at most 30 tool calls, and other scripts 100.

When the output of a script is longer than its limit, Pi writes the complete
output to a temporary file. The output can contain thread messages and GitHub
content. Runling deletes the file at once for agents without a read tool, and
when the agent ends otherwise. When the Chatto source or the implementation CLI
starts, the bot also creates a directory with a random name in the temporary
directory, which only the bot's user can read, and points `TMPDIR` at it. The
bot removes the directory when its process ends; after a crash, it stays. On
Windows, Node does not use `TMPDIR`, and the temporary directory already belongs
to the user.

## Development

`runling.config.ts` registers the `chatto` event source. `workflows/chat.ts`
owns the agent instructions and conversation task.
The `chatto/` directory owns delivery routing, conversation queues, posting,
and typing indicators. `workflows/implement.ts` owns the `implementChatto` tool.
The implementation run is in `implementation-task.ts`, which uses
`implementation-tools.ts` (worker tools), `implementation-validation.ts` (host
checks), `implementation-publication.ts` (commit, push, and PR),
`implementation-ci.ts` (PR checks, failed job logs, and reruns),
`implementation-artifacts.ts` (retained state), `implementation-safety.ts`
(redaction and protected paths), and `implementation-settings.ts`. The run is a
composition of stage functions: `implementation-preflight.ts`,
`implementation-progress.ts`, `implementation-work.ts` (worker turns), and
`implementation-follow-ci.ts` (CI after publication). `workflows/deliverables.ts`
defines the investigation deliverables. `github/` owns the GitHub App tokens,
the `gh` command policy and runner, and the command rendering; `workflows/github.ts`
registers the supervisor's GitHub tools.

To run an implementation without Chatto, use `workflows/implement-cli.ts` with
the same `.env` settings:

```sh
mise x -- pnpm exec runling run workflows/implement-cli.ts \
  --input '{"request":"Fix the typo on the login page"}'
```

Add `"context"` for more detail. The host posts no chat messages; follow the run
in the terminal. Artifacts from these runs have no owner key, so the bot cannot
resume them.

Retained implementation metadata stores an owner key: the SHA-256 hash of the
conversation key from `deliveryConversationKey`. A resume request succeeds only
when the hash matches the current conversation. If you change how the conversation
key is built, keep its value unchanged for existing conversations. Otherwise,
retained implementations cannot be resumed.

Short disconnects resume from the last received event. Unavailable replay
reports a recovery gap and continues from a new snapshot. Each source
generation opens its own connection and closes it when the generation ends, so
a config reload or a process restart starts from a new snapshot. Messages that
arrive while no generation runs are not replayed; ChattoBot logs a warning
after each reload. There is no durable inbox or
exactly-once delivery. Changing server or bot identity starts separate
conversation state. Failed run registration gets up to three attempts, with delays of 250 ms
and 500 ms. Shutdown or reload cancels the wait. Inbox deliveries are not retried.
If all attempts fail, the event remains unaccepted and the source stops.
Terminal connection or routing errors stop the source until a valid
config reload or process restart.

The configured Chatto server receives the host's IP address and bot API key.
Agent requests separately send conversation text to the configured model provider.
Reference requests disclose the host's IP address and requested page path to
`docs.chatto.run`, `dev-docs.chatto.run`, or GitHub (`raw.githubusercontent.com`)
for the Awesome Chatto list. They do not send Chatto credentials. Retrieved page
text is sent to the model provider as reference material. When web access is
configured, Tavily and Cloudflare receive the data described in
[Web research](#web-research). When GitHub access is configured, GitHub receives
the host's IP address, App token requests, and each command; issue text
and comments become public in public repositories.

Run checks from the repository root:

```sh
mise check-chattobot
mise test-chattobot
```

To evaluate the reply policy against synthetic regression cases, run from this
package:

```sh
pnpm runling run evaluations/run.ts --input '{"model":"openrouter/google/gemma-4-26b-a4b-it","repeats":2}' --json
```

This command makes model requests and can incur provider charges. It sends only
synthetic conversation data and the shared reply policy. It has no tools or
Chatto connection and reads no source checkout. Set `"dryRun":true` in the input
to inspect the cases without model requests. The results contain replies, simple
checks, and a review rubric. Review each reply: passing these checks does not
prove semantic correctness. These cases test the reply policy, not tool selection
or complete multi-turn conversations. Use the same model and repeat count when
comparing changes.

To test the full investigator-to-owner handoff against a model, run from this
package with the model's credentials:

```sh
CHATTO_EVAL_MODEL=openrouter/google/gemma-4-26b-a4b-it mise x -- node --env-file-if-exists=.env node_modules/vitest/vitest.mjs run workflows/investigate.test.ts -t 'live worker'
```

This opt-in test uses the production investigation flow with a temporary synthetic
Git repository. It checks actual file access, accepted citations, completion, and
the owner's reply. It makes paid model requests but sends no real source checkout
or Chatto conversation. Normal unit-test runs skip it.
