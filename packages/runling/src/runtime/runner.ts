import { createTimeout } from './timeout.ts';
import {
  createObservedWorkflowContext,
  type WorkflowContext,
  type RunIdentity,
  type TextHandler
} from './context.ts';
import { withExecutionServices } from './execution.ts';
import { resolve } from 'node:path';
import { createInterface } from 'node:readline/promises';
import { pathToFileURL } from 'node:url';
import type { RunOptions } from './cli.ts';
import {
  observeRunlingEvents,
  emitRunlingEvent,
  bindRunlingContext,
  type RunlingEventListener
} from './events.ts';
import type { InputHandler } from './input.ts';
import { log } from './log.ts';
import { renderMarkdown } from './markdown.ts';
import {
  createRunJournal,
  eventRecord,
  newRun,
  runJournalDirectory,
  type RunDetail,
  type RunJournal
} from './run-journal.ts';
import { relative } from 'node:path';
import {
  isJsonValue,
  type JsonValue,
  type WorkflowResult,
  type WorkflowReturn
} from './runtime.ts';
import { isTask, type TaskFunction } from './workflow.ts';
import { formatTokenUsage, type TokenUsage, totalTokens } from './usage.ts';

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

  return parts.join('') || '0s';
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
  terminal?: TerminalCapabilities;
  /** Answers workflow input requests. Without it, a request fails. */
  onInput?: InputHandler;
  /** Cancels the workflow cooperatively. */
  signal?: AbortSignal;
  /** Records every event and the final result in the run's journal. */
  journal?: { run: RunDetail; writer: RunJournal };
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
  /** The run that `ctx.run` identifies. Hosts that record runs pass it. */
  run?: RunIdentity;
}

export function formatWorkflowDetails(
  details: string,
  terminal: TerminalCapabilities = process.stdout
): string {
  return terminal.isTTY ? renderMarkdown(details, terminal.columns ?? 80) : details;
}

export function normalizeWorkflowResult(value: unknown): WorkflowResult | null {
  if (value === undefined) {
    return null;
  }
  if (!isJsonValue(value)) {
    throw new Error('Workflow output must be valid JSON');
  }
  if (typeof value === 'string') {
    return { summary: value };
  }
  if (typeof value === 'object' && value !== null) {
    const candidate = value as Record<string, unknown>;
    if (
      typeof candidate.summary === 'string' &&
      (candidate.details === undefined || typeof candidate.details === 'string') &&
      (candidate.outputs === undefined ||
        (isJsonValue(candidate.outputs) && !Array.isArray(candidate.outputs)))
    ) {
      return candidate as unknown as WorkflowResult;
    }
  }
  return { summary: 'Workflow completed', outputs: { value } };
}

export async function executeWorkflow(
  run: (ctx: WorkflowContext) => Promise<WorkflowReturn> | WorkflowReturn,
  options: ExecutionOptions = {}
): Promise<WorkflowExecution> {
  return reportExecution((ctx) => log.indented(() => run(ctx)), options);
}

/** Run a workflow without assuming a terminal, printing, or changing process state. */
export async function runWorkflow<Input, Output>(
  run: (ctx: WorkflowContext, input: Input) => Output,
  {
    input,
    verbose = false,
    onInput,
    onText,
    onEvent = () => {},
    timeout,
    signal,
    run: identity
  }: RunWorkflowOptions<Input>
): Promise<WorkflowExecution<Awaited<Output>>> {
  return withExecutionServices({ verbose }, () =>
    observeRunlingEvents(onEvent, () =>
      log.withDestination('silent', () =>
        captureExecution(
          (ctx) => log.indented(() => run(ctx, input)),
          onInput,
          timeout,
          signal,
          onText,
          false,
          identity
        )
      )
    )
  );
}

async function reportExecution(
  run: (ctx: WorkflowContext) => Promise<unknown> | unknown,
  { json = false, terminal = process.stdout, onInput, signal, journal }: ExecutionOptions = {}
): Promise<WorkflowExecution> {
  const execution = await log.withDestination(json ? 'stderr' : 'stdout', async () => {
    log.info(
      journal ? `Runling starting run ${log.highlight(journal.run.reference!)}` : 'Runling starting'
    );
    const identity = journal && { id: journal.run.id, reference: journal.run.reference };
    const capture = () =>
      captureExecution(run, onInput, undefined, signal, undefined, true, identity);
    const base = performance.now();
    const execution = await (journal
      ? observeRunlingEvents(
          // A write failure is reported when the finished record is written.
          (event) => void journal.writer.append(eventRecord(event, base)).catch(() => {}),
          capture
        )
      : capture());

    if (execution.error !== null) {
      log.error(execution.error);
      process.exitCode = 1;
    } else if (!json && execution.result !== null) {
      log.success(execution.result.summary);
      if (execution.result.details !== undefined) {
        console.log(`\n${formatWorkflowDetails(execution.result.details, terminal)}\n`);
      }
    }

    if (totalTokens(execution.usage) > 0) {
      log.info(`Total token usage: ${formatTokenUsage(execution.usage)}`);
    }
    log.info(`Finished in ${formatDuration(execution.durationMs)}`);
    if (journal) {
      const path = relative(process.cwd(), journal.writer.path);
      try {
        await journal.writer.append({
          type: 'finished',
          status: signal?.aborted ? 'cancelled' : execution.ok ? 'completed' : 'failed',
          finishedAt: Date.now(),
          durationMs: execution.durationMs,
          usage: execution.usage,
          output: execution.output,
          error: execution.error
        });
        log.info(`Run ${journal.run.reference} journal: ${path}`);
      } catch (cause) {
        log.error(`Cannot save the run journal ${path}: ${String(cause)}`);
      }
    }
    return execution;
  });

  if (json) console.log(JSON.stringify(execution));
  return execution;
}

