# ADR-041: Runtime Units for Optional Chatto Processes

**Date:** 2026-06-21

## Context

Chatto is growing beyond a single web/API process. Some capabilities should be
able to run as independent processes in production while still being easy for
single-process self-hosters to run from `chatto run`.

Examples include:

- a Prometheus exporter that reads existing NATS resources and does not need
  `ChattoCore`
- a future Bleve search service that replays `EVT`, maintains its own index,
  and answers search requests over NATS
- future media workers for CPU-heavy video transcoding, image processing, and
  derivative generation

These processes need common configuration loading, logging, NATS connection
setup, graceful shutdown, and access to shared Chatto infrastructure. At the
same time, they must not casually call `ChattoCore.Run`, because the main core
boot path legitimately performs boot-time mutations and repair work.

## Decision

Introduce **runtime units** as the convention for optional Chatto processes.

A runtime unit:

- can run standalone as `chatto <unit>`
- can run embedded in `chatto run` when its config section has
  `enabled = true`
- receives shared config, NATS, JetStream, logger, and version through a small
  runtime environment
- decides explicitly which existing resources or domain services it opens
- does not start embedded NATS when running standalone

Standalone units connect to an existing NATS server as clients. For default
single-process embedded-NATS installs, operators either enable the embedded TCP
listener or set the unit's `enabled = true` flag so `chatto run` starts it in
process using the already-established NATS connection.

Runtime units are classified by behavior:

- **Observer:** reads existing resources and exposes diagnostics, such as the
  Prometheus exporter. No durable writes.
- **Projection service:** consumes `EVT`, maintains a unit-owned index or read
  model, and exposes a NATS service, such as future search. Usually no durable
  writes.
- **Worker:** performs background work and may append durable facts through the
  owning service or `events.Publisher`, such as future media processing.
- **Main app:** the ConnectRPC/web/realtime-delivery process that owns
  `ChattoCore` boot and HTTP compatibility facades.

Durable domain facts still go through `EVT`, and any unit that writes them must
use the same multi-replica-safe OCC and service-boundary rules as the main
process.

## Consequences

Standalone workers and embedded single-process deployments can share one unit
implementation instead of maintaining separate boot paths.

The main `chatto run` process remains the only path that starts embedded NATS
and runs the full `ChattoCore` boot sequence. Side units stay explicit about
whether they are read-only projections, request/reply services, or durable
writers.

Future units should reuse the runtime-unit harness before adding new command
setup, signal handling, NATS connection logic, or ad hoc embedded-mode flags.
Workers that need singleton behavior must still coordinate through NATS
primitives such as `MEMORY_CACHE` leases; embedding a unit in `chatto run` is an
operator convenience, not a correctness boundary.
