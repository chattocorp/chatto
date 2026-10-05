/** One best-effort presence refresh; cancellation must reach the transport. */
export type TypingUpdate = (signal: AbortSignal) => Promise<unknown>;

/**
 * Refresh typing during work without blocking it. Refreshes never overlap.
 * Completion or failure aborts an in-flight refresh and removes the timer.
 */
export async function withTyping<Result>(
  signal: AbortSignal,
  update: TypingUpdate | undefined,
  work: () => Promise<Result>
): Promise<Result> {
  if (!update) return work();
  const controller = new AbortController();
  const refreshSignal = AbortSignal.any([signal, controller.signal]);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const refresh = async () => {
    if (refreshSignal.aborted) return;
    try {
      await update(AbortSignal.any([refreshSignal, AbortSignal.timeout(2000)]));
    } catch {
      // Presence is optional and must not prevent the primary operation.
    }
    if (!refreshSignal.aborted)
      timer = setTimeout(() => {
        void refresh();
      }, 3000);
  };
  void refresh();
  try {
    return await work();
  } finally {
    controller.abort();
    clearTimeout(timer);
  }
}

/**
 * Wait for the initial best-effort update, then refresh every three seconds.
 * The returned stop function is idempotent; it prevents future updates but
 * does not cancel an update already in flight. The host owns its request timeout.
 */
export async function startTyping(update: () => Promise<unknown>): Promise<() => void> {
  const refresh = () => update().catch(() => undefined);
  await refresh();
  let stopped = false;
  let timer: ReturnType<typeof setTimeout>;
  const schedule = () => {
    timer = setTimeout(async () => {
      await refresh();
      if (!stopped) schedule();
    }, 3000);
  };
  schedule();
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}
