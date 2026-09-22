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

The client also resolves viewer identity and message reads, and recognizes whether
a realtime message addresses the viewer through a DM, mention, or verified reply.
It returns normalized Chatto message data and addressing reasons. It does not
construct webhook deliveries or workflow inputs. Reply checks use the existing
message API on the configured server and propagate lookup failures to the host.
Sender restrictions and the decision to ignore or retry those failures remain bot
policy. No additional external service or protocol change is required.

## Consequences

Three integrations share client behavior and retain tests of their own bot
policies. Protocol changes can be fixed in one place. Runling's published
runtime remains independent of Chatto.

ChattoBot is a private workspace package under `packages/chattobot/`. It uses
the public Runling CLI and package exports, with its own dependencies, checks,
and tests. It retains the MIT license of its former Runling project directory.

The package does not replace generated protocol definitions, the bundled
frontend's state layer, or OAuth. Realtime uses the generated public event types;
it does not provide durable delivery or recovery across process restarts. Typed JSON calls require
tests against server behavior because caller-supplied response types do not
validate responses at runtime.
