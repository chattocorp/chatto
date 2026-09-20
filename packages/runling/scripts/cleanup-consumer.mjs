import { execFile } from "node:child_process";
import { rm } from "node:fs/promises";
import { promisify } from "node:util";

const exec = promisify(execFile);

// The npm launcher can exit before its server descendants close their pipes.
export async function stopConsumer(server, timeout = 5000) {
  if (!server?.pid) return;
  let timer;
  let onClose;
  const closed = new Promise((resolve) => {
    onClose = resolve;
    server.once("close", onClose);
  });
  const signalGroup = (signal) => {
    try {
      process.kill(-server.pid, signal);
    } catch (error) {
      if (error.code !== "ESRCH") throw error;
    }
  };
  try {
    if (process.platform === "win32") {
      // Killing npm alone leaves the actual server running on Windows.
      if (server.exitCode === null && server.signalCode === null) {
        await exec("taskkill", ["/pid", String(server.pid), "/T", "/F"]);
      }
    } else {
      signalGroup("SIGTERM");
    }
    await Promise.race([
      closed,
      new Promise((resolve) => { timer = setTimeout(resolve, timeout); }),
    ]);
    // Also stop descendants that outlive npm or ignore graceful shutdown.
    if (process.platform !== "win32") signalGroup("SIGKILL");
  } finally {
    clearTimeout(timer);
    server.removeListener("close", onClose);
  }
}

export async function cleanupConsumer(directory, server) {
  await stopConsumer(server);
  // Recursive rm does not retry ENOTEMPTY unless maxRetries is set.
  await rm(directory, {
    recursive: true,
    force: true,
    maxRetries: 10,
    retryDelay: 100,
  });
}
