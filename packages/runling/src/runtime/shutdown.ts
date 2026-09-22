import { serverLog } from "./server-log.ts";

/** Keep signal handlers and the event loop alive until cooperative cleanup finishes.
 * Launchers can forward a signal already delivered to the process group.
 * Repeated signals must not restore Node's default termination mid-flush.
 */
export function installShutdown(cleanup: () => Promise<void>): () => void {
  let stopping: Promise<void> | undefined;
  const detach = () => {
    process.off("SIGINT", stop);
    process.off("SIGTERM", stop);
  };
  const stop = () => {
    if (stopping) return;
    const keepAlive = setInterval(() => {}, 1000);
    stopping = Promise.resolve().then(cleanup).catch(() => {
      process.exitCode = 1;
      serverLog("error", "server.shutdown_failed");
    }).finally(() => { clearInterval(keepAlive); detach(); });
  };
  process.on("SIGINT", stop);
  process.on("SIGTERM", stop);
  return detach;
}