/** Write updates that a root workflow emits to the log: a root workflow has no parent to report
 * to. A spawned child's updates go to its parent instead. Agent status and tool activity are skipped because the agent logs them, and
 * `output` is skipped because it repeats text that the agent logged. A state update is logged
 * only through its `activity`, and a repeated activity is logged once. */
export function createUpdateLogger(): (update: unknown) => Promise<void> {
  let lastActivity: string | undefined;
  return async (update) => {
    if (typeof update === 'string') {
      if (update.trim()) log.info(update);
      return;
    }
    if (typeof update !== 'object' || update === null || !('type' in update)) {
      log.debug(`Update: ${JSON.stringify(update)}`);
      return;
    }
    const value = update as {
      type: unknown;
      text?: unknown;
      activity?: unknown;
      activityLevel?: unknown;
    };
    if (
      (value.type === 'finding' || value.type === 'reply' || value.type === 'notice') &&
      typeof value.text === 'string'
    ) {
      log.info(value.text);
    } else if (value.type === 'state') {
      if (typeof value.activity !== 'string' || value.activity === lastActivity) return;
      lastActivity = value.activity;
      const level =
        value.activityLevel === 'success' || value.activityLevel === 'error'
          ? value.activityLevel
          : 'info';
      log[level](value.activity);
    } else if (!['output', 'tool', 'working', 'retrying', 'blocked'].includes(String(value.type))) {
      log.debug(`Update: ${JSON.stringify(update)}`);
    }
  };
}

/** Ask for workflow input on an interactive terminal, one line at a time. */
export const terminalInput: InputHandler = async (request) => {
  const prompt = createInterface({ input: process.stdin, output: process.stdout });
  // Without a listener, readline pauses on Ctrl-C instead of stopping the run.
  prompt.on('SIGINT', () => process.emit('SIGINT'));
  try {
    const suffix = request.defaultValue ? ` (${request.defaultValue})` : '';
    const answer = await prompt.question(`? ${request.message}${suffix} `, {
      ...(request.signal ? { signal: request.signal } : {})
    });
    return answer || request.defaultValue || '';
  } finally {
    prompt.close();
  }
};

async function captureExecution<Output>(
  run: (ctx: WorkflowContext) => Promise<Output> | Output,
  onInput?: InputHandler,
  timeout?: number,
  signal?: AbortSignal,
  onText?: TextHandler,
  logUpdates = false,
  identity?: RunIdentity
): Promise<WorkflowExecution<Awaited<Output>>> {
  const deadline = createTimeout(timeout, 'Workflow');
  const ctx = createObservedWorkflowContext(
    bindRunlingContext((usage: TokenUsage) => emitRunlingEvent({ type: 'usage.updated', usage })),
    signal && deadline.signal
      ? AbortSignal.any([signal, deadline.signal])
      : (signal ?? deadline.signal),
    identity
  );
  ctx.onInput = onInput;
  ctx.onText = onText;
  if (logUpdates) ctx.emit = createUpdateLogger();
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
    ok: error === null
  };
}

export async function loadWorkflow(path: string): Promise<TaskFunction> {
  const resolvedPath = resolve(path);
  const module = await import(/* @vite-ignore */ pathToFileURL(resolvedPath).href);
  if (!isTask(module.default)) {
    throw new Error(`Workflow ${resolvedPath} must have a default task export`);
  }
  return module.default;
}

export async function runRunling(workflowPath: string, prompt: string, options: RunOptions) {
  const { json, verbose } = options;
  // The first interrupt cancels the workflow so that its cleanup runs; a second one exits.
  const controller = new AbortController();
  const interrupt = () => {
    if (controller.signal.aborted) process.exit(130);
    log.error('Stopping. Press Ctrl-C again to exit without cleanup.');
    controller.abort(new Error('Interrupted'));
  };
  process.on('SIGINT', interrupt);
  process.on('SIGTERM', interrupt);
  let journal: ExecutionOptions['journal'];
  try {
    let input: unknown = prompt;
    if (options.input !== undefined) {
      try {
        input = JSON.parse(options.input);
      } catch {
        input = options.input; // The run reports the parse error.
      }
    }
    const run = newRun({ webhook: 'cli', workflow: workflowPath, source: 'cli', input });
    journal = {
      run,
      writer: await createRunJournal(await runJournalDirectory(process.cwd()), run)
    };
  } catch (cause) {
    log.error(`Cannot create a run journal: ${String(cause)}`);
  }
  try {
    await reportExecution(
      async (ctx) => {
        const run = await loadWorkflow(workflowPath);
        const input = options.input === undefined ? prompt : JSON.parse(options.input);
        return withExecutionServices({ verbose }, () => log.indented(() => run(ctx, input)));
      },
      {
        json,
        signal: controller.signal,
        ...(journal ? { journal } : {}),
        ...(process.stdin.isTTY ? { onInput: terminalInput } : {})
      }
    );
  } finally {
    process.off('SIGINT', interrupt);
    process.off('SIGTERM', interrupt);
  }
}
