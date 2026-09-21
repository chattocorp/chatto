# ADR-100: Share Chatto Integration Client Helpers

**Date:** 2026-09-21

**Status:** Accepted

## Context

The Runling demos, its ChattoBot project, and Chatto's local bot repeat HTTP
requests, thread pagination, message splitting, and typing timers. The copies
have diverged, including a demo that calls a retired typing method.

## Decision

Keep these helpers in the internal workspace package `@chatto/client` under
`packages/chatto-client/`. Preserve the extracted Runling code's MIT license.
Do not publish this package to npm yet.

The client uses the existing ConnectRPC JSON API with no runtime dependency
on Runling or a UI framework. Hosts supply credentials, cancellation, and an
optional fetch transport. Environment loading, conversation state, agent tools,
webhook authentication, and bot policy remain outside the client.

Requests reject redirects and have bounded durations. Do not retry writes:
a transport failure does not prove that a message was not delivered. Errors
must not include private response bodies or credentials.

## Consequences

Three integrations share client behavior and retain tests of their own bot
policies. Protocol changes can be fixed in one place. Runling's published
runtime remains independent of Chatto.

The package does not replace generated protocol definitions, the bundled
frontend's state layer, OAuth, or realtime delivery. Typed JSON calls require
tests against server behavior because caller-supplied response types do not
validate responses at runtime.
