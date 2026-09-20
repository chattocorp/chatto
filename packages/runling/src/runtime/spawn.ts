import {
  bindRunlingContext,
  emitRunlingEvent,
  observeRunlingEvents,
} from "./events.ts";
import { observeMessageReceipt } from "./message-observation.ts";
import { createChannel } from "./channel.ts";
import type { WorkflowContext } from "./context.ts";

export interface TaskHandle<Incoming, Update, Result> {
  /** Queue input without waiting for the child to process it. */
  send(value: Incoming): Promise<void>;

  readonly updates: AsyncIterable<Update>;
  readonly result: Promise<Result>;

  /** Let the child finish reading its buffered input. */
  closeInput(): void;

  /** Cancel this child without cancelling its parent or siblings. */
  cancel(reason?: unknown): void;
}

/** Start a child with its own bounded input/output channels and cancellation. */
export function spawn<Incoming, Update, Args extends unknown[], Result>(
  parent: WorkflowContext<any, any>,
  run: (ctx: WorkflowContext<Incoming, Update>, ...args: Args) => Result,
  ...args: Args
): TaskHandle<Incoming, Update, Awaited<Result>> {
  // Parent cancellation flows down; cancellation through this handle stays local.
  const controller = new AbortController();
  const signal = AbortSignal.any([parent.signal, controller.signal]);

  const channelId = crypto.randomUUID();
  const record = bindRunlingContext(emitRunlingEvent);

  function messages<T>(direction: "input" | "update") {
    type Envelope = { id: string; value: T; ready: Promise<void> };
    const channel = createChannel<Envelope>({ signal });

    return {
      close: channel.close,
      fail: channel.fail,
      send(value: T): Promise<void> {
        const id = crypto.randomUUID();
        let payload: string;
        try {
          payload =
            (typeof value === "string" ? value : JSON.stringify(value)) ??
            String(value);
        } catch {
          payload = "[Value cannot be represented as JSON]";
        }
        const envelope: Envelope = { id, value, ready: Promise.resolve() };
        envelope.ready = channel.send(envelope).then(() => {
          record({
            type: "message.sent",
            id,
            channelId,
            direction,
            payload:
              payload.length > 16000
                ? payload.slice(0, 16000) + "… [truncated]"
                : payload,
          });
        });
        return envelope.ready;
      },
      [Symbol.asyncIterator]() {
        const reader = channel[Symbol.asyncIterator]();
        return {
          async next(): Promise<IteratorResult<T>> {
            const item = await reader.next();
            if (item.done) return { done: true, value: undefined };

            await item.value.ready;
            const result = { done: false as const, value: item.value.value };
            record({ type: "message.read", id: item.value.id });
            observeMessageReceipt(result, (consumed) => {
              record({ type: "message.receipt", id: item.value.id, consumed });
            });
            return result;
          },
          return: async (): Promise<IteratorResult<T>> => {
            await reader.return?.();
            return { done: true, value: undefined };
          },
        };
      },
    };
  }

  const incoming = messages<Incoming>("input");
  const outgoing = messages<Update>("update");
  const ctx: WorkflowContext<Incoming, Update> = {
    ...parent,
    signal,
    inbox: incoming,
    emit: outgoing.send,
  };

  // Settle the handle on cancellation even if the task ignores its signal.
  // The task's underlying work still needs to cooperate to stop side effects.
  let rejectAbort!: (reason: unknown) => void;
  const aborted = new Promise<never>((_resolve, reject) => {
    rejectAbort = reject;
  });
  const onAbort = () => rejectAbort(signal.reason);
  signal.addEventListener("abort", onAbort, { once: true });

  // Start synchronously, preserving the caller's task/event scope.
  let work: Promise<Awaited<Result>>;

  try {
    signal.throwIfAborted();
    let linked = false;
    work = Promise.resolve(
      observeRunlingEvents(
        (event) => {
          if (!linked && event.type === "step.started") {
            linked = true;
            record({ type: "task.linked", channelId, taskId: event.id });
          }
        },
        () => run(ctx, ...args),
      ),
    );
  } catch (error) {
    work = Promise.reject(error);
  }

  const result = Promise.race([work, aborted])
    .then(
      (value) => {
        // Cancellation can arrive between task return and promise settlement.
        signal.throwIfAborted();
        incoming.close();
        outgoing.close();

        return value;
      },
      (error) => {
        incoming.fail(error);
        outgoing.fail(error);

        throw error;
      },
    )
    .finally(() => signal.removeEventListener("abort", onAbort));

  // Observe rejection now: the parent may read updates before awaiting result.
  void result.catch(() => {});

  return {
    send: incoming.send,
    updates: outgoing,
    result,
    closeInput: incoming.close,

    cancel(reason = new Error("Task cancelled")) {
      controller.abort(reason);
    },
  };
}
