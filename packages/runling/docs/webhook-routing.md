# Webhook routing

`runling.config.ts` maps webhook names to routing functions. A router receives
an explicit routing context and the JSON request body. It can start any number
of workflows, send data to an existing conversation, or do nothing.

```ts
import { defineWebConfig, startWorkflow } from "runling/web";
import { task } from "runling";

const echo = task((_ctx, message: string) => message);

export default defineWebConfig({
  webhooks: {
    echo: startWorkflow(echo),
  },
});
```

`startWorkflow(task)` returns a router that always starts that task. This is
separate from `runWorkflow()` in `runling`, which executes and awaits a workflow.

## Custom routes

Call `ctx.start(task, { input })` to register a run. It returns `{ id }` once the
host has saved its initial record, without waiting for the workflow to finish.
The host owns execution and run history. Each call creates a separate run:

```ts
import { defineWebConfig } from "runling/web";
import { task, Type } from "runling";

const echo = task({
  name: "Echo",
  input: Type.String(),
  output: Type.String(),
}, (_ctx, value) => value);

export default defineWebConfig({
  webhooks: {
    greetings: async (ctx, payload: unknown) => {
      if (typeof payload !== "string") return;
      await Promise.all([
        ctx.start(echo, { input: payload }),
        ctx.start(echo, { input: `Hello, ${payload}` }),
      ]);
    },
  },
});
```

Router return values are ignored; return `ctx.start(...)` directly if convenient.
Await all routing work before returning. The host observes starts already
initiated by the router, including calls it did not await. A retained routing
context cannot start runs after the router returns. Workflow completion is
independent of the HTTP connection.

Routing state and existing-conversation delivery belong to the adapter. Runling
does not add a global message registry or deduplicate deliveries automatically.

## Validation and discovery

Plain routers receive arbitrary JSON and decide how to validate and parse it.
A function can also have `input` schema metadata for boundary validation and the
web app's sample request. The host validates this schema before routing and
passes the original body; schema transforms do not replace the router payload.

```ts
import { Type } from "runling";
import type { WebhookRouter } from "runling/web";

const route: WebhookRouter<{ message: string }> = async (ctx, payload) => {
  // Route the validated raw message here.
};
route.input = Type.Object({ message: Type.String() });
route.label = "Chat messages";
```

`startWorkflow(task)` copies the task's input and output schema metadata when
available. Ordinary tasks without schemas are also supported. Optional `output`
metadata describes a workflow result, not the HTTP response. Declared schemas
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

The web composer opens a single run directly. For multiple runs or partial
success, it shows links to the registered runs.

## Migration

Replace `{ task: root }` with `startWorkflow(root)`. Replace `{ task, route }`
with a routing function that explicitly calls `ctx.start(task, { input })` when
needed. The old `route(input, start)` signature and fixed one-run limit are gone.

Clients must read the `runs` array instead of `{ id }`, `{ handled: true }`, or
a synchronous `{ output }` response.
