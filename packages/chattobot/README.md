# ChattoBot

ChattoBot is the Chatto project's assistant. It runs in a Chatto server as a bot
account. It answers questions about Chatto from the documentation, and
maintainers can ask it to investigate source code and create or update GitHub issues.

ChattoBot is a [Runling](../runling/README.md) application. Each thread in which
someone addresses the bot, or the bot accepts a report, gets one conversation run with one agent session. The
Runling console shows each run, its turns, and its tool calls.

## How it works

1. The `chatto` event source connects to Chatto as the bot. It receives the
   messages that address the bot: direct messages, mentions, and replies to
   the bot.
2. The router sends each message to the conversation run of its thread. When a
   thread has no run, the router starts one.
3. The conversation answers the messages in turns. A turn includes all messages
   that arrived since the previous turn, and the other new messages in the
   thread as context. The first turn includes the thread so far.
4. The agent's final text is posted to the thread as the reply.

A conversation ends after one hour without an addressed message or an accepted
automatic follow-up, once its current turn and background jobs finish. The next
addressed message starts a new conversation, which reads the thread again.
A restart of the bot also ends all conversations.

## Automatic report intake

Set `CHATTO_AUTO_ENGAGE_ROOM_IDS` to the stable IDs of rooms where the bot should
help with new bug reports and feature requests, separated by commas or spaces.
For example, set it to the ID of `#bug-reports`. Use room IDs, not room names.
Restart the bot after changing `.env`. With no value, intake is disabled.

In these rooms, each new top-level post starts a thread workflow.
The workflow verifies a human author and calls a classifier task. The classifier
uses the conversation model with low reasoning effort, no tools, and a
30-second model deadline. It returns a decision and a short reason. The task
does not post, react, or show typing. Ignored posts and failed classifications
stay silent. Users can still mention the bot to get help.

For accepted reports, the workflow runs the normal conversation task as a child
in the same run. It forwards later addressed messages to that child. The bot asks questions
to help prepare a useful report. The classifier receives the post text and
attachment MIME types. Image-only posts are accepted for the conversation
agent to inspect. Clearly unrelated text with an image can still be ignored.

In a conversation started by accepted automatic intake, the bot can add ✅ to
the root message when the request is handled. This means the user confirmed a
resolution or an existing issue, withdrew the report, or authorized filing or
updating succeeded and the bot shared the result. It does not mean a bug is fixed.
Waiting for details, approval, or background work does not qualify. A partial
GitHub result qualifies only after the remaining work succeeds or the maintainer
explicitly accepts the partial result. The bot can remove its checkmark when the
request reopens. Completion includes one short closing
reply, with the numbered issue link when an existing issue covers the report.
The bot needs permission to react; a failed reaction does not stop the conversation.
This tool is limited to the active automatic-intake conversation. Reactions remain
after expiry or restart, but a new conversation does not recover the original
intake status.

Mentions, replies to the bot, and direct messages bypass classification.
Bot posts, forwarded copies, and existing history do not start intake.
`CHATTO_ALLOWED_USER_ID` also applies to intake.

Automatic intake never authorizes GitHub changes or source investigations,
even when the report author is a maintainer. A maintainer must address the
bot with that request. The classifier does not grant permissions.

Duplicate intake deliveries are suppressed for 24 hours in the running
process, including source reloads. A process restart clears this memory.
Failed classifications are not retried automatically. A mention received during
classification cancels the classifier and starts the conversation child with
that addressed message. It does not add a second intake turn or start another run.

## Automatic thread follow-ups

Once the bot joins a conversation, people can answer its questions with ordinary
thread replies. This applies to conversations started by a mention or direct reply
as well as accepted reports. Other rooms still require an addressed message to
start a conversation.

