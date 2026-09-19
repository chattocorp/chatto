/** Inputs for recovery of discovery and saved sessions, before realtime starts. */
export interface RecoveryRegistry {
  servers: readonly { id: string }[];
  needsRecovery(id: string): boolean;
  recoverServer(id: string): Promise<void>;
}

/** Foreground-only retries. Each server has independent backoff and one attempt at a time. */
export function startServerRecovery(registry: RecoveryRegistry): () => void {
  const attempts = new Map<string, { due: number; delay: number; running: boolean }>();
  let disposed = false;
  let paused = false;

  function tick(immediate = false) {
    if (disposed || paused || document.visibilityState === 'hidden' || !navigator.onLine) return;
    const ids = new Set(registry.servers.map((server) => server.id));
    for (const [id, attempt] of attempts) {
      if (!ids.has(id) || (!attempt.running && !registry.needsRecovery(id))) attempts.delete(id);
    }
    for (const id of ids) {
      if (!registry.needsRecovery(id)) continue;
      let attempt = attempts.get(id);
      if (!attempt) {
        attempt = { due: Date.now() + 1000, delay: 1000, running: false };
        attempts.set(id, attempt);
      }
      if (attempt.running || (!immediate && Date.now() < attempt.due)) continue;
      attempt.running = true;
      const current = attempt;
      // The registry reports failure through state. Do not log request details.
      void registry.recoverServer(id).catch(() => {}).finally(() => {
        if (disposed || attempts.get(id) !== current) return;
        current.running = false;
        current.delay = Math.min(current.delay * 2, 30_000);
        current.due = Date.now() + current.delay;
      });
    }
  }

  const retry = () => tick(true);
  const resume = () => { paused = false; retry(); };
  const pause = () => { paused = true; };
  // Capacitor dispatches document resume/pause events for native scene changes.
  document.addEventListener('resume', resume);
  document.addEventListener('pause', pause);
  document.addEventListener('visibilitychange', retry);
  window.addEventListener('online', retry);
  const timer = setInterval(tick, 1000);
  tick();

  return () => {
    disposed = true;
    clearInterval(timer);
    attempts.clear();
    document.removeEventListener('resume', resume);
    document.removeEventListener('pause', pause);
    document.removeEventListener('visibilitychange', retry);
    window.removeEventListener('online', retry);
  };
}
