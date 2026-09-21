# Chatto plan demo

Send the bot a feature request or bug report in a new root DM. It updates a
Chatto checkout, inspects the code with a read-only agent, and discusses a plan
with you in the reply thread. Reply with feedback to revise the plan.

Once a plan is ready, send exactly `/implement` to accept it. This enters an
implementation task that is a stub: it records the plan and base revision and
posts a confirmation. It does not implement the change. Send `/cancel` at any time to abort the run.

## Setup

Use the same `CHATTO_URL` and `CHATTO_API_KEY` as the
[input demo](chatto-input-demo.md). Set the checkout path in
`packages/runling/.env`. The model override is optional:

```dotenv
CHATTO_WORKING_COPY=/absolute/path/to/dedicated/chatto-checkout
# CHATTO_PLAN_MODEL=openai-codex/gpt-5.6-sol
```

There is no default checkout. If `CHATTO_WORKING_COPY` is empty or unset, the
planning and coordinator demos fail before accessing a checkout or starting
an agent. Other demos can still run. The planning agent uses
the same model credentials as other Runling workflows. From the monorepo root,
run `mise x -- pnpm --dir packages/runling dev`, then
point the bot's outbound webhook to:

```text
http://localhost:5173/api/runs/start/chatto-plan-demo
```

Use this endpoint instead of the input-demo endpoint for this bot. Sending each
DM to both endpoints starts both demos. The existing input demo remains available
at its original URL.

Restart the command after changing `packages/runling/.env`. Send a new DM describing a change,
answer the bot's questions in its thread, and review the plan before sending
`/implement`. The Runling console shows checkout updates, agent turns, input waits,
and the implementation stub. The bot refreshes a thread-scoped typing indicator
while fetching or planning. Completed assistant text messages are posted to the
thread during the interaction, so replies to steering are visible before the
final question or plan. Reasoning and tool output are not posted.
Typing stops refreshing at an input prompt; Chatto clears it through its normal
expiry. Typing failures do not fail the conversation.

## Checkout and conversation rules

Use a dedicated checkout on `main` with an `origin` remote. Before planning, the
bot checks for local changes, fetches `origin/main`, and fast-forwards to the fetched
revision. It refuses dirty files, another branch, or local commits ahead of or
diverged from the fetched revision. It never resets or stashes your work.
Do not edit or switch this checkout during an interview.

One conversation can use a checkout at a time, including while it waits for an
answer. Another root DM receives a busy message; send a new DM when the first
conversation ends. This lock is local to one process. Use one Runling process
for this checkout.

Questions are sequential and each allows 15 minutes for an answer. While the bot
works, messages from the original sender go into a steering inbox. The planner
forwards them into the active agent interaction as steering. The agent receives
steering after the current assistant turn finishes its tool calls, before the
next model call. It cannot change a response that is already being generated.
Messages that miss this delivery window remain in the inbox for the next interaction.
Once a question opens, the next new message answers it. Earlier inbox messages
never become answers. Only the original sender can steer or answer in that thread.

`/implement` is accepted only when a completed plan is awaiting review and no
additional feedback is pending. Commands received while busy are not saved as
approval. `/cancel` aborts the run, including cooperative agent work. Timeouts
post a notice and end the run.

Pending questions and agent conversations live in memory. Restarting Runling loses
them. Config reloads lose reply routing; an existing interview can retain its checkout
lock until it times out. Finish conversations before editing the config. The local webhook endpoint has no authentication;
keep this demo on loopback, as described in the input-demo setup.

## Shared Chatto adapter

`examples/chatto/webhook.ts` contains the webhook schema, duplicate detection,
thread routing, input handler, and ConnectRPC message sender used by both demos.
`createChattoWebhook({ name, output, post, run })` supplies a normal workflow
context with `onInput`; the conversation calls `input(ctx, ...)` as usual.
The fourth `run` argument is an inbox with `drain(): string[]`. Pass it to tasks
that need steering. Draining returns and removes the queued messages in arrival
order; it does not affect input answers. `subscribe(listener)` notifies a task
when a message arrives and returns an unsubscribe function. `prepend(messages)`
restores undelivered messages at the front without notifying listeners again.
`runChattoAgent(ctx, agent, prompt, { destination, inbox, post, typing,
reservedCommands })` handles typing, live steering, and ordered outgoing replies.
It restores undelivered steering and waits for pending replies before returning.
The planning workflow reserves `/implement` and handles approval itself.
See [agent steering](../docs/agent-steering.md) for the delivery contract.
`packages/runling/runling.config.ts` attaches each demo's `route` function. It delivers answers to
pending questions before creating a run. Replies, duplicates, and unrelated
messages return `202 { "runs": [] }` without adding run history. Only a new
root DM starts a conversation. Failed run creation permits a retry.

The adapter presents concurrent input requests one at a time in call order.
Input timeouts include time spent waiting for presentation. It registers the
answer handler immediately before sending the question, so replies during that
request count as answers. Plain chat cannot distinguish an answer from a thought
sent at that boundary. Long outgoing messages are split into smaller posts.
The adapter does not retry posts or persist conversations.
