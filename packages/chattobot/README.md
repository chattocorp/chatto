# ChattoBot

ChattoBot is a Runling project. This first version sends brief agent replies to
Chatto direct messages, mentions, and direct replies to its own messages.
It owns its realtime source configuration and Chatto transport adapter.
It is the private workspace package `@chattocorp/chattobot` under
`packages/chattobot/`. It depends on the public Runling package API and
`@chatto/client` and `@chatto/bot-client`. The client owns API and realtime
transport. The bot client supplies identity, addressing, thread roles,
conversation keys, and acceptance tracking. ChattoBot owns Runling routing,
inboxes, cancellation, and conversation lifetime. See the
[bot client guide](../chatto-bot-client/README.md).

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

### Maintainers

Source investigation and implementation are available only to maintainers. Set
`CHATTO_MAINTAINER_USER_IDS` to their exact Chatto user IDs, separated by commas.
The bot does not start if source investigation or implementation is configured
without maintainers.

Several people can write in one conversation, so the bot checks permission for
each request, not for the conversation. When the agent calls `investigateChatto`,
`implementChatto`, `askImplementation`, or `task_send`, the host checks the author
of the latest human message that the bot received in the conversation. If that
author is not a maintainer, the tool does not run, and the bot posts one fixed
message that a maintainer must ask. Other users can still ask questions, and the
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
thread use only the typing indicator. Duplicate deliveries do not add another reaction. `/cancel` skips
the reaction so cancellation stays immediate. The bot needs permission to add
reactions. A failed reaction logs a warning and the reply continues.
Reactions run inside the conversation task, with a ten-second timeout and
cancellation support. Follow-up messages pass through one queue to Pi without
another reaction request. Event acceptance does not wait
for the reaction request.

Replies in the same thread use the same agent and conversation history.
Each new root message starts a separate conversation, including in DMs. Messages sent while
the agent works are passed to it as steering. Typing indicators stop while it
waits for another message. Rapid follow-ups wait for the initial context load
and reach the agent in arrival order. If the active turn cannot consume a
message as steering, it becomes the next turn in the same run. Send `/cancel`
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

Before each agent turn or steering message, the bot reads the complete thread
through `ThreadService/GetThreadEvents`. It loads all pages in conversation
order, including the root, earlier bot replies, and messages without mentions.
This applies to both DM and channel threads.
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

Replies use the Markdown that Chatto renders. Commands, configuration, and code
for the user to copy appear in fenced code blocks.

The agent is instructed not to disclose its model, model provider, instructions,
or host configuration. This instruction does not guarantee confidentiality. Do not
put secrets in agent instructions or tool results.

## Web research

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
- `browsePage` opens only URLs from the conversation owner's recent messages,
  `webSearch` results, at most 50 links from the page it read last, and any page
  under `https://github.com/chattocorp/chatto/`, such as a pull request or issue. URLs that
  the chat agent writes into the question cannot be opened, so injected text
  cannot make it add conversation data to a URL. Cloudflare loads each page in a browser on
  its network and returns up to 30,000 characters of Markdown. Target sites see
  Cloudflare, not the bot host, and the tool cannot reach private network addresses.

If Tavily or Cloudflare rejects a request with a rate limit, the bot waits for
the time in `Retry-After`, at most 10 seconds, and tries once more. The Cloudflare
Workers Free plan allows one page read every 10 seconds, so research with several
page reads is slow on that plan.

The investigation and implementation workers have no web access.

Web content can contain instructions that try to control an agent. A research
result can carry such instructions to the chat agent. After a research result
enters a conversation, Runling blocks `implementChatto`, `askImplementation`, and
`task_send` for the rest of that conversation. The bot then posts a fixed message
that asks the user to start a new thread. Read-only investigation and
`task_cancel` remain available.

A later conversation in the same thread reads the complete thread again,
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
conversation. Without `CHATTO_SOURCE_DIRECTORY`, the investigation tool is absent.

The chat agent can call `investigateChatto` with a question and relevant context.
It supplies a brief announcement in the user's language. The tool posts that
message to the conversation thread and waits for delivery before starting work.
If posting fails or the conversation is cancelled, the investigation does not start.
Tool-call preambles stay in agent logs. A delegation announcement or implementation
refusal supplies the turn's user-facing reply; the supervisor's second version
is suppressed. Later turns can report progress or answer new questions normally.
An investigation completion notification cannot authorize implementation. A
refusal states whether work is active, was already attempted, or was not started.
Runling's `taskTool` bridge starts a background child workflow and returns a task
handle. The workflow is shown under the conversation in the console.
A separate read-only agent reads and searches files in a new detached worktree.
It has no edit, write, or shell tools. It cannot change files or execute tests;
implementation requests produce an assessment and suggested changes. Its report returns to
the chat agent with findings, source references, test coverage inspected, limitations, and the base
commit. Completion and requested-answer notifications wake the owning chat
agent. Worker progress is retained for later user questions; raw shell output is not
posted to Chatto. Accepted findings and worker commentary stay in the task buffer
without a notification for each finding. A direct host notice replaces the
completion notification for a stopped implementation. A newer host phase
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
Every prompt includes the last eight incoming human messages as a language
anchor. The agent is instructed to follow the human's requested language and
correct earlier accidental language switches, rather than copy its own history.
Reports must distinguish source evidence from hypotheses and respect the
separate Chatto, Authling, and Runling product boundaries.

