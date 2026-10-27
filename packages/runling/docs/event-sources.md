# Event sources

Use `sources` in `runling.config.ts` to receive events through outbound
connections or other adapters. Runling does not require an inbound webhook.

```ts
import { defineWebConfig, startWorkflow } from 'runling/web';
import echo from './workflows/echo.ts';

export default defineWebConfig({
  sources: {
    messages: async ({ signal, dispatch, state }) => {
      const input = await openInput({ signal, state });
      try {
        for await (const message of input) {
          await dispatch(startWorkflow(echo), message);
        }
      } finally {
        await input.close();
      }
    }
  }
});
```

`openInput` represents your adapter. Runling exports `EventSource` and
`EventSourceContext` from `runling/web`; neither depends on a transport or
Chatto. `webhooks` and `sources` can be used together or separately.

## Dispatch

`dispatch(router, input)` uses the same routing functions as webhooks. It
returns registered run IDs after all starts initiated by the router settle.
It does not wait for workflow completion. Errors reject dispatch; registered
runs are not rolled back. A retained routing context cannot start runs after
its router returns. Await each dispatch.

Source inputs are adapter-owned typed values. HTTP JSON parsing and webhook
schema validation do not apply. Validate external input in the adapter.
Workflows still validate their declared task schemas.

A router can keep the run handle that `ctx.start` returns, for example in
`state`, and send later events to that run. See
[webhook routing](webhook-routing.md#keep-a-run-and-send-it-messages).

Runs appear in the console with their source name. Their records have
`source: "source"` and `sourceName`. Existing HTTP run records remain readable.

## Lifecycle and reload

`runling serve` loads configuration once. Use `runling serve --watch` to reload
configuration and sources when project files change. Without this flag, restart
the server to load changes. Sources still start eagerly in both modes.

Development and installed `runling serve` start sources before any HTTP
request. Source functions must wait for cancellation and release their
resources before returning. Shutdown waits for cleanup and pending dispatches,
then cancels active workflows and flushes their final run records. Workflows
must honor their cancellation signal. Repeated `SIGINT` or `SIGTERM` signals
during cleanup do not interrupt the journal flush. Forced termination cannot
provide this guarantee.

A valid config reload aborts old sources and waits for cleanup before starting
replacements. Invalid config leaves current sources running. A source exception
logs `source.failed` without the exception content and leaves the source stopped
until the next valid reload. Sources own transport retry policy.

The `state` map survives reloads for the same source name. Use it for active
inboxes and checkpoints. Removing or renaming a source discards its state.
State is never written to disk. Scope it by connection identity. Maintain
compatible state shapes across code reloads; restart after incompatible changes.

Active workflows continue across reloads. If later deliveries must reach an
active run, keep its run handle in `state`. New runs use the updated code. A
process restart loses source state and queued messages. Registered roots can
recover from [workflow state](workflow-state.md). Sources start after recovery and
can use `ctx.runs()` to rebuild routing from each live run's original input and
handle. Runling provides no durable inbox. Batch queue deduplication is process-local.

For finite roots, use `ctx.runs({ includeFinished: true })` to find retained
workflows, including after restart. Select by application identity in their
latest input, then call `routerContext.start(task, { input, runId: previous.id })`
inside `dispatch`. Omit `runId` when there is no retained workflow. This ends each
execution after its work without keeping a root waiting for future events.
Only one execution can be active per run; sources own serialization.

## Keyed batching

Use `BatchQueue` from `runling` to collect input before executing work.
Supply `key(item)`, `id(item)`, `delay(items)`, `run(items)`, and
`failed(error)` callbacks, plus positive `maxKeys` and `maxItems` limits.
The queue runs one batch at a time per key and permits separate keys to run
concurrently. It does not create tasks or choose a workflow lifecycle.

`accept(item)` returns `accepted`, `duplicate`, `full`, or `closed`.
Capacity includes running and queued items. Retain input outside the queue
when admission returns `full`. Duplicate detection is bounded and process-local.
Failed items can be admitted again; the source decides whether retry is safe.

`touch(predicate)` extends the quiet period for matching queued batches.
`has(key)` includes active work; `queued(key)` exposes queued items for
read-only policy decisions. Closing the queue discards queued items and waits
for active callbacks. Signal cancellation first when those callbacks must stop.

## Long-lived root tasks

A root task can wait for several submissions and start child tasks for them.
It uses the same task, messaging, cancellation, and state APIs as any other
workflow. The application decides how sources find or start a root, what work
they admit, and when the root ends. The parent retains its child handles and
awaits their cleanup. A batch queue can own serial execution where required;
the root need not add another serial queue.

Use source-owned run handles and `ctx.runs()` to route to active or recovered
roots. Scope routing keys to the application's identity. Recovery restores
root state, not pending messages or child execution. See
[task invocations](task-channels.md) and [workflow state](workflow-state.md).

## Host in your application

```ts
import { serve } from 'runling/server';
import { defineWebConfig } from 'runling/web';

const server = await serve({
  config: defineWebConfig({ sources: { messages: mySource } }),
  dataDirectory: './data/runling',
  host: '127.0.0.1',
  port: 5173
});
console.info(server.url);
// At explicit application shutdown:
await server.close();
```

Build Runling's web assets before using the host. The API loads an object once,
starts sources eagerly, and installs SIGINT/SIGTERM cleanup by default. Set
`signals: false` when your application owns signals. `close()` and asynchronous
disposal are idempotent and wait for source and task cleanup. Only one host can
run in a process at a time. It uses the same Runling module as application tasks.

The default bind address is `127.0.0.1`. The console has no authentication; use
an authenticated proxy before exposing it beyond a trusted local environment.
`dataDirectory` separates journals, snapshots, and server logs by instance.
`directory` sets the base for relative paths without changing the working directory.
For file configuration, pass a path as `config`; `watch: true` enables reload.
Object configuration cannot be watched. Existing CLI commands remain supported.
