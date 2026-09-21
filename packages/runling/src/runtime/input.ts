import { createTimeout, TimeoutError } from "./timeout.ts";
import type { WorkflowContext } from "./context.ts";
import { bindRunlingContext, emitRunlingEvent } from "./events.ts";
import { log, logInput } from "./log.ts";

export interface InputOptions {
  /** Maximum elapsed time in seconds. Omit for no deadline. */
  timeout?: number;
  defaultValue?: string;
  signal?: AbortSignal;
}

export interface InputRequest extends InputOptions {
  id: string;
  message: string;
}

export type InputHandler = (request: InputRequest) => Promise<string>;
export type Input = (
  message: string,
  options?: InputOptions,
) => Promise<string>;

export class InputUnavailableError extends Error {
  constructor(message: string) {
    super(`Workflow requested input, but this host cannot provide it: ${message}`);
    this.name = "InputUnavailableError";
  }
}

export const createInput = (handleInput?: InputHandler): Input =>
  async (message, options = {}) => {
    const deadline = createTimeout(options.timeout, "Input");
    const signals = [options.signal, deadline.signal].filter((signal): signal is AbortSignal => !!signal);
    const signal = signals.length > 1 ? AbortSignal.any(signals) : signals[0];
    let abort: (() => void) | undefined;
    const id = crypto.randomUUID();
    const startedAt = performance.now();
    const request = { id, message, ...options, signal };
    const emit = bindRunlingContext(emitRunlingEvent);

    try {
      emit({
        type: "input.requested",
        id,
        message,
        defaultValue: options.defaultValue,
      });
      logInput(id, "info", `${log.highlight("Asking", "#f59f00")} ${message}`);
      signal?.throwIfAborted();
      if (handleInput === undefined) throw new InputUnavailableError(message);

      const value = await new Promise<string>((resolve, reject) => {
        if (signal) {
          abort = () => reject(signal.reason);
          signal.addEventListener("abort", abort, { once: true });
        }
        Promise.resolve().then(() => {
          signal?.throwIfAborted();
          return handleInput(request);
        }).then(resolve, reject);
      });
      signal?.throwIfAborted();
      if (typeof value !== "string") {
        throw new TypeError("An input handler must return a string");
      }

      emit({
        type: "input.finished",
        id,
        status: "answered",
        value,
        durationMs: performance.now() - startedAt,
      });
      logInput(id, "success", `${log.highlight("Answered")} ${value}`);
      return value;
    } catch (error) {
      emit({
        type: "input.finished",
        id,
        status: "failed",
        ...(signal?.aborted ? { reason: signal.reason instanceof TimeoutError ? "timeout" as const : "cancelled" as const } : {}),
        durationMs: performance.now() - startedAt,
      });
      logInput(id, "error", `${log.highlight("Input failed", "crimson")} ${message}`);
      throw error;
    } finally {
      deadline.dispose();
      if (abort) signal?.removeEventListener("abort", abort);
    }
  };

/** Ask through the handler supplied by this context. */
export const input = (
  ctx: WorkflowContext<unknown, never>,
  message: string,
  options: InputOptions = {},
): Promise<string> => createInput(ctx.onInput)(message, {
  ...options,
  signal: options.signal
    ? AbortSignal.any([ctx.signal, options.signal])
    : ctx.signal,
});