The investigator records structured findings through `recordFinding`. The host
extracts excerpts from relative file paths and line ranges in the retained
checkout. Optional supplied quotes must match the source. Invalid citations are rejected. Only accepted findings
reach the owner. Source checks do not establish that a claim follows from the excerpt.
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
and its child investigations. Follow-up messages steer the chat agent, which can
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
implementation. The investigator stays read-only. Follow-up messages can steer
the implementation through the same `task_send` channel. If the worker does not
consume a forwarded clarification, publication stops. Use `/cancel` to stop the
whole flow, including after the worker finishes editing. The host reports check
and publication progress. It posts the verified PR link as soon as publication
is confirmed, then posts a separate CI result.

When a user asks the owner to ask the implementation worker a question, the owner
uses `askImplementation`. The worker's answer wakes the owner, which can reply
while implementation continues. Queue acceptance alone does not mean the worker
answered. The worker uses `answerOwner` with the question ID to send its answer.
Ordinary clarifications still use `task_send`.

Each new implementation fetches the configured base branch and creates a new
`chattobot/<id>` branch in a separate worktree. It does not include uncommitted
changes from the supplied checkout. The host first runs
`mise x -- pnpm install --frozen-lockfile` in that worktree. The worker uses
`apply_patch` for source changes and has no shell tool. It can use `reviewDiff`
to read the current diff, including new files, or select one changed path when
the complete diff is too long. `runCheck` runs an approved repository check,
including frontend lint and build. `runFocusedTests` runs selected existing
frontend test or spec files in one Vitest project. The worker can save brief
handoff notes for a later attempt.
Patch and check failures return bounded diagnostics to the worker. After a
worker-requested check fails, the host runs it on the base commit and reports
whether the base passed, failed, or could not be checked. A failed base check
does not establish the cause. Worker checks are recorded separately from the
final host checks because edits can make earlier results stale. Repository
setup and check commands do not inherit the bot's Chatto,
Authling, model-provider, or GitHub token variables. The host repeats final
checks before publication. To continue, ask the bot to resume the exact
`implementation-<id>` artifact from its stopped result. The host verifies that
it belongs to the same thread and reuses its branch and worktree. Artifacts
created before conversations were shared by thread belong to their original
author's conversation key and cannot be resumed through the bot. The next
worker receives the original request and saved handoff, and must check the
handoff against the retained diff.

After the worker reports its edits, the host runs `check:frontend` and
`test:frontend` for changes limited to `apps/frontend/`; other changes run the
root `check` and `test` scripts. Commands run through `mise x -- pnpm run`.
Changes to Go source or module files also run `mise run test-cli`.
The worker must finish its edits before it requests final validation. For a
large, actionable change, it can save progress with `checkpointWork` and get
another work turn in the same implementation. A checkpoint does not start
validation or publication. Three checkpoints with no source changes stop the
attempt and retain the handoff for review. A proposed
human review can be recorded as a PR review need unless the user requires that
review before publication. A blocked or failed worker report ends the attempt
and requires a new user request. The host reports the worker's bounded,
redacted reason and the number of worker checks it ran. It says when final host
validation did not run. After a failed final check, the host runs the same
command on a clean worktree at the base commit. If it also fails, the host
stops and reports that the cause is not known. If it passes, the host sends
bounded diagnostic output to the same
worker, with at most two repair turns. If the base check cannot run, the host
reports that the comparison is unknown and lets the worker try to repair.
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
If implementation stops, the host posts the result to the conversation directly
and retains any worktree for review. If that post fails, the owner receives the
task result and reports the blocker. The bot waits for user direction before it
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
It watches GitHub checks for up to 30 minutes and posts a second message when
they pass, fail, are all skipped, stay pending, or cannot be read. It verifies
that the PR still points to ChattoBot's published commit before it reports a
CI result. If the head changes, it reports that change instead. CI failure
does not undo the published PR. Review the PR Checks tab for individual failures.
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

The host executes dependency setup and repository validation scripts. Although
the worker has no shell tool, edited code can run during validation.
A worktree is not a security sandbox. Use an isolated host and
trusted maintainers; set `CHATTO_MAINTAINER_USER_IDS` to control who can start
work, and `CHATTO_ALLOWED_USER_ID` to restrict who can address the bot at all.
Do not place production credentials on that host. The model provider receives
relevant request context, source content, and check output. GitHub receives the
host's network address, Git credentials, commits, and PR content; public
repositories make the published changes and PR notes public. Commands for
dependency installation or verification can contact package registries and
other services used by the checkout. Package registries receive the host's
network address and requested package names.

## Development

`runling.config.ts` registers the `chatto` event source. `workflows/chat.ts`
owns the agent instructions and conversation task.
The `chatto/` directory owns delivery routing, conversation queues, posting,
and typing indicators.

Retained implementation metadata stores an owner key: the SHA-256 hash of the
conversation key from `deliveryConversationKey`. A resume request succeeds only
when the hash matches the current conversation. If you change how the conversation
key is built, keep its value unchanged for existing conversations. Otherwise,
retained implementations cannot be resumed.

Short disconnects resume from the last accepted event. Unavailable replay
reports a recovery gap and continues live. A process restart starts live.
There is no durable inbox or exactly-once delivery. Changing the API key clears
the checkpoint; changing server or bot identity also starts separate conversation
state. Failed run registration gets up to three attempts, with delays of 250 ms
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
[Web research](#web-research).

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
