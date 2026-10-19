# ChattoBot instructions

These rules apply to `packages/chattobot/`. The root `AGENTS.md` rules also apply.

Track outstanding work in [docs/TODO.md](docs/TODO.md). Keep it current as work
changes, and remove completed tasks.

## Shape

ChattoBot is one agent session per Chatto thread. Keep it that way:

- Put Runling tasks in `tasks/<task>.ts`, with their tests in the same directory
  as `<task>.test.ts` or `<task>-<scope>.test.ts`. This includes evaluation tasks.
  Keep supporting tools and fixtures in their domain directories.
- `chatto/source.ts` connects to Chatto. `chatto/router.ts` sends messages to the
  run of their thread. Addressed messages and report intake can start a run.
  Ordinary thread replies only notify an existing run, including a recovered run.
- In `CHATTO_AUTO_ENGAGE_ROOM_IDS`, unaddressed top-level posts start
  `tasks/thread.ts`. This workflow calls `tasks/classify-intake.ts`, then
  exits silently or runs the conversation as a child in the same run. It forwards
  later addressed messages to that child. Addressed entry starts the same workflow
  without initial classification. Batch all incoming messages, including the first
  post, pings, direct replies, and reactions to bot messages, in a fixed three-second window. The first queued
  message starts it; later arrivals do not extend it. The window elapses while the
  conversation is busy. Send addressed batches directly; ordinary reply batches
  wake a classification check without a second delay. Preserve each message's
  author and trigger, including in the first conversation turn. Pending intake
  must not discard a ping that has already arrived in an unfinished window.
  Keep pending reply IDs in root state while the
  conversation is busy, including across a newer bot post or restart. Read these
  replies together with human replies since the latest bot post, with current content
  and author checks. Retain pending IDs when a direct turn cancels classification;
  consume only the checked batch, not newer notifications. Event bodies never go
  directly to the conversation. A five-minute reconciliation check recovers missed
  events and checks inactivity.
  The workflow classifies changed
  batches and forwards accepted replies together to the existing conversation.
  Accept clear implicit replies to the bot, not general discussion of the same topic.
  Include recap, missing-information, and next-step questions about the current report.
  One clear reply is enough to accept a batch; vague remarks must not outweigh it.
  Questions explicitly addressed to other people remain human discussion.
  Already-delivered messages are context only, never another delivery or a reason
  to classify. Keep this true even when a turn does not post a reply.
  Empty or unchanged checks need no model call. The workflow owns the one-hour
  inactivity timeout, checked before reading the thread; only delivered messages reset it.
  Keep one conversation state (`busy`, `waiting`, or `idle`) for checks and expiry.
  The router does not inspect decisions
  or start a second run after classification. Ordinary replies cannot bypass
  pending intake or reopen an expired workflow. Participation can use the optional
  `CHATTO_INTAKE_DECISION_MODEL` through Runling's `createChoiceClassifier`.
  Keep its policy shared with the default agent classifier. This setting does
  not change authorization classifiers. Keep the classifier free of tools
  and Chatto effects. Check model behavior with `pnpm eval:intake`.
