/** A process-local wake-up signal. The caller owns the data that needs processing. */
export interface Notifier {
  /** Wake the waiter, or retain one notification. Repeated notifications coalesce. */
  notify(): void;
  /** Consume a notification (true), or reach the timeout (false). Only one wait can
   * be pending. Cancellation rejects with the signal reason without consuming a
   * retained notification. Timers and abort listeners are removed when the wait ends. */
  wait(options?: { signal?: AbortSignal; timeoutMs?: number }): Promise<boolean>;
}

/** Create a single-consumer notification signal, without a queue or task lifecycle.
 * Notifications are not durable; persist application data separately when needed. */
export function createNotifier(): Notifier {
  let pending = false;
  let wake: (() => void) | undefined;
  return {
    notify() {
      if (wake) wake();
      else pending = true;
    },
    async wait({ signal, timeoutMs } = {}) {
      signal?.throwIfAborted();
      if (wake) throw new Error('Notifier already has a pending wait');
      if (
        timeoutMs !== undefined &&
        (!Number.isFinite(timeoutMs) || timeoutMs < 0 || timeoutMs > 2_147_483_647)
      )
        throw new RangeError('timeoutMs must be between 0 and 2147483647');
      if (pending) {
        pending = false;
        return true;
      }
      if (timeoutMs === 0) return false;
      return new Promise<boolean>((resolve, reject) => {
        let timer: ReturnType<typeof setTimeout> | undefined;
        const cleanup = () => {
          clearTimeout(timer);
          signal?.removeEventListener('abort', abort);
          wake = undefined;
        };
        const abort = () => {
          cleanup();
          reject(signal!.reason);
        };
        wake = () => {
          cleanup();
          resolve(true);
        };
        signal?.addEventListener('abort', abort, { once: true });
        if (timeoutMs !== undefined)
          timer = setTimeout(() => {
            cleanup();
            resolve(false);
          }, timeoutMs);
      });
    }
  };
}
