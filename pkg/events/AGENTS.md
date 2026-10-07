# Instructions for Agents Working in `pkg/events/`

Read the root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-056](../../docs/adr/ADR-056-extractable-nats-event-sourcing-framework.md)
and
[ADR-057](../../docs/adr/ADR-057-temporarily-incubate-authling.md), plus
[ADR-069](../../docs/adr/ADR-069-explicit-durable-consumer-lifecycle.md) for
durable worker resource ownership.

## Boundary

- Keep production code neutral to envelopes and applications.
- Production imports are limited to the Go standard library,
  `github.com/nats-io/nats.go`, `github.com/jellydator/ttlcache/v3`, and
  `golang.org/x/sync/errgroup`. Add another production dependency only after a
  concrete framework use shows that maintaining the mechanic locally is less
  safe or substantially more complex.
- Do not import Chatto or Authling domain packages, protobuf envelopes,
  subjects, resource names, configuration, or lifecycle policy.
- Tests may additionally use `github.com/nats-io/nats-server/v2`, but must not
  borrow product-specific test helpers.
- Base exported API changes on actual external-package users. Do not add a
  general API only to make one application setup shorter.
- Before you remove or change exported API, run `mise x -- go vet ./...` in
  `cli/` and `GOWORK=off mise x -- go vet ./...` in `authling/`. Build the
  embedded web assets first (`mise build-frontend`, and `mise build-web` in
  `authling/`). `go vet` compiles the tests, so it also finds users that a
  text search can miss.
- Guard a write only when its records or the permission to write them depend
  on state that a concurrent write can change. Publish other facts with
  `AppendEventually` or the `Unguarded` mutation boundary. NATS stores the
  guard headers with every message. Keep `AppendBatch` rejecting a batch
  without a guard, so that an unguarded batch is always an explicit choice.
- Constructors panic when a required argument is missing or malformed.
  They return an error when options are invalid or a construction-time check
  fails. Keep new constructors consistent with this rule.
- `DurableWorker` executes an already configured consumer; it must not infer
  consumer ownership from process lifecycle or create, delete, retire, or
  garbage-collect application consumers. Persisted names, inactivity policy,
  rollout, and safe retirement remain application responsibilities.
- This module has an independent pre-1.0 version. It has no API stability
  promise.
- The complete module is licensed under Apache-2.0. Keep its source,
  tests, documentation, and standalone license metadata inside that
  permissive boundary.

## Compatibility

Framework refactors must preserve application-owned event bytes, subjects,
headers, OCC guards, replay order, stream positions, and snapshot/checkpoint
semantics. Change them only when all consuming applications coordinate a
compatible change.

## Verification

Run:

```sh
mise lint-events
mise test-events
mise license-check
```

When Chatto integration changes, also run `mise test-cli`. Authling consumes
this module: also keep `(cd authling && mise test)` passing with `GOWORK=off`.