Every 90 seconds, the thread workflow checks for human messages after the bot's
latest post. It waits while the conversation agent is answering. No new messages
means no model call. An unchanged batch is not classified again. When another
message arrives, the classifier considers replies since that bot post that have not
reached the conversation. Already-delivered replies are context only. They cannot
trigger another check or delivery, even when a turn produces no bot post.
The classifier sees text and attachment MIME types, not files.
Replies that answer questions, correct advice, or supply relevant details can
continue the same conversation in one turn. Thanks, unrelated discussion, and
requests to leave the conversation to humans should stay silent.
For automatic-intake conversations, clear resolutions and report withdrawals also
reach the conversation agent so it can update the checkmark; plain thanks stay silent.

Pings and direct replies still reach the conversation immediately and cancel a
pending follow-up check. Polling stops when the workflow ends. Empty, ignored,
and failed checks do not extend its one-hour inactivity timeout. Inactivity is
checked on the polling tick, so expiry can wait for the next check. Classification
failures stay silent until new messages arrive. No follow-up state survives a
process restart. If the newest thread page no longer contains a bot post, the
check skips the thread rather than guessing which messages are new; a ping still works.

Other bots, deleted or unknown authors, and users excluded by
`CHATTO_ALLOWED_USER_ID` cannot trigger an automatic follow-up. These follow-ups
grant no permission for GitHub work or investigations. A maintainer must still
address the bot with such a request.

## Tools

### Chatto questions

