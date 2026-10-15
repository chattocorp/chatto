# ChattoBot instructions

These rules apply to `packages/chattobot/`. The root `AGENTS.md` rules also apply.

## Shape

ChattoBot is one agent session per Chatto thread. Keep it that way:

- Put Runling tasks in `tasks/<task>.ts`, with their tests in the same directory
  as `<task>.test.ts` or `<task>-<scope>.test.ts`. This includes evaluation tasks.
  Keep supporting tools and fixtures in their domain directories.
- `chatto/source.ts` connects to Chatto. `chatto/router.ts` sends each message
  that addresses the bot to the run of its thread, or starts one.
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
  The agents interpret the request. There is no classifier or per-action approval
  yet. Do not describe this as command-level authorization.
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
