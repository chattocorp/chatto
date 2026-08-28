# ADR-058: Extract an Application-Neutral Embedded NATS Runtime

**Date:** 2026-07-31

## Context

Chatto and Authling both support simple single-process deployments backed by an
embedded NATS server with JetStream. Each product independently created the
server, started it, waited for readiness, constructed an in-process client
option, cleaned up partial startup, and shut the server down. Chatto's restore
command contained a third copy of most of that lifecycle.

The duplication is application-neutral, but it does not belong in the
`hmans.de/chatto/pkg/events` module from ADR-056. Embedded server process
management is useful without event sourcing and requires the production
`nats-server` dependency that the events framework deliberately excludes.

Authling must also remain independently buildable and movable to another
repository. Importing Chatto's internal embedded-server package would violate
the boundary established by ADR-057.

## Decision

Extract embedded NATS lifecycle mechanics into the independently versioned
`hmans.de/chatto/pkg/natsruntime` module.

The module owns:

- NATS server creation and startup;
- waiting for connection readiness with an explicit caller-supplied timeout;
- cleanup when startup does not become ready;
- construction of an in-process NATS client option; and
- idempotent shutdown that waits for the server to exit.

Applications pass native `nats-server` options rather than a mirrored shared
configuration schema. Chatto and Authling retain ownership of their
configuration, defaults, storage paths, listener exposure, authentication,
monitoring, logging, and deployment policy. The shared runtime overrides
`NoSigs` so the embedding application always owns process signal handling.

The module's repository path is `pkg/natsruntime/`. Its release tags use
`pkg/natsruntime/v<version>`, and its API remains pre-1.0 while both products
establish the smallest useful contract. Chatto and Authling declare the module
dependency explicitly and use repository-local replacements while it is
co-developed here.

`pkg/events` remains the separate event-sourcing framework and keeps its
existing production dependency boundary. Neither shared module imports product
configuration or domain packages.

ADR-059 licenses the shared framework modules under Apache-2.0. Their
permissive licensing is independent of their pre-1.0 API stability.

## Consequences

Embedded startup, readiness, failure cleanup, in-process connection, and
shutdown semantics now have one implementation exercised by both products.
Chatto's normal runtime and restore path use the same lifecycle, and Authling
can move repositories without first replacing a Chatto-internal dependency.

Native NATS server options avoid maintaining an incomplete parallel
configuration API, but the shared module's public contract is intentionally
coupled to the `nats-server/v2` options type. Applications remain responsible
for validating and testing their option mapping.

The repository gains a fourth Go module and release component. Workspace,
standalone tests, dependency caches, Release Please configuration, and
consumer module replacements must include it.
