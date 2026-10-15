# Timeouts

Runs and task invocations have no default timeout. A run can be active for a
long time, for example while a task waits for a message or a parent waits for a
long child. The console shows "Waiting for a message" when a task waits.

## Workflow timeout

`runWorkflow()` accepts `timeout` in seconds. Omit it to wait without a
deadline.

```ts
import { runWorkflow } from 'runling';

const execution = await runWorkflow(review, { input, timeout: 3600 });
if (!execution.ok) console.error(execution.error); // "Workflow timed out after 3600 seconds"
```

When the deadline expires, Runling cancels `ctx.signal` with a `TimeoutError`.
The execution result is failed, also when the task catches the cancellation.
The timeout includes the time that tasks wait for messages.

Cancellation is cooperative. Runling waits for the task and its cleanup to
finish. Operations that ignore the signal can continue, and a timer cannot
interrupt synchronous JavaScript.

Zero expires before the task starts. Values must be finite, not negative, and
at most 2147483.647 seconds (the timer limit). Runling rounds fractions up to
whole milliseconds. An invalid value rejects with `RangeError`. Runling clears
the timer when the work finishes.

`runling run` and `runling serve` do not set a timeout. Stop a run with Ctrl-C
or with the cancel button of the console.

## Timeout for a child task

`ctx.start()` has no timeout option. To limit a child, cancel its invocation:

```ts
await using child = ctx.start(investigate, question);
const timer = setTimeout(() => child.cancel(new Error('Investigation took too long')), 600_000);
try {
  return await child.result;
} finally {
  clearTimeout(timer);
}
```

`cancel()` cancels the child and its children. It does not cancel the parent.
