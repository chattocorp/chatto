# Webhook routing

`runling.config.ts` maps webhook names to routing functions. A router receives
an explicit routing context and the JSON request body. It can start any number
of workflows, send messages to a run that it started before, or do nothing.

```ts
import { defineWebConfig, startWorkflow } from 'runling/web';
import { task, Type } from 'runling';

const echo = task(
  { name: 'Echo', input: Type.String(), result: Type.String() },
  async (_ctx, message) => message
);

export default defineWebConfig({
  webhooks: {
    echo: startWorkflow(echo)
  }
});
```

`startWorkflow(task)` returns a router that always starts that task. This is
separate from `runWorkflow()` in `runling`, which executes and awaits a workflow.

## Custom routes

Call `ctx.start(task, { input })` to register a run. It resolves with a run
handle when the host has saved the initial record of the run. It does not wait
for the workflow to finish. The host owns execution and run history. Each call
creates a separate run:

```ts
import { defineWebConfig } from 'runling/web';
import { task, Type } from 'runling';

const echo = task(
  {
    name: 'Echo',
    input: Type.String(),
    result: Type.String()
  },
  async (_ctx, value) => value
);

export default defineWebConfig({
  webhooks: {
    greetings: async (ctx, payload: unknown) => {
      if (typeof payload !== 'string') return;
      await Promise.all([
        ctx.start(echo, { input: payload }),
        ctx.start(echo, { input: `Hello, ${payload}` })
      ]);
    }
  }
});
```

Router return values are ignored; return `ctx.start(...)` directly if convenient.
Await all routing work before returning. The host observes starts already
initiated by the router, including calls it did not await. A retained routing
context cannot start runs after the router returns. Workflow completion is
independent of the HTTP connection.

Runling does not add a global message registry or deduplicate deliveries
automatically.

## Reactivate a retained run

Call `ctx.start(task, { input, runId })` to execute a finished workflow again with
fresh input. Register the task in `workflows`. The retained run must belong to
the same source kind, route name, and registered task. A missing run, invalid
input, or an active execution rejects registration. The router owns queuing and
authorization; do not use untrusted run IDs without checking application access.

The new execution keeps the ID, reference, saved state, files, and journal.
It does not replay prior work. Retain the new handle: an old handle cannot steer
or cancel the new execution. See [workflow state](workflow-state.md#reactivate-a-finished-workflow)
for persistence and retention rules.

## Keep a run and send it messages

The run handle has these members:

- `id`: the ID of the run.
- `steer(message)`: validate a message with the `steering` schema of the
  top-level task and put it in the queue of the task. It rejects when the run is
  not running, does not accept more messages, or has too many unread messages.
- `closeSteering()`: tell the top-level task that no more messages come.
- `cancel()`: cancel the run. Its tasks get the abort signal and do their
  cleanup.
- `finished`: a promise that resolves when the run finishes. It also resolves
  when the run fails or is cancelled.

A router can keep handles after it returns, for example one run for each chat
thread. The router owns this state:

```ts
import type { RunHandle, WebhookRouter } from 'runling/web';

// `conversation` is a task with `steering: ChatMessage`.
const threads = new Map<string, RunHandle<ChatMessage>>();

const route: WebhookRouter<ChatMessage> = async (ctx, message) => {
  const run = threads.get(message.threadId);
  if (run) {
    try {
      return await run.steer(message);
    } catch {
      // The run has finished. Start a new one.
    }
  }
  const started = await ctx.start(conversation, { input: message });
  threads.set(message.threadId, started);
  void started.finished.then(() => {
    if (threads.get(message.threadId) === started) threads.delete(message.threadId);
  });
};
```

A task cannot start a workflow run. If a task must cause a different run, it
uses an external mechanism, for example an HTTP request to a webhook route. See
[ADR-009](adr/ADR-009-task-invocations.md).

## Validation and discovery

Plain routers receive arbitrary JSON and decide how to validate and parse it.
A function can also have `input` schema metadata for boundary validation and the
web app's sample request. The host validates this schema before routing and
passes the original body; schema transforms do not replace the router payload.

```ts
import { Type } from 'runling';
import type { WebhookRouter } from 'runling/web';

const route: WebhookRouter<{ message: string }> = async (ctx, payload) => {
  // Route the validated raw message here.
};
route.input = Type.Object({ message: Type.String() });
route.label = 'Chat messages';
```

`startWorkflow(task)` copies the input and output schemas of the task. Optional
`output` metadata describes a workflow result, not the HTTP response. Declared schemas
must support JSON Schema export; tasks still parse and validate their own inputs
and outputs when executed.

## HTTP responses

Both `POST /api/webhooks/:name` and `POST /api/runs/start/:name` return `202`:

```json
{ "runs": [{ "id": "run-id" }] }
```

No new runs means `{ "runs": [] }`. Read results through
`GET /api/runs/:id` or subscribe to `/api/runs/:id/events`. Workflow failure after
registration does not change the webhook acknowledgement.

Invalid JSON or declared input returns `400`; unknown names return `404`. A
routing or registration failure returns `500` with `error` and `runs` containing
any successfully registered IDs. Already-started runs are not rolled back.
Retries can duplicate those runs unless the router implements deduplication.

To send more messages to a running run over HTTP, use
`POST /api/runs/:id/messages`. See [workflow messages](workflow-messages.md).

The web composer opens a single run directly. For multiple runs or partial
success, it shows links to the registered runs.

## Migration

Replace `{ task: root }` with `startWorkflow(root)`. Replace `{ task, route }`
with a routing function that explicitly calls `ctx.start(task, { input })` when
needed. The old `route(input, start)` signature and fixed one-run limit are gone.

`ctx.start` in a router resolves with a run handle instead of `{ id }`. The
handle keeps the `id` property.

Clients must read the `runs` array instead of `{ id }`, `{ handled: true }`, or
a synchronous `{ output }` response.
