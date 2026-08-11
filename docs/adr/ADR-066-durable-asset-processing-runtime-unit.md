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
of that composition setting. Worker-owned execution settings such as ffmpeg
paths, temporary storage, and per-process concurrency also live under
`[asset_processing]`; `[video]` contains only upload admission and limits.

`AssetProcessingStartedEvent` remains the durable PENDING marker and becomes
the work item. Message posting appends it in the same atomic OCC batch as the
owning `MessageBodyEvent` and `MessagePostedEvent`, with an additional guard on
the complete asset aggregate. A rejected message therefore cannot leave an
orphan processing request, and a committed video message cannot lose its
request in a post-commit crash window.

All worker replicas share the durable pull consumer
`chatto-asset-processing-v1` on `EVT`, filtered to canonical
`evt.asset.*.asset_processing_started` and legacy
`evt.room.*.asset_processing_started` facts. Workers wait for their private
`AssetProjection` through the delivery sequence, process with bounded local
concurrency, publish an OCC-protected succeeded or failed outcome, and
acknowledge only after a terminal asset state is projected. Interrupted work is
negatively acknowledged or allowed to time out for redelivery. Redelivery after
a terminal append is harmless because the worker observes the terminal state
and acknowledges without processing again.

One processing attempt has a fixed 30-minute safety budget. Exhausting that
worker-owned budget records a terminal processing failure because retrying the
same input under the same limit would churn indefinitely. Process shutdown and
other parent-context cancellation remain retryable handoffs.

The runtime unit opens existing `EVT` and asset storage resources and runs only
the asset/media boundary needed by the processor. It does not start
`ChattoCore`, execute main-app boot mutations, or use NATS request/reply as its
work transport. A startup compatibility pass creates missing Started markers
for messages written by pre-queue versions; existing Started-only histories are
already discoverable through the durable consumer.

The first rollout from callback-era binaries is staged. Every server replica
is upgraded with `asset_processing.enabled = false` before any durable worker
is started. Old replicas continue their local callbacks during that window and
new replicas durably queue their requests. After the last old replica has
stopped, operators enable the embedded worker or start standalone workers.
This prevents an old callback and the new deliver-all consumer from performing
the same transcode concurrently.

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
consumer contract/name and the explicit lifecycle migration from ADR-069.
