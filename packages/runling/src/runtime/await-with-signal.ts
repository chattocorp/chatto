/** Wait for external work that does not accept an abort signal. */

/** Start `operation` unless already aborted, and reject with `signal.reason` on abort.
 * Removes its listener when the wait ends and observes late operation failures.
 * This only stops waiting: the operation can still finish or cause external effects.
 * The caller owns cleanup and must not assume that an interrupted write failed remotely. */
export async function awaitWithSignal<T>(
  operation: () => Promise<T>,
  signal: AbortSignal
): Promise<T> {
  signal.throwIfAborted();
  const stopped = Promise.withResolvers<never>();
  const onAbort = () => stopped.reject(signal.reason);
  signal.addEventListener('abort', onAbort, { once: true });
  try {
    // Observe synchronous throws too, including an operation that aborts then throws.
    const work = Promise.resolve().then(() => {
      signal.throwIfAborted();
      return operation();
    });
    return await Promise.race([work, stopped.promise]);
  } finally {
    signal.removeEventListener('abort', onAbort);
  }
}
