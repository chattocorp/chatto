import { appendFileSync, mkdirSync, renameSync, statSync } from "node:fs";
import { dirname, resolve } from "node:path";

export function serverLogPath(): string {
  return resolve(dirname(resolve(process.env.RUNLING_WEB_CONFIG ?? "runling.config.ts")), ".runling/logs/server.jsonl");
}

/** Low-volume server diagnostics; workflow output stays in the run journal. */
export function serverLog(
  level: "info" | "warn" | "error",
  event: string,
  fields: Record<string, unknown> = {},
): void {
  let line: string;
  try {
    line = JSON.stringify({ ...fields, time: new Date().toISOString(), level, event },
      (_key, value) => value instanceof Error
        ? { name: value.name, message: value.message, stack: value.stack }
        : value);
  } catch {
    line = JSON.stringify({ time: new Date().toISOString(), level, event, message: "Log details could not be serialized" });
  }
  console[level](line);
  try {
    const path = serverLogPath();
    mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
    // Keep one previous file, so a long-running server cannot fill the disk.
    try {
      if (statSync(path).size >= 10 * 1024 * 1024) renameSync(path, `${path}.1`);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    }
    appendFileSync(path, `${line}\n`, { mode: 0o600 });
  } catch (error) {
    // Diagnostics must not stop a request or workflow when the disk is unavailable.
    console.error("Cannot write Runling server log:", error);
  }
}
