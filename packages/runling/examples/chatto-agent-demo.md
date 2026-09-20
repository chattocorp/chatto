# Chatto agent demo

One agent answers a new root DM and receives thread replies as steering while
it works. The bot posts progress and its final reply in that thread, with a
best-effort typing indicator. It can fetch public HTTP/HTTPS URLs with
`web_fetch`, but has no file, shell, or specialist tools. This fetch tool reads
URLs; it is not a web search engine.

The demo registers only the bundled web-fetch extension. Automatic extension,
skill, prompt-template, theme, and project-context loading are disabled.
Pi still uses its normal model/auth configuration. These are tool and resource
settings, not an OS sandbox.

## Try it

Use the Chatto credentials and model authentication from the
[planning demo](chatto-plan-demo.md). Set `CHATTO_URL` and `CHATTO_API_KEY` in
`packages/runling/.env`. The model defaults to `openai-codex/gpt-5.6-luna` with medium reasoning; set
`CHATTO_AGENT_MODEL` to choose another configured model.

From the monorepo root, start `mise x -- pnpm --dir packages/runling dev`
and point the bot's outbound webhook at:

```text
http://localhost:5173/api/webhooks/chatto-agent-demo
```

Use the port shown by your local server. The Chatto server must be able to reach
this URL. Enable only this demo's webhook for the bot while testing.

Send a new root DM, then send another thought in its thread while the bot works.
Messages that miss steering are processed in another turn of the same agent.
This includes messages received while the final reply is being posted.
Send `/cancel` in the thread to stop the run.

After each reply, the run waits for the next thread message using the same agent
and conversation history. Runling shows it as waiting for input; the bot does
not post an extra question or keep sending typing indicators while idle.

The conversation ends after 15 minutes without another message, or on `/cancel`.
The adapter's `settings.timeout` option sets this idle limit in seconds and resets after
each reply. Replies to a finished thread do not start another run; send a new root DM.
Conversations are in memory and do not survive a server restart.

## Code

[`examples/chatto-agent-demo.ts`](chatto-agent-demo.ts) uses the
public `runling/agents` API. Read it in two parts:

1. `conversation` is an ordinary async task. It creates one agent, runs the
   conversation, and releases the agent when the task ends. Its context receives
   incoming text through `inbox` and sends replies through `emit`.
2. `chattoConversation` connects the task to Chatto. New root DMs start runs;
   thread replies go to the task's inbox. Task updates become thread messages.
   The adapter handles typing, duplicate deliveries, and `/cancel`.

The `settings` object supplies the directory and model. Tests supply fake agents
and transports through the same adapter, outside the tutorial's task logic.
The demo-specific [`createWebChatAgent`](chatto/web-chat-agent.ts)
helper holds tool, resource, and prompt configuration.

`runAgentConversation` handles steering while busy and waits for messages while
idle. The timeline shows one conversation lane with working/waiting intervals
and clickable message markers.

The agent uses `output: "text"`: assistant text is the only reply path. Returned summaries and
structured report history are not posted.

Tests use a fake agent and Chatto transport. They cover live steering, missed
messages, replies during posting, progress order, cancellation, and cleanup.
