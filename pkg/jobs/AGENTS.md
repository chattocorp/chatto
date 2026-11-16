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
- This module owns durable job execution: job facts, the worker, backoff,
  manual retry, the job view, retention, and the effect adapter. Use the
  consumer semantics of JetStream for claims, heartbeats, attempt counts, and
  redelivery. Do not add claims, leases, or repair passes of our own.
  Event-sourcing mechanics belong in `pkg/events`; JetStream client helpers
  belong in `pkg/jetstreamutil`.
- Applications own resource names and configurations, consumers, job types and
  policies, handlers, payloads, and user interfaces. Do not create, delete, or
  retire application consumers here.
- Base exported API changes on actual Chatto or Authling use.
- The JSON form of the facts in `fact.go` is a persisted contract of every
  application that uses this module. Add fields only. Do not change the
  meaning of a field or of a fact kind, and do not reuse a name. A reader must
  ignore fields and kinds that it does not know. `fact_test.go` holds stored
  sample facts and pins the form. When the format gains a field, add samples.
  Never change or remove a sample: running servers hold facts in that form.
- Do not add a protobuf dependency. The module encodes only its own small
  facts, and the standard library is sufficient for them. Applications encode
  their payloads as they choose; the queue stores a payload as opaque bytes.
- Never store or log handler error text from `Queue` handlers; store only safe
  failure categories. The effect adapter logs handler errors, because
  `events.DurableDeliveryHandler` errors must already be free of secrets and
  personal data.
- Production imports are limited to the Go standard library,
  `github.com/nats-io/nats.go`, `golang.org/x/sync/errgroup`, and
  `pkg/events`. Tests
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
