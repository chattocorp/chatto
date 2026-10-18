import { bindRunlingContext, emitRunlingEvent, observeRunlingEvents } from './events.ts';
import { createChannel } from './channel.ts';
import type { TaskContext, WorkflowState } from './context.ts';
import { step } from './step.ts';
import { parseBoundary } from './schema.ts';
import type { AnyTask } from './task.ts';

/** Internal: the key of a task's validated implementation. Only `task()` sets it. */
export const implementation = Symbol.for('runling.task');

/**
 * A running task. The parent and the task exchange messages until the task returns.
 * Messages are buffered in bounded channels; the result and the cleanup have separate lifetimes.
 */
export interface Invocation<Receives, Emits, Output> extends AsyncDisposable {
  /** Identity of the invocation and of its recorded message channel. */
  readonly id: string;
  readonly status: 'running' | 'completed' | 'failed' | 'cancelled';
  /** Validate a message and queue it for the task. Does not wait for the task to read it.
   * Rejects with `ChannelFullError` when the buffer is full. */
  send(message: Receives): Promise<void>;
  /** Tell the task that no more messages come. It can still read the queued messages. */
  closeInput(): void;
  /** Messages that the task emits. One consumer only. The task's `emit` rejects with
   * `ChannelFullError` when the parent does not read them. */
  readonly updates: AsyncIterable<Emits>;
  /** The task's validated output. */
  readonly result: Promise<Output>;
  /** Cancel this task and its children, not its parent or siblings. */
  cancel(reason?: unknown): void;
  /** Cancel unfinished work and wait for cooperative cleanup. */
  [Symbol.asyncDispose](): Promise<void>;
}

export interface InvokeOptions {
  /** Host-owned root state. Never inherited by child invocations. */
  state?: WorkflowState;
  /** A direct call: nobody sends messages or reads updates. Updates are recorded, then dropped. */
  direct?: boolean;
}

/** Start a task as a child of `parent`. `ctx.start` and direct task calls both use this. */
export function invoke(
  parent: TaskContext<any, any>,
  task: AnyTask,
  input: unknown,
  { direct = false, state = {} }: InvokeOptions = {}
): Invocation<unknown, unknown, unknown> {
  // Parent cancellation flows down; cancellation through this handle stays local.
  const controller = new AbortController();
  const signal = AbortSignal.any([parent.signal, controller.signal]);
  const channelId = crypto.randomUUID();
  const record = bindRunlingContext(emitRunlingEvent);
  let status: Invocation<unknown, unknown, unknown>['status'] = 'running';

  function messages(direction: 'input' | 'update') {
    // A reader waits for `ready`, so the journal records a message before its read.
    type Envelope = { id: string; value: unknown; ready: Promise<void> };
    const channel = createChannel<Envelope>({ signal });
    const schema = direction === 'input' ? task.receives : task.emits;
    let unread = 0;

    return {
      channel,
      async send(value: unknown): Promise<void> {
        const parsed = await parseBoundary(
          task.name,
          direction === 'input' ? 'message' : 'update',
          schema,
          value
        );
        const id = crypto.randomUUID();
        const sent = () =>
          record({ type: 'message.sent', id, channelId, direction, payload: describe(parsed) });
        // A direct call has no reader for updates: record them, then drop them.
        if (direct && direction === 'update') return sent();
        const envelope: Envelope = { id, value: parsed, ready: Promise.resolve() };
        // Count before sending: a waiting reader resumes as soon as the channel accepts it.
        unread++;
        envelope.ready = channel.send(envelope).then(sent, (error: unknown) => {
          unread--;
          throw error;
        });
        return envelope.ready;
      },
      [Symbol.asyncIterator](): AsyncIterator<unknown> {
        const reader = channel[Symbol.asyncIterator]();
        return {
          async next() {
            // Record a wait when the task asks for a message before one is queued. The event
            // belongs to the reading task, so it is emitted in the reader's context.
            const waiting =
              direction === 'input' && unread === 0
                ? { id: crypto.randomUUID(), startedAt: performance.now() }
                : undefined;
            if (waiting) emitRunlingEvent({ type: 'wait.started', id: waiting.id, channelId });
            let item: IteratorResult<Envelope>;
            try {
              item = await reader.next();
            } finally {
              if (waiting)
                emitRunlingEvent({
                  type: 'wait.finished',
                  id: waiting.id,
                  status: unread > 0 ? 'received' : 'closed',
                  durationMs: performance.now() - waiting.startedAt
                });
            }
            if (item.done) return { done: true, value: undefined };
            unread--;
            await item.value.ready;
            record({ type: 'message.read', id: item.value.id });
            return { done: false, value: item.value.value };
          },
          async return() {
            await reader.return?.();
            return { done: true, value: undefined };
          }
        };
      }
    };
  }

  const incoming = messages('input');
  const outgoing = messages('update');
  if (direct) incoming.channel.close();
  const ctx: TaskContext<unknown, unknown> = {
    ...parent,
    state,
    signal,
    inbox: { [Symbol.asyncIterator]: () => incoming[Symbol.asyncIterator]() },
    emit: outgoing.send
  };

  // Settle the handle on cancellation even if the task ignores its signal.
  // The task's underlying work still needs to cooperate to stop side effects.
  let rejectAbort!: (reason: unknown) => void;
  const aborted = new Promise<never>((_resolve, reject) => {
    rejectAbort = reject;
  });
  const onAbort = () => rejectAbort(signal.reason);
  signal.addEventListener('abort', onAbort, { once: true });

  // Start synchronously, preserving the caller's step and event scope.
  let work: Promise<unknown>;
  try {
    signal.throwIfAborted();
    let linked = false;
    work = Promise.resolve(
      observeRunlingEvents(
        (event) => {
          if (!linked && event.type === 'step.started') {
            linked = true;
            record({ type: 'task.linked', channelId, taskId: event.id });
          }
        },
        () => step(task.name, () => task[implementation](ctx, input))
      )
    );
  } catch (error) {
    work = Promise.reject(error);
  }

  const result = Promise.race([work, aborted])
    .then(
      (value) => {
        // Cancellation can arrive between task return and promise settlement.
        signal.throwIfAborted();
        incoming.channel.close();
        outgoing.channel.close();
        status = 'completed';
        return value;
      },
      (error) => {
        status = signal.aborted ? 'cancelled' : 'failed';
        incoming.channel.fail(error);
        outgoing.channel.fail(error);
        throw error;
      }
    )
    .finally(() => signal.removeEventListener('abort', onAbort));

  // Observe rejection now: the parent may read updates before it awaits the result.
  void result.catch(() => {});

  const settled = work.then(
    () => result.then(noop, noop),
    () => result.then(noop, noop)
  );
  const cancel = (reason: unknown = new Error('Task cancelled')) => {
    if (status === 'running') controller.abort(reason);
  };

  return {
    id: channelId,
    get status() {
      return status === 'running' && signal.aborted ? 'cancelled' : status;
    },
    send: incoming.send,
    closeInput: () => incoming.channel.close(),
    updates: { [Symbol.asyncIterator]: () => outgoing[Symbol.asyncIterator]() },
    result,
    cancel,
    async [Symbol.asyncDispose]() {
      cancel();
      await settled;
    }
  };
}

const noop = () => {};

/** A message as journal text, bounded. */
function describe(value: unknown): string {
  let payload: string;
  try {
    payload = (typeof value === 'string' ? value : JSON.stringify(value)) ?? String(value);
  } catch {
    payload = '[Value cannot be represented as JSON]';
  }
  return payload.length > 16_000 ? `${payload.slice(0, 16_000)}… [truncated]` : payload;
}
