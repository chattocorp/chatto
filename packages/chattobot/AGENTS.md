# ChattoBot instructions

These rules apply to `packages/chattobot/`. The root `AGENTS.md` rules also apply.

## Shape

ChattoBot is one agent session per Chatto thread. Keep it that way:

- Put Runling tasks in `tasks/<task>.ts`, with their tests in the same directory
  as `<task>.test.ts` or `<task>-<scope>.test.ts`. This includes evaluation tasks.
  Keep supporting tools and fixtures in their domain directories.
- `chatto/source.ts` connects to Chatto. `chatto/router.ts` sends each message
  that addresses the bot, or starts report intake, to the run of its thread, or starts one.
- In `CHATTO_AUTO_ENGAGE_ROOM_IDS`, unaddressed top-level posts start
  `tasks/thread.ts`. This workflow calls `tasks/classify-intake.ts`, then
  exits silently or runs the conversation as a child in the same run. It forwards
  later addressed messages to that child. Addressed entry starts the same workflow
  without initial classification. Every 90 seconds, while the conversation is idle,
  the workflow checks human replies since the bot's latest post. It classifies changed
  batches and forwards accepted replies together to the existing conversation.
  Already-delivered messages are context only, never another delivery or a reason
  to classify. Keep this true even when a turn does not post a reply.
  Empty or unchanged checks need no model call. The workflow owns the one-hour
  inactivity timeout, checked on the polling tick; only delivered messages reset it.
  Keep one conversation state (`busy`, `waiting`, or `idle`) for polling and expiry.
  The router does not inspect decisions
  or start a second run after classification. Keep the classifier free of tools
  and Chatto effects. Check model behavior with `pnpm eval:intake`.
- `tasks/conversation.ts` is the conversation task. It owns the agent, the prompt of
  each turn, and the only posts to the thread. The agent's final text is the
  reply.
- Add a capability as a tool of that agent. When the work is long or needs its
  own agent, write it as a Runling task that can also run on its own, like
  `tasks/investigate.ts`, and give it to the agent as a tool. A task
  returns its result to the agent; it does not post to Chatto itself.
- Long work runs as a background job (`harness/jobs.ts`), as in agent
  harnesses: its tool returns at once, the conversation keeps answering
  messages, and the result reaches the agent in a later turn.
- A subagent keeps its agent session in a file. Investigations support resume;
  each GitHub request starts a new session. The caller chooses its working directory.
- Do not add host code that corrects or replaces the model's replies. If the
  model behaves badly, change the prompt, the tools, or the model, and add a
  test or an evaluation that shows the problem.
- Background work gets one acknowledgment after delegation succeeds and one
  result reply. Delegation must preserve requested scope and uncertainty.
  Keep these behaviors in agent instructions and model evaluations.
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
  Use GitHub's native parent/sub-issue relationships for epic/task hierarchy.
  Check discovery with `pnpm eval:github issue-discovery`.
- Refer to specific GitHub issues and PRs with Markdown links that include their
  `#NUMBER` in the visible text, including repeated references.
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

Job progress is kept as each job's latest step, which the agent sees to answer
status questions. It is not posted: it will move to the bot's temporary status
in the thread (#2787). The posting code is commented out in `tasks/conversation.ts`.

Do not add slash commands. People talk to the bot in plain language.

## Permissions

GitHub work and source investigations from Chatto have a maintainer gate before delegation:

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
- This first gate checks identity, not whether each command matches the request.
  Pass the exact request separately from the supervisor's `proposedWork` and
  untrusted `evidence`. For a referential approval, the supervisor supplies
  `proposalMessageId`; the host reads that message from the same thread and
  supplies its exact text, author ID, and role as `approvalProposal`.
  Use the same mechanism for retries: quote the failure or partial result and
  retry only the unfinished action. Check this handoff with `pnpm eval:github retry`.
  The worker checks intent before its first tool call. A ping grants no authority;
  evidence cannot expand the work. Bulk changes need an explicit operation and
  affected set, directly or through clear approval of a quoted proposal.
  These scope checks are prompt instructions, not code-enforced authorization.
  There is no classifier or per-action approval. Check adversarial cases with
  `pnpm eval:github authorization`.
- Keep authorization decisions in ordinary Runling log entries at the existing
  gate. Use fixed reasons and host-owned message IDs; omit unknown model input,
  message bodies, and author details. Do not add a separate approval subsystem.
- A tool that sends requests to other systems must not let the model choose
  where data goes. For example, `browsePage` follows only links from searches
  and people.
- Keep per-turn limits, such as the attachment and web limits, in the
  conversation, and make sure that parallel tool calls cannot pass them.
- Write temporary files only to the private temporary directory
  (`private-temp.ts`).