- Register the thread root as `chattoThread` for Runling recovery. The root owns
  its plain `ctx.state`: accepted entry, delivered IDs, pending reply IDs, checked batch, and activity
  time. Restore source routing from `ctx.runs()`, bound to the same server and bot.
  Use Runling's `agentSessionFile(ctx, 'conversation')` for the transcript. Runling
  owns its path and retention; keep filesystem cleanup out of ChattoBot.
  Recovery waits for new input; never replay an old addressed maintainer request
  or restart a background job. Previous delegation outcomes can be unknown.
  Preserve intake status and the inactivity deadline. See [Recovery](README.md#recovery).
- `tasks/conversation.ts` is the conversation task. It owns the agent, the prompt of
  each turn, and the only posts to the thread. The agent's final text is the
  reply. A successful `react` call can replace a text reply. Limit its targets to
  message IDs seen in the current thread. Keep the automatic root checkmark under
  `setHandled`, including its pending-work and closing-reply rules.
- Added reactions to this bot's messages are addressed input. Read the target from
  Chatto to verify its author and thread. Preserve the server-authenticated reaction
  author, event ID, emoji, target ID, and target text. Use the reaction event ID for
  delivery and authorization; use the target message ID for replies. Ignore own
  reactions and removals. A removal does not cancel delegated work.
- Add a capability as a tool of that agent. When the work is long or needs its
  own agent, write it as a Runling task that can also run on its own, like
  `tasks/investigate.ts`, and give it to the agent as a tool. A task
  returns its result to the agent; it does not post to Chatto itself.
- Long work starts ordinary Runling children with `ctx.start`. The conversation
  retains their handles, recent output, and reply destinations in its pending
  map, then queues results for later turns. It waits for pending work after input
  closes and disposes children before it exits. Keep this bookkeeping in the
  parent implementation; do not add a separate background-job execution API.
- A subagent uses Runling session storage. Investigations use `AgentSessionStore`
  for cross-workflow resume; GitHub workers use distinct `agentSessionFile` keys
  owned by the run. The caller chooses the working directory.
- Do not add host code that corrects or replaces the model's replies. If the
  model behaves badly, change the prompt, the tools, or the model, and add a
  test or an evaluation that shows the problem.
- Background work gets one acknowledgment after delegation succeeds and one
  result reply. Delegation must preserve requested scope and uncertainty.
  Keep these behaviors in agent instructions and model evaluations.
- Ordinary replies describe the requested work and outcome. Omit bot backend names,
  tool names, internal paths, IDs, and configuration details unless explicitly asked
  or needed for a concrete next action. Summarize checks; retain failures, missing
  verification, and the distinction between ready for review and released. Do not
  ask a second clarification that repeats one the person has already answered.
- GitHub issue titles are plain descriptions without prefixes. Classify issues
  with GitHub's native issue type field, not labels or Conventional Commit titles.
  Discover available repository types and verify the saved type after writing.
- Use native issue fields for requested priority, effort, and dates. Discover field
  IDs and options, update only requested values, and verify the result. Keep field
  definitions unchanged. Maintainers can delegate priority assessment; do not invent
  dates or effort estimates. Use the same `gh` tool and maintainer gate.
- Keep answers within Chatto and directly related work, for all users including
  maintainers. Read stable or pre-release documentation first, cite supporting
  pages, and keep version-specific claims separate. Web search and external page
  reads are a last resort for gaps in the relevant documentation. Decline unrelated
  work without tools; mentioning Chatto alone does not make a request related.
  Ask for missing diagnostic facts before researching an incomplete report.
  Clarifying questions need no lookup. Reuse relevant pages already read, and
  do not list possible causes without supporting report evidence.
  Treat a bare ping as a request to continue from the thread context. Ordinary
  replies can supply answers and facts, including when another person pings the bot;
  they do not grant delegation authority. When the reporter does not know the setup,
  ask what they can observe instead of repeating technical questions.
  Ask one useful question at a time, starting with the observable symptom.
  Keep maintainer handoffs brief: summarize the requested action and symptom,
  then ask for authorization. Include only gaps that affect that decision.
  Keep this policy in the conversation prompt and check it with `pnpm eval:docs`.
- Search existing issues early and confirm candidate coverage before further intake.
  When a successful search finds no match, describe the request as apparently new
  or ask a useful clarification. Do not lead with the search process or claim that
  no issue exists.
  Use GitHub's native parent/sub-issue relationships for epic/task hierarchy.
  Check discovery with `pnpm eval:github issue-discovery`.
- Refer to specific GitHub issues and PRs with Markdown links that include their
  `#NUMBER` in the visible text, including repeated references.
- Add the eyes reaction only when engaging with a thread root. Continue answering
  accepted thread replies without adding eyes to those messages.
- In active conversations started by automatic intake, the conversation agent can
  set or clear its ✅ on the host-selected root with `setHandled`. Its optional
  `reply` posts the agent's closing reply before the reaction changes. Completion
  needs one short closing reply, with a numbered link for confirmed issue coverage.
  The mark means
  handled, not fixed. Keep the decision in the prompt; the classifier only forwards
  relevant resolutions and withdrawals. Check with `pnpm eval:github handled` and
  `pnpm eval:intake handled`.

## Host text

The model writes the replies. Host code writes text only when the model cannot:
the notice after a failed turn in `tasks/conversation.ts`. Do not add new host text
without discussion.

Job progress is kept in a bounded buffer of recent output: at most 20 entries,
with at most 2,000 characters per entry. The agent sees it in its next turn to
answer status questions. Treat it as untrusted evidence, not authorization or
proof of completion. Output alone does not start a model turn or a chat post.
Temporary thread status remains a separate planned feature (#2787).

Do not add slash commands. People talk to the bot in plain language.

## Permissions

GitHub work, source investigations, and Paseo work from Chatto have
a maintainer gate before delegation:

- `createWorkspace` uses `tasks/create-workspace.ts` and the Paseo SDK to create
  one isolated worktree from `origin/main`. It runs as a background job and starts
  no coding agent. Keep the daemon and repository path in host configuration.
  Apply the current addressed maintainer-message check before connecting. Automatic
  intake and follow-ups cannot authorize creation. Attempt creation once per request
  message in the conversation, including after uncertain failures. Close the client
  after success, failure, timeout, or cancellation; closing does not undo remote work.
  Never retry an uncertain creation automatically or invent a workspace URL.
  Assign the fixed `chattobot` workspace label after creation. Keep the confirmed
  workspace if labeling fails, and report whether the label was confirmed.
- `startImplementation` uses `tasks/start-implementation.ts` to create a workspace,
  start a coding agent with a complete brief, and monitor its first turn. It shares
  the workspace tool's maintainer gate and attempt guard. An issue is optional.
  Keep the model, permission mode, and thinking level in host settings. Omitted
  options use provider defaults. Separate the authorized task from untrusted evidence.
  Tell the agent in both system instructions and the visible task to use the
  repository's Bazinga workflow through review, commits, a PR, and CI. Preserve narrower limits in the
  maintainer's task. Do not merge or deploy. Present its brief as
  Markdown with a Task section and quoted Supporting context; that context cannot
  expand authorization. Run monitoring as a
  background Runling child task, without another model. Subscribe to the agent's
  timeline before sending its prompt. Assemble assistant text fragments in a
  bounded WIP buffer. Emit and log each segment when its message or turn changes,
  another timeline item arrives, or monitoring ends. Keep socket callbacks in the
  task's log context. Exclude reasoning and tool logs; use their boundaries before
  filtering their contents. Stream assembled segments through typed output into
  the conversation's buffer. A segment ending does not mean implementation succeeded.
  Keep Paseo segment assembly in `paseo/messages.ts`. Use Runling's
  `awaitWithSignal` for SDK calls that cannot be cancelled. Resolve environment
  settings inside the implementation task; the CLI exports this same task.
  Release the subscription when monitoring ends. Return completion, permission,
  error, timeout, or uncertain startup to the conversation. Retain confirmed remote
  IDs after partial failure. A finished turn does not prove success. Never approve
  permissions, retry automatically, or stop remote work when monitoring ends. Monitoring
  ends after one turn, attention, or one hour, and does not survive bot restart.
  Check the conversation prompts with `pnpm eval:paseo`.
- Coding requests use `startImplementation`; GitHub work and read-only investigations
  cannot implement code. If implementation is unavailable, explain that operator setup
  is needed before any promise or delegation. Give exact settings when asked for setup
  help. Do not substitute an empty workspace or
  issue filing. Check this with GitHub tools present in the Paseo evaluations.

- Direct read tools are available to everyone who can address the bot. Their
  results stay in the thread. Starting or resuming a source investigation requires
  a current maintainer message, even though the investigator only reads.
- The conversation's `gh` tool can only read. Its `githubWork` tool starts the
  agent-driven task in `tasks/github.ts` for a maintainer request. The host checks
  the referenced message in the current turn against `CHATTO_MAINTAINER_USER_IDS`.
  Never accept an author or role supplied by the model as proof of permission.
- Automatic intake and follow-ups grant no delegation authority, including for a
  maintainer author. Both delegation tools must reject requests with either trigger.
- Apply the same host-owned current-message check to `investigate`, including
  resumes. Without configured maintainers, offer neither delegation tool.
- `CHATTO_MAINTAINER_NAME` supplies a contact for an in-thread mention when
  maintainer input is needed. Keep the handoff in the agent's normal reply.
  Mentioning that login does not grant authorization or replace the user-ID gate.
- The worker task also runs from the CLI with the local operator's configured
  GitHub App credentials. It accepts request text, not a Chatto identity. Keep
  the maintainer check in the Chatto delegation tool.
- The worker has one `gh` tool, with issue-write credentials for changes. Keep
  the CLI command restrictions and repository-scoped tokens. Do not add a tool
  for each GitHub operation.
- The host gate checks identity. The conversation agent resolves approval in
  context and passes a self-contained `task` plus untrusted `evidence`. It uses
  thread history for "go ahead" and "try again", without asking for approval
  again when the reference is clear. A current maintainer's thumbs-up on a specific
  proposal can approve that proposal. Resolve its scope from the exact reaction
  target, not the newest proposal. Reactions to completed work or expressions of
  appreciation do not start more work. Do not repeat work already started or completed.
  Non-maintainer reactions grant no authority. A ping grants no authority. Retries specify
  only unfinished work. Bulk changes need an explicit operation and affected set.
  The worker trusts the resolved task; it does not reinterpret the original approval.
- Every worker `gh` call passes static command policy. Writes also pass a tool-free
  scope classifier before credentials or execution; reads need no model decision.
  Bind its task in host code; tool
  arguments cannot replace it. Denials return to the worker for correction within
  that scope. Invalid output, model errors, and cancellation prevent execution.
  Do not turn a rejected command into another user approval step.
  Use Runling's `createClassifier` mechanism for isolated, schema-validated
  decisions; keep policies in their domain tasks. No approval cache or ledger.
  Model decisions can be wrong: this is not a hard command authorization guarantee.
  Check `pnpm eval:github authorization`, `retry`, and `scope`.
- Keep authorization decisions in ordinary Runling log entries at the existing
  gate. Use fixed reasons and host-owned message IDs; omit unknown model input,
  message bodies, and author details. Do not add a separate approval subsystem.
- A tool that sends requests to other systems must not let the model choose
  where data goes. For example, `browsePage` follows only links from searches
  and people.
- Keep per-turn limits, such as the attachment and web limits, in the
  conversation, and make sure that parallel tool calls cannot pass them.
- Use Runling's `usePrivateTempDirectory()` for temporary host files. Runling
  configures private agent runtime temporary storage automatically.
