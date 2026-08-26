# Instructions for Agents Working in `pkg/natsruntime/`

Read the root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-058](../../docs/adr/ADR-058-application-neutral-embedded-nats-runtime.md).

## Boundary

- Keep production code independent of applications.
- This module owns only embedded NATS process lifecycle mechanics: creation,
  startup readiness, failure cleanup, in-process connection options, and
  shutdown.
- Applications own their configuration schemas, defaults, listeners,
  authentication, monitoring, logging, storage paths, and deployment policy.
- Do not import Chatto or Authling packages.
- Keep the API small over `nats-server`. Do not copy all its options into a
  second configuration model.
- This module has an independent pre-1.0 version. It has no API stability
  promise.
- The complete module is licensed under Apache-2.0. Keep its source,
  tests, documentation, and standalone license metadata inside that
  permissive boundary.

## Verification

Run:

```sh
mise test-natsruntime
(cd authling && mise test)
mise test-cli
mise license-check
```
