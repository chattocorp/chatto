# Chatto input demo

Send the bot a new root DM. The workflow asks two questions with
`input(ctx, ...)`, waits for your replies, then posts a summary.
All bot messages go into a thread rooted at your message. Its context supplies `onInput` to send questions through Chatto.

The workflow is `packages/runling/examples/chatto-input-demo.ts`, registered as
`chatto-input-demo` in `packages/runling/runling.config.ts`.

## Run

Copy `packages/runling/.env.example` to `packages/runling/.env` and set
`CHATTO_URL` and `CHATTO_API_KEY`. Keep the API key in this ignored file.
Runling loads this file when it starts.
The key must belong to that bot. Give it permission to read DMs and `message.post-in-thread` in the
Direct messages scope.
Then run from the monorepo root:

```sh
mise x -- pnpm --dir packages/runling dev
```

In Chatto's bot settings, create an outbound webhook pointing to
`http://localhost:5173/api/runs/start/chatto-input-demo` when both services run on the same
machine. This endpoint returns `202` while the workflow continues. Thread replies are routed
to that run without creating separate runs.
Send the bot a new root DM, then answer each question in the bot’s
reply thread. Open Runling's console to inspect the waiting run and its events.

This local demo has no webhook authentication. Use loopback only; do not expose
Runling's console or endpoints to the internet. A remote Chatto server needs an
authenticated receiver before this demo can be used there.

## Limits

Each question uses `input(ctx, message, { timeout: 30 })`. To test a timeout,
send a new root DM and leave the question unanswered for 30 seconds. The bot
posts a timeout notice in the same thread, and the console marks that input as
“timed out”. Send a new root DM to try again. Answering the first question gives
you a fresh 30 seconds for the second question.

Pending conversations and duplicate checks
live in memory. A restart or config reload loses them. Only one conversation per
person and thread can run at a time. Wait for each question before replying.
Channel mentions are ignored. Replies use the configured Chatto server, never a
URL from the incoming payload. Failed message requests are not retried.

See Chatto's [bot webhook documentation](https://dev-docs.chatto.run/guides/integrations/bot-accounts/#outbound-webhooks)
and [message API](https://dev-docs.chatto.run/reference/connectrpc-api/messages/).
