import { createTimeout } from "./timeout.ts";
import { createObservedWorkflowContext, type WorkflowContext, type TextHandler } from "./context.ts";
import { withExecutionServices } from "./execution.ts";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import type { RunOptions } from "./cli.ts";
import {
  observeRunlingEvents,
  emitRunlingEvent,
  bindRunlingContext,
  type RunlingEventListener,
} from "./events.ts";
import type { InputHandler } from "./input.ts";
import { log } from "./log.ts";
import { renderMarkdown } from "./markdown.ts";
import {
  type JsonValue,
  type WorkflowResult,
  type WorkflowReturn,
} from "./runtime.ts";
import { TuiReporter } from "./tui.ts";
import {
  isTask,
  type TaskFunction,
} from "./workflow.ts";
import {
  formatTokenUsage,
  type TokenUsage,
  totalTokens,
} from "./usage.ts";

export function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${Math.round(ms)}ms`;
  }

  const tenths = Math.round(ms / 100);
  if (tenths < 600) {
    return `${(tenths / 10).toFixed(1)}s`;
  }

  // Round before splitting the duration so seconds cannot render as `60s`.
  const totalSeconds = Math.round(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const parts: string[] = [];

  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (seconds > 0) parts.push(`${seconds}s`);

  return parts.join("") || "0s";
}

export interface WorkflowExecution<Output = unknown> {
  durationMs: number;
  usage: TokenUsage;
  output: Output | null;
  result: WorkflowResult | null;
  error: string | null;
  ok: boolean;
}

export interface TerminalCapabilities {
  isTTY?: boolean;
  columns?: number;
}

export interface ExecutionOptions {
  json?: boolean;
  presentation?: "log" | "tui";
  terminal?: TerminalCapabilities;
  title?: string;
}

export interface RunWorkflowOptions<Input = unknown> {
  /** Maximum elapsed time in seconds, including input waits. Cancellation is cooperative. */
  timeout?: number;
  signal?: AbortSignal;
  input: Input;
  verbose?: boolean;
  onInput?: InputHandler;
  onText?: TextHandler;
  onEvent?: RunlingEventListener;
}

export function formatWorkflowDetails(
  details: string,
  terminal: TerminalCapabilities = process.stdout,
): string {
  return terminal.isTTY
    ? renderMarkdown(details, terminal.columns ?? 80)
    : details;
}

function isJsonValue(
  value: unknown,
  ancestors = new Set<object>(),
): value is JsonValue {
  if (
    value === null ||
    typeof value === "string" ||
    typeof value === "boolean"
  ) {
    return true;
  }
  if (typeof value === "number") {
    return Number.isFinite(value);
  }
  if (typeof value !== "object" || ancestors.has(value)) {
    return false;
  }

  const prototype = Object.getPrototypeOf(value);
  if (
    !Array.isArray(value) &&
    prototype !== Object.prototype &&
    prototype !== null
  ) {
    return false;
  }

  ancestors.add(value);
  // JSON omits undefined object properties. Array entries must remain valid values.
  const valid = Object.values(value).every((entry) =>
    (entry === undefined && !Array.isArray(value)) || isJsonValue(entry, ancestors),
  );
  ancestors.delete(value);
  return valid;
}

export function normalizeWorkflowResult(
  value: unknown,
): WorkflowResult | null {
  if (value === undefined) {
    return null;
  }
  if (!isJsonValue(value)) {
    throw new Error("Workflow output must be valid JSON");
  }
  if (typeof value === "string") {
    return { summary: value };
  }
  if (typeof value === "object" && value !== null) {
    const candidate = value as Record<string, unknown>;
    if (
      typeof candidate.summary === "string" &&
      (candidate.details === undefined ||
        typeof candidate.details === "string") &&
      (candidate.outputs === undefined ||
        (isJsonValue(candidate.outputs) && !Array.isArray(candidate.outputs)))
    ) {
      return candidate as unknown as WorkflowResult;
    }
  }
  return { summary: "Workflow completed", outputs: { value } };
}

export async function executeWorkflow(
  run: (ctx: WorkflowContext) => Promise<WorkflowReturn> | WorkflowReturn,
  options: ExecutionOptions = {},
): Promise<WorkflowExecution> {
  return reportExecution(
    ctx => log.indented(() => run(ctx)),
    options,
  );
}

/** Run a workflow without assuming a terminal, printing, or changing process state. */
export async function runWorkflow<Input, Output>(
  run: (ctx: WorkflowContext, input: Input) => Output,
  { input, verbose = false, onInput, onText, onEvent = () => {}, timeout, signal }: RunWorkflowOptions<Input>,
): Promise<WorkflowExecution<Awaited<Output>>> {
  return withExecutionServices({ verbose }, () =>
    observeRunlingEvents(onEvent, () =>
      log.withDestination("silent", () =>
        captureExecution(ctx => log.indented(() => run(ctx, input)), onInput, timeout, signal, onText),
      ),
    ),
  );
}

async function reportExecution(
  run: (ctx: WorkflowContext) => Promise<unknown> | unknown,
  {
    json = false,
    presentation = "log",
    terminal = process.stdout,
    title = "Workflow",
  }: ExecutionOptions = {},
): Promise<WorkflowExecution> {
  const reporter = presentation === "tui" ? new TuiReporter(title) : undefined;

  try {
    reporter?.start();
    const execution = await observeRunlingEvents(
      reporter?.handle ?? (() => {}),
      () =>
        log.withDestination(
          presentation === "tui" ? "silent" : json ? "stderr" : "stdout",
          async () => {
            if (presentation === "log") log.info("Runling starting");
            const execution = await captureExecution(run, reporter?.input);

            if (execution.error !== null) {
              log.error(execution.error);
              process.exitCode = 1;
            } else if (!json && execution.result !== null) {
              if (
                presentation === "log"
              ) {
                log.success(execution.result.summary);
                if (execution.result.details !== undefined) {
                  console.log(
                    `\n${formatWorkflowDetails(execution.result.details, terminal)}\n`,
                  );
                }
              }
            }

            if (presentation === "log" && totalTokens(execution.usage) > 0) {
              log.info(`Total token usage: ${formatTokenUsage(execution.usage)}`);
            }
            if (presentation === "log") {
              log.info(`Finished in ${formatDuration(execution.durationMs)}`);
            }

            return execution;
          },
        ),
    );

    reporter?.finish(execution);
    if (json) console.log(JSON.stringify(execution));

    return execution;
  } finally {
    reporter?.stop();
  }
}

async function captureExecution<Output>(
  run: (ctx: WorkflowContext) => Promise<Output> | Output,
  onInput?: InputHandler,
  timeout?: number,
  signal?: AbortSignal,
  onText?: TextHandler,
): Promise<WorkflowExecution<Awaited<Output>>> {
  const deadline = createTimeout(timeout, "Workflow");
  const ctx = createObservedWorkflowContext(bindRunlingContext((usage: TokenUsage) =>
    emitRunlingEvent({ type: "usage.updated", usage }),
  ), signal && deadline.signal ? AbortSignal.any([signal, deadline.signal]) : signal ?? deadline.signal);
  ctx.onInput = onInput;
  ctx.onText = onText;
  const start = performance.now();
  let result: WorkflowResult | null = null;
  let output: Awaited<Output> | null = null;
  let error: string | null = null;

  try {
    ctx.signal.throwIfAborted();
    const value = await run(ctx);
    ctx.signal.throwIfAborted();
    result = normalizeWorkflowResult(value);
    output = value ?? null;
  } catch (cause) {
    const failure = ctx.signal.aborted ? ctx.signal.reason : cause;
    error = failure instanceof Error ? failure.message : String(failure);
  } finally {
    deadline.dispose();
  }

  const usage = { ...ctx.usage };
  const durationMs = performance.now() - start;

  return {
    durationMs,
    usage,
    output,
    result,
    error,
    ok: error === null,
  };
}

export async function loadWorkflow(path: string): Promise<TaskFunction> {
  const resolvedPath = resolve(path);
  const module = await import(/* @vite-ignore */ pathToFileURL(resolvedPath).href);
  if (!isTask(module.default)) {
    throw new Error(
      `Workflow ${resolvedPath} must have a default task export`,
    );
  }
  return module.default;
}

export async function runRunling(
  workflowPath: string,
  prompt: string,
  options: RunOptions,
) {
  const { json, verbose } = options;
  const presentation = shouldUseTui(options) ? "tui" : "log";
  await reportExecution(
    async ctx => {
      const run = await loadWorkflow(workflowPath);
      const input = options.input === undefined ? prompt : JSON.parse(options.input);
      return withExecutionServices({ verbose }, () =>
        log.indented(() => run(ctx, input)),
      );
    },
    { json, presentation, title: workflowPath },
  );
}

export function shouldUseTui(
  options: RunOptions,
  terminal: { stdinIsTTY?: boolean; stdoutIsTTY?: boolean } = {
    stdinIsTTY: process.stdin.isTTY,
    stdoutIsTTY: process.stdout.isTTY,
  },
): boolean {
  return (
    terminal.stdinIsTTY === true &&
    terminal.stdoutIsTTY === true &&
    !options.json &&
    !options.log &&
    !options.verbose
  );
}
