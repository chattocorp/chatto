/** Start best-effort typing updates. Stop is idempotent and cancels future refreshes. */
export async function startTyping(
  update: () => Promise<unknown>,
): Promise<() => void> {
  // Typing is optional: a failed update must not interrupt the reply.
  const refresh = () => update().catch(() => undefined);
  await refresh();

  let stopped = false;
  let timer: ReturnType<typeof setTimeout>;

  // Wait for each request before scheduling the next one to avoid overlap.
  const schedule = () => {
    timer = setTimeout(async () => {
      await refresh();

      // Stop can be called while the refresh request is still in flight.
      if (!stopped) {
        schedule();
      }
    }, 3000);
  };

  schedule();

  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}
