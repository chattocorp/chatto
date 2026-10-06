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
- Add a helper only when a concrete Chatto or Authling use needs it, and when
  the helper is useful to more than one call site. Do not let the module
  become a general collection of NATS shortcuts.
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
