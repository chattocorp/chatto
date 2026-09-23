# ADR-100: Share Chatto Integration Client Helpers

**Date:** 2026-09-21

**Status:** Accepted

## Context

The Runling demos, ChattoBot, and Chatto's local bot repeat HTTP
requests, thread pagination, message splitting, and typing timers. The copies
have diverged, including a demo that calls a retired typing method.

## Decision

Keep these helpers in the internal workspace package `@chatto/client` under
`packages/chatto-client/`. Preserve the extracted Runling code's MIT license.
Do not publish this package to npm yet.

The client uses the existing ConnectRPC JSON API and protobuf realtime v4 with no runtime dependency
on Runling or a UI framework. Hosts supply credentials, cancellation, and an
optional HTTP and WebSocket transports. Environment loading, conversation state, agent tools,
webhook authentication, and bot policy remain outside the client.

Requests reject redirects and have bounded durations. Do not retry writes:
a transport failure does not prove that a message was not delivered. Errors
must not include private response bodies or credentials.

Realtime consumption waits for event acceptance before advancing an in-memory
checkpoint. Transient failures reconnect with bounded buffering and backoff.
Unavailable replay starts live and reports a gap. Bot adapters own deduplication
and conversation routing. Runling hosts these adapters as generic event sources
with cancellation and state retained across configuration reloads.

The client resolves viewer identity and message reads. Thread reads take a room
and thread ID, without webhook fields or bot/human roles.

Keep bot conventions in the separate private MIT package `@chatto/bot-client`
under `packages/chatto-bot-client/`. It composes an existing client and supplies
identity retention, configurable DM/mention/reply recognition, reply destinations,
bot-relative thread roles, default conversation keys, and process-local accepted
delivery tracking. It has no Runling dependency or event loop. Reply checks use
the configured server and propagate lookup failures. No additional external
service or protocol change is required.

Hosts own sender restrictions, conversation lifetime, inboxes, cancellation, and
registration. Mark a delivery accepted only after inbox insertion or successful
registration. Keep routing state separate for each server and bot identity.
The tracker does not serialize concurrent deliveries or provide durable recovery.

## Consequences

Three integrations share client behavior and retain tests of their own bot
policies. Protocol changes can be fixed in one place. Runling's published
runtime remains independent of Chatto.

Callers of `client.addressedMessage` must use the bot package. Callers of
`client.readThread` must supply `{ roomId, threadRootId }`; use `bot.readThread`
when bot-relative roles are required. ChattoBot retains its existing conversation
keys and storage across source reloads.

ChattoBot is a private workspace package under `packages/chattobot/`. It uses
the public Runling CLI and package exports, with its own dependencies, checks,
and tests. It retains the MIT license of its former Runling project directory.

The package does not replace generated protocol definitions, the bundled
frontend's state layer, or OAuth. Realtime uses the generated public event types;
it does not provide durable delivery or recovery across process restarts. Typed JSON calls require
tests against server behavior because caller-supplied response types do not
validate responses at runtime.
