/** A deadline expired. Timeout values are in seconds. */
export class TimeoutError extends Error {
  override readonly name = "TimeoutError";

  constructor(scope: "Input" | "Workflow", readonly timeout: number) {
    super(`${scope} timed out after ${timeout} seconds`);
  }
}

/** Validate a timeout in seconds without starting a timer. */
export function validateTimeout(timeout: number | undefined): void {
  if (timeout === undefined) {
    return;
  }

  if (!Number.isFinite(timeout) || timeout < 0 || Math.ceil(timeout * 1000) > 2_147_483_647) {
    throw new RangeError("timeout must be seconds between 0 and 2147483.647");
  }
}

/** Internal timer with explicit cleanup; no timer when timeout is omitted. */
export function createTimeout(timeout: number | undefined, scope: "Input" | "Workflow") {
  validateTimeout(timeout);
  if (timeout === undefined) return { signal: undefined, dispose() {} };
  const ms = Math.ceil(timeout * 1000);
  const controller = new AbortController();
  const expire = () => controller.abort(new TimeoutError(scope, timeout));
  if (ms === 0) expire();
  const timer = ms === 0 ? undefined : setTimeout(expire, ms);
  return { signal: controller.signal, dispose: () => clearTimeout(timer) };
}
