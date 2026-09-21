# Timeouts

Use `timeout` in seconds. Omit it to wait without a deadline.

```ts
import { input, runWorkflow, task, TimeoutError } from "runling";

const review = task(async ctx => {
  try {
    return await input(ctx, "Approve the change?", { timeout: 300 });
  } catch (error) {
    if (error instanceof TimeoutError) return "No answer received";
    throw error;
  }
});

await runWorkflow(review, {
  input: undefined,
  timeout: 3600,
  onInput: async request => {
    // The host supplies the answer and uses request.signal for cancellation.
    return "Approved";
  },
});
```

An input timeout rejects only that question. Concurrent questions have separate
deadlines. The host receives a signal that combines the question deadline,
workflow cancellation, and any signal supplied to `input()`.

A workflow timeout includes time spent waiting for input. It cancels `ctx.signal`
and produces a failed execution result, even if the task catches the cancellation.
Cancellation is cooperative: Runling waits for the workflow to settle. Operations
that ignore the signal can continue; a timer cannot interrupt synchronous JavaScript.

Zero expires before user work starts. Values must be finite, non-negative, and at
most 2147483.647 seconds (the timer limit). Fractions round up to milliseconds.
Invalid options reject with `RangeError`. Timers are cleared when work settles.

The web console shows the number of pending inputs. Each question has its own
timeline entry with its wait duration and answer, timeout, or cancellation.
Other branches can continue while inputs are pending. Hosts still choose how to
ask questions and route replies; this feature adds no restart recovery.
