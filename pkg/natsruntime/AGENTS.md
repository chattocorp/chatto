# Instructions for Agents Working in `pkg/natsruntime/`

Read the repository-root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-058](../../docs/adr/ADR-058-application-neutral-embedded-nats-runtime.md).

## Boundary

- Keep production code application-neutral.
- This module owns only embedded NATS process lifecycle mechanics: creation,
  startup readiness, failure cleanup, in-process connection options, and
  shutdown.
- Applications own their configuration schemas, defaults, listeners,
  authentication, monitoring, logging, storage paths, and deployment policy.
- Do not import Chatto or Authling packages.
- Keep the API thin over `nats-server`; do not mirror its complete options
  surface in a second configuration model.
- The module is independently versioned but pre-1.0 and has no API stability
  promise yet.

## Verification

Run:

```sh
mise test-natsruntime
mise test-authling
mise test-cli
mise license-check
```
