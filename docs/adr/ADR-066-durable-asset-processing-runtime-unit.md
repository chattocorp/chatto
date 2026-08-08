# ADR-066: Durable Asset Processing as a Runtime Unit

**Date:** 2026-08-08

## Context

Video derivative generation used a process-local callback after message commit.
Boot-time projection scans repaired some interrupted work, but a stable worker
could not discover work committed by another replica, and multiple enabled
replicas could process the same video independently. The main server process
also owned ffmpeg lifecycle even though transcoding is CPU-heavy operational
work that operators may want to scale separately.

Search uses NATS request/reply because callers need an immediate query result.
Asset processing is different: accepting a message creates an asynchronous
obligation that must survive absent workers, process crashes, and handover.

## Decision

Run video derivative generation in the `asset-processing` Chatto runtime unit.
The same unit runs embedded under `chatto run` or standalone as
`chatto asset-processing`. `video.enabled` controls whether the main app accepts
videos and creates processing work. The independent
`asset_processing.enabled` setting controls whether `chatto run` embeds the
worker. `chatto init` writes `asset_processing.enabled = true` for the default
single-process deployment; the standalone command runs explicitly regardless
of that composition setting.

`AssetProcessingStartedEvent` remains the durable PENDING marker and becomes
the work item. Message posting appends it in the same atomic OCC batch as the
owning `MessageBodyEvent` and `MessagePostedEvent`, with an additional guard on
the complete asset aggregate. A rejected message therefore cannot leave an
orphan processing request, and a committed video message cannot lose its
request in a post-commit crash window.

All worker replicas share the durable pull consumer
`chatto-asset-processing-v1` on `EVT`, filtered to
`evt.asset.*.asset_processing_started`. Workers wait for their private
`AssetProjection` through the delivery sequence, process with bounded local
concurrency, publish an OCC-protected succeeded or failed outcome, and
acknowledge only after a terminal asset state is projected. Interrupted work is
negatively acknowledged or allowed to time out for redelivery. Redelivery after
a terminal append is harmless because the worker observes the terminal state
and acknowledges without processing again.

The runtime unit opens existing `EVT` and asset storage resources and runs only
the asset/media boundary needed by the processor. It does not start
`ChattoCore`, execute main-app boot mutations, or use NATS request/reply as its
work transport. A startup compatibility pass creates missing Started markers
for messages written by pre-queue versions; existing Started-only histories are
already discoverable through the durable consumer.

## Consequences

Operators can keep the historical single-process deployment, isolate ffmpeg in
one process, or scale multiple workers against the same queue. The API remains
available while workers are offline; affected attachments stay pending until a
worker returns.

The queue does not require a second work stream or an outbox. EVT remains the
source of truth, while JetStream consumer state records delivery progress. If
that consumer state is lost, replay begins from older Started facts; terminal
state checks make this safe, although replay cost may be higher.

Delivery is at least once. External derivative generation and prompt cleanup
must therefore remain idempotent or OCC-protected. A crash can still leave
unused derivative objects created before the winning terminal event, so durable
failed-generation cleanup remains separate follow-up work.

Rolling deployments must not run incompatible consumer contracts under the
same durable name. A future incompatible work interpretation requires a new
consumer contract/name and an explicit migration plan.
