/** Collect incoming messages in fixed windows while the consumer can be busy. */
import { createNotifier } from 'runling';
import type { IncomingMessage } from '../chatto/message.ts';

export const MESSAGE_BATCH_MS = 3_000;

/** One reader drains the task inbox. The first queued message starts the window;
 * later messages do not extend it. Closing input flushes the last window, while
 * cancellation discards it. The owning task must close or cancel its inbox. */
export function messageBatches(
  input: AsyncIterable<IncomingMessage>,
  signal: AbortSignal,
  initial: IncomingMessage[] = [],
  windowMs = MESSAGE_BATCH_MS
) {
  let queue = initial;
  let deadline = Date.now() + windowMs;
  let ended = false;
  let failed = false;
  let failure: unknown;
  const changed = createNotifier();
  void (async () => {
    try {
      for await (const message of input) {
        if (!queue.length) deadline = Date.now() + windowMs;
        queue.push(message);
        changed.notify();
      }
    } catch (error) {
      failed = true;
      failure = error;
    } finally {
      ended = true;
      changed.notify();
    }
  })();
  return {
    /** Reconciliation must not read past a window that has not been delivered. */
    get pending() {
      return queue.length > 0;
    },
    /** Intake must not discard an addressed request that arrived during classification. */
    get addressedPending() {
      return queue.some((message) => !message.trigger);
    },
    batches: (async function* (): AsyncGenerator<IncomingMessage[]> {
      while (true) {
        signal.throwIfAborted();
        if (failed) throw failure;
        if (!queue.length) {
          if (ended) return;
          await changed.wait({ signal });
          continue;
        }
        const remaining = deadline - Date.now();
        if (remaining > 0) {
          await changed.wait({ signal, timeoutMs: remaining });
          continue;
        }
        const batch = queue;
        queue = [];
        yield batch;
      }
    })()
  };
}
