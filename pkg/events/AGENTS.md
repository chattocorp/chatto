# Instructions for Agents Working in `pkg/events/`

Read the repository-root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-056](../../docs/adr/ADR-056-extractable-nats-event-sourcing-framework.md)
and
[ADR-057](../../docs/adr/ADR-057-temporarily-incubate-authling.md).

## Boundary

- Keep production code envelope-neutral and application-neutral.
- Production imports are limited to the Go standard library and
  `github.com/nats-io/nats.go`.
- Do not import Chatto or Authling domain packages, protobuf envelopes,
  subjects, resource names, configuration, or lifecycle policy.
- Tests may additionally use `github.com/nats-io/nats-server/v2`, but must not
  borrow product-specific test helpers.
- Drive exported API changes from concrete external-package consumers. Do not
  add generic surface only to shorten one application's wiring.
- The module is independently versioned but pre-1.0 and has no API stability
  promise yet.

## Compatibility

Framework refactors must preserve application-owned event bytes, subjects,
headers, OCC guards, replay ordering, stream positions, and snapshot/checkpoint
semantics unless the consuming applications explicitly coordinate a compatible
change.

## Verification

Run:

```sh
mise test-events
mise license-check
```

When Chatto integration changes, also run `mise test-cli`. When Authling begins
consuming this module, keep `mise test-authling` passing with `GOWORK=off`.