Any user can ask about Chatto, including its setup, use, integrations, and
troubleshooting. The conversation agent reads the relevant pages at
[the stable documentation site](https://docs.chatto.run/) or
[the pre-release documentation site](https://dev-docs.chatto.run/) first.
It defaults to stable documentation, uses pre-release documentation for beta
and development versions, and checks both for comparisons or gaps. Answers
cite supporting pages and keep version-specific behavior separate.

For incomplete bug reports, the bot first asks for missing diagnostic facts,
such as the deployment method and startup error. Clarifying questions do not
need research. The bot reads documentation before giving factual advice and
reuses pages already read when they remain relevant. It avoids listing possible
causes before the report supplies evidence for them.

Deployment answers distinguish documented instructions from proposed adaptations.
The bot asks about single-instance or high-availability goals when that choice
changes the advice, and verifies undocumented platform-specific steps against
official platform documentation before recommending commands or configuration.

Web searches and external page reads are a last resort when the relevant
documentation is missing, unavailable, or insufficient. The bot prefers official
sources and reports gaps when it cannot find an answer. Search queries must
not contain private conversation content, logs, credentials, or personal data.

The bot declines unrelated requests without research, including requests that
only mention Chatto or say the answer will be posted there. For mixed requests,
it helps with the Chatto part. Brief greetings and social replies are allowed.
These rules apply to maintainers too. They are model instructions, checked with
behavioral evaluations, not a host-enforced topic classifier.

### Available tools

| Tool             | What it does                                                       |
| ---------------- | ------------------------------------------------------------------ |
| `fetchPage`      | Reads the Chatto documentation and the Awesome Chatto list.        |
| `readThread`     | Reads the newest messages of the thread.                           |
| `viewAttachment` | Looks at an image or a small text file in the thread.              |
| `webSearch`      | Searches the web with Tavily, when configured.                     |
| `browsePage`     | Reads a web page with Cloudflare Browser Run, when configured.     |
| `gh`             | Runs read-only GitHub CLI commands, when GitHub is configured.     |
| `githubWork`     | Starts a GitHub worker for a request from a maintainer.            |
| `investigate`    | Starts or resumes a source investigation for a maintainer request. |

Everyone who can address the bot can use direct read tools. GitHub changes and
source investigations are reserved for maintainers:

- **Changes need a maintainer request.** The bot starts a GitHub worker only
  for an addressed message from a user in `CHATTO_MAINTAINER_USER_IDS` in the
  current turn. The host checks the authenticated author. Claims in message
  text and old maintainer messages do not grant access. Without configured
  maintainers, the bot offers no GitHub worker or source investigator.
- **Investigations need a maintainer request.** The same current-message check
  applies to `investigate`, including resumed investigations. A non-maintainer
  can still use documentation reads and prepare a bug report in the thread.
- **The page reader follows links only.** `browsePage` reads only pages from
  search results or links that people posted, so the bot cannot send thread
  content to an address that it put together.
- **Reads stay in the thread.** Read tools return their results to the bot,
  which answers in the same thread.

Set `CHATTO_MAINTAINER_NAME` to a Chatto user login, for example `reviewer`, to
give the bot a contact for maintainer handoffs. When a report needs a maintainer
decision, the bot mentions `@reviewer` in its normal reply, summarizes the report,
and asks for a reply with the requested action. It should mention the contact
once per unresolved request, not on every status reply. Without this setting,
it explains that a maintainer must authorize the work without guessing a handle.

The login is a contact only. Add the maintainer's user ID to
`CHATTO_MAINTAINER_USER_IDS` to authorize their requests. A mention does not
authorize work, and the maintainer must address the bot in their reply. The
contact must be a member of the room for the mention to be delivered; their
notification settings still apply. The model provider receives the configured
login as part of the bot's instructions. Restart the bot after changing `.env`.

## Background jobs

Long work runs in the background, like subagents in an agent harness: an
investigation or GitHub work. The tool returns at once,
and the bot keeps talking. Its turns list the running jobs with their latest
step, so it can answer status questions. When a job finishes, its result
reaches the bot as a new turn, and the bot replies to the person who asked.

The steps of a job are not posted to the thread yet. They will show as the bot's
temporary status in the thread when Chatto supports it (#2787).

## GitHub work

With GitHub configured, the bot searches existing issues early, summarizes matches,
and asks whether they cover the request before further intake. Reads are available
to all users; writes require a current addressed maintainer request.
The bot's instructions use GitHub's native parent/sub-issue relationships for
epics and their tasks, including verification after an authorized link change.

Issue titles describe the issue without prefixes. The worker uses GitHub's
native issue type field for classification, such as Bug, Feature, or Enhancement,
and discovers the types available in the repository. It does not use labels for
this classification. It verifies the saved type. If no suitable type is available,
it stops before creating or reclassifying the issue and reports the limitation.

The worker also uses native issue fields for requested metadata such as Priority,
Effort, Start date, and Target date. It discovers field definitions and options,
preserves unrelated values, and reads back changes to verify them. A maintainer
can supply values or ask it to assess priority. It does not invent dates or effort
estimates. These fields are separate from issue types, labels, and Projects fields.

A maintainer can ask the bot to create an issue, update its text or labels, or
add a comment. The conversation starts `tasks/github.ts` as a Runling child task
through `githubWork`. The worker uses the same model and reasoning effort as
the conversation. It receives the original request and context from the
conversation, uses one `gh` tool, and returns its result to the conversation.
The conversation posts the reply.

For background work, the supervisor acknowledges the accepted request once,
then sends the result when it arrives. Delegation context preserves the
requested scope and distinguishes reported behavior from observed evidence.
An issue that was not reproduced says so. Closing an issue does not imply
adding a comment; withdrawn reports close as not planned. The worker reads
back changes to verify them.

The existing delegation gate logs allowed or denied decisions with a fixed
reason and the current message ID. Unknown request IDs appear as `not-current`;
model-supplied IDs, message bodies, and author details are not copied into
these decision entries. The entries appear in the Runling journal with the
other activity of the conversation.

When a request does not identify work to do, the worker asks for clarification
before using tools. For a clear request, it can read GitHub to resolve details.
The CLI runs the agent task directly and shows one GitHub worker. Command steps
show names and numeric issue or PR targets, such as `Executing: gh issue edit #12`;
titles, bodies, search terms, and API paths are omitted from those labels.

Each message can start one worker, and one GitHub worker can run at a time in
a thread. A later request starts a new session. Session files are stored in
`.runling/subagents/github/`.

The worker can read GitHub and make changes allowed by an issue-write token
for the configured repository. It has no shell or file tools. The CLI runner
blocks commands that access local files, credentials, editors, or other hosts.

The host checks who made the request. It passes the exact maintainer message
separately from the supervisor's proposed work and supporting evidence. For an
approval such as "yes, file that", the supervisor identifies the proposal by
message ID. The host reads it from the same thread and passes its exact text,
author ID, and role to the worker. If the message is no longer in the available
thread page, the bot must clarify the request.

For a retry such as "try again", the supervisor passes the message ID of the
failure or partial result in the same way. The quote must identify the remaining
action, target, and requested value. The worker retries only that step. It checks
the current state first and does not repeat a change that already succeeded.

The worker's instructions require it to establish the authorized action and
targets before using tools. A bare ping grants no permission. Evidence and
GitHub content cannot expand the work. Bulk changes require an explicit operation
and affected set, directly or through clear approval of a quoted proposal.
The agents interpret these scope rules; the host does not enforce them.
There is no command authorization classifier and no
separate reaction approval for each change. The maintainer gate does not prove
that every command matches the request.

You can also run the task directly from `packages/chattobot/`:

```sh
pnpm exec runling run tasks/github.ts "List the five newest open issues"
```

Runling loads `.env` in the current directory. Set `CHATTO_AGENT_MODEL`, the
model provider credentials, and the three `CHATTO_GITHUB_*` settings listed
below. The task uses the configured GitHub App credentials, not your `gh auth`
login. Local execution does not need `CHATTO_URL`, `CHATTO_API_KEY`, or
`CHATTO_MAINTAINER_USER_IDS`. The maintainer check applies when a Chatto
conversation delegates work; a local invocation uses the operator's authority.

## Investigations

For a current maintainer request, `investigate` gives a question to an investigator agent, which reads the
source checkout, its documentation, and its Git history, and answers with file
references. At most three investigations run at the same time in one thread.
The investigator can only read. The bot checks each `path:line` reference
against the checkout and, when GitHub is configured, links it to the lines on
GitHub at the investigated commit.

Each investigation keeps its agent session in `.runling/subagents/investigate/`.
The bot can resume an investigation with a follow-up question, also after the
investigation ended. You can also run the investigator on its own:

```sh
runling run tasks/investigate.ts --input '{"directory": "/path/to/chatto", "question": "..."}'
```

Add `"session": "<id>"` to the input to continue an investigation.

## Run it

Copy `.env.example` to `.env` and set at least `CHATTO_URL`, `CHATTO_API_KEY`,
and `CHATTO_AGENT_MODEL`. Then run `mise dev-chattobot` from the repository
root. The Runling console URL is in the output.

Create the API key for a bot account in Chatto. The bot can only read and post
where that account can.

## Settings

| Setting                                                                                         | Description                                                                                                    |
| ----------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `CHATTO_URL`, `CHATTO_API_KEY`                                                                  | The Chatto server and the bot account's API key. Required.                                                     |
| `CHATTO_AGENT_MODEL`                                                                            | The model, as `provider/model`, for example `openrouter/openai/gpt-5.5`. Required.                             |
| `CHATTO_AGENT_THINKING`                                                                         | Reasoning effort: `off`, `minimal`, `low`, `medium` (default), `high`, `xhigh`, or `max`.                      |
| `CHATTO_ALLOWED_USER_ID`                                                                        | Answer only this Chatto user ID. Use it for tests on a shared server.                                          |
| `CHATTO_AUTO_ENGAGE_ROOM_IDS`                                                                   | Optional room IDs for automatic report intake, separated by commas or spaces. Disabled by default.             |
| `CHATTO_MAINTAINER_USER_IDS`                                                                    | Chatto user IDs of maintainers, separated by commas.                                                           |
| `CHATTO_MAINTAINER_NAME`                                                                        | Optional Chatto user login to mention for maintainer input. Grants no permissions.                             |
| `CHATTO_GITHUB_APP_CLIENT_ID`, `CHATTO_GITHUB_APP_PRIVATE_KEY_FILE`, `CHATTO_GITHUB_REPOSITORY` | GitHub access through a GitHub App, for one repository (`owner/repo`).                                         |
| `CHATTO_TAVILY_API_KEY`                                                                         | Enables `webSearch`.                                                                                           |
| `CHATTO_SOURCE_DIRECTORY`                                                                       | A checkout of the Chatto source. Enables `investigate`. Keep it current yourself, for example with `git pull`. |
| `CHATTO_INVESTIGATION_MODEL`, `CHATTO_INVESTIGATION_THINKING`                                   | The investigator's model (default: `CHATTO_AGENT_MODEL`) and reasoning effort (default: `medium`).             |
| `CHATTO_CLOUDFLARE_ACCOUNT_ID`, `CHATTO_CLOUDFLARE_API_TOKEN`                                   | Enables `browsePage`.                                                                                          |

Model providers use their own settings, for example `OPENROUTER_API_KEY`, or a
Pi login on the bot host.

### GitHub App

Create a GitHub App and install it on the repository only. Give it these
repository permissions: Issues (read and write), Pull requests, Actions,
Checks, and Commit statuses (read). ChattoBot creates a token for each command,
limited to the repository and to the permissions that the command needs. Read
commands get read-only tokens. The private key stays on the bot host.

To use native issue fields, also grant the App the organization permission
**Issue Fields: read**, and approve the updated installation permissions.
ChattoBot requests this permission only for field discovery; other GitHub work
does not require it. Updating values uses the existing **Issues: write** permission.
See GitHub's [issue field API](https://docs.github.com/en/rest/issues/issue-field-values).

## Privacy

ChattoBot sends these data to other systems:

- **The model provider** receives the messages to the bot, the other new
  messages in the thread, the attachments that the bot views, and all tool
  results.
- **The Chatto documentation sites and GitHub** receive the requests of
  `fetchPage` and the bot host's IP address.
- **GitHub** receives each `gh` command, the changes that the bot makes, and the
  bot host's IP address.
- **Tavily** receives search queries, and **Cloudflare** receives page URLs,
  when they are configured.
- **The model provider** of the investigator receives the questions and the
  source code that the investigator reads.
- **The model provider** of the GitHub worker receives the maintainer request,
  the proposed work, supporting evidence, any quoted proposal with its author
  and message ID, and the GitHub tool results.

With automatic intake enabled, the model provider also receives eligible new
posts and attachment MIME types in the configured rooms, without a mention.
For every active bot conversation, follow-up classification can send ordinary
human replies and a short preceding excerpt to the same provider without a ping.
These requests reveal the bot host's IP address, not the author's IP address.
Tell room members that the bot processes their posts. Classification incurs
model costs even when the bot stays silent.

The Runling run journals in `.runling/runs/` contain the messages and replies.
The sessions in `.runling/subagents/` contain investigation questions and source
code, and GitHub worker requests, context, and tool results.
Keep that directory private.

## Develop

- `mise check-chattobot` checks the types.
- `mise test-chattobot` runs the tests.

### Documentation model evaluations

From `packages/chattobot/`, run:

```sh
pnpm eval:docs
pnpm eval:docs web-last-resort
```

These opt-in evaluations use the configured model and reasoning effort. They
contact the model provider and incur model costs. Chatto messages, documentation,
and web results are synthetic; no documentation site, Tavily, Cloudflare, or
Chatto server is contacted. Production tool schemas and descriptions are used
with fixture executors.

Cases cover stable and pre-release docs, version comparisons, web fallback,
missing evidence, unrelated and mixed requests, and instructions hidden in
reference content. Two-turn cases check startup clarification before research,
reuse of documentation already read, and bare pings after another person answers
without a mention. The bare-ping cases include a maintainer nudge with privileged
tool schemas available; all executors still use fixtures. They check that the bot
asks about observable symptoms when the reporter does not know the server setup.
Deployment cases check topology clarification and the limits
of a documented example. They check tool calls, their order, citations, and reply
content. Each case has a two-minute deadline. Reports are saved under
`.runling/evaluations/`. A passing run is a behavioral sample, not proof that
the model will always follow the topic restriction.

### Intake model evaluations

From `packages/chattobot/`, run `pnpm eval:intake`, or select a case with
`pnpm eval:intake screenshot-only`. These evaluations send synthetic posts
to the configured model provider. They incur model costs but do not contact
Chatto or GitHub. Cases cover reports, feature requests, incomplete reports,
German text, unrelated messages, instruction injection, and image metadata.
Follow-up cases cover partial and batched answers, corrections, screenshots,
resolved questions, human discussion, and requests for silence.
Reports are saved under `.runling/evaluations/`. A passing run is a behavioral
sample, not proof that every classification will be correct.

### GitHub model evaluations

From `packages/chattobot/`, run:

```sh
pnpm eval:github
pnpm eval:github ambiguous-moo
pnpm eval:github authorization
pnpm eval:github issue-discovery
pnpm eval:github issue-fields
pnpm eval:github retry
pnpm eval:github language-report-withdrawal
```

These opt-in evaluations use `CHATTO_AGENT_MODEL` and
`CHATTO_AGENT_THINKING` from `.env`, with the model provider's credentials.
They contact the model provider and incur model costs. They use synthetic
messages and an in-memory GitHub fixture; they do not contact GitHub or Chatto
and do not load GitHub App credentials.

Worker cases cover ambiguous input, explicit searches, issue creation and
edits, instructions hidden in issue content, and a lost response after a
successful write. Supervisor cases cover maintainer and member requests,
including both authors in one turn and an unrelated maintainer message. They
also cover investigation authorization, report intake, and acknowledgments
that must not claim completed work.
The `authorization` group checks injected bulk changes, bare maintainer pings,
missing or ambiguous proposal quotes, explicit approval of a quoted proposal,
and legitimate bulk closure. Case-name prefixes select a group of evaluations.
The `issue-discovery` group checks early feature and bug searches, multiple
plausible matches, closed proposals, no matches, and vague reports that need
clarification first.
The `issue-fields` case checks priority discovery, update, and verification,
preservation of other values, and rejection of extra instructions in evidence.
They check commands, changed issue state, and delegation targets. They do not
require exact reply wording. Most supervisor cases use fake GitHub workers and
investigators; the worker cases use the real worker agent.

The `retry` group checks the complete supervisor-to-worker handoff with both real
agents, retries after partial success, already-saved values, missing or ambiguous
references, and extra instructions in evidence. It also checks that a partial
result does not mark the thread handled.

The language-report-withdrawal case uses both the real supervisor and real
worker agents. It replays a language complaint, an explicit request to file
it, and a request to close it as a joke. It checks acknowledgment counts,
uncertainty in the report, closure without an extra comment, the closure
reason, and reads that verify changes.

Each case runs once, with a two-minute deadline and a limit of 16 fixture
commands. A failed assertion or an unsupported fixture command fails the run.
Reports and worker sessions are saved under `.runling/evaluations/`.
The report includes the model, commands, replies, state, and failure details.
The fixture supports a small issue CLI and REST subset, not the complete
GitHub API. A passing run is a behavioral sample, not proof of authorization
or a live GitHub integration test. These evaluations are separate from the
deterministic tests in `mise test-chattobot`.

Read [AGENTS.md](AGENTS.md) before you add a tool or change the conversation.
