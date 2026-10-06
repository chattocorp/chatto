# Instructions for Agents Working in `pkg/jetstreamutil/`

Read the root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-056](../../docs/adr/ADR-056-extractable-nats-event-sourcing-framework.md).

## Boundary

- Keep production code independent of applications.
- This module owns only client-side JetStream helpers: key-value reads
  through the stream leader, retried resource provisioning, and conflict
  classification. Event sourcing belongs in `pkg/events`; embedded server
  lifecycle belongs in `pkg/natsruntime`.
- Add only application-neutral JetStream client mechanics whose correctness is
  subtle, such as read consistency, retry, or error classification, and only
  for a concrete Chatto or Authling use. Do not add convenience wrappers or
  general NATS shortcuts.
- Before you remove or change exported API, run `mise x -- go vet ./...` in
  `cli/` and `GOWORK=off mise x -- go vet ./...` in `authling/`.
- Constructors panic when a required argument is missing or malformed. They
  return an error when a construction-time check fails.
- Applications own resource names, configurations, identity metadata,
  timeout policy, and the decisions that they make from key-value state.
- Production imports are limited to the Go standard library and
  `github.com/nats-io/nats.go`. Tests may also use
  `github.com/nats-io/nats-server/v2`.
- Do not import Chatto or Authling packages.
- This module has an independent pre-1.0 version. It has no API stability
  promise.
- The complete module is licensed under Apache-2.0. Keep its source, tests,
  documentation, and standalone license metadata inside that permissive
  boundary.

## Verification

Run:

```sh
mise lint-jetstreamutil
mise test-jetstreamutil
(cd authling && mise test)
mise test-cli
mise license-check
```
