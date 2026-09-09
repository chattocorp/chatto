# Runling bot

This local example receives a Chatto outbound webhook through Runling and posts
an agent-generated reply through the Chatto API. It uses Runling’s agent with
`openrouter/google/gemini-2.5-flash-lite` with thinking disabled. It does not connect to the realtime WebSocket. Root messages start a reply thread. Messages in
an existing thread receive a reply in that thread. Messages from bots are
ignored.

Runling is installed as a development dependency at the repository root.
Run the commands below from that root unless a command changes the directory.

## Run locally

1. Set `OPENROUTER_API_KEY` before starting the stack. In fish:

   ```fish
   set -gx OPENROUTER_API_KEY 'your-openrouter-api-key'
   ```

   Run `mise dev` from the repository root. It starts Chatto and Runling together.
   Runling uses the workspace port plus three (`4003` without Conductor).
   The task sets the backend URL and absolute bootstrap API key path. No manual
   Chatto environment variables are required. `CHATTO_DEV_DATA_ROOT` selects the same
   data directory for the server and bot.

2. On an empty server, the stack creates TestBot, writes its API key to
   `cli/data/bootstrap/test_bot.key`, and creates an enabled **Local development**
   outbound webhook at `http://localhost:<base-port-plus-three>/api/runs/start/chatto`.
   `CHATTO_DEV_DATA_ROOT` changes the data directory for both services.

   Existing servers are not changed by bootstrap. If your server predates this
   setup, create the endpoint in **Settings → Server → Bots → TestBot**. For base
   port `55000`, use `http://localhost:55003/api/runs/start/chatto`.
   The local example does not verify the signing secret or Authorization header.
   Runling binds to loopback; its console and run endpoints have no authentication.

3. Post `@test_bot Hello Runling` in `general`, or send TestBot a direct message.
   Open the resulting thread to see its reply. Open `http://localhost:55003`
   (with your workspace's Runling port) to inspect the workflow run.

Conductor also provides a **Runling** preview at
`https://runling.<workspace>.localhost:42444`. Outside Conductor, use `local`
as the workspace name. The direct HTTP port remains available for webhooks.

Stop `mise dev` to stop Runling and the other development services. Do not run
another Runling process on the same port. The bootstrap account remains named
TestBot; the Runling workflow supplies its replies.

The `/api/runs/start/chatto` route validates the workflow input and returns
`202` after starting the run. This avoids holding Chatto's webhook request
open during the workflow. A later workflow failure appears in Runling and does
not cause Chatto to retry the accepted webhook. The synchronous
`/api/webhooks/chatto` route is not used here.

## Agent behavior

Each delivery creates an independent agent session with a general-purpose chat
system prompt. This replaces Pi’s default coding-agent role. For channel mentions and DMs, the workflow loads every page of the current thread
before calling the agent. Context includes the root, human messages, and prior
bot replies in display order, without truncating message text. Every ping
loads fresh context; there is no persistent conversation cache. The agent
can call `read_thread` to refresh the
complete thread. Chatto checks access on each API request.
The agent can use `web_fetch` to read public HTTP and HTTPS pages. This tool
retains the old test bot’s network protections: public addresses only, pinned
DNS results, at most five redirects with destination checks, a 30-second
request timeout, and at most 100 KB of text per response. It rejects URL
credentials and non-text responses. It has no shell or file tools.

For Chatto questions, the system prompt requires the agent to fetch
`https://docs.chatto.run/`, follow relevant documentation links, and cite the
source pages. Fetched content is reference data, not instructions. If the docs
are unavailable or incomplete, the bot must say so instead of inventing an
answer.

The workflow posts one final answer per delivery. Intermediate assistant text,
thinking, tool calls, and tool results are not posted to Chatto. Runling's
local history remains available for diagnostics. The typing indicator shows
that the bot is working.

The agent finishes with Runling's `report_outcome` tool, which stays available
throughout the run, including recovery prompts. `outcome` is the status:
`completed`, `blocked`, or `failed`; it must not contain answer text. The
workflow posts the full answer from `details`, or `summary` when `details` is
empty. The report must contain the complete answer without relying on earlier
assistant text.

The room and reply thread are fixed by the workflow. The sender allows one
HTTP POST attempt per run, shared by final-answer delivery and the error
notification. A failed or uncertain POST is never retried. This protection
applies within one run only. The workflow waits for delivery and disposes the
agent session on exit. Success requires a completed Runling outcome and
confirmed final-answer delivery. The workflow's `replyId` identifies that
answer. Valid blocked or failed reports are posted but keep the run failed.
A missing report is a failure even if the agent produced intermediate text.

A separate **Start typing** step sends a typing indicator in that thread. The
indicator refreshes every three seconds through context loading, composition,
and delivery. Refreshes stop on success or failure; the indicator then expires
through Chatto's normal typing timeout. Typing errors do not fail delivery.
Model runs have a two-minute timeout. Missing model credentials and model
errors fail the run. If context loading or composition fails before a reply
POST, a **Send error reply** step sends a fixed failure message. It contains
no raw error text. The run remains failed in Runling. A previous POST attempt
suppresses this notification because delivery can be uncertain. Error
notifications are best effort and are not retried.

The current message and any requested thread context are sent to OpenRouter.
Credentials are not included in prompts or tool results. Agent text is omitted
from process logs. Local Runling history still contains workflow data.

## Limits

This is one run per webhook delivery. There is no queue, KV state, duplicate suppression,
burst merging, or restart recovery. Repeated deliveries can produce repeated
replies. Runling saves webhook inputs and run history in the ignored `.runling`
directory. Do not put credentials in the payload or workflow output.

The API key must belong to the payload's bot. That bot needs message-read
access and `message.post-in-thread` in the target room. Its current owner and
membership must also permit access. API destinations come from the configured
server URL, never from the webhook body.

## Test

```sh
mise test-runling-bot
```

The tests cover new and existing threads, bot identity, bot-message loops, and
failed reply requests. They make no network or model calls. Agent integration
tests use the real Runling and Pi runtime with synthetic provider responses.
They check the outbound prompt and tools, report validation, recovery, and
suppression of intermediate text. The end-to-end test injects a fixed answer
while testing real webhook delivery and Chatto API calls.

## Code layout

- `reply.ts` defines the webhook schema, inline Chatto API calls, thread loading,
  and workflow steps.
- `agent.ts` defines the chat prompt, read-only tools, and final-report delivery.
- `sender.ts` owns the single delivery attempt and error fallback for each run.
- `typing.ts` owns best-effort typing refreshes and shutdown.
- `web-fetch.ts` contains the public-network fetch protections.

Helper tests cover sender and typing lifecycles. Agent tests cover event
filtering, outcome reports, recovery prompts, and delivery status. Workflow
tests cover authentication, thread context, replies, and error notifications. The browser integration test covers real
webhook delivery and API calls without a paid model request.
