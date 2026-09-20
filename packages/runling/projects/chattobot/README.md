# ChattoBot

ChattoBot is a Runling project. This first version sends brief agent replies to
Chatto direct messages and mentions, including mentions in existing threads.
It owns its webhook configuration and Chatto transport adapter.
It runs under `packages/runling/projects/chattobot/` and uses Runling's
dependencies from the monorepo pnpm workspace.

## Run locally

1. Run `mise setup-frontend` from the monorepo root.
2. Copy `.env.example` to `.env` in this directory. Set `CHATTO_URL` to your Chatto
   server and `CHATTO_API_KEY` to the bot's API key. The bot needs permission to
   read messages and use `message.post-in-thread` (Post in threads) in the Direct messages
   scope. The bot must also be a member of the DM room. In the current Chatto
   implementation, `message.read-interactions` checks for authorship or an
   explicit mention even in DMs; membership alone does not pass that check.
   This can suppress ordinary DM webhooks. Broad `message.read` access works
   around that server behavior, but is not a ChattoBot requirement.
3. Configure OpenRouter in Pi with `/login openrouter`, or set
   `OPENROUTER_API_KEY` in `.env`. The default model is
   `openrouter/google/gemma-4-26b-a4b-it`. Set `CHATTO_AGENT_MODEL` to select
   another configured model.
4. From the repository root, run:

   ```sh
   mise x -- pnpm --dir packages/runling/projects/chattobot dev
   ```

5. Set the bot's outbound webhook to
   `http://localhost:5173/api/webhooks/chatto`. Use only this webhook for the bot
   during testing. The Chatto server must be able to reach it.
6. Send the bot a DM or mention it in a channel message. Replies appear in
   the triggering message's thread in both DMs and channels. For channels,
   the bot needs permission to read the thread and `message.post-in-thread`.

The server loads `.env` from this project directory. Restart it after changing
credentials. The webhook has no authentication; this setup is for local use.
The web console is available at `http://localhost:5173`.

## Conversation behavior

The bot adds an eyes reaction (`👀`) to the message that starts a conversation,
before loading context or engaging the agent. Follow-up messages in the active
thread use only the typing indicator. Duplicate deliveries do not add another reaction. `/cancel` skips
the reaction so cancellation stays immediate. The bot needs permission to add
reactions. A failed reaction logs a warning and the reply continues.
Reactions run inside the conversation task, with a ten-second timeout and
cancellation support. Follow-up messages pass through one queue to Pi without
another reaction request. The webhook does not wait
for the reaction request.

Replies in the same thread use the same agent and conversation history.
Each new root message starts a separate conversation, including in DMs. Messages sent while
the agent works are passed to it as steering. Typing indicators stop while it
waits for another message. Send `/cancel` in the DM thread or mention the bot with
`/cancel` in the thread to stop the conversation.
Duplicate deliveries do not start another run.

A conversation ends after 15 minutes without a new message. Conversation state
is held in memory and is lost on restart or config reload. A mention can start
a new conversation in an existing thread. In channels, mention the bot again
in thread replies so Chatto delivers them. Only the original sender can continue
an active conversation; another participant's mention starts their own run.

Before each agent turn or steering message, the bot reads the complete thread
through `ThreadService/GetThreadEvents`. It loads all pages in conversation
order, including the root, earlier bot replies, and messages without mentions.
This applies to both DM and channel threads.
The bot needs permission to read that history. Failed reads stop the run instead
of generating a reply from incomplete context.

This version has no documentation, source, web, or shell tools. Documentation
answers and code changes are future work.

## Development

`runling.config.ts` registers the `chatto` webhook. Add future webhook routes
there. `workflows/chat.ts` owns the agent instructions and conversation task.
The `chatto/` directory owns delivery routing, conversation queues, posting,
and typing indicators. It does not import example code.

Run checks from the repository root:

```sh
mise check-runling
mise x -- pnpm --dir packages/runling exec vitest run projects/chattobot
```
