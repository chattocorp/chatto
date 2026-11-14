# Instructions for Agents Working in `pkg/jobs/`

Read the root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-056](../../docs/adr/ADR-056-extractable-nats-event-sourcing-framework.md),
[ADR-069](../../docs/adr/ADR-069-explicit-durable-consumer-lifecycle.md), and
[ADR-123](../../docs/adr/ADR-123-durable-background-jobs.md).

## Boundary

- Keep production code independent of applications.
- This module owns durable job execution: records, dispatches, claims and
  leases, backoff, manual retry, repair, retention, and the effect adapter.
  Event-sourcing mechanics belong in `pkg/events`; JetStream client helpers
  belong in `pkg/jetstreamutil`.
- Applications own resource names and configurations, consumers, job types and
  policies, handlers, payloads, and user interfaces. Do not create, delete, or
  retire application consumers here.
- Base exported API changes on actual Chatto or Authling use.
- The protobuf records in `proto/` are a persisted contract of every
  application that uses this module. Follow the protobuf compatibility rules in
  [`proto/AGENTS.md`](../../proto/AGENTS.md). Run `mise codegen-jobs-proto`
  after a schema change.
- Never store or log handler error text from `Queue` handlers; store only safe
  failure categories. The effect adapter logs handler errors, because
  `events.DurableDeliveryHandler` errors must already be free of secrets and
  personal data.
- Production imports are limited to the Go standard library,
  `github.com/nats-io/nats.go`, `google.golang.org/protobuf`,
  `golang.org/x/sync/errgroup`, `pkg/events`, and `pkg/jetstreamutil`. Tests
  may also use `github.com/nats-io/nats-server/v2`.
- Do not import Chatto or Authling packages.
- Constructors panic when a required argument is missing. They return an error
  for invalid options.
- This module has an independent pre-1.0 version. It has no API stability
  promise.
- The complete module is licensed under Apache-2.0. Keep its source, tests,
  documentation, and standalone license metadata inside that permissive
  boundary.

## Verification

Run:

```sh
mise lint-jobs
mise test-jobs
(cd authling && mise test)
mise test-cli
mise license-check
```
