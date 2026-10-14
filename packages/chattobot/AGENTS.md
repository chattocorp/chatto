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
  own agent, write it as a Runling task and give it to the agent with
  `taskTool`. A task returns its result to the agent; it does not post to
  Chatto itself.
- Do not add host code that corrects or replaces the model's replies. If the
  model behaves badly, change the prompt, the tools, or the model, and add a
  test or an evaluation that shows the problem.

## Host text

The model writes the replies. Host code writes text only when the model cannot:
the notice after a failed turn in `conversation.ts`. Do not add new host text
without discussion.

Do not add slash commands. People talk to the bot in plain language.

## Permissions

- Read tools are available to everyone who can address the bot.
- Tools that change something outside the thread, such as `createIssue`, run
  only when maintainers (`CHATTO_MAINTAINER_USER_IDS`) wrote every message of
  the current turn. Check this in the tool, with the server-authenticated
  author ID, never with names or model text.
- A maintainer's turn can still contain text that another person wrote earlier
  in the thread. Keep write tools narrow, and give each one only the GitHub
  permissions that it needs.
- Keep per-turn limits, such as the attachment and web limits, in the
  conversation, and make sure that parallel tool calls cannot pass them.
- Write temporary files only to the private temporary directory
  (`private-temp.ts`).
