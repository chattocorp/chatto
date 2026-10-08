/** One message send identity and prepared request, retained across explicit retries. */
import type { RequestOptions } from './types.js';

/** Maximum retry period from the start of the first message-create attempt. */
export const MESSAGE_SEND_RETRY_WINDOW_MS = 30 * 60 * 1000;

/** A send whose duplicate-protection window has ended. Check its result before a new send. */
export class MessageSendExpiredError extends Error {
  constructor() {
    super('The message retry window has ended. Check the original send before sending again.');
    this.name = 'MessageSendExpiredError';
  }
}

/**
 * One logical message send. Retain this object and call `send` again after a
 * lost response. Concurrent calls share the first call's attempt and signal.
 * The prepared request stays in memory; hosts own recovery across a restart.
 */
export interface MessageSendOperation<Result> {
  readonly idempotencyKey: string;
  send(options?: RequestOptions): Promise<Result>;
}

/**
 * Allocate a send key once, prepare once after success, and preserve both on
 * retry. Preparation failures can be tried again. Each send reaches the server
 * so its current authorization and message state govern the returned result.
 * This requires a server that implements message-create idempotency.
 */
export function createMessageSend<Request, Result>({
  idempotencyKey = crypto.randomUUID(),
  prepare,
  post,
  assertScope
}: {
  idempotencyKey?: string;
  prepare: (key: string) => Promise<Request>;
  post: (request: Request, options: RequestOptions) => Promise<Result>;
  assertScope?: () => void;
}): MessageSendOperation<Result> {
  let prepared: Promise<Request> | undefined;
  let inFlight: Promise<Result> | undefined;
  let startedAt: { wall: number; monotonic: number } | undefined;
  const expired = () =>
    startedAt !== undefined &&
    (Date.now() - startedAt.wall >= MESSAGE_SEND_RETRY_WINDOW_MS ||
      performance.now() - startedAt.monotonic >= MESSAGE_SEND_RETRY_WINDOW_MS);

  return {
    idempotencyKey,
    send(options = {}) {
      try {
        options.signal?.throwIfAborted();
        assertScope?.();
      } catch (error) {
        return Promise.reject(error);
      }
      if (expired()) {
        return Promise.reject(new MessageSendExpiredError());
      }
      if (inFlight) return inFlight;
      const attempt = async () => {
        prepared ??= prepare(idempotencyKey).catch((error: unknown) => {
          prepared = undefined;
          throw error;
        });
        const request = await prepared;
        options.signal?.throwIfAborted();
        assertScope?.();
        // A browser can resume this continuation after the window has ended.
        // The monotonic clock also prevents wall-clock rollback from extending it.
        if (expired()) throw new MessageSendExpiredError();
        startedAt ??= { wall: Date.now(), monotonic: performance.now() };
        return post(request, options);
      };
      inFlight = attempt().finally(() => {
        inFlight = undefined;
      });
      return inFlight;
    }
  };
}
