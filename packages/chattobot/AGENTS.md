# ChattoBot instructions

These rules apply to `packages/chattobot/`. The root `AGENTS.md` rules also apply.

## Shape

ChattoBot is one agent session per Chatto thread. Keep it that way:

- `chatto/source.ts` connects to Chatto. `chatto/router.ts` sends each message
  that addresses the bot to the run of its thread, or starts one.
- `conversation.ts` is the conversation task. It owns the agent, the prompt of
  each turn, and the only posts to the thread. The agent's final text is the
  reply.
- Add a capability as a tool of that agent. When the work is long or needs its
  own agent, write it as a Runling task that can also run on its own, like
  `investigation/investigate.ts`, and give it to the agent as a tool. A task
  returns its result to the agent; it does not post to Chatto itself.
- Long work runs as a background job (`harness/jobs.ts`), as in agent
  harnesses: its tool returns at once, the conversation keeps answering
  messages, and the result reaches the agent in a later turn.
- A subagent keeps its agent session in a file, so the conversation can resume
  it with a follow-up question. The caller chooses its working directory.
- Do not add host code that corrects or replaces the model's replies. If the
  model behaves badly, change the prompt, the tools, or the model, and add a
  test or an evaluation that shows the problem.

## Host text

The model writes the replies. Host code writes text only when the model cannot:
the notice after a failed turn in `conversation.ts`. Do not add new host text
without discussion.

Job progress is kept as each job's latest step, which the agent sees to answer
status questions. It is not posted: it will move to the bot's temporary status
in the thread (#2787). The posting code is commented out in `conversation.ts`.

The approval request (`harness/approvals.ts`) is host text too: it must show
the exact action, not words that the model chose.

Do not add slash commands. People talk to the bot in plain language.

## Permissions

Permissions belong to actions, not to the person who wrote a turn:

- Read tools are available to everyone who can address the bot. Their results
  stay in the thread.
- A tool that changes something outside the thread, such as `createIssue`, is an
  approvable tool (`harness/approval-tool.ts`). It runs only after a maintainer
  (`CHATTO_MAINTAINER_USER_IDS`) approved the exact action with a reaction.
  Text in the model's context cannot change what people approve. Give each such
  tool only the GitHub permissions that it needs.
- A tool that sends requests to other systems must not let the model choose
  where data goes. For example, `browsePage` follows only links from searches
  and people.
- Keep per-turn limits, such as the attachment and web limits, in the
  conversation, and make sure that parallel tool calls cannot pass them.
- Write temporary files only to the private temporary directory
  (`private-temp.ts`).
