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
process restart loses run handles, messages in queues, and checkpoints. Runling
provides no durable inbox or automatic deduplication.
